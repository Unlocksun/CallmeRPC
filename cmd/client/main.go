package main

import (
	"GeeRPC/foo"
	"GeeRPC/service"
	"log"
	"sync"
)

func main() {
	// client
	addr := "124.223.48.188:9007"

	if client, err := service.Dial("tcp", addr); err != nil {
		log.Fatal("dial failed:", err)
	} else {
		// client会有routine负责数据接收. 主线程进行调用即可
		defer client.Close()
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(int) {
				defer wg.Done()
				args := &foo.Args{Num1: i, Num2: i * i}
				var reply int
				if err := client.Call("Foo.sum", args, &reply); err != nil {
					log.Fatal("call foo.sum failed: ", err)
				}
				log.Printf("Reply from server: %d + %d = %d", args.Num1, args.Num2, reply)
			}(i)
		}
		wg.Wait()
	}
}
