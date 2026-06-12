package perftest

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseArgsDefaults(t *testing.T) {
	var out bytes.Buffer
	cfg, err := ParseArgs("go-send_bw", nil, &out)
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if cfg.TxDepth != 128 {
		t.Errorf("default tx-depth = %d, want 128", cfg.TxDepth)
	}
	if cfg.Transport != TransportRC {
		t.Errorf("default transport = %v, want RC", cfg.Transport)
	}
	if cfg.ConnMethod != ConnTCP {
		t.Errorf("default conn method = %v, want tcp", cfg.ConnMethod)
	}
	if !cfg.IsServer() {
		t.Error("no peer addr should be server mode")
	}
}

func TestParseArgsClientAndFlags(t *testing.T) {
	var out bytes.Buffer
	args := []string{"-s", "1024", "-n", "5000", "-c", "UD", "-R", "-t", "256", "-x", "3", "--output", "histogram", "10.0.0.2:18515"}
	cfg, err := ParseArgs("go-send_lat", args, &out)
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if cfg.Size != 1024 || cfg.Iters != 5000 || cfg.TxDepth != 256 || cfg.GIDIndex != 3 {
		t.Errorf("flags not parsed: %+v", cfg)
	}
	if cfg.Transport != TransportUD {
		t.Error("expected UD transport")
	}
	if cfg.ConnMethod != ConnRDMACM {
		t.Error("expected rdma_cm with -R")
	}
	if !cfg.Histogram {
		t.Error("expected histogram output")
	}
	if cfg.IsServer() || cfg.ServerAddr != "10.0.0.2:18515" {
		t.Errorf("expected client mode to 10.0.0.2:18515, got %q", cfg.ServerAddr)
	}
}

func TestParseArgsRejectsBadTransport(t *testing.T) {
	var out bytes.Buffer
	if _, err := ParseArgs("t", []string{"-c", "XX"}, &out); err == nil {
		t.Fatal("expected error for bad -c")
	}
}

func TestParseArgsValidation(t *testing.T) {
	var out bytes.Buffer
	if _, err := ParseArgs("t", []string{"-s", "0"}, &out); err == nil {
		t.Fatal("expected error for -s 0")
	}
}

func TestBWResult(t *testing.T) {
	// 1000 iterations of 1MB in 1s -> 1000 MB/s, 0.001 Mpps.
	r := BWResult{Bytes: 1_000_000, Iterations: 1000, Elapsed: time.Second}
	if got := r.MBPerSec(); got < 999 || got > 1001 {
		t.Errorf("MBPerSec = %f, want ~1000", got)
	}
	if got := r.MsgRateMpps(); got < 0.0009 || got > 0.0011 {
		t.Errorf("MsgRateMpps = %f, want ~0.001", got)
	}
	var b bytes.Buffer
	r.WriteBW(&b)
	if !strings.Contains(b.String(), "BW average[MB/s]") {
		t.Error("WriteBW missing header")
	}
}

func TestLatResultStats(t *testing.T) {
	samples := make([]time.Duration, 100)
	for i := 0; i < 100; i++ {
		samples[i] = time.Duration(i+1) * time.Microsecond // 1..100 us
	}
	r := LatResult{Samples: samples, Bytes: 64}
	if min := r.Min(); min < 0.99 || min > 1.01 {
		t.Errorf("Min = %f, want ~1", min)
	}
	if max := r.Max(); max < 99.9 || max > 100.1 {
		t.Errorf("Max = %f, want ~100", max)
	}
	if avg := r.Avg(); avg < 50 || avg > 51 {
		t.Errorf("Avg = %f, want ~50.5", avg)
	}
	// p99 nearest-rank over 1..100 should be 99 or 100.
	if p99 := r.Percentile(99); p99 < 98.9 || p99 > 100.1 {
		t.Errorf("p99 = %f, want ~99-100", p99)
	}
}

func TestLatHistogram(t *testing.T) {
	samples := make([]time.Duration, 10)
	for i := range samples {
		samples[i] = time.Duration(i+1) * time.Microsecond
	}
	r := LatResult{Samples: samples}
	bins := r.Histogram(5)
	if len(bins) != 5 {
		t.Fatalf("expected 5 bins, got %d", len(bins))
	}
	total := 0
	for _, b := range bins {
		total += b.Count
	}
	if total != 10 {
		t.Errorf("histogram counts sum = %d, want 10", total)
	}
}

func TestLatHistogramAllEqual(t *testing.T) {
	r := LatResult{Samples: []time.Duration{5 * time.Microsecond, 5 * time.Microsecond}}
	bins := r.Histogram(4)
	if len(bins) != 1 || bins[0].Count != 2 {
		t.Fatalf("equal samples should yield one bin with all counts, got %+v", bins)
	}
}
