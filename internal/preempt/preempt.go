package preempt

import (
	"os"
	"strings"
	"syscall"
)

// envKey is the GODEBUG sub-key that turns off signal-based async preemption.
const envKey = "asyncpreemptoff"

// Disable ensures the current process runs with GODEBUG=asyncpreemptoff=1,
// re-executing itself once if the runtime did not already see that setting.
//
// GODEBUG is parsed by the runtime before main runs, so a process cannot apply
// asyncpreemptoff to itself any other way. Disable is a no-op when the variable
// already specifies asyncpreemptoff (the re-exec'd child, or a user who set it
// explicitly — including asyncpreemptoff=0 to keep preemption on), so there is
// no re-exec loop. It is best-effort: if locating or re-executing the binary
// fails, the program simply continues with preemption enabled.
//
// Call it as the first statement in main, before any RDMA/cgo setup.
func Disable() {
	cur := os.Getenv("GODEBUG")
	next, changed := mergedGODEBUG(cur)
	if !changed {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		return // best-effort: run with preemption rather than fail
	}

	// Replaces the process image (same PID, inherited fds, args, exit code).
	// Returns only on error, in which case we fall through and run anyway.
	_ = syscall.Exec(exe, os.Args, withGODEBUG(os.Environ(), next))
}

// mergedGODEBUG returns the GODEBUG value that adds asyncpreemptoff=1, and
// reports whether a change is needed. If cur already mentions asyncpreemptoff
// (any value), it is left untouched and changed is false.
func mergedGODEBUG(cur string) (next string, changed bool) {
	for _, kv := range strings.Split(cur, ",") {
		if strings.HasPrefix(strings.TrimSpace(kv), envKey+"=") {
			return cur, false
		}
	}
	if cur == "" {
		return envKey + "=1", true
	}
	return cur + "," + envKey + "=1", true
}

// withGODEBUG returns env with any existing GODEBUG entry replaced by val, so
// the runtime sees a single unambiguous GODEBUG.
func withGODEBUG(env []string, val string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if strings.HasPrefix(kv, "GODEBUG=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "GODEBUG="+val)
}
