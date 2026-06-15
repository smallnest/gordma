// Command go_send_lat is a Go reimplementation of perftest's ib_send_lat: it
// measures Send/Recv round-trip latency via ping-pong between two endpoints.
// Run without an address to act as the server; pass the server address to act
// as the client. Use --output=histogram for the full latency histogram.
//
// Requires RDMA hardware (Linux + libibverbs/librdmacm).
package main

import (
	"fmt"
	"os"

	"github.com/smallnest/gordma/perftest"
)

func main() {
	cfg, err := perftest.ParseArgs("go_send_lat", os.Args[1:], os.Stderr)
	if err != nil {
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "go_send_lat: %v\n", err)
		os.Exit(1)
	}
}

func run(cfg perftest.Config) error {
	ep, mr, err := perftest.SetupTCPOrCM(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = ep.Close() }()
	defer func() { _ = mr.Close() }()

	res, err := perftest.RunSendLat(cfg, ep, mr)
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
