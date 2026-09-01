// Package v1alpha1 defines the driller.k8s.io custom resource types used as
// k8s-driller's only persistent state (SPECS.md §5) — user role assignments
// and alert/webhook configuration. There is no generated clientset; these
// types are read/written through the dynamic client and unstructured
// conversion in internal/crdstore, which is enough for plain CRUD without
// codegen.
package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	GroupName = "driller.k8s.io"
	Version   = "v1alpha1"

	KindUserRole    = "DrillerUserRole"
	KindAlertConfig = "DrillerAlertConfig"

	ResourceUserRoles    = "drilleruserroles"
	ResourceAlertConfigs = "drilleralertconfigs"
)

// Role is a user's assigned access level. Every OIDC login defaults to
// RoleViewer unless a DrillerUserRole already exists for their subject
// (SPECS.md §4.1 Auth module).
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

// DrillerUserRole maps one OIDC subject to a Role (SPECS.md §5.1).
type DrillerUserRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DrillerUserRoleSpec `json:"spec"`
}

type DrillerUserRoleSpec struct {
	Subject   string `json:"subject"`
	Email     string `json:"email,omitempty"`
	Role      Role   `json:"role"`
	UpdatedBy string `json:"updatedBy,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// WebhookType selects the payload shape a webhook target expects.
type WebhookType string

const (
	WebhookSlack   WebhookType = "slack"
	WebhookGeneric WebhookType = "generic"
)

// Webhook is one alert delivery target. The URL is never stored inline —
// only a reference to the Secret key holding it (SPECS.md §5.2).
type Webhook struct {
	Type      WebhookType      `json:"type"`
	SecretRef WebhookSecretRef `json:"secretRef"`
	Enabled   bool             `json:"enabled"`
}

type WebhookSecretRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type Thresholds struct {
	NodeMemLivePct        float64 `json:"nodeMemLivePct"`
	NodeCPULivePct        float64 `json:"nodeCpuLivePct"`
	OvercommitEnabled     bool    `json:"overcommitEnabled"`
	OOMRiskEnabled        bool    `json:"oomRiskEnabled"`
	ThrottlingRiskEnabled bool    `json:"throttlingRiskEnabled"`
}

// DrillerAlertConfig holds alert thresholds and webhook targets (SPECS.md
// §5.2). A single instance named "default" is expected.
type DrillerAlertConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DrillerAlertConfigSpec `json:"spec"`
}

type DrillerAlertConfigSpec struct {
	Webhooks        []Webhook  `json:"webhooks,omitempty"`
	Thresholds      Thresholds `json:"thresholds"`
	DebounceMinutes int        `json:"debounceMinutes"`
}

// DefaultAlertConfigName is the name of the single expected DrillerAlertConfig
// instance (SPECS.md §5.2 example).
const DefaultAlertConfigName = "default"
