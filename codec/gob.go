package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	conn io.ReadWriteCloser // conn 是由构建函数传入，通常是通过 TCP 或者 Unix 建立 socket 时得到的链接实例
	buf  *bufio.Writer      // buf 是为了防止阻塞而创建的带缓冲的 Writer
	dec  *gob.Decoder
	enc  *gob.Encoder
}

// 确保 GobCodec 结构体实现了 Codec 接口
var _ Codec = (*GobCodec)(nil) // 将 nil 转换为 *GobCodec 类型的指针。这种写法通常用于表示一个空的、未初始化的指针

// gob的构造函数
func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		// 解码(读)可以直接进行, 编码(写)需要用buf来减少对conn的操作
		dec: gob.NewDecoder(conn),
		enc: gob.NewEncoder(buf),
	}
}

// 下面实现Gob的Codec接口
func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *GobCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err := c.enc.Encode(h); err != nil {
		log.Println("rpc codec: gob error encoding header: ", err)
		return err
	}
	if err := c.enc.Encode(body); err != nil {
		log.Println("rpc codec: gob error encoding body: ", err)
		return err
	}
	return nil
}

func (c *GobCodec) Close() error {
	return c.conn.Close()
}
