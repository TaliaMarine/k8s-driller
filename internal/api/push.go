package api

import "context"

// Recompute rebuilds the cluster/node view and pushes it to every SSE
// subscriber. Called after any informer event and on every metrics poll
// tick (SPECS.md §2 data flow). Each push is a full recomputed snapshot
// rather than a computed diff — cheap at the ~500 node / ~5,000 pod v1
// scale target (SPECS.md §10); revisit with real diffing only if profiling
// shows it's needed.
func (s *Server) Recompute(reason string) {
	summary := s.buildClusterSummary()
	s.hub.PublishPatch("cluster", summary)

	var allPods []PodDTO
	for _, n := range summary.Nodes {
		pods := s.watch.PodsOnNode(n.Name)
		dtos := make([]PodDTO, 0, len(pods))
		for _, p := range pods {
			dtos = append(dtos, s.buildPodDTO(p))
		}
		s.hub.PublishPatch("node:"+n.Name, dtos)
		allPods = append(allPods, dtos...)
	}

	go s.evaluateAlerts(context.Background(), summary, allPods)
}
