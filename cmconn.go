package gordma

import "time"

// DefaultCMTimeout is the default timeout applied to rdma_cm address/route
// resolution and connection establishment when none is specified.
const DefaultCMTimeout = 5 * time.Second

// CMConn is a connection established via the RDMA connection manager
// (librdmacm). Its underlying QP is already in RTS, so the caller can post
// work requests immediately. It bundles the QP with the PD/CQ that rdma_cm
// created so resources can be released together on Close.
type CMConn struct {
	qp  *QP
	cq  *CQ
	pd  *PD
	id  cmID // platform handle
}

// QP returns the connection's queue pair (already in RTS).
func (c *CMConn) QP() *QP {
	if c == nil {
		return nil
	}
	return c.qp
}

// CQ returns the connection's completion queue.
func (c *CMConn) CQ() *CQ {
	if c == nil {
		return nil
	}
	return c.cq
}

// PD returns the connection's protection domain, for registering MRs.
func (c *CMConn) PD() *PD {
	if c == nil {
		return nil
	}
	return c.pd
}
