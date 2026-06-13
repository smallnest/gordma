//go:build linux && cgo

package rdmanet

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/smallnest/gordma"
)

// ErrDatagramTooLarge is returned by WriteTo when a datagram exceeds the UD
// payload limit (the path MTU). The message boundary is preserved: the call
// fails rather than truncating.
var ErrDatagramTooLarge = errors.New("rdmanet: datagram exceeds UD MTU")

// udDefaultMTU is used when the port's active MTU cannot be determined.
const udDefaultMTU = 1024

// PacketConn is an unreliable-datagram (UD) RDMA endpoint, analogous to
// net.UDPConn. It preserves message boundaries: each WriteTo sends one
// datagram, each ReadFrom returns one datagram. AddressHandles are cached per
// destination Addr. A PacketConn is safe for one concurrent reader and one
// concurrent writer.
type PacketConn struct {
	cfg config

	ctx *gordma.Context
	pd  *gordma.PD
	cq  *gordma.CQ
	qp  *gordma.QP

	sendMR  *gordma.MR
	recvMR  *gordma.MR
	sendBuf []byte
	recvBuf []byte
	mtu     int // max UD payload (path MTU)
	depth   int

	local *Addr

	sendMu sync.Mutex
	recvMu sync.Mutex

	ahMu sync.Mutex
	ahs  map[string]*gordma.AddressHandle

	closed    chan struct{}
	closeOnce sync.Once
}

// ListenPacket creates a UD datagram endpoint. The address is currently used
// only to satisfy the net-style API; UD is connectionless, so peers are
// addressed per-WriteTo via *Addr. Use LocalAddr to obtain this endpoint's
// Addr (GID/QPN/QKey) for out-of-band distribution to peers.
func ListenPacket(addr string, opts ...Option) (*PacketConn, error) {
	cfg := applyOptions(opts)
	pc := &PacketConn{
		cfg:    cfg,
		depth:  cfg.queueDepth,
		ahs:    make(map[string]*gordma.AddressHandle),
		closed: make(chan struct{}),
	}
	if err := pc.setup(); err != nil {
		_ = pc.Close()
		return nil, err
	}
	return pc, nil
}

func (pc *PacketConn) setup() error {
	devs, free, err := gordma.GetDeviceList()
	if err != nil {
		return err
	}
	defer free()
	if len(devs) == 0 {
		return gordma.ErrNoDevice
	}
	dev := devs[0]
	if pc.cfg.device != "" {
		dev = nil
		for _, d := range devs {
			if d.Name() == pc.cfg.device {
				dev = d
				break
			}
		}
		if dev == nil {
			return fmt.Errorf("rdmanet: device %q not found", pc.cfg.device)
		}
	}

	ctx, err := dev.Open()
	if err != nil {
		return err
	}
	pc.ctx = ctx

	port, err := ctx.QueryPort(pc.cfg.port)
	if err != nil {
		return err
	}
	pc.mtu = port.ActiveMTU
	if pc.mtu <= 0 {
		pc.mtu = udDefaultMTU
	}
	gid, err := ctx.QueryGID(pc.cfg.port, pc.cfg.gidIndex)
	if err != nil {
		return err
	}

	pd, err := ctx.AllocPD()
	if err != nil {
		return err
	}
	pc.pd = pd

	cq, err := ctx.CreateCQ(pc.depth*2+1, nil)
	if err != nil {
		return err
	}
	pc.cq = cq

	qpAttr := gordma.QPInitAttr{
		Type:   gordma.QPTypeUD,
		SendCQ: cq,
		RecvCQ: cq,
		Cap:    gordma.DefaultQPCapacity(),
	}
	qpAttr.Cap.MaxSendWR = uint32(pc.depth)
	qpAttr.Cap.MaxRecvWR = uint32(pc.depth)
	qp, err := pc.pd.CreateUDQP(qpAttr)
	if err != nil {
		return err
	}
	pc.qp = qp

	// Send slots hold payload; recv slots must also hold the 40-byte GRH that
	// UD prepends to every received datagram.
	slotSend := pc.mtu
	slotRecv := pc.mtu + gordma.GRHLength
	pc.sendMR, err = pd.RegMRBuffer(slotSend*pc.depth, gordma.AccessLocalWrite)
	if err != nil {
		return err
	}
	pc.recvMR, err = pd.RegMRBuffer(slotRecv*pc.depth, gordma.AccessLocalWrite)
	if err != nil {
		return err
	}
	pc.sendBuf = pc.sendMR.Bytes()
	pc.recvBuf = pc.recvMR.Bytes()

	localPSN := uint32(time.Now().UnixNano() & 0xffffff)
	if err := qp.ModifyUDToInit(gordma.UDConnParams{QKey: DefaultQKey, PortNum: pc.cfg.port}); err != nil {
		return err
	}
	if err := qp.ModifyUDToRTR(); err != nil {
		return err
	}
	if err := qp.ModifyUDToRTS(localPSN); err != nil {
		return err
	}

	for i := 0; i < pc.depth; i++ {
		if err := pc.postRecv(i); err != nil {
			return err
		}
	}

	pc.local = &Addr{GID: gid, QPN: qp.QPN(), QKey: DefaultQKey}
	return nil
}

func (pc *PacketConn) recvSlotSize() int { return pc.mtu + gordma.GRHLength }

func (pc *PacketConn) postRecv(slot int) error {
	off := slot * pc.recvSlotSize()
	return pc.qp.PostRecv(gordma.RecvWR{
		WRID:   uint64(slot),
		SGList: []gordma.SGE{gordma.SGEFromMR(pc.recvMR, off, pc.recvSlotSize())},
	})
}

// ah returns a cached AddressHandle for to, creating one on first use.
func (pc *PacketConn) ah(to *Addr) (*gordma.AddressHandle, error) {
	key := to.String()
	pc.ahMu.Lock()
	defer pc.ahMu.Unlock()
	if h, ok := pc.ahs[key]; ok {
		return h, nil
	}
	port, err := pc.ctx.QueryPort(pc.cfg.port)
	if err != nil {
		return nil, err
	}
	h, err := pc.pd.CreateAH(gordma.AHAttr{
		IsRoCE:    port.LinkLayer == "Ethernet",
		DestGID:   to.GID,
		SGIDIndex: pc.cfg.gidIndex,
		HopLimit:  1,
		PortNum:   pc.cfg.port,
	})
	if err != nil {
		return nil, err
	}
	pc.ahs[key] = h
	return h, nil
}

// WriteTo sends b as a single UD datagram to to. A datagram larger than the
// path MTU returns ErrDatagramTooLarge (no truncation). Concurrent writers
// serialize on sendMu.
func (pc *PacketConn) WriteTo(b []byte, to *Addr) (int, error) {
	if to == nil {
		return 0, fmt.Errorf("rdmanet: WriteTo nil address")
	}
	if len(b) > pc.mtu {
		return 0, ErrDatagramTooLarge
	}
	h, err := pc.ah(to)
	if err != nil {
		return 0, err
	}
	qkey := to.QKey
	if qkey == 0 {
		qkey = DefaultQKey
	}

	pc.sendMu.Lock()
	defer pc.sendMu.Unlock()

	const slot = 0 // single-depth send path; serialized by sendMu
	off := slot * pc.mtu
	copy(pc.sendBuf[off:off+len(b)], b)
	wr := gordma.SendWR{
		WRID:       uint64(slot),
		Opcode:     gordma.OpSend,
		SGList:     []gordma.SGE{gordma.SGEFromMR(pc.sendMR, off, len(b))},
		Signaled:   true,
		AH:         h,
		RemoteQPN:  to.QPN,
		RemoteQKey: qkey,
	}
	if err := pc.qp.PostSend(wr); err != nil {
		return 0, err
	}
	if err := pc.waitSend(); err != nil {
		return 0, err
	}
	return len(b), nil
}

// waitSend polls until the single outstanding send completes. UD send depth is
// effectively 1 (serialized by sendMu), so a small poll loop suffices.
func (pc *PacketConn) waitSend() error {
	wc := make([]gordma.WorkCompletion, pc.depth)
	for {
		select {
		case <-pc.closed:
			return gordma.ErrClosed
		default:
		}
		n, err := pc.cq.Poll(wc)
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			c := &wc[i]
			if c.Opcode == gordma.WCSend {
				if !c.Status.OK() {
					return &gordma.CompletionError{Status: c.Status, WRID: c.WRID}
				}
				return nil
			}
		}
		if n == 0 {
			runtime.Gosched()
		}
	}
}

// ReadFrom blocks for the next datagram, copies its payload into b, and reports
// the sender's QPN (the GID/QKey of the sender are not recovered from the GRH
// here; SrcQP is filled). Concurrent readers serialize on recvMu.
func (pc *PacketConn) ReadFrom(b []byte) (int, *Addr, error) {
	pc.recvMu.Lock()
	defer pc.recvMu.Unlock()

	wc := make([]gordma.WorkCompletion, pc.depth)
	for {
		select {
		case <-pc.closed:
			return 0, nil, gordma.ErrClosed
		default:
		}
		n, err := pc.cq.Poll(wc)
		if err != nil {
			return 0, nil, err
		}
		for i := 0; i < n; i++ {
			c := &wc[i]
			if c.Opcode != gordma.WCRecv {
				continue
			}
			if !c.Status.OK() {
				return 0, nil, &gordma.CompletionError{Status: c.Status, WRID: c.WRID}
			}
			slot := int(c.WRID)
			// Skip the 40-byte GRH UD prepends; payload follows.
			off := slot*pc.recvSlotSize() + gordma.GRHLength
			payloadLen := int(c.ByteLen) - gordma.GRHLength
			if payloadLen < 0 {
				payloadLen = 0
			}
			var nn int
			var rerr error
			if payloadLen > len(b) {
				rerr = ErrShortBuffer
			} else {
				nn = copy(b, pc.recvBuf[off:off+payloadLen])
			}
			from := &Addr{QPN: c.SrcQP, QKey: DefaultQKey}
			if perr := pc.postRecv(slot); perr != nil && rerr == nil {
				rerr = perr
			}
			return nn, from, rerr
		}
		if n == 0 {
			runtime.Gosched()
		}
	}
}

// WriteToBatch sends each datagram in bs to to, returning the number fully
// sent and the first error encountered. It holds sendMu for the whole batch to
// amortize lock overhead; semantically equivalent to repeated WriteTo.
func (pc *PacketConn) WriteToBatch(bs [][]byte, to *Addr) (int, error) {
	if to == nil {
		return 0, fmt.Errorf("rdmanet: WriteToBatch nil address")
	}
	for i, b := range bs {
		if _, err := pc.WriteTo(b, to); err != nil {
			return i, err
		}
	}
	return len(bs), nil
}

// ReadFromBatch reads up to max datagrams into freshly allocated buffers. It
// blocks for the first datagram, then collects any further datagrams already
// completed without blocking, returning 1..max. All datagrams in one call come
// from the same sender is NOT guaranteed; each is paired with its own from
// Addr. max <= 0 is treated as 1.
func (pc *PacketConn) ReadFromBatch(max int) ([][]byte, []*Addr, error) {
	if max <= 0 {
		max = 1
	}
	buf := make([]byte, pc.mtu)
	n, from, err := pc.ReadFrom(buf)
	if err != nil {
		return nil, nil, err
	}
	first := make([]byte, n)
	copy(first, buf[:n])
	msgs := [][]byte{first}
	addrs := []*Addr{from}
	for len(msgs) < max {
		n, from, ok, err := pc.tryReadFrom(buf)
		if err != nil {
			return msgs, addrs, err
		}
		if !ok {
			break
		}
		m := make([]byte, n)
		copy(m, buf[:n])
		msgs = append(msgs, m)
		addrs = append(addrs, from)
	}
	return msgs, addrs, nil
}

// tryReadFrom does a single non-blocking poll for one datagram. It returns
// (n, from, true, nil) when one was received, (0, nil, false, nil) when none is
// immediately available, or an error. Caller must hold recvMu (ReadFromBatch
// does not, since it relies on ReadFrom's own locking for the first datagram;
// subsequent tries take recvMu here).
func (pc *PacketConn) tryReadFrom(b []byte) (int, *Addr, bool, error) {
	pc.recvMu.Lock()
	defer pc.recvMu.Unlock()
	wc := make([]gordma.WorkCompletion, pc.depth)
	select {
	case <-pc.closed:
		return 0, nil, false, gordma.ErrClosed
	default:
	}
	n, err := pc.cq.Poll(wc)
	if err != nil {
		return 0, nil, false, err
	}
	for i := 0; i < n; i++ {
		c := &wc[i]
		if c.Opcode != gordma.WCRecv {
			continue
		}
		if !c.Status.OK() {
			return 0, nil, false, &gordma.CompletionError{Status: c.Status, WRID: c.WRID}
		}
		slot := int(c.WRID)
		off := slot*pc.recvSlotSize() + gordma.GRHLength
		payloadLen := int(c.ByteLen) - gordma.GRHLength
		if payloadLen < 0 {
			payloadLen = 0
		}
		var nn int
		var rerr error
		if payloadLen > len(b) {
			rerr = ErrShortBuffer
		} else {
			nn = copy(b, pc.recvBuf[off:off+payloadLen])
		}
		from := &Addr{QPN: c.SrcQP, QKey: DefaultQKey}
		if perr := pc.postRecv(slot); perr != nil && rerr == nil {
			rerr = perr
		}
		return nn, from, true, rerr
	}
	return 0, nil, false, nil
}

// LocalAddr returns this endpoint's UD address (GID/QPN/QKey) for out-of-band
// distribution to peers.
func (pc *PacketConn) LocalAddr() *Addr {
	if pc == nil {
		return nil
	}
	return pc.local
}

// Close releases the datagram endpoint and all cached AddressHandles. It is
// idempotent.
func (pc *PacketConn) Close() error {
	if pc == nil {
		return nil
	}
	pc.closeOnce.Do(func() {
		close(pc.closed)
		pc.ahMu.Lock()
		for _, h := range pc.ahs {
			_ = h.Close()
		}
		pc.ahs = nil
		pc.ahMu.Unlock()
		if pc.sendMR != nil {
			_ = pc.sendMR.Close()
		}
		if pc.recvMR != nil {
			_ = pc.recvMR.Close()
		}
		if pc.qp != nil {
			_ = pc.qp.Close()
		}
		if pc.cq != nil {
			_ = pc.cq.Close()
		}
		if pc.pd != nil {
			_ = pc.pd.Close()
		}
		if pc.ctx != nil {
			_ = pc.ctx.Close()
		}
	})
	return nil
}

// compile-time check that Addr behaves like net.Addr.
var _ net.Addr = (*Addr)(nil)
