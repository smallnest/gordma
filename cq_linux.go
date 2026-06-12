//go:build linux && cgo

package gordma

/*
#include <stdlib.h>
#include <arpa/inet.h>
#include <infiniband/verbs.h>

// wc_imm_data extracts the immediate data from a completion. imm_data lives in
// an anonymous union in struct ibv_wc (so cgo cannot name it) and is carried in
// network byte order, so we convert to host order with ntohl.
static uint32_t wc_imm_data(struct ibv_wc *wc) {
	return ntohl(wc->imm_data);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// CompChannel is a completion event channel used for the blocking/event-driven
// notification mode (as opposed to busy polling).
type CompChannel struct {
	ch  *C.struct_ibv_comp_channel
	ctx *Context
}

// CreateCompChannel creates a completion event channel on this context.
func (c *Context) CreateCompChannel() (*CompChannel, error) {
	if c == nil || c.ctx == nil {
		return nil, ErrClosed
	}
	ch := C.ibv_create_comp_channel(c.ctx)
	if ch == nil {
		return nil, fmt.Errorf("gordma: ibv_create_comp_channel failed: %w", lastErrno())
	}
	return &CompChannel{ch: ch, ctx: c}, nil
}

// Close destroys the completion channel. Idempotent.
func (cc *CompChannel) Close() error {
	if cc == nil || cc.ch == nil {
		return nil
	}
	if rc := C.ibv_destroy_comp_channel(cc.ch); rc != 0 {
		return fmt.Errorf("gordma: ibv_destroy_comp_channel failed: %w", errnoFromRC(rc))
	}
	cc.ch = nil
	return nil
}

// CQ is a completion queue.
type CQ struct {
	cq  *C.struct_ibv_cq
	ctx *Context
	ch  *CompChannel
	// cwc is a reusable C array backing Poll, grown on demand so the hot path
	// allocates nothing once it has reached its steady-state batch size.
	cwc []C.struct_ibv_wc
}

// CreateCQ creates a completion queue with room for at least depth entries.
// If ch is non-nil the CQ is bound to that completion channel for event mode.
func (c *Context) CreateCQ(depth int, ch *CompChannel) (*CQ, error) {
	if c == nil || c.ctx == nil {
		return nil, ErrClosed
	}
	if depth <= 0 {
		return nil, fmt.Errorf("gordma: CQ depth must be > 0, got %d", depth)
	}
	var cch *C.struct_ibv_comp_channel
	if ch != nil {
		cch = ch.ch
	}
	cq := C.ibv_create_cq(c.ctx, C.int(depth), nil, cch, 0)
	if cq == nil {
		return nil, fmt.Errorf("gordma: ibv_create_cq failed: %w", lastErrno())
	}
	return &CQ{cq: cq, ctx: c, ch: ch}, nil
}

// Close destroys the completion queue. Idempotent.
func (q *CQ) Close() error {
	if q == nil || q.cq == nil {
		return nil
	}
	if rc := C.ibv_destroy_cq(q.cq); rc != 0 {
		return fmt.Errorf("gordma: ibv_destroy_cq failed: %w", errnoFromRC(rc))
	}
	q.cq = nil
	return nil
}

// Poll polls up to len(wc) completions into the caller-provided slice and
// returns the number polled. The slice is reused in place, so the hot path
// performs no per-call heap allocation. A return of 0 means no completions are
// ready. A negative C return is reported as an error.
func (q *CQ) Poll(wc []WorkCompletion) (int, error) {
	if q == nil || q.cq == nil {
		return 0, ErrClosed
	}
	if len(wc) == 0 {
		return 0, nil
	}
	// Reuse a C array sized to the request, growing only when a larger batch
	// is requested. This keeps the steady-state poll path allocation-free.
	n := len(wc)
	if cap(q.cwc) < n {
		q.cwc = make([]C.struct_ibv_wc, n)
	}
	cwc := q.cwc[:n]
	got := C.ibv_poll_cq(q.cq, C.int(n), (*C.struct_ibv_wc)(unsafe.Pointer(&cwc[0])))
	if got < 0 {
		return 0, fmt.Errorf("gordma: ibv_poll_cq failed: %w", lastErrno())
	}
	for i := 0; i < int(got); i++ {
		c := &cwc[i]
		wc[i] = WorkCompletion{
			WRID:    uint64(c.wr_id),
			Status:  WCStatus(c.status),
			Opcode:  WCOpcode(c.opcode),
			ByteLen: uint32(c.byte_len),
			QPNum:   uint32(c.qp_num),
			SrcQP:   uint32(c.src_qp),
		}
		if (c.wc_flags & C.IBV_WC_WITH_IMM) != 0 {
			wc[i].HasImm = true
			wc[i].ImmData = uint32(C.wc_imm_data(c))
		}
	}
	return int(got), nil
}

// ReqNotify arms the CQ to signal its completion channel on the next
// completion. Use with the channel returned by GetCQEvent for event mode.
// If solicitedOnly is true, only solicited completions trigger the event.
func (q *CQ) ReqNotify(solicitedOnly bool) error {
	if q == nil || q.cq == nil {
		return ErrClosed
	}
	var s C.int
	if solicitedOnly {
		s = 1
	}
	if rc := C.ibv_req_notify_cq(q.cq, s); rc != 0 {
		return fmt.Errorf("gordma: ibv_req_notify_cq failed: %w", errnoFromRC(rc))
	}
	return nil
}

// WaitEvent blocks on the bound completion channel until a notification
// arrives, then acks it. The CQ must have been created with a CompChannel and
// armed via ReqNotify. Returns ErrNoChannel if no channel is bound.
func (q *CQ) WaitEvent() error {
	if q == nil || q.cq == nil {
		return ErrClosed
	}
	if q.ch == nil || q.ch.ch == nil {
		return ErrNoChannel
	}
	var ev *C.struct_ibv_cq
	var ctxp unsafe.Pointer
	if rc := C.ibv_get_cq_event(q.ch.ch, &ev, &ctxp); rc != 0 {
		return fmt.Errorf("gordma: ibv_get_cq_event failed: %w", lastErrno())
	}
	C.ibv_ack_cq_events(ev, 1)
	return nil
}
