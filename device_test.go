package gordma

import (
	"errors"
	"testing"
)

// TestGetDeviceListNoDevice verifies that on an unsupported platform (or a
// machine with no RDMA device) GetDeviceList returns cleanly rather than
// panicking. On the stub build it must return ErrNotSupported.
func TestGetDeviceListBehaves(t *testing.T) {
	devs, free, err := GetDeviceList()
	if free == nil {
		t.Fatal("free function must never be nil")
	}
	defer free()

	if !Supported() {
		if !errors.Is(err, ErrNotSupported) {
			t.Fatalf("on unsupported platform want ErrNotSupported, got %v", err)
		}
		if devs != nil {
			t.Fatalf("expected nil device slice on unsupported platform, got %v", devs)
		}
		return
	}
	// On supported platforms we don't assert a device exists (CI may lack
	// hardware); we only require no panic and a sane error contract.
	if err != nil {
		t.Logf("GetDeviceList returned error (no hardware?): %v", err)
	}
}

// TestPortStateString checks the human-readable port states.
func TestPortStateString(t *testing.T) {
	cases := map[PortState]string{
		PortDown:   "DOWN",
		PortInit:   "INIT",
		PortActive: "ACTIVE",
	}
	for st, want := range cases {
		if got := st.String(); got != want {
			t.Errorf("PortState(%d).String() = %q, want %q", int(st), got, want)
		}
	}
}

// TestGIDString verifies the colon-separated formatting.
func TestGIDString(t *testing.T) {
	var g GID
	g[0] = 0xfe
	g[1] = 0x80
	g[15] = 0x01
	want := "fe80:0000:0000:0000:0000:0000:0000:0001"
	if got := g.String(); got != want {
		t.Errorf("GID.String() = %q, want %q", got, want)
	}
}
