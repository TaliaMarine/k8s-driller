// Package metricsclient wraps the metrics.k8s.io API (metrics-server) for
// live node/pod CPU and memory usage — the primary "reality" signal in
// SPECS.md §2.1/§9. This is the one polled path in the architecture; there is
// no watch API for point-in-time usage.
package metricsclient

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// NodeUsage is one node's live CPU (millicores) and memory (bytes) usage.
type NodeUsage struct {
	Node   string
	CPU    int64
	Memory int64
}

// ContainerUsage is one container's live CPU (millicores) and memory (bytes)
// usage within a pod.
type ContainerUsage struct {
	Container string
	CPU       int64
	Memory    int64
}

// PodUsage is one pod's live usage, broken out per container so callers can
// compare against per-container limits (SPECS.md §2.3 Delta Visualizer).
type PodUsage struct {
	Namespace  string
	Name       string
	Containers []ContainerUsage
}

// Client fetches live usage from metrics-server. Real is the only production
// implementation; the interface exists so callers (and their tests) don't
// depend on a live cluster.
type Client interface {
	NodeMetrics(ctx context.Context) ([]NodeUsage, error)
	PodMetrics(ctx context.Context) ([]PodUsage, error)
}

type client struct {
	clientset metricsclientset.Interface
}

// New builds a Client backed by a real metrics.k8s.io clientset.
func New(clientset metricsclientset.Interface) Client {
	return &client{clientset: clientset}
}

func (c *client) NodeMetrics(ctx context.Context) ([]NodeUsage, error) {
	list, err := c.clientset.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list node metrics: %w", err)
	}
	out := make([]NodeUsage, 0, len(list.Items))
	for _, m := range list.Items {
		out = append(out, NodeUsage{
			Node:   m.Name,
			CPU:    m.Usage.Cpu().MilliValue(),
			Memory: m.Usage.Memory().Value(),
		})
	}
	return out, nil
}

func (c *client) PodMetrics(ctx context.Context) ([]PodUsage, error) {
	list, err := c.clientset.MetricsV1beta1().PodMetricses(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pod metrics: %w", err)
	}
	out := make([]PodUsage, 0, len(list.Items))
	for _, m := range list.Items {
		out = append(out, PodUsage{
			Namespace:  m.Namespace,
			Name:       m.Name,
			Containers: containerUsages(m.Containers),
		})
	}
	return out, nil
}

func containerUsages(containers []metricsv1beta1.ContainerMetrics) []ContainerUsage {
	out := make([]ContainerUsage, 0, len(containers))
	for _, c := range containers {
		out = append(out, ContainerUsage{
			Container: c.Name,
			CPU:       c.Usage.Cpu().MilliValue(),
			Memory:    c.Usage.Memory().Value(),
		})
	}
	return out
}
