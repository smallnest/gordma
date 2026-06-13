package rdmanet

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/smallnest/gordma"
)

// DefaultQKey is the Q_Key used for UD endpoints when none is specified. It is
// omitted from Addr.String() output.
const DefaultQKey = 0x11111111

// Addr identifies a UD (unreliable-datagram) endpoint. It carries the GID,
// queue-pair number, and Q_Key needed to address a remote UD QP. Addr mirrors
// the shape of net.Addr (Network/String) but is not registered as one.
type Addr struct {
	// GID is the 16-byte RoCE/IB global identifier of the endpoint.
	GID gordma.GID
	// QPN is the queue-pair number of the endpoint.
	QPN uint32
	// QKey is the Q_Key guarding the UD QP. Zero is treated as DefaultQKey.
	QKey uint32
}

// Network returns the address's network name, always "rdma-ud".
func (a *Addr) Network() string { return "rdma-ud" }

// String formats the address as "gid%qpn", where qpn is rendered in hex with a
// 0x prefix (for example "fe80::1%0x12ab"). When QKey is set to a non-default
// value, "#qkey" (hex) is appended. The result round-trips through ResolveAddr.
func (a *Addr) String() string {
	if a == nil {
		return "<nil>"
	}
	var b strings.Builder
	b.WriteString(a.GID.String())
	b.WriteByte('%')
	b.WriteString("0x")
	b.WriteString(strconv.FormatUint(uint64(a.QPN), 16))
	if a.QKey != 0 && a.QKey != DefaultQKey {
		b.WriteString("#0x")
		b.WriteString(strconv.FormatUint(uint64(a.QKey), 16))
	}
	return b.String()
}

// ResolveAddr parses the "gid%qpn[#qkey]" format produced by Addr.String back
// into an Addr. The qpn and optional qkey may be given in hex (0x-prefixed) or
// decimal. It is the inverse of String.
func ResolveAddr(s string) (*Addr, error) {
	pct := strings.IndexByte(s, '%')
	if pct < 0 {
		return nil, fmt.Errorf("rdmanet: invalid Addr %q: missing '%%' separator", s)
	}
	gidStr := s[:pct]
	rest := s[pct+1:]
	if gidStr == "" {
		return nil, fmt.Errorf("rdmanet: invalid Addr %q: empty GID", s)
	}

	gid, err := parseGID(gidStr)
	if err != nil {
		return nil, fmt.Errorf("rdmanet: invalid Addr %q: %w", s, err)
	}

	var qpnStr, qkeyStr string
	if hash := strings.IndexByte(rest, '#'); hash >= 0 {
		qpnStr = rest[:hash]
		qkeyStr = rest[hash+1:]
	} else {
		qpnStr = rest
	}
	if qpnStr == "" {
		return nil, fmt.Errorf("rdmanet: invalid Addr %q: empty QPN", s)
	}

	qpn, err := parseUint32(qpnStr)
	if err != nil {
		return nil, fmt.Errorf("rdmanet: invalid Addr %q: bad QPN: %w", s, err)
	}

	addr := &Addr{GID: gid, QPN: qpn}
	if qkeyStr != "" {
		qkey, err := parseUint32(qkeyStr)
		if err != nil {
			return nil, fmt.Errorf("rdmanet: invalid Addr %q: bad QKey: %w", s, err)
		}
		addr.QKey = qkey
	}
	return addr, nil
}

// parseUint32 parses a hex (0x-prefixed) or decimal unsigned 32-bit integer.
func parseUint32(s string) (uint32, error) {
	base := 10
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		base = 16
		s = s[2:]
	}
	v, err := strconv.ParseUint(s, base, 32)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}

// parseGID parses the colon-separated form produced by gordma.GID's String
// method (8 groups of 4 hex chars = 16 bytes, "xxxx:xxxx:...:xxxx") back into a
// gordma.GID.
func parseGID(s string) (gordma.GID, error) {
	var gid gordma.GID
	parts := strings.Split(s, ":")
	const groups = len(gid) / 2 // 8 groups of 2 bytes
	if len(parts) != groups {
		return gid, fmt.Errorf("expected %d hex groups, got %d", groups, len(parts))
	}
	for i, p := range parts {
		v, err := strconv.ParseUint(p, 16, 16)
		if err != nil {
			return gid, fmt.Errorf("group %d: %w", i, err)
		}
		gid[i*2] = byte(v >> 8)
		gid[i*2+1] = byte(v)
	}
	return gid, nil
}
