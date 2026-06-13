// Command go-rdmanet_lat measures round-trip message latency over the rdmanet
// high-level API. Run without an address to act as the server (ping-pong
// responder); pass the server address to act as the client (initiator), which
// prints round-trip latency statistics.
//
// Requires RDMA hardware (Linux + libibverbs/librdmacm). On unsupported
// platforms it exits with an error.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/smallnest/gordma/rdmanet"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "go-rdmanet_lat: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("go-rdmanet_lat", flag.ContinueOnError)
	var (
		device   = fs.String("d", "", "RDMA device name (default: first available)")
		port     = fs.Int("i", 1, "HCA port number")
		gidIndex = fs.Int("x", 0, "GID index (RoCE v2)")
		size     = fs.Int("s", 64, "message size in bytes")
		iters    = fs.Int("n", 1000, "number of ping-pong round trips")
		tcpPort  = fs.Int("p", 18515, "TCP listen/connect port for establishment")
		handshk  = fs.Bool("handshake", false, "use TCP out-of-band handshake instead of rdma_cm")
		pollMode = fs.String("poll", "event", "CQ poll mode: busy|event")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts, err := buildLatOptions(*device, *port, *gidIndex, *size, *handshk, *pollMode)
	if err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return runClientLat(fs.Arg(0), *size, *iters, opts)
	}
	return runServerLat(fmt.Sprintf("0.0.0.0:%d", *tcpPort), *size, *iters, opts)
}

func buildLatOptions(device string, port, gidIndex, size int, handshake bool, pollMode string) ([]rdmanet.Option, error) {
	opts := []rdmanet.Option{
		rdmanet.WithPort(port),
		rdmanet.WithGIDIndex(gidIndex),
		rdmanet.WithBufferSize(size + 64),
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

// runServerLat echoes each received message back (ping-pong responder).
func runServerLat(addr string, size, iters int, opts []rdmanet.Option) error {
	ln, err := rdmanet.Listen(addr, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()
	fmt.Printf("go-rdmanet_lat server listening on %s\n", ln.Addr())
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	buf := make([]byte, size)
	for i := 0; i < iters; i++ {
		n, err := conn.RecvMsgBuf(buf)
		if err != nil {
			return err
		}
		if err := conn.SendMsg(buf[:n]); err != nil {
			return err
		}
	}
	return nil
}

// runClientLat sends a message and waits for its echo, timing each round trip.
func runClientLat(server string, size, iters int, opts []rdmanet.Option) error {
	conn, err := rdmanet.Dial(server, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	msg := make([]byte, size)
	buf := make([]byte, size)
	lats := make([]time.Duration, 0, iters)
	for i := 0; i < iters; i++ {
		start := time.Now()
		if err := conn.SendMsg(msg); err != nil {
			return err
		}
		if _, err := conn.RecvMsgBuf(buf); err != nil {
			return err
		}
		lats = append(lats, time.Since(start))
	}
	report(lats)
	return nil
}

func report(lats []time.Duration) {
	if len(lats) == 0 {
		return
	}
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	var sum time.Duration
	for _, l := range lats {
		sum += l
	}
	avg := sum / time.Duration(len(lats))
	p50 := lats[len(lats)*50/100]
	p99 := lats[len(lats)*99/100]
	fmt.Printf("round-trips=%d avg=%v p50=%v p99=%v min=%v max=%v\n",
		len(lats), avg, p50, p99, lats[0], lats[len(lats)-1])
}
