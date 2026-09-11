// Package usagecache holds the latest metrics-server poll results in memory
// and a short rolling history of per-pod CPU usage, which is all the
// ThrottlingRisk rule needs ("sustained for at least 3 consecutive polls",
// SPECS.md §9) without reaching for Prometheus on the live path.
package usagecache

import (
	"sync"

	"github.com/TaliaMarine/k8s-driller/internal/metricsclient"
)

// Cache is safe for concurrent reads from API handlers while the poll loop
// writes to it.
type Cache struct {
	mu         sync.RWMutex
	maxHistory int

	nodes map[string]metricsclient.NodeUsage
	pods  map[string]podAggregate
}

type podAggregate struct {
	cpu        int64 // latest aggregate CPU usage (millicores), summed across containers
	mem        int64 // latest aggregate memory usage (bytes), summed across containers
	cpuHistory []int64
	maxCPU     int64 // historical peak of cpu, ever observed since this pod first appeared here
	maxMem     int64 // historical peak of mem, same lifetime as maxCPU
}

func New(maxHistory int) *Cache {
	return &Cache{
		maxHistory: maxHistory,
		nodes:      make(map[string]metricsclient.NodeUsage),
		pods:       make(map[string]podAggregate),
	}
}

// Update replaces node usage wholesale and folds the latest pod usage into
// each pod's rolling CPU history. Pods absent from this poll (deleted or
// not yet scraped) are dropped, so history doesn't grow unbounded across pod
// churn.
func (c *Cache) Update(nodes []metricsclient.NodeUsage, pods []metricsclient.PodUsage) {
	nodeMap := make(map[string]metricsclient.NodeUsage, len(nodes))
	for _, n := range nodes {
		nodeMap[n.Node] = n
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.nodes = nodeMap

	next := make(map[string]podAggregate, len(pods))
	for _, p := range pods {
		key := p.Namespace + "/" + p.Name
		var cpu, mem int64
		for _, ctr := range p.Containers {
			cpu += ctr.CPU
			mem += ctr.Memory
		}
		prev := c.pods[key]
		history := prev.cpuHistory
		history = append(history, cpu)
		if len(history) > c.maxHistory {
			history = history[len(history)-c.maxHistory:]
		}
		maxCPU := prev.maxCPU
		if cpu > maxCPU {
			maxCPU = cpu
		}
		maxMem := prev.maxMem
		if mem > maxMem {
			maxMem = mem
		}
		next[key] = podAggregate{cpu: cpu, mem: mem, cpuHistory: history, maxCPU: maxCPU, maxMem: maxMem}
	}
	c.pods = next
}

// SeedMax pre-populates a pod's historical peak CPU/memory before any
// metrics-server poll has run — called once at startup with a Prometheus
// max_over_time lookback, so a freshly (re)started backend doesn't forget
// weeks of prior peak usage. A no-op in whichever direction the given value
// isn't actually higher than what's already recorded, so seeding can run
// safely regardless of whether it happens before or after the first
// Update() (e.g. pod already has a live-polled max by the time Prometheus
// history for it comes back).
func (c *Cache) SeedMax(namespace, name string, maxCPU, maxMem int64) {
	key := namespace + "/" + name
	c.mu.Lock()
	defer c.mu.Unlock()
	agg := c.pods[key]
	if maxCPU > agg.maxCPU {
		agg.maxCPU = maxCPU
	}
	if maxMem > agg.maxMem {
		agg.maxMem = maxMem
	}
	c.pods[key] = agg
}

// PodMax returns one pod's historical peak CPU (millicores) and memory
// (bytes) usage recorded since it was first observed, either from live
// polls or seeded from Prometheus at startup — the reference value behind
// the pod detail Charts tab's Max bar.
func (c *Cache) PodMax(namespace, name string) (maxCPU, maxMem int64, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	agg, found := c.pods[namespace+"/"+name]
	if !found {
		return 0, 0, false
	}
	return agg.maxCPU, agg.maxMem, true
}

// Node returns the latest usage for one node.
func (c *Cache) Node(name string) (metricsclient.NodeUsage, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	u, ok := c.nodes[name]
	return u, ok
}

// Nodes returns a copy of every node's latest usage.
func (c *Cache) Nodes() map[string]metricsclient.NodeUsage {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]metricsclient.NodeUsage, len(c.nodes))
	for k, v := range c.nodes {
		out[k] = v
	}
	return out
}

// PodUsage returns one pod's latest aggregate usage and its CPU history
// (oldest-first, as pressure.Config.ThrottlingRisk expects).
func (c *Cache) PodUsage(namespace, name string) (cpu, mem int64, cpuHistory []int64, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	agg, found := c.pods[namespace+"/"+name]
	if !found {
		return 0, 0, nil, false
	}
	historyCopy := make([]int64, len(agg.cpuHistory))
	copy(historyCopy, agg.cpuHistory)
	return agg.cpu, agg.mem, historyCopy, true
}
