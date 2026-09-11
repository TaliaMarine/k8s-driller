package api

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"net/http"
	"sigs.k8s.io/yaml"
)

// podGVR/controllerGVRs are the resources the Details tab can fetch a raw
// manifest for (SPECS.md's read-only guarantee, §4.2: get only, never
// write) — every kind k8swatch.ControllerRef.Kind can already resolve to
// (see k8swatch.resolveController), plus the bare Pod itself. All are
// namespace-scoped.
var (
	podGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	controllerGVRs = map[string]schema.GroupVersionResource{
		"Deployment":  {Group: "apps", Version: "v1", Resource: "deployments"},
		"StatefulSet": {Group: "apps", Version: "v1", Resource: "statefulsets"},
		"DaemonSet":   {Group: "apps", Version: "v1", Resource: "daemonsets"},
		"ReplicaSet":  {Group: "apps", Version: "v1", Resource: "replicasets"},
		"Job":         {Group: "batch", Version: "v1", Resource: "jobs"},
		"CronJob":     {Group: "batch", Version: "v1", Resource: "cronjobs"},
	}
)

// ManifestDTO is one Kubernetes object's raw manifest, rendered as YAML for
// the pod detail Details tab.
type ManifestDTO struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	YAML string `json:"yaml"`
}

// PodManifestDTO is a pod's own manifest plus (when it has one) its owning
// controller's manifest — Controller is omitted entirely if the pod is bare
// or its controller couldn't be fetched (e.g. deleted concurrently).
type PodManifestDTO struct {
	Pod        ManifestDTO  `json:"pod"`
	Controller *ManifestDTO `json:"controller,omitempty"`
}

// fetchManifestYAML gets one namespaced object via the dynamic client and
// renders it as YAML text. metadata.managedFields is stripped first — it's
// pure server-side-apply bookkeeping, often longer than the rest of the
// manifest combined, and never something a viewer of this dashboard needs.
func (s *Server) fetchManifestYAML(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (string, error) {
	obj, err := s.dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	out, err := yaml.Marshal(obj.Object)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (s *Server) handlePodManifest(w http.ResponseWriter, r *http.Request) {
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	pod, ok := s.watch.Pod(namespace, name)
	if !ok {
		http.Error(w, "pod not found", http.StatusNotFound)
		return
	}

	podYAML, err := s.fetchManifestYAML(r.Context(), podGVR, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "pod not found", http.StatusNotFound)
			return
		}
		http.Error(w, "fetch pod manifest failed", http.StatusBadGateway)
		return
	}

	resp := PodManifestDTO{Pod: ManifestDTO{Kind: "Pod", Name: name, YAML: podYAML}}

	if pod.Controller != nil {
		controllerKind, controllerName := pod.Controller.Kind, pod.Controller.Name
		// k8swatch.resolveController already resolves a ReplicaSet-owned
		// pod to its Deployment via its in-memory rsOwner cache, but that
		// cache is timing-dependent (populated by a separate informer) and
		// falls back to the bare ReplicaSet if it hasn't caught up yet.
		// The Details tab always wants the Deployment, so — since this
		// handler already does live dynamic-client Gets — take one more
		// live hop here instead of trusting the cache for this specific
		// case.
		if controllerKind == "ReplicaSet" {
			if kind, name, ok := s.resolveReplicaSetOwner(r.Context(), namespace, controllerName); ok {
				controllerKind, controllerName = kind, name
			}
		}
		if gvr, ok := controllerGVRs[controllerKind]; ok {
			controllerYAML, err := s.fetchManifestYAML(r.Context(), gvr, namespace, controllerName)
			switch {
			case err == nil:
				resp.Controller = &ManifestDTO{Kind: controllerKind, Name: controllerName, YAML: controllerYAML}
			case !apierrors.IsNotFound(err):
				s.log.Warn("fetch controller manifest failed",
					"kind", controllerKind, "name", controllerName, "error", err)
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// resolveReplicaSetOwner fetches replicaSetName live and returns its own
// owning Deployment, if it has one — see handlePodManifest's ReplicaSet
// case above.
func (s *Server) resolveReplicaSetOwner(ctx context.Context, namespace, replicaSetName string) (kind, name string, ok bool) {
	obj, err := s.dynamic.Resource(controllerGVRs["ReplicaSet"]).Namespace(namespace).Get(ctx, replicaSetName, metav1.GetOptions{})
	if err != nil {
		return "", "", false
	}
	for _, owner := range obj.GetOwnerReferences() {
		if owner.Kind == "Deployment" {
			return "Deployment", owner.Name, true
		}
	}
	return "", "", false
}
