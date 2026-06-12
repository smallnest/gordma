//go:build linux && cgo

package gordma

/*
#include <stdlib.h>
#include <string.h>
#include <infiniband/verbs.h>

// bytes_to_mtu maps a byte count to enum ibv_mtu, defaulting to 1024.
static enum ibv_mtu bytes_to_mtu(int b) {
	switch (b) {
	case 256:  return IBV_MTU_256;
	case 512:  return IBV_MTU_512;
	case 1024: return IBV_MTU_1024;
	case 2048: return IBV_MTU_2048;
	case 4096: return IBV_MTU_4096;
	default:   return IBV_MTU_1024;
	}
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// QP is a queue pair.
type QP struct {
	qp  *C.struct_ibv_qp
	pd  *PD
	typ QPType
	// sendSG/recvSG are reusable SGE scratch buffers so the post hot path
	// allocates nothing in steady state.
	sendSG sgeScratch
	recvSG sgeScratch
}

// CreateQP creates a queue pair in the RESET state on this PD.
func (p *PD) CreateQP(attr QPInitAttr) (*QP, error) {
	if p == nil || p.pd == nil {
		return nil, ErrClosed
	}
	if attr.SendCQ == nil || attr.SendCQ.cq == nil || attr.RecvCQ == nil || attr.RecvCQ.cq == nil {
		return nil, fmt.Errorf("gordma: CreateQP requires non-nil send and recv CQs")
	}
	var ia C.struct_ibv_qp_init_attr
	ia.send_cq = attr.SendCQ.cq
	ia.recv_cq = attr.RecvCQ.cq
	ia.qp_type = C.enum_ibv_qp_type(attr.Type)
	ia.cap.max_send_wr = C.uint32_t(attr.Cap.MaxSendWR)
	ia.cap.max_recv_wr = C.uint32_t(attr.Cap.MaxRecvWR)
	ia.cap.max_send_sge = C.uint32_t(attr.Cap.MaxSendSGE)
	ia.cap.max_recv_sge = C.uint32_t(attr.Cap.MaxRecvSGE)
	ia.cap.max_inline_data = C.uint32_t(attr.Cap.MaxInlineData)
	if attr.SignalAll {
		ia.sq_sig_all = 1
	}

	qp := C.ibv_create_qp(p.pd, &ia)
	if qp == nil {
		return nil, fmt.Errorf("gordma: ibv_create_qp failed: %w", lastErrno())
	}
	return &QP{qp: qp, pd: p, typ: attr.Type}, nil
}

// QPN returns the local queue-pair number.
func (q *QP) QPN() uint32 {
	if q == nil || q.qp == nil {
		return 0
	}
	return uint32(q.qp.qp_num)
}

// Type returns the QP transport type.
func (q *QP) Type() QPType { return q.typ }

// Close destroys the queue pair. Idempotent.
func (q *QP) Close() error {
	if q == nil || q.qp == nil {
		return nil
	}
	if rc := C.ibv_destroy_qp(q.qp); rc != 0 {
		return fmt.Errorf("gordma: ibv_destroy_qp failed: %w", errnoFromRC(rc))
	}
	q.qp = nil
	return nil
}

// modifyErr wraps an ibv_modify_qp failure with the target state.
func modifyErr(state string, rc C.int) error {
	return fmt.Errorf("gordma: ibv_modify_qp(->%s) failed: %w", state, errnoFromRC(rc))
}

// ModifyToInit transitions an RC QP from RESET to INIT.
func (q *QP) ModifyToInit(portNum int, access AccessFlag) error {
	if q == nil || q.qp == nil {
		return ErrClosed
	}
	var a C.struct_ibv_qp_attr
	a.qp_state = C.IBV_QPS_INIT
	a.pkey_index = 0
	a.port_num = C.uint8_t(portNum)
	a.qp_access_flags = C.uint(access)
	mask := C.int(C.IBV_QP_STATE | C.IBV_QP_PKEY_INDEX | C.IBV_QP_PORT | C.IBV_QP_ACCESS_FLAGS)
	if rc := C.ibv_modify_qp(q.qp, &a, mask); rc != 0 {
		return modifyErr("INIT", rc)
	}
	return nil
}

// ModifyToRTR transitions an RC QP from INIT to RTR (ready to receive) using
// the remote endpoint information in p.
func (q *QP) ModifyToRTR(p RCConnParams) error {
	if q == nil || q.qp == nil {
		return ErrClosed
	}
	var a C.struct_ibv_qp_attr
	a.qp_state = C.IBV_QPS_RTR
	a.path_mtu = C.bytes_to_mtu(C.int(p.MTU))
	a.dest_qp_num = C.uint32_t(p.DestQPN)
	a.rq_psn = C.uint32_t(p.DestPSN)
	a.max_dest_rd_atomic = 1
	a.min_rnr_timer = 12

	// Address vector.
	a.ah_attr.port_num = C.uint8_t(p.PortNum)
	if p.IsRoCE {
		a.ah_attr.is_global = 1
		a.ah_attr.grh.hop_limit = C.uint8_t(p.HopLimit)
		a.ah_attr.grh.sgid_index = C.uint8_t(p.SGIDIndex)
		// Copy remote GID bytes into the union.
		C.memcpy(unsafe.Pointer(&a.ah_attr.grh.dgid), unsafe.Pointer(&p.DestGID[0]), C.size_t(len(p.DestGID)))
	} else {
		a.ah_attr.is_global = 0
		a.ah_attr.dlid = C.uint16_t(p.DestLID)
	}

	mask := C.int(C.IBV_QP_STATE | C.IBV_QP_AV | C.IBV_QP_PATH_MTU |
		C.IBV_QP_DEST_QPN | C.IBV_QP_RQ_PSN |
		C.IBV_QP_MAX_DEST_RD_ATOMIC | C.IBV_QP_MIN_RNR_TIMER)
	if rc := C.ibv_modify_qp(q.qp, &a, mask); rc != 0 {
		return modifyErr("RTR", rc)
	}
	return nil
}

// ModifyToRTS transitions an RC QP from RTR to RTS (ready to send).
func (q *QP) ModifyToRTS(p RCConnParams) error {
	if q == nil || q.qp == nil {
		return ErrClosed
	}
	var a C.struct_ibv_qp_attr
	a.qp_state = C.IBV_QPS_RTS
	a.timeout = 14
	a.retry_cnt = 7
	a.rnr_retry = 7
	a.sq_psn = C.uint32_t(p.LocalPSN)
	a.max_rd_atomic = 1

	mask := C.int(C.IBV_QP_STATE | C.IBV_QP_TIMEOUT | C.IBV_QP_RETRY_CNT |
		C.IBV_QP_RNR_RETRY | C.IBV_QP_SQ_PSN | C.IBV_QP_MAX_QP_RD_ATOMIC)
	if rc := C.ibv_modify_qp(q.qp, &a, mask); rc != 0 {
		return modifyErr("RTS", rc)
	}
	return nil
}
