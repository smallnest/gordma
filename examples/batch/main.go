// Command batch demonstrates SendBatch/RecvBatch for amortized message I/O.
//
//	server: go run . -l :18515
//	client: go run . 192.0.2.1:18515
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

const batchN = 8

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
		log.Fatal("usage: batch -l :PORT | batch HOST:PORT")
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

func client(addr string, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(addr, opts...)
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
