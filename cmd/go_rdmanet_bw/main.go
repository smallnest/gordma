// Command go_rdmanet_bw measures message bandwidth over the rdmanet high-level
// API (in contrast to the lower-level perftest tools). Run without an address
// to act as the server (echoes/receives); pass the server address to act as the
// client (sends).
//
// By default it sends one message at a time with SendMsg, which is latency-
// bound (one message in flight). Pass --batch N to use SendBatch/RecvBatch,
// keeping N messages in flight per call — much closer to the perftest tools'
// pipelined throughput. Combine with --poll busy for the lowest overhead.
//
// Requires RDMA hardware (Linux + libibverbs/librdmacm). On unsupported
// platforms it exits with an error.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/smallnest/gordma/rdmanet"
	flag "github.com/spf13/pflag"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "go_rdmanet_bw: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("go_rdmanet_bw", flag.ContinueOnError)
	var (
		device   = fs.StringP("device", "d", "", "RDMA device name (default: first available)")
		port     = fs.IntP("ib-port", "i", 1, "HCA port number")
		gidIndex = fs.IntP("gid-index", "x", 0, "GID index (RoCE v2)")
		size     = fs.IntP("size", "s", 65536, "message size in bytes")
		iters    = fs.IntP("count", "n", 1000, "number of messages")
		batch    = fs.IntP("batch", "b", 1, "messages per SendBatch/RecvBatch (1 = one-at-a-time SendMsg)")
		tcpPort  = fs.IntP("tcp-port", "p", 18515, "TCP listen/connect port for establishment")
		handshk  = fs.Bool("handshake", false, "use TCP out-of-band handshake instead of rdma_cm")
		pollMode = fs.String("poll", "event", "CQ poll mode: busy|event")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *batch < 1 {
		return fmt.Errorf("invalid --batch %d (want >= 1)", *batch)
	}
	opts, err := buildOptions(*device, *port, *gidIndex, *size, *handshk, *pollMode)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("0.0.0.0:%d", *tcpPort)
	if fs.NArg() > 0 {
		// client
		return runClientBW(fs.Arg(0), *size, *iters, *batch, opts)
	}
	return runServerBW(addr, *size, *iters, *batch, opts)
}

// buildOptions translates the common flags into rdmanet.Options shared by both
// the bw and lat tools.
func buildOptions(device string, port, gidIndex, size int, handshake bool, pollMode string) ([]rdmanet.Option, error) {
	opts := []rdmanet.Option{
		rdmanet.WithPort(port),
		rdmanet.WithGIDIndex(gidIndex),
		rdmanet.WithBufferSize(size + 64), // room for the frame header
	}
	if device != "" {
		opts = append(opts, rdmanet.WithDevice(device))
	}
	if handshake {
		opts = append(opts, rdmanet.WithHandshake())
	}
	switch pollMode {
	case "busy":
		opts = append(opts, rdmanet.WithPollMode(rdmanet.PollBusy))
	case "event":
		opts = append(opts, rdmanet.WithPollMode(rdmanet.PollEvent))
	default:
		return nil, fmt.Errorf("invalid -poll %q (want busy|event)", pollMode)
	}
	return opts, nil
}

func runServerBW(addr string, size, iters, batch int, opts []rdmanet.Option) error {
	ln, err := rdmanet.Listen(addr, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()
	fmt.Printf("go_rdmanet_bw server listening on %s\n", ln.Addr())
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if batch > 1 {
		for got := 0; got < iters; {
			msgs, err := conn.RecvBatch(batch)
			if err != nil {
				return err
			}
			got += len(msgs)
		}
	} else {
		buf := make([]byte, size)
		for i := 0; i < iters; i++ {
			if _, err := conn.RecvMsgBuf(buf); err != nil {
				return err
			}
		}
	}
	fmt.Println("server received", iters, "messages")
	return nil
}

func runClientBW(server string, size, iters, batch int, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(server, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	msg := make([]byte, size)
	start := time.Now()
	if batch > 1 {
		group := make([][]byte, batch)
		for i := range group {
			group[i] = msg
		}
		for sent := 0; sent < iters; sent += batch {
			k := batch
			if sent+k > iters {
				k = iters - sent
			}
			if err := conn.SendBatch(group[:k]); err != nil {
				return err
			}
		}
	} else {
		for i := 0; i < iters; i++ {
			if err := conn.SendMsg(msg); err != nil {
				return err
			}
		}
	}
	elapsed := time.Since(start)
	totalBytes := float64(size) * float64(iters)
	gbps := totalBytes * 8 / elapsed.Seconds() / 1e9
	mpps := float64(iters) / elapsed.Seconds() / 1e6
	mode := "SendMsg"
	if batch > 1 {
		mode = fmt.Sprintf("SendBatch(%d)", batch)
	}
	fmt.Printf("%s: sent %d x %d bytes in %v: %.2f Gb/s, %.3f Mpps\n",
		mode, iters, size, elapsed, gbps, mpps)
	return nil
}
