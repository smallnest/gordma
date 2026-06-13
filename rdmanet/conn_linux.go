//go:build linux && cgo

package rdmanet

import (
	"errors"
	"time"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/handshake"
)

// errNotImplemented is returned by Linux entry points whose real RDMA
// machinery is delivered by later issues in the rdmanet series (messaging,
// flow control, datagrams, zero-copy). The connection-management surface
// (Dial/Listen/Accept/Close + addresses) is implemented here; the data-path
// methods remain stubbed until their issues land.
var errNotImplemented = errors.New("rdmanet: not implemented yet")

// Conn is a reliable-connected (RC) RDMA endpoint. It is produced either by
// the rdma_cm path (wrapping a gordma.CMConn whose QP is in RTS) or by the TCP
// out-of-band handshake path (owning the verbs resources it created). Both
// produce the same Conn type so callers are agnostic to how it was built.
type Conn struct {
	cfg config

	// rdma_cm ownership: when cm != nil, it owns PD/CQ/QP and is closed on Close.
	cm *gordma.CMConn

	// handshake ownership: when cm == nil, these are owned directly and closed
	// in order on Close. peer carries the exchanged remote endpoint info that
	// the data path (issue #35) needs to target the peer.
	ctx  *gordma.Context
	pd   *gordma.PD
	cq   *gordma.CQ
	qp   *gordma.QP
	mr   *gordma.MR
	peer *handshake.EndpointInfo

	localAddr  string
	remoteAddr string
}

// Listener accepts incoming RC connections established via rdma_cm, or via the
// TCP out-of-band handshake when WithHandshake was set.
type Listener struct {
	cfg  config
	l    *gordma.Listener  // rdma_cm path
	hs   *handshake.Server // TCP handshake path
	addr string
}

// PacketConn is an unreliable-datagram (UD) RDMA endpoint. Implemented by the
// UD issue (#39); the skeleton keeps it inert here.
type PacketConn struct {
	cfg config
}

// Dial establishes an RC connection to addr ("host:port") using the default
// DefaultDialTimeout. Connection is made via the RDMA connection manager
// (rdma_cm) unless WithHandshake is supplied (handled by a later issue).
func Dial(addr string, opts ...Option) (*Conn, error) {
	return DialTimeout(addr, DefaultDialTimeout, opts...)
}

// DialTimeout is Dial with an explicit timeout. A non-positive timeout falls
// back to DefaultDialTimeout.
func DialTimeout(addr string, timeout time.Duration, opts ...Option) (*Conn, error) {
	cfg := applyOptions(opts)
	if timeout <= 0 {
		timeout = DefaultDialTimeout
	}
	if cfg.handshake {
		return dialHandshake(addr, timeout, cfg)
	}
	cm, err := gordma.Dial(addr, timeout)
	if err != nil {
		return nil, err
	}
	return &Conn{cfg: cfg, cm: cm, remoteAddr: addr}, nil
}

// Listen creates an RC listener bound to addr ("host:port") via rdma_cm, or via
// the TCP out-of-band handshake when WithHandshake is set.
func Listen(addr string, opts ...Option) (*Listener, error) {
	cfg := applyOptions(opts)
	if cfg.handshake {
		hs, err := handshake.Listen(addr)
		if err != nil {
			return nil, err
		}
		return &Listener{cfg: cfg, hs: hs, addr: addr}, nil
	}
	l, err := gordma.Listen(addr)
	if err != nil {
		return nil, err
	}
	return &Listener{cfg: cfg, l: l, addr: addr}, nil
}

// Accept waits for and returns the next RC connection. The returned Conn's QP
// is already in RTS.
func (l *Listener) Accept() (*Conn, error) {
	if l == nil {
		return nil, gordma.ErrClosed
	}
	if l.hs != nil {
		return acceptHandshake(l)
	}
	if l.l == nil {
		return nil, gordma.ErrClosed
	}
	cm, err := l.l.Accept()
	if err != nil {
		return nil, err
	}
	return &Conn{cfg: l.cfg, cm: cm, localAddr: l.addr}, nil
}

// Close stops listening and releases the listener's resources.
func (l *Listener) Close() error {
	if l == nil {
		return nil
	}
	if l.hs != nil {
		return l.hs.Close()
	}
	if l.l == nil {
		return nil
	}
	return l.l.Close()
}

// Addr returns the listener's bind address ("host:port").
func (l *Listener) Addr() string {
	if l == nil {
		return ""
	}
	return l.addr
}

// SendMsg sends a single message, preserving its boundary. (Issue #35.)
func (c *Conn) SendMsg(p []byte) error { return errNotImplemented }

// RecvMsg receives a single message. (Issue #35.)
func (c *Conn) RecvMsg() ([]byte, error) { return nil, errNotImplemented }

// RecvMsgBuf receives a single message into p. (Issue #35.)
func (c *Conn) RecvMsgBuf(p []byte) (int, error) { return 0, errNotImplemented }

// Read implements io.Reader over the message stream. (Issue #37.)
func (c *Conn) Read(p []byte) (int, error) { return 0, errNotImplemented }

// Write implements io.Writer over the message stream. (Issue #37.)
func (c *Conn) Write(p []byte) (int, error) { return 0, errNotImplemented }

// Close releases the connection's QP/CQ/PD and CM resources. For the handshake
// path it releases the directly-owned verbs resources in order
// (MR→QP→CQ→PD→Context).
func (c *Conn) Close() error {
	if c == nil {
		return nil
	}
	if c.cm != nil {
		return c.cm.Close()
	}
	if c.mr != nil {
		_ = c.mr.Close()
		c.mr = nil
	}
	if c.qp != nil {
		_ = c.qp.Close()
		c.qp = nil
	}
	if c.cq != nil {
		_ = c.cq.Close()
		c.cq = nil
	}
	if c.pd != nil {
		_ = c.pd.Close()
		c.pd = nil
	}
	if c.ctx != nil {
		_ = c.ctx.Close()
		c.ctx = nil
	}
	return nil
}

// LocalAddr returns the local endpoint address ("host:port"). It is populated
// for accepted connections (the listener's bind address); for dialed
// connections the local address is not exposed by rdma_cm and is empty.
func (c *Conn) LocalAddr() string {
	if c == nil {
		return ""
	}
	return c.localAddr
}

// RemoteAddr returns the remote endpoint address ("host:port"). It is populated
// for dialed connections (the dial target); for accepted connections the peer
// address is not exposed by rdma_cm and is empty.
func (c *Conn) RemoteAddr() string {
	if c == nil {
		return ""
	}
	return c.remoteAddr
}

// ListenPacket creates a UD datagram endpoint bound to addr. (Issue #39.)
func ListenPacket(addr string, opts ...Option) (*PacketConn, error) {
	_ = applyOptions(opts)
	return nil, errNotImplemented
}

// ReadFrom reads a datagram and reports the sender's address. (Issue #39.)
func (p *PacketConn) ReadFrom(b []byte) (int, *Addr, error) {
	return 0, nil, errNotImplemented
}

// WriteTo writes a datagram to the given address. (Issue #39.)
func (p *PacketConn) WriteTo(b []byte, to *Addr) (int, error) {
	return 0, errNotImplemented
}

// Close releases the datagram endpoint. (Issue #39.)
func (p *PacketConn) Close() error { return errNotImplemented }

// LocalAddr returns the local UD address. (Issue #39.)
func (p *PacketConn) LocalAddr() *Addr { return nil }
