//go:build !linux || !cgo

package gordma

// CreateUDQP returns ErrNotSupported.
func (p *PD) CreateUDQP(attr QPInitAttr) (*QP, error) { return nil, ErrNotSupported }

// ModifyUDToInit returns ErrNotSupported.
func (q *QP) ModifyUDToInit(p UDConnParams) error { return ErrNotSupported }

// ModifyUDToRTR returns ErrNotSupported.
func (q *QP) ModifyUDToRTR() error { return ErrNotSupported }

// ModifyUDToRTS returns ErrNotSupported.
func (q *QP) ModifyUDToRTS(localPSN uint32) error { return ErrNotSupported }

// AddressHandle wraps a UD address handle. Inert on unsupported platforms.
type AddressHandle struct{}

// CreateAH returns ErrNotSupported.
func (p *PD) CreateAH(attr AHAttr) (*AddressHandle, error) { return nil, ErrNotSupported }

// Close is a no-op.
func (h *AddressHandle) Close() error { return nil }
