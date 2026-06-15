// Command registry demonstrates the out-of-band UD address registry: a server
// registers its Addr under a name, and a client looks it up to send a datagram
// — no manual Addr copy-paste.
//
//	registry: go run . --registry :9100
//	server:   go run . -r 10.214.180.34:9100 --name nodeA
//	client:   go run . -r 10.214.180.34:9100 --name nodeA --send
//
// Defaults target the first GPU NIC: device mlx5_1 (GPU net xgbe1) and
// RoCE v2 GID index 3. Override with -d / -x (use -d mlx5_0 for CPU xgbe0).
package main

import (
	"fmt"
	"log"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
	flag "github.com/spf13/pflag"
)

var (
	reg      = flag.String("registry", "", "run a registry server on this address")
	regAddr  = flag.StringP("registry-addr", "r", "", "registry address to use")
	name     = flag.String("name", "node", "name to register/lookup")
	send     = flag.Bool("send", false, "client mode: look up name and send to it")
	device   = flag.StringP("device", "d", "mlx5_1", "RDMA device (mlx5_0=CPU/xgbe0, mlx5_1..8=GPU/xgbe1..8)")
	gidIndex = flag.IntP("gid-index", "x", 3, "GID index (RoCE v2)")
)

func main() {
	flag.Parse()

	if *reg != "" {
		runRegistry(*reg)
		return
	}
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	opts := []rdmanet.Option{rdmanet.WithDevice(*device), rdmanet.WithGIDIndex(*gidIndex)}
	var err error
	if *send {
		err = client(*regAddr, *name, opts)
	} else {
		err = server(*regAddr, *name, opts)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func runRegistry(addr string) {
	r, err := rdmanet.NewRegistry(addr)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()
	fmt.Printf("registry listening on %s\n", r.Addr())
	select {} // serve until killed
}

func server(regAddr, name string, opts []rdmanet.Option) error {
	pc, err := rdmanet.ListenPacket("", opts...)
	if err != nil {
		return err
	}
	defer pc.Close()
	if err := pc.Register(regAddr, name); err != nil {
		return err
	}
	fmt.Printf("registered %q as %s\n", name, pc.LocalAddr())
	buf := make([]byte, 2048)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		return err
	}
	fmt.Printf("got %q\n", buf[:n])
	return nil
}

func client(regAddr, name string, opts []rdmanet.Option) error {
	to, err := rdmanet.LookupAddr(regAddr, name)
	if err != nil {
		return err
	}
	pc, err := rdmanet.ListenPacket("", opts...)
	if err != nil {
		return err
	}
	defer pc.Close()
	_, err = pc.WriteTo([]byte("hello via registry"), to)
	return err
}
