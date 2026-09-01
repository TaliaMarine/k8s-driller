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
		history := c.pods[key].cpuHistory
		history = append(history, cpu)
		if len(history) > c.maxHistory {
			history = history[len(history)-c.maxHistory:]
		}
		next[key] = podAggregate{cpu: cpu, mem: mem, cpuHistory: history}
	}
	c.pods = next
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
