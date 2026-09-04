package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/TaliaMarine/k8s-driller/internal/alerts"
	"github.com/TaliaMarine/k8s-driller/internal/analysis"
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
		"name":    sess.Name,
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

// analysisMaxDays caps the Analysis tab's lookback window (SPECS.md §9's
// history/recommendation logic, extended). A month is enough to catch
// weekly cycles without the query fanning out over an unbounded range.
const analysisMaxDays = 30

// SampleDTO is one point in a historical usage series, used both for the
// Analysis tab's chart and the raw data included in the AI export.
type SampleDTO struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// PodAnalysisDTO is the Analysis tab's full payload: raw historical series,
// summary statistics, and the derived request/limit recommendations
// (internal/analysis).
type PodAnalysisDTO struct {
	RangeStart    time.Time `json:"rangeStart"`
	RangeEnd      time.Time `json:"rangeEnd"`
	RequestedDays int       `json:"requestedDays"`
	// AvailableDays is how much history Prometheus actually had, which can
	// be less than RequestedDays on a freshly deployed Prometheus.
	AvailableDays float64 `json:"availableDays"`

	CPUSamples []SampleDTO    `json:"cpuSamples"`
	MemSamples []SampleDTO    `json:"memSamples"`
	CPUStats   analysis.Stats `json:"cpuStats"`
	MemStats   analysis.Stats `json:"memStats"`

	CurrentRequestCPU *int64 `json:"currentRequestCpu,omitempty"`
	CurrentLimitCPU   *int64 `json:"currentLimitCpu,omitempty"`
	CurrentRequestMem *int64 `json:"currentRequestMem,omitempty"`
	CurrentLimitMem   *int64 `json:"currentLimitMem,omitempty"`

	CPURecommendation analysis.Recommendation `json:"cpuRecommendation"`
	MemRecommendation analysis.Recommendation `json:"memRecommendation"`

	Wasteful         bool `json:"wasteful"`
	UnderProvisioned bool `json:"underProvisioned"`
}

func sampleValues(samples []promclient.Sample) []float64 {
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = s.Value
	}
	return out
}

func sampleTimestamps(samples []promclient.Sample) []time.Time {
	out := make([]time.Time, len(samples))
	for i, s := range samples {
		out[i] = s.Timestamp
	}
	return out
}

func toSampleDTOs(samples []promclient.Sample) []SampleDTO {
	out := make([]SampleDTO, len(samples))
	for i, s := range samples {
		out[i] = SampleDTO{T: s.Timestamp, V: s.Value}
	}
	return out
}

func (s *Server) handlePodAnalysis(w http.ResponseWriter, r *http.Request) {
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	pod, ok := s.watch.Pod(namespace, name)
	if !ok {
		http.Error(w, "pod not found", http.StatusNotFound)
		return
	}

	days := analysisMaxDays
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= analysisMaxDays {
			days = n
		}
	}

	end := time.Now()
	start := end.Add(-time.Duration(days) * 24 * time.Hour)
	const step = 15 * time.Minute

	cpuSamples, err := s.prom.PodCPUUsageRange(r.Context(), namespace, name, start, end, step)
	if err != nil && err != promclient.ErrNotConfigured {
		http.Error(w, "prometheus query failed", http.StatusBadGateway)
		return
	}
	memSamples, err := s.prom.PodMemoryUsageRange(r.Context(), namespace, name, start, end, step)
	if err != nil && err != promclient.ErrNotConfigured {
		http.Error(w, "prometheus query failed", http.StatusBadGateway)
		return
	}
	if len(cpuSamples) == 0 && len(memSamples) == 0 {
		http.Error(w, "prometheus unavailable or no history in this window", http.StatusNotFound)
		return
	}

	cpuStats := analysis.Compute(sampleValues(cpuSamples))
	memStats := analysis.Compute(sampleValues(memSamples))

	// Peak-bucket memory before recommending off it (see RecommendMemory):
	// daily buckets normally, hourly when there isn't even two days of
	// history yet to bucket daily.
	peakBucket := 24 * time.Hour
	if len(memSamples) > 0 && end.Sub(memSamples[0].Timestamp) < 2*24*time.Hour {
		peakBucket = time.Hour
	}
	memPeakStats := analysis.Compute(analysis.PeakBucket(sampleTimestamps(memSamples), sampleValues(memSamples), peakBucket))

	alloc := pressure.AggregatePod(pod.Containers)
	var reqCPU, limitCPU, reqMem, limitMem *int64
	if alloc.RequestsCPU > 0 {
		v := alloc.RequestsCPU
		reqCPU = &v
	}
	if alloc.LimitsCPU > 0 {
		v := alloc.LimitsCPU
		limitCPU = &v
	}
	if alloc.RequestsMem > 0 {
		v := alloc.RequestsMem
		reqMem = &v
	}
	if alloc.LimitsMem > 0 {
		v := alloc.LimitsMem
		limitMem = &v
	}

	cpuRec := analysis.RecommendCPU(cpuStats, s.pressure.RecommendationHeadroomPct, s.pressure.CPULimitMultiplier)
	memRec := analysis.RecommendMemory(memPeakStats, s.pressure.RecommendationHeadroomPct, s.pressure.MemLimitMultiplier)

	wasteful := s.pressure.Wasteful(int64(cpuStats.P95), reqCPU) || s.pressure.Wasteful(int64(memStats.P95), reqMem)
	underProvisioned := (reqCPU != nil && cpuRec.RecommendedRequest > int64(float64(*reqCPU)*1.2)) ||
		(reqMem != nil && memRec.RecommendedRequest > int64(float64(*reqMem)*1.2))

	availableDays := float64(days)
	switch {
	case len(memSamples) > 0:
		availableDays = end.Sub(memSamples[0].Timestamp).Hours() / 24
	case len(cpuSamples) > 0:
		availableDays = end.Sub(cpuSamples[0].Timestamp).Hours() / 24
	}

	writeJSON(w, http.StatusOK, PodAnalysisDTO{
		RangeStart:        start,
		RangeEnd:          end,
		RequestedDays:     days,
		AvailableDays:     availableDays,
		CPUSamples:        toSampleDTOs(cpuSamples),
		MemSamples:        toSampleDTOs(memSamples),
		CPUStats:          cpuStats,
		MemStats:          memStats,
		CurrentRequestCPU: reqCPU,
		CurrentLimitCPU:   limitCPU,
		CurrentRequestMem: reqMem,
		CurrentLimitMem:   limitMem,
		CPURecommendation: cpuRec,
		MemRecommendation: memRec,
		Wasteful:          wasteful,
		UnderProvisioned:  underProvisioned,
	})
}

func (s *Server) handleListPods(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.buildAllPodDTOs())
}

func (s *Server) handleStreamWorkloads(w http.ResponseWriter, r *http.Request) {
	s.hub.ServeHTTP(w, r, "workloads")
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
