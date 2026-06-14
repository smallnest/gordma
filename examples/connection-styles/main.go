// Command connection-styles demonstrates the connection establishment methods
// rdmanet offers side by side, selected with -mode:
//
//	rc-cm        RC via the RDMA connection manager (rdma_cm) — the default
//	rc-handshake RC via the TCP out-of-band handshake (WithHandshake)
//	ud           UD datagrams (connectionless; address peers by Addr string)
//
// Run a server and a client with the same -mode.
//
//	rc-cm / rc-handshake:
//	  server: go run . -mode rc-cm -l :18515
//	  client: go run . -mode rc-cm 33.0.226.25:18515
//	ud:
//	  server: go run . -mode ud
//	  client: go run . -mode ud '<server-addr-string>'
//
// Defaults target the first GPU NIC: device mlx5_1 (GPU net xgbe1) and
// RoCE v2 GID index 3. Override with -d / -x (use -d mlx5_0 for CPU xgbe0).
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
)

func main() {
	mode := flag.String("mode", "rc-cm", "connection style: rc-cm | rc-handshake | ud")
	listen := flag.String("l", "", "listen address (RC server mode)")
	device := flag.String("d", "mlx5_1", "RDMA device (mlx5_0=CPU/xgbe0, mlx5_1..8=GPU/xgbe1..8)")
	gidIndex := flag.Int("x", 3, "GID index (RoCE v2)")
	flag.Parse()
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	opts := []rdmanet.Option{rdmanet.WithDevice(*device), rdmanet.WithGIDIndex(*gidIndex)}

	var err error
	switch *mode {
	case "rc-cm":
		err = runRC(*listen, flagArg(), opts)
	case "rc-handshake":
		err = runRC(*listen, flagArg(), append(opts, rdmanet.WithHandshake()))
	case "ud":
		err = runUD(flagArg(), opts)
	default:
		log.Fatalf("unknown -mode %q (want rc-cm|rc-handshake|ud)", *mode)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func flagArg() string {
	if flag.NArg() > 0 {
		return flag.Arg(0)
	}
	return ""
}

// runRC drives both RC styles (rc-cm and rc-handshake differ only by options).
func runRC(listen, peer string, opts []rdmanet.Option) error {
	if listen != "" {
		ln, err := rdmanet.Listen(listen, opts...)
		if err != nil {
			return err
		}
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		defer conn.Close()
		msg, err := conn.RecvMsg()
		if err != nil {
			return err
		}
		return conn.SendMsg(msg)
	}
	if peer == "" {
		return fmt.Errorf("RC client needs a HOST:PORT (or use -l for server)")
	}
	conn, err := rdmanet.Dial(peer, opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SendMsg([]byte("hello RC")); err != nil {
		return err
	}
	reply, err := conn.RecvMsg()
	if err != nil {
		return err
	}
	fmt.Printf("echo: %s\n", reply)
	return nil
}

// runUD drives the connectionless UD style: the server prints its Addr, the
// client sends a datagram to the given Addr string.
func runUD(peerAddr string, opts []rdmanet.Option) error {
	pc, err := rdmanet.ListenPacket("", opts...)
	if err != nil {
		return err
	}
	defer pc.Close()
	if peerAddr == "" {
		fmt.Printf("UD server addr: %s\n", pc.LocalAddr())
		buf := make([]byte, 2048)
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			return err
		}
		fmt.Printf("got %q from QPN %s\n", buf[:n], from)
		return nil
	}
	to, err := rdmanet.ResolveAddr(peerAddr)
	if err != nil {
		return err
	}
	_, err = pc.WriteTo([]byte("hello UD"), to)
	return err
}
