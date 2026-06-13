//go:build linux && cgo

package rdmanet

import (
	"errors"
	"io"
	"testing"
)

// These tests cover the device-free parts of the message data path: error
// sentinels and the transport sizing guard. Framing, reassembly, and credit
// accounting are tested build-agnostically in framing_test.go / credit_test.go
// (including under -race). The full hardware round-trip is exercised by the
// bench tool (#44) on a real NIC.

func TestMessageErrorSentinelsDistinct(t *testing.T) {
	if errors.Is(ErrMessageTooLarge, ErrShortBuffer) {
		t.Error("ErrMessageTooLarge and ErrShortBuffer must be distinct")
	}
	if ErrMessageTooLarge == nil || ErrShortBuffer == nil {
		t.Error("error sentinels must be non-nil")
	}
}

// TestNewTransportRejectsBadSizing verifies invalid sizing is rejected before
// any verbs call (no device needed), for both poll modes.
func TestNewTransportRejectsBadSizing(t *testing.T) {
	for _, m := range []PollMode{PollEvent, PollBusy} {
		if _, err := newTransport(nil, nil, nil, frameHeaderSize, 4, m); err == nil {
			t.Errorf("newTransport(%v): want error for slotSize == frameHeaderSize", m)
		}
		if _, err := newTransport(nil, nil, nil, 1024, 0, m); err == nil {
			t.Errorf("newTransport(%v): want error for depth=0", m)
		}
	}
}

func TestSendMsgRejectsHugeMessage(t *testing.T) {
	// A message beyond the reassembly cap is rejected by the length guard before
	// any credit/slot work, so this needs no device.
	tr := &transport{payload: 8, depth: 1, closed: make(chan struct{}), credits: newCreditTracker(0)}
	if err := tr.sendMsg(make([]byte, maxMessageBytes+1)); !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("sendMsg oversized: want ErrMessageTooLarge, got %v", err)
	}
}

// TestSendBatchRejectsHugeMessage verifies the batch oversize guard runs before
// any credit/slot work, so it needs no device.
func TestSendBatchRejectsHugeMessage(t *testing.T) {
	tr := &transport{payload: 8, depth: 1, closed: make(chan struct{}), credits: newCreditTracker(0)}
	msgs := [][]byte{[]byte("ok"), make([]byte, maxMessageBytes+1)}
	if err := tr.sendBatch(msgs); !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("sendBatch oversized: want ErrMessageTooLarge, got %v", err)
	}
}

// TestNextMessageEOFOnFin verifies the receive path returns io.EOF once a FIN
// has been signalled and no buffered frames remain — without any RDMA device.
func TestNextMessageEOFOnFin(t *testing.T) {
	tr := &transport{
		depth:   1,
		recvQ:   make(chan recvFrame, 1),
		peerFin: make(chan struct{}),
		closed:  make(chan struct{}),
		credits: newCreditTracker(0),
		reasm:   newReassembler(0),
	}
	tr.finOnce.Do(func() { close(tr.peerFin) })
	if _, err := tr.nextMessage(); !errors.Is(err, io.EOF) {
		t.Errorf("nextMessage after FIN: want io.EOF, got %v", err)
	}
}

// TestNextMessageClosedReturnsClosedErr verifies a closed transport's receive
// path returns the closed error rather than blocking.
func TestNextMessageClosedReturnsClosedErr(t *testing.T) {
	tr := &transport{
		recvQ:   make(chan recvFrame),
		peerFin: make(chan struct{}),
		closed:  make(chan struct{}),
		credits: newCreditTracker(0),
		reasm:   newReassembler(0),
	}
	close(tr.closed)
	if _, err := tr.nextMessage(); err == nil {
		t.Error("nextMessage on closed transport: want error, got nil")
	}
}
