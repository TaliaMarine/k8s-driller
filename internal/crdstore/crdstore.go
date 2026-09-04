// Package crdstore is the only persistence layer k8s-driller has: CRUD
// against the driller.dev custom resources (SPECS.md §5) via the dynamic
// client, so no generated clientset or controller-runtime scheme is needed.
package crdstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	v1alpha1 "github.com/TaliaMarine/k8s-driller/pkg/apis/driller/v1alpha1"
)

var (
	userRoleGVR = schema.GroupVersionResource{Group: v1alpha1.GroupName, Version: v1alpha1.Version, Resource: v1alpha1.ResourceUserRoles}
	alertGVR    = schema.GroupVersionResource{Group: v1alpha1.GroupName, Version: v1alpha1.Version, Resource: v1alpha1.ResourceAlertConfigs}
)

// Store is a thin CRUD wrapper around the dynamic client for k8s-driller's
// two cluster-scoped CRDs.
type Store struct {
	client dynamic.Interface

	alertMu    sync.RWMutex
	alertCache *v1alpha1.DrillerAlertConfig // nil until the watch syncs and/or the object doesn't exist
}

func New(client dynamic.Interface) *Store {
	return &Store{client: client}
}

// StartAlertConfigWatch runs an informer over the single DrillerAlertConfig
// object and keeps it cached in memory, so GetAlertConfig never makes a
// live API call. Without this, GetAlertConfig was called from
// evaluateAlerts on every single Recompute (internal/api/push.go) — on a
// cluster with heavy pod churn, thousands of these GETs queued behind the
// dynamic client's default rate limit (client-go's default 5 QPS/burst 10),
// starving interactive requests sharing that client behind them. That
// starvation took down OIDC login in production: the callback handler's
// GetUserRoleBySubject uses the same crdstore.Store, so it queued behind
// the backlog and blew past the ingress route timeout.
//
// watchClient is deliberately a separate dynamic.Interface from the one
// used for interactive Get/Create/Update calls (see cmd/driller/main.go) —
// client-go builds an independent rate limiter per client, so this
// informer's relist traffic can never again share a bucket with
// request-serving code, even if some future change reintroduces bursty
// background reads.
func (s *Store) StartAlertConfigWatch(ctx context.Context, watchClient dynamic.Interface, resync time.Duration) error {
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(watchClient, resync, metav1.NamespaceAll, nil)
	informer := factory.ForResource(alertGVR).Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    s.setAlertCache,
		UpdateFunc: func(_, obj interface{}) { s.setAlertCache(obj) },
		DeleteFunc: func(interface{}) { s.setAlertCacheValue(nil) },
	})
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("crdstore: alert config informer failed to sync")
	}
	return nil
}

func (s *Store) setAlertCache(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	var cfg v1alpha1.DrillerAlertConfig
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &cfg); err != nil {
		return // keep serving the last good cache rather than clobber it with a decode failure
	}
	s.setAlertCacheValue(&cfg)
}

func (s *Store) setAlertCacheValue(cfg *v1alpha1.DrillerAlertConfig) {
	s.alertMu.Lock()
	s.alertCache = cfg
	s.alertMu.Unlock()
}

// SubjectToName derives a deterministic, DNS-1123-safe resource name from an
// OIDC subject claim, since subjects (e.g. "oidc|1234567890") aren't
// themselves valid Kubernetes object names (SPECS.md §5.1).
func SubjectToName(subject string) string {
	sum := sha256.Sum256([]byte(subject))
	return hex.EncodeToString(sum[:])
}

func (s *Store) ListUserRoles(ctx context.Context) ([]v1alpha1.DrillerUserRole, error) {
	list, err := s.client.Resource(userRoleGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list DrillerUserRole: %w", err)
	}
	out := make([]v1alpha1.DrillerUserRole, 0, len(list.Items))
	for _, item := range list.Items {
		var role v1alpha1.DrillerUserRole
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &role); err != nil {
			return nil, fmt.Errorf("decode DrillerUserRole %q: %w", item.GetName(), err)
		}
		out = append(out, role)
	}
	return out, nil
}

// GetUserRoleBySubject returns nil, nil when no role has been assigned yet —
// callers treat an absent role as the default viewer role (SPECS.md §4.1).
func (s *Store) GetUserRoleBySubject(ctx context.Context, subject string) (*v1alpha1.DrillerUserRole, error) {
	obj, err := s.client.Resource(userRoleGVR).Get(ctx, SubjectToName(subject), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get DrillerUserRole for subject: %w", err)
	}
	var role v1alpha1.DrillerUserRole
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &role); err != nil {
		return nil, fmt.Errorf("decode DrillerUserRole: %w", err)
	}
	return &role, nil
}

// SetUserRole creates or updates the role assignment for subject.
func (s *Store) SetUserRole(ctx context.Context, subject, email string, role v1alpha1.Role, updatedBy, updatedAt string) error {
	name := SubjectToName(subject)
	existing, err := s.client.Resource(userRoleGVR).Get(ctx, name, metav1.GetOptions{})

	obj := &v1alpha1.DrillerUserRole{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.GroupName + "/" + v1alpha1.Version, Kind: v1alpha1.KindUserRole},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.DrillerUserRoleSpec{
			Subject:   subject,
			Email:     email,
			Role:      role,
			UpdatedBy: updatedBy,
			UpdatedAt: updatedAt,
		},
	}
	unstructuredObj, convErr := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if convErr != nil {
		return fmt.Errorf("encode DrillerUserRole: %w", convErr)
	}
	u := &unstructured.Unstructured{Object: unstructuredObj}

	if apierrors.IsNotFound(err) {
		_, createErr := s.client.Resource(userRoleGVR).Create(ctx, u, metav1.CreateOptions{})
		return createErr
	}
	if err != nil {
		return fmt.Errorf("get DrillerUserRole before update: %w", err)
	}
	u.SetResourceVersion(existing.GetResourceVersion())
	_, err = s.client.Resource(userRoleGVR).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

// GetAlertConfig returns the single "default" alert config from the
// in-memory cache kept fresh by StartAlertConfigWatch, or nil, nil if it
// hasn't been created yet — callers fall back to built-in defaults. Never
// makes a live API call (see StartAlertConfigWatch for why that matters).
func (s *Store) GetAlertConfig(ctx context.Context) (*v1alpha1.DrillerAlertConfig, error) {
	s.alertMu.RLock()
	defer s.alertMu.RUnlock()
	return s.alertCache, nil
}

// SetAlertConfig creates or updates the single "default" alert config.
func (s *Store) SetAlertConfig(ctx context.Context, spec v1alpha1.DrillerAlertConfigSpec) error {
	existing, err := s.client.Resource(alertGVR).Get(ctx, v1alpha1.DefaultAlertConfigName, metav1.GetOptions{})

	obj := &v1alpha1.DrillerAlertConfig{
		TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.GroupName + "/" + v1alpha1.Version, Kind: v1alpha1.KindAlertConfig},
		ObjectMeta: metav1.ObjectMeta{Name: v1alpha1.DefaultAlertConfigName},
		Spec:       spec,
	}
	unstructuredObj, convErr := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if convErr != nil {
		return fmt.Errorf("encode DrillerAlertConfig: %w", convErr)
	}
	u := &unstructured.Unstructured{Object: unstructuredObj}

	if apierrors.IsNotFound(err) {
		if _, createErr := s.client.Resource(alertGVR).Create(ctx, u, metav1.CreateOptions{}); createErr != nil {
			return createErr
		}
		s.setAlertCacheValue(obj) // don't wait for the watch to see our own write
		return nil
	}
	if err != nil {
		return fmt.Errorf("get DrillerAlertConfig before update: %w", err)
	}
	u.SetResourceVersion(existing.GetResourceVersion())
	if _, err := s.client.Resource(alertGVR).Update(ctx, u, metav1.UpdateOptions{}); err != nil {
		return err
	}
	s.setAlertCacheValue(obj)
	return nil
}
