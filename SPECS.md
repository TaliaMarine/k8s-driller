# Kubernetes Driller (KubePressure-Lens) — Specification

## 1. Overview

Kubernetes Driller is a read-only, real-time cluster visualization application. It bridges node hardware
capacity, workload resource configuration (requests/limits), and live consumption, surfacing
misconfigurations and pressure bottlenecks visually and immediately.

**Product name used internally during design:** KubePressure-Lens (the visualization concept). Binary/Helm
release name: `k8s-driller`.

### 1.1 Goals

- Give operators an at-a-glance, visual answer to "which nodes/pods are under pressure, and why?"
- Make missing or misconfigured requests/limits impossible to miss ("Wild West" flag).
- Surface overcommit risk (sum of limits > node capacity) per node.
- Classify individual pods into pressure states (OOM-Risk, Throttling-Risk, Wasteful) computed from live +
  historical data.
- Push updates to the UI as soon as the backend observes a relevant change — not just on a fixed poll
  interval.
- Be simple to install into any cluster via Helm, with sane RBAC and no mandatory external dependencies
  beyond `metrics-server`.

### 1.2 Non-goals (v1)

- No write/remediation actions against the cluster (no patch, scale, cordon, drain, delete, exec, restart).
  The app never mutates workloads or nodes it observes. Recommended values are surfaced for the operator to
  apply themselves (e.g. copy-paste, or export as YAML) via a separate change process (kubectl/GitOps).
- No log streaming or exec-into-pod terminal.
- No multi-cluster management from a single instance — one deployment watches one cluster.
- No namespace-scoped viewer restrictions — any authenticated viewer sees the whole cluster.
- No bundled long-term metrics storage — Prometheus (external, pre-existing) is the only history source.

## 2. Architecture Overview

```
                    ┌─────────────────────────────────────────────┐
                    │              Kubernetes Cluster              │
                    │                                               │
                    │  ┌──────────────┐   ┌────────────────────┐   │
                    │  │ kube-apiserver│   │ metrics-server API │   │
                    │  │ (watch/list)  │   │  (metrics.k8s.io)  │   │
                    │  └──────┬───────┘   └─────────┬──────────┘   │
                    │         │                     │              │
                    │  ┌──────▼─────────────────────▼───────────┐  │
                    │  │        k8s-driller backend (Go)         │  │
                    │  │  - Informers (Node/Pod/Deploy/RS/STS/DS)│  │
                    │  │  - Metrics poller (metrics-server)      │  │
                    │  │  - Prometheus client (history/recs)     │  │
                    │  │  - Pressure/state calculation engine    │  │
                    │  │  - CRD store (roles, alert config)      │  │
                    │  │  - OIDC auth + admin-token bootstrap    │  │
                    │  │  - SSE hub (fan-out to clients)         │  │
                    │  │  - Alert/webhook dispatcher             │  │
                    │  └──────────────┬───────────────────────┬─┘  │
                    │                 │ SSE (HTTP/1.1 chunked  │    │
                    │                 │  or HTTP/2 stream)     │    │
                    └─────────────────┼────────────────────────┼───┘
                                       │                        │
                                       ▼                        ▼
                          ┌───────────────────────┐   ┌─────────────────┐
                          │  Vue3 + Vuetify SPA    │   │ Slack / generic │
                          │  (dark/light, reactive)│   │ webhook targets │
                          └───────────────────────┘   └─────────────────┘

              (optional, external, pre-existing)
              ┌────────────────────┐
              │     Prometheus      │◄─── scraped from kube-state-metrics / cAdvisor,
              │ (history + recs)    │      not deployed or managed by this app
              └────────────────────┘
```

### 2.1 Data flow summary

1. Backend starts informers against Nodes, Pods, Deployments, ReplicaSets, StatefulSets, DaemonSets.
2. On every relevant informer event (add/update/delete), the backend recomputes the affected node's and
   pod's derived state and pushes a diff to subscribed SSE clients — this is the primary "reactive" path,
   not a timer.
3. In parallel, a lightweight poller (default every 15s, configurable) queries `metrics.k8s.io` for
   node/pod live CPU & memory usage. New readings are merged into state and also trigger an SSE push (usage
   changes don't have a native watch API, so this is the one necessarily-polled path).
4. Prometheus (if configured) is queried on-demand (dashboard load, or a background refresh every N
   minutes) for 24h+ historical usage, used only for: (a) the "Wasteful" pressure state, and (b)
   recommended request/limit calculation. Prometheus is never on the live-update hot path.
5. All computed state is held in-memory (a single process, no external cache required). CRDs hold the only
   state that must survive a restart: user role assignments and alert/webhook configuration.
6. The alert dispatcher evaluates threshold crossings as state is recomputed and fires webhook/Slack
   notifications independently of whether any UI client is connected.

## 3. Tech Stack

| Layer | Choice |
|---|---|
| Backend language | Go 1.27+ |
| K8s client | `client-go`, informer/lister pattern (no controller-runtime needed — no CRD reconciliation loops beyond simple CRUD) |
| Metrics (live) | `metrics.k8s.io` via the Kubernetes metrics client (`metrics-server` must be installed in-cluster; documented as a prerequisite) |
| Metrics (history/recs) | Prometheus HTTP API (PromQL), official `prometheus/client_golang/api` |
| Realtime transport | Server-Sent Events only (`text/event-stream`), no WebSocket server |
| Auth | OIDC (`coreos/go-oidc` or `golang.org/x/oauth2`) for user login; a separate bootstrap **admin token** (env var/Secret) for first-run role assignment |
| State storage | Kubernetes CRDs (via a small CRD group `driller.dev`) — no database, no PVC |
| Frontend framework | Vue 3 (Composition API) |
| State management | Pinia (holds the SSE-derived reactive cluster/node/pod state) |
| UI library | Vuetify 4+ (Material 3 component set — cards, progress-linear, badges, theming) |
| Build tooling | Vite |
| Packaging | Single Docker image serving the built SPA as static assets from the Go binary, + Helm chart |
| Deployment | Kubernetes native, Helm chart, generic ingress support (ingress-nginx **and** Gateway API `HTTPRoute`) |

## 4. Backend Design

### 4.1 Components

- **Watcher/Informer layer** — SharedInformers for `Node`, `Pod`, `Deployment`, `ReplicaSet`,
  `StatefulSet`, `DaemonSet`. Used for topology, ownership resolution (pod → controller), and requests/limits
  (read directly from pod specs — no polling needed for these).
- **Metrics poller** — periodic `metrics.k8s.io` NodeMetrics/PodMetrics list, default interval 15s
  (configurable via Helm value), jittered to avoid thundering herd on large clusters.
- **Prometheus client** — issues PromQL range queries on demand:
  - 24h avg/max CPU & memory usage per pod (for Wasteful detection & recommendations).
  - Cluster/node historical trend charts (secondary, "history" tab).
  - Recommended request = `max(p95_usage_24h, safety_margin)`; recommended limit = configurable multiple of
    recommended request (default 2x for CPU burst headroom, 1.5x for memory) — exact formula must be
    tunable via Helm values (see §9).
  - If Prometheus is not configured, all Prometheus-derived features (Wasteful tag, recommendations,
    history charts) degrade gracefully to "unavailable" with a clear UI message — they are not required for
    the app to function.
- **State/pressure engine** — pure functions over the merged view (spec + live + optional history) that
  compute:
  - Per-node: CPU/Mem allocated-vs-capacity %, live-vs-capacity %, overcommit flag (limits sum > 100%
    capacity), health label (Healthy / CPU Pressure / Mem Pressure / Overcommit).
  - Per-pod: Wild-West flag (missing request/limit, per resource), OOM-Risk, Throttling-Risk, Wasteful,
    and the raw usage/request/limit triple for the Delta Visualizer.
- **CRD store** — thin CRUD wrapper over two custom resources (cluster-scoped, see §5):
  `DrillerUserRole` and `DrillerAlertConfig`.
- **Auth module**:
  - OIDC login (Authorization Code + PKCE) → on first login, a user gets an implicit `viewer` role unless a
    `DrillerUserRole` already exists for their subject.
  - A separate **admin bootstrap token** (random value in a Secret, resolved by the app itself at startup —
    see §4.2 — not mounted via `secretKeyRef`) grants a one-time-use-per-session "break glass" admin API
    scope, used solely to promote the first real admin user(s) via the Role Management screen. Once at
    least one OIDC user holds `admin`, routine role changes happen through that UI/API, not the token.
  - Session cookies are signed, `HttpOnly`, `Secure`, `SameSite=Lax`.
- **SSE hub** — one broadcast channel per logical topic (`cluster`, `node:<name>`, `alerts`); clients
  subscribe to the topics their current view needs; hub fans out JSON-patch-style diffs, with an initial
  full snapshot on connect and a periodic heartbeat comment (`:keepalive`) to keep proxies/ingress from
  timing out the stream.
- **Alert dispatcher** — evaluates configured thresholds (e.g. "node mem live > 90%", "pod OOM-Risk",
  "overcommit detected") against recomputed state; deduplicates/debounces (won't refire on every recompute
  while a condition persists) and posts to configured Slack incoming-webhook URL(s) and/or generic webhook
  URL(s) as JSON.

### 4.2 Read-only guarantee

The backend's ServiceAccount is granted **only** `get`/`list`/`watch` verbs on cluster/workload resources
(see §8.2 RBAC). There is no code path in the backend that issues a `create`/`update`/`patch`/`delete`
against a node, pod, or workload controller. This is enforced both by RBAC (defense in depth) and by simply
not implementing those client calls.

**One explicit, documented exception:** the app owns two runtime Secrets in its own namespace — the
session-signing key and the admin bootstrap token (§4.1) — and is granted `create` on `secrets` (namespace-
scoped, not cluster-wide) to generate them itself if they don't already exist, gated per-secret by
`autoGenerate` in `values.yaml` (§8.4; both default `true`; RBAC drops the `create` verb entirely when
both are `false`, see §8.2). This replaced an earlier chart-side design that generated these with Helm's
`randAlphaNum` plus a `lookup`-based "preserve across upgrades" trick — that isn't GitOps-safe: `lookup`
evaluates empty under `helm template` (what ArgoCD, Flux, and any dry-run tooling actually render), so every
such render minted a fresh random Secret, and a GitOps controller with self-heal enabled reapplied that
fresh value on every reconcile — silently rotating the admin bootstrap token and invalidating every signed
session cookie on a live deployment. `create` can't be scoped to specific resource names in Kubernetes RBAC
(the object doesn't exist yet at authorization time), so this is "may create any Secret in its own
namespace" — the narrowest expression available, not name-restricted. Operators who'd rather keep the app
fully read-only can set `autoGenerate: false` on both and supply the Secrets themselves (Vault, SOPS,
External Secrets Operator, etc.) — see §8.4.

## 5. Data Model (CRDs)

Group: `driller.dev/v1alpha1`, cluster-scoped.

### 5.1 `DrillerUserRole`

```yaml
apiVersion: driller.dev/v1alpha1
kind: DrillerUserRole
metadata:
  name: <sanitized-oidc-subject-hash>
spec:
  subject: "oidc|1234567890"     # OIDC `sub` claim (issuer-qualified)
  email: "alice@example.com"     # display only
  role: admin | viewer
  updatedBy: "oidc|admin-sub"
  updatedAt: "2026-08-01T12:00:00Z"
```

### 5.2 `DrillerAlertConfig`

```yaml
apiVersion: driller.dev/v1alpha1
kind: DrillerAlertConfig
metadata:
  name: default
spec:
  webhooks:
    - type: slack | generic
      url: <string, stored as a referenced Secret name, not inline>
      enabled: true
  thresholds:
    nodeMemLivePct: 90
    nodeCpuLivePct: 90
    overcommitEnabled: true
    oomRiskEnabled: true
    throttlingRiskEnabled: true
  debounceMinutes: 15
```

Webhook URLs are secrets by reference (`secretRef`) — never stored in the CRD spec in plaintext, to keep
them out of `kubectl get` output and audit logs.

## 6. API Design

All endpoints under `/api/v1`, JSON, session-cookie authenticated (except `/healthz`, `/readyz`,
`/api/v1/auth/*`).

### 6.1 REST

| Method | Path | Role | Purpose |
|---|---|---|---|
| GET | `/api/v1/cluster/summary` | viewer | Cluster totals snapshot (CPU/Mem alloc vs live) |
| GET | `/api/v1/nodes` | viewer | List nodes with computed pressure state |
| GET | `/api/v1/nodes/{name}/pods` | viewer | Pods scheduled on a node, grouped by namespace/controller |
| GET | `/api/v1/pods/{namespace}/{name}` | viewer | Pod detail: usage/request/limit triple, pressure tags |
| GET | `/api/v1/pods/{namespace}/{name}/recommendation` | viewer | Prometheus-derived recommended request/limit (404 if Prometheus unconfigured or insufficient history) |
| GET | `/api/v1/history/nodes/{name}` | viewer | Historical trend series for charts (Prometheus-backed) |
| GET | `/api/v1/auth/me` | any authenticated | Current user, role, session expiry |
| POST | `/api/v1/auth/login` \| `/callback` | public | OIDC flow |
| POST | `/api/v1/auth/logout` | any authenticated | Clear session |
| GET | `/api/v1/admin/users` | admin | List all `DrillerUserRole` |
| PUT | `/api/v1/admin/users/{subject}/role` | admin | Change a user's role |
| GET/PUT | `/api/v1/admin/alerts/config` | admin | View/update `DrillerAlertConfig` |
| POST | `/api/v1/admin/alerts/test` | admin | Fire a test notification to configured webhooks |
| GET | `/healthz`, `/readyz` | public | Liveness/readiness |

### 6.2 SSE streams

| Path | Topic | Payload |
|---|---|---|
| `/api/v1/stream/cluster` | Cluster + all-nodes summary | Full snapshot on connect, then incremental patches |
| `/api/v1/stream/nodes/{name}` | One node's pods/pressure | Snapshot + patches, used by the drilldown view |
| `/api/v1/stream/alerts` | Fired alerts (for an in-app toast/notification feed) | Event per alert fired |

Event format: standard SSE `event:` + `data:` (JSON), with `event: snapshot` on connect and
`event: patch` thereafter, so the client can apply the same reducer either way.

## 7. Frontend Design

### 7.1 Views

1. **Cluster Dashboard** (`/`) — header (cluster name, connection/live indicator, theme toggle), cluster
   totals bar, responsive grid of Node Cards. Node card size/saturation scales with pressure; dual layered
   progress bars (allocation layer + live-usage overlay layer); overcommit banner when applicable.
2. **Node Drilldown** (`/nodes/:name`) — entered via card click with a shared-element/morph transition
   (not a hard route reload) so the node card visually expands into the detail header. Two sections:
   - "Wild West" list — pods missing request/limit, chips per missing dimension.
   - Workload list grouped by namespace → controller (Deployment/StatefulSet/DaemonSet/bare Pod), each row
     expandable to the Delta Visualizer (three-way usage/request/limit bars) and pressure-state chips
     (OOM-Risk / Throttling-Risk / Wasteful), plus a "Recommended" bar when Prometheus data is available.
3. **Role Management** (`/admin/users`, admin-only) — table of OIDC users with role dropdown; the
   first-ever admin promotion flow (using the bootstrap token) is a distinct, clearly-labeled one-time
   screen, not mixed into routine role editing.
4. **Alert Settings** (`/admin/alerts`, admin-only) — threshold sliders, webhook URL management (write-only
   once saved — displayed masked), "send test alert" button.
5. **Login** (`/login`) — OIDC provider button(s); no local username/password.

### 7.2 Visual language

- Dark theme is default; light/dark toggle persisted client-side (and ideally synced to OS
  `prefers-color-scheme` on first visit).
- Color semantics are consistent across the whole app: a shared severity scale (healthy → watch → warning →
  critical) maps to the same colors everywhere a pressure state appears — node card border, pod chip,
  progress-bar fill — so users learn the palette once. Bright orange/red reserved specifically for the
  "Wild West" (missing config) flag per the brief, kept visually distinct from the OOM/Throttling severity
  colors so "misconfigured" and "under pressure" are never confused at a glance.
- All live-updating numbers animate on change (no re-render jump) so the "reactive" feel promised by the
  SSE architecture is visible, not just structurally true.
- Fully responsive grid (node cards reflow, not just shrink, on narrow viewports); no fixed desktop-only
  breakpoint.

### 7.3 Realtime client

A single `EventSource`-based composable per open view manages subscribe/reconnect (with backoff) and
applies incoming patches to local reactive state; a persistent small connection-status indicator in the
header reflects `live` / `reconnecting` / `disconnected` so users always know whether what they're looking
at is current.

## 8. Kubernetes / Helm

### 8.1 Chart shape

```
charts/k8s-driller/
  Chart.yaml
  values.yaml
  templates/
    deployment.yaml
    service.yaml
    serviceaccount.yaml
    clusterrole.yaml
    clusterrolebinding.yaml
    ingress.yaml                  # rendered only if .Values.ingress.type == nginx
    httproute.yaml                # rendered only if .Values.ingress.type == gateway-api
    poddisruptionbudget.yaml      # optional
    networkpolicy.yaml            # optional, default-deny except k8s API + DNS + Prometheus/webhook targets
  crds/
    driller.dev_drilleruserroles.yaml     # DrillerUserRole CRD
    driller.dev_drilleralertconfigs.yaml  # DrillerAlertConfig CRD
```

CRDs live in the chart's top-level `crds/` directory rather than under `templates/`, per Helm's own convention for
CRDs: installed before every other template, never templated, and never modified on `helm upgrade` (avoids Helm
silently clobbering a CRD another tool also manages).

### 8.2 RBAC (ClusterRole, read-only + one documented, opt-out exception)

```yaml
# ClusterRole
rules:
  - apiGroups: [""]
    resources: ["nodes", "pods", "namespaces"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["metrics.k8s.io"]
    resources: ["nodes", "pods"]
    verbs: ["get", "list"]
  - apiGroups: ["driller.dev"]
    resources: ["drilleruserroles", "drilleralertconfigs"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]   # own CRDs only

# namespaced Role, bound only in the app's own namespace
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get"]                # always: webhook secretRefs (§5.2), reading back its own runtime secrets
    # + "create" only when sessionSigningKey.autoGenerate or adminBootstrapToken.autoGenerate is true (both
    # default true) — see §4.2 for why this exists and how to opt out.
```

No verbs are ever granted on workloads beyond read — this list is the enforcement point backing the
read-only guarantee in §4.2, aside from the single documented `secrets: create` exception above.

### 8.3 Ingress genericness

`values.yaml` exposes:

```yaml
ingress:
  type: none | nginx | gateway-api
  host: driller.example.com
  tls:
    enabled: true
    secretName: ""
  nginx:
    className: nginx
    annotations: {}
  gatewayApi:
    gatewayName: ""
    gatewayNamespace: ""
    sectionName: https
```

`ingress.yaml` renders a standard `networking.k8s.io/v1` Ingress (className-based, not the deprecated
annotation-only form) when `type: nginx`; `httproute.yaml` renders a `gateway.networking.k8s.io/v1`
`HTTPRoute` attaching to an existing Gateway when `type: gateway-api`. Only one is ever rendered. TLS
termination assumed to be handled by the ingress/gateway layer, not the app itself.

### 8.4 Other values of note

```yaml
oidc:
  issuerUrl: ""
  clientId: ""
  clientSecretRef: {name: "", key: ""}
  redirectUrl: ""
adminBootstrapToken:
  autoGenerate: true      # if false, the app requires secretRef to already exist — see §4.2
  secretRef: {name: "", key: "token"}
sessionSigningKey:
  autoGenerate: true      # same pattern, for the signed-session-cookie key
  secretRef: {name: "", key: "key"}
prometheus:
  enabled: false
  baseUrl: ""             # e.g. http://prometheus-server.monitoring:9090
metricsPoll:
  intervalSeconds: 15
recommendation:
  lookbackHours: 24
  cpuLimitMultiplier: 2.0
  memLimitMultiplier: 1.5
  wastefulThresholdPct: 70
resources: {...}           # the app's own request/limit — dogfooding, should itself never be "Wild West"
replicaCount: 1             # single replica; SSE hub state is in-memory, so no leader election / HA story for v1
```

Single-replica note: because SSE fan-out and computed state live in one process's memory, running >1
replica today would give clients inconsistent views depending on which pod they land on. This is an
accepted v1 limitation, not an oversight — call out explicitly rather than silently support multi-replica
with broken behavior.

## 9. Pressure & Recommendation Calculations (precise definitions)

- **Node allocation %** = `Σ(pod requests on node) / node allocatable capacity`, computed separately for
  limits.
- **Overcommit** = `Σ(pod limits on node) > node allocatable capacity` (per resource, CPU and Memory
  evaluated independently; a node can be CPU-overcommitted, Mem-overcommitted, both, or neither).
- **Wild-West** = pod has ≥1 container missing `resources.requests.cpu`, `requests.memory`, `limits.cpu`,
  or `limits.memory` — each missing dimension flagged individually, not collapsed into one generic
  "misconfigured" tag.
- **OOM-Risk** = live memory usage (from metrics-server) `> 90%` of the container's memory **limit**. Not
  computable (and not shown, rather than shown as false) if no memory limit is set — that pod is Wild-West
  instead.
- **Throttling-Risk** = live CPU usage sampled at the poll interval sustained `≥ 95%` of the CPU **limit**
  for at least 3 consecutive polls (default ~45s at 15s interval) — avoids flagging a single momentary
  spike as throttling.
- **Wasteful** = requires Prometheus: `p95(usage_24h) < (1 - wastefulThresholdPct%) × configured request`
  (default threshold 70%, i.e. request is more than ~3.3x the observed p95 usage). Only evaluated for pods
  with an explicit request set (Wild-West pods aren't "wasteful", they're unconfigured).
- **Recommended request** = `p95(usage_24h)` with a small headroom floor (configurable, default +10%) to
  avoid recommending exactly the observed ceiling.
- **Recommended limit** = `recommended request × cpuLimitMultiplier` (CPU, default 2.0) or
  `× memLimitMultiplier` (Memory, default 1.5) — asymmetric because CPU limits are throttling (safe to set
  loose) while memory limits are OOM-kill (kept tighter), per standard K8s sizing guidance.

All constants above are Helm-configurable (§8.4), not hardcoded, since "sensible defaults" vary by
workload profile across clusters.

## 10. Non-Functional Requirements

- **Scale target (v1):** comfortably handle clusters up to ~500 nodes / ~5,000 pods with sub-second SSE
  patch propagation on a single replica; beyond that is explicitly out of scope until a documented need
  arises (avoid speculative horizontal-scaling work now).
- **Security:** read-only RBAC (§8.2/§4.2); OIDC for all human login; admin bootstrap token single-purpose
  and rotatable; webhook URLs stored only as Secret references, never inline in CRDs or logs; SSE and REST
  both behind the same session auth (no separate unauthenticated metrics endpoint).
- **Resilience:** SSE clients auto-reconnect with backoff and resync via a fresh snapshot; a metrics-server
  or Prometheus outage degrades the relevant features (live usage bars / recommendations respectively) to a
  clear "unavailable" state rather than crashing or showing stale data silently.
- **Observability of the app itself:** the backend exposes its own `/metrics` (Prometheus format) for
  operators to monitor k8s-driller's own health — informer sync status, SSE client count, alert dispatch
  success/failure counts.
- **Testing strategy:** the pressure/recommendation engine (§9) is pure functions over plain data
  (node/pod specs + usage + optional history) — unit-tested exhaustively against the documented formulas
  and their edge cases (missing limits, no Prometheus, single-sample history), independent of any live
  cluster. Handler/API and CRD-store code get integration tests against a fake clientset/`envtest`; the
  frontend's severity-color mapping and SSE-patch reducer get component tests so the "one palette,
  everywhere" guarantee in §7.2 doesn't silently drift.

## 11. Optional Complementary Integrations (opt-in, disabled by default)

Two read-only, additive integrations surfaced by looking at adjacent tools in this space. Both fit the
existing constraints (no writes, no new *required* dependency) and are off by default behind their own
Helm value — never a dependency of the core pressure/audit views, and hidden (not shown as an error) when
disabled or unreachable.

- **VPA recommendation read (`integrations.vpa.enabled`)** — Tools like Fairwinds Goldilocks run the
  Kubernetes Vertical Pod Autoscaler in "recommendation mode" per workload and dashboard its output. If VPA
  CRDs (`autoscaling.k8s.io/v1` `VerticalPodAutoscaler`) already exist in-cluster, k8s-driller can read
  their `.status.recommendation` and show it next to its own Prometheus-derived recommendation (§9) in the
  Delta Visualizer, as a cross-check. k8s-driller never creates, updates, or deletes VPA objects — reading
  `verticalpodautoscalers` (`get`/`list`/`watch`) is the only RBAC addition required.
- **Cost-of-waste overlay (`integrations.opencost.enabled`)** — If an OpenCost instance is reachable (it
  computes Kubernetes cost allocation from Prometheus + cloud billing APIs and exposes a query API),
  k8s-driller can annotate "Wasteful" pods and overcommitted nodes with an estimated $/month figure, turning
  "this pod requests 3x its p95 usage" into "...costing ~$14/mo more than needed." This is a plain HTTP
  read against OpenCost's own API — k8s-driller never touches cloud billing credentials directly.

## 12. Open Items Deferred Past v1 (explicitly out of scope, revisit later if requested)

- Multi-cluster management from one instance.
- Namespace-scoped viewer permissions.
- Pod log streaming and exec-into-pod (would reintroduce a WebSocket/bidirectional need dropped in v1).
- Any write/remediation action (patch, cordon, drain, scale, restart, delete).
- High-availability / multi-replica backend (would need to move SSE state out of process memory, e.g. to a
  shared pub/sub — not justified for v1's stated scale).
- Cost estimation / chargeback.
