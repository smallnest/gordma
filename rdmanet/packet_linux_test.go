//go:build linux && cgo

package rdmanet

import (
	"errors"
	"testing"
)

// PacketConn's full datagram round-trip is hardware-dependent (exercised by the
// bench tool, #44). These tests cover the device-free surface: clean failure
// without a NIC, the MTU guard, and nil-address handling.

func TestListenPacketNoDeviceFailsCleanly(t *testing.T) {
	pc, err := ListenPacket("127.0.0.1:0")
	if err == nil {
		_ = pc.Close()
		t.Skip("ListenPacket unexpectedly succeeded (RDMA device present); skipping")
	}
	if pc != nil {
		t.Errorf("ListenPacket: want nil PacketConn on error, got %v", pc)
	}
}

func TestPacketConnWriteToGuards(t *testing.T) {
	// A PacketConn with a known mtu but no live QP lets us exercise the input
	// guards (nil addr, oversize) which run before any verbs call.
	pc := &PacketConn{mtu: 1024, closed: make(chan struct{})}
	if _, err := pc.WriteTo([]byte("x"), nil); err == nil {
		t.Error("WriteTo(nil addr): want error, got nil")
	}
	big := make([]byte, pc.mtu+1)
	if _, err := pc.WriteTo(big, &Addr{QPN: 1}); !errors.Is(err, ErrDatagramTooLarge) {
		t.Errorf("WriteTo oversize: want ErrDatagramTooLarge, got %v", err)
	}
}

func TestPacketConnLocalAddrNilSafe(t *testing.T) {
	var pc *PacketConn
	if pc.LocalAddr() != nil {
		t.Error("nil PacketConn.LocalAddr: want nil")
	}
	if err := pc.Close(); err != nil {
		t.Errorf("nil PacketConn.Close: want nil, got %v", err)
	}
}
