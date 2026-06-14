// Command chat is a full-duplex example: each side reads stdin and sends in one
// goroutine while receiving and printing in another, demonstrating that one
// concurrent reader + one concurrent writer are safe, and that Close wakes a
// blocked RecvMsg.
//
//	server: go run . -l :18515
//	client: go run . 192.0.2.1:18515
//
// Type lines and press enter; Ctrl-D (EOF on stdin) closes the connection,
// which wakes the peer's receive loop with io.EOF.
//
// Defaults target the first GPU NIC: device mlx5_1 (GPU net xgbe1) and
// RoCE v2 GID index 3. Override with -d / -x (use -d mlx5_0 for CPU xgbe0).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/rdmanet"
)

func main() {
	listen := flag.String("l", "", "listen address (server mode)")
	device := flag.String("d", "mlx5_1", "RDMA device (mlx5_0=CPU/xgbe0, mlx5_1..8=GPU/xgbe1..8)")
	gidIndex := flag.Int("x", 3, "GID index (RoCE v2)")
	flag.Parse()
	if !gordma.Supported() {
		fmt.Println("RDMA not supported on this platform:", gordma.ErrNotSupported)
		return
	}
	opts := []rdmanet.Option{rdmanet.WithDevice(*device), rdmanet.WithGIDIndex(*gidIndex)}
	var conn *rdmanet.Conn
	var err error
	if *listen != "" {
		var ln *rdmanet.Listener
		if ln, err = rdmanet.Listen(*listen, opts...); err == nil {
			defer ln.Close()
			conn, err = ln.Accept()
		}
	} else if flag.NArg() > 0 {
		conn, err = rdmanet.Dial(flag.Arg(0), opts...)
	} else {
		log.Fatal("usage: chat -l :PORT | chat HOST:PORT")
	}
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	if err := chat(conn); err != nil {
		log.Fatal(err)
	}
}

// chat runs a send goroutine (stdin -> peer) and a receive loop (peer ->
// stdout) over the same Conn concurrently.
func chat(conn *rdmanet.Conn) error {
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if err := conn.SendMsg(sc.Bytes()); err != nil {
				return
			}
		}
		conn.Close() // stdin EOF: closing wakes the peer's RecvMsg
	}()
	for {
		msg, err := conn.RecvMsg()
		if err == io.EOF {
			fmt.Println("peer closed")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Printf("peer> %s\n", msg)
	}
}
