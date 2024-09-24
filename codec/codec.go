// 消息编解码
package codec

import "io"

type Header struct {
	ServiceMethod string // "Service.Method"
	Seq           uint64 // 请求ID
	Err           string
}

// 编解码的接口
type Codec interface {
	io.Closer
	ReadBody(interface{}) error // 解码后存在空接口类型中
	ReadHeader(*Header) error
	Write(*Header, interface{}) error // encodes and writes a message, consisting of a header and a body, to the underlying connection
}

// 抽象出构造函数
type NewCodecFunc func(io.ReadWriteCloser) Codec

type Type string

// 两种编码方式
const (
	GobType  Type = "application/gob"
	JsonType Type = "application/json"
)

// 根据type获得构造函数
var NewCodecFuncMap map[Type]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
}
