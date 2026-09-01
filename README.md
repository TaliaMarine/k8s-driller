# Kubernetes Driller

> **⚠️ AI-generated codebase.** The spec, backend, frontend, Helm chart, CI/CD, and this README were written
> by an AI coding agent (Claude), directed and reviewed by a human. It has been built, tested, and smoke-run
> against a real cluster (see commit history), but it has not had the level of scrutiny you'd expect before
> running it against a production cluster with real user data. Review the code — especially
> [`internal/pressure`](./internal/pressure), RBAC in
> [`charts/k8s-driller/templates/clusterrole.yaml`](./charts/k8s-driller/templates/clusterrole.yaml),
> and auth in [`internal/auth`](./internal/auth) — before trusting it with anything that matters.

**Real-time cluster pressure and misconfiguration dashboard.** Kubernetes Driller answers one question at
a glance: *which nodes and pods are under pressure, and why?* It overlays configured requests/limits
against live usage, flags pods running without resource requests/limits ("Wild-West" workloads), and
surfaces overcommit risk — all pushed to the browser reactively over Server-Sent Events, not on a polling
timer.

Full product/architecture spec: [SPECS.md](./SPECS.md).

## Features

- **Cluster & node topology view** — every node as a card, sized/colored by pressure, with dual-layer bars
  (configured allocation vs. live usage) and an overcommit banner when `Σ(pod limits) > node capacity`.
- **Node drilldown** — pods on a node grouped by namespace/controller (Deployment, StatefulSet, DaemonSet),
  with a Wild-West list for anything missing requests/limits.
- **Per-pod pressure states** — OOM-Risk, Throttling-Risk, and (with Prometheus configured) Wasteful,
  computed from precise, unit-tested rules (see [`internal/pressure`](./internal/pressure)).
- **Recommended requests/limits** — derived from 24h p95 usage via Prometheus, never applied automatically.
- **Reactive, not polled** — the backend watches the Kubernetes API via informers and pushes SSE patches
  the moment something changes; metrics-server is polled only because there's no watch API for point-in-time
  usage.
- **OIDC login, role-based access** — every login defaults to `viewer`; an admin promotes users to `admin`
  through the UI (or a one-time bootstrap token for the very first admin).
- **Strictly read-only** — the backend's Kubernetes RBAC never grants anything beyond `get`/`list`/`watch`
  on cluster resources. It cannot patch, scale, cordon, or delete anything it observes.
- **Optional, off-by-default integrations** — read VPA recommendations as a cross-check, or overlay
  OpenCost's cost-of-waste figures — see SPECS.md §11.

## Architecture

```
 kube-apiserver ──watch──┐
                         │
 metrics-server ──poll───┼──▶  Go backend  ──SSE──▶  Vue 3 + Vuetify SPA
                         │      (cmd/driller)
 Prometheus (optional) ──┘         │
   history/recommendations         └─▶ driller.k8s.io CRDs (roles, alert config — the only persisted state)
```

- **Backend** (`cmd/driller`, `internal/`): informer-based topology (`k8swatch`), a metrics-server client
  polled on an interval, an optional Prometheus client for history/recommendations only, a pure
  pressure/recommendation engine (`internal/pressure`), an SSE hub, OIDC auth with signed sessions
  (`internal/auth`), and an alert dispatcher for Slack/generic webhooks.
- **Frontend** (`frontend/`): Vue 3 + Vuetify 4, Pinia for state, one `EventSource` subscription per view.
- **State**: no database. The only things that survive a restart — user role assignments and alert config —
  are stored as `driller.k8s.io` custom resources, read/written via the dynamic client.

## Quickstart

### Prerequisites

- A Kubernetes cluster with [metrics-server](https://github.com/kubernetes-sigs/metrics-server) installed
  (required — this is the live usage source).
- An OIDC provider (issuer URL + client ID/secret) for login.
- *(Optional)* A running Prometheus, for history charts and recommended request/limit values.

### Install with Helm

```sh
helm install driller ./charts/k8s-driller \
  --namespace driller --create-namespace \
  --set oidc.issuerUrl=https://your-issuer.example.com \
  --set oidc.clientId=<client-id> \
  --set oidc.clientSecretRef.name=<secret-name> \
  --set oidc.clientSecretRef.key=<secret-key> \
  --set oidc.redirectUrl=https://driller.example.com/api/v1/auth/callback \
  --set ingress.type=nginx \
  --set ingress.host=driller.example.com
```

The admin bootstrap token and session signing key are auto-generated as Secrets on first install (and
preserved across upgrades). Fetch the bootstrap token to promote your first admin:

```sh
kubectl -n driller get secret driller-admin-token -o jsonpath='{.data.token}' | base64 -d
```

See [`charts/k8s-driller/values.yaml`](./charts/k8s-driller/values.yaml) for every configurable value
(Prometheus, recommendation thresholds, Gateway API ingress, optional integrations, etc.), and SPECS.md §8
for the rationale behind each.

## Local development

```sh
# Backend — requires a reachable cluster (in-cluster config, or your current kubeconfig context)
export DRILLER_NAMESPACE=default
export DRILLER_ADMIN_TOKEN=dev-token
export DRILLER_SESSION_SIGNING_KEY=$(openssl rand -base64 32)
export DRILLER_OIDC_ISSUER_URL=... DRILLER_OIDC_CLIENT_ID=... DRILLER_OIDC_REDIRECT_URL=...
go run ./cmd/driller

# Frontend (separate terminal) — proxies /api to :8080 in dev, see frontend/vite.config.ts
cd frontend
npm install
npm run dev
```

### Tests & checks

```sh
go build ./... && go vet ./... && go test ./...     # backend
cd frontend && npm run lint && npm run build          # frontend (type-check + build)
helm lint charts/k8s-driller                          # chart
```

CI (`.github/workflows/ci.yml`) runs all of the above on every PR, plus a check that the chart's rendered
RBAC never grants a write verb outside the app's own CRDs. Every GitHub Action referenced in this repo's
workflows is pinned to a full commit SHA (enforced by `.github/workflows/pinact-check.yml` via
[pinact](https://github.com/suzuki-shunsuke/pinact)).

## Project layout

```
cmd/driller/            backend entrypoint
internal/
  pressure/              pure pressure/recommendation formulas (SPECS.md §9) — fully unit tested
  k8swatch/               informer-based cluster topology
  metricsclient/          metrics-server client (live usage)
  promclient/             Prometheus client (history/recommendations only)
  crdstore/               CRUD for the driller.k8s.io CRDs (dynamic client, no codegen)
  auth/                   OIDC login, signed sessions, admin bootstrap token
  sse/                    the SSE hub
  alerts/                 Slack/generic webhook dispatch with debounce
  api/                    REST + SSE route handlers
pkg/apis/driller/v1alpha1/  CRD Go types
charts/k8s-driller/      Helm chart (RBAC, CRDs, ingress/HTTPRoute, secrets)
frontend/                Vue 3 + Vuetify 4 SPA
```

## Security model

The backend's ServiceAccount is granted only `get`/`list`/`watch` on cluster resources — see
[`charts/k8s-driller/templates/clusterrole.yaml`](./charts/k8s-driller/templates/clusterrole.yaml). There is
no code path anywhere in this repo that creates, updates, patches, or deletes a workload or node. Webhook
URLs are referenced by Secret, never stored inline. See SPECS.md §4.2/§8.2/§10 for the full rationale.

## License

[MIT](./LICENSE)
