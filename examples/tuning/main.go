// Command tuning shows every rdmanet Option in one place with guidance on when
// each matters. It prints the resolved configuration, then runs a small echo so
// you can experiment with flags.
//
//	server: go run . -l :18515
//	client: go run . 192.0.2.1:18515
//
// Flags map 1:1 to options:
//
//	-d        WithDevice      which NIC (mlx5_0=CPU/xgbe0, mlx5_1..8=GPU/xgbe1..8)
//	-i        WithPort        HCA port number (usually 1)
//	-x        WithGIDIndex    GID table index (RoCE v2 = 3 here)
//	-depth    WithQueueDepth  outstanding WRs / flow-control credits
//	-buf      WithBufferSize  per-frame bounce slot bytes (KiB)
//	-poll     WithPollMode    event (low CPU) | busy (low latency)
//	-handshake WithHandshake  TCP out-of-band establishment instead of rdma_cm
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
	device := flag.String("d", "mlx5_1", "WithDevice: NIC (mlx5_0=CPU/xgbe0, mlx5_1..8=GPU/xgbe1..8)")
	port := flag.Int("i", 1, "WithPort: HCA port number")
	gidIndex := flag.Int("x", 3, "WithGIDIndex: GID table index (RoCE v2)")
	depth := flag.Int("depth", 128, "WithQueueDepth: outstanding WRs / credits")
	bufKiB := flag.Int("buf", 64, "WithBufferSize: per-frame bounce slot (KiB)")
	poll := flag.String("poll", "event", "WithPollMode: event|busy")
	handshake := flag.Bool("handshake", false, "WithHandshake: TCP out-of-band establishment")
	flag.Parse()
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}

	mode := rdmanet.PollEvent
	if *poll == "busy" {
		mode = rdmanet.PollBusy
	}
	opts := []rdmanet.Option{
		rdmanet.WithDevice(*device),
		rdmanet.WithPort(*port),
		rdmanet.WithGIDIndex(*gidIndex),
		rdmanet.WithQueueDepth(*depth),
		rdmanet.WithBufferSize(*bufKiB * 1024),
		rdmanet.WithPollMode(mode),
	}
	if *handshake {
		opts = append(opts, rdmanet.WithHandshake())
	}
	fmt.Printf("config: device=%s port=%d gidIndex=%d depth=%d buf=%dKiB poll=%s handshake=%v\n",
		*device, *port, *gidIndex, *depth, *bufKiB, mode, *handshake)

	var err error
	if *listen != "" {
		err = server(*listen, opts)
	} else if flag.NArg() > 0 {
		err = client(flag.Arg(0), opts)
	} else {
		log.Fatal("usage: tuning -l :PORT [flags] | tuning HOST:PORT [flags]")
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
	return conn.SendMsg(msg)
}

func client(addr string, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(addr, opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SendMsg([]byte("tuned")); err != nil {
		return err
	}
	reply, err := conn.RecvMsg()
	if err != nil {
		return err
	}
	fmt.Printf("echo: %s\n", reply)
	return nil
}
