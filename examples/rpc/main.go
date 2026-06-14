// Command rpc is a minimal request/response example: the client sends a request
// message and blocks for the reply; the server loops RecvMsg -> handle ->
// SendMsg. It demonstrates a simple one-question-one-answer pattern over RC.
//
//	server: go run . -l :18515
//	client: go run . 192.0.2.1:18515 -req "ping"
//
// Defaults target the first GPU NIC: device mlx5_1 (GPU net xgbe1) and
// RoCE v2 GID index 3. Override with -d / -x (use -d mlx5_0 for CPU xgbe0).
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
)

func main() {
	listen := flag.String("l", "", "listen address (server mode)")
	req := flag.String("req", "ping", "client: request payload")
	n := flag.Int("n", 3, "client: number of requests to send")
	device := flag.String("d", "mlx5_1", "RDMA device (mlx5_0=CPU/xgbe0, mlx5_1..8=GPU/xgbe1..8)")
	gidIndex := flag.Int("x", 3, "GID index (RoCE v2)")
	flag.Parse()
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	opts := []rdmanet.Option{rdmanet.WithDevice(*device), rdmanet.WithGIDIndex(*gidIndex)}
	var err error
	if *listen != "" {
		err = server(*listen, opts)
	} else if flag.NArg() > 0 {
		err = client(flag.Arg(0), *req, *n, opts)
	} else {
		log.Fatal("usage: rpc -l :PORT | rpc HOST:PORT -req MSG -n N")
	}
	if err != nil {
		log.Fatal(err)
	}
}

// handle is the trivial RPC handler: uppercase the request.
func handle(req []byte) []byte {
	return []byte("reply: " + strings.ToUpper(string(req)))
}

func server(addr string, opts []rdmanet.Option) error {
	ln, err := rdmanet.Listen(addr, opts...)
	if err != nil {
		return err
	}
	defer ln.Close()
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	for {
		req, err := conn.RecvMsg()
		if err == io.EOF {
			return nil // client done
		}
		if err != nil {
			return err
		}
		if err := conn.SendMsg(handle(req)); err != nil {
			return err
		}
	}
}

func client(addr, req string, n int, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(addr, opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	for i := 0; i < n; i++ {
		if err := conn.SendMsg([]byte(fmt.Sprintf("%s-%d", req, i))); err != nil {
			return err
		}
		reply, err := conn.RecvMsg()
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", reply)
	}
	return nil
}
