---
name: helm-verify
description: Lint and render the k8s-driller Helm chart under both ingress modes (nginx and Gateway API) and both metrics/Prometheus configurations, and check the rendered RBAC never grants write verbs on workload/node resources. Use before committing chart changes or cutting a release.
disable-model-invocation: true
allowed-tools: Bash(helm *), Bash(find *), Bash(grep *)
effort: low
---

Run these checks against the chart in `charts/k8s-driller/` (adjust the path below if it differs) and report
pass/fail for each — this is a mechanical verification pass, not a design review.

1. `helm lint charts/k8s-driller` — must exit clean.
2. Render with each ingress mode and confirm exactly one of Ingress/HTTPRoute appears, never both:
   - `helm template charts/k8s-driller --set ingress.type=nginx`
   - `helm template charts/k8s-driller --set ingress.type=gateway-api`
   - `helm template charts/k8s-driller --set ingress.type=none`
3. Render with Prometheus enabled and disabled, confirming the chart doesn't hard-require it:
   - `helm template charts/k8s-driller --set prometheus.enabled=false`
   - `helm template charts/k8s-driller --set prometheus.enabled=true --set prometheus.baseUrl=http://prometheus.monitoring:9090`
4. Render with the optional integrations from SPECS.md §11 both on and off:
   - `helm template charts/k8s-driller --set integrations.vpa.enabled=true`
   - `helm template charts/k8s-driller --set integrations.opencost.enabled=true`
5. Extract every `ClusterRole`/`Role` from a default render and grep its `verbs:` lists — fail the check if
   any resource outside `driller.dev` (and `verticalpodautoscalers` when the VPA integration is on) has
   any verb other than `get`, `list`, or `watch` — **except** `secrets`, which may also carry `create`
   (SPECS.md §4.2). This mirrors the read-only guarantee in SPECS.md §4.2/§8.2.
6. Confirm the CRD files under the chart's top-level `crds/` directory are valid YAML and their
   `metadata.name`/`spec.names` match SPECS.md §5 (`drilleruserroles`, `drilleralertconfigs`).
7. Secret-bootstrap toggle (SPECS.md §4.2) — render both ways and confirm the `secrets` rule's verbs differ:
   - `helm template charts/k8s-driller` (defaults, both `autoGenerate: true`) → verbs must include `create`.
   - `helm template charts/k8s-driller --set sessionSigningKey.autoGenerate=false --set sessionSigningKey.secretRef.name=x --set adminBootstrapToken.autoGenerate=false --set adminBootstrapToken.secretRef.name=y`
     → verbs must be `get` only, no `create`.
   - Also confirm no `templates/secret-*.yaml` exists in the chart at all — Secret generation lives in the
     app (`internal/runtimesecrets`), not the chart, specifically because the chart's old
     `randAlphaNum`+`lookup` approach wasn't deterministic under `helm template`.
8. `oidc.clientId` vs `oidc.clientIdRef` — every chart value that has an operator-supplied-Secret
   equivalent (this one, plus the two in check 7) must be settable entirely through `values.yaml`, with
   zero Kustomize/Helm post-render patches required downstream — that's the point of the `*Ref` pattern.
   Render both ways and confirm the env var switches shape correctly:
   - `helm template charts/k8s-driller --set oidc.clientId=plain-id` → `DRILLER_OIDC_CLIENT_ID` is a plain
     `value:`.
   - `helm template charts/k8s-driller --set oidc.clientIdRef.name=x --set oidc.clientIdRef.key=y` →
     `DRILLER_OIDC_CLIENT_ID` is a `valueFrom.secretKeyRef`, and `oidc.clientId` is ignored.

Report each numbered check as pass/fail with the offending output inline for any failure. Don't attempt to
fix chart issues yourself unless explicitly asked — this skill's job is detection, not remediation.
