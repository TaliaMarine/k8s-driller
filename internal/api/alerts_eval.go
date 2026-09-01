package api

import (
	"context"
	"fmt"

	"github.com/TaliaMarine/k8s-driller/internal/alerts"
)

// evaluateAlerts checks the just-recomputed state against the configured
// thresholds (SPECS.md §5.2) and fires or clears each condition's debounce
// record. Called from Recompute in its own goroutine so a slow webhook
// target never delays the SSE push.
func (s *Server) evaluateAlerts(ctx context.Context, summary ClusterSummaryDTO, pods []PodDTO) {
	cfg, err := s.crds.GetAlertConfig(ctx)
	if err != nil || cfg == nil {
		return
	}
	th := cfg.Spec.Thresholds

	fireOrClear := func(cond bool, key, kind, subject, message string) {
		if cond {
			_ = s.alerts.Fire(ctx, key, alerts.Alert{Kind: kind, Subject: subject, Message: message})
		} else {
			s.alerts.Clear(key)
		}
	}

	for _, n := range summary.Nodes {
		fireOrClear(n.Pressure.LiveMemPct > th.NodeMemLivePct,
			"node-mem:"+n.Name, "node-mem-pressure", n.Name,
			fmt.Sprintf("live memory at %.1f%% of capacity", n.Pressure.LiveMemPct))

		fireOrClear(n.Pressure.LiveCPUPct > th.NodeCPULivePct,
			"node-cpu:"+n.Name, "node-cpu-pressure", n.Name,
			fmt.Sprintf("live CPU at %.1f%% of capacity", n.Pressure.LiveCPUPct))

		if th.OvercommitEnabled {
			fireOrClear(n.Pressure.OvercommitCPU, "overcommit-cpu:"+n.Name, "overcommit", n.Name, "CPU limits exceed node capacity")
			fireOrClear(n.Pressure.OvercommitMem, "overcommit-mem:"+n.Name, "overcommit", n.Name, "memory limits exceed node capacity")
		}
	}

	for _, p := range pods {
		subject := p.Namespace + "/" + p.Name
		if th.OOMRiskEnabled {
			fireOrClear(p.OOMRisk, "oom-risk:"+subject, "oom-risk", subject, "live memory usage exceeds the OOM-Risk threshold of its limit")
		}
		if th.ThrottlingRiskEnabled {
			fireOrClear(p.ThrottlingRisk, "throttling-risk:"+subject, "throttling-risk", subject, "CPU usage has sustained at the throttling threshold of its limit")
		}
	}
}
