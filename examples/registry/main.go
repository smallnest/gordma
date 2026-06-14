// Command registry demonstrates the out-of-band UD address registry: a server
// registers its Addr under a name, and a client looks it up to send a datagram
// — no manual Addr copy-paste.
//
//	registry: go run . -registry :9100
//	server:   go run . -r 127.0.0.1:9100 -name nodeA
//	client:   go run . -r 127.0.0.1:9100 -name nodeA -send
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
)

func main() {
	reg := flag.String("registry", "", "run a registry server on this address")
	regAddr := flag.String("r", "", "registry address to use")
	name := flag.String("name", "node", "name to register/lookup")
	send := flag.Bool("send", false, "client mode: look up name and send to it")
	flag.Parse()

	if *reg != "" {
		runRegistry(*reg)
		return
	}
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	var err error
	if *send {
		err = client(*regAddr, *name)
	} else {
		err = server(*regAddr, *name)
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

func server(regAddr, name string) error {
	pc, err := rdmanet.ListenPacket("")
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

func client(regAddr, name string) error {
	to, err := rdmanet.LookupAddr(regAddr, name)
	if err != nil {
		return err
	}
	pc, err := rdmanet.ListenPacket("")
	if err != nil {
		return err
	}
	defer pc.Close()
	_, err = pc.WriteTo([]byte("hello via registry"), to)
	return err
}
