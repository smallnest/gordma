package perftest

import (
	"fmt"
	"time"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/handshake"
)

// SetupTCP builds the verbs resources for the manual (TCP handshake) path and
// brings the RC QP to RTS by exchanging endpoint info with the peer. It
// registers a single MR of cfg.Size (plus GRH room for UD) and returns the
// ready Endpoint. This path requires RDMA hardware at runtime; on the stub
// build the first verbs call returns gordma.ErrNotSupported.
func SetupTCP(cfg Config) (*Endpoint, *gordma.MR, error) {
	devs, free, err := gordma.GetDeviceList()
	if err != nil {
		return nil, nil, err
	}
	defer free()
	if len(devs) == 0 {
		return nil, nil, gordma.ErrNoDevice
	}
	dev := devs[0]
	if cfg.Device != "" {
		dev = nil
		for _, d := range devs {
			if d.Name() == cfg.Device {
				dev = d
				break
			}
		}
		if dev == nil {
			return nil, nil, fmt.Errorf("perftest: device %q not found", cfg.Device)
		}
	}

	ctx, err := dev.Open()
	if err != nil {
		return nil, nil, err
	}
	ep := &Endpoint{Ctx: ctx}

	port, err := ctx.QueryPort(cfg.IBPort)
	if err != nil {
		ep.Close()
		return nil, nil, err
	}
	gid, err := ctx.QueryGID(cfg.IBPort, cfg.GIDIndex)
	if err != nil {
		ep.Close()
		return nil, nil, err
	}

	pd, err := ctx.AllocPD()
	if err != nil {
		ep.Close()
		return nil, nil, err
	}
	ep.PD = pd

	cq, err := ctx.CreateCQ(cfg.TxDepth*2+1, nil)
	if err != nil {
		ep.Close()
		return nil, nil, err
	}
	ep.CQ = cq

	bufSize := cfg.Size
	if cfg.Transport == TransportUD {
		bufSize += gordma.GRHLength
	}
	mr, err := pd.RegMRBuffer(bufSize, gordma.AccessLocalWrite|gordma.AccessRemoteWrite|gordma.AccessRemoteRead)
	if err != nil {
		ep.Close()
		return nil, nil, err
	}

	qpAttr := gordma.QPInitAttr{
		Type:   gordma.QPTypeRC,
		SendCQ: cq,
		RecvCQ: cq,
		Cap:    gordma.DefaultQPCapacity(),
	}
	if cfg.Transport == TransportUD {
		qpAttr.Type = gordma.QPTypeUD
	}
	qpAttr.Cap.MaxSendWR = uint32(cfg.TxDepth)
	qpAttr.Cap.MaxRecvWR = uint32(cfg.TxDepth)

	var qp *gordma.QP
	if cfg.Transport == TransportUD {
		qp, err = pd.CreateUDQP(qpAttr)
	} else {
		qp, err = pd.CreateQP(qpAttr)
	}
	if err != nil {
		_ = mr.Close()
		ep.Close()
		return nil, nil, err
	}
	ep.QP = qp

	isRoCE := port.LinkLayer == "Ethernet"
	localPSN := uint32(time.Now().UnixNano() & 0xffffff)
	local := handshake.EndpointInfo{
		QPN:        qp.QPN(),
		PSN:        localPSN,
		LID:        port.LID,
		GID:        [16]byte(gid),
		GIDIndex:   cfg.GIDIndex,
		RKey:       mr.RKey(),
		RemoteAddr: mr.Addr(),
	}
	peer, err := ExchangeOverTCP(cfg, local)
	if err != nil {
		_ = mr.Close()
		ep.Close()
		return nil, nil, err
	}
	ep.Peer = &peer

	mtu := port.ActiveMTU
	if mtu <= 0 {
		mtu = 1024
	}
	if cfg.Transport == TransportUD {
		if err := bringUpUD(qp, cfg.IBPort, localPSN); err != nil {
			_ = mr.Close()
			ep.Close()
			return nil, nil, err
		}
		// Build the address handle to reach the peer on UD.
		ah, err := pd.CreateAH(gordma.AHAttr{
			IsRoCE:    isRoCE,
			DestLID:   peer.LID,
			DestGID:   gordma.GID(peer.GID),
			SGIDIndex: cfg.GIDIndex,
			HopLimit:  1,
			PortNum:   cfg.IBPort,
		})
		if err != nil {
			_ = mr.Close()
			ep.Close()
			return nil, nil, err
		}
		ep.udAH = ah
	} else {
		conn := gordma.RCConnParams{
			DestQPN:   peer.QPN,
			DestPSN:   peer.PSN,
			LocalPSN:  localPSN,
			MTU:       mtu,
			PortNum:   cfg.IBPort,
			IsRoCE:    isRoCE,
			DestLID:   peer.LID,
			DestGID:   gordma.GID(peer.GID),
			SGIDIndex: cfg.GIDIndex,
			HopLimit:  1,
		}
		if err := bringUpRC(qp, cfg.IBPort, conn); err != nil {
			_ = mr.Close()
			ep.Close()
			return nil, nil, err
		}
	}
	return ep, mr, nil
}

func bringUpRC(qp *gordma.QP, port int, conn gordma.RCConnParams) error {
	access := gordma.AccessLocalWrite | gordma.AccessRemoteWrite | gordma.AccessRemoteRead
	if err := qp.ModifyToInit(port, access); err != nil {
		return err
	}
	if err := qp.ModifyToRTR(conn); err != nil {
		return err
	}
	return qp.ModifyToRTS(conn)
}

func bringUpUD(qp *gordma.QP, port int, localPSN uint32) error {
	if err := qp.ModifyUDToInit(gordma.UDConnParams{QKey: udQKey, PortNum: port}); err != nil {
		return err
	}
	if err := qp.ModifyUDToRTR(); err != nil {
		return err
	}
	return qp.ModifyUDToRTS(localPSN)
}

// udQKey is a fixed qkey shared by both UD endpoints in the tools.
const udQKey = 0x11111111

// SetupTCPOrCM builds a ready Endpoint and a registered MR using whichever
// connection method cfg selects. For the TCP path it returns SetupTCP's result
// directly. For the rdma_cm path it establishes the connection, then registers
// an MR on the CM-provided PD. The QP is in RTS in both cases.
func SetupTCPOrCM(cfg Config) (*Endpoint, *gordma.MR, error) {
	if cfg.ConnMethod == ConnTCP {
		return SetupTCP(cfg)
	}
	ep, err := ConnectRDMACM(cfg)
	if err != nil {
		return nil, nil, err
	}
	mr, err := ep.PD.RegMRBuffer(cfg.Size, gordma.AccessLocalWrite|gordma.AccessRemoteWrite|gordma.AccessRemoteRead)
	if err != nil {
		ep.Close()
		return nil, nil, err
	}
	return ep, mr, nil
}
