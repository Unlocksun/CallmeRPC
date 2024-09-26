package main

import (
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
	service.Accept(l)
}

func main() {
	startServer()
	var foo service.Foo
	if err := service.Register(&foo); err != nil {
		log.Fatal("register error: ", err)
	}
}
