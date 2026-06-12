//go:build !linux || !cgo

package gordma

// AccessFlag controls NIC access to a memory region.
type AccessFlag int

const (
	AccessLocalWrite AccessFlag = 1 << iota
	AccessRemoteWrite
	AccessRemoteRead
	AccessRemoteAtomic
	AccessMWBind
)

// PD is a protection domain. Inert on unsupported platforms.
type PD struct{}

// AllocPD returns ErrNotSupported.
func (c *Context) AllocPD() (*PD, error) { return nil, ErrNotSupported }

// Close is a no-op.
func (p *PD) Close() error { return nil }

// MR is a registered memory region. Inert on unsupported platforms.
type MR struct{}

// RegMRBuffer returns ErrNotSupported.
func (p *PD) RegMRBuffer(size int, flags AccessFlag) (*MR, error) { return nil, ErrNotSupported }

// Bytes returns nil.
func (m *MR) Bytes() []byte { return nil }

// LKey returns 0.
func (m *MR) LKey() uint32 { return 0 }

// RKey returns 0.
func (m *MR) RKey() uint32 { return 0 }

// Addr returns 0.
func (m *MR) Addr() uint64 { return 0 }

// Len returns 0.
func (m *MR) Len() int { return 0 }

// Close is a no-op.
func (m *MR) Close() error { return nil }
