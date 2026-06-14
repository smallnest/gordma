// Command bench-throughput is a minimal in-example throughput self-test: the
// client sends N messages two ways — one-at-a-time with SendMsg and in groups
// with SendBatch — and reports Gb/s for each so you can see the batching
// benefit. The server drains with RecvBatch.
//
//	server: go run . -l :18515
//	client: go run . 192.0.2.1:18515 -n 100000 -s 1024
//
// Defaults target the first GPU NIC: device mlx5_1 (GPU net xgbe1) and
// RoCE v2 GID index 3. Override with -d / -x (use -d mlx5_0 for CPU xgbe0).
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
)

func main() {
	listen := flag.String("l", "", "listen address (server mode)")
	n := flag.Int("n", 100000, "number of messages")
	size := flag.Int("s", 1024, "message size in bytes")
	batch := flag.Int("b", 32, "batch size for the SendBatch pass")
	device := flag.String("d", "mlx5_1", "RDMA device (mlx5_0=CPU/xgbe0, mlx5_1..8=GPU/xgbe1..8)")
	gidIndex := flag.Int("x", 3, "GID index (RoCE v2)")
	flag.Parse()
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	opts := []rdmanet.Option{
		rdmanet.WithDevice(*device),
		rdmanet.WithGIDIndex(*gidIndex),
		rdmanet.WithBufferSize(*size + 64),
	}
	var err error
	if *listen != "" {
		err = server(*listen, *n, opts)
	} else if flag.NArg() > 0 {
		err = client(flag.Arg(0), *n, *size, *batch, opts)
	} else {
		log.Fatal("usage: bench-throughput -l :PORT | bench-throughput HOST:PORT -n N -s SIZE")
	}
	if err != nil {
		log.Fatal(err)
	}
}

// server drains 2*n messages (one set per client pass) via RecvBatch.
func server(addr string, n int, opts []rdmanet.Option) error {
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
	for got := 0; got < 2*n; {
		msgs, err := conn.RecvBatch(256)
		if err != nil {
			return err
		}
		got += len(msgs)
	}
	fmt.Println("server drained both passes")
	return nil
}

func client(addr string, n, size, batch int, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(addr, opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	msg := make([]byte, size)

	// Pass 1: one message at a time.
	t0 := time.Now()
	for i := 0; i < n; i++ {
		if err := conn.SendMsg(msg); err != nil {
			return err
		}
	}
	report("SendMsg ", n, size, time.Since(t0))

	// Pass 2: batched.
	group := make([][]byte, batch)
	for i := range group {
		group[i] = msg
	}
	t1 := time.Now()
	for sent := 0; sent < n; sent += batch {
		k := batch
		if sent+k > n {
			k = n - sent
		}
		if err := conn.SendBatch(group[:k]); err != nil {
			return err
		}
	}
	report("SendBatch", n, size, time.Since(t1))
	return nil
}

func report(label string, n, size int, d time.Duration) {
	gbps := float64(n) * float64(size) * 8 / d.Seconds() / 1e9
	mps := float64(n) / d.Seconds() / 1e6
	fmt.Printf("%s: %d x %d B in %v -> %.2f Gb/s, %.2f Mmsg/s\n", label, n, size, d, gbps, mps)
}
