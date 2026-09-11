// Package promclient wraps the Prometheus HTTP API for the two things
// SPECS.md keeps on the history/recommendation side, never the live path
// (§2/§4.1): 24h p95 usage (feeds Wasteful detection and recommended
// request/limit, §9) and historical trend series for the secondary history
// view (§6.1 /api/v1/history). Nil-safe throughout: when Prometheus isn't
// configured, callers get ErrNotConfigured rather than a hard failure.
package promclient

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// ErrNotConfigured is returned by every method on a nil-baseURL Client, so
// callers can treat Prometheus-derived features as "unavailable" rather than
// erroring the whole request (SPECS.md §4.1, §10 Resilience).
var ErrNotConfigured = errors.New("prometheus not configured")

// Client queries Prometheus for pod-level p95 usage and trend series.
type Client struct {
	api promv1.API
}

// New builds a Client against baseURL, or a nil-safe stub Client when
// baseURL is empty (Prometheus integration disabled).
func New(baseURL string) (*Client, error) {
	if baseURL == "" {
		return &Client{}, nil
	}
	c, err := promapi.NewClient(promapi.Config{Address: baseURL})
	if err != nil {
		return nil, fmt.Errorf("create prometheus client: %w", err)
	}
	return &Client{api: promv1.NewAPI(c)}, nil
}

func (c *Client) configured() bool { return c.api != nil }

// P95PodCPU returns p95 CPU usage in millicores over lookback for one pod's
// containers combined, and whether enough history exists to compute it.
func (c *Client) P95PodCPU(ctx context.Context, namespace, pod string, lookback time.Duration) (millicores int64, ok bool, err error) {
	if !c.configured() {
		return 0, false, ErrNotConfigured
	}
	query := fmt.Sprintf(
		`quantile_over_time(0.95, sum(rate(container_cpu_usage_seconds_total{namespace=%q,pod=%q,container!="",container!="POD"}[5m]))[%s:5m]) * 1000`,
		namespace, pod, lookback.String(),
	)
	return c.scalarQuery(ctx, query)
}

// P95PodMemory returns p95 working-set memory usage in bytes over lookback
// for one pod's containers combined.
func (c *Client) P95PodMemory(ctx context.Context, namespace, pod string, lookback time.Duration) (bytes int64, ok bool, err error) {
	if !c.configured() {
		return 0, false, ErrNotConfigured
	}
	query := fmt.Sprintf(
		`quantile_over_time(0.95, sum(container_memory_working_set_bytes{namespace=%q,pod=%q,container!="",container!="POD"})[%s:5m])`,
		namespace, pod, lookback.String(),
	)
	return c.scalarQuery(ctx, query)
}

func (c *Client) scalarQuery(ctx context.Context, query string) (int64, bool, error) {
	value, _, err := c.api.Query(ctx, query, time.Time{})
	if err != nil {
		return 0, false, fmt.Errorf("prometheus query: %w", err)
	}
	vector, ok := value.(model.Vector)
	if !ok || len(vector) == 0 {
		return 0, false, nil
	}
	return int64(vector[0].Value), true, nil
}

// MaxAllPodsCPU returns the max_over_time CPU usage (millicores) of every
// pod Prometheus has data for, over lookback, in one bulk query rather than
// one per pod — used once at startup to seed usagecache's historical max
// before the first metrics-server poll has run. Keyed by "namespace/pod".
func (c *Client) MaxAllPodsCPU(ctx context.Context, lookback time.Duration) (map[string]int64, error) {
	if !c.configured() {
		return nil, ErrNotConfigured
	}
	query := fmt.Sprintf(
		`max_over_time(sum by (namespace,pod) (rate(container_cpu_usage_seconds_total{container!="",container!="POD"}[5m]))[%s:5m]) * 1000`,
		lookback.String(),
	)
	return c.vectorByPod(ctx, query)
}

// MaxAllPodsMemory returns the max_over_time working-set memory usage
// (bytes) of every pod Prometheus has data for, over lookback, in one bulk
// query. Keyed by "namespace/pod".
func (c *Client) MaxAllPodsMemory(ctx context.Context, lookback time.Duration) (map[string]int64, error) {
	if !c.configured() {
		return nil, ErrNotConfigured
	}
	query := fmt.Sprintf(
		`max_over_time(sum by (namespace,pod) (container_memory_working_set_bytes{container!="",container!="POD"})[%s:5m])`,
		lookback.String(),
	)
	return c.vectorByPod(ctx, query)
}

// vectorByPod runs an instant query expected to return one sample per
// namespace/pod pair and indexes the result by "namespace/pod".
func (c *Client) vectorByPod(ctx context.Context, query string) (map[string]int64, error) {
	value, _, err := c.api.Query(ctx, query, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("prometheus query: %w", err)
	}
	vector, ok := value.(model.Vector)
	if !ok {
		return nil, nil
	}
	out := make(map[string]int64, len(vector))
	for _, sample := range vector {
		namespace := string(sample.Metric["namespace"])
		pod := string(sample.Metric["pod"])
		if namespace == "" || pod == "" {
			continue
		}
		out[namespace+"/"+pod] = int64(sample.Value)
	}
	return out, nil
}

// PodGroupCPUUsageRange returns a merged CPU usage series (millicores) over
// [start, end] across every name in podNames — the pod being analyzed plus
// its currently-alive siblings sharing the same Deployment/ReplicaSet/
// StatefulSet/etc (internal/api's handlePodAnalysis) — for the Analysis
// tab's historical stats and recommendations (SPECS.md §9, extended with
// internal/analysis). A pod that's only lived a short time often has
// little history of its own; pooling siblings' history gives a fuller
// picture of how this workload actually behaves. Each pod's usage is kept
// as its own series (`sum by (pod)`, not a plain `sum()`) and then merged
// by timestamp — see queryRangeMerged — rather than summed together, since
// the goal is coverage of the timeline, not concurrent combined usage.
func (c *Client) PodGroupCPUUsageRange(ctx context.Context, namespace string, podNames []string, start, end time.Time, step time.Duration) ([]Sample, error) {
	query := fmt.Sprintf(
		`sum by (pod) (rate(container_cpu_usage_seconds_total{namespace=%q,pod=~%q,container!="",container!="POD"}[5m])) * 1000`,
		namespace, podNameRegex(podNames),
	)
	return c.queryRangeMerged(ctx, query, start, end, step)
}

// PodGroupMemoryUsageRange is PodGroupCPUUsageRange's memory counterpart.
func (c *Client) PodGroupMemoryUsageRange(ctx context.Context, namespace string, podNames []string, start, end time.Time, step time.Duration) ([]Sample, error) {
	query := fmt.Sprintf(
		`sum by (pod) (container_memory_working_set_bytes{namespace=%q,pod=~%q,container!="",container!="POD"})`,
		namespace, podNameRegex(podNames),
	)
	return c.queryRangeMerged(ctx, query, start, end, step)
}

// podNameRegex anchors and alternates a set of pod names into one RE2
// pattern for a PromQL `=~` matcher. Kubernetes pod names can't actually
// contain regex metacharacters, but each name is escaped anyway — defense
// in depth, matching the same posture as internal/api/handlers.go's node
// name interpolation into PromQL.
func podNameRegex(names []string) string {
	escaped := make([]string, len(names))
	for i, n := range names {
		escaped[i] = regexp.QuoteMeta(n)
	}
	return "^(" + strings.Join(escaped, "|") + ")$"
}

// queryRangeMerged runs a range query expected to return one series per
// some grouping label (e.g. one per pod, via `sum by (pod)`) and merges
// them into a single timeline: at each timestamp, whichever series has a
// sample there wins (arbitrarily, if more than one does), rather than
// being summed — the query already ran with one fixed start/end/step, so
// every returned series shares the exact same aligned timestamps and
// merging is just picking one value per timestamp.
func (c *Client) queryRangeMerged(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Sample, error) {
	if !c.configured() {
		return nil, ErrNotConfigured
	}
	value, _, err := c.api.QueryRange(ctx, query, promv1.Range{Start: start, End: end, Step: step})
	if err != nil {
		return nil, fmt.Errorf("prometheus range query: %w", err)
	}
	matrix, ok := value.(model.Matrix)
	if !ok || len(matrix) == 0 {
		return nil, nil
	}
	byTime := make(map[time.Time]float64)
	for _, series := range matrix {
		for _, v := range series.Values {
			byTime[v.Timestamp.Time()] = float64(v.Value)
		}
	}
	out := make([]Sample, 0, len(byTime))
	for ts, v := range byTime {
		out = append(out, Sample{Timestamp: ts, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	return out, nil
}

// Sample is one point in a historical trend series (SPECS.md §6.1 history
// endpoints).
type Sample struct {
	Timestamp time.Time
	Value     float64
}

// QueryRange runs an arbitrary PromQL range query, used to feed the
// secondary history/trend charts (SPECS.md §7.1 history view).
func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Sample, error) {
	if !c.configured() {
		return nil, ErrNotConfigured
	}
	value, _, err := c.api.QueryRange(ctx, query, promv1.Range{Start: start, End: end, Step: step})
	if err != nil {
		return nil, fmt.Errorf("prometheus range query: %w", err)
	}
	matrix, ok := value.(model.Matrix)
	if !ok || len(matrix) == 0 {
		return nil, nil
	}
	series := matrix[0]
	out := make([]Sample, 0, len(series.Values))
	for _, v := range series.Values {
		out = append(out, Sample{Timestamp: v.Timestamp.Time(), Value: float64(v.Value)})
	}
	return out, nil
}
