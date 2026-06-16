//go:build linux && cgo

package rdmanet

import (
	"fmt"
	"time"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/handshake"
)

// handshakeTimeout bounds the TCP out-of-band exchange.
const handshakeTimeout = 10 * time.Second

// rcAccess is the access-flag set for RC bounce buffers: local read/write plus
// remote write/read so the same Conn can later back one-sided operations.
const rcAccess = gordma.AccessLocalWrite | gordma.AccessRemoteWrite | gordma.AccessRemoteRead

// dialHandshake establishes an RC connection using the TCP out-of-band
// handshake: it creates the verbs resources locally, exchanges endpoint info
// with the server over TCP, then drives the QP INIT→RTR→RTS.
func dialHandshake(addr string, timeout time.Duration, cfg config) (*Conn, error) {
	c, local, err := buildHandshakeConn(cfg)
	if err != nil {
		return nil, err
	}
	peer, err := handshake.Dial(addr, local, timeout)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	if err := c.bringUpRC(local.PSN, peer); err != nil {
		_ = c.Close()
		return nil, err
	}
	c.peer = &peer
	c.remoteAddr = addr
	return c, nil
}

// acceptHandshake is the server-side counterpart of dialHandshake.
func acceptHandshake(l *Listener) (*Conn, error) {
	c, local, err := buildHandshakeConn(l.cfg)
	if err != nil {
		return nil, err
	}
	peer, err := l.hs.Accept(local, handshakeTimeout)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	if err := c.bringUpRC(local.PSN, peer); err != nil {
		_ = c.Close()
		return nil, err
	}
	c.peer = &peer
	c.localAddr = l.addr
	return c, nil
}

// buildHandshakeConn opens the configured device, allocates PD/CQ, creates an
// RC QP, registers a bounce buffer, and returns the partially-built Conn along
// with the local EndpointInfo to exchange. The QP is still in RESET.
func buildHandshakeConn(cfg config) (*Conn, handshake.EndpointInfo, error) {
	var zero handshake.EndpointInfo

	devs, free, err := gordma.GetDeviceList()
	if err != nil {
		return nil, zero, err
	}
	defer free()
	if len(devs) == 0 {
		return nil, zero, gordma.ErrNoDevice
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
			return nil, zero, fmt.Errorf("rdmanet: device %q not found", cfg.device)
		}
	}

	ctx, err := dev.Open()
	if err != nil {
		return nil, zero, err
	}
	c := &Conn{cfg: cfg, ctx: ctx}

	port, err := ctx.QueryPort(cfg.port)
	if err != nil {
		_ = c.Close()
		return nil, zero, err
	}
	gid, err := ctx.QueryGID(cfg.port, cfg.gidIndex)
	if err != nil {
		_ = c.Close()
		return nil, zero, err
	}

	pd, err := ctx.AllocPD()
	if err != nil {
		_ = c.Close()
		return nil, zero, err
	}
	c.pd = pd

	// For event-mode polling the CQ must be bound to a completion channel;
	// busy-mode uses a plain CQ. The channel is owned by the Context and is
	// released when the Context closes.
	var ch *gordma.CompChannel
	if cfg.pollMode == PollEvent {
		ch, err = ctx.CreateCompChannel()
		if err != nil {
			_ = c.Close()
			return nil, zero, err
		}
		c.compCh = ch
	}
	cq, err := ctx.CreateCQ(cfg.queueDepth*2+1, ch)
	if err != nil {
		_ = c.Close()
		return nil, zero, err
	}
	c.cq = cq

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
		_ = c.Close()
		return nil, zero, err
	}
	c.qp = qp

	mr, err := pd.RegMRBuffer(cfg.bufferSize, rcAccess)
	if err != nil {
		_ = c.Close()
		return nil, zero, err
	}
	c.mr = mr

	localPSN := uint32(time.Now().UnixNano() & 0xffffff)
	local := handshake.EndpointInfo{
		QPN:        qp.QPN(),
		PSN:        localPSN,
		LID:        port.LID,
		GID:        [16]byte(gid),
		GIDIndex:   cfg.gidIndex,
		RKey:       mr.RKey(),
		RemoteAddr: mr.Addr(),
	}
	mtu := port.ActiveMTU
	if mtu <= 0 {
		mtu = 1024
	}
	c.info = connInfoSnapshot{
		device:   dev.Name(),
		link:     port.LinkLayer,
		mtu:      mtu,
		gidIndex: cfg.gidIndex,
		localLID: port.LID,
		localQPN: qp.QPN(),
		localPSN: localPSN,
		localGID: [16]byte(gid),
		have:     true,
	}
	return c, local, nil
}

// bringUpRC drives the RC QP INIT→RTR→RTS using the exchanged peer info.
func (c *Conn) bringUpRC(localPSN uint32, peer handshake.EndpointInfo) error {
	return bringUpRCQP(c.qp, c.ctx, c.cfg, localPSN, peer)
}
