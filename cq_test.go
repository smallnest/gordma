package gordma

import (
	"errors"
	"testing"
)

func TestCQStubContract(t *testing.T) {
	if Supported() {
		t.Skip("real platform: CQ needs hardware")
	}
	c := &Context{}
	if _, err := c.CreateCQ(16, nil); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("CreateCQ want ErrNotSupported, got %v", err)
	}
	if _, err := c.CreateCompChannel(); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("CreateCompChannel want ErrNotSupported, got %v", err)
	}
	q := &CQ{}
	if _, err := q.Poll(make([]WorkCompletion, 4)); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("Poll want ErrNotSupported, got %v", err)
	}
}

func TestWCStatusOK(t *testing.T) {
	if !WCSuccess.OK() {
		t.Fatal("WCSuccess.OK() must be true")
	}
	if WCLocalProtErr.OK() {
		t.Fatal("error status OK() must be false")
	}
}

func TestCompletionErrorMessage(t *testing.T) {
	e := &CompletionError{Status: WCRetryExcErr, WRID: 7}
	if e.Error() == "" {
		t.Fatal("CompletionError.Error() must be non-empty")
	}
}
