package service

import (
	"GeeRPC/codec"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type Call struct {
	Seq           uint64 // 唯一标识
	ServiceMethod string
	Args          interface{}
	Reply         interface{}
	Error         error
	Done          chan *Call // 异步调用时, 用于通知用户完成
}

// 通知客户端调用结束
func (call *Call) done() {
	call.Done <- call
}

type Client struct {
	cc       codec.Codec
	opt      *Option
	sending  sync.Mutex   // 保证请求有序发送
	header   codec.Header // 由于发送是互斥的, 所以客户端所有请求复用一个header就行
	mu       sync.Mutex   //protect following
	seq      uint64
	pending  map[uint64]*Call
	closing  bool // 用户主动关闭
	shutdown bool // 发生错误关闭
}

var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("connection is already shut down")

// 关闭客户端
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrShutdown
	}
	client.closing = true
	return client.cc.Close()
}

// 检查是否可用
func (client *Client) IsAvalable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.closing && !client.shutdown
}

// 用户参数配置
func parseOptions(opts ...*Option) (*Option, error) {
	// if opts is nil or pass nil as parameter
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.OptionIdentify = DefaultOption.OptionIdentify
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	return opt, nil
}

// 超时处理添加
type clientResult struct {
	client *Client
	err    error
}

type newClientFunc func(conn net.Conn, opt *Option) (client *Client, err error)

func dialTimeout(f newClientFunc, network, addr string, opts ...*Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	// 建立tcp连接, 如果超时返回错误
	conn, err := net.DialTimeout(network, addr, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	ch := make(chan clientResult)
	// 通过子协程创建新的client
	go func() {
		client, err := f(conn, opt)
		ch <- clientResult{client: client, err: err}
	}()
	// 无超时限制时
	if opt.ConnectTimeout == 0 {
		result := <-ch
		return result.client, result.err
	}
	// 有超时限制时
	select {
	// 如果超时, 说明未能在指定时间创建客户端s
	case <-time.After(opt.ConnectTimeout):
		return nil, fmt.Errorf("rpc client: connect timeout: expect within %s", opt.ConnectTimeout)
	case result := <-ch:
		return result.client, result.err
	}
}

// 连接指定地址的rpc server
func Dial(network, address string, opts ...*Option) (client *Client, err error) {
	return dialTimeout(NewClient, network, address, opts...)
}

// 启动client
func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}

	// 协议交换
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: option error: ", err)
		_ = conn.Close()
		return nil, err
	}
	time.Sleep(time.Second)
	return NewClientWithCodec(f(conn), opt), nil
}

// 创建实例并起routine进行数据接受
func NewClientWithCodec(codec codec.Codec, opt *Option) *Client {
	client := &Client{
		seq:     1,
		cc:      codec,
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}

// 接受响应
func (client *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header

		if err = client.cc.ReadHeader(&h); err != nil {
			break
		}
		call := client.removeCall(h.Seq)

		switch {
		case call == nil:
			// call已经被提走了
			err = client.cc.ReadBody(nil)
		case h.Err != "":
			// 服务器处理调用出错
			call.Error = fmt.Errorf(h.Err)
			err = client.cc.ReadBody(nil)
			call.done()

		default:
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body" + err.Error())
			}
			call.done()
		}
	}
	// error occurs or EOF
	client.terminateCall(err)
}

// 异步调用方法
func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered!")
	}

	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	client.send(call)
	return call
}

// 同步接口, 阻塞了call.Done
func (client *Client) Call(serviceMethod string, args, reply interface{}) error {
	call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}

// 发送调用请求
func (client *Client) send(call *Call) {
	client.sending.Lock()
	defer client.sending.Unlock()

	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	// request header
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Err = ""

	if err := client.cc.Write(&client.header, call.Args); err != nil {
		call := client.removeCall(seq)
		// call 如果为空, 说明write部分失败, 但客户端仍然收到了响应并处理了call
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// 将call加入pending队列, 并更新序列号
func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, ErrShutdown
	}

	call.Seq = client.seq
	client.pending[call.Seq] = call
	client.seq++
	return call.Seq, nil
}

// 从队列中取出call
func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending, seq)
	return call
}

// 服务端或客户端发生错误时调用，将 shutdown 设置为 true，且将错误信息通知所有 pending 状态的 call
func (client *Client) terminateCall(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
}
