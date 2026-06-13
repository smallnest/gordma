//go:build !linux || !cgo

package rdmanet

import (
	"errors"
	"testing"
	"time"

	"github.com/smallnest/gordma"
)

// TestStubReturnsNotSupported guards that every primary rdmanet entry point on
// the stub build returns gordma.ErrNotSupported (and never panics). This is the
// cross-platform contract: importing rdmanet links and runs on non-Linux /
// no-cgo targets, failing cleanly rather than crashing.
func TestStubReturnsNotSupported(t *testing.T) {
	if gordma.Supported() {
		t.Fatal("stub build must report gordma.Supported() == false")
	}

	if _, err := Dial("127.0.0.1:1"); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("Dial: want ErrNotSupported, got %v", err)
	}
	if _, err := DialTimeout("127.0.0.1:1", time.Second); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("DialTimeout: want ErrNotSupported, got %v", err)
	}
	if _, err := Listen("127.0.0.1:0"); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("Listen: want ErrNotSupported, got %v", err)
	}
	if _, err := ListenPacket("127.0.0.1:0"); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("ListenPacket: want ErrNotSupported, got %v", err)
	}

	l := &Listener{}
	if _, err := l.Accept(); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("Listener.Accept: want ErrNotSupported, got %v", err)
	}

	c := &Conn{}
	if err := c.SendMsg([]byte("x")); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("Conn.SendMsg: want ErrNotSupported, got %v", err)
	}
	if _, err := c.RecvMsg(); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("Conn.RecvMsg: want ErrNotSupported, got %v", err)
	}
	if _, err := c.RecvMsgBuf(make([]byte, 4)); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("Conn.RecvMsgBuf: want ErrNotSupported, got %v", err)
	}
	if _, err := c.Read(make([]byte, 4)); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("Conn.Read: want ErrNotSupported, got %v", err)
	}
	if _, err := c.Write([]byte("x")); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("Conn.Write: want ErrNotSupported, got %v", err)
	}

	pc := &PacketConn{}
	if _, _, err := pc.ReadFrom(make([]byte, 4)); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("PacketConn.ReadFrom: want ErrNotSupported, got %v", err)
	}
	if _, err := pc.WriteTo([]byte("x"), &Addr{}); !errors.Is(err, gordma.ErrNotSupported) {
		t.Errorf("PacketConn.WriteTo: want ErrNotSupported, got %v", err)
	}
}
