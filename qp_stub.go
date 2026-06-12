//go:build !linux || !cgo

package gordma

// QP is a queue pair. Inert on unsupported platforms.
type QP struct {
	typ QPType
}

// CreateQP returns ErrNotSupported.
func (p *PD) CreateQP(attr QPInitAttr) (*QP, error) { return nil, ErrNotSupported }

// QPN returns 0.
func (q *QP) QPN() uint32 { return 0 }

// Type returns the QP type.
func (q *QP) Type() QPType { return q.typ }

// Close is a no-op.
func (q *QP) Close() error { return nil }

// ModifyToInit returns ErrNotSupported.
func (q *QP) ModifyToInit(portNum int, access AccessFlag) error { return ErrNotSupported }

// ModifyToRTR returns ErrNotSupported.
func (q *QP) ModifyToRTR(p RCConnParams) error { return ErrNotSupported }

// ModifyToRTS returns ErrNotSupported.
func (q *QP) ModifyToRTS(p RCConnParams) error { return ErrNotSupported }
