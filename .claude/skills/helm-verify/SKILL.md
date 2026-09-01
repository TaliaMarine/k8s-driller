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
   any resource outside `driller.k8s.io` (and `verticalpodautoscalers` when the VPA integration is on) has
   any verb other than `get`, `list`, or `watch`. This mirrors the read-only guarantee in SPECS.md §4.2/§8.2.
6. Confirm the CRD files under the chart's top-level `crds/` directory are valid YAML and their
   `metadata.name`/`spec.names` match SPECS.md §5 (`drilleruserroles`, `drilleralertconfigs`).

Report each numbered check as pass/fail with the offending output inline for any failure. Don't attempt to
fix chart issues yourself unless explicitly asked — this skill's job is detection, not remediation.
