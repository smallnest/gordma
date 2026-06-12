package gordma

import "errors"

// ErrClosed is returned when an operation is attempted on a resource that has
// already been closed (for example a Context after Close).
var ErrClosed = errors.New("gordma: resource is closed")

// ErrNoDevice is returned when an operation requires an RDMA device but none is
// present.
var ErrNoDevice = errors.New("gordma: no RDMA device found")
