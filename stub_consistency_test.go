//go:build !linux || !cgo

package gordma

import (
	"errors"
	"testing"
	"time"
)

// TestStubReturnsNotSupported is a single guard that every primary entry point
// on the stub build returns ErrNotSupported (and never panics). This is the
// core requirement of cross-platform compilation: the package links and runs
// on non-Linux / no-cgo targets, failing cleanly rather than crashing.
func TestStubReturnsNotSupported(t *testing.T) {
	if Supported() {
		t.Fatal("stub build must report Supported() == false")
	}

	if _, _, err := GetDeviceList(); !errors.Is(err, ErrNotSupported) {
		t.Errorf("GetDeviceList: want ErrNotSupported, got %v", err)
	}

	d := &Device{}
	if _, err := d.Open(); !errors.Is(err, ErrNotSupported) {
		t.Errorf("Device.Open: want ErrNotSupported, got %v", err)
	}

	c := &Context{}
	if _, err := c.QueryDevice(); !errors.Is(err, ErrNotSupported) {
		t.Errorf("QueryDevice: want ErrNotSupported, got %v", err)
	}
	if _, err := c.AllocPD(); !errors.Is(err, ErrNotSupported) {
		t.Errorf("AllocPD: want ErrNotSupported, got %v", err)
	}
	if _, err := c.CreateCQ(16, nil); !errors.Is(err, ErrNotSupported) {
		t.Errorf("CreateCQ: want ErrNotSupported, got %v", err)
	}

	if _, err := Listen("127.0.0.1:0"); !errors.Is(err, ErrNotSupported) {
		t.Errorf("Listen: want ErrNotSupported, got %v", err)
	}
	if _, err := Dial("127.0.0.1:1", time.Second); !errors.Is(err, ErrNotSupported) {
		t.Errorf("Dial: want ErrNotSupported, got %v", err)
	}
}
