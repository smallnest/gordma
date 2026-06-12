//go:build !linux || !cgo

package gordma

// PostSend returns ErrNotSupported.
func (q *QP) PostSend(wr SendWR) error { return ErrNotSupported }

// PostRecv returns ErrNotSupported.
func (q *QP) PostRecv(wr RecvWR) error { return ErrNotSupported }
