//go:build linux && cgo

package rdmanet

import (
	"errors"
	"time"
)

// errNotImplemented is returned by the Linux build for entry points whose real
// RDMA machinery is delivered by later issues in the rdmanet series. The
// skeleton issue (#32) establishes the package, options, and API surface; the
// connection, messaging, datagram, and zero-copy implementations land on top
// of it. This keeps the Linux build compiling and vet-clean without pretending
// to do work it does not yet do.
var errNotImplemented = errors.New("rdmanet: not implemented yet")

// Conn is a reliable-connected (RC) RDMA endpoint. Fields are filled in by the
// connection/messaging issues; the skeleton keeps it empty.
type Conn struct {
	cfg config
}

// Listener accepts incoming RC connections.
type Listener struct {
	cfg config
}

// PacketConn is an unreliable-datagram (UD) RDMA endpoint.
type PacketConn struct {
	cfg config
}

// Dial establishes an RC connection to addr. (Implementation lands in a later
// issue; the skeleton resolves options and reports not-implemented.)
func Dial(addr string, opts ...Option) (*Conn, error) {
	_ = applyOptions(opts)
	return nil, errNotImplemented
}

// DialTimeout is Dial with an explicit timeout.
func DialTimeout(addr string, timeout time.Duration, opts ...Option) (*Conn, error) {
	_ = applyOptions(opts)
	return nil, errNotImplemented
}

// Listen creates an RC listener bound to addr.
func Listen(addr string, opts ...Option) (*Listener, error) {
	_ = applyOptions(opts)
	return nil, errNotImplemented
}

// Accept waits for and returns the next RC connection.
func (l *Listener) Accept() (*Conn, error) { return nil, errNotImplemented }

// Close releases the listener.
func (l *Listener) Close() error { return errNotImplemented }

// Addr returns the listener's address.
func (l *Listener) Addr() string { return "" }

// SendMsg sends a single message, preserving its boundary.
func (c *Conn) SendMsg(p []byte) error { return errNotImplemented }

// RecvMsg receives a single message.
func (c *Conn) RecvMsg() ([]byte, error) { return nil, errNotImplemented }

// RecvMsgBuf receives a single message into p.
func (c *Conn) RecvMsgBuf(p []byte) (int, error) { return 0, errNotImplemented }

// Read implements io.Reader over the message stream.
func (c *Conn) Read(p []byte) (int, error) { return 0, errNotImplemented }

// Write implements io.Writer over the message stream.
func (c *Conn) Write(p []byte) (int, error) { return 0, errNotImplemented }

// Close releases the connection's resources.
func (c *Conn) Close() error { return errNotImplemented }

// LocalAddr returns the local endpoint address.
func (c *Conn) LocalAddr() string { return "" }

// RemoteAddr returns the remote endpoint address.
func (c *Conn) RemoteAddr() string { return "" }

// ListenPacket creates a UD datagram endpoint bound to addr.
func ListenPacket(addr string, opts ...Option) (*PacketConn, error) {
	_ = applyOptions(opts)
	return nil, errNotImplemented
}

// ReadFrom reads a datagram and reports the sender's address.
func (p *PacketConn) ReadFrom(b []byte) (int, *Addr, error) {
	return 0, nil, errNotImplemented
}

// WriteTo writes a datagram to the given address.
func (p *PacketConn) WriteTo(b []byte, to *Addr) (int, error) {
	return 0, errNotImplemented
}

// Close releases the datagram endpoint.
func (p *PacketConn) Close() error { return errNotImplemented }

// LocalAddr returns the local UD address.
func (p *PacketConn) LocalAddr() *Addr { return nil }
