package gordma

import (
	"errors"
	"testing"
)

func TestUDStubContract(t *testing.T) {
	if Supported() {
		t.Skip("real platform: UD needs hardware")
	}
	p := &PD{}
	if _, err := p.CreateUDQP(QPInitAttr{Cap: DefaultQPCapacity()}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("CreateUDQP want ErrNotSupported, got %v", err)
	}
	if _, err := p.CreateAH(AHAttr{PortNum: 1}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("CreateAH want ErrNotSupported, got %v", err)
	}
	q := &QP{typ: QPTypeUD}
	if err := q.ModifyUDToInit(UDConnParams{QKey: 0x11111111, PortNum: 1}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("ModifyUDToInit want ErrNotSupported, got %v", err)
	}
	if err := q.ModifyUDToRTR(); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("ModifyUDToRTR want ErrNotSupported, got %v", err)
	}
	if err := q.ModifyUDToRTS(123); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("ModifyUDToRTS want ErrNotSupported, got %v", err)
	}
}

func TestGRHLength(t *testing.T) {
	if GRHLength != 40 {
		t.Fatalf("GRHLength must be 40, got %d", GRHLength)
	}
}
