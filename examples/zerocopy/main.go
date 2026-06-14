// Command zerocopy demonstrates the zero-copy Buffer send path: fill a
// pre-registered buffer and send it without a bounce copy.
//
//	server: go run . -l :18515
//	client: go run . 10.214.180.34:18515
//
// Defaults match the gajl H20 GPU nodes: device mlx5_1 (GPU net xgbe1) and
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
		err = client(flag.Arg(0), opts)
	} else {
		log.Fatal("usage: zerocopy -l :PORT | zerocopy HOST:PORT")
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

func client(addr string, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(addr, opts...)
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
