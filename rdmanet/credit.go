package rdmanet

import "sync"

// creditTracker is a counting semaphore used for receiver-driven flow control.
// A sender acquires one credit before transmitting a frame and blocks when no
// credits remain; the receiver releases credits back as it frees recv slots.
// This prevents the sender from posting more SENDs than the peer has posted
// RECVs, which would otherwise cause RNR (receiver-not-ready) errors or data
// loss. creditTracker is safe for concurrent use.
type creditTracker struct {
	mu      sync.Mutex
	cond    *sync.Cond
	credits int
	closed  bool
}

// newCreditTracker returns a tracker seeded with initial credits (typically the
// peer's receive-queue depth).
func newCreditTracker(initial int) *creditTracker {
	t := &creditTracker{credits: initial}
	t.cond = sync.NewCond(&t.mu)
	return t
}

// acquire blocks until at least one credit is available, then consumes it.
// It returns false if the tracker was closed while waiting.
func (t *creditTracker) acquire() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for t.credits == 0 && !t.closed {
		t.cond.Wait()
	}
	if t.closed {
		return false
	}
	t.credits--
	return true
}

// release returns n credits to the pool and wakes any waiting acquirers.
func (t *creditTracker) release(n int) {
	if n <= 0 {
		return
	}
	t.mu.Lock()
	t.credits += n
	t.mu.Unlock()
	t.cond.Broadcast()
}

// available returns the current credit count (for tests/metrics).
func (t *creditTracker) available() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.credits
}

// close wakes all waiters; subsequent acquire calls return false.
func (t *creditTracker) close() {
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	t.cond.Broadcast()
}
