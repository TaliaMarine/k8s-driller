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

// PodInfo is everything the pressure engine and API layer need about one
// pod's spec (not its live usage).
type PodInfo struct {
	Namespace      string
	Name           string
	NodeName       string
	Phase          string
	Controller     *ControllerRef // nil for a bare pod with no owning controller
	ContainerNames []string
	Containers     []pressure.ContainerResources // same order as ContainerNames
}

// NodeInfo is a node's identity and allocatable capacity.
type NodeInfo struct {
	Name     string
	Capacity pressure.NodeCapacity
	Ready    bool
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

	onChange OnChangeFunc
	factory  informers.SharedInformerFactory
}

// New builds a Store and registers informer event handlers. Call Start to
// begin watching.
func New(clientset kubernetes.Interface, resync time.Duration, onChange OnChangeFunc) *Store {
	s := &Store{
		nodes:    make(map[string]NodeInfo),
		pods:     make(map[string]PodInfo),
		rsOwner:  make(map[string]ControllerRef),
		onChange: onChange,
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
	if s.onChange != nil {
		s.onChange(reason)
	}
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
		Ready: ready,
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

func (s *Store) upsertPod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	names, resources := toContainerResources(pod.Spec.Containers)
	info := PodInfo{
		Namespace:      pod.Namespace,
		Name:           pod.Name,
		NodeName:       pod.Spec.NodeName,
		Phase:          string(pod.Status.Phase),
		Controller:     s.resolveController(pod),
		ContainerNames: names,
		Containers:     resources,
	}
	s.mu.Lock()
	s.pods[pod.Namespace+"/"+pod.Name] = info
	s.mu.Unlock()
}

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
	s.mu.Lock()
	delete(s.pods, pod.Namespace+"/"+pod.Name)
	s.mu.Unlock()
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
