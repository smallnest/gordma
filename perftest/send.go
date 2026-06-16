package perftest

import (
	"io"
	"os"
	"time"

	"github.com/smallnest/gordma"
)

// probeEnabled reports whether GORDMA_PROBE is set, turning on the post-vs-poll
// timing split in the bandwidth loops. It mirrors the convention used by the
// rdmanet transport and RawConn probes so the tools report comparable numbers.
func probeEnabled() bool { return os.Getenv("GORDMA_PROBE") != "" }

// pollOne busy-polls the CQ until exactly one completion arrives, returning an
// error if the completion status is not success or the QP/CQ is closed.
func pollOne(cq *gordma.CQ, wc []gordma.WorkCompletion) error {
	for {
		n, err := cq.Poll(wc)
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			if !wc[i].Status.OK() {
				return &gordma.CompletionError{Status: wc[i].Status, WRID: wc[i].WRID}
			}
		}
		if n > 0 {
			return nil
		}
	}
}

// runBWPipeline drives the credit-based bandwidth loop shared by the send,
// write and read bandwidth benchmarks: it keeps up to cfg.TxDepth work requests
// outstanding, posting a new one (via post) on each completion until cfg.Iters
// have completed. post is given the work-request id to use. It returns the
// measured bandwidth, including a post-vs-poll timing split when GORDMA_PROBE
// is set (the same diagnostic RawConn exposes).
func runBWPipeline(cfg Config, cq *gordma.CQ, post func(wrID uint64) error) (BWResult, error) {
	probe := probeEnabled()
	var postNs, pollNs int64
	wc := make([]gordma.WorkCompletion, cfg.TxDepth)
	start := time.Now()
	posted, completed := 0, 0
	for posted < cfg.TxDepth && posted < cfg.Iters {
		if probe {
			t0 := time.Now()
			if err := post(uint64(posted)); err != nil {
				return BWResult{}, err
			}
			postNs += int64(time.Since(t0))
		} else if err := post(uint64(posted)); err != nil {
			return BWResult{}, err
		}
		posted++
	}
	for completed < cfg.Iters {
		var n int
		var err error
		if probe {
			t0 := time.Now()
			n, err = cq.Poll(wc)
			pollNs += int64(time.Since(t0))
		} else {
			n, err = cq.Poll(wc)
		}
		if err != nil {
			return BWResult{}, err
		}
		for i := 0; i < n; i++ {
			if !wc[i].Status.OK() {
				return BWResult{}, &gordma.CompletionError{Status: wc[i].Status, WRID: wc[i].WRID}
			}
			completed++
			if posted < cfg.Iters {
				if probe {
					t0 := time.Now()
					if err := post(uint64(posted)); err != nil {
						return BWResult{}, err
					}
					postNs += int64(time.Since(t0))
				} else if err := post(uint64(posted)); err != nil {
					return BWResult{}, err
				}
				posted++
			}
		}
	}
	return BWResult{
		Bytes:      cfg.Size,
		Iterations: cfg.Iters,
		Elapsed:    time.Since(start),
		PostWait:   time.Duration(postNs),
		PollWait:   time.Duration(pollNs),
	}, nil
}

// RunSendBW runs the Send/Recv bandwidth benchmark. The client posts cfg.Iters
// sends keeping up to cfg.TxDepth outstanding; the server posts matching recvs.
// It returns the measured bandwidth (client side) or a zero result (server).
func RunSendBW(cfg Config, ep *Endpoint, mr *gordma.MR) (BWResult, error) {
	sg := gordma.SGEFromMR(mr, grhOffset(cfg), cfg.Size)

	if cfg.IsServer() {
		// Server receives cfg.Iters messages, keeping TxDepth recvs posted.
		wc := make([]gordma.WorkCompletion, cfg.TxDepth)
		rsg := recvSGE(cfg, mr)
		for i := 0; i < cfg.TxDepth && i < cfg.Iters; i++ {
			if err := ep.QP.PostRecv(gordma.RecvWR{WRID: uint64(i), SGList: []gordma.SGE{rsg}}); err != nil {
				return BWResult{}, err
			}
		}
		got := 0
		for got < cfg.Iters {
			n, err := ep.CQ.Poll(wc)
			if err != nil {
				return BWResult{}, err
			}
			for i := 0; i < n; i++ {
				if !wc[i].Status.OK() {
					return BWResult{}, &gordma.CompletionError{Status: wc[i].Status, WRID: wc[i].WRID}
				}
				got++
				if got+cfg.TxDepth <= cfg.Iters {
					if err := ep.QP.PostRecv(gordma.RecvWR{WRID: wc[i].WRID, SGList: []gordma.SGE{rsg}}); err != nil {
						return BWResult{}, err
					}
				}
			}
		}
		return BWResult{}, nil
	}

	return runBWPipeline(cfg, ep.CQ, func(wrID uint64) error {
		return postSend(cfg, ep, sg, wrID)
	})
}

// RunSendLat runs the Send/Recv ping-pong latency benchmark. The client times
// each round-trip: send then wait for the echoed reply. The server echoes.
func RunSendLat(cfg Config, ep *Endpoint, mr *gordma.MR) (LatResult, error) {
	wc := make([]gordma.WorkCompletion, 4)
	sg := gordma.SGEFromMR(mr, grhOffset(cfg), cfg.Size)
	rsg := recvSGE(cfg, mr)

	if cfg.IsServer() {
		// Server: for each iter, recv then send back.
		for i := 0; i < cfg.Iters; i++ {
			if err := ep.QP.PostRecv(gordma.RecvWR{WRID: 1, SGList: []gordma.SGE{rsg}}); err != nil {
				return LatResult{}, err
			}
			if err := pollOne(ep.CQ, wc); err != nil {
				return LatResult{}, err
			}
			if err := postSend(cfg, ep, sg, 2); err != nil {
				return LatResult{}, err
			}
			if err := pollOne(ep.CQ, wc); err != nil {
				return LatResult{}, err
			}
		}
		return LatResult{Bytes: cfg.Size}, nil
	}

	// Client: time each ping-pong.
	samples := make([]time.Duration, 0, cfg.Iters)
	for i := 0; i < cfg.Iters; i++ {
		if err := ep.QP.PostRecv(gordma.RecvWR{WRID: 1, SGList: []gordma.SGE{rsg}}); err != nil {
			return LatResult{}, err
		}
		t0 := time.Now()
		if err := postSend(cfg, ep, sg, 2); err != nil {
			return LatResult{}, err
		}
		if err := pollOne(ep.CQ, wc); err != nil { // send completion
			return LatResult{}, err
		}
		if err := pollOne(ep.CQ, wc); err != nil { // recv reply
			return LatResult{}, err
		}
		samples = append(samples, time.Since(t0))
	}
	return LatResult{Samples: samples, Bytes: cfg.Size}, nil
}

// postSend posts one SEND, adding UD addressing when needed.
func postSend(cfg Config, ep *Endpoint, sg gordma.SGE, wrID uint64) error {
	wr := gordma.SendWR{WRID: wrID, Opcode: gordma.OpSend, SGList: []gordma.SGE{sg}, Signaled: true}
	if cfg.Transport == TransportUD && ep.udAH != nil {
		wr.AH = ep.udAH
		wr.RemoteQPN = ep.Peer.QPN
		wr.RemoteQKey = udQKey
	}
	return ep.QP.PostSend(wr)
}

// recvSGE returns the receive SGE, which for UD must include the leading GRH.
func recvSGE(cfg Config, mr *gordma.MR) gordma.SGE {
	if cfg.Transport == TransportUD {
		return gordma.SGEFromMR(mr, 0, cfg.Size+gordma.GRHLength)
	}
	return gordma.SGEFromMR(mr, 0, cfg.Size)
}

// grhOffset returns the send-buffer offset that skips the GRH region on UD.
func grhOffset(cfg Config) int {
	if cfg.Transport == TransportUD {
		return gordma.GRHLength
	}
	return 0
}

// PrintResult writes either bandwidth or latency output to w based on cfg.
func PrintBW(w io.Writer, r BWResult) { r.WriteBW(w) }

// PrintLat writes the latency summary, or the full histogram when requested.
func PrintLat(w io.Writer, cfg Config, r LatResult) {
	if cfg.Histogram {
		r.WriteHistogram(w, 64)
		return
	}
	r.WriteSummary(w)
}
