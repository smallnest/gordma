// Package preempt lets a command-line tool disable Go's signal-based
// asynchronous goroutine preemption for its own process.
//
// Why a benchmark needs this: the busy-poll bandwidth loops spin in Go code for
// many milliseconds without crossing a function-call safepoint. Go's runtime
// has sysmon send such a goroutine a SIGURG every ~10ms to force an
// asynchronous preemption; while the handler runs, nothing drains the
// completion queue and the send pipeline empties. Measured with GORDMA_PROBE
// this roughly doubles the per-WR poll time (1.46µs -> 0.74µs) and halves
// throughput (0.41 -> 0.75 Mpps), and the random preemption timing is what
// makes back-to-back runs swing wildly. Turning it off keeps the spin tight and
// steady at line rate; the cgo submit cost is unaffected (post stays ~260ns/WR
// either way).
//
// The setting lives in GODEBUG, which the runtime parses before main runs, so a
// process cannot apply it to itself except by re-executing with the variable
// set. Disable does exactly that, and is a no-op the second time around (and
// when the user has set asyncpreemptoff explicitly), so there is no loop.
package preempt
