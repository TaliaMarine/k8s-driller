package api

import (
	"github.com/TaliaMarine/k8s-driller/internal/k8swatch"
	"github.com/TaliaMarine/k8s-driller/internal/pressure"
)

// Health labels shown on a node card (SPECS.md §7.1). Node-level "pressure"
// (as opposed to the exact OOM-Risk/Throttling-Risk pod states in §9) is
// presentation logic, not one of the formulas in §9, so its threshold is a
// simple local constant rather than a Helm-configurable value.
const nodeLivePressureThresholdPct = 90

const (
	NodeHealthy     = "Healthy"
	NodeCPUPressure = "CPU Pressure"
	NodeMemPressure = "Mem Pressure"
	NodeOvercommit  = "Overcommit"
)

// ContainerDTO is one container's configured resources, live usage share,
// and Wild-West flags — the raw material for the Delta Visualizer (SPECS.md
// §2.3).
type ContainerDTO struct {
	Name        string                     `json:"name"`
	RequestsCPU *int64                     `json:"requestsCpu,omitempty"`
	RequestsMem *int64                     `json:"requestsMem,omitempty"`
	LimitsCPU   *int64                     `json:"limitsCpu,omitempty"`
	LimitsMem   *int64                     `json:"limitsMem,omitempty"`
	WildWest    pressure.ContainerWildWest `json:"wildWest"`
}

// ControllerRefDTO is the owning workload controller shown for grouping
// (SPECS.md §2.2).
type ControllerRefDTO struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PodDTO is one pod's full picture: spec, live usage, and computed pressure
// states (SPECS.md §2.3).
type PodDTO struct {
	Namespace      string            `json:"namespace"`
	Name           string            `json:"name"`
	NodeName       string            `json:"nodeName"`
	Phase          string            `json:"phase"`
	Controller     *ControllerRefDTO `json:"controller,omitempty"`
	Containers     []ContainerDTO    `json:"containers"`
	UsageCPU       int64             `json:"usageCpu"`
	UsageMem       int64             `json:"usageMem"`
	WildWest       bool              `json:"wildWest"`
	OOMRisk        bool              `json:"oomRisk"`
	ThrottlingRisk bool              `json:"throttlingRisk"`
}

// NodeDTO is one node card's worth of data (SPECS.md §2.1/§7.1).
type NodeDTO struct {
	Name           string                `json:"name"`
	Ready          bool                  `json:"ready"`
	CapacityCPU    int64                 `json:"capacityCpu"`
	CapacityMemory int64                 `json:"capacityMemory"`
	Pressure       pressure.NodePressure `json:"pressure"`
	Health         string                `json:"health"`
	PodCount       int                   `json:"podCount"`
}

// ClusterSummaryDTO is the top-of-dashboard totals bar plus every node
// (SPECS.md §3 Main Dashboard View).
type ClusterSummaryDTO struct {
	Nodes               []NodeDTO `json:"nodes"`
	TotalCapacityCPU    int64     `json:"totalCapacityCpu"`
	TotalCapacityMem    int64     `json:"totalCapacityMem"`
	TotalRequestsCPUPct float64   `json:"totalRequestsCpuPct"`
	TotalRequestsMemPct float64   `json:"totalRequestsMemPct"`
	TotalLiveCPUPct     float64   `json:"totalLiveCpuPct"`
	TotalLiveMemPct     float64   `json:"totalLiveMemPct"`
}

func nodeHealth(p pressure.NodePressure) string {
	switch {
	case p.OvercommitCPU || p.OvercommitMem:
		return NodeOvercommit
	case p.LiveMemPct > nodeLivePressureThresholdPct:
		return NodeMemPressure
	case p.LiveCPUPct > nodeLivePressureThresholdPct:
		return NodeCPUPressure
	default:
		return NodeHealthy
	}
}

func toContainerDTOs(names []string, resources []pressure.ContainerResources) []ContainerDTO {
	out := make([]ContainerDTO, len(resources))
	for i, r := range resources {
		out[i] = ContainerDTO{
			Name:        names[i],
			RequestsCPU: r.RequestsCPU,
			RequestsMem: r.RequestsMem,
			LimitsCPU:   r.LimitsCPU,
			LimitsMem:   r.LimitsMem,
			WildWest:    pressure.DetectWildWest(r),
		}
	}
	return out
}

func controllerDTO(ref *k8swatch.ControllerRef) *ControllerRefDTO {
	if ref == nil {
		return nil
	}
	return &ControllerRefDTO{Kind: ref.Kind, Name: ref.Name}
}
