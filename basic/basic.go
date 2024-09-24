package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	reader := strings.NewReader("Coding is interesting!")
	writer, _ := os.OpenFile("./output.log", os.O_RDWR, os.ModeAppend)
	p := make([]byte, 4)

	for {
		nr, err := reader.Read(p)
		if err != io.EOF {
			fmt.Printf("read : n: %d, err: %v\n", nr, err)
		} else if err == io.EOF {
			break
		}
		nw, err := writer.Write(p)
		if err == io.ErrShortWrite {
			fmt.Printf("write: n: %d, err: %v\n", nw, io.ErrShortWrite)
			break
		}
		fmt.Println()
		fmt.Println(nw, string(p))
	}

}
