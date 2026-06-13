package rdmanet

import "time"

// PollMode selects how a connection's completion queue is drained.
type PollMode int

const (
	// PollEvent waits for completion-channel events (ibv_get_cq_event) and
	// reaps in batches when woken. It is the default: lower CPU usage at the
	// cost of some latency.
	PollEvent PollMode = iota
	// PollBusy dedicates a goroutine to busy-polling the CQ. Lowest latency,
	// highest CPU usage.
	PollBusy
)

// String implements fmt.Stringer for PollMode.
func (m PollMode) String() string {
	switch m {
	case PollEvent:
		return "event"
	case PollBusy:
		return "busy"
	default:
		return "unknown"
	}
}

// Default configuration values applied when no option overrides them.
const (
	// DefaultPort is the HCA port number used when WithPort is not supplied.
	DefaultPort = 1
	// DefaultGIDIndex is the GID table index used when WithGIDIndex is not
	// supplied.
	DefaultGIDIndex = 0
	// DefaultQueueDepth is the send/recv queue depth (number of outstanding
	// work requests) used when WithQueueDepth is not supplied.
	DefaultQueueDepth = 128
	// DefaultBufferSize is the size in bytes of each registered bounce buffer
	// slot used when WithBufferSize is not supplied.
	DefaultBufferSize = 64 * 1024
	// DefaultDialTimeout is the timeout applied to Dial when none is given.
	DefaultDialTimeout = 5 * time.Second
)

// config holds the resolved settings for a Conn, PacketConn, or Listener. It
// is populated from the package defaults and then mutated by the supplied
// Options.
type config struct {
	device     string
	port       int
	gidIndex   int
	queueDepth int
	bufferSize int
	handshake  bool
	pollMode   PollMode
}

// defaultConfig returns a config seeded with the package default values.
func defaultConfig() config {
	return config{
		device:     "",
		port:       DefaultPort,
		gidIndex:   DefaultGIDIndex,
		queueDepth: DefaultQueueDepth,
		bufferSize: DefaultBufferSize,
		handshake:  false,
		pollMode:   PollEvent,
	}
}

// applyOptions returns a config built from defaults with all opts applied in
// order.
func applyOptions(opts []Option) config {
	c := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	return c
}

// Option configures a Conn, PacketConn, or Listener. Options follow the
// functional-options pattern and are passed to Dial, Listen, and ListenPacket.
type Option func(*config)

// WithDevice selects the RDMA device by name (for example "mlx5_3"). The empty
// string (the default) means "use the first available device".
func WithDevice(name string) Option {
	return func(c *config) { c.device = name }
}

// WithPort sets the HCA port number to use. Defaults to DefaultPort.
func WithPort(n int) Option {
	return func(c *config) { c.port = n }
}

// WithGIDIndex sets the GID table index (relevant for RoCE v2). Defaults to
// DefaultGIDIndex.
func WithGIDIndex(n int) Option {
	return func(c *config) { c.gidIndex = n }
}

// WithQueueDepth sets the send/recv queue depth — the number of work requests
// that may be outstanding at once. Defaults to DefaultQueueDepth.
func WithQueueDepth(n int) Option {
	return func(c *config) { c.queueDepth = n }
}

// WithBufferSize sets the size in bytes of each registered bounce-buffer slot.
// Defaults to DefaultBufferSize.
func WithBufferSize(n int) Option {
	return func(c *config) { c.bufferSize = n }
}

// WithHandshake selects the TCP out-of-band handshake (see the gordma/handshake
// package) for connection establishment instead of the default rdma_cm path.
func WithHandshake() Option {
	return func(c *config) { c.handshake = true }
}

// WithPollMode selects how the completion queue is drained (PollEvent or
// PollBusy). Defaults to PollEvent.
func WithPollMode(mode PollMode) Option {
	return func(c *config) { c.pollMode = mode }
}
