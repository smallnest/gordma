//go:build linux && cgo

package rdmanet

import (
	"errors"
	"testing"
)

// These tests cover the parts of the message data path that do not require an
// RDMA device. The full send/recv round-trip is hardware-dependent and is
// exercised by the bench tool (#44) on a real NIC.

func TestMessageErrorSentinelsDistinct(t *testing.T) {
	if errors.Is(ErrMessageTooLarge, ErrShortBuffer) {
		t.Error("ErrMessageTooLarge and ErrShortBuffer must be distinct")
	}
	if ErrMessageTooLarge == nil || ErrShortBuffer == nil {
		t.Error("error sentinels must be non-nil")
	}
}

func TestNewTransportRejectsBadSizing(t *testing.T) {
	// Invalid sizing is rejected before any verbs call, so this runs without a
	// device.
	if _, err := newTransport(nil, nil, nil, 0, 4); err == nil {
		t.Error("newTransport: want error for slotSize=0")
	}
	if _, err := newTransport(nil, nil, nil, 1024, 0); err == nil {
		t.Error("newTransport: want error for depth=0")
	}
}

func TestSendMsgTooLargeWithoutHardware(t *testing.T) {
	// A message larger than the buffer is rejected by the length guard. Build a
	// transport only if a device is available; otherwise assert the guard via a
	// hand-made transport whose rings are nil but slotSize is set — sendMsg's
	// size check happens before touching the rings.
	tr := &transport{slotSize: 8, depth: 1, closed: make(chan struct{})}
	if err := tr.sendMsg(make([]byte, 9)); !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("sendMsg oversized: want ErrMessageTooLarge, got %v", err)
	}
}
