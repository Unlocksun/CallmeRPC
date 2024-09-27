package main

import (
	"GeeRPC/foo"
	"GeeRPC/service"
	"log"
	"net"
)

func startServer() {
	l, err := net.Listen("tcp", ":9007")
	if err != nil {
		log.Fatal("network error!:", err)
	}
	log.Println("start rpc server!", l.Addr())

	var fooservice foo.Foo
	if err := service.Register(&fooservice); err != nil {
		log.Fatal("register error: ", err)
	}

	service.Accept(l)
}

func main() {
	startServer()
}
