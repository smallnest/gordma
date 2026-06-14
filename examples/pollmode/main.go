// Command pollmode shows selecting the CQ poll mode (busy vs event) via
// WithPollMode. Pass -poll=busy or -poll=event.
//
//	server: go run . -l :18515 -poll=busy
//	client: go run . 10.0.0.1:18515 -poll=event
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
	poll := flag.String("poll", "event", "CQ poll mode: busy|event")
	flag.Parse()
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	mode := rdmanet.PollEvent
	if *poll == "busy" {
		mode = rdmanet.PollBusy
	}
	opt := rdmanet.WithPollMode(mode)
	fmt.Printf("poll mode: %s\n", mode)

	var err error
	if *listen != "" {
		err = server(*listen, opt)
	} else if flag.NArg() > 0 {
		err = client(flag.Arg(0), opt)
	} else {
		log.Fatal("usage: pollmode -l :PORT [-poll=busy|event] | pollmode HOST:PORT [-poll=...]")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func server(addr string, opt rdmanet.Option) error {
	ln, err := rdmanet.Listen(addr, opt)
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
	return conn.SendMsg(msg)
}

func client(addr string, opt rdmanet.Option) error {
	conn, err := rdmanet.Dial(addr, opt)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SendMsg([]byte("ping")); err != nil {
		return err
	}
	reply, err := conn.RecvMsg()
	if err != nil {
		return err
	}
	fmt.Printf("echo: %s\n", reply)
	return nil
}
