package gordma

// QPType identifies the queue-pair transport type.
type QPType int

const (
	// QPTypeRC is a reliable connected QP.
	QPTypeRC QPType = 2
	// QPTypeUC is an unreliable connected QP (not a primary target).
	QPTypeUC QPType = 3
	// QPTypeUD is an unreliable datagram QP.
	QPTypeUD QPType = 4
)

// QPState mirrors enum ibv_qp_state.
type QPState int

const (
	QPStateReset QPState = 0
	QPStateInit  QPState = 1
	QPStateRTR   QPState = 2 // ready to receive
	QPStateRTS   QPState = 3 // ready to send
	QPStateErr   QPState = 6
)

// QPCapacity describes the QP's send/recv work-queue and SGE limits.
type QPCapacity struct {
	MaxSendWR  uint32
	MaxRecvWR  uint32
	MaxSendSGE uint32
	MaxRecvSGE uint32
	// MaxInlineData is the largest payload that may be sent inline.
	MaxInlineData uint32
}

// DefaultQPCapacity returns sensible defaults for the perftest-style tools.
func DefaultQPCapacity() QPCapacity {
	return QPCapacity{
		MaxSendWR:     256,
		MaxRecvWR:     256,
		MaxSendSGE:    1,
		MaxRecvSGE:    1,
		MaxInlineData: 64,
	}
}

// QPInitAttr holds the parameters needed to create a QP.
type QPInitAttr struct {
	Type QPType
	// SendCQ and RecvCQ may be the same CQ.
	SendCQ *CQ
	RecvCQ *CQ
	Cap    QPCapacity
	// SignalAll, when true, generates a completion for every send WR even if
	// the WR is posted unsignaled.
	SignalAll bool
}

// RCConnParams carries the remote endpoint information needed to transition an
// RC QP from INIT->RTR->RTS. These values are exchanged out-of-band (see the
// TCP handshake module) before the transitions are applied.
type RCConnParams struct {
	// DestQPN is the remote QP number.
	DestQPN uint32
	// DestPSN is the remote packet sequence number (used for RTR rq_psn).
	DestPSN uint32
	// LocalPSN is our starting packet sequence number (used for RTS sq_psn).
	LocalPSN uint32
	// MTU is the path MTU in bytes (256/512/1024/2048/4096).
	MTU int
	// PortNum is the local 1-based port number.
	PortNum int
	// IsRoCE selects GID-based (RoCE) vs LID-based (InfiniBand) addressing.
	IsRoCE bool
	// DestLID is the remote LID (InfiniBand addressing).
	DestLID uint16
	// DestGID is the remote GID (RoCE addressing).
	DestGID GID
	// SGIDIndex is the local GID table index to use (RoCE).
	SGIDIndex int
	// HopLimit for the global route header (RoCE), commonly 1 or 64.
	HopLimit uint8
}
