// Command multi-client is an RC server that serves many clients sequentially in
// an Accept loop (rdma_cm), echoing one message per connection. Each accepted
// Conn is handled independently and closed before the next Accept.
//
//	server: go run . -l :18515
//	client: go run . 33.0.226.25:18515 -msg hello
//
// Defaults target the first GPU NIC: device mlx5_1 (GPU net xgbe1) and
// RoCE v2 GID index 3. Override with -d / -x (use -d mlx5_0 for CPU xgbe0).
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
	msg := flag.String("msg", "hello", "client: message to send")
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
		err = client(flag.Arg(0), *msg, opts)
	} else {
		log.Fatal("usage: multi-client -l :PORT | multi-client HOST:PORT -msg M")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func server(addr string, opts []rdmanet.Option) error {
	ln, err := rdmanet.Listen(addr, opts...)
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Printf("serving on %s — Ctrl-C to stop\n", ln.Addr())
	for i := 0; ; i++ {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		if err := handleConn(i, conn); err != nil {
			fmt.Printf("client %d: %v\n", i, err)
		}
	}
}

// handleConn echoes one message then closes, isolating each client.
func handleConn(id int, conn *rdmanet.Conn) error {
	defer conn.Close()
	msg, err := conn.RecvMsg()
	if err != nil {
		return err
	}
	fmt.Printf("client %d sent %q\n", id, msg)
	return conn.SendMsg(msg)
}

func client(addr, msg string, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(addr, opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SendMsg([]byte(msg)); err != nil {
		return err
	}
	reply, err := conn.RecvMsg()
	if err != nil {
		return err
	}
	fmt.Printf("echo: %s\n", reply)
	return nil
}
