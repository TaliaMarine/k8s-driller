// Package pressure implements the node/pod pressure-state and recommendation
// calculations defined in SPECS.md §9. Every function here is pure: plain
// numbers in, plain results out. No Kubernetes or Prometheus client lives in
// this package, so the formulas can be unit-tested without a live cluster.
package pressure

// CPU quantities are millicores, memory quantities are bytes — callers
// convert from resource.Quantity at the k8s/metrics boundary.

// Config holds every tunable threshold from SPECS.md §9, sourced from Helm
// values (SPECS.md §8.4) rather than hardcoded.
type Config struct {
	OOMRiskThresholdPct        float64 // default 90
	ThrottlingThresholdPct     float64 // default 95
	ThrottlingConsecutivePolls int     // default 3
	WastefulThresholdPct       float64 // default 70
	RecommendationHeadroomPct  float64 // default 10
	CPULimitMultiplier         float64 // default 2.0
	MemLimitMultiplier         float64 // default 1.5
}

// DefaultConfig returns the defaults documented in SPECS.md §9.
func DefaultConfig() Config {
	return Config{
		OOMRiskThresholdPct:        90,
		ThrottlingThresholdPct:     95,
		ThrottlingConsecutivePolls: 3,
		WastefulThresholdPct:       70,
		RecommendationHeadroomPct:  10,
		CPULimitMultiplier:         2.0,
		MemLimitMultiplier:         1.5,
	}
}

// ContainerResources mirrors one container's resource fields. A nil pointer
// means the field is unset on the container spec, distinct from a value of 0.
type ContainerResources struct {
	RequestsCPU *int64
	RequestsMem *int64
	LimitsCPU   *int64
	LimitsMem   *int64
}

// ContainerWildWest flags each missing resource dimension individually, per
// SPECS.md §9 ("each missing dimension flagged individually, not collapsed
// into one generic tag").
type ContainerWildWest struct {
	MissingRequestsCPU bool `json:"missingRequestsCpu"`
	MissingRequestsMem bool `json:"missingRequestsMem"`
	MissingLimitsCPU   bool `json:"missingLimitsCpu"`
	MissingLimitsMem   bool `json:"missingLimitsMem"`
}

// Any reports whether at least one dimension is missing.
func (w ContainerWildWest) Any() bool {
	return w.MissingRequestsCPU || w.MissingRequestsMem || w.MissingLimitsCPU || w.MissingLimitsMem
}

// DetectWildWest evaluates a single container against the Wild-West rule.
func DetectWildWest(c ContainerResources) ContainerWildWest {
	return ContainerWildWest{
		MissingRequestsCPU: c.RequestsCPU == nil,
		MissingRequestsMem: c.RequestsMem == nil,
		MissingLimitsCPU:   c.LimitsCPU == nil,
		MissingLimitsMem:   c.LimitsMem == nil,
	}
}

// PodAllocation is a pod's total requests/limits, summed across containers
// (unset fields contribute 0, matching how the scheduler treats them).
type PodAllocation struct {
	RequestsCPU int64
	RequestsMem int64
	LimitsCPU   int64
	LimitsMem   int64
}

// AggregatePod sums per-container resources into a pod-level allocation.
func AggregatePod(containers []ContainerResources) PodAllocation {
	var a PodAllocation
	for _, c := range containers {
		if c.RequestsCPU != nil {
			a.RequestsCPU += *c.RequestsCPU
		}
		if c.RequestsMem != nil {
			a.RequestsMem += *c.RequestsMem
		}
		if c.LimitsCPU != nil {
			a.LimitsCPU += *c.LimitsCPU
		}
		if c.LimitsMem != nil {
			a.LimitsMem += *c.LimitsMem
		}
	}
	return a
}

// NodeCapacity is a node's allocatable capacity.
type NodeCapacity struct {
	CPU    int64
	Memory int64
}

// NodeAllocation is the sum of every pod's requests/limits scheduled on a
// node.
type NodeAllocation struct {
	RequestsCPU int64
	RequestsMem int64
	LimitsCPU   int64
	LimitsMem   int64
}

// AggregateNode sums pod-level allocations into a node-level allocation.
func AggregateNode(pods []PodAllocation) NodeAllocation {
	var a NodeAllocation
	for _, p := range pods {
		a.RequestsCPU += p.RequestsCPU
		a.RequestsMem += p.RequestsMem
		a.LimitsCPU += p.LimitsCPU
		a.LimitsMem += p.LimitsMem
	}
	return a
}

// pct returns used/total as a percentage, or 0 when total is 0 (an
// allocatable capacity of 0 has no meaningful percentage).
func pct(used, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

// NodePressure is the set of allocation/live percentages and flags shown on
// a node card (SPECS.md §2.1/§7.1).
type NodePressure struct {
	RequestsCPUPct float64 `json:"requestsCpuPct"`
	RequestsMemPct float64 `json:"requestsMemPct"`
	LimitsCPUPct   float64 `json:"limitsCpuPct"`
	LimitsMemPct   float64 `json:"limitsMemPct"`
	LiveCPUPct     float64 `json:"liveCpuPct"`
	LiveMemPct     float64 `json:"liveMemPct"`
	OvercommitCPU  bool    `json:"overcommitCpu"`
	OvercommitMem  bool    `json:"overcommitMem"`
}

// ComputeNodePressure evaluates allocation, live usage, and overcommit for a
// single node. Overcommit is evaluated per-resource independently, per
// SPECS.md §9 ("a node can be CPU-overcommitted, Mem-overcommitted, both, or
// neither").
func ComputeNodePressure(alloc NodeAllocation, liveCPU, liveMem int64, cap NodeCapacity) NodePressure {
	return NodePressure{
		RequestsCPUPct: pct(alloc.RequestsCPU, cap.CPU),
		RequestsMemPct: pct(alloc.RequestsMem, cap.Memory),
		LimitsCPUPct:   pct(alloc.LimitsCPU, cap.CPU),
		LimitsMemPct:   pct(alloc.LimitsMem, cap.Memory),
		LiveCPUPct:     pct(liveCPU, cap.CPU),
		LiveMemPct:     pct(liveMem, cap.Memory),
		OvercommitCPU:  alloc.LimitsCPU > cap.CPU,
		OvercommitMem:  alloc.LimitsMem > cap.Memory,
	}
}

// OOMRisk reports whether live memory usage exceeds the OOM-Risk threshold of
// the configured memory limit. Returns false when limitMem is nil — a pod
// with no memory limit is Wild-West, not OOM-Risk (SPECS.md §9).
func (cfg Config) OOMRisk(usageMem int64, limitMem *int64) bool {
	if limitMem == nil || *limitMem <= 0 {
		return false
	}
	return pct(usageMem, *limitMem) > cfg.OOMRiskThresholdPct
}

// ThrottlingRisk reports whether CPU usage has stayed at or above the
// throttling threshold of the CPU limit for at least ThrottlingConsecutivePolls
// consecutive samples. samples must be ordered oldest-first; only the most
// recent ThrottlingConsecutivePolls entries are considered, so a single spike
// followed by normal usage never triggers this state (SPECS.md §9).
func (cfg Config) ThrottlingRisk(samples []int64, limitCPU *int64) bool {
	if limitCPU == nil || *limitCPU <= 0 {
		return false
	}
	if len(samples) < cfg.ThrottlingConsecutivePolls {
		return false
	}
	recent := samples[len(samples)-cfg.ThrottlingConsecutivePolls:]
	for _, s := range recent {
		if pct(s, *limitCPU) < cfg.ThrottlingThresholdPct {
			return false
		}
	}
	return true
}

// Wasteful reports whether a pod's configured request is significantly
// higher than its observed p95 usage over the lookback window. Only
// meaningful when request is set — a pod with no request is Wild-West, not
// Wasteful (SPECS.md §9).
func (cfg Config) Wasteful(p95Usage24h int64, request *int64) bool {
	if request == nil || *request <= 0 {
		return false
	}
	return float64(p95Usage24h) < (1-cfg.WastefulThresholdPct/100)*float64(*request)
}

// RecommendedRequest derives a recommended request from observed p95 usage
// plus a headroom floor, per SPECS.md §9.
func (cfg Config) RecommendedRequest(p95Usage int64) int64 {
	return int64(float64(p95Usage) * (1 + cfg.RecommendationHeadroomPct/100))
}

// RecommendedCPULimit derives a recommended CPU limit from a recommended
// request using the (looser) CPU multiplier.
func (cfg Config) RecommendedCPULimit(recommendedRequest int64) int64 {
	return int64(float64(recommendedRequest) * cfg.CPULimitMultiplier)
}

// RecommendedMemLimit derives a recommended memory limit from a recommended
// request using the (tighter) memory multiplier.
func (cfg Config) RecommendedMemLimit(recommendedRequest int64) int64 {
	return int64(float64(recommendedRequest) * cfg.MemLimitMultiplier)
}
