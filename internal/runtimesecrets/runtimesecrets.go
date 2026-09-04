// Package runtimesecrets resolves the two Secrets k8s-driller needs at
// startup — the session-signing key and the admin bootstrap token — against
// the live cluster, generating them itself when asked to.
//
// This exists because the Helm chart used to generate these with
// `randAlphaNum` + a `lookup`-based preserve-across-upgrades trick
// (SPECS.md §8.4 history). That only works when Helm is talking to a live
// cluster: `helm template` — which is what ArgoCD, Flux, and any dry-run
// tooling actually run — always evaluates `lookup` as empty, so every
// render minted a brand-new random value. GitOps tooling diffing that
// render against live state saw permanent drift, and with auto-sync/self-
// heal enabled it re-applied that fresh random Secret on every reconcile,
// silently rotating the admin bootstrap token and invalidating every signed
// session cookie. Moving generation into the app makes the rendered
// manifests fully deterministic; see SPECS.md §4.2 for the RBAC tradeoff
// this requires.
package runtimesecrets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Ensure returns the value at key in the namespace/name Secret. If the
// Secret doesn't exist:
//   - with autoCreate true, it generates randomBytes of cryptographically
//     random data, base64-encodes it, creates the Secret, and returns the
//     new value;
//   - with autoCreate false, it returns an error — the operator is expected
//     to have supplied their own Secret (values.yaml's `secretRef`, e.g.
//     from Vault/SOPS/ESO) and a missing one is a misconfiguration, not
//     something to paper over.
func Ensure(ctx context.Context, client kubernetes.Interface, namespace, name, key string, autoCreate bool, randomBytes int) ([]byte, error) {
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		value, ok := secret.Data[key]
		if !ok {
			return nil, fmt.Errorf("secret %s/%s has no key %q", namespace, name, key)
		}
		return value, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("get secret %s/%s: %w", namespace, name, err)
	}
	if !autoCreate {
		return nil, fmt.Errorf(
			"secret %s/%s not found and auto-create is disabled; supply it yourself or enable autoGenerate in the chart values",
			namespace, name,
		)
	}

	raw := make([]byte, randomBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate random value: %w", err)
	}
	value := make([]byte, base64.RawURLEncoding.EncodedLen(len(raw)))
	base64.RawURLEncoding.Encode(value, raw)

	created, createErr := client.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{key: value},
	}, metav1.CreateOptions{})
	if createErr == nil {
		return created.Data[key], nil
	}
	if !apierrors.IsAlreadyExists(createErr) {
		return nil, fmt.Errorf("create secret %s/%s: %w", namespace, name, createErr)
	}

	// Lost a race creating it (e.g. a restart racing an old pod's
	// termination) — converge on whatever value won instead of erroring.
	secret, err = client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s/%s after AlreadyExists: %w", namespace, name, err)
	}
	value, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("secret %s/%s has no key %q", namespace, name, key)
	}
	return value, nil
}
