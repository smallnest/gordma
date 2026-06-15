// Command go_rdmanet_bw measures message bandwidth over the rdmanet high-level
// API (in contrast to the lower-level perftest tools). Run without an address
// to act as the server (echoes/receives); pass the server address to act as the
// client (sends).
//
// Requires RDMA hardware (Linux + libibverbs/librdmacm). On unsupported
// platforms it exits with an error.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/smallnest/gordma/rdmanet"
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
		device   = fs.String("d", "", "RDMA device name (default: first available)")
		port     = fs.Int("i", 1, "HCA port number")
		gidIndex = fs.Int("x", 0, "GID index (RoCE v2)")
		size     = fs.Int("s", 65536, "message size in bytes")
		iters    = fs.Int("n", 1000, "number of messages")
		tcpPort  = fs.Int("p", 18515, "TCP listen/connect port for establishment")
		handshk  = fs.Bool("handshake", false, "use TCP out-of-band handshake instead of rdma_cm")
		pollMode = fs.String("poll", "event", "CQ poll mode: busy|event")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts, err := buildOptions(*device, *port, *gidIndex, *size, *handshk, *pollMode)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("0.0.0.0:%d", *tcpPort)
	if fs.NArg() > 0 {
		// client
		return runClientBW(fs.Arg(0), *size, *iters, opts)
	}
	return runServerBW(addr, *size, *iters, opts)
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

func runServerBW(addr string, size, iters int, opts []rdmanet.Option) error {
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

	buf := make([]byte, size)
	for i := 0; i < iters; i++ {
		if _, err := conn.RecvMsgBuf(buf); err != nil {
			return err
		}
	}
	fmt.Println("server received", iters, "messages")
	return nil
}

func runClientBW(server string, size, iters int, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(server, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	msg := make([]byte, size)
	start := time.Now()
	for i := 0; i < iters; i++ {
		if err := conn.SendMsg(msg); err != nil {
			return err
		}
	}
	elapsed := time.Since(start)
	totalBytes := float64(size) * float64(iters)
	gbps := totalBytes * 8 / elapsed.Seconds() / 1e9
	fmt.Printf("sent %d x %d bytes in %v: %.2f Gb/s\n", iters, size, elapsed, gbps)
	return nil
}
