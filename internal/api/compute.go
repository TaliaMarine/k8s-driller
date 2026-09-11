package api

import (
	"sort"
	"time"

	"github.com/TaliaMarine/k8s-driller/internal/k8swatch"
	"github.com/TaliaMarine/k8s-driller/internal/pressure"
)

// activePods filters out terminal pods, e.g. a finished Job/CronJob pod
// still lingering until garbage collection — it holds no resource
// reservation and including it would double-count nothing but noise into
// every node's allocation totals, PodCount, and the drilldown pod list.
// Tombstoned pods (DeletedAt set, see k8swatch.Store.deletePod) are
// excluded here too, for the same reason — the Distribution view's
// dedicated builders below are the one deliberate exception that wants
// them (within a short grace period, see withinDeadPodGracePeriod).
func activePods(pods []k8swatch.PodInfo) []k8swatch.PodInfo {
	out := make([]k8swatch.PodInfo, 0, len(pods))
	for _, p := range pods {
		if !k8swatch.IsTerminalPhase(p.Phase) && p.DeletedAt == nil {
			out = append(out, p)
		}
	}
	return out
}

// withinDeadPodGracePeriod keeps a pod that's no longer live only if it
// became so (TerminalAt) within k8swatch.DeadPodGracePeriod — a completed
// Job pod otherwise lingers in Kubernetes (and therefore in this view)
// until GC, which can be far longer than that grace period, flooding the
// Distribution honeycomb with pods that stopped being interesting a while
// ago. Pods that never went terminal (TerminalAt nil) pass through
// unaffected; deleted pods are handled separately by
// k8swatch.Store.PruneDeleted actually removing them from the store once
// their own grace period elapses.
func withinDeadPodGracePeriod(pods []k8swatch.PodInfo) []k8swatch.PodInfo {
	out := make([]k8swatch.PodInfo, 0, len(pods))
	for _, p := range pods {
		if p.TerminalAt != nil && time.Since(*p.TerminalAt) > k8swatch.DeadPodGracePeriod {
			continue
		}
		out = append(out, p)
	}
	return out
}

// sortPodsByCreation orders oldest-first, for the Distribution honeycomb
// (SPECS.md §7.1: pods laid out in the order they were created).
func sortPodsByCreation(pods []k8swatch.PodInfo) {
	sort.Slice(pods, func(i, j int) bool {
		ti, tj := pods[i].CreationTimestamp, pods[j].CreationTimestamp
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})
}

func (s *Server) buildPodDTO(p k8swatch.PodInfo) PodDTO {
	alloc := pressure.AggregatePod(p.Containers)
	usageCPU, usageMem, cpuHistory, _ := s.usage.PodUsage(p.Namespace, p.Name)

	var maxUsageCPU, maxUsageMem *int64
	if maxCPU, maxMem, ok := s.usage.PodMax(p.Namespace, p.Name); ok {
		maxUsageCPU, maxUsageMem = &maxCPU, &maxMem
	}

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
		MaxUsageCPU:    maxUsageCPU,
		MaxUsageMem:    maxUsageMem,
		Teams:          extractTeams(p.Labels),
		Ready:          p.Ready,
		Deleted:        p.DeletedAt != nil,
		CreationTime:   p.CreationTimestamp,
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
		Unschedulable:    n.Unschedulable,
		CapacityCPU:      n.Capacity.CPU,
		CapacityMemory:   n.Capacity.Memory,
		Pressure:         p,
		Health:           nodeHealth(n.Ready, n.Unschedulable, p),
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

// buildNodePodDistributionDTOs returns every pod k8swatch still knows about
// for this node — including ones that aren't Ready — sorted oldest-first
// by creation time, but only within k8swatch.DeadPodGracePeriod of no
// longer being live: a deleted pod is pruned from the store itself after
// that window (k8swatch.Store.PruneDeleted), and withinDeadPodGracePeriod
// filters out a terminal (Succeeded/Failed) pod once it's been that long
// since it finished, even though Kubernetes itself may keep it around
// much longer pending GC. This deliberately sees more than buildNodePodDTOs:
// the Distribution honeycomb (SPECS.md §7.1) is the one view that wants to
// show not-Ready pods (gray) and recently-dead ones (darker gray) instead
// of silently excluding them like every pressure/allocation total and pod
// list elsewhere in the app does — just not forever.
func (s *Server) buildNodePodDistributionDTOs(nodeName string) []PodDTO {
	pods := withinDeadPodGracePeriod(s.watch.PodsOnNode(nodeName))
	sortPodsByCreation(pods)
	dtos := make([]PodDTO, 0, len(pods))
	for _, p := range pods {
		dtos = append(dtos, s.buildPodDTO(p))
	}
	return dtos
}

// buildAllPodDistributionDTOs is the cluster-wide counterpart to
// buildNodePodDistributionDTOs, sorted oldest-first across every node (not
// per-node-then-concatenated, which would only be locally sorted).
func (s *Server) buildAllPodDistributionDTOs() []PodDTO {
	var pods []k8swatch.PodInfo
	for _, n := range s.watch.Nodes() {
		pods = append(pods, s.watch.PodsOnNode(n.Name)...)
	}
	pods = withinDeadPodGracePeriod(pods)
	sortPodsByCreation(pods)
	dtos := make([]PodDTO, 0, len(pods))
	for _, p := range pods {
		dtos = append(dtos, s.buildPodDTO(p))
	}
	return dtos
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
