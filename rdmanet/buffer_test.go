package rdmanet

import (
	"errors"
	"sync"
	"testing"
)

// The Buffer lifecycle state machine is build-agnostic, so its transitions and
// misuse handling are tested here without any RDMA hardware.

func TestBufferStateHappyPath(t *testing.T) {
	var s bufferState // zero value == idle
	if err := s.beginSend(); err != nil {
		t.Fatalf("beginSend idle: %v", err)
	}
	// A second send while in-flight is rejected.
	if err := s.beginSend(); !errors.Is(err, ErrBufferInFlight) {
		t.Errorf("beginSend in-flight: want ErrBufferInFlight, got %v", err)
	}
	s.completeSend()
	// After completion the buffer is reusable.
	if err := s.beginSend(); err != nil {
		t.Errorf("beginSend after completion: %v", err)
	}
	s.completeSend()
}

func TestBufferStateRelease(t *testing.T) {
	var s bufferState
	if err := s.release(); err != nil {
		t.Fatalf("first release: %v", err)
	}
	if !s.isReleased() {
		t.Error("isReleased: want true after release")
	}
	// Double-release is a misuse.
	if err := s.release(); !errors.Is(err, ErrBufferReleased) {
		t.Errorf("double release: want ErrBufferReleased, got %v", err)
	}
	// Sending a released buffer is rejected.
	if err := s.beginSend(); !errors.Is(err, ErrBufferReleased) {
		t.Errorf("beginSend released: want ErrBufferReleased, got %v", err)
	}
}

// TestBufferStateConcurrentSend exercises the atomic transitions under -race:
// many goroutines racing beginSend must yield exactly one winner per in-flight
// window.
func TestBufferStateConcurrentSend(t *testing.T) {
	var s bufferState
	const tries = 200
	var wins int32
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < tries; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.beginSend(); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
				s.completeSend()
			}
		}()
	}
	wg.Wait()
	if wins == 0 {
		t.Error("expected at least one successful beginSend")
	}
}
