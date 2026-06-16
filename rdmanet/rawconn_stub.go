//go:build !linux || !cgo

package rdmanet

import (
	"time"

	"github.com/smallnest/gordma"
)

// This file provides the stub build of RawConn for non-Linux platforms or
// builds without cgo. Every entry point returns gordma.ErrNotSupported (or a
// nil/zero accessor result) and nothing panics, matching conn_stub.go.

// RawConn is the stub low-level RC endpoint. It holds no resources.
type RawConn struct{}

// RawListener is the stub low-level listener. It holds no resources.
type RawListener struct{}

// DialRaw returns ErrNotSupported on the stub build.
func DialRaw(addr string, opts ...Option) (*RawConn, error) { return nil, gordma.ErrNotSupported }

// DialRawTimeout returns ErrNotSupported on the stub build.
func DialRawTimeout(addr string, timeout time.Duration, opts ...Option) (*RawConn, error) {
	return nil, gordma.ErrNotSupported
}

// ListenRaw returns ErrNotSupported on the stub build.
func ListenRaw(addr string, opts ...Option) (*RawListener, error) {
	return nil, gordma.ErrNotSupported
}

// Accept returns ErrNotSupported on the stub build.
func (l *RawListener) Accept() (*RawConn, error) { return nil, gordma.ErrNotSupported }

// Addr returns the empty string on the stub build.
func (l *RawListener) Addr() string { return "" }

// Close is a no-op on the stub build.
func (l *RawListener) Close() error { return nil }

// RegisterMemory returns ErrNotSupported on the stub build.
func (rc *RawConn) RegisterMemory(size int) (*gordma.MR, error) { return nil, gordma.ErrNotSupported }

// QP returns nil on the stub build.
func (rc *RawConn) QP() *gordma.QP { return nil }

// CQ returns nil on the stub build.
func (rc *RawConn) CQ() *gordma.CQ { return nil }

// PD returns nil on the stub build.
func (rc *RawConn) PD() *gordma.PD { return nil }

// PeerRKey returns 0 on the stub build.
func (rc *RawConn) PeerRKey() uint32 { return 0 }

// PeerAddr returns 0 on the stub build.
func (rc *RawConn) PeerAddr() uint64 { return 0 }

// PostSend returns ErrNotSupported on the stub build.
func (rc *RawConn) PostSend(wr gordma.SendWR) error { return gordma.ErrNotSupported }

// PostRecv returns ErrNotSupported on the stub build.
func (rc *RawConn) PostRecv(wr gordma.RecvWR) error { return gordma.ErrNotSupported }

// Poll returns ErrNotSupported on the stub build.
func (rc *RawConn) Poll(wc []gordma.WorkCompletion) (int, error) { return 0, gordma.ErrNotSupported }

// Pipeline returns ErrNotSupported on the stub build.
func (rc *RawConn) Pipeline(iters, txDepth int, post func(wrID uint64) error) error {
	return gordma.ErrNotSupported
}

// PostSendBatch returns ErrNotSupported on the stub build.
func (rc *RawConn) PostSendBatch(wrs []gordma.SendWR) error { return gordma.ErrNotSupported }

// PipelineBatch returns ErrNotSupported on the stub build.
func (rc *RawConn) PipelineBatch(iters, txDepth int, build func(wrID uint64) gordma.SendWR) error {
	return gordma.ErrNotSupported
}

// RecvDrain returns ErrNotSupported on the stub build.
func (rc *RawConn) RecvDrain(iters, txDepth int, rebuild func(wrID uint64) gordma.RecvWR) error {
	return gordma.ErrNotSupported
}

// ProbeStats returns zero on the stub build.
func (rc *RawConn) ProbeStats() (post, poll time.Duration) { return 0, 0 }

// Close is a no-op on the stub build.
func (rc *RawConn) Close() error { return nil }
