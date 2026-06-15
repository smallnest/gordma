// Package perftest provides the shared command-line skeleton for the six
// perftest-style tools (send/write/read × bw/lat): flag parsing, connection
// method selection, and bandwidth/latency statistics with histogram output.
// It is pure Go (no cgo) so the parsing and statistics logic is unit-testable
// anywhere; the actual RDMA transfers are wired in by each tool.
package perftest

import (
	"fmt"
	"io"

	flag "github.com/spf13/pflag"
)

// ConnMethod selects how the two endpoints establish their connection.
type ConnMethod int

const (
	// ConnTCP uses the TCP out-of-band handshake (perftest default).
	ConnTCP ConnMethod = iota
	// ConnRDMACM uses the rdma_cm connection manager (-R).
	ConnRDMACM
)

func (m ConnMethod) String() string {
	if m == ConnRDMACM {
		return "rdma_cm"
	}
	return "tcp"
}

// Transport selects the QP transport type.
type Transport int

const (
	// TransportRC is reliable connected.
	TransportRC Transport = iota
	// TransportUD is unreliable datagram.
	TransportUD
)

func (t Transport) String() string {
	if t == TransportUD {
		return "UD"
	}
	return "RC"
}

// Config holds the parsed common parameters shared by all six tools. It mirrors
// the subset of perftest flags the PRD requires.
type Config struct {
	// Size is the message size in bytes (-s).
	Size int
	// Iters is the number of iterations (-n).
	Iters int
	// Device is the RDMA device name (-d); empty means first available.
	Device string
	// IBPort is the 1-based HCA port number (-i).
	IBPort int
	// TCPPort is the TCP handshake port (-p).
	TCPPort int
	// Transport is RC or UD (-c).
	Transport Transport
	// ConnMethod is TCP or rdma_cm (-R toggles rdma_cm).
	ConnMethod ConnMethod
	// TxDepth is the number of outstanding send WRs (-t).
	TxDepth int
	// GIDIndex is the local GID table index (-x).
	GIDIndex int
	// Histogram requests full latency histogram output (--output=histogram).
	Histogram bool
	// ServerAddr, when non-empty, runs in client mode targeting this address;
	// empty runs in server mode.
	ServerAddr string
}

// IsServer reports whether this config runs the server side (no peer address).
func (c *Config) IsServer() bool { return c.ServerAddr == "" }

// DefaultConfig returns a Config populated with perftest-compatible defaults.
func DefaultConfig() Config {
	return Config{
		Size:       65536,
		Iters:      1000,
		IBPort:     1,
		TCPPort:    18515,
		Transport:  TransportRC,
		ConnMethod: ConnTCP,
		TxDepth:    128,
		GIDIndex:   0,
	}
}

// Validate checks the configuration for obviously invalid combinations.
func (c *Config) Validate() error {
	if c.Size <= 0 {
		return fmt.Errorf("perftest: -s message size must be > 0, got %d", c.Size)
	}
	if c.Iters <= 0 {
		return fmt.Errorf("perftest: -n iterations must be > 0, got %d", c.Iters)
	}
	if c.IBPort <= 0 {
		return fmt.Errorf("perftest: -i ib port must be > 0, got %d", c.IBPort)
	}
	if c.TCPPort <= 0 || c.TCPPort > 65535 {
		return fmt.Errorf("perftest: -p tcp port out of range: %d", c.TCPPort)
	}
	if c.TxDepth <= 0 {
		return fmt.Errorf("perftest: -t tx-depth must be > 0, got %d", c.TxDepth)
	}
	if c.GIDIndex < 0 {
		return fmt.Errorf("perftest: -x gid index must be >= 0, got %d", c.GIDIndex)
	}
	return nil
}

// RequireOneSidedTCP validates the constraints common to the RDMA Write/Read
// tools: those ops are RC-only and need the TCP handshake to exchange the
// peer's RKey/remote address, so rdma_cm (-R) and UD (-c UD) are rejected.
// requireRC controls whether the UD check applies (Read is RC-only; Write is
// too in this toolset). Returns a descriptive error or nil.
func (c *Config) RequireOneSidedTCP() error {
	if c.ConnMethod == ConnRDMACM {
		return fmt.Errorf("-R (rdma_cm) is not supported; RDMA Write/Read needs the TCP handshake to exchange RKey/addr")
	}
	if c.Transport == TransportUD {
		return fmt.Errorf("RDMA Write/Read is RC-only; -c UD is not supported")
	}
	return nil
}

// ParseArgs parses the common flags from args (typically os.Args[1:]) into a
// Config. A trailing non-flag argument is taken as the server address (client
// mode). toolName is used in usage output. Output and errors are written to
// out; on -h/parse error it returns flag.ErrHelp or the parse error.
func ParseArgs(toolName string, args []string, out io.Writer) (Config, error) {
	cfg := DefaultConfig()
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(out)

	fs.IntVarP(&cfg.Size, "size", "s", cfg.Size, "message size in bytes")
	fs.IntVarP(&cfg.Iters, "count", "n", cfg.Iters, "number of iterations")
	fs.StringVarP(&cfg.Device, "device", "d", cfg.Device, "RDMA device name (default: first)")
	fs.IntVarP(&cfg.IBPort, "ib-port", "i", cfg.IBPort, "HCA port number (1-based)")
	fs.IntVarP(&cfg.TCPPort, "tcp-port", "p", cfg.TCPPort, "TCP port for out-of-band handshake")
	fs.IntVarP(&cfg.TxDepth, "tx-depth", "t", cfg.TxDepth, "tx depth (outstanding send WRs)")
	fs.IntVarP(&cfg.GIDIndex, "gid-index", "x", cfg.GIDIndex, "GID index")

	var transport string
	fs.StringVarP(&transport, "transport", "c", "RC", "connection transport: RC or UD")
	var useCM bool
	fs.BoolVarP(&useCM, "rdma-cm", "R", false, "use rdma_cm for connection establishment")
	var output string
	fs.StringVar(&output, "output", "", "extra output mode; set to 'histogram' for full latency histogram")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	switch transport {
	case "RC", "rc":
		cfg.Transport = TransportRC
	case "UD", "ud":
		cfg.Transport = TransportUD
	default:
		return Config{}, fmt.Errorf("perftest: -c must be RC or UD, got %q", transport)
	}
	if useCM {
		cfg.ConnMethod = ConnRDMACM
	}
	if output == "histogram" {
		cfg.Histogram = true
	} else if output != "" {
		return Config{}, fmt.Errorf("perftest: --output must be 'histogram' or empty, got %q", output)
	}

	if rest := fs.Args(); len(rest) > 0 {
		cfg.ServerAddr = rest[0]
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
