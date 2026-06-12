// Command go-read_lat is a Go reimplementation of perftest's ib_read_lat: it
// measures RDMA Read latency. Each RDMA Read is itself a round trip, so the
// latency is the time from posting the read to its completion.
//
// RDMA Read is RC-only and requires the TCP handshake path. Run without an
// address for server mode; pass the server address for client mode. Use
// --output=histogram for the full histogram. Requires RDMA hardware.
package main

import (
	"fmt"
	"os"

	"github.com/smallnest/gordma/perftest"
)

func main() {
	cfg, err := perftest.ParseArgs("go-read_lat", os.Args[1:], os.Stderr)
	if err != nil {
		os.Exit(2)
	}
	if err := cfg.RequireOneSidedTCP(); err != nil {
		fmt.Fprintf(os.Stderr, "go-read_lat: %v\n", err)
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "go-read_lat: %v\n", err)
		os.Exit(1)
	}
}

func run(cfg perftest.Config) error {
	ep, mr, err := perftest.SetupTCP(cfg)
	if err != nil {
		return err
	}
	defer ep.Close()
	defer mr.Close()

	res, err := perftest.RunReadLat(cfg, ep, mr)
	if err != nil {
		return err
	}
	if cfg.IsServer() {
		fmt.Println("server done")
		return nil
	}
	perftest.PrintLat(os.Stdout, cfg, res)
	return nil
}
