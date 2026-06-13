package rdmanet

import (
	"errors"
	"sync/atomic"
)

// Buffer lifecycle errors.
var (
	// ErrBufferInFlight is returned when a buffer is submitted for send while a
	// previous send has not completed, or otherwise used while owned by the
	// transport.
	ErrBufferInFlight = errors.New("rdmanet: buffer is in flight")
	// ErrBufferReleased is returned when a released/closed buffer is used.
	ErrBufferReleased = errors.New("rdmanet: buffer already released")
	// errBufferSize is returned by AllocBuffer for a non-positive size.
	errBufferSize = errors.New("rdmanet: buffer size must be > 0")
)

// bufState is the lifecycle state of a zero-copy Buffer. The state machine is
// build-agnostic so it can be unit-tested without RDMA hardware; the linux
// Buffer embeds it alongside the registered MR.
//
//	idle ──Send()──▶ inFlight ──completion──▶ idle
//	  │
//	  └──Close()──▶ released  (terminal)
type bufState int32

const (
	bufIdle bufState = iota
	bufInFlight
	bufReleased
)

// bufferState tracks a Buffer's lifecycle with atomic transitions so the send
// path and completion poller can coordinate without a mutex on the hot path.
type bufferState struct {
	state atomic.Int32
}

// beginSend transitions idle→inFlight. It returns ErrBufferInFlight if a send
// is already outstanding and ErrBufferReleased if the buffer was closed.
func (b *bufferState) beginSend() error {
	switch bufState(b.state.Load()) {
	case bufReleased:
		return ErrBufferReleased
	case bufInFlight:
		return ErrBufferInFlight
	}
	if !b.state.CompareAndSwap(int32(bufIdle), int32(bufInFlight)) {
		// Lost the race; re-read for the precise error.
		if bufState(b.state.Load()) == bufReleased {
			return ErrBufferReleased
		}
		return ErrBufferInFlight
	}
	return nil
}

// completeSend transitions inFlight→idle after a send completion. It is a no-op
// if the buffer was released meanwhile.
func (b *bufferState) completeSend() {
	b.state.CompareAndSwap(int32(bufInFlight), int32(bufIdle))
}

// release transitions the buffer to the terminal released state. It returns
// ErrBufferReleased if already released (double-release is a misuse).
func (b *bufferState) release() error {
	if b.state.Swap(int32(bufReleased)) == int32(bufReleased) {
		return ErrBufferReleased
	}
	return nil
}

// isReleased reports whether the buffer has been released/closed.
func (b *bufferState) isReleased() bool {
	return bufState(b.state.Load()) == bufReleased
}
