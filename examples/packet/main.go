// Command packet demonstrates the UD PacketConn datagram API. The server prints
// its Addr; the client is given that Addr string to send a datagram to.
//
//	server: go run .
//	client: go run . '<server-addr-string>'
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
	device := flag.String("d", "mlx5_1", "RDMA device (mlx5_0=CPU/xgbe0, mlx5_1..8=GPU/xgbe1..8)")
	gidIndex := flag.Int("x", 3, "GID index (RoCE v2)")
	flag.Parse()
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	opts := []rdmanet.Option{rdmanet.WithDevice(*device), rdmanet.WithGIDIndex(*gidIndex)}
	var err error
	if flag.NArg() > 0 {
		err = client(flag.Arg(0), opts)
	} else {
		err = server(opts)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func server(opts []rdmanet.Option) error {
	pc, err := rdmanet.ListenPacket("", opts...)
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

func client(serverAddr string, opts []rdmanet.Option) error {
	to, err := rdmanet.ResolveAddr(serverAddr)
	if err != nil {
		return err
	}
	pc, err := rdmanet.ListenPacket("", opts...)
	if err != nil {
		return err
	}
	defer pc.Close()
	_, err = pc.WriteTo([]byte("hello datagram"), to)
	return err
}
