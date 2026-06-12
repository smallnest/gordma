// Package handshake provides a TCP out-of-band channel for exchanging the QP
// connection information that two RDMA endpoints need before they can talk —
// the same approach perftest uses by default. It is pure Go (no cgo) so it can
// be unit-tested anywhere, independent of RDMA hardware.
package handshake

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// EndpointInfo is the set of values exchanged out-of-band so each side can
// transition its QP to RTR/RTS and target the peer's memory for RDMA.
//
// Addressing carries both LID (InfiniBand) and GID (RoCE v2); the consumer
// picks based on the link layer. RKey/RemoteAddr are only meaningful for
// RDMA Write/Read targets.
type EndpointInfo struct {
	// QPN is the queue-pair number.
	QPN uint32 `json:"qpn"`
	// PSN is the starting packet sequence number.
	PSN uint32 `json:"psn"`
	// LID is the InfiniBand local identifier (0 on RoCE).
	LID uint16 `json:"lid"`
	// GID is the 16-byte RoCE/IB global identifier, hex-encoded.
	GID [16]byte `json:"gid"`
	// GIDIndex is the GID table index the peer used.
	GIDIndex int `json:"gid_index"`
	// RKey is the remote key for RDMA Write/Read into RemoteAddr.
	RKey uint32 `json:"rkey"`
	// RemoteAddr is the virtual address of the peer's registered buffer.
	RemoteAddr uint64 `json:"remote_addr"`
}

// wire frames an EndpointInfo with a newline so both ends can use a simple
// line-delimited exchange over the TCP connection.
func writeInfo(conn net.Conn, info EndpointInfo, timeout time.Duration) error {
	if timeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(timeout))
		defer conn.SetWriteDeadline(time.Time{})
	}
	b, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("handshake: marshal local info: %w", err)
	}
	b = append(b, '\n')
	if _, err := conn.Write(b); err != nil {
		return fmt.Errorf("handshake: write local info: %w", err)
	}
	return nil
}

func readInfo(r *bufio.Reader, conn net.Conn, timeout time.Duration) (EndpointInfo, error) {
	if timeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		defer conn.SetReadDeadline(time.Time{})
	}
	line, err := r.ReadBytes('\n')
	if err != nil {
		return EndpointInfo{}, fmt.Errorf("handshake: read peer info: %w", err)
	}
	var info EndpointInfo
	if err := json.Unmarshal(line, &info); err != nil {
		return EndpointInfo{}, fmt.Errorf("handshake: unmarshal peer info: %w", err)
	}
	return info, nil
}

// exchange writes the local info and reads the peer's. Write-then-read works on
// both ends without deadlock because the payloads are tiny (well under any
// socket buffer), so neither side blocks on write before the other reads.
func exchange(conn net.Conn, local EndpointInfo, timeout time.Duration) (EndpointInfo, error) {
	if err := writeInfo(conn, local, timeout); err != nil {
		return EndpointInfo{}, err
	}
	r := bufio.NewReader(conn)
	return readInfo(r, conn, timeout)
}
