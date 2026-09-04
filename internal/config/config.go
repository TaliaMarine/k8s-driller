// Package config loads k8s-driller's runtime configuration from environment
// variables, which the Helm chart populates from values.yaml and Secrets
// (SPECS.md §8.4). There is no config file format — one Deployment, one set
// of env vars, matching the single-cluster/single-replica scope (SPECS.md
// §1.2, §8.4).
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/TaliaMarine/k8s-driller/internal/pressure"
)

type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type IntegrationsConfig struct {
	VPAEnabled      bool
	OpenCostEnabled bool
	OpenCostBaseURL string
}

// SecretRef names a Kubernetes Secret (in Config.Namespace) that
// runtimesecrets.Ensure resolves at startup — either read as-is (an
// operator-supplied Secret from their own secret manager) or created with a
// fresh random value if AutoCreate is true and it doesn't exist yet
// (SPECS.md §4.2).
type SecretRef struct {
	Name       string
	Key        string
	AutoCreate bool
}

// Config is every environment-derived setting the backend needs.
type Config struct {
	HTTPAddr  string
	Namespace string // the app's own namespace: webhook secretRefs, and both SecretRefs below, resolve here

	OIDC             OIDCConfig
	AdminTokenSecret SecretRef
	SessionKeySecret SecretRef

	PrometheusEnabled bool
	PrometheusBaseURL string

	MetricsPollInterval time.Duration

	Pressure pressure.Config

	RecommendationLookbackHours int

	Integrations IntegrationsConfig
}

// Load reads and validates configuration from the environment. Fields with
// no sensible default (OIDC issuer/client, the two secret names) are
// required — a misconfigured deployment should fail fast at startup rather
// than serve with broken auth.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:                    getEnv("DRILLER_HTTP_ADDR", ":8080"),
		Namespace:                   getEnv("DRILLER_NAMESPACE", ""),
		PrometheusEnabled:           getEnvBool("DRILLER_PROMETHEUS_ENABLED", false),
		PrometheusBaseURL:           os.Getenv("DRILLER_PROMETHEUS_BASE_URL"),
		MetricsPollInterval:         getEnvDuration("DRILLER_METRICS_POLL_INTERVAL", 15*time.Second),
		RecommendationLookbackHours: getEnvInt("DRILLER_RECOMMENDATION_LOOKBACK_HOURS", 24),
		OIDC: OIDCConfig{
			IssuerURL:    os.Getenv("DRILLER_OIDC_ISSUER_URL"),
			ClientID:     os.Getenv("DRILLER_OIDC_CLIENT_ID"),
			ClientSecret: os.Getenv("DRILLER_OIDC_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("DRILLER_OIDC_REDIRECT_URL"),
		},
		AdminTokenSecret: SecretRef{
			Name:       os.Getenv("DRILLER_ADMIN_TOKEN_SECRET_NAME"),
			Key:        getEnv("DRILLER_ADMIN_TOKEN_SECRET_KEY", "token"),
			AutoCreate: getEnvBool("DRILLER_ADMIN_TOKEN_AUTO_CREATE", true),
		},
		SessionKeySecret: SecretRef{
			Name:       os.Getenv("DRILLER_SESSION_KEY_SECRET_NAME"),
			Key:        getEnv("DRILLER_SESSION_KEY_SECRET_KEY", "key"),
			AutoCreate: getEnvBool("DRILLER_SESSION_KEY_AUTO_CREATE", true),
		},
		Integrations: IntegrationsConfig{
			VPAEnabled:      getEnvBool("DRILLER_INTEGRATIONS_VPA_ENABLED", false),
			OpenCostEnabled: getEnvBool("DRILLER_INTEGRATIONS_OPENCOST_ENABLED", false),
			OpenCostBaseURL: os.Getenv("DRILLER_INTEGRATIONS_OPENCOST_BASE_URL"),
		},
	}

	cfg.Pressure = pressure.Config{
		OOMRiskThresholdPct:        getEnvFloat("DRILLER_OOM_RISK_THRESHOLD_PCT", 90),
		ThrottlingThresholdPct:     getEnvFloat("DRILLER_THROTTLING_THRESHOLD_PCT", 95),
		ThrottlingConsecutivePolls: getEnvInt("DRILLER_THROTTLING_CONSECUTIVE_POLLS", 3),
		WastefulThresholdPct:       getEnvFloat("DRILLER_WASTEFUL_THRESHOLD_PCT", 70),
		RecommendationHeadroomPct:  getEnvFloat("DRILLER_RECOMMENDATION_HEADROOM_PCT", 10),
		CPULimitMultiplier:         getEnvFloat("DRILLER_CPU_LIMIT_MULTIPLIER", 2.0),
		MemLimitMultiplier:         getEnvFloat("DRILLER_MEM_LIMIT_MULTIPLIER", 1.5),
	}

	var missing []string
	if cfg.Namespace == "" {
		missing = append(missing, "DRILLER_NAMESPACE")
	}
	if cfg.AdminTokenSecret.Name == "" {
		missing = append(missing, "DRILLER_ADMIN_TOKEN_SECRET_NAME")
	}
	if cfg.SessionKeySecret.Name == "" {
		missing = append(missing, "DRILLER_SESSION_KEY_SECRET_NAME")
	}
	if cfg.OIDC.IssuerURL == "" {
		missing = append(missing, "DRILLER_OIDC_ISSUER_URL")
	}
	if cfg.OIDC.ClientID == "" {
		missing = append(missing, "DRILLER_OIDC_CLIENT_ID")
	}
	if cfg.OIDC.RedirectURL == "" {
		missing = append(missing, "DRILLER_OIDC_REDIRECT_URL")
	}
	if cfg.PrometheusEnabled && cfg.PrometheusBaseURL == "" {
		missing = append(missing, "DRILLER_PROMETHEUS_BASE_URL (required when DRILLER_PROMETHEUS_ENABLED=true)")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required configuration: %v", missing)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
