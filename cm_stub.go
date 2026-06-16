//go:build !linux || !cgo

package gordma

import "time"

// cmID is the platform handle stored in CMConn. Empty on unsupported platforms.
type cmID struct{}

// Listener accepts rdma_cm connections. Inert on unsupported platforms.
type Listener struct{}

// Listen returns ErrNotSupported.
func Listen(addr string, opts ...CMOption) (*Listener, error) { return nil, ErrNotSupported }

// Accept returns ErrNotSupported.
func (l *Listener) Accept() (*CMConn, error) { return nil, ErrNotSupported }

// Close is a no-op.
func (l *Listener) Close() error { return nil }

// Dial returns ErrNotSupported.
func Dial(addr string, timeout time.Duration, opts ...CMOption) (*CMConn, error) {
	return nil, ErrNotSupported
}

// Close is a no-op. It references c.id so the shared CMConn.id field (only
// populated on the cgo build) is not reported unused on the stub build.
func (c *CMConn) Close() error {
	if c != nil {
		_ = c.id
	}
	return nil
}
