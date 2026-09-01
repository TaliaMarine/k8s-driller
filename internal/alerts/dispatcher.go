// Package alerts evaluates threshold crossings computed by the pressure
// engine and posts notifications to configured Slack/generic webhooks,
// independent of whether any UI client is connected (SPECS.md §4.1 Alert
// dispatcher). It debounces so a persisting condition doesn't refire on
// every recompute.
package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/TaliaMarine/k8s-driller/internal/appmetrics"
	"github.com/TaliaMarine/k8s-driller/internal/crdstore"
	v1alpha1 "github.com/TaliaMarine/k8s-driller/pkg/apis/driller/v1alpha1"
)

// Alert is one notification-worthy event: node pressure, overcommit,
// OOM-Risk, or Throttling-Risk (SPECS.md §5.2 thresholds).
type Alert struct {
	Kind    string
	Subject string // e.g. node name, or "namespace/pod"
	Message string
}

// Dispatcher fires alerts against the webhooks configured in
// DrillerAlertConfig, resolving each webhook's URL from a referenced Secret
// (SPECS.md §5.2 — never stored inline).
type Dispatcher struct {
	k8s        kubernetes.Interface
	store      *crdstore.Store
	namespace  string // namespace webhook secretRefs are resolved against (the app's own namespace)
	httpClient *http.Client

	mu        sync.Mutex
	lastFired map[string]time.Time
}

const defaultDebounceMinutes = 15

func New(k8s kubernetes.Interface, store *crdstore.Store, namespace string) *Dispatcher {
	return &Dispatcher{
		k8s:        k8s,
		store:      store,
		namespace:  namespace,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		lastFired:  make(map[string]time.Time),
	}
}

// Fire dispatches alert to every enabled webhook, unless the same dedup key
// fired within the configured debounce window. key should identify the
// specific condition instance (e.g. "oom-risk:default/pod-a"), not just its
// kind, so unrelated pods don't share a debounce window.
func (d *Dispatcher) Fire(ctx context.Context, key string, alert Alert) error {
	cfg, err := d.store.GetAlertConfig(ctx)
	if err != nil {
		return fmt.Errorf("load alert config: %w", err)
	}
	if cfg == nil {
		return nil // no alert config created yet: nothing to dispatch to
	}

	debounce := time.Duration(cfg.Spec.DebounceMinutes) * time.Minute
	if debounce <= 0 {
		debounce = defaultDebounceMinutes * time.Minute
	}

	d.mu.Lock()
	if last, seen := d.lastFired[key]; seen && time.Since(last) < debounce {
		d.mu.Unlock()
		return nil
	}
	d.lastFired[key] = time.Now()
	d.mu.Unlock()

	var errs []error
	for _, wh := range cfg.Spec.Webhooks {
		if !wh.Enabled {
			continue
		}
		if err := d.send(ctx, wh, alert); err != nil {
			errs = append(errs, err)
			appmetrics.AlertDispatchTotal.WithLabelValues("error").Inc()
		} else {
			appmetrics.AlertDispatchTotal.WithLabelValues("success").Inc()
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("dispatch alert %q: %v", key, errs)
	}
	return nil
}

// Clear drops the debounce record for key, so the next Fire for the same
// condition is treated as a fresh occurrence — call this once the
// underlying condition recovers (SPECS.md §4.1 dedup/debounce).
func (d *Dispatcher) Clear(key string) {
	d.mu.Lock()
	delete(d.lastFired, key)
	d.mu.Unlock()
}

func (d *Dispatcher) send(ctx context.Context, wh v1alpha1.Webhook, alert Alert) error {
	url, err := d.resolveSecret(ctx, wh.SecretRef)
	if err != nil {
		return fmt.Errorf("resolve webhook secret: %w", err)
	}

	var payload []byte
	switch wh.Type {
	case v1alpha1.WebhookSlack:
		payload, err = json.Marshal(map[string]string{
			"text": fmt.Sprintf("[%s] %s: %s", alert.Kind, alert.Subject, alert.Message),
		})
	default:
		payload, err = json.Marshal(alert)
	}
	if err != nil {
		return fmt.Errorf("encode webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *Dispatcher) resolveSecret(ctx context.Context, ref v1alpha1.WebhookSecretRef) (string, error) {
	secret, err := d.k8s.CoreV1().Secrets(d.namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	value, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("secret %s/%s has no key %q", d.namespace, ref.Name, ref.Key)
	}
	return string(value), nil
}
