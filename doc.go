// Package gordma provides an idiomatic Go wrapper around the RDMA verbs
// API (libibverbs) and the RDMA connection manager (librdmacm).
//
// It targets Linux with libibverbs-dev and librdmacm-dev installed and is
// built with cgo. On non-Linux platforms (or when built without cgo) a stub
// implementation is compiled instead so that the package always builds; the
// stub returns ErrNotSupported at runtime rather than crashing.
//
// The library mirrors the object model of rdma-core: Device, Context, PD, MR,
// CQ, QP, AH and CompChannel. See the cmd/ tools for perftest-style bandwidth
// and latency examples.
package gordma

import "errors"

// ErrNotSupported is returned by the stub implementation on platforms where
// RDMA verbs are unavailable (for example macOS, or a build without cgo).
var ErrNotSupported = errors.New("gordma: RDMA is not supported on this platform")

// Supported reports whether this build links against libibverbs/librdmacm and
// can perform real RDMA operations. It is true only on Linux builds with cgo
// enabled.
func Supported() bool {
	return supported
}
