// Package analysis turns a raw historical usage series (as returned by
// Prometheus range queries) into the summary statistics and request/limit
// recommendations shown on the pod detail "Analysis" tab. Every function
// here is pure — plain numbers in, plain results out — so the logic is
// unit-testable without a live Prometheus.
//
// The recommendation approach is deliberately modeled on the Kubernetes
// Vertical Pod Autoscaler recommender (kubernetes/autoscaler,
// vertical-pod-autoscaler/pkg/recommender): target a high percentile of
// observed usage rather than the mean, apply a safety margin, and — for
// memory specifically — recommend off the peak usage within each
// aggregation interval rather than continuous samples, since memory tends
// to plateau near its ceiling during an active period instead of
// oscillating the way CPU does. VPA maintains a decayed histogram
// continuously; this package instead recomputes from a Prometheus range
// query fetched on demand, so it trades VPA's exponential decay for a
// simpler CV-weighted blend of mean and p90 (see recommendRequest).
package analysis

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Stats summarizes one resource's historical usage: central tendency
// (Mean/Median), spread (StdDev/CV), and the percentiles used as
// recommendation targets.
type Stats struct {
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	P90    float64 `json:"p90"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
	StdDev float64 `json:"stddev"`
	// CV is the coefficient of variation (stddev/mean): near 0 means flat,
	// steady usage; above ~0.5 means bursty or bimodal (e.g. idle stretches
	// plus real active periods) usage.
	CV float64 `json:"cv"`
}

// Compute derives Stats from a set of samples. Returns the zero Stats for
// an empty input.
func Compute(samples []float64) Stats {
	if len(samples) == 0 {
		return Stats{}
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)

	var sum float64
	for _, v := range sorted {
		sum += v
	}
	mean := sum / float64(len(sorted))

	var sqDiff float64
	for _, v := range sorted {
		d := v - mean
		sqDiff += d * d
	}
	stddev := math.Sqrt(sqDiff / float64(len(sorted)))
	cv := 0.0
	if mean > 0 {
		cv = stddev / mean
	}

	return Stats{
		Mean:   mean,
		Median: percentile(sorted, 50),
		Min:    sorted[0],
		Max:    sorted[len(sorted)-1],
		P90:    percentile(sorted, 90),
		P95:    percentile(sorted, 95),
		P99:    percentile(sorted, 99),
		StdDev: stddev,
		CV:     cv,
	}
}

// percentile interpolates linearly between closest ranks (numpy's default
// convention) on an already-sorted, non-empty slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := p / 100 * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// PeakBucket collapses a timestamped series into one value per bucket — the
// maximum observed within it — mirroring VPA's memory histogram, which is
// built from one peak sample per aggregation interval (default 24h) instead
// of continuous usage. timestamps and values must be the same length and
// timestamps ascending.
func PeakBucket(timestamps []time.Time, values []float64, bucket time.Duration) []float64 {
	if len(values) == 0 {
		return nil
	}
	peaks := make([]float64, 0, len(values))
	bucketStart := timestamps[0]
	max := values[0]
	for i := 1; i < len(values); i++ {
		if timestamps[i].Sub(bucketStart) >= bucket {
			peaks = append(peaks, max)
			bucketStart = timestamps[i]
			max = values[i]
			continue
		}
		if values[i] > max {
			max = values[i]
		}
	}
	peaks = append(peaks, max)
	return peaks
}

// Recommendation is one resource's recommended request/limit plus a
// plain-language explanation of what drove the number.
type Recommendation struct {
	RecommendedRequest int64  `json:"recommendedRequest"`
	RecommendedLimit   int64  `json:"recommendedLimit"`
	Rationale          string `json:"rationale"`
}

// Plateau describes a sustained stretch of usage that sits significantly
// above the series' own 90th percentile — e.g. a workload that jumped to a
// new, higher steady-state and stayed there — which the mean/CV blend in
// recommendRequest would otherwise under-recommend for, since it treats the
// whole window as one distribution.
type Plateau struct {
	// Level is the mean usage while on the plateau.
	Level float64
	// Fraction is the share of the total observed time window spent on
	// plateau (there can be more than one stretch).
	Fraction float64
}

const (
	// plateauAboveP90 is how far above p90 a stretch must sit to count as a
	// plateau rather than ordinary noise just above the percentile.
	plateauAboveP90 = 1.10
	// plateauMinDuration is the minimum length of one contiguous stretch
	// above threshold before it counts toward the plateau at all — a brief
	// spike isn't a plateau, however high it goes.
	plateauMinDuration = 2 * time.Hour
	// plateauMinFraction is the minimum share of the total window that must
	// be spent on plateau(s) before the recommender prefers the plateau
	// level over the mean/p90 blend.
	plateauMinFraction = 0.30
)

// DetectPlateau scans a timestamped series for a sustained step up to a new,
// higher steady state. It works in three passes:
//
//  1. Mark contiguous stretches at or above the series' own p90 that last at
//     least plateauMinDuration — candidates for "a new higher level", not a
//     brief spike. (p90 alone isn't the bar for "significant": once a
//     stretch covers more than ~10% of the series, it pulls the series' own
//     p90 up into itself, so p90 can't be compared against directly.)
//  2. Compare the candidate's level against the baseline p90 — the
//     percentile of everything outside the candidate — and require it to be
//     meaningfully higher. This is the real "significantly higher than
//     p90" test, computed on the data the plateau would otherwise
//     contaminate.
//  3. Require the candidate to cover at least plateauMinFraction of the
//     total window, so a rare event doesn't override the normal
//     recommendation.
//
// timestamps and values must be the same length and timestamps ascending.
func DetectPlateau(timestamps []time.Time, values []float64, p90 float64) (Plateau, bool) {
	if len(timestamps) < 2 || len(timestamps) != len(values) || p90 <= 0 {
		return Plateau{}, false
	}
	totalDuration := timestamps[len(timestamps)-1].Sub(timestamps[0])
	if totalDuration <= 0 {
		return Plateau{}, false
	}

	inPlateau := make([]bool, len(values))
	var plateauDuration time.Duration
	runStart := -1
	markRun := func(end int) {
		if runStart < 0 {
			return
		}
		if dur := timestamps[end].Sub(timestamps[runStart]); dur >= plateauMinDuration {
			plateauDuration += dur
			for i := runStart; i <= end; i++ {
				inPlateau[i] = true
			}
		}
		runStart = -1
	}
	for i, v := range values {
		if v >= p90 {
			if runStart < 0 {
				runStart = i
			}
		} else {
			markRun(i - 1)
		}
	}
	markRun(len(values) - 1)

	var plateauValues, baselineValues []float64
	for i, v := range values {
		if inPlateau[i] {
			plateauValues = append(plateauValues, v)
		} else {
			baselineValues = append(baselineValues, v)
		}
	}
	// No baseline left to compare against (the whole series qualified as
	// one candidate stretch) means there's nothing to be "significantly
	// higher" than.
	if len(plateauValues) == 0 || len(baselineValues) == 0 {
		return Plateau{}, false
	}

	baselineP90 := Compute(baselineValues).P90
	plateauLevel := mean(plateauValues)
	if baselineP90 <= 0 || plateauLevel < baselineP90*plateauAboveP90 {
		return Plateau{}, false
	}

	fraction := float64(plateauDuration) / float64(totalDuration)
	if fraction < plateauMinFraction {
		return Plateau{}, false
	}
	return Plateau{Level: plateauLevel, Fraction: fraction}, true
}

func mean(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// recommendRequest blends the mean and the 90th percentile by how variable
// the series is: a flat series (CV≈0) recommends close to the mean, while a
// bursty or bimodal one — long idle stretches plus real active periods,
// which drag the mean down without lowering actual peak needs — leans
// toward the 90th percentile instead ("lean towards the higher ground, not
// just the average"). headroomPct then adds a flat safety floor on top.
//
// Before falling back to that blend, it checks for a sustained plateau
// (see DetectPlateau) — a workload that spends a large share of the window
// meaningfully above its own p90 wants the recommendation anchored to that
// plateau, not diluted by whatever it was doing the rest of the time.
func recommendRequest(s Stats, headroomPct float64, timestamps []time.Time, values []float64) (float64, string) {
	if s.Mean <= 0 {
		return 0, "no usage observed in this window"
	}

	if plateau, ok := DetectPlateau(timestamps, values, s.P90); ok {
		request := plateau.Level * (1 + headroomPct/100)
		reason := fmt.Sprintf(
			"usage holds a sustained plateau around %.0f for %.0f%% of the observed window — well above the 90th percentile (%.0f) — so the recommendation targets that plateau instead of the mean/percentile blend",
			plateau.Level, plateau.Fraction*100, s.P90,
		)
		return request, reason
	}

	weight := s.CV * 2
	if weight > 1 {
		weight = 1
	}
	target := s.Mean + weight*(s.P90-s.Mean)
	request := target * (1 + headroomPct/100)

	var reason string
	switch {
	case weight < 0.15:
		reason = fmt.Sprintf("usage is stable (CV %.2f), so the recommendation tracks close to the mean", s.CV)
	case weight < 0.7:
		reason = fmt.Sprintf("usage varies (CV %.2f), so the recommendation leans toward the 90th percentile rather than the mean", s.CV)
	default:
		reason = fmt.Sprintf("usage swings between idle and active periods (CV %.2f), so the recommendation tracks the 90th percentile instead of being dragged down by the idle stretches", s.CV)
	}
	return request, reason
}

// round rounds to the nearest integer, flooring at 1 whenever the true
// value is positive but sub-1 (e.g. a near-idle container recommending
// 0.2 millicores) — plain truncation would show a misleading "0" rather
// than the negligible-but-nonzero request it actually is.
func round(v float64) int64 {
	if v <= 0 {
		return 0
	}
	r := int64(math.Round(v))
	if r < 1 {
		return 1
	}
	return r
}

// RecommendCPU derives a CPU request/limit recommendation from raw usage
// Stats (no peak-bucketing — CPU is compressible and naturally noisy, so a
// percentile of continuous samples already captures burstiness; only
// memory gets the peak-bucket treatment, per VPA). timestamps/values are
// the same raw series raw was computed from, used for plateau detection.
func RecommendCPU(raw Stats, timestamps []time.Time, values []float64, headroomPct, limitMultiplier float64) Recommendation {
	req, reason := recommendRequest(raw, headroomPct, timestamps, values)
	limit := req * limitMultiplier
	// A burst that spikes past p99 should still fit under the limit even if
	// the multiplier alone wouldn't cover it.
	if p99Limit := raw.P99 * 1.05; p99Limit > limit {
		limit = p99Limit
	}
	return Recommendation{
		RecommendedRequest: round(req),
		RecommendedLimit:   round(limit),
		Rationale:          reason,
	}
}

// RecommendMemory derives a memory request/limit recommendation from
// peak-bucketed Stats (see PeakBucket). Memory gets extra headroom beyond
// the configured floor because under-provisioning risks an OOM-kill —
// unrecoverable — where CPU under-provisioning only causes recoverable
// throttling. timestamps/values are the raw (non-bucketed) usage series,
// used for plateau detection — memory sitting continuously near its
// ceiling for hours is exactly the plateau shape peak-bucketing alone
// would smooth over.
func RecommendMemory(peaks Stats, timestamps []time.Time, values []float64, headroomPct, limitMultiplier float64) Recommendation {
	req, reason := recommendRequest(peaks, headroomPct*1.5, timestamps, values)
	limit := req * limitMultiplier
	// The limit must cover the single worst observed peak, not just a
	// multiple of the (already conservative) recommended request.
	if peakLimit := peaks.Max * 1.05; peakLimit > limit {
		limit = peakLimit
	}
	return Recommendation{
		RecommendedRequest: round(req),
		RecommendedLimit:   round(limit),
		Rationale:          reason + " (based on the peak usage within each day, not the raw average, since memory usage plateaus near its ceiling during an active period rather than oscillating like CPU)",
	}
}
