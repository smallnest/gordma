package rdmanet

import "testing"

func TestDefaultConfig(t *testing.T) {
	c := defaultConfig()
	if c.device != "" {
		t.Errorf("device: want empty, got %q", c.device)
	}
	if c.port != DefaultPort {
		t.Errorf("port: want %d, got %d", DefaultPort, c.port)
	}
	if c.gidIndex != DefaultGIDIndex {
		t.Errorf("gidIndex: want %d, got %d", DefaultGIDIndex, c.gidIndex)
	}
	if c.queueDepth != DefaultQueueDepth {
		t.Errorf("queueDepth: want %d, got %d", DefaultQueueDepth, c.queueDepth)
	}
	if c.bufferSize != DefaultBufferSize {
		t.Errorf("bufferSize: want %d, got %d", DefaultBufferSize, c.bufferSize)
	}
	if c.handshake {
		t.Error("handshake: want false by default")
	}
	if c.pollMode != PollEvent {
		t.Errorf("pollMode: want PollEvent, got %v", c.pollMode)
	}
}

func TestApplyOptions(t *testing.T) {
	c := applyOptions([]Option{
		WithDevice("mlx5_3"),
		WithPort(2),
		WithGIDIndex(3),
		WithQueueDepth(256),
		WithBufferSize(8192),
		WithHandshake(),
		WithPollMode(PollBusy),
		nil, // nil options must be ignored
	})
	if c.device != "mlx5_3" {
		t.Errorf("device: got %q", c.device)
	}
	if c.port != 2 {
		t.Errorf("port: got %d", c.port)
	}
	if c.gidIndex != 3 {
		t.Errorf("gidIndex: got %d", c.gidIndex)
	}
	if c.queueDepth != 256 {
		t.Errorf("queueDepth: got %d", c.queueDepth)
	}
	if c.bufferSize != 8192 {
		t.Errorf("bufferSize: got %d", c.bufferSize)
	}
	if !c.handshake {
		t.Error("handshake: want true")
	}
	if c.pollMode != PollBusy {
		t.Errorf("pollMode: want PollBusy, got %v", c.pollMode)
	}
}

func TestPollModeString(t *testing.T) {
	cases := map[PollMode]string{
		PollEvent:     "event",
		PollBusy:      "busy",
		PollMode(999): "unknown",
	}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("PollMode(%d).String(): want %q, got %q", m, want, got)
		}
	}
}
