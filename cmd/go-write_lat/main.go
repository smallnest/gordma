// Command go-write_lat is a Go reimplementation of perftest's ib_write_lat: it
// measures RDMA Write round-trip latency using the "polling on last byte"
// method — each side writes into the peer's buffer and polls its local last
// byte for the reply, matching perftest's approach.
//
// RDMA Write requires the TCP handshake path. Run without an address for server
// mode; pass the server address for client mode. Use --output=histogram for the
// full latency histogram. Requires RDMA hardware.
package main

import (
	"fmt"
	"os"

	"github.com/smallnest/gordma/perftest"
)

func main() {
	cfg, err := perftest.ParseArgs("go-write_lat", os.Args[1:], os.Stderr)
	if err != nil {
		os.Exit(2)
	}
	if cfg.ConnMethod == perftest.ConnRDMACM {
		fmt.Fprintln(os.Stderr, "go-write_lat: -R (rdma_cm) is not supported; RDMA Write needs the TCP handshake to exchange RKey/addr")
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "go-write_lat: %v\n", err)
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

	res, err := perftest.RunWriteLat(cfg, ep, mr)
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
