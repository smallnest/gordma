package rdmanet

import (
	"testing"

	"github.com/smallnest/gordma"
)

func TestAddrStringRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		addr Addr
		want string
	}{
		{
			name: "default qkey omitted",
			addr: Addr{GID: gordma.GID{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}, QPN: 0x12ab},
			want: "fe80:0000:0000:0000:0000:0000:0000:0001%0x12ab",
		},
		{
			name: "explicit DefaultQKey omitted",
			addr: Addr{GID: gordma.GID{}, QPN: 1, QKey: DefaultQKey},
			want: "0000:0000:0000:0000:0000:0000:0000:0000%0x1",
		},
		{
			name: "custom qkey appended",
			addr: Addr{GID: gordma.GID{0x01}, QPN: 0xff, QKey: 0xdead},
			want: "0100:0000:0000:0000:0000:0000:0000:0000%0xff#0xdead",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.addr.String()
			if got != tc.want {
				t.Fatalf("String(): want %q, got %q", tc.want, got)
			}
			parsed, err := ResolveAddr(got)
			if err != nil {
				t.Fatalf("ResolveAddr(%q): %v", got, err)
			}
			if parsed.GID != tc.addr.GID {
				t.Errorf("GID: want %v, got %v", tc.addr.GID, parsed.GID)
			}
			if parsed.QPN != tc.addr.QPN {
				t.Errorf("QPN: want %#x, got %#x", tc.addr.QPN, parsed.QPN)
			}
			// DefaultQKey/0 both normalize to "not appended" → parsed QKey 0.
			wantQKey := tc.addr.QKey
			if wantQKey == DefaultQKey {
				wantQKey = 0
			}
			if parsed.QKey != wantQKey {
				t.Errorf("QKey: want %#x, got %#x", wantQKey, parsed.QKey)
			}
		})
	}
}

func TestResolveAddrDecimal(t *testing.T) {
	a, err := ResolveAddr("0000:0000:0000:0000:0000:0000:0000:0001%4779#57005")
	if err != nil {
		t.Fatalf("ResolveAddr: %v", err)
	}
	if a.QPN != 4779 {
		t.Errorf("QPN: want 4779, got %d", a.QPN)
	}
	if a.QKey != 57005 {
		t.Errorf("QKey: want 57005, got %d", a.QKey)
	}
}

func TestResolveAddrErrors(t *testing.T) {
	bad := []string{
		"",
		"nopercent",
		"%0x1", // empty GID
		"fe80:0000:0000:0000:0000:0000:0000:0001%", // empty QPN
		"shortgid%0x1", // too few GID groups
		"0000:0000:0000:0000:0000:0000:0000:0000%zz",   // bad QPN
		"0000:0000:0000:0000:0000:0000:0000:0000%1#zz", // bad QKey
	}
	for _, s := range bad {
		if _, err := ResolveAddr(s); err == nil {
			t.Errorf("ResolveAddr(%q): want error, got nil", s)
		}
	}
}

func TestAddrNetwork(t *testing.T) {
	a := &Addr{}
	if a.Network() != "rdma-ud" {
		t.Errorf("Network(): got %q", a.Network())
	}
}
