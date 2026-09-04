// Package api implements the REST + SSE surface in SPECS.md §6, wiring the
// k8swatch topology store, the metrics-server usage cache, the pressure
// engine, the CRD store, and the SSE hub together.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/TaliaMarine/k8s-driller/internal/alerts"
	"github.com/TaliaMarine/k8s-driller/internal/auth"
	"github.com/TaliaMarine/k8s-driller/internal/crdstore"
	"github.com/TaliaMarine/k8s-driller/internal/k8swatch"
	"github.com/TaliaMarine/k8s-driller/internal/pressure"
	"github.com/TaliaMarine/k8s-driller/internal/promclient"
	"github.com/TaliaMarine/k8s-driller/internal/sse"
	"github.com/TaliaMarine/k8s-driller/internal/usagecache"
	v1alpha1 "github.com/TaliaMarine/k8s-driller/pkg/apis/driller/v1alpha1"
)

// Server holds every dependency the HTTP handlers need. It has no state of
// its own beyond references — the actual cluster state lives in watch and
// usage.
type Server struct {
	watch    *k8swatch.Store
	usage    *usagecache.Cache
	prom     *promclient.Client
	crds     *crdstore.Store
	sessions *auth.SessionManager
	authN    *auth.Authenticator
	hub      *sse.Hub
	alerts   *alerts.Dispatcher
	pressure pressure.Config

	adminBootstrapToken    string
	recommendationLookback int // hours
	log                    *slog.Logger

	// alertWork feeds the single background worker started by
	// StartAlertWorker. Depth 1, and Recompute's send is non-blocking (see
	// push.go): a burst of recomputes should evaluate alerts at most once
	// more after the worker's current pass finishes, never queue up one
	// evaluation per recompute. That bound is what stops the unbounded
	// per-event goroutine growth that contributed to an OOMKill in
	// production — each pending evaluation held a full ClusterSummaryDTO
	// plus every PodDTO.
	alertWork chan alertWorkItem
}

type alertWorkItem struct {
	summary ClusterSummaryDTO
	pods    []PodDTO
}

type Deps struct {
	Watch                       *k8swatch.Store
	Usage                       *usagecache.Cache
	Prom                        *promclient.Client
	CRDs                        *crdstore.Store
	Sessions                    *auth.SessionManager
	AuthN                       *auth.Authenticator
	Hub                         *sse.Hub
	Alerts                      *alerts.Dispatcher
	Pressure                    pressure.Config
	AdminBootstrapToken         string
	RecommendationLookbackHours int
	Log                         *slog.Logger
}

func NewServer(d Deps) *Server {
	return &Server{
		watch:                  d.Watch,
		usage:                  d.Usage,
		prom:                   d.Prom,
		crds:                   d.CRDs,
		sessions:               d.Sessions,
		authN:                  d.AuthN,
		hub:                    d.Hub,
		alerts:                 d.Alerts,
		pressure:               d.Pressure,
		adminBootstrapToken:    d.AdminBootstrapToken,
		recommendationLookback: d.RecommendationLookbackHours,
		log:                    d.Log,
		alertWork:              make(chan alertWorkItem, 1),
	}
}

// StartAlertWorker runs the single goroutine that evaluates alerts,
// serializing every request from Recompute so evaluation work can never
// grow unbounded under a burst of recomputes (see the alertWork field
// comment). Call once at startup.
func (s *Server) StartAlertWorker(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case item := <-s.alertWork:
				s.evaluateAlerts(ctx, item.summary, item.pods)
			}
		}
	}()
}

// Routes builds the HTTP router (SPECS.md §6). Uses Go's net/http
// ServeMux method+pattern routing (Go 1.22+) rather than a router
// dependency — the route set is small and stable enough not to need one.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)

	mux.HandleFunc("GET /api/v1/auth/login", s.authN.LoginHandler)
	mux.HandleFunc("GET /api/v1/auth/callback", s.authN.CallbackHandler)
	mux.HandleFunc("POST /api/v1/auth/logout", s.authN.LogoutHandler)
	mux.HandleFunc("GET /api/v1/auth/me", s.sessions.RequireAuth(s.handleAuthMe))

	mux.HandleFunc("GET /api/v1/cluster/summary", s.sessions.RequireAuth(s.handleClusterSummary))
	mux.HandleFunc("GET /api/v1/nodes", s.sessions.RequireAuth(s.handleListNodes))
	mux.HandleFunc("GET /api/v1/nodes/{name}/pods", s.sessions.RequireAuth(s.handleNodePods))
	mux.HandleFunc("GET /api/v1/pods", s.sessions.RequireAuth(s.handleListPods))
	mux.HandleFunc("GET /api/v1/pods/{namespace}/{name}", s.sessions.RequireAuth(s.handlePodDetail))
	mux.HandleFunc("GET /api/v1/pods/{namespace}/{name}/recommendation", s.sessions.RequireAuth(s.handlePodRecommendation))
	mux.HandleFunc("GET /api/v1/pods/{namespace}/{name}/analysis", s.sessions.RequireAuth(s.handlePodAnalysis))
	mux.HandleFunc("GET /api/v1/history/nodes/{name}", s.sessions.RequireAuth(s.handleNodeHistory))

	mux.HandleFunc("GET /api/v1/stream/cluster", s.sessions.RequireAuth(s.handleStreamCluster))
	mux.HandleFunc("GET /api/v1/stream/nodes/{name}", s.sessions.RequireAuth(s.handleStreamNode))
	mux.HandleFunc("GET /api/v1/stream/workloads", s.sessions.RequireAuth(s.handleStreamWorkloads))
	mux.HandleFunc("GET /api/v1/stream/alerts", s.sessions.RequireAuth(s.handleStreamAlerts))

	mux.HandleFunc("GET /api/v1/admin/users", s.sessions.RequireRole(v1alpha1.RoleAdmin, s.handleListUsers))
	mux.HandleFunc("PUT /api/v1/admin/users/{subject}/role", s.sessions.AdminOrBootstrap(s.adminBootstrapToken, s.handleSetUserRole))
	mux.HandleFunc("GET /api/v1/admin/alerts/config", s.sessions.RequireRole(v1alpha1.RoleAdmin, s.handleGetAlertConfig))
	mux.HandleFunc("PUT /api/v1/admin/alerts/config", s.sessions.RequireRole(v1alpha1.RoleAdmin, s.handleSetAlertConfig))
	mux.HandleFunc("POST /api/v1/admin/alerts/test", s.sessions.RequireRole(v1alpha1.RoleAdmin, s.handleTestAlert))

	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
