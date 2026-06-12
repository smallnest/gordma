package gordma

import (
	"errors"
	"testing"
	"time"
)

func TestCMStubContract(t *testing.T) {
	if Supported() {
		t.Skip("real platform: rdma_cm needs hardware")
	}
	if _, err := Listen("127.0.0.1:0"); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("Listen want ErrNotSupported, got %v", err)
	}
	if _, err := Dial("127.0.0.1:18515", time.Second); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("Dial want ErrNotSupported, got %v", err)
	}
}

func TestCMConnAccessorsNilSafe(t *testing.T) {
	var c *CMConn
	if c.QP() != nil || c.CQ() != nil || c.PD() != nil {
		t.Fatal("nil CMConn accessors must return nil")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("nil CMConn Close must be nil, got %v", err)
	}
}

func TestDefaultCMTimeout(t *testing.T) {
	if DefaultCMTimeout <= 0 {
		t.Fatal("DefaultCMTimeout must be positive")
	}
}
