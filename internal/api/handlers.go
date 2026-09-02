package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/TaliaMarine/k8s-driller/internal/alerts"
	"github.com/TaliaMarine/k8s-driller/internal/auth"
	"github.com/TaliaMarine/k8s-driller/internal/pressure"
	"github.com/TaliaMarine/k8s-driller/internal/promclient"
	v1alpha1 "github.com/TaliaMarine/k8s-driller/pkg/apis/driller/v1alpha1"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// --- auth ---

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.SessionFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"subject": sess.Subject,
		"email":   sess.Email,
		"role":    sess.Role,
		"expires": sess.ExpiresAt,
	})
}

// --- cluster / nodes / pods (viewer) ---

func (s *Server) handleClusterSummary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.buildClusterSummary())
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.buildClusterSummary().Nodes)
}

func (s *Server) handleNodePods(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.buildNodePodDTOs(r.PathValue("name")))
}

func (s *Server) handlePodDetail(w http.ResponseWriter, r *http.Request) {
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	pod, ok := s.watch.Pod(namespace, name)
	if !ok {
		http.Error(w, "pod not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, s.buildPodDTO(pod))
}

// RecommendationDTO is the Prometheus-derived recommendation shown next to
// the Delta Visualizer (SPECS.md §2.3, §9).
type RecommendationDTO struct {
	P95CPU                int64 `json:"p95Cpu"`
	P95Mem                int64 `json:"p95Mem"`
	RecommendedRequestCPU int64 `json:"recommendedRequestCpu"`
	RecommendedLimitCPU   int64 `json:"recommendedLimitCpu"`
	RecommendedRequestMem int64 `json:"recommendedRequestMem"`
	RecommendedLimitMem   int64 `json:"recommendedLimitMem"`
	Wasteful              bool  `json:"wasteful"`
}

func (s *Server) handlePodRecommendation(w http.ResponseWriter, r *http.Request) {
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	pod, ok := s.watch.Pod(namespace, name)
	if !ok {
		http.Error(w, "pod not found", http.StatusNotFound)
		return
	}
	lookback := time.Duration(s.recommendationLookback) * time.Hour

	p95CPU, cpuOK, err := s.prom.P95PodCPU(r.Context(), namespace, name, lookback)
	if err != nil && err != promclient.ErrNotConfigured {
		http.Error(w, "prometheus query failed", http.StatusBadGateway)
		return
	}
	p95Mem, memOK, err := s.prom.P95PodMemory(r.Context(), namespace, name, lookback)
	if err != nil && err != promclient.ErrNotConfigured {
		http.Error(w, "prometheus query failed", http.StatusBadGateway)
		return
	}
	if !cpuOK || !memOK {
		http.Error(w, "prometheus unavailable or insufficient history", http.StatusNotFound)
		return
	}

	alloc := pressure.AggregatePod(pod.Containers)
	var reqCPU, reqMem *int64
	if alloc.RequestsCPU > 0 {
		v := alloc.RequestsCPU
		reqCPU = &v
	}
	if alloc.RequestsMem > 0 {
		v := alloc.RequestsMem
		reqMem = &v
	}

	recReqCPU := s.pressure.RecommendedRequest(p95CPU)
	recReqMem := s.pressure.RecommendedRequest(p95Mem)

	writeJSON(w, http.StatusOK, RecommendationDTO{
		P95CPU:                p95CPU,
		P95Mem:                p95Mem,
		RecommendedRequestCPU: recReqCPU,
		RecommendedLimitCPU:   s.pressure.RecommendedCPULimit(recReqCPU),
		RecommendedRequestMem: recReqMem,
		RecommendedLimitMem:   s.pressure.RecommendedMemLimit(recReqMem),
		Wasteful:              s.pressure.Wasteful(p95CPU, reqCPU) || s.pressure.Wasteful(p95Mem, reqMem),
	})
}

func (s *Server) handleNodeHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := s.watch.Node(name); !ok {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	end := time.Now()
	start := end.Add(-24 * time.Hour)

	// name is confirmed to be a real, already-known node name above, and is
	// additionally Go-quoted (compatible with PromQL's double-quoted string
	// escaping) before being interpolated — defense in depth against PromQL
	// injection even if a node name ever contained a quote character.
	instanceMatcher := fmt.Sprintf("%q", name+".*")
	cpuQuery := fmt.Sprintf(`sum(rate(node_cpu_seconds_total{mode!="idle",instance=~%s}[5m])) * 1000`, instanceMatcher)
	memQuery := fmt.Sprintf(`sum(node_memory_MemTotal_bytes{instance=~%s} - node_memory_MemAvailable_bytes{instance=~%s})`, instanceMatcher, instanceMatcher)

	cpu, err := s.prom.QueryRange(r.Context(), cpuQuery, start, end, 5*time.Minute)
	if err != nil && err != promclient.ErrNotConfigured {
		http.Error(w, "prometheus query failed", http.StatusBadGateway)
		return
	}
	mem, err := s.prom.QueryRange(r.Context(), memQuery, start, end, 5*time.Minute)
	if err != nil && err != promclient.ErrNotConfigured {
		http.Error(w, "prometheus query failed", http.StatusBadGateway)
		return
	}
	if cpu == nil && mem == nil {
		http.Error(w, "prometheus unavailable", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cpu": cpu, "memory": mem})
}

// --- SSE streams ---

func (s *Server) handleStreamCluster(w http.ResponseWriter, r *http.Request) {
	s.hub.ServeHTTP(w, r, "cluster")
}

func (s *Server) handleStreamNode(w http.ResponseWriter, r *http.Request) {
	s.hub.ServeHTTP(w, r, "node:"+r.PathValue("name"))
}

func (s *Server) handleStreamAlerts(w http.ResponseWriter, r *http.Request) {
	s.hub.ServeHTTP(w, r, "alerts")
}

// --- admin: users ---

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	roles, err := s.crds.ListUserRoles(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

func (s *Server) handleSetUserRole(w http.ResponseWriter, r *http.Request) {
	subject := r.PathValue("subject")
	var body struct {
		Role  v1alpha1.Role `json:"role"`
		Email string        `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Role != v1alpha1.RoleAdmin && body.Role != v1alpha1.RoleViewer {
		http.Error(w, "role must be admin or viewer", http.StatusBadRequest)
		return
	}

	updatedBy := "bootstrap-token"
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		updatedBy = sess.Subject
	}

	if err := s.crds.SetUserRole(r.Context(), subject, body.Email, body.Role, updatedBy, time.Now().Format(time.RFC3339)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- admin: alerts ---

func (s *Server) handleGetAlertConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.crds.GetAlertConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cfg == nil {
		writeJSON(w, http.StatusOK, v1alpha1.DrillerAlertConfigSpec{})
		return
	}
	writeJSON(w, http.StatusOK, cfg.Spec)
}

func (s *Server) handleSetAlertConfig(w http.ResponseWriter, r *http.Request) {
	var spec v1alpha1.DrillerAlertConfigSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.crds.SetAlertConfig(r.Context(), spec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestAlert(w http.ResponseWriter, r *http.Request) {
	err := s.alerts.Fire(r.Context(), "test-alert", alerts.Alert{
		Kind:    "test",
		Subject: "k8s-driller",
		Message: "This is a test alert from the Alert Settings screen.",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
