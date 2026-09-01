package pressure

import "testing"

func ptr(v int64) *int64 { return &v }

func TestDetectWildWest(t *testing.T) {
	cases := []struct {
		name string
		in   ContainerResources
		want ContainerWildWest
	}{
		{
			name: "fully configured",
			in:   ContainerResources{RequestsCPU: ptr(100), RequestsMem: ptr(256), LimitsCPU: ptr(200), LimitsMem: ptr(512)},
			want: ContainerWildWest{},
		},
		{
			name: "missing only mem limit",
			in:   ContainerResources{RequestsCPU: ptr(100), RequestsMem: ptr(256), LimitsCPU: ptr(200)},
			want: ContainerWildWest{MissingLimitsMem: true},
		},
		{
			name: "nothing set",
			in:   ContainerResources{},
			want: ContainerWildWest{MissingRequestsCPU: true, MissingRequestsMem: true, MissingLimitsCPU: true, MissingLimitsMem: true},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DetectWildWest(c.in)
			if got != c.want {
				t.Fatalf("DetectWildWest() = %+v, want %+v", got, c.want)
			}
			if got.Any() != c.want.Any() {
				t.Fatalf("Any() = %v, want %v", got.Any(), c.want.Any())
			}
		})
	}
}

func TestAggregatePod(t *testing.T) {
	containers := []ContainerResources{
		{RequestsCPU: ptr(100), RequestsMem: ptr(256), LimitsCPU: ptr(200), LimitsMem: ptr(512)},
		{RequestsCPU: ptr(50), LimitsMem: ptr(128)}, // missing mem request and cpu limit
	}
	got := AggregatePod(containers)
	want := PodAllocation{RequestsCPU: 150, RequestsMem: 256, LimitsCPU: 200, LimitsMem: 640}
	if got != want {
		t.Fatalf("AggregatePod() = %+v, want %+v", got, want)
	}
}

func TestComputeNodePressure_Overcommit(t *testing.T) {
	cap := NodeCapacity{CPU: 1000, Memory: 1000}

	cases := []struct {
		name         string
		alloc        NodeAllocation
		wantOvercCPU bool
		wantOvercMem bool
	}{
		{"healthy", NodeAllocation{LimitsCPU: 800, LimitsMem: 800}, false, false},
		{"cpu overcommit only", NodeAllocation{LimitsCPU: 1200, LimitsMem: 800}, true, false},
		{"mem overcommit only", NodeAllocation{LimitsCPU: 800, LimitsMem: 1200}, false, true},
		{"both overcommit", NodeAllocation{LimitsCPU: 1200, LimitsMem: 1500}, true, true},
		{"exactly 100pct is not overcommit", NodeAllocation{LimitsCPU: 1000, LimitsMem: 1000}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := ComputeNodePressure(c.alloc, 0, 0, cap)
			if p.OvercommitCPU != c.wantOvercCPU || p.OvercommitMem != c.wantOvercMem {
				t.Fatalf("overcommit = (cpu:%v mem:%v), want (cpu:%v mem:%v)", p.OvercommitCPU, p.OvercommitMem, c.wantOvercCPU, c.wantOvercMem)
			}
		})
	}
}

func TestComputeNodePressure_Percentages(t *testing.T) {
	cap := NodeCapacity{CPU: 2000, Memory: 4000}
	alloc := NodeAllocation{RequestsCPU: 1000, RequestsMem: 2000, LimitsCPU: 1500, LimitsMem: 3000}
	p := ComputeNodePressure(alloc, 500, 3520, cap)

	if p.RequestsCPUPct != 50 || p.RequestsMemPct != 50 {
		t.Fatalf("unexpected request pct: cpu=%v mem=%v", p.RequestsCPUPct, p.RequestsMemPct)
	}
	if p.LimitsCPUPct != 75 || p.LimitsMemPct != 75 {
		t.Fatalf("unexpected limit pct: cpu=%v mem=%v", p.LimitsCPUPct, p.LimitsMemPct)
	}
	if p.LiveCPUPct != 25 || p.LiveMemPct != 88 {
		t.Fatalf("unexpected live pct: cpu=%v mem=%v", p.LiveCPUPct, p.LiveMemPct)
	}
}

func TestComputeNodePressure_ZeroCapacity(t *testing.T) {
	p := ComputeNodePressure(NodeAllocation{RequestsCPU: 100}, 50, 0, NodeCapacity{})
	if p.RequestsCPUPct != 0 || p.LiveCPUPct != 0 {
		t.Fatalf("expected 0 pct against 0 capacity, got %+v", p)
	}
}

func TestOOMRisk(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.OOMRisk(480, nil) {
		t.Fatal("OOMRisk must be false with no memory limit set (that pod is Wild-West instead)")
	}
	if !cfg.OOMRisk(480, ptr(512)) { // 93.75% > 90%
		t.Fatal("expected OOM-Risk at 93.75% of limit")
	}
	if cfg.OOMRisk(400, ptr(512)) { // 78.1% < 90%
		t.Fatal("did not expect OOM-Risk at 78.1% of limit")
	}
	if cfg.OOMRisk(1, ptr(0)) {
		t.Fatal("OOMRisk must be false when limit is 0")
	}
}

func TestThrottlingRisk_RequiresConsecutivePolls(t *testing.T) {
	cfg := DefaultConfig()
	limit := ptr(int64(200))

	// A single spike among otherwise-normal samples must not trigger.
	if cfg.ThrottlingRisk([]int64{50, 195, 60}, limit) {
		t.Fatal("a single spike must not trigger throttling risk")
	}

	// Exactly ThrottlingConsecutivePolls (3) samples at/above threshold.
	if !cfg.ThrottlingRisk([]int64{195, 196, 200}, limit) {
		t.Fatal("3 consecutive samples >= 95% of limit should trigger throttling risk")
	}

	// Only 2 consecutive high samples, most recent one dips below threshold.
	if cfg.ThrottlingRisk([]int64{195, 196, 100}, limit) {
		t.Fatal("most recent sample below threshold should not trigger throttling risk")
	}

	// Fewer samples than required.
	if cfg.ThrottlingRisk([]int64{200, 200}, limit) {
		t.Fatal("fewer samples than ThrottlingConsecutivePolls should not trigger")
	}

	if cfg.ThrottlingRisk([]int64{200, 200, 200}, nil) {
		t.Fatal("no CPU limit set should never trigger throttling risk")
	}
}

func TestWasteful(t *testing.T) {
	cfg := DefaultConfig() // 70% threshold

	// p95 usage of 25 vs request of 100: usage is 75% below request -> wasteful.
	if !cfg.Wasteful(25, ptr(100)) {
		t.Fatal("expected wasteful: p95 usage is 75% below request")
	}

	// p95 usage of 40 vs request of 100: usage is 60% below request -> not wasteful (threshold 70%).
	if cfg.Wasteful(40, ptr(100)) {
		t.Fatal("did not expect wasteful: only 60% below request")
	}

	if cfg.Wasteful(0, nil) {
		t.Fatal("no request set must never be Wasteful (that pod is Wild-West instead)")
	}
}

func TestRecommendations(t *testing.T) {
	cfg := DefaultConfig() // headroom 10%, cpu x2.0, mem x1.5

	req := cfg.RecommendedRequest(100)
	if req != 110 {
		t.Fatalf("RecommendedRequest(100) = %d, want 110", req)
	}

	cpuLimit := cfg.RecommendedCPULimit(req)
	if cpuLimit != 220 {
		t.Fatalf("RecommendedCPULimit(110) = %d, want 220", cpuLimit)
	}

	memLimit := cfg.RecommendedMemLimit(req)
	if memLimit != 165 {
		t.Fatalf("RecommendedMemLimit(110) = %d, want 165", memLimit)
	}
}
