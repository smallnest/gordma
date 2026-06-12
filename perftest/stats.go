package perftest

import (
	"fmt"
	"io"
	"sort"
	"time"
)

// BWResult holds bandwidth measurement results.
type BWResult struct {
	Bytes      int
	Iterations int
	Elapsed    time.Duration
}

// MBPerSec returns the average bandwidth in megabytes per second (10^6 bytes).
func (r BWResult) MBPerSec() float64 {
	if r.Elapsed <= 0 {
		return 0
	}
	totalBytes := float64(r.Bytes) * float64(r.Iterations)
	return totalBytes / 1e6 / r.Elapsed.Seconds()
}

// MsgRateMpps returns the message rate in millions of packets per second.
func (r BWResult) MsgRateMpps() float64 {
	if r.Elapsed <= 0 {
		return 0
	}
	return float64(r.Iterations) / 1e6 / r.Elapsed.Seconds()
}

// WriteBW prints the perftest-style bandwidth table to w.
func (r BWResult) WriteBW(w io.Writer) {
	fmt.Fprintf(w, "%-12s %-12s %-18s %-14s\n", "#bytes", "#iterations", "BW average[MB/s]", "MsgRate[Mpps]")
	fmt.Fprintf(w, "%-12d %-12d %-18.2f %-14.6f\n", r.Bytes, r.Iterations, r.MBPerSec(), r.MsgRateMpps())
}

// LatResult holds per-iteration latency samples (in nanoseconds) and computes
// summary statistics and an optional histogram, mirroring perftest output.
type LatResult struct {
	// Samples are per-iteration round-trip (or operation) times.
	Samples []time.Duration
	Bytes   int
}

// summary returns min/avg/max/p99 over the samples (sorted copy).
func (r LatResult) sortedMicros() []float64 {
	us := make([]float64, len(r.Samples))
	for i, s := range r.Samples {
		us[i] = float64(s.Nanoseconds()) / 1000.0
	}
	sort.Float64s(us)
	return us
}

// Min returns the minimum latency in microseconds.
func (r LatResult) Min() float64 {
	s := r.sortedMicros()
	if len(s) == 0 {
		return 0
	}
	return s[0]
}

// Max returns the maximum latency in microseconds.
func (r LatResult) Max() float64 {
	s := r.sortedMicros()
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1]
}

// Avg returns the mean latency in microseconds.
func (r LatResult) Avg() float64 {
	if len(r.Samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range r.Samples {
		sum += float64(s.Nanoseconds()) / 1000.0
	}
	return sum / float64(len(r.Samples))
}

// Percentile returns the p-th percentile latency in microseconds, where p is
// in [0,100]. Uses the nearest-rank method on the sorted samples.
func (r LatResult) Percentile(p float64) float64 {
	s := r.sortedMicros()
	if len(s) == 0 {
		return 0
	}
	if p <= 0 {
		return s[0]
	}
	if p >= 100 {
		return s[len(s)-1]
	}
	// nearest-rank: rank = ceil(p/100 * N), 1-based.
	rank := int((p/100.0)*float64(len(s)) + 0.999999)
	if rank < 1 {
		rank = 1
	}
	if rank > len(s) {
		rank = len(s)
	}
	return s[rank-1]
}

// WriteSummary prints the t_min/t_avg/t_max/p99 summary line in microseconds.
func (r LatResult) WriteSummary(w io.Writer) {
	fmt.Fprintf(w, "%-12s %-10s %-10s %-10s %-10s\n", "#bytes", "t_min[us]", "t_avg[us]", "t_max[us]", "p99[us]")
	fmt.Fprintf(w, "%-12d %-10.2f %-10.2f %-10.2f %-10.2f\n", r.Bytes, r.Min(), r.Avg(), r.Max(), r.Percentile(99))
}

// HistogramBin is one bucket of a latency histogram.
type HistogramBin struct {
	// LowMicros and HighMicros bound the bucket (inclusive low, exclusive high).
	LowMicros  float64
	HighMicros float64
	Count      int
}

// Histogram buckets the samples into nbins linear buckets between min and max.
func (r LatResult) Histogram(nbins int) []HistogramBin {
	if nbins <= 0 || len(r.Samples) == 0 {
		return nil
	}
	min, max := r.Min(), r.Max()
	if max <= min {
		// All samples equal: a single bucket.
		return []HistogramBin{{LowMicros: min, HighMicros: min, Count: len(r.Samples)}}
	}
	width := (max - min) / float64(nbins)
	bins := make([]HistogramBin, nbins)
	for i := range bins {
		bins[i].LowMicros = min + float64(i)*width
		bins[i].HighMicros = min + float64(i+1)*width
	}
	for _, s := range r.Samples {
		v := float64(s.Nanoseconds()) / 1000.0
		idx := int((v - min) / width)
		if idx >= nbins {
			idx = nbins - 1
		}
		if idx < 0 {
			idx = 0
		}
		bins[idx].Count++
	}
	return bins
}

// WriteHistogram prints a full latency histogram followed by the summary.
func (r LatResult) WriteHistogram(w io.Writer, nbins int) {
	bins := r.Histogram(nbins)
	fmt.Fprintf(w, "%-12s %-12s %-8s\n", "low[us]", "high[us]", "count")
	for _, b := range bins {
		fmt.Fprintf(w, "%-12.2f %-12.2f %-8d\n", b.LowMicros, b.HighMicros, b.Count)
	}
	r.WriteSummary(w)
}
