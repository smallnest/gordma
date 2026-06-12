package gordma

// SendOpcode identifies the kind of work request posted to the send queue.
type SendOpcode int

const (
	// OpSend is a two-sided SEND (consumes a recv WR at the peer).
	OpSend SendOpcode = 0
	// OpWrite is a one-sided RDMA WRITE into the peer's memory.
	OpWrite SendOpcode = 1
	// OpWriteImm is RDMA WRITE with immediate data.
	OpWriteImm SendOpcode = 2
	// OpRead is a one-sided RDMA READ from the peer's memory.
	OpRead SendOpcode = 3
)

// SGE is a scatter/gather element referencing a span of a registered MR.
type SGE struct {
	// Addr is the virtual address of the span (within a registered MR).
	Addr uint64
	// Length is the span length in bytes.
	Length uint32
	// LKey is the local key of the MR the span belongs to.
	LKey uint32
}

// SGEFromMR builds an SGE covering [off, off+length) of the given MR.
func SGEFromMR(mr *MR, off, length int) SGE {
	return SGE{
		Addr:   mr.Addr() + uint64(off),
		Length: uint32(length),
		LKey:   mr.LKey(),
	}
}

// SendWR describes a single work request for the send queue. Fields that do
// not apply to the chosen Opcode are ignored.
type SendWR struct {
	// WRID is echoed back in the completion.
	WRID uint64
	// Opcode selects SEND/WRITE/READ.
	Opcode SendOpcode
	// SGList is the local scatter/gather list.
	SGList []SGE
	// Signaled requests a completion for this WR. When the QP was not created
	// with SignalAll, only signaled WRs generate completions.
	Signaled bool
	// Inline sends the payload inline (small-message optimization). Valid for
	// SEND/WRITE when the total length fits the QP's max_inline_data.
	Inline bool
	// ImmData is the immediate value for OpWriteImm.
	ImmData uint32

	// RemoteAddr and RKey target the peer's memory for WRITE/READ.
	RemoteAddr uint64
	RKey       uint32

	// UD addressing (UD QPs only).
	AH      *AddressHandle
	RemoteQPN uint32
	RemoteQKey uint32
}

// RecvWR describes a single work request for the receive queue.
type RecvWR struct {
	// WRID is echoed back in the completion.
	WRID uint64
	// SGList is the local scatter/gather list to receive into. For UD QPs the
	// first GRHLength bytes will hold the Global Routing Header.
	SGList []SGE
}
