package api

import (
	"sort"

	"github.com/TaliaMarine/k8s-driller/internal/k8swatch"
	"github.com/TaliaMarine/k8s-driller/internal/pressure"
)

// isTerminalPhase reports whether a pod has permanently stopped running —
// Succeeded or Failed — and therefore no longer holds any resource
// reservation on its node (the kubelet releases it once every container has
// exited for good). Pending is deliberately NOT terminal here: a pod is
// already counted against the node's allocatable capacity from the moment
// it's scheduled/bound, before its containers actually start, so excluding
// Pending would understate allocation exactly when it matters most — a
// burst of pods being scheduled at once.
func isTerminalPhase(phase string) bool {
	return phase == "Succeeded" || phase == "Failed"
}

// activePods filters out terminal pods, e.g. a finished Job/CronJob pod
// still lingering until garbage collection — it holds no resource
// reservation and including it would double-count nothing but noise into
// every node's allocation totals, PodCount, and the drilldown pod list.
func activePods(pods []k8swatch.PodInfo) []k8swatch.PodInfo {
	out := make([]k8swatch.PodInfo, 0, len(pods))
	for _, p := range pods {
		if !isTerminalPhase(p.Phase) {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) buildPodDTO(p k8swatch.PodInfo) PodDTO {
	alloc := pressure.AggregatePod(p.Containers)
	usageCPU, usageMem, cpuHistory, _ := s.usage.PodUsage(p.Namespace, p.Name)

	wildWest := false
	for _, c := range p.Containers {
		if pressure.DetectWildWest(c).Any() {
			wildWest = true
			break
		}
	}

	var limitCPU, limitMem *int64
	if alloc.LimitsCPU > 0 {
		v := alloc.LimitsCPU
		limitCPU = &v
	}
	if alloc.LimitsMem > 0 {
		v := alloc.LimitsMem
		limitMem = &v
	}

	return PodDTO{
		Namespace:      p.Namespace,
		Name:           p.Name,
		NodeName:       p.NodeName,
		Phase:          p.Phase,
		Controller:     controllerDTO(p.Controller),
		Containers:     toContainerDTOs(p.ContainerNames, p.Containers),
		UsageCPU:       usageCPU,
		UsageMem:       usageMem,
		WildWest:       wildWest,
		OOMRisk:        s.pressure.OOMRisk(usageMem, limitMem),
		ThrottlingRisk: s.pressure.ThrottlingRisk(cpuHistory, limitCPU),
	}
}

// podOverRequests reports whether a pod's live usage exceeds its own
// requested CPU or memory — the same "is this pod already eating into its
// neighbors' headroom" signal the drilldown page's isOverRequest computes
// client-side, surfaced here so the dashboard card can show a count without
// shipping every pod's usage down to the browser.
func (s *Server) podOverRequests(p k8swatch.PodInfo) bool {
	alloc := pressure.AggregatePod(p.Containers)
	usageCPU, usageMem, _, _ := s.usage.PodUsage(p.Namespace, p.Name)
	if alloc.RequestsCPU > 0 && usageCPU > alloc.RequestsCPU {
		return true
	}
	if alloc.RequestsMem > 0 && usageMem > alloc.RequestsMem {
		return true
	}
	return false
}

func (s *Server) buildNodeDTO(n k8swatch.NodeInfo) NodeDTO {
	pods := activePods(s.watch.PodsOnNode(n.Name))
	allocations := make([]pressure.PodAllocation, 0, len(pods))
	overRequests := 0
	for _, p := range pods {
		allocations = append(allocations, pressure.AggregatePod(p.Containers))
		if s.podOverRequests(p) {
			overRequests++
		}
	}
	alloc := pressure.AggregateNode(allocations)

	var liveCPU, liveMem int64
	if u, ok := s.usage.Node(n.Name); ok {
		liveCPU, liveMem = u.CPU, u.Memory
	}

	p := pressure.ComputeNodePressure(alloc, liveCPU, liveMem, n.Capacity)

	return NodeDTO{
		Name:             n.Name,
		Ready:            n.Ready,
		CapacityCPU:      n.Capacity.CPU,
		CapacityMemory:   n.Capacity.Memory,
		Pressure:         p,
		Health:           nodeHealth(p),
		PodCount:         len(pods),
		PodsOverRequests: overRequests,
	}
}

// buildNodePodDTOs fetches every pod on a node and returns their DTOs in a
// stable order (namespace, then pod name). k8swatch.Store.PodsOnNode backs
// onto a Go map internally, so its iteration order is randomized on every
// call — without sorting here, the pod list (and therefore the frontend's
// rendering, including which expansion panels stay open) reshuffles on
// every single SSE push.
func (s *Server) buildNodePodDTOs(nodeName string) []PodDTO {
	pods := activePods(s.watch.PodsOnNode(nodeName))
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})
	dtos := make([]PodDTO, 0, len(pods))
	for _, p := range pods {
		dtos = append(dtos, s.buildPodDTO(p))
	}
	return dtos
}

// buildAllPodDTOs returns every active pod across every node, sorted node
// by node (each buildNodePodDTOs call is already namespace/name-sorted
// within it) — the Workloads view's cluster-wide counterpart to
// buildNodePodDTOs.
func (s *Server) buildAllPodDTOs() []PodDTO {
	nodes := s.watch.Nodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	out := make([]PodDTO, 0)
	for _, n := range nodes {
		out = append(out, s.buildNodePodDTOs(n.Name)...)
	}
	return out
}

func (s *Server) buildClusterSummary() ClusterSummaryDTO {
	nodes := s.watch.Nodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	nodeDTOs := make([]NodeDTO, 0, len(nodes))

	var totalCapCPU, totalCapMem, totalReqCPU, totalReqMem, totalLiveCPU, totalLiveMem int64
	for _, n := range nodes {
		dto := s.buildNodeDTO(n)
		nodeDTOs = append(nodeDTOs, dto)

		totalCapCPU += dto.CapacityCPU
		totalCapMem += dto.CapacityMemory
		totalReqCPU += int64(dto.Pressure.RequestsCPUPct / 100 * float64(dto.CapacityCPU))
		totalReqMem += int64(dto.Pressure.RequestsMemPct / 100 * float64(dto.CapacityMemory))
		totalLiveCPU += int64(dto.Pressure.LiveCPUPct / 100 * float64(dto.CapacityCPU))
		totalLiveMem += int64(dto.Pressure.LiveMemPct / 100 * float64(dto.CapacityMemory))
	}

	pct := func(used, total int64) float64 {
		if total <= 0 {
			return 0
		}
		return float64(used) / float64(total) * 100
	}

	return ClusterSummaryDTO{
		Nodes:               nodeDTOs,
		TotalCapacityCPU:    totalCapCPU,
		TotalCapacityMem:    totalCapMem,
		TotalRequestsCPUPct: pct(totalReqCPU, totalCapCPU),
		TotalRequestsMemPct: pct(totalReqMem, totalCapMem),
		TotalLiveCPUPct:     pct(totalLiveCPU, totalCapCPU),
		TotalLiveMemPct:     pct(totalLiveMem, totalCapMem),
	}
}
