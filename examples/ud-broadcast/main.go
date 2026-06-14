// Command ud-broadcast sends one datagram to several UD peers discovered via the
// registry: the sender looks up each name and WriteTo's it in turn, reusing
// cached AddressHandles. Receivers register under a name and print what they
// get.
//
//	registry: go run . -registry :9100
//	receiver: go run . -r 192.0.2.1:9100 -name node1   (run several, different names)
//	sender:   go run . -r 192.0.2.1:9100 -to node1,node2,node3 -send
//
// Defaults target the first GPU NIC: device mlx5_1 (GPU net xgbe1) and
// RoCE v2 GID index 3. Override with -d / -x (use -d mlx5_0 for CPU xgbe0).
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
)

func main() {
	reg := flag.String("registry", "", "run a registry server on this address")
	regAddr := flag.String("r", "", "registry address to use")
	name := flag.String("name", "", "receiver: register under this name")
	to := flag.String("to", "", "sender: comma-separated peer names")
	send := flag.Bool("send", false, "sender mode")
	device := flag.String("d", "mlx5_1", "RDMA device (mlx5_0=CPU/xgbe0, mlx5_1..8=GPU/xgbe1..8)")
	gidIndex := flag.Int("x", 3, "GID index (RoCE v2)")
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
		err = sender(*regAddr, *to, opts)
	} else {
		err = receiver(*regAddr, *name, opts)
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
	select {}
}

func receiver(regAddr, name string, opts []rdmanet.Option) error {
	if name == "" {
		return fmt.Errorf("receiver requires -name")
	}
	pc, err := rdmanet.ListenPacket("", opts...)
	if err != nil {
		return err
	}
	defer pc.Close()
	if err := pc.Register(regAddr, name); err != nil {
		return err
	}
	fmt.Printf("%s registered as %s\n", name, pc.LocalAddr())
	buf := make([]byte, 2048)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		return err
	}
	fmt.Printf("%s got %q\n", name, buf[:n])
	return nil
}

func sender(regAddr, names string, opts []rdmanet.Option) error {
	pc, err := rdmanet.ListenPacket("", opts...)
	if err != nil {
		return err
	}
	defer pc.Close()
	for _, name := range strings.Split(names, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		to, err := rdmanet.LookupAddr(regAddr, name)
		if err != nil {
			return fmt.Errorf("lookup %s: %w", name, err)
		}
		// AddressHandles are cached per destination Addr inside the PacketConn.
		if _, err := pc.WriteTo([]byte("broadcast hello"), to); err != nil {
			return fmt.Errorf("send to %s: %w", name, err)
		}
		fmt.Printf("sent to %s (%s)\n", name, to)
	}
	return nil
}
