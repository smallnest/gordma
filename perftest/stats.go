package perftest

import (
	"fmt"
	"io"
	"math"
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
	_, _ = fmt.Fprintf(w, "%-12s %-12s %-18s %-14s\n", "#bytes", "#iterations", "BW average[MB/s]", "MsgRate[Mpps]")
	_, _ = fmt.Fprintf(w, "%-12d %-12d %-18.2f %-14.6f\n", r.Bytes, r.Iterations, r.MBPerSec(), r.MsgRateMpps())
}

// LatResult holds per-iteration latency samples (in nanoseconds) and computes
// summary statistics and an optional histogram, mirroring perftest output.
type LatResult struct {
	// Samples are per-iteration round-trip (or operation) times.
	Samples []time.Duration
	Bytes   int
}

// LatStats is the summary of a latency sample set, all in microseconds.
type LatStats struct {
	Min, Avg, Max, P99 float64
}

// sortedMicros converts the samples to microseconds and returns them sorted
// ascending. Callers that need several statistics should sort once via this
// helper and derive everything from the result rather than re-sorting.
func (r LatResult) sortedMicros() []float64 {
	us := make([]float64, len(r.Samples))
	for i, s := range r.Samples {
		us[i] = float64(s.Nanoseconds()) / 1000.0
	}
	sort.Float64s(us)
	return us
}

// percentileOf returns the p-th percentile (nearest-rank, 1-based) of an
// already-sorted slice. p is clamped to [0,100].
func percentileOf(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	rank := int(math.Ceil((p / 100.0) * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// Stats computes min/avg/max/p99 in a single sort pass over the samples.
func (r LatResult) Stats() LatStats {
	if len(r.Samples) == 0 {
		return LatStats{}
	}
	sorted := r.sortedMicros()
	var sum float64
	for _, v := range sorted {
		sum += v
	}
	return LatStats{
		Min: sorted[0],
		Avg: sum / float64(len(sorted)),
		Max: sorted[len(sorted)-1],
		P99: percentileOf(sorted, 99),
	}
}

// WriteSummary prints the t_min/t_avg/t_max/p99 summary line in microseconds.
func (r LatResult) WriteSummary(w io.Writer) {
	s := r.Stats()
	_, _ = fmt.Fprintf(w, "%-12s %-10s %-10s %-10s %-10s\n", "#bytes", "t_min[us]", "t_avg[us]", "t_max[us]", "p99[us]")
	_, _ = fmt.Fprintf(w, "%-12d %-10.2f %-10.2f %-10.2f %-10.2f\n", r.Bytes, s.Min, s.Avg, s.Max, s.P99)
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
	// Single O(n) pass for the extremes — no sort needed for bucketing.
	min := float64(r.Samples[0].Nanoseconds()) / 1000.0
	max := min
	for _, s := range r.Samples {
		v := float64(s.Nanoseconds()) / 1000.0
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
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
	_, _ = fmt.Fprintf(w, "%-12s %-12s %-8s\n", "low[us]", "high[us]", "count")
	for _, b := range bins {
		_, _ = fmt.Fprintf(w, "%-12.2f %-12.2f %-8d\n", b.LowMicros, b.HighMicros, b.Count)
	}
	r.WriteSummary(w)
}
