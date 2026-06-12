//go:build !linux || !cgo

package gordma

// supported is false on non-Linux platforms or builds without cgo. All real
// verbs entry points are compiled out and replaced by stubs that return
// ErrNotSupported, so that the package builds and links everywhere (for
// example for development on macOS) while failing cleanly at runtime.
const supported = false
