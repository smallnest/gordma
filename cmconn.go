package gordma

import "time"

// DefaultCMTimeout is the default timeout applied to rdma_cm address/route
// resolution and connection establishment when none is specified.
const DefaultCMTimeout = 5 * time.Second

// DefaultCMQueueDepth is the QP send/recv queue depth used for rdma_cm
// connections when WithCMQueueDepth is not supplied.
const DefaultCMQueueDepth = 128

// cmConfig holds the resolved settings for an rdma_cm Dial/Listen.
type cmConfig struct {
	depth int
}

func defaultCMConfig() cmConfig { return cmConfig{depth: DefaultCMQueueDepth} }

// CMOption configures an rdma_cm Dial or Listen.
type CMOption func(*cmConfig)

// WithCMQueueDepth sets the QP send/recv queue depth (number of outstanding
// work requests) for rdma_cm connections. Non-positive values are ignored and
// keep the default (DefaultCMQueueDepth). The CQ is sized to hold completions
// for both queues.
func WithCMQueueDepth(n int) CMOption {
	return func(c *cmConfig) {
		if n > 0 {
			c.depth = n
		}
	}
}

// applyCMOptions returns a cmConfig built from defaults with all opts applied.
func applyCMOptions(opts []CMOption) cmConfig {
	c := defaultCMConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	return c
}

// CMConn is a connection established via the RDMA connection manager
// (librdmacm). Its underlying QP is already in RTS, so the caller can post
// work requests immediately. It bundles the QP with the PD/CQ that rdma_cm
// created so resources can be released together on Close.
type CMConn struct {
	qp *QP
	cq *CQ
	pd *PD
	id cmID // platform handle
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
