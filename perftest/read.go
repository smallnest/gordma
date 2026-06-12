package perftest

import (
	"time"

	"github.com/smallnest/gordma"
)

// RunReadBW runs the RDMA Read bandwidth benchmark. The client issues one-sided
// RDMA_READ operations from the server's registered buffer, keeping up to
// cfg.TxDepth outstanding. RDMA Read is RC-only. The server is passive.
func RunReadBW(cfg Config, ep *Endpoint, mr *gordma.MR) (BWResult, error) {
	if cfg.IsServer() {
		return BWResult{}, nil
	}
	if ep.Peer == nil {
		return BWResult{}, errNoPeer
	}
	wc := make([]gordma.WorkCompletion, cfg.TxDepth)
	sg := gordma.SGEFromMR(mr, 0, cfg.Size)

	read := func(wrID uint64) error {
		return ep.QP.PostSend(gordma.SendWR{
			WRID:       wrID,
			Opcode:     gordma.OpRead,
			SGList:     []gordma.SGE{sg},
			Signaled:   true,
			RemoteAddr: ep.Peer.RemoteAddr,
			RKey:       ep.Peer.RKey,
		})
	}

	start := time.Now()
	posted, completed := 0, 0
	for posted < cfg.TxDepth && posted < cfg.Iters {
		if err := read(uint64(posted)); err != nil {
			return BWResult{}, err
		}
		posted++
	}
	for completed < cfg.Iters {
		n, err := ep.CQ.Poll(wc)
		if err != nil {
			return BWResult{}, err
		}
		for i := 0; i < n; i++ {
			if !wc[i].Status.OK() {
				return BWResult{}, &gordma.CompletionError{Status: wc[i].Status, WRID: wc[i].WRID}
			}
			completed++
			if posted < cfg.Iters {
				if err := read(uint64(posted)); err != nil {
					return BWResult{}, err
				}
				posted++
			}
		}
	}
	return BWResult{Bytes: cfg.Size, Iterations: cfg.Iters, Elapsed: time.Since(start)}, nil
}

// RunReadLat runs the RDMA Read latency benchmark. Each RDMA_READ is itself a
// round trip (request out, data back), so latency is simply the time from
// posting the read to its completion. There is no work for the server. RC-only.
func RunReadLat(cfg Config, ep *Endpoint, mr *gordma.MR) (LatResult, error) {
	if cfg.IsServer() {
		// Passive: the client's reads are serviced by the NIC. Nothing to do
		// other than keep the process alive (handled by the caller).
		return LatResult{Bytes: cfg.Size}, nil
	}
	if ep.Peer == nil {
		return LatResult{}, errNoPeer
	}
	wc := make([]gordma.WorkCompletion, 2)
	sg := gordma.SGEFromMR(mr, 0, cfg.Size)

	samples := make([]time.Duration, 0, cfg.Iters)
	for i := 0; i < cfg.Iters; i++ {
		t0 := time.Now()
		if err := ep.QP.PostSend(gordma.SendWR{
			WRID:       uint64(i),
			Opcode:     gordma.OpRead,
			SGList:     []gordma.SGE{sg},
			Signaled:   true,
			RemoteAddr: ep.Peer.RemoteAddr,
			RKey:       ep.Peer.RKey,
		}); err != nil {
			return LatResult{}, err
		}
		if err := pollOne(ep.CQ, wc); err != nil {
			return LatResult{}, err
		}
		samples = append(samples, time.Since(t0))
	}
	return LatResult{Samples: samples, Bytes: cfg.Size}, nil
}
