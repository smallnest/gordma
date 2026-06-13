//go:build linux && cgo

package rdmanet

import (
	"errors"
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

func TestNewTransportRejectsBadSizing(t *testing.T) {
	// Invalid sizing is rejected before any verbs call, so this runs without a
	// device. slotSize must exceed the frame header.
	if _, err := newTransport(nil, nil, nil, frameHeaderSize, 4); err == nil {
		t.Error("newTransport: want error for slotSize == frameHeaderSize")
	}
	if _, err := newTransport(nil, nil, nil, 1024, 0); err == nil {
		t.Error("newTransport: want error for depth=0")
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
