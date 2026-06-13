package rdmanet

import (
	"sync"
	"testing"
	"time"
)

func TestCreditTrackerAcquireRelease(t *testing.T) {
	ct := newCreditTracker(2)
	if !ct.acquire() || !ct.acquire() {
		t.Fatal("first two acquires should succeed")
	}
	if ct.available() != 0 {
		t.Fatalf("available: want 0, got %d", ct.available())
	}

	// A third acquire blocks until a release arrives.
	done := make(chan struct{})
	go func() {
		ct.acquire()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("acquire returned with no credits available")
	case <-time.After(50 * time.Millisecond):
	}
	ct.release(1)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("acquire did not wake after release")
	}
}

func TestCreditTrackerCloseUnblocks(t *testing.T) {
	ct := newCreditTracker(0)
	got := make(chan bool, 1)
	go func() { got <- ct.acquire() }()
	time.Sleep(20 * time.Millisecond)
	ct.close()
	select {
	case ok := <-got:
		if ok {
			t.Error("acquire after close should return false")
		}
	case <-time.After(time.Second):
		t.Fatal("acquire did not unblock on close")
	}
}

// TestCreditTrackerConcurrent exercises the tracker under -race with many
// concurrent acquirers and releasers, mirroring the sender/receiver contention
// on the flow-control credits.
func TestCreditTrackerConcurrent(t *testing.T) {
	ct := newCreditTracker(4)
	var wg sync.WaitGroup
	const workers = 8
	const iters = 1000
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				if ct.acquire() {
					ct.release(1)
				}
			}
		}()
	}
	wg.Wait()
	if ct.available() != 4 {
		t.Errorf("available: want 4 after balanced acquire/release, got %d", ct.available())
	}
}
