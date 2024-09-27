package service

import (
	"GeeRPC/codec"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"
)

// GeeRPC 客户端固定采用 JSON 编码 Option，后续的 header 和 body 的编码方式由 Option 中的 CodeType 指定
// | Option{MagicNumber: xxx, CodecType: xxx} | Header{ServiceMethod ...} | Body interface{} |
// | <------      固定 JSON 编码      ------>  | <-------   编码方式由 CodeType 决定   ------->|
const Identify = 0x31dfa9

// 协议协商信息
type Option struct {
	OptionIdentify int //标识这是个geerpc包
	CodecType      codec.Type
	ConnectTimeout time.Duration // 0为无限制
	HandleTimeout  time.Duration
}

var DefaultOption = &Option{
	OptionIdentify: Identify,
	CodecType:      codec.GobType,
	ConnectTimeout: time.Second * 10,
}

type Server struct {
	serviceMap sync.Map
}

func NewServer() *Server {
	return &Server{}
}

// 提供一个全局的默认Server示例, 类似单例模式
var DefaultServer = NewServer()

// 注册方法
func (server *Server) Register(receiver interface{}) error {
	s := newService(receiver)
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return errors.New("rpc server: service already defined: " + s.name)
	}
	return nil
}

// 默认Server的注册
func Register(receiver interface{}) error { return DefaultServer.Register(receiver) }

// 根据传入的方法字符寻找对应的方法
func (server *Server) findServiceDotMethod(serviceMethod string) (svc *service, mtype *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]

	// 寻找service
	svci, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service " + serviceName)
		return
	}
	svc = svci.(*service)
	// 寻找service的methodName方法
	mtype = svc.method[methodName]
	if mtype == nil {
		err = errors.New("rpc server: can't find method " + methodName)
		return svc, mtype, err
	}
	return svc, mtype, nil
}

// 处理新的连接
func (server *Server) Accept(lis net.Listener) {
	// 循环监听
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc server: accept err: ", err)
			return
		}
		// 利用协程处理
		go server.ServeConn(conn)
	}
}

func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
}

// 协程连接处理
func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }()

	var opt Option
	// json解码出opt
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: option error: ", err)
		return
	}

	if opt.OptionIdentify != Identify {
		log.Printf("rpc server: invalid identifer %x", opt.OptionIdentify)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
		return
	}

	// 根据opt进行head和body解码
	// f(conn)返回一个具体类型的解编码接口
	server.serverCodec(f(conn))
}

// 存储调请求的信息
type request struct {
	h            *codec.Header
	argv, replyv reflect.Value // 由于编码方式要到运行时确定, 所以用reflect.Value类型
	mtype        *methodType
	svc          *service
}

// invalidRequest is a placeholder for response argv when error occurs
var invalidRequest = struct{}{}

// 读取, 处理, 回复请求
func (server *Server) serverCodec(cc codec.Codec) {
	sending := new(sync.Mutex) // 保证回复报文不会交织
	wg := new(sync.WaitGroup)  // 类似于信号量, 确保goroutine在关闭连接前已经全部handleRequest结束
	for {
		req, err := server.readRequest(cc)
		if err != nil {
			// 解析失败, 结束循环
			if req == nil {
				break
			}
			req.h.Err = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		// 新起routine处理请求
		go server.handleRequest(cc, req, sending, wg)
	}
	wg.Wait()
	_ = cc.Close()
}

func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		// 读完了
		return nil, err
	}
	return &h, nil
}

func (server *Server) readRequest(cc codec.Codec) (req *request, err error) {
	h, err := server.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req = &request{h: h}
	// 根据header找到对应服务
	req.svc, req.mtype, err = server.findServiceDotMethod(h.ServiceMethod)
	if err != nil {
		return req, err
	}
	req.argv = req.mtype.newArgv()
	req.replyv = req.mtype.newReplyv()

	// 读取输入参数
	// make sure that argvi is a pointer, ReadBody need a pointer as parameter
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}

	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read argv error:", err)
		return req, err
	}

	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error: ", err)
	}
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	err := req.svc.call(req.mtype, req.argv, req.replyv)
	if err != nil {
		req.h.Err = err.Error()
		server.sendResponse(cc, req.h, invalidRequest, sending)
		return
	}
	server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
}
