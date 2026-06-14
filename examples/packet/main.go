// Command packet demonstrates the UD PacketConn datagram API. The server prints
// its Addr; the client is given that Addr string to send a datagram to.
//
//	server: go run .
//	client: go run . '<server-addr-string>'
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
)

func main() {
	flag.Parse()
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	var err error
	if flag.NArg() > 0 {
		err = client(flag.Arg(0))
	} else {
		err = server()
	}
	if err != nil {
		log.Fatal(err)
	}
}

func server() error {
	pc, err := rdmanet.ListenPacket("")
	if err != nil {
		return err
	}
	defer pc.Close()
	fmt.Printf("server addr: %s\n", pc.LocalAddr())
	buf := make([]byte, 2048)
	n, from, err := pc.ReadFrom(buf)
	if err != nil {
		return err
	}
	fmt.Printf("got %q from QPN %s\n", buf[:n], from)
	return nil
}

func client(serverAddr string) error {
	to, err := rdmanet.ResolveAddr(serverAddr)
	if err != nil {
		return err
	}
	pc, err := rdmanet.ListenPacket("")
	if err != nil {
		return err
	}
	defer pc.Close()
	_, err = pc.WriteTo([]byte("hello datagram"), to)
	return err
}
