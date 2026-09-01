// Package appmetrics exposes k8s-driller's own health as Prometheus metrics
// (SPECS.md §10 "Observability of the app itself") — separate from the
// cluster metrics the app reads via metricsclient/promclient.
package appmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	InformerSynced = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "driller_informer_synced",
		Help: "1 once the node/pod informer caches have completed their initial sync, 0 otherwise.",
	})

	SSEClientsConnected = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "driller_sse_clients_connected",
		Help: "Number of currently connected SSE clients, by topic.",
	}, []string{"topic"})

	AlertDispatchTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "driller_alert_dispatch_total",
		Help: "Alert webhook dispatch attempts, by result.",
	}, []string{"result"})
)
