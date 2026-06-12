package perftest

import (
	"fmt"
	"time"

	"github.com/smallnest/gordma"
	"github.com/smallnest/gordma/handshake"
)

// Endpoint is the set of RDMA resources a tool drives during a run. It is the
// product of connection setup, regardless of which method (TCP handshake or
// rdma_cm) was used.
type Endpoint struct {
	// Ctx/PD/CQ/QP are the verbs resources. With rdma_cm they come from the
	// CMConn; with TCP they are created by the tool and connected manually.
	Ctx *gordma.Context
	PD  *gordma.PD
	CQ  *gordma.CQ
	QP  *gordma.QP

	// Peer holds the remote endpoint info exchanged over TCP (nil for rdma_cm).
	Peer *handshake.EndpointInfo

	// cm, when non-nil, owns the resources and is closed on Close.
	cm *gordma.CMConn
}

// Close releases endpoint resources in the correct order.
func (e *Endpoint) Close() error {
	if e == nil {
		return nil
	}
	if e.cm != nil {
		return e.cm.Close()
	}
	if e.QP != nil {
		_ = e.QP.Close()
	}
	if e.CQ != nil {
		_ = e.CQ.Close()
	}
	if e.PD != nil {
		_ = e.PD.Close()
	}
	if e.Ctx != nil {
		_ = e.Ctx.Close()
	}
	return nil
}

// ConnectRDMACM establishes the connection via rdma_cm based on cfg, returning
// an Endpoint whose QP is already in RTS. Server mode listens; client mode
// dials cfg.ServerAddr.
func ConnectRDMACM(cfg Config) (*Endpoint, error) {
	if cfg.IsServer() {
		addr := fmt.Sprintf(":%d", cfg.TCPPort)
		ln, err := gordma.Listen(addr)
		if err != nil {
			return nil, err
		}
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return nil, err
		}
		return endpointFromCM(conn), nil
	}
	conn, err := gordma.Dial(cfg.ServerAddr, time.Duration(0))
	if err != nil {
		return nil, err
	}
	return endpointFromCM(conn), nil
}

func endpointFromCM(conn *gordma.CMConn) *Endpoint {
	return &Endpoint{
		PD:  conn.PD(),
		CQ:  conn.CQ(),
		QP:  conn.QP(),
		cm:  conn,
	}
}

// ExchangeOverTCP performs the TCP out-of-band handshake for the manual
// (non-rdma_cm) path, returning the peer's endpoint info. The tool supplies its
// local info (QPN/PSN/GID/RKey/addr) after creating its QP.
func ExchangeOverTCP(cfg Config, local handshake.EndpointInfo) (handshake.EndpointInfo, error) {
	timeout := 10 * time.Second
	if cfg.IsServer() {
		addr := fmt.Sprintf(":%d", cfg.TCPPort)
		srv, err := handshake.Listen(addr)
		if err != nil {
			return handshake.EndpointInfo{}, err
		}
		defer srv.Close()
		return srv.Accept(local, timeout)
	}
	return handshake.Dial(cfg.ServerAddr, local, timeout)
}
