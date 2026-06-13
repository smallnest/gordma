//go:build linux && cgo

package rdmanet

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

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
// [0, depth); credit sends use WRID >= creditWRIDBase.
const creditWRIDBase = 1 << 32

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

	creditSlotBase int // first send-slot index reserved for credit frames

	pollOnce sync.Once
	closed   chan struct{}
	closeOne sync.Once
	pollErr  atomic.Pointer[error]
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
// first send/recv.
func newTransport(pd *gordma.PD, qp *gordma.QP, cq *gordma.CQ, slotSize, depth int) (*transport, error) {
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
		creditSlotBase: depth,
		closed:         make(chan struct{}),
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

// poll drains the CQ and dispatches completions. Data-SEND completions release
// a data slot and signal sendDone; credit-SEND completions just free a credit
// slot. Inbound credit frames release flow-control credits and immediately
// repost their recv slot; inbound data frames are enqueued for reassembly.
func (t *transport) poll() {
	wc := make([]gordma.WorkCompletion, t.depth)
	for {
		select {
		case <-t.closed:
			return
		default:
		}
		n, err := t.cq.Poll(wc)
		if err != nil {
			t.fail(err)
			return
		}
		if n == 0 {
			runtime.Gosched()
			continue
		}
		for i := 0; i < n; i++ {
			c := &wc[i]
			if !c.Status.OK() {
				t.fail(&gordma.CompletionError{Status: c.Status, WRID: c.WRID})
				return
			}
			switch c.Opcode {
			case gordma.WCSend:
				if c.WRID >= creditWRIDBase {
					// Credit-return SEND completed: free its slot.
					select {
					case t.creditFree <- int(c.WRID-creditWRIDBase) + t.creditSlotBase:
					case <-t.closed:
						return
					}
					continue
				}
				select {
				case t.sendDone <- struct{}{}:
				case <-t.closed:
					return
				}
			case gordma.WCRecv:
				slot := int(c.WRID)
				hdr, derr := decodeHeader(t.recvBuf[slot*t.slotSize:])
				if derr != nil {
					t.fail(derr)
					return
				}
				if hdr.isCredit() {
					// Peer returned credits; repost the slot immediately.
					t.credits.release(int(hdr.value))
					if rerr := t.postRecv(slot); rerr != nil {
						t.fail(rerr)
						return
					}
					continue
				}
				fr := recvFrame{slot: slot, hdr: hdr, n: int(hdr.value)}
				select {
				case t.recvQ <- fr:
				case <-t.closed:
					return
				}
			}
		}
	}
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
// (avoiding RNR). The message boundary is carried by flagMore: all frames but
// the last set it. Concurrent senders serialize on sendMu.
func (t *transport) sendMsg(p []byte) error {
	if len(p) > maxMessageBytes {
		return ErrMessageTooLarge
	}
	t.startPoller()
	t.sendMu.Lock()
	defer t.sendMu.Unlock()

	frames := fragmentCount(len(p), t.payload)
	for f := 0; f < frames; f++ {
		start := f * t.payload
		end := start + t.payload
		if end > len(p) {
			end = len(p)
		}
		more := f < frames-1
		if err := t.sendFrame(p[start:end], more); err != nil {
			return err
		}
	}
	return nil
}

// sendFrame posts a single data frame: acquire a credit (flow control), grab a
// free send slot, write header+payload, post a signaled SEND, and wait for its
// completion.
func (t *transport) sendFrame(payload []byte, more bool) error {
	if !t.credits.acquire() {
		return t.closedErr()
	}
	var slot int
	select {
	case slot = <-t.sendFree:
	case <-t.closed:
		return t.closedErr()
	}
	defer func() { t.sendFree <- slot }()

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
		return err
	}
	select {
	case <-t.sendDone:
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

// nextMessage pulls data frames from recvQ and reassembles them into a complete
// message. Each consumed frame's recv slot is reposted and one credit is
// returned to the peer.
func (t *transport) nextMessage() ([]byte, error) {
	for {
		select {
		case fr := <-t.recvQ:
			off := fr.slot*t.slotSize + frameHeaderSize
			payload := t.recvBuf[off : off+fr.n]
			msg, complete, aerr := t.reasm.add(payload, fr.hdr.hasMore())
			// Repost the slot and return a credit before handling errors so the
			// ring keeps flowing.
			if rerr := t.postRecv(fr.slot); rerr != nil {
				return nil, rerr
			}
			t.returnCredit(1)
			if aerr != nil {
				return nil, aerr
			}
			if complete {
				return msg, nil
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

func (t *transport) shutdown() {
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
