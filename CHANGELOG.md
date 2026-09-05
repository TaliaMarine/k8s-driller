# Changelog

## [2.7.0](https://github.com/TaliaMarine/k8s-driller/compare/v2.6.0...v2.7.0) (2026-09-05)


### Features

* add kube-system visibility toggle to pod lists ([703020a](https://github.com/TaliaMarine/k8s-driller/commit/703020a8852e9902e63a996b73d48733a6be2229))
* rework node card resource bars, drop colored border and overcommit alert ([8159c03](https://github.com/TaliaMarine/k8s-driller/commit/8159c031c0b4f0bd34849d46349fceadbf9ab5c1))

## [2.6.0](https://github.com/TaliaMarine/k8s-driller/compare/v2.5.2...v2.6.0) (2026-09-05)


### Features

* show Unschedulable instead of Healthy for cordoned nodes ([aa71478](https://github.com/TaliaMarine/k8s-driller/commit/aa71478b67c3c7913ba5547e4c31426944aa38f5))

## [2.5.2](https://github.com/TaliaMarine/k8s-driller/compare/v2.5.1...v2.5.2) (2026-09-05)


### Bug Fixes

* usage bar coloring/icons, node-link grouping, merged missing chips ([9abeccb](https://github.com/TaliaMarine/k8s-driller/commit/9abeccb49a6bc0ac5094aeb93a19ca56de1d417e))

## [2.5.1](https://github.com/TaliaMarine/k8s-driller/compare/v2.5.0...v2.5.1) (2026-09-05)


### Bug Fixes

* NotReady node health, plateau-aware recommendations, reworked pod usage bars ([6846f4b](https://github.com/TaliaMarine/k8s-driller/commit/6846f4b3c8a253a65b560dc652a20861dbd51299))

## [2.5.0](https://github.com/TaliaMarine/k8s-driller/compare/v2.4.1...v2.5.0) (2026-09-04)


### Features

* add cluster-wide Workloads view, pod history analysis, and AI export ([2f1a5c6](https://github.com/TaliaMarine/k8s-driller/commit/2f1a5c604ac9318bc89a84af5269e90252b2c64c))
* add drill-bit favicon ([91bb3f9](https://github.com/TaliaMarine/k8s-driller/commit/91bb3f9ff7ef9096c958865f024fa65f15233881))
* flag pods with implausibly high CPU/memory requests or limits ([e4c46f5](https://github.com/TaliaMarine/k8s-driller/commit/e4c46f57e72f43a24d942841af280a16df415e3f))

## [2.4.1](https://github.com/TaliaMarine/k8s-driller/compare/v2.4.0...v2.4.1) (2026-09-04)


### Bug Fixes

* soften node severity signals, add filter dropdown and over-requests count ([5739b0a](https://github.com/TaliaMarine/k8s-driller/commit/5739b0a58027a5545c3acc887599515150553cf9))

## [2.4.0](https://github.com/TaliaMarine/k8s-driller/compare/v2.3.0...v2.4.0) (2026-09-04)


### Features

* user menu with display name and profile dialog, home nav from anywhere ([7329392](https://github.com/TaliaMarine/k8s-driller/commit/7329392ea4b1d480d46db3e6d58b9fdab1a1db6b))

## [2.3.0](https://github.com/TaliaMarine/k8s-driller/compare/v2.2.1...v2.3.0) (2026-09-04)


### Features

* per-pod usage/request mini bars, node-wide over-request stat ([8a52693](https://github.com/TaliaMarine/k8s-driller/commit/8a526930d1e997855f7fc56bf022dd5dc107911a))


### Bug Fixes

* set HTTPRoute fields the Gateway API CRD schema server-defaults ([3187c7f](https://github.com/TaliaMarine/k8s-driller/commit/3187c7f5c450829718bb19d2278ebbc93bb2ca94))

## [2.2.1](https://github.com/TaliaMarine/k8s-driller/compare/v2.2.0...v2.2.1) (2026-09-04)


### Bug Fixes

* distinguish requests/limits colors, add a third usage row to the chart ([9dd0be1](https://github.com/TaliaMarine/k8s-driller/commit/9dd0be16a432183782a8b9dcab4550f214c2422e))
* exclude terminal (Succeeded/Failed) pods from node calculations ([0b2e526](https://github.com/TaliaMarine/k8s-driller/commit/0b2e526a26a01d775ec88fa60b38bca292a392e9))

## [2.2.0](https://github.com/TaliaMarine/k8s-driller/compare/v2.1.3...v2.2.0) (2026-09-04)


### Features

* support oidc.clientIdRef, so client ID can come from a Secret too ([fd6b98c](https://github.com/TaliaMarine/k8s-driller/commit/fd6b98cfcb3fc801ff8052af2373e96676616855))

## [2.1.3](https://github.com/TaliaMarine/k8s-driller/compare/v2.1.2...v2.1.3) (2026-09-04)


### Bug Fixes

* eliminate an OIDC-login outage and make the chart GitOps-safe ([fb1c12f](https://github.com/TaliaMarine/k8s-driller/commit/fb1c12f7ed90438f2449eca7f7a78c8090554902))

## [2.1.2](https://github.com/TaliaMarine/k8s-driller/compare/v2.1.1...v2.1.2) (2026-09-02)


### Bug Fixes

* stable ordering, instant SSE snapshots, and a real node-drilldown UX pass ([dd4647d](https://github.com/TaliaMarine/k8s-driller/commit/dd4647d98870677d899f93a9d33d37fa8de9d1f1))

## [2.1.1](https://github.com/TaliaMarine/k8s-driller/compare/v2.1.0...v2.1.1) (2026-09-02)


### Bug Fixes

* use numeric USER in Dockerfile so kubelet can verify non-root ([cf0f8e3](https://github.com/TaliaMarine/k8s-driller/commit/cf0f8e3881384b296a55736a3ae37997c80c6c9c))

## [2.1.0](https://github.com/TaliaMarine/k8s-driller/compare/v2.0.1...v2.1.0) (2026-09-02)


### Features

* publish the Helm chart as an OCI artifact alongside the image ([22df6fa](https://github.com/TaliaMarine/k8s-driller/commit/22df6fac9e1b3089643dc40a9c080fdc78d532c2))


### Bug Fixes

* bump remaining CI actions off deprecated Node.js 20 runtime ([27961fc](https://github.com/TaliaMarine/k8s-driller/commit/27961fc6f2b1cc80721345dce66096ded9120eed))

## [2.0.1](https://github.com/TaliaMarine/k8s-driller/compare/v2.0.0...v2.0.1) (2026-09-02)


### Bug Fixes

* rename CRD group off the reserved driller.k8s.io suffix ([785ff5f](https://github.com/TaliaMarine/k8s-driller/commit/785ff5fa4f2053de69108d53c12ef502079f366b))

## [2.0.0](https://github.com/TaliaMarine/k8s-driller/compare/v1.0.0...v2.0.0) (2026-09-02)


### ⚠ BREAKING CHANGES

* cut the first versioned release

### Features

* add release-please and version the chart/image off real releases ([dc5fa74](https://github.com/TaliaMarine/k8s-driller/commit/dc5fa747d389b5b3eb2ad06ba8fa5078c9d0a03e))
* cut the first versioned release ([12392c1](https://github.com/TaliaMarine/k8s-driller/commit/12392c1aed571b00fa014554299a99268f3ab1bd))


### Bug Fixes

* correct ghcr image tags, published v1.0.0 as k8s-driller-v1.0.0 ([dcdadcc](https://github.com/TaliaMarine/k8s-driller/commit/dcdadccfbe250484e40ede109dbc303d99995b14))
* install pinact directly instead of via pinact-action ([ca51803](https://github.com/TaliaMarine/k8s-driller/commit/ca51803a1741c35c0bddbd78d1c626bb9a71245c))

## [1.0.0](https://github.com/TaliaMarine/k8s-driller/compare/k8s-driller-v0.1.0...k8s-driller-v1.0.0) (2026-09-02)


### ⚠ BREAKING CHANGES

* cut the first versioned release

### Features

* add release-please and version the chart/image off real releases ([dc5fa74](https://github.com/TaliaMarine/k8s-driller/commit/dc5fa747d389b5b3eb2ad06ba8fa5078c9d0a03e))
* cut the first versioned release ([12392c1](https://github.com/TaliaMarine/k8s-driller/commit/12392c1aed571b00fa014554299a99268f3ab1bd))
