// Command driller is the k8s-driller backend: it watches the cluster it's
// deployed into, polls metrics-server for live usage, optionally queries
// Prometheus for history/recommendations, and serves the REST + SSE API
// described in SPECS.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/TaliaMarine/k8s-driller/internal/alerts"
	"github.com/TaliaMarine/k8s-driller/internal/api"
	"github.com/TaliaMarine/k8s-driller/internal/appmetrics"
	"github.com/TaliaMarine/k8s-driller/internal/auth"
	"github.com/TaliaMarine/k8s-driller/internal/config"
	"github.com/TaliaMarine/k8s-driller/internal/crdstore"
	"github.com/TaliaMarine/k8s-driller/internal/k8swatch"
	"github.com/TaliaMarine/k8s-driller/internal/metricsclient"
	"github.com/TaliaMarine/k8s-driller/internal/promclient"
	"github.com/TaliaMarine/k8s-driller/internal/runtimesecrets"
	"github.com/TaliaMarine/k8s-driller/internal/sse"
	"github.com/TaliaMarine/k8s-driller/internal/usagecache"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// staticDir holds the built frontend SPA (SPECS.md §3 Packaging: single
// image, backend serves the built assets). The production Dockerfile copies
// frontend/dist here; if the directory doesn't exist (e.g. running the
// backend alone during development) the static handler is simply skipped.
const staticDir = "frontend/dist"

// informerResync is how often informers re-list as a correctness backstop
// on top of watch events; short enough to recover from a missed watch event
// without meaningfully adding load.
const informerResync = 10 * time.Minute

// throttlingHistorySize must be at least ThrottlingConsecutivePolls; a
// little headroom lets the threshold be tuned without also touching this.
const throttlingHistorySize = 10

// recomputeDebounce coalesces a burst of informer events (e.g. a
// few-hundred-pod rollout) into a single Recompute instead of one per event
// — see k8swatch.New's doc comment for the production incident this fixes.
const recomputeDebounce = 500 * time.Millisecond

// clientQPS/clientBurst replace client-go's conservative defaults (5 QPS /
// burst 10), which are sized for a single-purpose client, not a cluster-wide
// watcher across nodes, pods, deployments, replicasets, statefulsets, and
// daemonsets. 50/100 is a typical starting point for that workload.
const (
	clientQPS   = 50
	clientBurst = 100
)

// secretRandomBytes is how much raw entropy runtimesecrets.Ensure generates
// for the session signing key and admin bootstrap token when auto-creating
// them — 32 bytes (256 bits) comfortably exceeds what HMAC-SHA256 needs.
const secretRandomBytes = 32

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	restConfig, err := buildKubeConfig()
	if err != nil {
		return err
	}
	restConfig.QPS = clientQPS
	restConfig.Burst = clientBurst

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	// A distinct client (and therefore a distinct client-go rate limiter)
	// from dynamicClient, used only by the alert-config informer's
	// relist/watch traffic — see crdstore.StartAlertConfigWatch — so that
	// background work can never again queue behind (or ahead of)
	// interactive dynamicClient calls like GetUserRoleBySubject.
	alertWatchRestConfig := rest.CopyConfig(restConfig)
	alertWatchClient, err := dynamic.NewForConfig(alertWatchRestConfig)
	if err != nil {
		return err
	}
	metricsSet, err := metricsclientset.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	crds := crdstore.New(dynamicClient)
	if err := crds.StartAlertConfigWatch(ctx, alertWatchClient, informerResync); err != nil {
		return err
	}

	// Resolved here, not by the chart: SPECS.md §4.2 explains why these two
	// Secrets are app-owned rather than Helm-generated.
	sessionKey, err := runtimesecrets.Ensure(ctx, clientset, cfg.Namespace,
		cfg.SessionKeySecret.Name, cfg.SessionKeySecret.Key, cfg.SessionKeySecret.AutoCreate, secretRandomBytes)
	if err != nil {
		return fmt.Errorf("resolve session signing key: %w", err)
	}
	adminToken, err := runtimesecrets.Ensure(ctx, clientset, cfg.Namespace,
		cfg.AdminTokenSecret.Name, cfg.AdminTokenSecret.Key, cfg.AdminTokenSecret.AutoCreate, secretRandomBytes)
	if err != nil {
		return fmt.Errorf("resolve admin bootstrap token: %w", err)
	}

	sessions := auth.NewSessionManager(sessionKey)
	authenticator, err := auth.NewAuthenticator(ctx, cfg.OIDC.IssuerURL, cfg.OIDC.ClientID, cfg.OIDC.ClientSecret, cfg.OIDC.RedirectURL, sessions, crds)
	if err != nil {
		return err
	}

	prom, err := promclient.New(cfg.PrometheusBaseURL)
	if err != nil {
		return err
	}

	hub := sse.New(15 * time.Second)
	alertDispatcher := alerts.New(clientset, crds, cfg.Namespace)
	usage := usagecache.New(throttlingHistorySize)

	// srv is referenced by the k8swatch onChange callback below, so it's
	// declared before the Store that will call it and assigned once built.
	var srv *api.Server
	watchStore := k8swatch.New(clientset, informerResync, recomputeDebounce, func(reason string) {
		if srv != nil {
			srv.Recompute(reason)
		}
	})

	srv = api.NewServer(api.Deps{
		Watch:                       watchStore,
		Usage:                       usage,
		Prom:                        prom,
		CRDs:                        crds,
		Sessions:                    sessions,
		AuthN:                       authenticator,
		Hub:                         hub,
		Alerts:                      alertDispatcher,
		Pressure:                    cfg.Pressure,
		AdminBootstrapToken:         string(adminToken),
		RecommendationLookbackHours: cfg.RecommendationLookbackHours,
		Log:                         log,
	})

	if err := watchStore.Start(ctx); err != nil {
		return err
	}
	appmetrics.InformerSynced.Set(1)
	log.Info("informer caches synced")

	srv.StartAlertWorker(ctx)

	metricsClient := metricsclient.New(metricsSet)
	go pollMetrics(ctx, log, metricsClient, usage, srv, cfg.MetricsPollInterval)

	mux := srv.Routes()
	mux.Handle("/metrics", promhttp.Handler())
	mountStaticFrontend(mux, log)

	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Info("listening", "addr", cfg.HTTPAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// buildKubeConfig uses in-cluster config, per SPECS.md §1.2 (single cluster,
// in-cluster ServiceAccount) — the sole exception is falling back to
// $KUBECONFIG for local development outside a cluster.
func buildKubeConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func pollMetrics(ctx context.Context, log *slog.Logger, client metricsclient.Client, usage *usagecache.Cache, srv *api.Server, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nodes, err := client.NodeMetrics(ctx)
			if err != nil {
				log.Warn("node metrics poll failed", "error", err)
				continue
			}
			pods, err := client.PodMetrics(ctx)
			if err != nil {
				log.Warn("pod metrics poll failed", "error", err)
				continue
			}
			usage.Update(nodes, pods)
			srv.Recompute("metrics poll")
		}
	}
}

func mountStaticFrontend(mux *http.ServeMux, log *slog.Logger) {
	info, err := os.Stat(staticDir)
	if err != nil || !info.IsDir() {
		log.Warn("static frontend directory not found, serving API only", "dir", staticDir)
		return
	}
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))
}
