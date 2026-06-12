//go:build linux && cgo

package gordma

/*
#include <errno.h>
static int gordma_errno(void) { return errno; }
*/
import "C"

import "syscall"

// lastErrno returns the current C errno as a Go error. Most verbs functions
// signal failure with a NULL return and set errno.
func lastErrno() error {
	return syscall.Errno(C.gordma_errno())
}

// errnoFromRC converts a non-zero verbs return code into a Go error. Several
// ibv_* functions return the errno value directly as rc; when rc is non-zero
// we trust it, otherwise we fall back to the live errno.
func errnoFromRC(rc C.int) error {
	if rc > 0 {
		return syscall.Errno(rc)
	}
	return lastErrno()
}
