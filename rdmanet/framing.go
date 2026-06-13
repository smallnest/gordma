package rdmanet

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// This file holds the build-agnostic wire framing, reassembly, and credit
// accounting for the RC data path. Keeping it free of cgo lets it be unit
// tested (including under -race) on any platform, independent of RDMA hardware.

// frameHeaderSize is the fixed per-frame header prepended to every SEND on the
// data path. Layout (big-endian):
//
//	byte 0:      flags (flagMore | flagCredit)
//	bytes 1..3:  reserved (zero)
//	bytes 4..7:  uint32 — for DATA frames, the payload length in this frame;
//	             for CREDIT frames, the number of credits being returned.
const frameHeaderSize = 8

const (
	// flagMore indicates more fragments of the current message follow. The
	// final fragment (or a single-frame message) has flagMore clear.
	flagMore uint8 = 1 << 0
	// flagCredit marks a control frame that carries returned receive credits
	// rather than message payload.
	flagCredit uint8 = 1 << 1
	// flagFin marks a graceful-shutdown control frame: the sender will send no
	// more messages. The receiver returns io.EOF once buffered frames drain.
	flagFin uint8 = 1 << 2
)

// ErrMessageTooLargeReassembly is returned when an inbound message exceeds the
// reassembler's configured maximum, protecting the receiver from unbounded
// growth driven by a misbehaving or buggy peer.
var ErrMessageTooLargeReassembly = errors.New("rdmanet: reassembled message exceeds maximum")

// frameHeader is the decoded form of the on-wire header.
type frameHeader struct {
	flags uint8
	// value is the payload length (DATA) or credit count (CREDIT).
	value uint32
}

// encode writes h into the first frameHeaderSize bytes of buf. buf must be at
// least frameHeaderSize long.
func (h frameHeader) encode(buf []byte) {
	_ = buf[frameHeaderSize-1] // bounds-check hint
	buf[0] = h.flags
	buf[1] = 0
	buf[2] = 0
	buf[3] = 0
	binary.BigEndian.PutUint32(buf[4:8], h.value)
}

// decodeHeader reads a frameHeader from the first frameHeaderSize bytes of buf.
func decodeHeader(buf []byte) (frameHeader, error) {
	if len(buf) < frameHeaderSize {
		return frameHeader{}, fmt.Errorf("rdmanet: frame too short: %d bytes", len(buf))
	}
	return frameHeader{
		flags: buf[0],
		value: binary.BigEndian.Uint32(buf[4:8]),
	}, nil
}

// isCredit reports whether the header marks a credit control frame.
func (h frameHeader) isCredit() bool { return h.flags&flagCredit != 0 }

// isFin reports whether the header marks a graceful-shutdown control frame.
func (h frameHeader) isFin() bool { return h.flags&flagFin != 0 }

// hasMore reports whether more fragments follow this data frame.
func (h frameHeader) hasMore() bool { return h.flags&flagMore != 0 }

// reassembler accumulates inbound DATA fragments into complete messages. It is
// not safe for concurrent use; the transport calls it only from the single
// receive path.
type reassembler struct {
	buf    []byte
	max    int
	active bool
}

// newReassembler returns a reassembler that rejects messages larger than max
// bytes. A non-positive max disables the limit.
func newReassembler(max int) *reassembler {
	return &reassembler{max: max}
}

// add appends a fragment's payload. When more is false the fragment completes
// the message: add returns the full message and true, and resets for the next
// message. Otherwise it returns nil, false. It errors if the accumulated size
// would exceed max.
func (r *reassembler) add(payload []byte, more bool) ([]byte, bool, error) {
	if r.max > 0 && len(r.buf)+len(payload) > r.max {
		// Drop the partial message to avoid unbounded growth.
		r.buf = r.buf[:0]
		r.active = false
		return nil, false, ErrMessageTooLargeReassembly
	}
	r.buf = append(r.buf, payload...)
	r.active = true
	if more {
		return nil, false, nil
	}
	msg := make([]byte, len(r.buf))
	copy(msg, r.buf)
	r.buf = r.buf[:0]
	r.active = false
	return msg, true, nil
}

// fragmentCount returns how many frames a message of msgLen bytes needs given a
// per-frame payload capacity of chunk bytes. A zero-length message still takes
// one frame so the boundary is transmitted.
func fragmentCount(msgLen, chunk int) int {
	if chunk <= 0 {
		return 0
	}
	if msgLen == 0 {
		return 1
	}
	return (msgLen + chunk - 1) / chunk
}
