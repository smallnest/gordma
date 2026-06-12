//go:build linux && cgo

package gordma

/*
#include <stdlib.h>
#include <string.h>
#include <infiniband/verbs.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// CreateUDQP creates a UD queue pair in the RESET state on this PD. UD QPs use
// the same QPInitAttr as RC but with Type forced to QPTypeUD.
func (p *PD) CreateUDQP(attr QPInitAttr) (*QP, error) {
	attr.Type = QPTypeUD
	return p.CreateQP(attr)
}

// ModifyUDToInit transitions a UD QP from RESET to INIT. UD INIT requires a
// qkey rather than access flags.
func (q *QP) ModifyUDToInit(p UDConnParams) error {
	if q == nil || q.qp == nil {
		return ErrClosed
	}
	var a C.struct_ibv_qp_attr
	a.qp_state = C.IBV_QPS_INIT
	a.pkey_index = 0
	a.port_num = C.uint8_t(p.PortNum)
	a.qkey = C.uint32_t(p.QKey)
	mask := C.int(C.IBV_QP_STATE | C.IBV_QP_PKEY_INDEX | C.IBV_QP_PORT | C.IBV_QP_QKEY)
	if rc := C.ibv_modify_qp(q.qp, &a, mask); rc != 0 {
		return modifyErr("UD INIT", rc)
	}
	return nil
}

// ModifyUDToRTR transitions a UD QP from INIT to RTR. UD RTR needs only the
// state change.
func (q *QP) ModifyUDToRTR() error {
	if q == nil || q.qp == nil {
		return ErrClosed
	}
	var a C.struct_ibv_qp_attr
	a.qp_state = C.IBV_QPS_RTR
	if rc := C.ibv_modify_qp(q.qp, &a, C.int(C.IBV_QP_STATE)); rc != 0 {
		return modifyErr("UD RTR", rc)
	}
	return nil
}

// ModifyUDToRTS transitions a UD QP from RTR to RTS, setting the send-queue PSN.
func (q *QP) ModifyUDToRTS(localPSN uint32) error {
	if q == nil || q.qp == nil {
		return ErrClosed
	}
	var a C.struct_ibv_qp_attr
	a.qp_state = C.IBV_QPS_RTS
	a.sq_psn = C.uint32_t(localPSN)
	mask := C.int(C.IBV_QP_STATE | C.IBV_QP_SQ_PSN)
	if rc := C.ibv_modify_qp(q.qp, &a, mask); rc != 0 {
		return modifyErr("UD RTS", rc)
	}
	return nil
}

// AddressHandle wraps an ibv_ah for addressing a remote UD endpoint.
type AddressHandle struct {
	ah *C.struct_ibv_ah
	pd *PD
}

// CreateAH builds an address handle for reaching a remote UD endpoint.
func (p *PD) CreateAH(attr AHAttr) (*AddressHandle, error) {
	if p == nil || p.pd == nil {
		return nil, ErrClosed
	}
	var a C.struct_ibv_ah_attr
	a.port_num = C.uint8_t(attr.PortNum)
	if attr.IsRoCE {
		a.is_global = 1
		a.grh.hop_limit = C.uint8_t(attr.HopLimit)
		a.grh.sgid_index = C.uint8_t(attr.SGIDIndex)
		C.memcpy(unsafe.Pointer(&a.grh.dgid), unsafe.Pointer(&attr.DestGID[0]), C.size_t(len(attr.DestGID)))
	} else {
		a.is_global = 0
		a.dlid = C.uint16_t(attr.DestLID)
	}
	ah := C.ibv_create_ah(p.pd, &a)
	if ah == nil {
		return nil, fmt.Errorf("gordma: ibv_create_ah failed: %w", lastErrno())
	}
	return &AddressHandle{ah: ah, pd: p}, nil
}

// Close destroys the address handle. Idempotent.
func (h *AddressHandle) Close() error {
	if h == nil || h.ah == nil {
		return nil
	}
	if rc := C.ibv_destroy_ah(h.ah); rc != 0 {
		return fmt.Errorf("gordma: ibv_destroy_ah failed: %w", errnoFromRC(rc))
	}
	h.ah = nil
	return nil
}
