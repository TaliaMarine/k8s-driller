package api

// Recompute rebuilds the cluster/node view and pushes it to every SSE
// subscriber. Called after any informer event and on every metrics poll
// tick (SPECS.md §2 data flow). Each push is a full recomputed snapshot
// rather than a computed diff — cheap at the ~500 node / ~5,000 pod v1
// scale target (SPECS.md §10); revisit with real diffing only if profiling
// shows it's needed.
//
// Published as EventSnapshot (not EventPatch) even though it's a recurring
// push: sse.Topic only caches an EventSnapshot as lastSnapshot, and a new
// subscriber (a freshly opened dashboard/drilldown tab) gets that cached
// value immediately on connect instead of waiting for the next event —
// without this, page loads stalled until the next informer event or metrics
// poll tick (up to MetricsPollInterval).
func (s *Server) Recompute(reason string) {
	summary := s.buildClusterSummary()
	s.hub.PublishSnapshot("cluster", summary)

	var allPods []PodDTO
	for _, n := range summary.Nodes {
		dtos := s.buildNodePodDTOs(n.Name)
		s.hub.PublishSnapshot("node:"+n.Name, dtos)
		allPods = append(allPods, dtos...)
	}

	// Non-blocking: StartAlertWorker's single goroutine serializes actual
	// evaluation, so a burst of recomputes here can queue at most one
	// pending item — anything beyond that is dropped, since the item
	// already queued will re-evaluate against equally fresh state by the
	// time the worker gets to it.
	select {
	case s.alertWork <- alertWorkItem{summary: summary, pods: allPods}:
	default:
	}
}
