package main

import (
	geerpc "GeeRPC"
	"log"
	"net"
)

func startServer(addr chan string) {
	l, err := net.Listen("tcp", ":9007")
	if err != nil {
		log.Fatal("network error!:", err)
	}
	log.Println("start rpc server!", l.Addr())
	addr <- l.Addr().String()
	geerpc.Accept(l)
}

func main() {
	addr := make(chan string)
	startServer(addr)
}
