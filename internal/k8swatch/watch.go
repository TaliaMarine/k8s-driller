// Package k8swatch keeps an in-memory, always-current view of cluster
// topology (nodes, pods, and pod ownership) using informers, so the backend
// reacts to actual API server events instead of polling (SPECS.md §2.1/§4.1
// data flow). Live usage numbers are not part of this package — those come
// from internal/metricsclient on its own poll loop, since there's no watch
// API for point-in-time metrics.
package k8swatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/TaliaMarine/k8s-driller/internal/pressure"
)

// ControllerRef identifies the owning workload controller of a pod, resolved
// through ReplicaSet -> Deployment where applicable (SPECS.md §2.2 Workload
// Grouping).
type ControllerRef struct {
	Kind string
	Name string
}

// DeadPodGracePeriod is how long a pod that's no longer live — deleted from
// the cluster (DeletedAt) or terminally completed (TerminalAt, e.g. a
// finished Job pod) — stays visible on the Distribution view before being
// treated as gone. Kubernetes itself can leave a completed/failed pod
// around far longer than this (GC, successfulJobsHistoryLimit, etc.), and a
// deleted pod's informer tombstone only needs a brief grace period, so both
// share one short, deliberately app-level cutoff rather than either
// depending on however long the cluster happens to keep the object around.
const DeadPodGracePeriod = 60 * time.Second

// IsTerminalPhase reports whether a pod has permanently stopped running —
// Succeeded or Failed — and therefore no longer holds any resource
// reservation on its node (the kubelet releases it once every container has
// exited for good). Pending is deliberately NOT terminal here: a pod is
// already counted against the node's allocatable capacity from the moment
// it's scheduled/bound, before its containers actually start, so excluding
// Pending would understate allocation exactly when it matters most — a
// burst of pods being scheduled at once.
func IsTerminalPhase(phase string) bool {
	return phase == "Succeeded" || phase == "Failed"
}

// PodInfo is everything the pressure engine and API layer need about one
// pod's spec (not its live usage).
type PodInfo struct {
	Namespace         string
	Name              string
	NodeName          string
	Phase             string
	Ready             bool // pod's own Ready condition — distinct from Phase (e.g. Running but failing readiness)
	CreationTimestamp time.Time
	// TerminalAt is set once, the first time this pod is observed in a
	// terminal phase (IsTerminalPhase) — e.g. a finished Job pod — and never
	// updated again after that, so it marks when the pod stopped being live
	// rather than being reset by every subsequent informer resync of the
	// same terminal pod. Distribution-view builders use it to stop showing
	// a completed pod after DeadPodGracePeriod, instead of however long
	// Kubernetes itself takes to actually garbage-collect it.
	TerminalAt *time.Time
	// DeletedAt is nil while the pod still exists in the cluster. Once the
	// informer sees it deleted, deletePod sets this instead of dropping the
	// entry immediately, so the Distribution view's honeycomb can keep
	// showing it — darker gray — for a short grace period (see
	// Store.PruneDeleted) instead of it vanishing without a trace the
	// moment it's gone.
	DeletedAt      *time.Time
	Controller     *ControllerRef // nil for a bare pod with no owning controller
	ContainerNames []string
	Containers     []pressure.ContainerResources // same order as ContainerNames
	Labels         map[string]string
}

// NodeInfo is a node's identity and allocatable capacity.
type NodeInfo struct {
	Name          string
	Capacity      pressure.NodeCapacity
	Ready         bool
	Unschedulable bool // node.Spec.Unschedulable — set by `kubectl cordon` or during drain
}

// OnChangeFunc is invoked after any add/update/delete that could affect
// computed pressure state, so the caller can push an SSE patch.
type OnChangeFunc func(reason string)

// Store is a thread-safe, informer-backed snapshot of cluster topology.
type Store struct {
	mu sync.RWMutex

	nodes   map[string]NodeInfo      // key: node name
	pods    map[string]PodInfo       // key: namespace/name
	rsOwner map[string]ControllerRef // key: namespace/replicaset -> owning Deployment

	onChange      OnChangeFunc
	debounce      time.Duration
	debounceMu    sync.Mutex
	debounceTimer *time.Timer
	factory       informers.SharedInformerFactory
}

// New builds a Store and registers informer event handlers. Call Start to
// begin watching.
//
// debounce coalesces onChange calls: a burst of informer events (e.g. a
// few-hundred-pod rollout) triggers exactly one onChange, debounce after the
// last event, instead of one per event. Without this, every single pod
// add/update/delete triggered a full Recompute (internal/api/push.go),
// which itself spawns an alert-evaluation pass — on a cluster with heavy
// churn this produced an unbounded burst of concurrent work. Pass 0 to
// disable coalescing and call onChange synchronously, as before.
func New(clientset kubernetes.Interface, resync time.Duration, debounce time.Duration, onChange OnChangeFunc) *Store {
	s := &Store{
		nodes:    make(map[string]NodeInfo),
		pods:     make(map[string]PodInfo),
		rsOwner:  make(map[string]ControllerRef),
		onChange: onChange,
		debounce: debounce,
		factory:  informers.NewSharedInformerFactory(clientset, resync),
	}

	nodeInformer := s.factory.Core().V1().Nodes().Informer()
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { s.upsertNode(obj); s.notify("node added") },
		UpdateFunc: func(_, obj interface{}) { s.upsertNode(obj); s.notify("node updated") },
		DeleteFunc: func(obj interface{}) { s.deleteNode(obj); s.notify("node deleted") },
	})

	rsInformer := s.factory.Apps().V1().ReplicaSets().Informer()
	rsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { s.upsertReplicaSetOwner(obj) },
		UpdateFunc: func(_, obj interface{}) { s.upsertReplicaSetOwner(obj) },
		DeleteFunc: func(obj interface{}) { s.deleteReplicaSetOwner(obj) },
	})

	podInformer := s.factory.Core().V1().Pods().Informer()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { s.upsertPod(obj); s.notify("pod added") },
		UpdateFunc: func(_, obj interface{}) { s.upsertPod(obj); s.notify("pod updated") },
		DeleteFunc: func(obj interface{}) { s.deletePod(obj); s.notify("pod deleted") },
	})

	return s
}

func (s *Store) notify(reason string) {
	if s.onChange == nil {
		return
	}
	if s.debounce <= 0 {
		s.onChange(reason)
		return
	}
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()
	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
	}
	s.debounceTimer = time.AfterFunc(s.debounce, func() { s.onChange(reason) })
}

// Start begins the informer factory and blocks until stopCh is closed or the
// initial cache sync fails.
func (s *Store) Start(ctx context.Context) error {
	s.factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), s.factory.Apps().V1().ReplicaSets().Informer().HasSynced) {
		return fmt.Errorf("k8swatch: replicaset informer cache sync failed")
	}
	if !cache.WaitForCacheSync(
		ctx.Done(),
		s.factory.Core().V1().Nodes().Informer().HasSynced,
		s.factory.Core().V1().Pods().Informer().HasSynced,
	) {
		return fmt.Errorf("k8swatch: informer cache sync failed")
	}
	return nil
}

func (s *Store) upsertNode(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return
	}
	ready := false
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			ready = true
			break
		}
	}
	info := NodeInfo{
		Name: node.Name,
		Capacity: pressure.NodeCapacity{
			CPU:    node.Status.Allocatable.Cpu().MilliValue(),
			Memory: node.Status.Allocatable.Memory().Value(),
		},
		Ready:         ready,
		Unschedulable: node.Spec.Unschedulable,
	}
	s.mu.Lock()
	s.nodes[node.Name] = info
	s.mu.Unlock()
}

func (s *Store) deleteNode(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		if tomb, isTomb := obj.(cache.DeletedFinalStateUnknown); isTomb {
			node, ok = tomb.Obj.(*corev1.Node)
		}
		if !ok {
			return
		}
	}
	s.mu.Lock()
	delete(s.nodes, node.Name)
	s.mu.Unlock()
}

func (s *Store) upsertReplicaSetOwner(obj interface{}) {
	rs, ok := obj.(interface {
		GetName() string
		GetNamespace() string
		GetOwnerReferences() []metav1.OwnerReference
	})
	if !ok {
		return
	}
	key := rs.GetNamespace() + "/" + rs.GetName()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, owner := range rs.GetOwnerReferences() {
		if owner.Kind == "Deployment" {
			s.rsOwner[key] = ControllerRef{Kind: "Deployment", Name: owner.Name}
			return
		}
	}
	delete(s.rsOwner, key)
}

func (s *Store) deleteReplicaSetOwner(obj interface{}) {
	rs, ok := obj.(interface {
		GetName() string
		GetNamespace() string
	})
	if !ok {
		if tomb, isTomb := obj.(cache.DeletedFinalStateUnknown); isTomb {
			rs, ok = tomb.Obj.(interface {
				GetName() string
				GetNamespace() string
			})
		}
		if !ok {
			return
		}
	}
	s.mu.Lock()
	delete(s.rsOwner, rs.GetNamespace()+"/"+rs.GetName())
	s.mu.Unlock()
}

func (s *Store) resolveController(pod *corev1.Pod) *ControllerRef {
	for _, owner := range pod.OwnerReferences {
		switch owner.Kind {
		case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob":
			return &ControllerRef{Kind: owner.Kind, Name: owner.Name}
		case "ReplicaSet":
			s.mu.RLock()
			deployment, ok := s.rsOwner[pod.Namespace+"/"+owner.Name]
			s.mu.RUnlock()
			if ok {
				return &deployment
			}
			return &ControllerRef{Kind: "ReplicaSet", Name: owner.Name}
		}
	}
	return nil
}

func toContainerResources(containers []corev1.Container) ([]string, []pressure.ContainerResources) {
	names := make([]string, len(containers))
	resources := make([]pressure.ContainerResources, len(containers))
	for i, c := range containers {
		names[i] = c.Name
		resources[i] = pressure.ContainerResources{
			RequestsCPU: quantityPtr(c.Resources.Requests, corev1.ResourceCPU, true),
			RequestsMem: quantityPtr(c.Resources.Requests, corev1.ResourceMemory, false),
			LimitsCPU:   quantityPtr(c.Resources.Limits, corev1.ResourceCPU, true),
			LimitsMem:   quantityPtr(c.Resources.Limits, corev1.ResourceMemory, false),
		}
	}
	return names, resources
}

// quantityPtr returns nil when the resource key is absent from the list —
// distinct from present-but-zero — matching the Wild-West rule in
// pressure.DetectWildWest.
func quantityPtr(list corev1.ResourceList, name corev1.ResourceName, milli bool) *int64 {
	q, ok := list[name]
	if !ok {
		return nil
	}
	var v int64
	if milli {
		v = q.MilliValue()
	} else {
		v = q.Value()
	}
	return &v
}

func podReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (s *Store) upsertPod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	names, resources := toContainerResources(pod.Spec.Containers)
	phase := string(pod.Status.Phase)
	key := pod.Namespace + "/" + pod.Name

	// Resolved before taking the write lock below — for a ReplicaSet-owned
	// pod (i.e. every Deployment-managed pod) this takes s.mu's read lock
	// internally, and sync.RWMutex isn't reentrant: doing that while
	// already holding the write lock would deadlock this goroutine (and
	// with it every subsequent pod/node/replicaset informer event, since
	// they share one handler goroutine per informer).
	controller := s.resolveController(pod)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Preserve an already-set TerminalAt across every subsequent
	// informer resync of the same terminal pod — only the first
	// observation should start the clock.
	terminalAt := s.pods[key].TerminalAt
	if terminalAt == nil && IsTerminalPhase(phase) {
		now := time.Now()
		terminalAt = &now
	}

	info := PodInfo{
		Namespace:         pod.Namespace,
		Name:              pod.Name,
		NodeName:          pod.Spec.NodeName,
		Phase:             phase,
		Ready:             podReady(pod),
		CreationTimestamp: pod.CreationTimestamp.Time,
		TerminalAt:        terminalAt,
		Controller:        controller,
		ContainerNames:    names,
		Containers:        resources,
		Labels:            pod.Labels,
	}
	s.pods[key] = info
}

// deletePod marks the pod deleted rather than removing it outright — see
// PodInfo.DeletedAt — so it stays visible (as a "just terminated" tombstone)
// on the Distribution view for a short grace period. Store.PruneDeleted is
// what actually removes it once that period elapses.
func (s *Store) deletePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		if tomb, isTomb := obj.(cache.DeletedFinalStateUnknown); isTomb {
			pod, ok = tomb.Obj.(*corev1.Pod)
		}
		if !ok {
			return
		}
	}
	key := pod.Namespace + "/" + pod.Name
	s.mu.Lock()
	defer s.mu.Unlock()
	info, found := s.pods[key]
	if !found {
		return
	}
	now := time.Now()
	info.DeletedAt = &now
	s.pods[key] = info
}

// PruneDeleted removes tombstoned pods (see deletePod) whose grace period
// has elapsed, so pods deleted from the cluster don't linger in memory
// forever just because the Distribution view briefly shows them. Terminal
// (Succeeded/Failed) pods that Kubernetes hasn't deleted yet are left in
// place — they're still real, queryable objects — the Distribution view's
// builders filter those by TerminalAt age instead (internal/api/compute.go)
// rather than this removing them from the store itself.
func (s *Store) PruneDeleted() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, info := range s.pods {
		if info.DeletedAt != nil && now.Sub(*info.DeletedAt) > DeadPodGracePeriod {
			delete(s.pods, key)
		}
	}
}

// Node returns one node by name, or false if not found.
func (s *Store) Node(name string) (NodeInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[name]
	return n, ok
}

// Nodes returns a snapshot of every known node.
func (s *Store) Nodes() []NodeInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]NodeInfo, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, n)
	}
	return out
}

// PodsOnNode returns every pod currently scheduled on nodeName.
func (s *Store) PodsOnNode(nodeName string) []PodInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PodInfo, 0)
	for _, p := range s.pods {
		if p.NodeName == nodeName {
			out = append(out, p)
		}
	}
	return out
}

// Pod returns one pod by namespace/name, or false if not found.
func (s *Store) Pod(namespace, name string) (PodInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.pods[namespace+"/"+name]
	return p, ok
}

// PodsByController returns every currently-known pod in namespace sharing
// controller as its owner (nil returns none) — a pod's siblings from the
// same Deployment/ReplicaSet/StatefulSet/etc, used to seed a fuller
// Prometheus history/max for a pod that hasn't itself been alive long
// (internal/api's handlePodAnalysis and seedUsageMaxFromProm in
// cmd/driller). Only reflects pods k8swatch currently knows about (live,
// plus the brief post-deletion tombstone window) — it has no memory of
// pod names from long-past rollouts.
func (s *Store) PodsByController(namespace string, controller *ControllerRef) []PodInfo {
	if controller == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PodInfo, 0)
	for _, p := range s.pods {
		if p.Namespace == namespace && p.Controller != nil &&
			p.Controller.Kind == controller.Kind && p.Controller.Name == controller.Name {
			out = append(out, p)
		}
	}
	return out
}
