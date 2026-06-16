package rdmanet

import (
	"errors"
	"testing"

	"github.com/smallnest/gordma"
)

// These tests exercise the build-agnostic pipeline/drain drivers with fake
// post/poll funcs, so they run on any platform without RDMA hardware.

func TestPipelineCompletesExactIters(t *testing.T) {
	const iters, txDepth = 100, 8
	posted, completed := 0, 0
	inflight := 0
	maxInflight := 0
	post := func(wrID uint64) error {
		posted++
		inflight++
		if inflight > maxInflight {
			maxInflight = inflight
		}
		return nil
	}
	poll := func(wc []gordma.WorkCompletion) (int, error) {
		// Complete one per poll to keep the in-flight bookkeeping observable.
		if inflight == 0 {
			return 0, nil
		}
		inflight--
		completed++
		wc[0] = gordma.WorkCompletion{Status: gordma.WCSuccess}
		return 1, nil
	}
	if err := pipeline(iters, txDepth, post, poll); err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	if posted != iters {
		t.Errorf("posted = %d, want %d", posted, iters)
	}
	if completed != iters {
		t.Errorf("completed = %d, want %d", completed, iters)
	}
	if maxInflight > txDepth {
		t.Errorf("max in-flight = %d, exceeds txDepth %d", maxInflight, txDepth)
	}
}

func TestPipelineZeroIters(t *testing.T) {
	called := false
	post := func(uint64) error { called = true; return nil }
	poll := func([]gordma.WorkCompletion) (int, error) { called = true; return 0, nil }
	if err := pipeline(0, 8, post, poll); err != nil {
		t.Fatalf("pipeline(0): %v", err)
	}
	if called {
		t.Error("pipeline(0) should not post or poll")
	}
}

func TestPipelinePropagatesPostError(t *testing.T) {
	want := errors.New("post boom")
	post := func(uint64) error { return want }
	poll := func([]gordma.WorkCompletion) (int, error) { return 0, nil }
	if err := pipeline(10, 4, post, poll); !errors.Is(err, want) {
		t.Errorf("pipeline post error: got %v, want %v", err, want)
	}
}

func TestPipelineFailsOnBadCompletion(t *testing.T) {
	post := func(uint64) error { return nil }
	poll := func(wc []gordma.WorkCompletion) (int, error) {
		wc[0] = gordma.WorkCompletion{Status: gordma.WCRetryExcErr, WRID: 7}
		return 1, nil
	}
	var ce *gordma.CompletionError
	if err := pipeline(10, 4, post, poll); !errors.As(err, &ce) {
		t.Errorf("pipeline bad completion: got %v, want *CompletionError", err)
	}
}

func TestDrainCompletesExactIters(t *testing.T) {
	const iters, txDepth = 50, 4
	posted, completed := 0, 0
	post := func(wrID uint64) error { posted++; return nil }
	poll := func(wc []gordma.WorkCompletion) (int, error) {
		wc[0] = gordma.WorkCompletion{Status: gordma.WCSuccess}
		completed++
		return 1, nil
	}
	if err := drain(iters, txDepth, post, poll); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if completed != iters {
		t.Errorf("completed = %d, want %d", completed, iters)
	}
	// txDepth pre-posts + one re-post per completion except the last txDepth.
	if posted != iters {
		t.Errorf("posted = %d, want %d", posted, iters)
	}
}

func TestPipelineBatchCompletesExactIters(t *testing.T) {
	const iters, txDepth = 100, 8
	built, completed := 0, 0
	inflight, maxInflight := 0, 0
	batchCalls := 0
	build := func(wrID uint64) gordma.SendWR { built++; return gordma.SendWR{WRID: wrID} }
	postBatch := func(wrs []gordma.SendWR) error {
		batchCalls++
		inflight += len(wrs)
		if inflight > maxInflight {
			maxInflight = inflight
		}
		return nil
	}
	// Complete in groups of up to 4 per poll to exercise batched refills.
	poll := func(wc []gordma.WorkCompletion) (int, error) {
		k := inflight
		if k > 4 {
			k = 4
		}
		for i := 0; i < k; i++ {
			wc[i] = gordma.WorkCompletion{Status: gordma.WCSuccess}
		}
		inflight -= k
		completed += k
		return k, nil
	}
	if err := pipelineBatch(iters, txDepth, build, postBatch, poll); err != nil {
		t.Fatalf("pipelineBatch: %v", err)
	}
	if built != iters {
		t.Errorf("built = %d, want %d", built, iters)
	}
	if completed != iters {
		t.Errorf("completed = %d, want %d", completed, iters)
	}
	if maxInflight > txDepth {
		t.Errorf("max in-flight = %d, exceeds txDepth %d", maxInflight, txDepth)
	}
	// Batched submit must use far fewer calls than one-per-WR.
	if batchCalls >= iters {
		t.Errorf("batch calls = %d, expected far fewer than %d", batchCalls, iters)
	}
}

func TestPipelineBatchPropagatesError(t *testing.T) {
	want := errors.New("batch boom")
	build := func(wrID uint64) gordma.SendWR { return gordma.SendWR{WRID: wrID} }
	postBatch := func(wrs []gordma.SendWR) error { return want }
	poll := func([]gordma.WorkCompletion) (int, error) { return 0, nil }
	if err := pipelineBatch(10, 4, build, postBatch, poll); !errors.Is(err, want) {
		t.Errorf("pipelineBatch error: got %v, want %v", err, want)
	}
}
