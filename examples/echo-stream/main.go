// Command echo-stream uses rdmanet's byte-stream adapter (Read/Write,
// io.ReadWriteCloser) instead of message semantics.
//
//	server: go run . -l :18515
//	client: go run . 33.0.226.25:18515
//
// Defaults target the first GPU NIC: device mlx5_1 (GPU net xgbe1) and
// RoCE v2 GID index 3. Override with -d / -x (use -d mlx5_0 for CPU xgbe0).
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
		log.Fatal("usage: echo-stream -l :PORT | echo-stream HOST:PORT")
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
	buf := make([]byte, 32)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	_, err = conn.Write(buf[:n]) // echo bytes
	return err
}

func client(addr string, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(addr, opts...)
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
