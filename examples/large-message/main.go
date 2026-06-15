// Command large-message sends a single multi-megabyte message to demonstrate
// transparent fragmentation/reassembly with message boundaries preserved, and
// tuning via WithBufferSize/WithQueueDepth.
//
//	server: go run . -l :18515
//	client: go run . 33.0.226.25:18515 --size 16
//
// Defaults target the first GPU NIC: device mlx5_1 (GPU net xgbe1) and
// RoCE v2 GID index 3. Override with -d / -x (use -d mlx5_0 for CPU xgbe0).
package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"log"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
	flag "github.com/spf13/pflag"
)

var (
	listen   = flag.StringP("listen", "l", "", "listen address (server mode)")
	sizeMiB  = flag.Int("size", 16, "client: message size in MiB")
	bufKiB   = flag.Int("buf", 256, "per-frame bounce buffer size in KiB")
	depth    = flag.Int("depth", 256, "send/recv queue depth")
	device   = flag.StringP("device", "d", "mlx5_1", "RDMA device (mlx5_0=CPU/xgbe0, mlx5_1..8=GPU/xgbe1..8)")
	gidIndex = flag.IntP("gid-index", "x", 3, "GID index (RoCE v2)")
)

func main() {
	flag.Parse()
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	opts := []rdmanet.Option{
		rdmanet.WithDevice(*device),
		rdmanet.WithGIDIndex(*gidIndex),
		rdmanet.WithBufferSize(*bufKiB * 1024),
		rdmanet.WithQueueDepth(*depth),
	}
	var err error
	if *listen != "" {
		err = server(*listen, opts)
	} else if flag.NArg() > 0 {
		err = client(flag.Arg(0), *sizeMiB, opts)
	} else {
		log.Fatal("usage: large-message -l :PORT | large-message HOST:PORT --size 16")
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
	// One RecvMsg returns the whole multi-MiB message, reassembled from many
	// frames, with its boundary intact.
	msg, err := conn.RecvMsg()
	if err != nil {
		return err
	}
	fmt.Printf("received %d bytes, sha256=%x\n", len(msg), sha256.Sum256(msg))
	return nil
}

func client(addr string, sizeMiB int, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(addr, opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	payload := bytes.Repeat([]byte("gordma-rdmanet!"), sizeMiB*1024*1024/15+1)[:sizeMiB*1024*1024]
	fmt.Printf("sending %d bytes, sha256=%x\n", len(payload), sha256.Sum256(payload))
	// A single SendMsg; the transport fragments it into frames under flow
	// control and the peer reassembles it into one message.
	return conn.SendMsg(payload)
}
