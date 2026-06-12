package gordma

import (
	"errors"
	"testing"
)

func TestQPStubContract(t *testing.T) {
	if Supported() {
		t.Skip("real platform: QP needs hardware")
	}
	p := &PD{}
	attr := QPInitAttr{Type: QPTypeRC, Cap: DefaultQPCapacity()}
	if _, err := p.CreateQP(attr); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("CreateQP want ErrNotSupported, got %v", err)
	}
	q := &QP{typ: QPTypeRC}
	if err := q.ModifyToInit(1, AccessLocalWrite); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("ModifyToInit want ErrNotSupported, got %v", err)
	}
	if err := q.ModifyToRTR(RCConnParams{}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("ModifyToRTR want ErrNotSupported, got %v", err)
	}
	if err := q.ModifyToRTS(RCConnParams{}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("ModifyToRTS want ErrNotSupported, got %v", err)
	}
}

func TestDefaultQPCapacity(t *testing.T) {
	c := DefaultQPCapacity()
	if c.MaxSendWR == 0 || c.MaxRecvWR == 0 || c.MaxSendSGE == 0 || c.MaxRecvSGE == 0 {
		t.Fatalf("default capacity has zero field: %+v", c)
	}
}

func TestQPTypeConstants(t *testing.T) {
	// RC=2, UD=4 per ibv enum — guards against accidental reordering.
	if QPTypeRC != 2 || QPTypeUD != 4 {
		t.Fatalf("QP type constants drifted: RC=%d UD=%d", QPTypeRC, QPTypeUD)
	}
}
