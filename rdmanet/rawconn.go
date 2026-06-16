package rdmanet

import "github.com/smallnest/gordma"

// RawConn is a minimal, high-throughput RC endpoint that trades rdmanet's
// net-style conveniences for raw speed. Unlike Conn it does NOT frame messages,
// do credit-based flow control, copy through bounce buffers, or run a
// background completion poller. The caller registers memory and drives
// post/poll directly (or via the built-in Pipeline / RecvDrain helpers), all on
// one goroutine — matching how the perftest tools reach line rate.
//
// Trade-offs vs Conn:
//   - No message boundaries: a Send/Recv is one work request, not a framed
//     "message"; the caller sizes transfers and matches sends to recvs.
//   - No flow control: the caller must keep outstanding sends within the peer's
//     posted recvs (the Pipeline/RecvDrain helpers and a shared tx-depth handle
//     this) or risk RNR.
//   - No managed buffers: register MRs with RegisterMemory and build the SGEs.
//
// RawConn is established over the TCP out-of-band handshake so the peer's RKey
// and remote address are exchanged, enabling one-sided RDMA Write/Read in
// addition to two-sided Send/Recv.
//
// The concrete fields and methods live in rawconn_linux.go (cgo) and
// rawconn_stub.go (everything else); this file holds the build-agnostic
// pipeline driver so it can be unit-tested without RDMA hardware.

// pipeline runs the credit-free throughput loop shared by RawConn.Pipeline (and
// mirrored from perftest.runBWPipeline): it keeps up to txDepth signaled work
// requests outstanding, posting a new one via post on each completion until
// iters have completed. poll drains the CQ into the supplied slice. It is
// generic over the post/poll funcs so it can be tested with fakes.
func pipeline(
	iters, txDepth int,
	post func(wrID uint64) error,
	poll func(wc []gordma.WorkCompletion) (int, error),
) error {
	if iters <= 0 {
		return nil
	}
	if txDepth < 1 {
		txDepth = 1
	}
	if txDepth > iters {
		txDepth = iters
	}
	wc := make([]gordma.WorkCompletion, txDepth)
	posted, completed := 0, 0
	for posted < txDepth {
		if err := post(uint64(posted)); err != nil {
			return err
		}
		posted++
	}
	for completed < iters {
		n, err := poll(wc)
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			if !wc[i].Status.OK() {
				return &gordma.CompletionError{Status: wc[i].Status, WRID: wc[i].WRID}
			}
			completed++
			if posted < iters {
				if err := post(uint64(posted)); err != nil {
					return err
				}
				posted++
			}
		}
	}
	return nil
}

// pipelineBatch is the batched variant of pipeline: it keeps up to txDepth
// signaled WRs in flight but submits refills in groups — on each poll it
// reaps k completions and re-posts them in a single postBatch call, cutting
// the per-WR cgo crossing. build(wrID) returns the WR for a given slot;
// postBatch submits a slice of WRs in one call. It is generic over the
// build/postBatch/poll funcs for testing.
func pipelineBatch(
	iters, txDepth int,
	build func(wrID uint64) gordma.SendWR,
	postBatch func(wrs []gordma.SendWR) error,
	poll func(wc []gordma.WorkCompletion) (int, error),
) error {
	if iters <= 0 {
		return nil
	}
	if txDepth < 1 {
		txDepth = 1
	}
	if txDepth > iters {
		txDepth = iters
	}
	wc := make([]gordma.WorkCompletion, txDepth)
	batch := make([]gordma.SendWR, 0, txDepth)
	posted, completed := 0, 0

	// Prime the pipeline with one full batch of txDepth WRs.
	for posted < txDepth {
		batch = append(batch, build(uint64(posted)))
		posted++
	}
	if err := postBatch(batch); err != nil {
		return err
	}

	for completed < iters {
		n, err := poll(wc)
		if err != nil {
			return err
		}
		batch = batch[:0]
		for i := 0; i < n; i++ {
			if !wc[i].Status.OK() {
				return &gordma.CompletionError{Status: wc[i].Status, WRID: wc[i].WRID}
			}
			completed++
			if posted < iters {
				batch = append(batch, build(uint64(posted)))
				posted++
			}
		}
		if len(batch) > 0 {
			if err := postBatch(batch); err != nil {
				return err
			}
		}
	}
	return nil
}

// drain runs the passive (receive) side of the throughput loop: pre-post
// txDepth receive work requests, then reap completions until iters have
// arrived, re-posting a fresh recv (via rebuild) on each completion. It is
// generic over the post/poll funcs for testing.
func drain(
	iters, txDepth int,
	postRecv func(wrID uint64) error,
	poll func(wc []gordma.WorkCompletion) (int, error),
) error {
	if iters <= 0 {
		return nil
	}
	if txDepth < 1 {
		txDepth = 1
	}
	if txDepth > iters {
		txDepth = iters
	}
	wc := make([]gordma.WorkCompletion, txDepth)
	posted, completed := 0, 0
	for posted < txDepth {
		if err := postRecv(uint64(posted)); err != nil {
			return err
		}
		posted++
	}
	for completed < iters {
		n, err := poll(wc)
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			if !wc[i].Status.OK() {
				return &gordma.CompletionError{Status: wc[i].Status, WRID: wc[i].WRID}
			}
			completed++
			if posted < iters {
				if err := postRecv(uint64(posted % txDepth)); err != nil {
					return err
				}
				posted++
			}
		}
	}
	return nil
}
