package gordma

import (
	"errors"
	"testing"
)

// TestAllocPDUnsupported verifies the stub contract for PD allocation.
func TestAllocPDUnsupported(t *testing.T) {
	if Supported() {
		t.Skip("real platform: PD allocation needs hardware")
	}
	c := &Context{}
	if _, err := c.AllocPD(); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("want ErrNotSupported, got %v", err)
	}
}

// TestRegMRBufferUnsupported verifies the stub contract for MR registration.
func TestRegMRBufferUnsupported(t *testing.T) {
	if Supported() {
		t.Skip("real platform: MR registration needs hardware")
	}
	p := &PD{}
	if _, err := p.RegMRBuffer(4096, AccessLocalWrite); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("want ErrNotSupported, got %v", err)
	}
}

// TestMRZeroValueSafe verifies that a zero-value MR's accessors and Close do
// not panic — important since callers may defer Close before a failed reg.
func TestMRZeroValueSafe(t *testing.T) {
	var m MR
	if m.LKey() != 0 || m.RKey() != 0 || m.Addr() != 0 || m.Len() != 0 || m.Bytes() != nil {
		t.Fatal("zero-value MR accessors must return zero values")
	}
	if err := m.Close(); err != nil {
		t.Fatalf("zero-value MR Close must be nil, got %v", err)
	}
}
