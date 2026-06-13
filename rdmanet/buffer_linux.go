//go:build linux && cgo

package rdmanet

import (
	"github.com/smallnest/gordma"
)

// Buffer is a pre-registered, pinned memory region lent to the caller for
// zero-copy sends. Fill Bytes()[:n], then submit it with Conn.SendBuffer to
// transmit without the bounce-buffer copy that SendMsg performs.
//
// Lifecycle:
//   - AllocBuffer returns an idle Buffer.
//   - SendBuffer transitions it to in-flight and blocks until the SEND
//     completes, then returns it to idle — so on return the buffer may be
//     refilled and sent again.
//   - Close (or the owning Conn's Close) releases it; a released Buffer must not
//     be used again (ErrBufferReleased).
//
// A Buffer must not be mutated while in flight. Zero-copy send and ordinary
// SendMsg may be mixed on the same Conn; both are serialized by the send mutex.
type Buffer struct {
	mr    *gordma.MR
	buf   []byte
	size  int
	state bufferState
	tr    *transport
}

// Bytes returns the buffer's pinned backing slice (length == requested size).
// Write your payload here before SendBuffer; the slice aliases registered
// memory and must not be retained after Close.
func (b *Buffer) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.buf
}

// Close deregisters and releases the buffer. It is an error (ErrBufferReleased)
// to close twice or to use the buffer afterward.
func (b *Buffer) Close() error {
	if b == nil {
		return nil
	}
	if err := b.state.release(); err != nil {
		return err
	}
	if b.mr != nil {
		_ = b.mr.Close()
		b.mr = nil
	}
	return nil
}

// AllocBuffer registers a pinned buffer of size bytes on this connection's PD
// for zero-copy sends. The returned Buffer is owned by the caller until Close.
func (c *Conn) AllocBuffer(size int) (*Buffer, error) {
	if size <= 0 {
		return nil, errBufferSize
	}
	tr, err := c.transport()
	if err != nil {
		return nil, err
	}
	_, _, pd := c.dataPath()
	if pd == nil {
		return nil, gordma.ErrClosed
	}
	mr, err := pd.RegMRBuffer(size, gordma.AccessLocalWrite)
	if err != nil {
		return nil, err
	}
	return &Buffer{mr: mr, buf: mr.Bytes(), size: size, tr: tr}, nil
}

// SendBuffer transmits the buffer's contents as one message with no
// intermediate copy, blocking until the SEND completes. On return the buffer is
// idle and may be refilled and sent again. It returns ErrBufferInFlight if a
// previous send is still outstanding and ErrBufferReleased if the buffer was
// closed.
func (c *Conn) SendBuffer(b *Buffer) error {
	if b == nil {
		return ErrBufferReleased
	}
	tr, err := c.transport()
	if err != nil {
		return err
	}
	return tr.sendBuffer(b)
}

// RecvBuffer receives one message into a freshly allocated Buffer's pinned
// memory. NOTE: the current RC data path reassembles into bounce slots, so the
// returned Buffer holds a copy; the API is provided for symmetry and forward
// compatibility with a future direct-placement recv path. Callers must Close
// the returned Buffer when done.
func (c *Conn) RecvBuffer() (*Buffer, error) {
	tr, err := c.transport()
	if err != nil {
		return nil, err
	}
	msg, err := tr.recvMsg()
	if err != nil {
		return nil, err
	}
	_, _, pd := c.dataPath()
	if pd == nil {
		return nil, gordma.ErrClosed
	}
	mr, err := pd.RegMRBuffer(len(msg)+1, gordma.AccessLocalWrite) // +1 so size>0 for empty msgs
	if err != nil {
		return nil, err
	}
	copy(mr.Bytes(), msg)
	return &Buffer{mr: mr, buf: mr.Bytes()[:len(msg)], size: len(msg), tr: tr}, nil
}
