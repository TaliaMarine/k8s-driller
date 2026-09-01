---
name: spec-drift-check
description: Compare the current backend/frontend/Helm implementation against SPECS.md and report where they've diverged — new API endpoints or SSE topics not in §6, calculation constants that no longer match §9, CRD fields not in §5, or Helm values not in §8.4. Use periodically during development, before a release, or when SPECS.md itself has just been edited.
context: fork
agent: general-purpose
effort: medium
---

Read `SPECS.md` at the repo root fully first — it is the source of truth for this project's intended
behavior. Then compare it against the actual repository state and report drift in both directions: places
where the code has moved ahead of the spec (undocumented additions) and places where the code has fallen
behind or diverged from it (missing or contradicting behavior).

Check specifically:

- **API surface (§6)**: every REST route and SSE stream path actually implemented vs. listed in the spec.
  Note additions, removals, and role-requirement mismatches (viewer vs admin).
- **CRDs (§5)**: `DrillerUserRole` / `DrillerAlertConfig` field names and types in the actual CRD
  definitions vs. the spec's YAML examples.
- **Pressure/recommendation constants (§9)**: threshold values (90% OOM, 95%/3-poll throttling, 70%
  wasteful, 2.0/1.5 multipliers) — confirm they're read from config/Helm values matching §8.4's
  `recommendation.*` keys, not hardcoded to different numbers.
- **Helm values (§8.4)**: keys present in `values.yaml` vs. keys the spec documents; flag both undocumented
  new keys and documented keys that no longer exist.
- **RBAC (§8.2)**: verbs granted vs. the read-only list, and whether the optional VPA/OpenCost integration
  RBAC (§11) is gated behind its own `enabled` flag as specified.
- **Non-goals (§1.2)**: scan for anything that looks like a write/remediation action, multi-cluster
  management, log streaming, or exec — these were explicitly deferred; flag any sign they've crept in.

If SPECS.md and the code disagree, don't assume the code is wrong — state both sides plainly (e.g. "spec
says X, code does Y") and let the human decide whether to update the spec or the code. Report as a short
list of drift items grouped by section number; if a section has no drift, omit it rather than writing
"no drift found" for every one.
