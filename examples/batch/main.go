// Command batch demonstrates SendBatch/RecvBatch for amortized message I/O.
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

const batchN = 8

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
		log.Fatal("usage: batch -l :PORT | batch HOST:PORT")
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
	got := 0
	for got < batchN {
		msgs, err := conn.RecvBatch(batchN - got)
		if err != nil {
			return err
		}
		got += len(msgs)
	}
	fmt.Printf("received %d messages\n", got)
	return nil
}

func client(addr string) error {
	conn, err := rdmanet.Dial(addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	msgs := make([][]byte, batchN)
	for i := range msgs {
		msgs[i] = []byte(fmt.Sprintf("msg-%d", i))
	}
	return conn.SendBatch(msgs)
}
