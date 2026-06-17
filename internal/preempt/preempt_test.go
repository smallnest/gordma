package preempt

import (
	"slices"
	"strings"
	"testing"
)

func TestMergedGODEBUG(t *testing.T) {
	tests := []struct {
		name        string
		cur         string
		wantNext    string
		wantChanged bool
	}{
		{"empty", "", "asyncpreemptoff=1", true},
		{"unrelated", "madvdontneed=1", "madvdontneed=1,asyncpreemptoff=1", true},
		{"already on", "asyncpreemptoff=1", "asyncpreemptoff=1", false},
		{"user kept it on", "asyncpreemptoff=0", "asyncpreemptoff=0", false},
		{"already on among others", "madvdontneed=1,asyncpreemptoff=1", "madvdontneed=1,asyncpreemptoff=1", false},
		{"spaces around entry", " asyncpreemptoff=1 ", " asyncpreemptoff=1 ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, changed := mergedGODEBUG(tt.cur)
			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}
			if next != tt.wantNext {
				t.Errorf("next = %q, want %q", next, tt.wantNext)
			}
		})
	}
}

func TestWithGODEBUG(t *testing.T) {
	env := []string{"PATH=/bin", "GODEBUG=old=1", "HOME=/root"}
	got := withGODEBUG(env, "asyncpreemptoff=1")

	// Exactly one GODEBUG entry, holding the new value.
	var godebug []string
	for _, kv := range got {
		if strings.HasPrefix(kv, "GODEBUG=") {
			godebug = append(godebug, kv)
		}
	}
	if !slices.Equal(godebug, []string{"GODEBUG=asyncpreemptoff=1"}) {
		t.Errorf("GODEBUG entries = %v, want exactly [GODEBUG=asyncpreemptoff=1]", godebug)
	}
	// Non-GODEBUG entries are preserved.
	if !slices.Contains(got, "PATH=/bin") || !slices.Contains(got, "HOME=/root") {
		t.Errorf("lost non-GODEBUG entries: %v", got)
	}
}
