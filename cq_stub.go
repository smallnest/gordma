//go:build !linux || !cgo

package gordma

// CompChannel is a completion event channel. Inert on unsupported platforms.
type CompChannel struct{}

// CreateCompChannel returns ErrNotSupported.
func (c *Context) CreateCompChannel() (*CompChannel, error) { return nil, ErrNotSupported }

// Close is a no-op.
func (cc *CompChannel) Close() error { return nil }

// CQ is a completion queue. Inert on unsupported platforms.
type CQ struct{}

// CreateCQ returns ErrNotSupported.
func (c *Context) CreateCQ(depth int, ch *CompChannel) (*CQ, error) { return nil, ErrNotSupported }

// Close is a no-op.
func (q *CQ) Close() error { return nil }

// Poll returns ErrNotSupported.
func (q *CQ) Poll(wc []WorkCompletion) (int, error) { return 0, ErrNotSupported }

// ReqNotify returns ErrNotSupported.
func (q *CQ) ReqNotify(solicitedOnly bool) error { return ErrNotSupported }

// WaitEvent returns ErrNotSupported.
func (q *CQ) WaitEvent() error { return ErrNotSupported }
