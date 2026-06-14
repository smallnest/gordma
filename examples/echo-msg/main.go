// Command echo-msg is a minimal RC message-semantics echo over rdmanet.
//
//	server: go run . -l :18515
//	client: go run . 10.0.0.1:18515
package main

import (
	"flag"
	"fmt"
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
		log.Fatal("usage: echo-msg -l :PORT  (server) | echo-msg HOST:PORT (client)")
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
	msg, err := conn.RecvMsg()
	if err != nil {
		return err
	}
	return conn.SendMsg(msg) // echo
}

func client(addr string) error {
	conn, err := rdmanet.Dial(addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SendMsg([]byte("hello rdmanet")); err != nil {
		return err
	}
	reply, err := conn.RecvMsg()
	if err != nil {
		return err
	}
	fmt.Printf("echo: %s\n", reply)
	return nil
}
