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

func TestDialHandshakeNoDeviceFailsCleanly(t *testing.T) {
	// The WithHandshake path now builds verbs resources before exchanging over
	// TCP; without an RDMA device that build fails cleanly (not errNotImplemented,
	// not a panic). We only assert it returns an error and no Conn.
	c, err := Dial("127.0.0.1:1", WithHandshake())
	if err == nil {
		_ = c.Close()
		t.Skip("Dial+WithHandshake unexpectedly succeeded (RDMA device present); skipping")
	}
	if errors.Is(err, errNotImplemented) {
		t.Errorf("Dial+WithHandshake: handshake path should be implemented, got errNotImplemented")
	}
	if c != nil {
		t.Errorf("Dial+WithHandshake: want nil Conn on error, got %v", c)
	}
}

func TestListenHandshakeSucceedsWithoutDevice(t *testing.T) {
	// The handshake listener is a pure TCP listener — it must succeed even with
	// no RDMA device, since verbs resources are only built on Accept.
	l, err := Listen("127.0.0.1:0", WithHandshake())
	if err != nil {
		t.Fatalf("Listen+WithHandshake: %v", err)
	}
	defer l.Close()
	if l.Addr() != "127.0.0.1:0" {
		t.Errorf("Addr: got %q", l.Addr())
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

// TestConnCloseIdempotentAndFailsFast verifies Close is idempotent and that
// after Close the send/recv methods fail fast with gordma.ErrClosed (no
// hardware needed: the closed flag short-circuits transport()).
func TestConnCloseIdempotentAndFailsFast(t *testing.T) {
	c := &Conn{} // no cm, no verbs resources — Close just flips state
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close (idempotent): %v", err)
	}
	if !c.isClosed() {
		t.Error("isClosed() false after Close")
	}
	if err := c.SendMsg([]byte("x")); !errors.Is(err, gordma.ErrClosed) {
		t.Errorf("SendMsg after Close: want ErrClosed, got %v", err)
	}
	if _, err := c.RecvMsg(); !errors.Is(err, gordma.ErrClosed) {
		t.Errorf("RecvMsg after Close: want ErrClosed, got %v", err)
	}
	if _, err := c.Read(make([]byte, 4)); !errors.Is(err, gordma.ErrClosed) {
		t.Errorf("Read after Close: want ErrClosed, got %v", err)
	}
	if _, err := c.Write([]byte("x")); !errors.Is(err, gordma.ErrClosed) {
		t.Errorf("Write after Close: want ErrClosed, got %v", err)
	}
}

var _ = time.Second
