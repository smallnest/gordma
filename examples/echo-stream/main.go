// Command echo-stream uses rdmanet's byte-stream adapter (Read/Write,
// io.ReadWriteCloser) instead of message semantics.
//
//	server: go run . -l :18515
//	client: go run . 10.0.0.1:18515
package main

import (
	"flag"
	"fmt"
	"io"
	"log"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
)

func main() {
	listen := flag.String("l", "", "listen address (server mode)")
	flag.Parse()
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	var err error
	if *listen != "" {
		err = server(*listen)
	} else if flag.NArg() > 0 {
		err = client(flag.Arg(0))
	} else {
		log.Fatal("usage: echo-stream -l :PORT | echo-stream HOST:PORT")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func server(addr string) error {
	ln, err := rdmanet.Listen(addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	buf := make([]byte, 32)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	_, err = conn.Write(buf[:n]) // echo bytes
	return err
}

func client(addr string) error {
	conn, err := rdmanet.Dial(addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "streamed bytes"); err != nil {
		return err
	}
	buf := make([]byte, 32)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	fmt.Printf("echo: %s\n", buf[:n])
	return nil
}
