// Command go-read_bw is a Go reimplementation of perftest's ib_read_bw: it
// measures one-sided RDMA Read bandwidth. The client reads from the server's
// registered buffer using the RKey/remote address exchanged out-of-band.
//
// RDMA Read is RC-only and requires the TCP handshake path (for RKey/addr).
// Run without an address for server mode; pass the server address for client
// mode. Requires RDMA hardware.
package main

import (
	"fmt"
	"os"

	"github.com/smallnest/gordma/perftest"
)

func main() {
	cfg, err := perftest.ParseArgs("go-read_bw", os.Args[1:], os.Stderr)
	if err != nil {
		os.Exit(2)
	}
	if err := cfg.RequireOneSidedTCP(); err != nil {
		fmt.Fprintf(os.Stderr, "go-read_bw: %v\n", err)
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "go-read_bw: %v\n", err)
		os.Exit(1)
	}
}

func run(cfg perftest.Config) error {
	ep, mr, err := perftest.SetupTCP(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = ep.Close() }()
	defer func() { _ = mr.Close() }()

	res, err := perftest.RunReadBW(cfg, ep, mr)
	if err != nil {
		return err
	}
	if cfg.IsServer() {
		fmt.Println("server done")
		return nil
	}
	perftest.PrintBW(os.Stdout, res)
	return nil
}
