//go:build linux && cgo

package rdmanet

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/handshake"
)

// RawConn is the cgo implementation of the low-level, high-throughput RC
// endpoint. See the doc comment in rawconn.go for the semantics and trade-offs
// versus Conn.
type RawConn struct {
	ctx  *gordma.Context
	pd   *gordma.PD
	cq   *gordma.CQ
	qp   *gordma.QP
	peer handshake.EndpointInfo

	// local mirrors the locally-generated endpoint info exchanged at bring-up,
	// and cfg/port snapshot the device/link attributes, so Info can report the
	// perftest-style header without re-querying.
	local  handshake.EndpointInfo
	device string
	link   string
	mtu    int

	// Lightweight loop probes (enabled when GORDMA_PROBE is set). The Pipeline/
	// RecvDrain drivers run on a single goroutine, so plain counters suffice;
	// they split wall time between the post path (CPU/cgo to submit WRs) and the
	// poll path (busy-spinning for completions, i.e. waiting on the wire/peer).
	probe  bool
	postNs int64
	pollNs int64

	closeOnce sync.Once
}

// RawListener accepts RawConn connections via the TCP out-of-band handshake.
type RawListener struct {
	cfg  config
	hs   *handshake.Server
	addr string
}

// DialRaw establishes a RawConn to addr ("host:port") using the default dial
// timeout. It always uses the TCP out-of-band handshake (so the peer's RKey and
// remote address are exchanged for one-sided operations) and a busy-polled CQ.
// WithDevice/WithPort/WithGIDIndex/WithQueueDepth apply; WithPollMode,
// WithBufferSize and WithHandshake are ignored.
func DialRaw(addr string, opts ...Option) (*RawConn, error) {
	return DialRawTimeout(addr, DefaultDialTimeout, opts...)
}

// DialRawTimeout is DialRaw with an explicit establishment timeout. A
// non-positive timeout falls back to DefaultDialTimeout.
func DialRawTimeout(addr string, timeout time.Duration, opts ...Option) (*RawConn, error) {
	cfg := applyOptions(opts)
	if timeout <= 0 {
		timeout = DefaultDialTimeout
	}
	rc, local, err := buildRawConn(cfg)
	if err != nil {
		return nil, err
	}
	peer, err := handshake.Dial(addr, local, timeout)
	if err != nil {
		_ = rc.Close()
		return nil, err
	}
	if err := bringUpRCQP(rc.qp, rc.ctx, cfg, local.PSN, peer); err != nil {
		_ = rc.Close()
		return nil, err
	}
	rc.peer = peer
	return rc, nil
}

// ListenRaw binds a TCP handshake listener for RawConn connections.
func ListenRaw(addr string, opts ...Option) (*RawListener, error) {
	cfg := applyOptions(opts)
	hs, err := handshake.Listen(addr)
	if err != nil {
		return nil, err
	}
	return &RawListener{cfg: cfg, hs: hs, addr: addr}, nil
}

// Accept waits for and returns the next RawConn. Its QP is already in RTS.
func (l *RawListener) Accept() (*RawConn, error) {
	if l == nil || l.hs == nil {
		return nil, gordma.ErrClosed
	}
	rc, local, err := buildRawConn(l.cfg)
	if err != nil {
		return nil, err
	}
	peer, err := l.hs.Accept(local, handshakeTimeout)
	if err != nil {
		_ = rc.Close()
		return nil, err
	}
	if err := bringUpRCQP(rc.qp, rc.ctx, l.cfg, local.PSN, peer); err != nil {
		_ = rc.Close()
		return nil, err
	}
	rc.peer = peer
	return rc, nil
}

// Addr returns the listener's bind address.
func (l *RawListener) Addr() string {
	if l == nil {
		return ""
	}
	return l.addr
}

// Close stops the listener.
func (l *RawListener) Close() error {
	if l == nil || l.hs == nil {
		return nil
	}
	return l.hs.Close()
}

// buildRawConn opens the device and creates PD, a busy-poll CQ (no completion
// channel), and an RC QP in RESET, returning the local EndpointInfo to exchange.
// Unlike buildHandshakeConn it registers no MR — the caller does that via
// RegisterMemory — and never binds a completion channel.
func buildRawConn(cfg config) (*RawConn, handshake.EndpointInfo, error) {
	var zero handshake.EndpointInfo
	ctx, pd, port, gid, devName, err := openDeviceRC(cfg)
	if err != nil {
		return nil, zero, err
	}
	mtu := port.ActiveMTU
	if mtu <= 0 {
		mtu = 1024
	}
	rc := &RawConn{
		ctx:    ctx,
		pd:     pd,
		probe:  os.Getenv("GORDMA_PROBE") != "",
		device: devName,
		link:   port.LinkLayer,
		mtu:    mtu,
	}

	cq, err := ctx.CreateCQ(cfg.queueDepth*2+1, nil) // busy-poll: no comp channel
	if err != nil {
		_ = rc.Close()
		return nil, zero, err
	}
	rc.cq = cq

	qpAttr := gordma.QPInitAttr{
		Type:   gordma.QPTypeRC,
		SendCQ: cq,
		RecvCQ: cq,
		Cap:    gordma.DefaultQPCapacity(),
	}
	qpAttr.Cap.MaxSendWR = uint32(cfg.queueDepth)
	qpAttr.Cap.MaxRecvWR = uint32(cfg.queueDepth)
	qp, err := pd.CreateQP(qpAttr)
	if err != nil {
		_ = rc.Close()
		return nil, zero, err
	}
	rc.qp = qp

	localPSN := uint32(time.Now().UnixNano() & 0xffffff)
	local := handshake.EndpointInfo{
		QPN:      qp.QPN(),
		PSN:      localPSN,
		LID:      port.LID,
		GID:      [16]byte(gid),
		GIDIndex: cfg.gidIndex,
		// RKey/RemoteAddr are filled by RegisterMemory's first MR if the caller
		// wants the peer to target it; for Send/Recv they are unused.
	}
	rc.local = local
	return rc, local, nil
}

// RegisterMemory registers a pinned buffer of size bytes with local read/write
// and remote write/read access (so it can back one-sided operations). The
// caller owns the returned MR and must Close it.
func (rc *RawConn) RegisterMemory(size int) (*gordma.MR, error) {
	if rc == nil || rc.pd == nil {
		return nil, gordma.ErrClosed
	}
	return rc.pd.RegMRBuffer(size, rcAccess)
}

// QP returns the underlying queue pair (already in RTS).
func (rc *RawConn) QP() *gordma.QP {
	if rc == nil {
		return nil
	}
	return rc.qp
}

// CQ returns the underlying completion queue.
func (rc *RawConn) CQ() *gordma.CQ {
	if rc == nil {
		return nil
	}
	return rc.cq
}

// PD returns the underlying protection domain.
func (rc *RawConn) PD() *gordma.PD {
	if rc == nil {
		return nil
	}
	return rc.pd
}

// PeerRKey returns the peer's RKey for one-sided RDMA Write/Read targets. It is
// meaningful only if the peer advertised an MR in its handshake info.
func (rc *RawConn) PeerRKey() uint32 {
	if rc == nil {
		return 0
	}
	return rc.peer.RKey
}

// PeerAddr returns the peer's registered remote address for one-sided targets.
func (rc *RawConn) PeerAddr() uint64 {
	if rc == nil {
		return 0
	}
	return rc.peer.RemoteAddr
}

// PostSend posts one send/write/read work request. The caller fills WRID,
// Opcode, SGList, Signaled, and (for one-sided ops) RemoteAddr/RKey.
func (rc *RawConn) PostSend(wr gordma.SendWR) error {
	if rc == nil || rc.qp == nil {
		return gordma.ErrClosed
	}
	return rc.qp.PostSend(wr)
}

// PostRecv posts one receive work request.
func (rc *RawConn) PostRecv(wr gordma.RecvWR) error {
	if rc == nil || rc.qp == nil {
		return gordma.ErrClosed
	}
	return rc.qp.PostRecv(wr)
}

// Poll drains up to len(wc) completions from the CQ, returning how many were
// written. It does not block.
func (rc *RawConn) Poll(wc []gordma.WorkCompletion) (int, error) {
	if rc == nil || rc.cq == nil {
		return 0, gordma.ErrClosed
	}
	return rc.cq.Poll(wc)
}

// Pipeline runs the active throughput loop: keep up to txDepth signaled work
// requests in flight (posted via post, which the caller writes as Send/Write/
// Read) and busy-poll completions until iters have completed. It returns once
// every posted WR has completed, or on the first failed completion / error.
func (rc *RawConn) Pipeline(iters, txDepth int, post func(wrID uint64) error) error {
	if rc == nil || rc.qp == nil || rc.cq == nil {
		return gordma.ErrClosed
	}
	post, poll := rc.probed(post, rc.cq.Poll)
	return pipeline(iters, txDepth, post, poll)
}

// RecvDrain runs the passive side of a Send/Recv throughput run: pre-post
// txDepth recvs (built by rebuild) and reap iters completions, re-posting on
// each. One-sided Write/Read passive sides need no recvs and should not call
// this.
func (rc *RawConn) RecvDrain(iters, txDepth int, rebuild func(wrID uint64) gordma.RecvWR) error {
	if rc == nil || rc.qp == nil || rc.cq == nil {
		return gordma.ErrClosed
	}
	post := func(wrID uint64) error { return rc.qp.PostRecv(rebuild(wrID)) }
	post, poll := rc.probed(post, rc.cq.Poll)
	return drain(iters, txDepth, post, poll)
}

// PostSendBatch submits several send WRs in one call (see gordma.QP.PostSendBatch
// for the supported shape: single SGE, uniform RC opcode, no inline/UD/imm).
func (rc *RawConn) PostSendBatch(wrs []gordma.SendWR) error {
	if rc == nil || rc.qp == nil {
		return gordma.ErrClosed
	}
	return rc.qp.PostSendBatch(wrs)
}

// PipelineBatch is the batched-submit variant of Pipeline: it keeps txDepth
// signaled WRs in flight but refills in groups via PostSendBatch, cutting the
// per-WR cgo crossing for higher message rate. build(wrID) returns the WR for a
// given slot.
func (rc *RawConn) PipelineBatch(iters, txDepth int, build func(wrID uint64) gordma.SendWR) error {
	if rc == nil || rc.qp == nil || rc.cq == nil {
		return gordma.ErrClosed
	}
	postBatch, poll := rc.probedBatch(rc.qp.PostSendBatch, rc.cq.Poll)
	return pipelineBatch(iters, txDepth, build, postBatch, poll)
}

// probed wraps a single-WR post func and the poll func with wall-clock timers
// when GORDMA_PROBE is set, accumulating time spent submitting WRs (postNs) vs.
// busy-polling for completions (pollNs). When probing is off it returns the
// originals unchanged so the hot path pays nothing.
func (rc *RawConn) probed(
	post func(wrID uint64) error,
	poll func(wc []gordma.WorkCompletion) (int, error),
) (func(uint64) error, func([]gordma.WorkCompletion) (int, error)) {
	if !rc.probe {
		return post, poll
	}
	wrappedPost := func(wrID uint64) error {
		t0 := time.Now()
		err := post(wrID)
		rc.postNs += int64(time.Since(t0))
		return err
	}
	wrappedPoll := func(wc []gordma.WorkCompletion) (int, error) {
		t0 := time.Now()
		n, err := poll(wc)
		rc.pollNs += int64(time.Since(t0))
		return n, err
	}
	return wrappedPost, wrappedPoll
}

// probedBatch is probed for the batched-submit path: it times PostSendBatch
// calls (postNs) and Poll calls (pollNs).
func (rc *RawConn) probedBatch(
	postBatch func(wrs []gordma.SendWR) error,
	poll func(wc []gordma.WorkCompletion) (int, error),
) (func([]gordma.SendWR) error, func([]gordma.WorkCompletion) (int, error)) {
	if !rc.probe {
		return postBatch, poll
	}
	wrappedPost := func(wrs []gordma.SendWR) error {
		t0 := time.Now()
		err := postBatch(wrs)
		rc.postNs += int64(time.Since(t0))
		return err
	}
	wrappedPoll := func(wc []gordma.WorkCompletion) (int, error) {
		t0 := time.Now()
		n, err := poll(wc)
		rc.pollNs += int64(time.Since(t0))
		return n, err
	}
	return wrappedPost, wrappedPoll
}

// ProbeStats returns the accumulated time the last Pipeline/PipelineBatch/
// RecvDrain run spent submitting WRs (post) vs. busy-polling completions (poll),
// when GORDMA_PROBE is set; both are zero otherwise. A high poll share means the
// loop is waiting on the wire/peer (not CPU-bound on submit); a high post share
// means the per-WR submit path (cgo crossing, WR build) is the bottleneck.
func (rc *RawConn) ProbeStats() (post, poll time.Duration) {
	if rc == nil {
		return 0, 0
	}
	return time.Duration(rc.postNs), time.Duration(rc.pollNs)
}

// Info returns the established connection's device/link attributes and the
// local/remote RC addressing, for printing a perftest-style (ib_send_bw)
// header. It is meaningful only after the connection is up.
func (rc *RawConn) Info() RawConnInfo {
	if rc == nil {
		return RawConnInfo{}
	}
	return RawConnInfo{
		Device:    rc.device,
		LinkLayer: rc.link,
		MTU:       rc.mtu,
		GIDIndex:  rc.local.GIDIndex,
		Local: RawAddr{
			LID: rc.local.LID,
			QPN: rc.local.QPN,
			PSN: rc.local.PSN,
			GID: rc.local.GID,
		},
		Remote: RawAddr{
			LID: rc.peer.LID,
			QPN: rc.peer.QPN,
			PSN: rc.peer.PSN,
			GID: rc.peer.GID,
		},
	}
}

// Close releases the QP/CQ/PD/Context. It is idempotent.
func (rc *RawConn) Close() error {
	if rc == nil {
		return nil
	}
	rc.closeOnce.Do(func() {
		if rc.qp != nil {
			_ = rc.qp.Close()
			rc.qp = nil
		}
		if rc.cq != nil {
			_ = rc.cq.Close()
			rc.cq = nil
		}
		if rc.pd != nil {
			_ = rc.pd.Close()
			rc.pd = nil
		}
		if rc.ctx != nil {
			_ = rc.ctx.Close()
			rc.ctx = nil
		}
	})
	return nil
}

// openDeviceRC selects the configured device, opens it, queries the port/GID
// used for RC bring-up, and allocates a PD. It also returns the resolved device
// name (for perftest-style reporting). Shared by Conn and RawConn setup.
func openDeviceRC(cfg config) (*gordma.Context, *gordma.PD, gordma.PortAttr, gordma.GID, string, error) {
	var zeroPort gordma.PortAttr
	var zeroGID gordma.GID
	devs, free, err := gordma.GetDeviceList()
	if err != nil {
		return nil, nil, zeroPort, zeroGID, "", err
	}
	defer free()
	if len(devs) == 0 {
		return nil, nil, zeroPort, zeroGID, "", gordma.ErrNoDevice
	}
	dev := devs[0]
	if cfg.device != "" {
		dev = nil
		for _, d := range devs {
			if d.Name() == cfg.device {
				dev = d
				break
			}
		}
		if dev == nil {
			return nil, nil, zeroPort, zeroGID, "", fmt.Errorf("rdmanet: device %q not found", cfg.device)
		}
	}
	devName := dev.Name()
	ctx, err := dev.Open()
	if err != nil {
		return nil, nil, zeroPort, zeroGID, "", err
	}
	port, err := ctx.QueryPort(cfg.port)
	if err != nil {
		_ = ctx.Close()
		return nil, nil, zeroPort, zeroGID, "", err
	}
	gid, err := ctx.QueryGID(cfg.port, cfg.gidIndex)
	if err != nil {
		_ = ctx.Close()
		return nil, nil, zeroPort, zeroGID, "", err
	}
	pd, err := ctx.AllocPD()
	if err != nil {
		_ = ctx.Close()
		return nil, nil, zeroPort, zeroGID, "", err
	}
	return ctx, pd, port, gid, devName, nil
}

// bringUpRCQP drives an RC QP INIT→RTR→RTS using exchanged peer info. Shared by
// Conn.bringUpRC and RawConn establishment.
func bringUpRCQP(qp *gordma.QP, ctx *gordma.Context, cfg config, localPSN uint32, peer handshake.EndpointInfo) error {
	port, err := ctx.QueryPort(cfg.port)
	if err != nil {
		return err
	}
	mtu := port.ActiveMTU
	if mtu <= 0 {
		mtu = 1024
	}
	params := gordma.RCConnParams{
		DestQPN:   peer.QPN,
		DestPSN:   peer.PSN,
		LocalPSN:  localPSN,
		MTU:       mtu,
		PortNum:   cfg.port,
		IsRoCE:    port.LinkLayer == "Ethernet",
		DestLID:   peer.LID,
		DestGID:   gordma.GID(peer.GID),
		SGIDIndex: cfg.gidIndex,
		HopLimit:  1,
	}
	if err := qp.ModifyToInit(cfg.port, rcAccess); err != nil {
		return err
	}
	if err := qp.ModifyToRTR(params); err != nil {
		return err
	}
	return qp.ModifyToRTS(params)
}
