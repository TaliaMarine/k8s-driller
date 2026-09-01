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
