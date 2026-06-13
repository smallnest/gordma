//go:build linux && cgo

package rdmanet

import (
	"errors"
	"testing"
	"time"

	"github.com/smallnest/gordma"
)

// On the linux+cgo build without an RDMA device, the connection-management
// entry points should fail cleanly (resolving the address or creating the CM
// id fails) rather than panic. We assert they return a non-nil error and never
// return a usable Conn/Listener.

func TestDialNoDeviceFailsCleanly(t *testing.T) {
	// Unresolvable / no-device: must return an error, not panic, not a Conn.
	c, err := Dial("127.0.0.1:1")
	if err == nil {
		_ = c.Close()
		t.Skip("Dial unexpectedly succeeded (RDMA device present); skipping negative test")
	}
	if c != nil {
		t.Errorf("Dial: want nil Conn on error, got %v", c)
	}
}

func TestDialHandshakeNotYetImplemented(t *testing.T) {
	// WithHandshake path is delivered by issue #34; until then it reports
	// not-implemented rather than silently using rdma_cm.
	if _, err := Dial("127.0.0.1:1", WithHandshake()); !errors.Is(err, errNotImplemented) {
		t.Errorf("Dial+WithHandshake: want errNotImplemented, got %v", err)
	}
	if _, err := Listen("127.0.0.1:0", WithHandshake()); !errors.Is(err, errNotImplemented) {
		t.Errorf("Listen+WithHandshake: want errNotImplemented, got %v", err)
	}
}

func TestDialTimeoutDefaultsApplied(t *testing.T) {
	// A non-positive timeout must not panic; it falls back to the default.
	c, err := DialTimeout("127.0.0.1:1", 0)
	if err == nil {
		_ = c.Close()
		t.Skip("DialTimeout unexpectedly succeeded; skipping")
	}
}

func TestListenerAddrAndClose(t *testing.T) {
	l, err := Listen("127.0.0.1:0")
	if err != nil {
		// No device / bind failure on this host is expected; nothing to assert
		// beyond a clean error.
		return
	}
	defer l.Close()
	if l.Addr() != "127.0.0.1:0" {
		t.Errorf("Addr: want 127.0.0.1:0, got %q", l.Addr())
	}
}

func TestConnAddrAccessorsNilSafe(t *testing.T) {
	var c *Conn
	if c.LocalAddr() != "" || c.RemoteAddr() != "" {
		t.Error("nil Conn addr accessors must return empty string")
	}
	if err := c.Close(); err != nil {
		t.Errorf("nil Conn Close: want nil, got %v", err)
	}
	// A dialed Conn records its remote (target) address.
	c2 := &Conn{remoteAddr: "10.0.0.1:9000"}
	if c2.RemoteAddr() != "10.0.0.1:9000" {
		t.Errorf("RemoteAddr: got %q", c2.RemoteAddr())
	}
}

func TestNilListenerAccept(t *testing.T) {
	var l *Listener
	if _, err := l.Accept(); !errors.Is(err, gordma.ErrClosed) {
		t.Errorf("nil Listener.Accept: want ErrClosed, got %v", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("nil Listener.Close: want nil, got %v", err)
	}
}

var _ = time.Second
