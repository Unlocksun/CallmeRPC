package main

import (
	geerpc "GeeRPC"
	"fmt"
	"log"
	"sync"
)

func main() {
	// client
	addr := "124.223.48.188:9007"

	client, _ := geerpc.Dial("tcp", addr)

	defer client.Close()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(int) {
			defer wg.Done()
			args := fmt.Sprintf("geerpc req %d", i)
			var reply string
			if err := client.Call("Foo.sum", args, &reply); err != nil {
				log.Fatal("call foo.sum failed: ", err)
			}
			log.Println("reply: ", reply)
		}(i)
	}
	wg.Wait()
}
