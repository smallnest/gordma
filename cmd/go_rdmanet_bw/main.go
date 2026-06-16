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
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/smallnest/gordma"
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
		device    = fs.StringP("device", "d", "", "RDMA device name (default: first available)")
		port      = fs.IntP("ib-port", "i", 1, "HCA port number")
		gidIndex  = fs.IntP("gid-index", "x", 0, "GID index (RoCE v2)")
		size      = fs.IntP("size", "s", 65536, "message size in bytes")
		iters     = fs.IntP("count", "n", 1000, "number of messages")
		batch     = fs.IntP("batch", "b", 1, "messages per SendBatch/RecvBatch (1 = one-at-a-time SendMsg)")
		depth     = fs.Int("depth", 0, "queue depth / credit window (0 = library default 128); set on both ends")
		raw       = fs.Bool("raw", false, "use RawConn (low-level, no framing/flow-control) for near-perftest speed")
		rawOne    = fs.Bool("raw-onebuf", false, "raw: use a single fixed region every iter (like perftest) instead of cycling slots; applies to client send buffer AND server recv buffer")
		rawSingle = fs.Bool("raw-single", false, "raw client: post one signaled SEND at a time (RawConn.Pipeline) instead of batched submit (PipelineBatch), matching go_send_bw")
		tcpPort   = fs.IntP("tcp-port", "p", 18515, "TCP listen/connect port for establishment")
		handshk   = fs.Bool("handshake", false, "use TCP out-of-band handshake instead of rdma_cm")
		pollMode  = fs.String("poll", "event", "CQ poll mode: busy|event")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *batch < 1 {
		return fmt.Errorf("invalid --batch %d (want >= 1)", *batch)
	}
	if *depth < 0 {
		return fmt.Errorf("invalid --depth %d (want >= 0)", *depth)
	}
	if *raw {
		// RawConn always uses the TCP handshake and busy-polls; --batch is the
		// tx-depth (work requests in flight). Default tx-depth to --depth or 128.
		txDepth := *batch
		if txDepth <= 1 {
			txDepth = *depth
		}
		if txDepth <= 0 {
			txDepth = 128
		}
		opts := rawOptions(*device, *port, *gidIndex, *depth)
		if fs.NArg() > 0 {
			return runClientRaw(fs.Arg(0), *size, *iters, txDepth, *rawOne, *rawSingle, opts)
		}
		return runServerRaw(fmt.Sprintf("0.0.0.0:%d", *tcpPort), *size, txDepth, *rawOne, opts)
	}
	opts, err := buildOptions(*device, *port, *gidIndex, *size, *depth, *handshk, *pollMode)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("0.0.0.0:%d", *tcpPort)
	if fs.NArg() > 0 {
		// client
		return runClientBW(fs.Arg(0), *size, *iters, *batch, opts)
	}
	return runServerBW(addr, *size, *batch, opts)
}

// rawOptions builds the option set RawConn honors (device/port/gid/depth).
func rawOptions(device string, port, gidIndex, depth int) []rdmanet.Option {
	opts := []rdmanet.Option{
		rdmanet.WithPort(port),
		rdmanet.WithGIDIndex(gidIndex),
	}
	if device != "" {
		opts = append(opts, rdmanet.WithDevice(device))
	}
	if depth > 0 {
		opts = append(opts, rdmanet.WithQueueDepth(depth))
	}
	return opts
}

// buildOptions translates the common flags into rdmanet.Options shared by both
// the bw and lat tools.
func buildOptions(device string, port, gidIndex, size, depth int, handshake bool, pollMode string) ([]rdmanet.Option, error) {
	opts := []rdmanet.Option{
		rdmanet.WithPort(port),
		rdmanet.WithGIDIndex(gidIndex),
		rdmanet.WithBufferSize(size + 64), // room for the frame header
	}
	if device != "" {
		opts = append(opts, rdmanet.WithDevice(device))
	}
	if depth > 0 {
		opts = append(opts, rdmanet.WithQueueDepth(depth))
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

func runServerBW(addr string, size, batch int, opts []rdmanet.Option) error {
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

	// Receive until the client closes (io.EOF): the server does not need to know
	// the client's message count.
	got := 0
	if batch > 1 {
		for {
			msgs, err := conn.RecvBatch(batch)
			got += len(msgs)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return err
			}
		}
	} else {
		buf := make([]byte, size)
		for {
			if _, err := conn.RecvMsgBuf(buf); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return err
			}
			got++
		}
	}
	fmt.Println("server received", got, "messages")
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
	if cw, sw := conn.ProbeStats(); cw > 0 || sw > 0 {
		fmt.Printf("probe: credit-wait=%v (%.1f%%), send-completion-wait=%v (%.1f%%) of %v\n",
			cw, 100*float64(cw)/float64(elapsed), sw, 100*float64(sw)/float64(elapsed), elapsed)
	}
	return nil
}

// printPerftestHeader prints the ib_send_bw-style header block describing the
// connection. The BW/result line is printed separately by printPerftestResult
// after the run.
func printPerftestHeader(info rdmanet.RawConnInfo, txDepth int) {
	const rule = "---------------------------------------------------------------------------------------"
	link := info.LinkLayer
	if link == "" {
		link = "Ethernet"
	}
	fmt.Println(rule)
	fmt.Println("                    Send BW Test")
	fmt.Printf(" Dual-port       : OFF          Device         : %s\n", info.Device)
	fmt.Printf(" Number of qps   : 1            Transport type : IB\n")
	fmt.Printf(" Connection type : RC           Using SRQ      : OFF\n")
	fmt.Printf(" TX depth        : %d\n", txDepth)
	fmt.Printf(" CQ Moderation   : 1\n")
	fmt.Printf(" Mtu             : %d[B]\n", info.MTU)
	fmt.Printf(" Link type       : %s\n", link)
	fmt.Printf(" GID index       : %d\n", info.GIDIndex)
	fmt.Printf(" Max inline data : 0[B]\n")
	fmt.Printf(" rdma_cm QPs     : OFF\n")
	fmt.Printf(" Data ex. method : Ethernet\n")
	fmt.Println(rule)
	fmt.Printf(" local address: LID %04d QPN 0x%05x PSN 0x%06x\n", info.Local.LID, info.Local.QPN, info.Local.PSN)
	fmt.Printf(" GID: %s\n", gidDecimal(info.Local.GID))
	fmt.Printf(" remote address: LID %04d QPN 0x%05x PSN 0x%06x\n", info.Remote.LID, info.Remote.QPN, info.Remote.PSN)
	fmt.Printf(" GID: %s\n", gidDecimal(info.Remote.GID))
	fmt.Println(rule)
}

// printPerftestResult prints the ib_send_bw-style result line. BW average is in
// MiB/sec (2^20 bytes) and MsgRate in Mpps, matching ib_send_bw's units so the
// numbers are directly comparable.
func printPerftestResult(size, iters int, elapsed time.Duration) {
	const rule = "---------------------------------------------------------------------------------------"
	secs := elapsed.Seconds()
	totalBytes := float64(size) * float64(iters)
	bwMiB := totalBytes / (1024 * 1024) / secs
	mpps := float64(iters) / secs / 1e6
	fmt.Printf(" #bytes     #iterations    BW peak[MiB/sec]    BW average[MiB/sec]   MsgRate[Mpps]\n")
	fmt.Printf(" %-10d %-14d %-19.2f %-21.2f %f\n", size, iters, 0.0, bwMiB, mpps)
	fmt.Println(rule)
}

// gidDecimal renders a GID the way ib_send_bw prints it: 16 decimal bytes
// joined by colons (e.g. 00:00:...:255:255:33:00:226:27).
func gidDecimal(gid [16]byte) string {
	var b []byte
	for i, x := range gid {
		if i > 0 {
			b = append(b, ':')
		}
		b = append(b, []byte(fmt.Sprintf("%02d", x))...)
	}
	return string(b)
}

// runServerRaw is the RawConn passive side: accept, register one MR, and drain
// recvs until the client closes (an RNR/transport error ends the run). When
// oneBuf is set the recv side reuses a single fixed 64KB region every iteration
// (like perftest's go_send_bw server) instead of cycling through txDepth slots;
// this keeps the receive-side working set hot to match the perftest baseline.
func runServerRaw(addr string, size, txDepth int, oneBuf bool, opts []rdmanet.Option) error {
	ln, err := rdmanet.ListenRaw(addr, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()
	fmt.Printf("go_rdmanet_bw (raw) server listening on %s\n", ln.Addr())
	rc, err := ln.Accept()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	printPerftestHeader(rc.Info(), txDepth)

	regSize := size * txDepth
	if oneBuf {
		regSize = size
	}
	mr, err := rc.RegisterMemory(regSize)
	if err != nil {
		return err
	}
	defer func() { _ = mr.Close() }()

	rebuild := func(wrID uint64) gordma.RecvWR {
		off := 0
		if !oneBuf {
			off = (int(wrID) % txDepth) * size
		}
		return gordma.RecvWR{
			WRID:   wrID,
			SGList: []gordma.SGE{gordma.SGEFromMR(mr, off, size)},
		}
	}
	// The client controls the count and closes when done; treat transport
	// teardown as the end of the run.
	if err := rc.RecvDrain(1<<62, txDepth, rebuild); err != nil {
		fmt.Println("raw server stopped:", err)
	}
	if post, poll := rc.ProbeStats(); post > 0 || poll > 0 {
		fmt.Printf("probe: recv-post=%v, poll=%v\n", post, poll)
	}
	return nil
}

// runClientRaw is the RawConn active side: register one MR, then pipeline
// txDepth signaled SENDs in flight until iters complete — the same loop the
// perftest go_send_bw tool uses. When oneBuf is set it sends the same fixed
// region every iteration (matching perftest's hot 64KB working set) instead of
// cycling through txDepth slots; this is a diagnostic to test whether the
// rotating working set is what separates RawConn from perftest. When single is
// set it posts one signaled SEND at a time via RawConn.Pipeline (per-WR
// PostSend), exactly like go_send_bw, instead of the batched PipelineBatch
// submit; combine --raw-onebuf --raw-single (with --raw-onebuf on the server
// too) to align RawConn fully with the perftest baseline.
func runClientRaw(server string, size, iters, txDepth int, oneBuf, single bool, opts []rdmanet.Option) error {
	rc, err := rdmanet.DialRaw(server, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	printPerftestHeader(rc.Info(), txDepth)

	regSize := size * txDepth
	if oneBuf {
		regSize = size
	}
	mr, err := rc.RegisterMemory(regSize)
	if err != nil {
		return err
	}
	defer func() { _ = mr.Close() }()

	build := func(wrID uint64) gordma.SendWR {
		off := 0
		if !oneBuf {
			off = (int(wrID) % txDepth) * size
		}
		return gordma.SendWR{
			WRID:     wrID,
			Opcode:   gordma.OpSend,
			SGList:   []gordma.SGE{gordma.SGEFromMR(mr, off, size)},
			Signaled: true,
		}
	}

	start := time.Now()
	if single {
		err = rc.Pipeline(iters, txDepth, func(wrID uint64) error {
			return rc.PostSend(build(wrID))
		})
	} else {
		err = rc.PipelineBatch(iters, txDepth, build)
	}
	if err != nil {
		return err
	}
	elapsed := time.Since(start)
	mode := "raw-batch"
	if single {
		mode = "raw-single"
	}
	if oneBuf {
		mode += "-onebuf"
	}
	printPerftestResult(size, iters, elapsed)
	if post, poll := rc.ProbeStats(); post > 0 || poll > 0 {
		fmt.Printf(" probe (%s, txDepth=%d): post=%v (%.1f%%), poll=%v (%.1f%%) of %v\n",
			mode, txDepth, post, 100*float64(post)/float64(elapsed), poll, 100*float64(poll)/float64(elapsed), elapsed)
	}
	return nil
}
