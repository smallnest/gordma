//go:build linux && cgo

package gordma

/*
#include <stdlib.h>
#include <infiniband/verbs.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// AccessFlag controls the operations the NIC may perform on a memory region.
// Values mirror enum ibv_access_flags.
type AccessFlag int

const (
	AccessLocalWrite   AccessFlag = C.IBV_ACCESS_LOCAL_WRITE
	AccessRemoteWrite  AccessFlag = C.IBV_ACCESS_REMOTE_WRITE
	AccessRemoteRead   AccessFlag = C.IBV_ACCESS_REMOTE_READ
	AccessRemoteAtomic AccessFlag = C.IBV_ACCESS_REMOTE_ATOMIC
	AccessMWBind       AccessFlag = C.IBV_ACCESS_MW_BIND
)

// PD is a protection domain. All MRs and QPs are associated with a PD.
type PD struct {
	pd  *C.struct_ibv_pd
	ctx *Context
}

// AllocPD allocates a protection domain on this context.
func (c *Context) AllocPD() (*PD, error) {
	if c == nil || c.ctx == nil {
		return nil, ErrClosed
	}
	pd := C.ibv_alloc_pd(c.ctx)
	if pd == nil {
		return nil, fmt.Errorf("gordma: ibv_alloc_pd failed: %w", lastErrno())
	}
	return &PD{pd: pd, ctx: c}, nil
}

// Close deallocates the protection domain. It is idempotent.
func (p *PD) Close() error {
	if p == nil || p.pd == nil {
		return nil
	}
	if rc := C.ibv_dealloc_pd(p.pd); rc != 0 {
		return fmt.Errorf("gordma: ibv_dealloc_pd failed: %w", errnoFromRC(rc))
	}
	p.pd = nil
	return nil
}

// MR is a registered memory region. The backing buffer is allocated in C
// memory (via C.malloc) so that it is pinned and never moved or collected by
// the Go garbage collector for the lifetime of the MR. Use Bytes() to access
// it as a Go slice; do not retain that slice after Close.
type MR struct {
	mr   *C.struct_ibv_mr
	pd   *PD
	cbuf unsafe.Pointer
	size int
}

// RegMRBuffer allocates a pinned buffer of the given size in C memory and
// registers it as a memory region with the given access flags. Because the
// buffer lives outside the Go heap, the GC will never move or reclaim it while
// the MR is open, satisfying the requirement that registered memory stays put.
func (p *PD) RegMRBuffer(size int, flags AccessFlag) (*MR, error) {
	if p == nil || p.pd == nil {
		return nil, ErrClosed
	}
	if size <= 0 {
		return nil, fmt.Errorf("gordma: RegMRBuffer size must be > 0, got %d", size)
	}
	buf := C.malloc(C.size_t(size))
	if buf == nil {
		return nil, fmt.Errorf("gordma: C.malloc(%d) failed", size)
	}
	mr := C.ibv_reg_mr(p.pd, buf, C.size_t(size), C.int(flags))
	if mr == nil {
		err := lastErrno()
		C.free(buf)
		return nil, fmt.Errorf("gordma: ibv_reg_mr failed: %w", err)
	}
	return &MR{mr: mr, pd: p, cbuf: buf, size: size}, nil
}

// Bytes returns the registered buffer as a Go slice aliasing the pinned C
// memory. The slice is valid only until Close; do not retain it afterward.
func (m *MR) Bytes() []byte {
	if m == nil || m.cbuf == nil {
		return nil
	}
	return unsafe.Slice((*byte)(m.cbuf), m.size)
}

// LKey returns the local key used to reference this MR in local SGEs.
func (m *MR) LKey() uint32 {
	if m == nil || m.mr == nil {
		return 0
	}
	return uint32(m.mr.lkey)
}

// RKey returns the remote key shared with peers for RDMA Read/Write.
func (m *MR) RKey() uint32 {
	if m == nil || m.mr == nil {
		return 0
	}
	return uint32(m.mr.rkey)
}

// Addr returns the virtual address of the registered buffer, as needed for the
// out-of-band exchange in RDMA Read/Write.
func (m *MR) Addr() uint64 {
	if m == nil || m.cbuf == nil {
		return 0
	}
	return uint64(uintptr(m.cbuf))
}

// Len returns the size of the registered buffer in bytes.
func (m *MR) Len() int {
	if m == nil {
		return 0
	}
	return m.size
}

// Close deregisters the MR and frees the pinned buffer. It is idempotent.
func (m *MR) Close() error {
	if m == nil || m.mr == nil {
		return nil
	}
	if rc := C.ibv_dereg_mr(m.mr); rc != 0 {
		return fmt.Errorf("gordma: ibv_dereg_mr failed: %w", errnoFromRC(rc))
	}
	m.mr = nil
	if m.cbuf != nil {
		C.free(m.cbuf)
		m.cbuf = nil
	}
	return nil
}
