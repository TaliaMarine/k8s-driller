package api

import (
	"github.com/TaliaMarine/k8s-driller/internal/k8swatch"
	"github.com/TaliaMarine/k8s-driller/internal/pressure"
)

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

func (s *Server) buildNodeDTO(n k8swatch.NodeInfo) NodeDTO {
	pods := s.watch.PodsOnNode(n.Name)
	allocations := make([]pressure.PodAllocation, 0, len(pods))
	for _, p := range pods {
		allocations = append(allocations, pressure.AggregatePod(p.Containers))
	}
	alloc := pressure.AggregateNode(allocations)

	var liveCPU, liveMem int64
	if u, ok := s.usage.Node(n.Name); ok {
		liveCPU, liveMem = u.CPU, u.Memory
	}

	p := pressure.ComputeNodePressure(alloc, liveCPU, liveMem, n.Capacity)

	return NodeDTO{
		Name:           n.Name,
		Ready:          n.Ready,
		CapacityCPU:    n.Capacity.CPU,
		CapacityMemory: n.Capacity.Memory,
		Pressure:       p,
		Health:         nodeHealth(p),
		PodCount:       len(pods),
	}
}

func (s *Server) buildClusterSummary() ClusterSummaryDTO {
	nodes := s.watch.Nodes()
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
