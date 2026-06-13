//go:build !linux || !cgo

package rdmanet

import (
	"time"

	"github.com/smallnest/gordma"
)

// This file provides the stub build of rdmanet for non-Linux platforms or
// builds without cgo. Every entry point returns gordma.ErrNotSupported and no
// call panics, so importing rdmanet never breaks a cross-platform build.

// Conn is the stub RC endpoint. It holds no resources.
type Conn struct{}

// Listener is the stub RC listener. It holds no resources.
type Listener struct{}

// PacketConn is the stub UD endpoint. It holds no resources.
type PacketConn struct{}

// Dial returns ErrNotSupported on the stub build.
func Dial(addr string, opts ...Option) (*Conn, error) {
	_ = applyOptions(opts)
	return nil, gordma.ErrNotSupported
}

// DialTimeout returns ErrNotSupported on the stub build.
func DialTimeout(addr string, timeout time.Duration, opts ...Option) (*Conn, error) {
	_ = applyOptions(opts)
	return nil, gordma.ErrNotSupported
}

// Listen returns ErrNotSupported on the stub build.
func Listen(addr string, opts ...Option) (*Listener, error) {
	_ = applyOptions(opts)
	return nil, gordma.ErrNotSupported
}

// Accept returns ErrNotSupported on the stub build.
func (l *Listener) Accept() (*Conn, error) { return nil, gordma.ErrNotSupported }

// Close is a no-op on the stub build.
func (l *Listener) Close() error { return nil }

// Addr returns nil on the stub build.
func (l *Listener) Addr() string { return "" }

// SendMsg returns ErrNotSupported on the stub build.
func (c *Conn) SendMsg(p []byte) error { return gordma.ErrNotSupported }

// RecvMsg returns ErrNotSupported on the stub build.
func (c *Conn) RecvMsg() ([]byte, error) { return nil, gordma.ErrNotSupported }

// RecvMsgBuf returns ErrNotSupported on the stub build.
func (c *Conn) RecvMsgBuf(p []byte) (int, error) { return 0, gordma.ErrNotSupported }

// Read returns ErrNotSupported on the stub build.
func (c *Conn) Read(p []byte) (int, error) { return 0, gordma.ErrNotSupported }

// Write returns ErrNotSupported on the stub build.
func (c *Conn) Write(p []byte) (int, error) { return 0, gordma.ErrNotSupported }

// Close is a no-op on the stub build.
func (c *Conn) Close() error { return nil }

// LocalAddr returns the empty string on the stub build.
func (c *Conn) LocalAddr() string { return "" }

// RemoteAddr returns the empty string on the stub build.
func (c *Conn) RemoteAddr() string { return "" }

// ListenPacket returns ErrNotSupported on the stub build.
func ListenPacket(addr string, opts ...Option) (*PacketConn, error) {
	_ = applyOptions(opts)
	return nil, gordma.ErrNotSupported
}

// ReadFrom returns ErrNotSupported on the stub build.
func (p *PacketConn) ReadFrom(b []byte) (int, *Addr, error) {
	return 0, nil, gordma.ErrNotSupported
}

// WriteTo returns ErrNotSupported on the stub build.
func (p *PacketConn) WriteTo(b []byte, to *Addr) (int, error) {
	return 0, gordma.ErrNotSupported
}

// Close is a no-op on the stub build.
func (p *PacketConn) Close() error { return nil }

// LocalAddr returns nil on the stub build.
func (p *PacketConn) LocalAddr() *Addr { return nil }
