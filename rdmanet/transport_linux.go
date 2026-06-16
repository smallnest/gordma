//go:build linux && cgo

package rdmanet

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smallnest/gordma"
)

// ErrMessageTooLarge is returned by SendMsg when a single message exceeds the
// reassembler's configured maximum on the receive side. With fragmentation
// (issue #36) a message may span many frames, so the practical limit is the
// receiver's max-message guard rather than one bounce slot.
var ErrMessageTooLarge = errors.New("rdmanet: message exceeds maximum size")

// ErrShortBuffer is returned by RecvMsgBuf when the caller's buffer is too
// small to hold the received message. The boundary is preserved: the message
// is not truncated, and the call reports the error instead.
var ErrShortBuffer = errors.New("rdmanet: buffer too small for message")

// creditWRIDBase separates credit-return SEND completions from data-SEND
// completions in the WRID space. Data sends use WRID = slot index in
// [0, depth); credit sends use WRID in [creditWRIDBase, bufferWRIDBase);
// zero-copy buffer sends use WRID >= bufferWRIDBase.
const creditWRIDBase = 1 << 32

// bufferWRIDBase marks zero-copy Buffer send completions. The low bits carry a
// per-send token used to find the waiting SendBuffer caller.
const bufferWRIDBase = 1 << 48

// maxMessageBytes caps a single reassembled message (default 64 MiB) to bound
// receiver memory against a misbehaving peer.
const maxMessageBytes = 64 << 20

// transport owns the RC data path on top of a QP/CQ/PD: pre-registered send
// and recv bounce-buffer rings, a background completion poller, credit-based
// flow control, and message fragmentation/reassembly. One SEND carries one
// frame; a message is one or more frames (flagMore set on all but the last).
// It is safe for one concurrent sender and one concurrent receiver.
type transport struct {
	qp       *gordma.QP
	cq       *gordma.CQ
	pollMode PollMode
	slotSize int // bytes per bounce slot (header + payload)
	payload  int // usable payload per frame = slotSize - frameHeaderSize
	depth    int

	sendMR  *gordma.MR
	recvMR  *gordma.MR
	sendBuf []byte
	recvBuf []byte

	sendMu sync.Mutex
	recvMu sync.Mutex

	sendFree   chan int       // free data-send slot indices
	sendDone   chan struct{}  // one token per completed data SEND
	creditFree chan int       // free credit-send slot indices
	recvQ      chan recvFrame // completed data frames awaiting reassembly
	credits    *creditTracker // outstanding-frame flow control (peer recv depth)
	reasm      *reassembler   // inbound fragment accumulator (recv path only)
	bufDone    chan struct{}  // one token per completed zero-copy Buffer SEND

	creditSlotBase int // first send-slot index reserved for credit frames

	pollOnce sync.Once
	closed   chan struct{}
	closeOne sync.Once
	pollErr  atomic.Pointer[error]

	peerFin chan struct{} // closed when a FIN frame is received from the peer
	finOnce sync.Once

	// Batched credit return (recv path; guarded by recvMu). Returning a credit
	// to the peer per consumed frame means one reverse SEND per message, which
	// pins the sender's credit window on a round-trip. Instead, accumulate
	// consumed credits and return them in bulk once half the window is used, or
	// just before the receiver blocks for new data.
	creditPending        int
	creditFlushThreshold int

	// Lightweight send-path probes (enabled when GORDMA_PROBE is set). They
	// accumulate nanoseconds spent in the two potential stall points on the
	// send side: acquiring a flow-control credit (waiting for the peer to
	// return credits) versus waiting for local send completions. A large
	// credit share points at receiver-driven flow control as the bottleneck.
	probe          bool
	creditWaitNs   atomic.Int64
	sendDoneWaitNs atomic.Int64
}

// recvFrame is a completed inbound data frame: the recv slot it landed in and
// its decoded header/length.
type recvFrame struct {
	slot int
	hdr  frameHeader
	n    int // payload length (excludes header)
}

// newTransport registers the send/recv rings, posts the initial recv WRs, and
// prepares flow-control state. depth data slots + a few credit slots are
// reserved on the send side; depth slots on the recv side. The poller starts on
// first send/recv. pollMode selects busy-poll vs. completion-event draining.
func newTransport(pd *gordma.PD, qp *gordma.QP, cq *gordma.CQ, slotSize, depth int, pollMode PollMode) (*transport, error) {
	if slotSize <= frameHeaderSize || depth <= 0 {
		return nil, fmt.Errorf("rdmanet: invalid transport sizing slot=%d depth=%d", slotSize, depth)
	}
	const creditSlots = 4
	totalSendSlots := depth + creditSlots
	sendMR, err := pd.RegMRBuffer(slotSize*totalSendSlots, gordma.AccessLocalWrite)
	if err != nil {
		return nil, err
	}
	recvMR, err := pd.RegMRBuffer(slotSize*depth, gordma.AccessLocalWrite)
	if err != nil {
		_ = sendMR.Close()
		return nil, err
	}
	t := &transport{
		qp:             qp,
		cq:             cq,
		pollMode:       pollMode,
		slotSize:       slotSize,
		payload:        slotSize - frameHeaderSize,
		depth:          depth,
		sendMR:         sendMR,
		recvMR:         recvMR,
		sendBuf:        sendMR.Bytes(),
		recvBuf:        recvMR.Bytes(),
		sendFree:       make(chan int, depth),
		sendDone:       make(chan struct{}, depth),
		creditFree:     make(chan int, creditSlots),
		recvQ:          make(chan recvFrame, depth),
		credits:        newCreditTracker(depth),
		reasm:          newReassembler(maxMessageBytes),
		bufDone:        make(chan struct{}, depth),
		creditSlotBase: depth,
		closed:         make(chan struct{}),
		peerFin:        make(chan struct{}),
		probe:          os.Getenv("GORDMA_PROBE") != "",
	}
	t.creditFlushThreshold = depth / 2
	if t.creditFlushThreshold < 1 {
		t.creditFlushThreshold = 1
	}
	for i := 0; i < depth; i++ {
		t.sendFree <- i
		if err := t.postRecv(i); err != nil {
			_ = sendMR.Close()
			_ = recvMR.Close()
			return nil, err
		}
	}
	for i := 0; i < creditSlots; i++ {
		t.creditFree <- t.creditSlotBase + i
	}
	return t, nil
}

func (t *transport) postRecv(slot int) error {
	off := slot * t.slotSize
	return t.qp.PostRecv(gordma.RecvWR{
		WRID:   uint64(slot),
		SGList: []gordma.SGE{gordma.SGEFromMR(t.recvMR, off, t.slotSize)},
	})
}

func (t *transport) startPoller() {
	t.pollOnce.Do(func() { go t.poll() })
}

// poll drains the CQ and dispatches completions. Data-SEND completions return
// the send slot to sendFree and push one sendDone token (senders wait for those
// after posting); credit-SEND completions just free a credit slot. Inbound
// credit frames release flow-control credits and immediately repost their recv
// slot; inbound data frames are enqueued for reassembly.
//
// In PollBusy mode it spins on Poll (yielding on empty polls). In PollEvent
// mode it arms the CQ and blocks on the completion channel between drains,
// trading a little latency for far lower CPU. Both modes feed the identical
// dispatch loop (dispatch).
func (t *transport) poll() {
	wc := make([]gordma.WorkCompletion, t.depth)
	for {
		select {
		case <-t.closed:
			return
		default:
		}
		if t.pollMode == PollEvent {
			// Arm for the next notification, then drain everything currently
			// ready. Re-arm/Poll ordering (arm before drain) avoids missing a
			// completion that lands between drain and arm. If no completion
			// channel is bound (e.g. an rdma_cm-created CQ), event mode is
			// impossible — fall back to busy-poll permanently.
			if err := t.cq.ReqNotify(false); err != nil {
				if errors.Is(err, gordma.ErrNoChannel) {
					t.pollMode = PollBusy
					continue
				}
				t.fail(err)
				return
			}
			// Drain whatever is already ready before blocking.
			drained, err := t.drainOnce(wc)
			if err != nil {
				t.fail(err)
				return
			}
			if drained == 0 {
				if err := t.cq.WaitEvent(); err != nil {
					if errors.Is(err, gordma.ErrNoChannel) {
						t.pollMode = PollBusy
						continue
					}
					// WaitEvent fails when the channel is torn down on Close.
					select {
					case <-t.closed:
						return
					default:
					}
					t.fail(err)
					return
				}
			}
			continue
		}
		// PollBusy
		n, err := t.drainOnce(wc)
		if err != nil {
			t.fail(err)
			return
		}
		if n == 0 {
			runtime.Gosched()
		}
	}
}

// drainOnce polls the CQ once and dispatches every completion in the batch. It
// returns the number of completions handled, or an error to fail on.
func (t *transport) drainOnce(wc []gordma.WorkCompletion) (int, error) {
	n, err := t.cq.Poll(wc)
	if err != nil {
		return 0, err
	}
	for i := 0; i < n; i++ {
		if err := t.dispatch(&wc[i]); err != nil {
			return n, err
		}
	}
	return n, nil
}

// dispatch routes a single completion. It returns an error only for a failed
// completion or a fatal post error; channel-send blocking respects t.closed.
func (t *transport) dispatch(c *gordma.WorkCompletion) error {
	if !c.Status.OK() {
		return &gordma.CompletionError{Status: c.Status, WRID: c.WRID}
	}
	switch c.Opcode {
	case gordma.WCSend:
		if c.WRID >= bufferWRIDBase {
			select {
			case t.bufDone <- struct{}{}:
			case <-t.closed:
			}
			return nil
		}
		if c.WRID >= creditWRIDBase {
			select {
			case t.creditFree <- int(c.WRID-creditWRIDBase) + t.creditSlotBase:
			case <-t.closed:
			}
			return nil
		}
		select {
		case t.sendFree <- int(c.WRID):
		case <-t.closed:
		}
		select {
		case t.sendDone <- struct{}{}:
		case <-t.closed:
		}
	case gordma.WCRecv:
		slot := int(c.WRID)
		hdr, derr := decodeHeader(t.recvBuf[slot*t.slotSize:])
		if derr != nil {
			return derr
		}
		if hdr.isCredit() {
			t.credits.release(int(hdr.value))
			return t.postRecv(slot)
		}
		if hdr.isFin() {
			t.finOnce.Do(func() { close(t.peerFin) })
			return t.postRecv(slot)
		}
		fr := recvFrame{slot: slot, hdr: hdr, n: int(hdr.value)}
		select {
		case t.recvQ <- fr:
		case <-t.closed:
		}
	}
	return nil
}

func (t *transport) fail(err error) {
	t.pollErr.CompareAndSwap(nil, &err)
	t.close()
}

func (t *transport) close() {
	t.closeOne.Do(func() {
		close(t.closed)
		t.credits.close()
	})
}

func (t *transport) err() error {
	if p := t.pollErr.Load(); p != nil {
		return *p
	}
	return nil
}

// sendMsg fragments p into payload-sized frames and sends them in order, each
// gated by a flow-control credit so the peer's recv ring is never overrun
// (avoiding RNR). All frames are posted back-to-back (up to `depth` in flight)
// and the call then waits for every completion, so it returns only once the
// whole message has been sent. The message boundary is carried by flagMore: all
// frames but the last set it. Concurrent senders serialize on sendMu.
func (t *transport) sendMsg(p []byte) error {
	if len(p) > maxMessageBytes {
		return ErrMessageTooLarge
	}
	t.startPoller()
	t.sendMu.Lock()
	defer t.sendMu.Unlock()
	return t.postFrames(p)
}

// postFrames fragments one message and posts every frame without per-frame
// waiting, then blocks for all their completions. The caller must hold sendMu.
func (t *transport) postFrames(p []byte) error {
	frames := fragmentCount(len(p), t.payload)
	posted, done := 0, 0
	for f := 0; f < frames; f++ {
		// Drain completions opportunistically so sendDone cannot fill (and block
		// the poller) when frames > the channel capacity.
		done += t.drainSendDone()
		start := f * t.payload
		end := start + t.payload
		if end > len(p) {
			end = len(p)
		}
		if err := t.sendFrame(p[start:end], f < frames-1); err != nil {
			// Wait out the frames already posted so no stray tokens leak into a
			// later call, then surface the error.
			_ = t.waitSends(posted - done)
			return err
		}
		posted++
	}
	return t.waitSends(posted - done)
}

// sendFrame posts a single data frame without waiting for its completion:
// acquire a credit (flow control), grab a free send slot, write header+payload,
// and post a signaled SEND. The slot is returned to sendFree by the poller when
// the completion arrives, and one token is pushed to sendDone; callers
// (sendMsg/sendBatch) wait for those tokens after posting all their frames, so
// up to `depth` SENDs can be in flight at once. The WRID is the slot index so
// dispatch can free the right slot.
func (t *transport) sendFrame(payload []byte, more bool) error {
	if t.probe {
		t0 := time.Now()
		if !t.credits.acquire() {
			return t.closedErr()
		}
		t.creditWaitNs.Add(int64(time.Since(t0)))
	} else if !t.credits.acquire() {
		return t.closedErr()
	}
	var slot int
	select {
	case slot = <-t.sendFree:
	case <-t.closed:
		return t.closedErr()
	}

	off := slot * t.slotSize
	hdr := frameHeader{value: uint32(len(payload))}
	if more {
		hdr.flags |= flagMore
	}
	hdr.encode(t.sendBuf[off:])
	copy(t.sendBuf[off+frameHeaderSize:off+frameHeaderSize+len(payload)], payload)

	wr := gordma.SendWR{
		WRID:     uint64(slot),
		Opcode:   gordma.OpSend,
		SGList:   []gordma.SGE{gordma.SGEFromMR(t.sendMR, off, frameHeaderSize+len(payload))},
		Signaled: true,
	}
	if err := t.qp.PostSend(wr); err != nil {
		// No completion will arrive for a failed post, so return the slot here.
		select {
		case t.sendFree <- slot:
		case <-t.closed:
		}
		return err
	}
	return nil
}

// drainSendDone consumes any send-completion tokens already queued, without
// blocking, and returns how many it drained. Callers use it while posting to
// keep the sendDone channel from filling (which would block the poller) when a
// single call posts more frames than the channel's capacity.
func (t *transport) drainSendDone() int {
	n := 0
	for {
		select {
		case <-t.sendDone:
			n++
		default:
			return n
		}
	}
}

// waitSends blocks until n send-completion tokens have been received, or the
// transport closes (returning the closed error). n may be <= 0 (no-op).
func (t *transport) waitSends(n int) error {
	if n <= 0 {
		return nil
	}
	var t0 time.Time
	if t.probe {
		t0 = time.Now()
		defer func() { t.sendDoneWaitNs.Add(int64(time.Since(t0))) }()
	}
	for ; n > 0; n-- {
		select {
		case <-t.sendDone:
		case <-t.closed:
			return t.closedErr()
		}
	}
	return nil
}

// ProbeStats returns the accumulated send-path wait times (credit-acquire vs.
// send-completion) when probing is enabled via GORDMA_PROBE; both are zero
// otherwise. Exposed for the bench tools to report where the send side stalls.
func (t *transport) ProbeStats() (creditWait, sendDoneWait time.Duration) {
	return time.Duration(t.creditWaitNs.Load()), time.Duration(t.sendDoneWaitNs.Load())
}

// sendBuffer transmits a caller-owned, pre-registered Buffer with no copy. The
// frame header is written into a reserved credit slot and sent as the first
// SGE; the user's MR is the second SGE, so its payload is DMA'd directly from
// the caller's pinned memory. Zero-copy sends are single-frame (no
// fragmentation): a Buffer larger than one frame payload is rejected.
func (t *transport) sendBuffer(b *Buffer) error {
	if len(b.buf) > t.payload {
		return ErrMessageTooLarge
	}
	if err := b.state.beginSend(); err != nil {
		return err
	}
	defer b.state.completeSend()

	t.startPoller()
	if !t.credits.acquire() {
		return t.closedErr()
	}
	t.sendMu.Lock()
	defer t.sendMu.Unlock()

	// Borrow a credit slot to hold the frame header bytes.
	var hslot int
	select {
	case hslot = <-t.creditFree:
	case <-t.closed:
		return t.closedErr()
	}
	defer func() { t.creditFree <- hslot }()

	hoff := hslot * t.slotSize
	hdr := frameHeader{value: uint32(len(b.buf))} // single frame: flagMore clear
	hdr.encode(t.sendBuf[hoff:])

	wr := gordma.SendWR{
		WRID:   bufferWRIDBase,
		Opcode: gordma.OpSend,
		SGList: []gordma.SGE{
			gordma.SGEFromMR(t.sendMR, hoff, frameHeaderSize),
			gordma.SGEFromMR(b.mr, 0, len(b.buf)),
		},
		Signaled: true,
	}
	if err := t.qp.PostSend(wr); err != nil {
		return err
	}
	select {
	case <-t.bufDone:
		return nil
	case <-t.closed:
		return t.closedErr()
	}
}

// returnCredit sends a credit-return control frame to the peer announcing that
// n recv slots have been freed. It uses a reserved credit slot and does not
// consume flow-control credits (control traffic is small and the recv ring is
// sized with headroom). Errors are reported via fail.
func (t *transport) returnCredit(n int) {
	var slot int
	select {
	case slot = <-t.creditFree:
	case <-t.closed:
		return
	}
	off := slot * t.slotSize
	hdr := frameHeader{flags: flagCredit, value: uint32(n)}
	hdr.encode(t.sendBuf[off:])
	wr := gordma.SendWR{
		WRID:     creditWRIDBase + uint64(slot-t.creditSlotBase),
		Opcode:   gordma.OpSend,
		SGList:   []gordma.SGE{gordma.SGEFromMR(t.sendMR, off, frameHeaderSize)},
		Signaled: true,
	}
	if err := t.qp.PostSend(wr); err != nil {
		t.creditFree <- slot
		t.fail(err)
	}
}

// sendFin sends a graceful-shutdown control frame telling the peer no more
// messages will follow. Best-effort: errors are ignored since the transport is
// being torn down.
func (t *transport) sendFin() {
	var slot int
	select {
	case slot = <-t.creditFree:
	case <-t.closed:
		return
	}
	off := slot * t.slotSize
	hdr := frameHeader{flags: flagFin}
	hdr.encode(t.sendBuf[off:])
	wr := gordma.SendWR{
		WRID:     creditWRIDBase + uint64(slot-t.creditSlotBase),
		Opcode:   gordma.OpSend,
		SGList:   []gordma.SGE{gordma.SGEFromMR(t.sendMR, off, frameHeaderSize)},
		Signaled: true,
	}
	_ = t.qp.PostSend(wr)
}

// consumeFrameLocked reassembles one data frame, reposts its recv slot, and
// accumulates a flow-control credit (returned to the peer in bulk by
// releaseCreditLocked). The caller must hold recvMu.
func (t *transport) consumeFrameLocked(fr recvFrame) (msg []byte, complete bool, err error) {
	off := fr.slot*t.slotSize + frameHeaderSize
	payload := t.recvBuf[off : off+fr.n]
	m, done, aerr := t.reasm.add(payload, fr.hdr.hasMore())
	// Repost the slot and account the credit before surfacing errors so the
	// ring keeps flowing.
	if rerr := t.postRecv(fr.slot); rerr != nil {
		return nil, false, rerr
	}
	t.releaseCreditLocked(1)
	if aerr != nil {
		return nil, false, aerr
	}
	return m, done, nil
}

// releaseCreditLocked accumulates n consumed credits, returning them to the
// peer in one reverse SEND once half the window has built up. The caller must
// hold recvMu.
func (t *transport) releaseCreditLocked(n int) {
	t.creditPending += n
	if t.creditPending >= t.creditFlushThreshold {
		t.flushCreditsLocked()
	}
}

// flushCreditsLocked returns any accumulated credits to the peer in a single
// control SEND. It is a no-op when nothing is pending. The caller must hold
// recvMu.
func (t *transport) flushCreditsLocked() {
	if t.creditPending > 0 {
		n := t.creditPending
		t.creditPending = 0
		t.returnCredit(n)
	}
}

// nextMessage pulls data frames from recvQ and reassembles them into a complete
// message. Consumed credits are batched (see releaseCreditLocked) and flushed
// to the peer just before the receiver blocks for new data, so the sender's
// window is refilled in bulk rather than one credit per frame. When the peer
// has sent FIN and no buffered frames remain, it returns io.EOF.
func (t *transport) nextMessage() ([]byte, error) {
	for {
		// Drain whatever is already queued without blocking first.
		select {
		case fr := <-t.recvQ:
			msg, complete, err := t.consumeFrameLocked(fr)
			if err != nil {
				return nil, err
			}
			if complete {
				return msg, nil
			}
			continue
		default:
		}
		// About to block for new data: hand the peer its accumulated credits so
		// it is not stalled waiting on them.
		t.flushCreditsLocked()
		select {
		case fr := <-t.recvQ:
			msg, complete, err := t.consumeFrameLocked(fr)
			if err != nil {
				return nil, err
			}
			if complete {
				return msg, nil
			}
		case <-t.peerFin:
			// Peer closed gracefully. Drain any frame that raced in ahead of the
			// FIN signal; otherwise report EOF.
			select {
			case fr := <-t.recvQ:
				msg, complete, err := t.consumeFrameLocked(fr)
				if err != nil {
					return nil, err
				}
				if complete {
					return msg, nil
				}
			default:
				return nil, io.EOF
			}
		case <-t.closed:
			return nil, t.closedErr()
		}
	}
}

// recvMsg blocks until a full (possibly multi-frame) message arrives.
func (t *transport) recvMsg() ([]byte, error) {
	t.startPoller()
	t.recvMu.Lock()
	defer t.recvMu.Unlock()
	return t.nextMessage()
}

// sendBatch sends each message in msgs in order, holding sendMu for the whole
// batch so the frames are not interleaved with another sender's. Every frame of
// every message is posted back-to-back (up to `depth` in flight) and the call
// then waits for all completions, so the batch streams through the NIC pipeline
// rather than stopping after each frame. It returns on the first error.
// Semantically equivalent to calling sendMsg per message.
func (t *transport) sendBatch(msgs [][]byte) error {
	for _, m := range msgs {
		if len(m) > maxMessageBytes {
			return ErrMessageTooLarge
		}
	}
	t.startPoller()
	t.sendMu.Lock()
	defer t.sendMu.Unlock()
	posted, done := 0, 0
	for _, m := range msgs {
		frames := fragmentCount(len(m), t.payload)
		for f := 0; f < frames; f++ {
			done += t.drainSendDone()
			start := f * t.payload
			end := start + t.payload
			if end > len(m) {
				end = len(m)
			}
			if err := t.sendFrame(m[start:end], f < frames-1); err != nil {
				_ = t.waitSends(posted - done)
				return err
			}
			posted++
		}
	}
	return t.waitSends(posted - done)
}

// recvBatch returns up to max complete messages. It blocks for the first
// message, then drains any additional messages already reassembled without
// blocking, returning at least one message. max <= 0 is treated as 1.
func (t *transport) recvBatch(max int) ([][]byte, error) {
	if max <= 0 {
		max = 1
	}
	t.startPoller()
	t.recvMu.Lock()
	defer t.recvMu.Unlock()

	first, err := t.nextMessage()
	if err != nil {
		return nil, err
	}
	out := [][]byte{first}
	for len(out) < max {
		msg, ok, err := t.tryNextMessage()
		if err != nil {
			return out, err
		}
		if !ok {
			break
		}
		out = append(out, msg)
	}
	// Return any accumulated credits before handing control back to the caller,
	// so the peer's window is refilled promptly even between RecvBatch calls.
	t.flushCreditsLocked()
	return out, nil
}

// tryNextMessage attempts to reassemble one more complete message from frames
// already queued, without blocking. It returns (msg, true, nil) when a message
// completed, (nil, false, nil) when no more frames are immediately available,
// or an error. Caller must hold recvMu.
func (t *transport) tryNextMessage() ([]byte, bool, error) {
	for {
		select {
		case fr := <-t.recvQ:
			msg, complete, err := t.consumeFrameLocked(fr)
			if err != nil {
				return nil, false, err
			}
			if complete {
				return msg, true, nil
			}
			// Partial: keep draining for the rest of this message.
		default:
			return nil, false, nil
		}
	}
}

// recvMsgBuf blocks until a full message arrives and copies it into p. If p is
// too small the boundary is preserved and ErrShortBuffer is returned.
func (t *transport) recvMsgBuf(p []byte) (int, error) {
	t.startPoller()
	t.recvMu.Lock()
	defer t.recvMu.Unlock()
	msg, err := t.nextMessage()
	if err != nil {
		return 0, err
	}
	if len(msg) > len(p) {
		return 0, ErrShortBuffer
	}
	return copy(p, msg), nil
}

func (t *transport) closedErr() error {
	if err := t.err(); err != nil {
		return err
	}
	return gordma.ErrClosed
}

// shutdown gracefully signals the peer (best-effort FIN), stops the poller, and
// releases the registered rings. It is safe to call once; the owning Conn
// guards against double-shutdown.
func (t *transport) shutdown() {
	// Best-effort graceful FIN so the peer's RecvMsg/Read sees io.EOF rather
	// than a connection error. Only attempt while still open.
	select {
	case <-t.closed:
	default:
		t.sendFin()
	}
	t.close()
	if t.sendMR != nil {
		_ = t.sendMR.Close()
		t.sendMR = nil
	}
	if t.recvMR != nil {
		_ = t.recvMR.Close()
		t.recvMR = nil
	}
}
