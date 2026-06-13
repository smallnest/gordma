package rdmanet

import (
	"testing"

	"github.com/smallnest/gordma"
)

// The registry is pure Go (no cgo), so its register/lookup protocol is fully
// testable here without RDMA hardware.

func TestRegistryRegisterLookup(t *testing.T) {
	reg, err := NewRegistry("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer reg.Close()
	addrStr := reg.Addr().String()

	want := &Addr{
		GID:  gordma.GID{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x07},
		QPN:  0x4242,
		QKey: 0xabcd,
	}
	if err := registerAddr(addrStr, "node-a", want); err != nil {
		t.Fatalf("registerAddr: %v", err)
	}

	got, err := LookupAddr(addrStr, "node-a")
	if err != nil {
		t.Fatalf("LookupAddr: %v", err)
	}
	if got.GID != want.GID || got.QPN != want.QPN || got.QKey != want.QKey {
		t.Errorf("lookup: want %+v, got %+v", want, got)
	}
}

func TestRegistryLookupMissing(t *testing.T) {
	reg, err := NewRegistry("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer reg.Close()
	if _, err := LookupAddr(reg.Addr().String(), "absent"); err == nil {
		t.Error("LookupAddr(absent): want error, got nil")
	}
}

func TestRegistryReregisterOverwrites(t *testing.T) {
	reg, err := NewRegistry("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer reg.Close()
	addrStr := reg.Addr().String()

	a1 := &Addr{QPN: 1}
	a2 := &Addr{QPN: 2}
	if err := registerAddr(addrStr, "n", a1); err != nil {
		t.Fatal(err)
	}
	if err := registerAddr(addrStr, "n", a2); err != nil {
		t.Fatal(err)
	}
	got, err := LookupAddr(addrStr, "n")
	if err != nil {
		t.Fatal(err)
	}
	if got.QPN != 2 {
		t.Errorf("re-register: want QPN 2, got %d", got.QPN)
	}
}

func TestRegisterNilAddr(t *testing.T) {
	if err := registerAddr("127.0.0.1:1", "n", nil); err == nil {
		t.Error("registerAddr(nil): want error, got nil")
	}
}

func TestLookupAddrDialError(t *testing.T) {
	// Unreachable registry: LookupAddr must return a dial error, not panic.
	if _, err := LookupAddr("127.0.0.1:1", "n"); err == nil {
		t.Error("LookupAddr to dead addr: want error, got nil")
	}
}
