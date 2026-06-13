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
// configured per-slot buffer size. Large-message fragmentation lands in a
// later issue (#36); until then a message must fit one bounce-buffer slot.
var ErrMessageTooLarge = errors.New("rdmanet: message exceeds buffer size")

// ErrShortBuffer is returned by RecvMsgBuf when the caller's buffer is too
// small to hold the received message. The boundary is preserved: the message
// is not truncated, and the call reports the error instead.
var ErrShortBuffer = errors.New("rdmanet: buffer too small for message")

// transport owns the RC data path on top of a QP/CQ/PD: pre-registered send
// and recv bounce-buffer rings, a background completion poller, and the
// send/recv synchronization. It preserves message boundaries — one SEND
// carries one message — and is safe for one concurrent sender and one
// concurrent receiver (additional senders/receivers serialize on the mutexes).
type transport struct {
	qp       *gordma.QP
	cq       *gordma.CQ
	slotSize int
	depth    int

	sendMR  *gordma.MR
	recvMR  *gordma.MR
	sendBuf []byte // aliases sendMR's pinned memory
	recvBuf []byte // aliases recvMR's pinned memory

	sendMu   sync.Mutex
	recvMu   sync.Mutex
	sendFree chan int      // indices of free send slots
	sendDone chan struct{} // one token per completed SEND
	recvQ    chan recvItem // completed receives (slot + length)

	pollOnce sync.Once
	closed   chan struct{}
	closeOne sync.Once
	pollErr  atomic.Pointer[error]
}

type recvItem struct {
	slot int
	n    int
}

// newTransport registers the send/recv rings on pd, posts the initial recv
// WRs, and starts the completion poller. depth slots of slotSize bytes are
// reserved per direction.
func newTransport(pd *gordma.PD, qp *gordma.QP, cq *gordma.CQ, slotSize, depth int) (*transport, error) {
	if slotSize <= 0 || depth <= 0 {
		return nil, fmt.Errorf("rdmanet: invalid transport sizing slot=%d depth=%d", slotSize, depth)
	}
	sendMR, err := pd.RegMRBuffer(slotSize*depth, gordma.AccessLocalWrite)
	if err != nil {
		return nil, err
	}
	recvMR, err := pd.RegMRBuffer(slotSize*depth, gordma.AccessLocalWrite)
	if err != nil {
		_ = sendMR.Close()
		return nil, err
	}
	t := &transport{
		qp:       qp,
		cq:       cq,
		slotSize: slotSize,
		depth:    depth,
		sendMR:   sendMR,
		recvMR:   recvMR,
		sendBuf:  sendMR.Bytes(),
		recvBuf:  recvMR.Bytes(),
		sendFree: make(chan int, depth),
		sendDone: make(chan struct{}, depth),
		recvQ:    make(chan recvItem, depth),
		closed:   make(chan struct{}),
	}
	for i := 0; i < depth; i++ {
		t.sendFree <- i
		if err := t.postRecv(i); err != nil {
			_ = sendMR.Close()
			_ = recvMR.Close()
			return nil, err
		}
	}
	return t, nil
}

// postRecv (re)posts the recv WR for slot, pointing at recvBuf[slot]. The WRID
// is the slot index so the completion identifies which slot filled.
func (t *transport) postRecv(slot int) error {
	off := slot * t.slotSize
	return t.qp.PostRecv(gordma.RecvWR{
		WRID:   uint64(slot),
		SGList: []gordma.SGE{gordma.SGEFromMR(t.recvMR, off, t.slotSize)},
	})
}

// startPoller launches the single background completion poller. It runs until
// Close; on a poll error or failed completion it records the error and unblocks
// any waiting sender/receiver.
func (t *transport) startPoller() {
	t.pollOnce.Do(func() { go t.poll() })
}

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
				select {
				case t.sendDone <- struct{}{}:
				case <-t.closed:
					return
				}
			case gordma.WCRecv:
				item := recvItem{slot: int(c.WRID), n: int(c.ByteLen)}
				select {
				case t.recvQ <- item:
				case <-t.closed:
					return
				}
			}
		}
	}
}

// fail records the first poller error and closes the transport so waiters wake.
func (t *transport) fail(err error) {
	t.pollErr.CompareAndSwap(nil, &err)
	t.close()
}

func (t *transport) close() {
	t.closeOne.Do(func() { close(t.closed) })
}

// err returns the recorded poller error, if any.
func (t *transport) err() error {
	if p := t.pollErr.Load(); p != nil {
		return *p
	}
	return nil
}

// sendMsg copies p into a free send slot and posts a signaled SEND, blocking
// until the send completes. The message boundary is preserved (one SEND = one
// message). Concurrent senders serialize on sendMu.
func (t *transport) sendMsg(p []byte) error {
	if len(p) > t.slotSize {
		return ErrMessageTooLarge
	}
	t.startPoller()
	t.sendMu.Lock()
	defer t.sendMu.Unlock()

	var slot int
	select {
	case slot = <-t.sendFree:
	case <-t.closed:
		return t.closedErr()
	}
	defer func() { t.sendFree <- slot }()

	off := slot * t.slotSize
	copy(t.sendBuf[off:off+len(p)], p)
	wr := gordma.SendWR{
		WRID:     uint64(slot),
		Opcode:   gordma.OpSend,
		SGList:   []gordma.SGE{gordma.SGEFromMR(t.sendMR, off, len(p))},
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

// recvMsg blocks until a full message arrives and returns it in a freshly
// allocated slice. Concurrent receivers serialize on recvMu.
func (t *transport) recvMsg() ([]byte, error) {
	t.startPoller()
	t.recvMu.Lock()
	defer t.recvMu.Unlock()

	select {
	case item := <-t.recvQ:
		off := item.slot * t.slotSize
		out := make([]byte, item.n)
		copy(out, t.recvBuf[off:off+item.n])
		if err := t.postRecv(item.slot); err != nil {
			return out, err
		}
		return out, nil
	case <-t.closed:
		return nil, t.closedErr()
	}
}

// recvMsgBuf blocks until a full message arrives and copies it into p. If p is
// too small the boundary is preserved: the message is dropped-to-error rather
// than truncated, and ErrShortBuffer is returned (the slot is still reposted).
func (t *transport) recvMsgBuf(p []byte) (int, error) {
	t.startPoller()
	t.recvMu.Lock()
	defer t.recvMu.Unlock()

	select {
	case item := <-t.recvQ:
		off := item.slot * t.slotSize
		var (
			n   int
			err error
		)
		if item.n > len(p) {
			err = ErrShortBuffer
		} else {
			n = copy(p, t.recvBuf[off:off+item.n])
		}
		if rerr := t.postRecv(item.slot); rerr != nil && err == nil {
			err = rerr
		}
		return n, err
	case <-t.closed:
		return 0, t.closedErr()
	}
}

// closedErr returns the poller error if one was recorded, else ErrClosed.
func (t *transport) closedErr() error {
	if err := t.err(); err != nil {
		return err
	}
	return gordma.ErrClosed
}

// shutdown stops the poller and releases the registered rings.
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
