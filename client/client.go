package main

import (
	geerpc "GeeRPC"
	"GeeRPC/codec"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

func main() {
	// client
	// 用chan确保阻塞在连接前
	addr := make(chan string)
	addr <- "124.223.48.188:9007"
	conn, _ := net.Dial("tcp", <-addr)
	defer func() {
		_ = conn.Close()
	}()

	time.Sleep(time.Second)

	log.Println("start the client")

	// send option, 协议交换
	_ = json.NewEncoder(conn).Encode(geerpc.DefaultOption)
	cc := codec.NewGobCodec(conn)

	for i := 0; i < 5; i++ {
		// 发送header和body
		h := &codec.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
			Err:           "nil",
		}
		_ = cc.Write(h, fmt.Sprintf("geerpc req %d", h.Seq))

		// 读取响应
		_ = cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply:", reply)
	}
}
