package rdmanet

import "testing"

// PollMode option parsing is build-agnostic, so it is verified here without any
// RDMA hardware. The behavioral equivalence of the two modes on a live QP is
// exercised by the bench tool (#44) via --poll=busy|event.

func TestPollModeDefaultIsEvent(t *testing.T) {
	c := defaultConfig()
	if c.pollMode != PollEvent {
		t.Errorf("default pollMode: want PollEvent, got %v", c.pollMode)
	}
}

func TestWithPollModeOption(t *testing.T) {
	for _, m := range []PollMode{PollEvent, PollBusy} {
		c := applyOptions([]Option{WithPollMode(m)})
		if c.pollMode != m {
			t.Errorf("WithPollMode(%v): got %v", m, c.pollMode)
		}
	}
}

func TestPollModeStringRoundTrip(t *testing.T) {
	if PollEvent.String() != "event" || PollBusy.String() != "busy" {
		t.Errorf("PollMode.String mismatch: event=%q busy=%q", PollEvent, PollBusy)
	}
}
