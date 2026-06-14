// Command file-transfer streams a file from client to server using Conn as a
// plain io.ReadWriteCloser (io.Copy), demonstrating the byte-stream adapter for
// bulk transfers with a graceful EOF.
//
//	server: go run . -l :18515 -out received.bin
//	client: go run . 192.0.2.1:18515 -in payload.bin
//
// Defaults target the first GPU NIC: device mlx5_1 (GPU net xgbe1) and
// RoCE v2 GID index 3. Override with -d / -x (use -d mlx5_0 for CPU xgbe0).
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
)

func main() {
	listen := flag.String("l", "", "listen address (server mode)")
	out := flag.String("out", "received.bin", "server: output file path")
	in := flag.String("in", "", "client: input file path to send")
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
		err = server(*listen, *out, opts)
	} else if flag.NArg() > 0 {
		err = client(flag.Arg(0), *in, opts)
	} else {
		log.Fatal("usage: file-transfer -l :PORT -out FILE | file-transfer HOST:PORT -in FILE")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func server(addr, outPath string, opts []rdmanet.Option) error {
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
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	// Conn is an io.Reader; copy until the peer closes (io.EOF).
	n, err := io.Copy(f, conn)
	if err != nil && err != io.EOF {
		return err
	}
	fmt.Printf("received %d bytes -> %s\n", n, outPath)
	return nil
}

func client(addr, inPath string, opts []rdmanet.Option) error {
	if inPath == "" {
		return fmt.Errorf("client requires -in FILE")
	}
	f, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer f.Close()
	conn, err := rdmanet.Dial(addr, opts...)
	if err != nil {
		return err
	}
	// Conn is an io.Writer; stream the file, then Close sends FIN so the
	// server's io.Copy sees EOF.
	n, err := io.Copy(conn, f)
	if err != nil {
		_ = conn.Close()
		return err
	}
	fmt.Printf("sent %d bytes\n", n)
	return conn.Close()
}
