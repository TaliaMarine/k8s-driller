package analysis

import (
	"math"
	"testing"
	"time"
)

func approxEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func TestComputeEmpty(t *testing.T) {
	s := Compute(nil)
	if s != (Stats{}) {
		t.Fatalf("expected zero Stats for empty input, got %+v", s)
	}
}

func TestComputeStableSeries(t *testing.T) {
	samples := []float64{100, 100, 100, 100, 100}
	s := Compute(samples)
	if s.Mean != 100 || s.Median != 100 || s.Min != 100 || s.Max != 100 {
		t.Fatalf("unexpected stats for flat series: %+v", s)
	}
	if s.CV != 0 {
		t.Fatalf("expected CV 0 for a flat series, got %v", s.CV)
	}
}

func TestComputePercentiles(t *testing.T) {
	samples := make([]float64, 0, 101)
	for i := 0; i <= 100; i++ {
		samples = append(samples, float64(i))
	}
	s := Compute(samples)
	if !approxEqual(s.Median, 50, 0.01) {
		t.Fatalf("expected median ~50, got %v", s.Median)
	}
	if !approxEqual(s.P90, 90, 0.01) {
		t.Fatalf("expected p90 ~90, got %v", s.P90)
	}
	if !approxEqual(s.P99, 99, 0.01) {
		t.Fatalf("expected p99 ~99, got %v", s.P99)
	}
}

func TestPeakBucket(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	timestamps := []time.Time{
		base,
		base.Add(1 * time.Hour),
		base.Add(25 * time.Hour),
		base.Add(26 * time.Hour),
	}
	values := []float64{10, 50, 5, 8}

	peaks := PeakBucket(timestamps, values, 24*time.Hour)
	if len(peaks) != 2 {
		t.Fatalf("expected 2 buckets, got %d: %v", len(peaks), peaks)
	}
	if peaks[0] != 50 {
		t.Fatalf("expected first bucket peak 50, got %v", peaks[0])
	}
	if peaks[1] != 8 {
		t.Fatalf("expected second bucket peak 8, got %v", peaks[1])
	}
}

func TestRecommendRequestStableLeansToMean(t *testing.T) {
	stable := Compute([]float64{100, 102, 98, 101, 99, 100})
	req, _ := recommendRequest(stable, 10, nil, nil)
	// Low CV: request should stay close to mean*(1+10%), not jump to p90.
	if req > stable.Mean*1.2 {
		t.Fatalf("expected stable-series request close to mean, got %v (mean %v)", req, stable.Mean)
	}
}

func TestRecommendRequestBimodalLeansHigh(t *testing.T) {
	// Long idle stretch (near 0) plus a real active plateau (~1000) — the
	// mean is dragged far below what the pod actually needs while active.
	var samples []float64
	for i := 0; i < 80; i++ {
		samples = append(samples, 5)
	}
	for i := 0; i < 20; i++ {
		samples = append(samples, 1000)
	}
	bimodal := Compute(samples)
	req, reason := recommendRequest(bimodal, 10, nil, nil)
	if req <= bimodal.Mean*1.5 {
		t.Fatalf("expected bimodal request to lean well above the mean (%v), got %v", bimodal.Mean, req)
	}
	if reason == "" {
		t.Fatalf("expected a non-empty rationale")
	}
}

func TestRecommendCPULimitCoversP99(t *testing.T) {
	// A rare, sharp spike should still fit under the limit even though the
	// bulk of the series is low.
	var samples []float64
	for i := 0; i < 99; i++ {
		samples = append(samples, 10)
	}
	samples = append(samples, 5000)
	stats := Compute(samples)

	rec := RecommendCPU(stats, nil, nil, 10, 2.0)
	if float64(rec.RecommendedLimit) < stats.P99 {
		t.Fatalf("expected limit (%d) to cover p99 (%v)", rec.RecommendedLimit, stats.P99)
	}
}

func TestRecommendMemoryLimitCoversPeak(t *testing.T) {
	stats := Compute([]float64{100, 110, 90, 105, 500})
	rec := RecommendMemory(stats, nil, nil, 10, 1.5)
	if float64(rec.RecommendedLimit) < stats.Max {
		t.Fatalf("expected memory limit (%d) to cover the observed peak (%v)", rec.RecommendedLimit, stats.Max)
	}
}

func TestRecommendSubUnitRequestFloorsToOne(t *testing.T) {
	// A near-idle container with one rare large burst (real production
	// shape): mean gets pulled up by the burst, but p90 sits below 1 since
	// the vast majority of samples are near zero. Truncating a ~0.2
	// recommendation to int64 would silently show "0", which reads as "no
	// CPU needed" rather than "negligible but nonzero".
	var samples []float64
	for i := 0; i < 265; i++ {
		samples = append(samples, 0.2)
	}
	samples = append(samples, 40, 42)
	stats := Compute(samples)

	rec := RecommendCPU(stats, nil, nil, 10, 2.0)
	if rec.RecommendedRequest < 1 {
		t.Fatalf("expected a positive sub-1 target to floor to at least 1, got %d", rec.RecommendedRequest)
	}
}

func TestRecommendZeroUsage(t *testing.T) {
	stats := Compute([]float64{0, 0, 0})
	rec := RecommendCPU(stats, nil, nil, 10, 2.0)
	if rec.RecommendedRequest != 0 || rec.RecommendedLimit != 0 {
		t.Fatalf("expected zero recommendation for zero usage, got %+v", rec)
	}
}

func TestDetectPlateauFindsSustainedHighStretch(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// 10 hours idle at 10, then 10 hours at a sustained plateau of 200
	// (50% of the window), on a 15-minute step.
	var timestamps []time.Time
	var values []float64
	step := 15 * time.Minute
	for i := 0; i < 40; i++ {
		timestamps = append(timestamps, base.Add(time.Duration(i)*step))
		values = append(values, 10)
	}
	for i := 40; i < 80; i++ {
		timestamps = append(timestamps, base.Add(time.Duration(i)*step))
		values = append(values, 200)
	}
	stats := Compute(values)

	plateau, ok := DetectPlateau(timestamps, values, stats.P90)
	if !ok {
		t.Fatalf("expected a plateau to be detected")
	}
	if plateau.Level < 190 || plateau.Level > 210 {
		t.Fatalf("expected plateau level near 200, got %v", plateau.Level)
	}
	if plateau.Fraction < 0.3 {
		t.Fatalf("expected plateau fraction >= 0.3, got %v", plateau.Fraction)
	}
}

func TestDetectPlateauIgnoresBriefSpike(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var timestamps []time.Time
	var values []float64
	step := 15 * time.Minute
	for i := 0; i < 100; i++ {
		timestamps = append(timestamps, base.Add(time.Duration(i)*step))
		values = append(values, 10)
	}
	// One brief spike, well under plateauMinDuration.
	values[50] = 500
	stats := Compute(values)

	if _, ok := DetectPlateau(timestamps, values, stats.P90); ok {
		t.Fatalf("expected a brief spike not to register as a plateau")
	}
}

func TestRecommendCPUPrefersPlateauOverBlend(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var timestamps []time.Time
	var values []float64
	step := 15 * time.Minute
	for i := 0; i < 40; i++ {
		timestamps = append(timestamps, base.Add(time.Duration(i)*step))
		values = append(values, 10)
	}
	for i := 40; i < 80; i++ {
		timestamps = append(timestamps, base.Add(time.Duration(i)*step))
		values = append(values, 200)
	}
	stats := Compute(values)

	rec := RecommendCPU(stats, timestamps, values, 10, 2.0)
	// The mean/p90 blend alone would land well under 200; the plateau
	// should push the recommendation up near it instead.
	if rec.RecommendedRequest < 190 {
		t.Fatalf("expected plateau-driven request near 200, got %d", rec.RecommendedRequest)
	}
}
