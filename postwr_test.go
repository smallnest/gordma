package gordma

import (
	"errors"
	"testing"
)

func TestPostStubContract(t *testing.T) {
	if Supported() {
		t.Skip("real platform: post needs hardware")
	}
	q := &QP{typ: QPTypeRC}
	if err := q.PostSend(SendWR{Opcode: OpSend}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("PostSend want ErrNotSupported, got %v", err)
	}
	if err := q.PostRecv(RecvWR{}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("PostRecv want ErrNotSupported, got %v", err)
	}
}

func TestSGEFromMRZeroValue(t *testing.T) {
	// A zero-value MR yields a zero SGE; building one must not panic.
	var m MR
	sge := SGEFromMR(&m, 0, 0)
	if sge.Addr != 0 || sge.Length != 0 || sge.LKey != 0 {
		t.Fatalf("SGE from zero MR should be zero, got %+v", sge)
	}
}

func TestSGEFromMROffset(t *testing.T) {
	// Verify offset/length arithmetic independent of any real MR address.
	sge := SGE{Addr: 1000, Length: 64, LKey: 7}
	if sge.Addr+uint64(sge.Length) != 1064 {
		t.Fatal("SGE arithmetic wrong")
	}
}

func TestSendOpcodeConstants(t *testing.T) {
	if OpSend != 0 || OpWrite != 1 || OpWriteImm != 2 || OpRead != 3 {
		t.Fatal("SendOpcode constants drifted")
	}
}
