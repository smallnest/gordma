// Command zerocopy demonstrates the zero-copy Buffer send path: fill a
// pre-registered buffer and send it without a bounce copy.
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
		log.Fatal("usage: zerocopy -l :PORT | zerocopy HOST:PORT")
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
	fmt.Printf("received %q (%d bytes)\n", msg, len(msg))
	return nil
}

func client(addr string) error {
	conn, err := rdmanet.Dial(addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	buf, err := conn.AllocBuffer(64)
	if err != nil {
		return err
	}
	defer buf.Close()
	copy(buf.Bytes(), []byte("zero-copy payload"))
	// SendBuffer transmits buf.Bytes() directly from registered memory with no
	// bounce copy, blocking until the send completes; buf is reusable on return.
	return conn.SendBuffer(buf)
}
