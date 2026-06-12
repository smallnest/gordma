package perftest

import (
	"time"

	"github.com/smallnest/gordma"
)

// RunWriteBW runs the RDMA Write bandwidth benchmark. The client issues
// one-sided RDMA_WRITE operations into the server's registered buffer, keeping
// up to cfg.TxDepth outstanding. The server is passive (its NIC services the
// writes), so it simply waits for the client to finish via the out-of-band
// channel; here we model that by having the server return immediately after
// setup — the caller keeps the process alive until the client disconnects.
//
// RemoteAddr/RKey come from the peer info exchanged during setup.
func RunWriteBW(cfg Config, ep *Endpoint, mr *gordma.MR) (BWResult, error) {
	if cfg.IsServer() {
		// Passive side: nothing to post for RDMA Write. The bandwidth is
		// measured entirely on the client.
		return BWResult{}, nil
	}
	if ep.Peer == nil {
		return BWResult{}, errNoPeer
	}
	wc := make([]gordma.WorkCompletion, cfg.TxDepth)
	sg := gordma.SGEFromMR(mr, 0, cfg.Size)

	start := time.Now()
	posted, completed := 0, 0
	post := func(wrID uint64) error {
		return ep.QP.PostSend(gordma.SendWR{
			WRID:       wrID,
			Opcode:     gordma.OpWrite,
			SGList:     []gordma.SGE{sg},
			Signaled:   true,
			RemoteAddr: ep.Peer.RemoteAddr,
			RKey:       ep.Peer.RKey,
		})
	}
	for posted < cfg.TxDepth && posted < cfg.Iters {
		if err := post(uint64(posted)); err != nil {
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
				if err := post(uint64(posted)); err != nil {
					return BWResult{}, err
				}
				posted++
			}
		}
	}
	return BWResult{Bytes: cfg.Size, Iterations: cfg.Iters, Elapsed: time.Since(start)}, nil
}

// RunWriteLat runs the RDMA Write latency benchmark using the perftest
// "polling on last byte" method: the client writes a payload whose final byte
// it has set to a known sentinel into the peer's buffer, and the peer polls its
// local last byte until it changes, then writes back. This avoids any CQ
// completion on the receive side. We measure the full round trip on the client.
//
// The sentinel alternates each iteration so a stale value is never mistaken for
// a fresh arrival.
func RunWriteLat(cfg Config, ep *Endpoint, mr *gordma.MR) (LatResult, error) {
	if ep.Peer == nil {
		return LatResult{}, errNoPeer
	}
	buf := mr.Bytes()
	if len(buf) < cfg.Size || cfg.Size < 1 {
		return LatResult{}, errBufferTooSmall
	}
	last := cfg.Size - 1
	sg := gordma.SGEFromMR(mr, 0, cfg.Size)
	wc := make([]gordma.WorkCompletion, 2)

	write := func(wrID uint64) error {
		return ep.QP.PostSend(gordma.SendWR{
			WRID:       wrID,
			Opcode:     gordma.OpWrite,
			SGList:     []gordma.SGE{sg},
			Signaled:   true,
			RemoteAddr: ep.Peer.RemoteAddr,
			RKey:       ep.Peer.RKey,
		})
	}

	if cfg.IsServer() {
		// Server: for each iteration, wait for the client's write to land
		// (last byte flips), then write back with the next sentinel.
		var expect byte = 1
		for i := 0; i < cfg.Iters; i++ {
			for buf[last] != expect {
				// busy-wait for the remote write to complete
			}
			buf[last] = expect + 1 // echo sentinel
			if err := write(uint64(i)); err != nil {
				return LatResult{}, err
			}
			if err := pollOne(ep.CQ, wc); err != nil {
				return LatResult{}, err
			}
			expect += 2
		}
		return LatResult{Bytes: cfg.Size}, nil
	}

	// Client: set sentinel, write, then poll local last byte for the echo.
	samples := make([]time.Duration, 0, cfg.Iters)
	var sentinel byte = 1
	for i := 0; i < cfg.Iters; i++ {
		buf[last] = sentinel
		echo := sentinel + 1
		t0 := time.Now()
		if err := write(uint64(i)); err != nil {
			return LatResult{}, err
		}
		if err := pollOne(ep.CQ, wc); err != nil {
			return LatResult{}, err
		}
		for buf[last] != echo {
			// busy-wait for the server's echo write
		}
		samples = append(samples, time.Since(t0))
		sentinel += 2
	}
	return LatResult{Samples: samples, Bytes: cfg.Size}, nil
}
