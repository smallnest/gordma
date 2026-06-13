//go:build linux && cgo

package rdmanet

import (
	"errors"
	"sync"
	"sync/atomic"
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

	// tr is the lazily-initialized RC data path (bounce rings + poller). It is
	// created on first SendMsg/RecvMsg so connections that are only used for
	// establishment pay nothing.
	trOnce sync.Once
	tr     *transport
	trErr  error

	// readMu serializes Read and guards reader, the byte-stream adapter over the
	// message transport (buffers the leftover of an oversized message).
	readMu sync.Mutex
	reader *streamReader

	// closed makes Close idempotent and lets send/recv fail fast after Close.
	closed    atomic.Bool
	closeOnce sync.Once
}

// Listener accepts incoming RC connections established via rdma_cm, or via the
// TCP out-of-band handshake when WithHandshake was set.
type Listener struct {
	cfg  config
	l    *gordma.Listener  // rdma_cm path
	hs   *handshake.Server // TCP handshake path
	addr string
}

// PacketConn is an unreliable-datagram (UD) RDMA endpoint. Its implementation
// lives in packet_linux.go.

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

// dataPath returns the connection's QP/CQ/PD regardless of how the Conn was
// built (rdma_cm CMConn or directly-owned handshake resources).
func (c *Conn) dataPath() (*gordma.QP, *gordma.CQ, *gordma.PD) {
	if c.cm != nil {
		return c.cm.QP(), c.cm.CQ(), c.cm.PD()
	}
	return c.qp, c.cq, c.pd
}

// transport lazily builds the RC data path (bounce rings + completion poller)
// on first use. The same transport is reused for all subsequent SendMsg/RecvMsg
// calls on this Conn. After Close it returns gordma.ErrClosed.
func (c *Conn) transport() (*transport, error) {
	if c.isClosed() {
		return nil, gordma.ErrClosed
	}
	c.trOnce.Do(func() {
		qp, cq, pd := c.dataPath()
		if qp == nil || cq == nil || pd == nil {
			c.trErr = gordma.ErrClosed
			return
		}
		c.tr, c.trErr = newTransport(pd, qp, cq, c.cfg.bufferSize, c.cfg.queueDepth)
	})
	return c.tr, c.trErr
}

func (c *Conn) isClosed() bool { return c.closed.Load() }

// SendMsg sends p as a single message, preserving its boundary, and blocks
// until the send completes. A message larger than the configured buffer size
// returns ErrMessageTooLarge (fragmentation lands in #36).
func (c *Conn) SendMsg(p []byte) error {
	tr, err := c.transport()
	if err != nil {
		return err
	}
	return tr.sendMsg(p)
}

// RecvMsg blocks until a full message arrives and returns it in a freshly
// allocated slice.
func (c *Conn) RecvMsg() ([]byte, error) {
	tr, err := c.transport()
	if err != nil {
		return nil, err
	}
	return tr.recvMsg()
}

// RecvMsgBuf blocks until a full message arrives and copies it into p. If p is
// too small, the message boundary is preserved and ErrShortBuffer is returned
// rather than truncating.
func (c *Conn) RecvMsgBuf(p []byte) (int, error) {
	tr, err := c.transport()
	if err != nil {
		return 0, err
	}
	return tr.recvMsgBuf(p)
}

// Read implements io.Reader over the message stream: it returns bytes from the
// received messages, transparently spanning message boundaries. A single Read
// returns at most one message's worth of remaining bytes; leftover bytes from a
// message larger than p are buffered and returned by subsequent Reads.
//
// Mixing Read with RecvMsg/RecvMsgBuf on the same Conn is not supported: the
// stream reader may have buffered part of a message that a subsequent RecvMsg
// would not see. Pick one receive style per Conn.
func (c *Conn) Read(p []byte) (int, error) {
	tr, err := c.transport()
	if err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if c.reader == nil {
		c.reader = &streamReader{recv: tr.recvMsg}
	}
	return c.reader.read(p)
}

// Write implements io.Writer over the message stream: it sends p as one message
// and reports it fully written, or returns an error. Per io.Writer semantics, a
// nil error means all len(p) bytes were written.
//
// Mixing Write with SendMsg on the same Conn is allowed (each Write is one
// SendMsg), but note that a reader using Read will see Write's payload spliced
// into the byte stream with no boundary.
func (c *Conn) Write(p []byte) (int, error) {
	tr, err := c.transport()
	if err != nil {
		return 0, err
	}
	if err := tr.sendMsg(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close releases the connection's QP/CQ/PD and CM resources. It is idempotent
// (safe to call multiple times) and wakes any goroutine blocked in
// SendMsg/RecvMsg/Read/Write, which then observe gordma.ErrClosed. For the
// handshake path it releases the directly-owned verbs resources in order
// (MR→QP→CQ→PD→Context). Releasing the transport sends a best-effort FIN so the
// peer's RecvMsg/Read returns io.EOF.
func (c *Conn) Close() error {
	if c == nil {
		return nil
	}
	var err error
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		if c.tr != nil {
			c.tr.shutdown()
		}
		if c.cm != nil {
			err = c.cm.Close()
			return
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
	})
	return err
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

// ListenPacket creates a UD datagram endpoint bound to addr. Implementation in
// packet_linux.go.

// (ReadFrom/WriteTo/Close/LocalAddr for PacketConn are defined in
// packet_linux.go.)
