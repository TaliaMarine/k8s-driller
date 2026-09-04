# Changelog

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
