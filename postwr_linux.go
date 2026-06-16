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

// post_send_batch posts n single-SGE send WRs in one ibv_post_send call by
// chaining them via wr.next. All arrays are length n (one entry per WR);
// opcode is one of 0=SEND,1=WRITE,2=READ. WRs and SGEs are allocated in C
// memory so the chain pointers never point at Go memory (cgo-safe). Returns 0
// on success or the errno-style rc from ibv_post_send.
static int post_send_batch(struct ibv_qp *qp, int n,
                           uint64_t *wr_id, int opcode, unsigned send_flags,
                           uint64_t *addr, uint32_t *length, uint32_t *lkey,
                           uint64_t *remote_addr, uint32_t *rkey) {
	if (n <= 0) return 0;
	enum ibv_wr_opcode op;
	switch (opcode) {
	case 0: op = IBV_WR_SEND; break;
	case 1: op = IBV_WR_RDMA_WRITE; break;
	case 3: op = IBV_WR_RDMA_READ; break;
	default: return EINVAL;
	}
	struct ibv_send_wr *wrs = calloc((size_t)n, sizeof(struct ibv_send_wr));
	struct ibv_sge *sges = calloc((size_t)n, sizeof(struct ibv_sge));
	if (wrs == NULL || sges == NULL) {
		free(wrs); free(sges);
		return ENOMEM;
	}
	for (int i = 0; i < n; i++) {
		sges[i].addr = addr[i];
		sges[i].length = length[i];
		sges[i].lkey = lkey[i];
		wrs[i].wr_id = wr_id[i];
		wrs[i].sg_list = &sges[i];
		wrs[i].num_sge = 1;
		wrs[i].opcode = op;
		wrs[i].send_flags = send_flags;
		wrs[i].next = (i + 1 < n) ? &wrs[i + 1] : NULL;
		if (opcode == 1 || opcode == 3) {
			wrs[i].wr.rdma.remote_addr = remote_addr[i];
			wrs[i].wr.rdma.rkey = rkey[i];
		}
	}
	struct ibv_send_wr *bad = NULL;
	int rc = ibv_post_send(qp, &wrs[0], &bad);
	free(wrs);
	free(sges);
	return rc;
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

// batchScratch holds reusable C-scalar arrays for PostSendBatch so the batched
// post path performs no per-call heap allocation in steady state. One entry per
// WR; all single-SGE.
type batchScratch struct {
	wrID   []C.uint64_t
	addr   []C.uint64_t
	length []C.uint32_t
	lkey   []C.uint32_t
	raddr  []C.uint64_t
	rkey   []C.uint32_t
}

// ensure grows the scratch arrays to hold at least n WRs.
func (b *batchScratch) ensure(n int) {
	if cap(b.wrID) >= n {
		b.wrID = b.wrID[:n]
		b.addr = b.addr[:n]
		b.length = b.length[:n]
		b.lkey = b.lkey[:n]
		b.raddr = b.raddr[:n]
		b.rkey = b.rkey[:n]
		return
	}
	b.wrID = make([]C.uint64_t, n)
	b.addr = make([]C.uint64_t, n)
	b.length = make([]C.uint32_t, n)
	b.lkey = make([]C.uint32_t, n)
	b.raddr = make([]C.uint64_t, n)
	b.rkey = make([]C.uint32_t, n)
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

// PostSendBatch posts several send work requests in one ibv_post_send call by
// chaining them, cutting the per-WR cgo crossing for high-message-rate paths.
// It is restricted to the common fast-path shape: each WR must have exactly one
// SGE and an opcode of OpSend, OpWrite or OpRead (no immediate data, no UD
// addressing, no inline). All WRs share one send_flags derived from the first
// WR's Signaled bit. For anything outside this shape, use PostSend per WR.
//
// An empty batch is a no-op. On failure the whole batch is rejected with the
// errno from ibv_post_send.
func (q *QP) PostSendBatch(wrs []SendWR) error {
	if q == nil || q.qp == nil {
		return ErrClosed
	}
	n := len(wrs)
	if n == 0 {
		return nil
	}
	opcode := wrs[0].Opcode
	for i := range wrs {
		if len(wrs[i].SGList) != 1 {
			return fmt.Errorf("gordma: PostSendBatch requires exactly one SGE per WR (wr %d has %d)", i, len(wrs[i].SGList))
		}
		if wrs[i].Opcode != opcode {
			return fmt.Errorf("gordma: PostSendBatch requires a uniform opcode (wr %d differs)", i)
		}
		if wrs[i].Inline || wrs[i].AH != nil || wrs[i].ImmData != 0 {
			return fmt.Errorf("gordma: PostSendBatch does not support inline/UD/imm (wr %d)", i)
		}
	}
	switch opcode {
	case OpSend, OpWrite, OpRead:
	default:
		return fmt.Errorf("gordma: PostSendBatch unsupported opcode %d", opcode)
	}

	q.batch.ensure(n)
	for i := range wrs {
		q.batch.wrID[i] = C.uint64_t(wrs[i].WRID)
		q.batch.addr[i] = C.uint64_t(wrs[i].SGList[0].Addr)
		q.batch.length[i] = C.uint32_t(wrs[i].SGList[0].Length)
		q.batch.lkey[i] = C.uint32_t(wrs[i].SGList[0].LKey)
		q.batch.raddr[i] = C.uint64_t(wrs[i].RemoteAddr)
		q.batch.rkey[i] = C.uint32_t(wrs[i].RKey)
	}
	var flags C.unsigned
	if wrs[0].Signaled {
		flags |= C.IBV_SEND_SIGNALED
	}
	rc := C.post_send_batch(q.qp, C.int(n),
		&q.batch.wrID[0], C.int(opcode), flags,
		&q.batch.addr[0], &q.batch.length[0], &q.batch.lkey[0],
		&q.batch.raddr[0], &q.batch.rkey[0])
	if rc != 0 {
		return fmt.Errorf("gordma: ibv_post_send (batch) failed: %w", syscall.Errno(rc))
	}
	return nil
}
