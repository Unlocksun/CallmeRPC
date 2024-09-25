package geerpc

import (
	"GeeRPC/codec"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

// GeeRPC 客户端固定采用 JSON 编码 Option，后续的 header 和 body 的编码方式由 Option 中的 CodeType 指定
// | Option{MagicNumber: xxx, CodecType: xxx} | Header{ServiceMethod ...} | Body interface{} |
// | <------      固定 JSON 编码      ------>  | <-------   编码方式由 CodeType 决定   ------->|
const Identify = 0x31dfa9

// 协议协商信息
type Option struct {
	OptionIdentify int //标识这是个geerpc包
	CodecType      codec.Type
}

var DefaultOption = &Option{
	OptionIdentify: Identify,
	CodecType:      codec.GobType,
}

type Server struct{}

func NewServer() *Server {
	return &Server{}
}

// 提供一个全局的默认Server示例, 类似单例模式
var DefaultServer = NewServer()

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
	// 暂且当是string
	req.argv = reflect.New(reflect.TypeOf(""))
	if err = cc.ReadBody(req.argv.Interface()); err != nil {
		log.Println("rpc server: read argv error:", err)
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
	// 注册rpc方式获得回复内容, 目前打印hello就行
	defer wg.Done()
	log.Println(req.h, req.argv.Elem())
	req.replyv = reflect.ValueOf(fmt.Sprintf("geerpc resp %d", req.h.Seq))
	server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
}
