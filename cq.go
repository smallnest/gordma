package gordma

import "fmt"

// WCStatus mirrors enum ibv_wc_status — the completion status of a work request.
type WCStatus int

const (
	WCSuccess        WCStatus = 0
	WCLocalLengthErr WCStatus = 1
	WCLocalQPOpErr   WCStatus = 2
	WCLocalProtErr   WCStatus = 4
	WCWRFlushErr     WCStatus = 5
	WCRetryExcErr    WCStatus = 12
	WCRNRRetryExcErr WCStatus = 13
)

// OK reports whether the completion succeeded.
func (s WCStatus) OK() bool { return s == WCSuccess }

// WCOpcode mirrors enum ibv_wc_opcode — the operation a completion refers to.
type WCOpcode int

const (
	WCSend     WCOpcode = 0
	WCRDMAWrite WCOpcode = 1
	WCRDMARead WCOpcode = 2
	WCRecv     WCOpcode = 128
	WCRecvRDMAWithImm WCOpcode = 129
)

// WorkCompletion is one polled completion. It is a flat value type so that a
// caller-supplied []WorkCompletion can be reused across Poll calls with no
// per-call heap allocation.
type WorkCompletion struct {
	// WRID is the user work-request id supplied at post time.
	WRID uint64
	// Status is the completion status; check OK().
	Status WCStatus
	// Opcode is the operation that completed.
	Opcode WCOpcode
	// ByteLen is the number of bytes transferred (relevant for recv).
	ByteLen uint32
	// ImmData is the immediate data, valid when HasImm is true.
	ImmData uint32
	// HasImm reports whether ImmData is present.
	HasImm bool
	// QPNum is the local QP number the completion belongs to.
	QPNum uint32
	// SrcQP is the source QP (UD receives).
	SrcQP uint32
}

// CompletionError describes a non-success completion status.
type CompletionError struct {
	Status WCStatus
	WRID   uint64
}

func (e *CompletionError) Error() string {
	return fmt.Sprintf("gordma: work completion %d failed with status %d", e.WRID, int(e.Status))
}
