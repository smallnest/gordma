//go:build linux && cgo

package gordma

/*
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <arpa/inet.h>
#include <infiniband/verbs.h>

// post_send_one posts a single send WR. The SGE array is supplied by the
// caller as a C array to avoid Go-pointer-to-Go-pointer cgo issues. Returns 0
// on success or the errno-style rc from ibv_post_send.
static int post_send_one(struct ibv_qp *qp, uint64_t wr_id, int opcode,
                         struct ibv_sge *sg, int num_sge,
                         unsigned send_flags, uint32_t imm_data,
                         uint64_t remote_addr, uint32_t rkey,
                         struct ibv_ah *ah, uint32_t rqpn, uint32_t rqkey) {
	struct ibv_send_wr wr;
	struct ibv_send_wr *bad = NULL;
	memset(&wr, 0, sizeof(wr));
	wr.wr_id = wr_id;
	wr.next = NULL;
	wr.sg_list = sg;
	wr.num_sge = num_sge;
	wr.send_flags = send_flags;
	// imm_data is __be32 on the wire; convert from host order.
	wr.imm_data = htonl(imm_data);

	switch (opcode) {
	case 0: wr.opcode = IBV_WR_SEND; break;
	case 1: wr.opcode = IBV_WR_RDMA_WRITE; break;
	case 2: wr.opcode = IBV_WR_RDMA_WRITE_WITH_IMM; break;
	case 3: wr.opcode = IBV_WR_RDMA_READ; break;
	default: return EINVAL;
	}

	if (opcode == 1 || opcode == 2 || opcode == 3) {
		wr.wr.rdma.remote_addr = remote_addr;
		wr.wr.rdma.rkey = rkey;
	}
	if (ah != NULL) {
		wr.wr.ud.ah = ah;
		wr.wr.ud.remote_qpn = rqpn;
		wr.wr.ud.remote_qkey = rqkey;
	}
	return ibv_post_send(qp, &wr, &bad);
}

static int post_recv_one(struct ibv_qp *qp, uint64_t wr_id,
                         struct ibv_sge *sg, int num_sge) {
	struct ibv_recv_wr wr;
	struct ibv_recv_wr *bad = NULL;
	memset(&wr, 0, sizeof(wr));
	wr.wr_id = wr_id;
	wr.next = NULL;
	wr.sg_list = sg;
	wr.num_sge = num_sge;
	return ibv_post_recv(qp, &wr, &bad);
}
*/
import "C"

import (
	"fmt"
	"syscall"
	"unsafe"
)

// sendSGEScratch and recvSGEScratch are per-QP reusable C arrays for the SGE
// list, so the post hot path performs no per-call heap allocation for the
// common single/few-SGE case.
type sgeScratch struct {
	buf []C.struct_ibv_sge
}

func (s *sgeScratch) fill(sgl []SGE) (*C.struct_ibv_sge, C.int) {
	n := len(sgl)
	if n == 0 {
		return nil, 0
	}
	if cap(s.buf) < n {
		s.buf = make([]C.struct_ibv_sge, n)
	}
	s.buf = s.buf[:n]
	for i := range sgl {
		s.buf[i].addr = C.uint64_t(sgl[i].Addr)
		s.buf[i].length = C.uint32_t(sgl[i].Length)
		s.buf[i].lkey = C.uint32_t(sgl[i].LKey)
	}
	return (*C.struct_ibv_sge)(unsafe.Pointer(&s.buf[0])), C.int(n)
}

// PostSend posts a single send work request. It supports SEND, RDMA_WRITE
// (+imm) and RDMA_READ, inline and signaled/unsignaled flags, and UD
// addressing. The hot path reuses a per-QP SGE scratch buffer.
func (q *QP) PostSend(wr SendWR) error {
	if q == nil || q.qp == nil {
		return ErrClosed
	}
	sg, num := q.sendSG.fill(wr.SGList)

	var flags C.unsigned
	if wr.Signaled {
		flags |= C.IBV_SEND_SIGNALED
	}
	if wr.Inline {
		flags |= C.IBV_SEND_INLINE
	}

	var ah *C.struct_ibv_ah
	if wr.AH != nil {
		ah = wr.AH.ah
	}

	rc := C.post_send_one(q.qp, C.uint64_t(wr.WRID), C.int(wr.Opcode),
		sg, num, flags, C.uint32_t(wr.ImmData),
		C.uint64_t(wr.RemoteAddr), C.uint32_t(wr.RKey),
		ah, C.uint32_t(wr.RemoteQPN), C.uint32_t(wr.RemoteQKey))
	if rc != 0 {
		return fmt.Errorf("gordma: ibv_post_send failed: %w", syscall.Errno(rc))
	}
	return nil
}

// PostRecv posts a single receive work request.
func (q *QP) PostRecv(wr RecvWR) error {
	if q == nil || q.qp == nil {
		return ErrClosed
	}
	sg, num := q.recvSG.fill(wr.SGList)
	rc := C.post_recv_one(q.qp, C.uint64_t(wr.WRID), sg, num)
	if rc != 0 {
		return fmt.Errorf("gordma: ibv_post_recv failed: %w", syscall.Errno(rc))
	}
	return nil
}
