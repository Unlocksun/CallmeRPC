package main

import (
	geerpc "GeeRPC"
	"log"
	"net"
)

func startServer() {
	l, err := net.Listen("tcp", ":9007")
	if err != nil {
		log.Fatal("network error!:", err)
	}
	log.Println("start rpc server!", l.Addr())
	geerpc.Accept(l)
}

func main() {
	startServer()
}
