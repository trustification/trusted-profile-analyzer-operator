# TLS Configurator — Status

**Last verified**: 2026-09-28 (branch `tls-configurator-merge`)
**Go**: 1.26.7 (`go.mod`)
**Build**: `go build ./...` clean · **Tests**: all `pkg/tlsconfigurator/...` and `test/tlsconfigurator` packages pass

> Supersedes the earlier version of this file, which described the standalone
> `tls-openshift-configurator` repository. That project has been merged into
> this operator; paths, packaging and the deployment model all changed. See
> "What changed in the merge" below.

---

## What this is

A TLS configurator for Red Hat Trusted Profile Analyzer on OpenShift. It reads
the cluster-wide TLS security profile, converts it to a Go `crypto/tls.Config`
(optionally post-quantum), and keeps the operator's TLS-serving workloads in
sync with it at runtime.

It is **not** a separate deliverable: the binary ships inside the operator
image at `/usr/local/bin/tls-configurator`, and the Helm chart runs it as its
own Deployment.

---

## Project evolution

| Phase | Outcome |
|---|---|
| 1. Initial implementation | Standalone CLI: IngressController TLS management, unit + integration tests, container build |
| 2. Toolchain refresh | Go 1.26, UBI10 base image, podman build/push |
| 3. OpenShift best practices (PR #316) | APIServer cluster-wide profile as source of truth, `crypto/tls.Config` conversion, OpenSSL→IANA cipher mapping |
| 4. Version-check guardrail | OpenShift 4.22+ detection and enforcement before updates |
| 5. PQC support | `--enable-pqc`: TLS 1.3-only + `X25519MLKEM768` hybrid key exchange, `validate` action |
| 6. Merge into the operator | Source moved under `pkg/tlsconfigurator/`, ships in the operator image, hook Job replaced by a long-running reconcile Deployment |

---

## Package layout

```
cmd/tls-configurator/main.go          CLI entry point, flag parsing, action dispatch
pkg/tlsconfigurator/
├── client/
│   ├── client.go        client_test.go    IngressController client
│   ├── apiserver.go                       APIServer CR client (+ watch)
│   ├── version.go       version_test.go   ClusterVersion / 4.22+ gate
│   └── workloads.go                       Deployment hash annotation client
├── config/
│   └── config.go        config_test.go    Config building, kubeconfig handling
├── controller/
│   └── controller.go    controller_test.go  One-shot TLS controller
├── crypto/
│   ├── crypto.go        crypto_test.go    TLSSecurityProfile -> crypto/tls.Config
│   └──                  pqc_test.go       Post-quantum coverage
└── reconcile/
    └── reconcile.go                       Long-running runtime reconciler
test/tlsconfigurator/suite_test.go         Ginkgo/Gomega integration suite
```

---

## CLI actions

| Action | Description | Notes |
|---|---|---|
| `get` | Get an IngressController's TLS profile | |
| `update` | Update an IngressController's TLS profile | Enforces the 4.22+ version gate first (`controller.go:88`) |
| `list` | List all IngressControllers | |
| `get-cluster` | Get the cluster-wide APIServer TLS profile | PR #316 |
| `show-tlsconfig` | Show the profile converted to `crypto/tls.Config` | PR #316 |
| `check-version` | Report the detected OpenShift version | |
| `validate` | Report PQC / TLS 1.3 compliance; exits non-zero when non-compliant | Exit code is the contract |
| `reconcile` | Long-running: watch the cluster TLS profile, roll target Deployments | Bypasses the one-shot controller (`main.go:86`) |

## Flags

| Flag | Purpose | Default |
|---|---|---|
| `--kubeconfig` | Path to kubeconfig | in-cluster |
| `--action` | Action to perform | `get` |
| `--ingress-controller` | IngressController name | `default` |
| `--namespace` | IngressController namespace | `openshift-ingress-operator` |
| `--type` | TLS profile type | `Custom` |
| `--min-tls-version` | Minimum TLS version | `VersionTLS13` |
| `--ciphers` | Comma-separated ciphers | — |
| `--enable-pqc` | Enforce TLS 1.3 + `X25519MLKEM768` | `false` |
| `--target-namespace` | Namespace of workloads to roll (reconcile mode) | — |
| `--target-deployments` | Comma-separated Deployment names to roll (reconcile mode) | — |
| `--resync-period` | Periodic drift-correction interval (reconcile mode) | `5m` |
| `--use-cluster-profile` | **Inert** — accepted, nothing reads it | `false` |
| `--skip-version-check` | **Inert** — accepted, nothing reads it | `false` |

The last two are flagged with a `TODO` in `main.go:48-57`. They are kept for CLI
compatibility; wiring them up or removing them is tracked separately. Note the
consequence: **the 4.22+ gate on `update` cannot currently be bypassed.**

---

## Runtime reconciliation

The chart deploys `tls-configurator` as a Deployment running `--action=reconcile`:

1. Reconcile once on startup (this covers what the old pre-install hook Job did).
2. Watch the cluster `APIServer` CR (`cluster`) `.spec.tlsSecurityProfile` — the
   authoritative cluster-wide TLS config.
3. On change, compute a SHA-256 hash of the effective profile **plus the PQC
   flag** (`reconcile.TLSConfigHash`), so toggling PQC also forces a rollout.
4. Compare against each target Deployment's `rhtpa.io/tls-config-hash`
   pod-template annotation; patch only those that differ, which triggers a
   rolling restart.
5. Re-establish the watch when the server closes it; a `--resync-period` ticker
   corrects drift independently.

The annotation is deliberately absent from the Helm templates so the operator's
periodic re-render does not fight the reconciler. An initial reconcile failure
is logged, not fatal — the watch/resync loop retries.

---

## Post-quantum cryptography

PQC in TLS 1.3 comes from the hybrid **key-exchange group** `X25519MLKEM768`
(X25519 + ML-KEM-768, NIST FIPS 203) — not from the cipher suites, which are
unchanged. It requires TLS 1.3.

Implemented in `pkg/tlsconfigurator/crypto`: `PQCCurvePreferences()`,
`ConvertTLSProfileWithPQC()`, `EnablePQC()`, `IsPQCCompliant()`, `CurveName()`.
`CurvePreferences` is set to `[X25519MLKEM768, X25519]`, keeping classical
X25519 as the fallback for peers without PQC.

Scope: this applies to Go services' `crypto/tls.Config` and to the rollout hash.
See "Open items" for the router-level gap.

---

## Packaging

- **Image**: the configurator has no image of its own. `Dockerfile` and
  `Dockerfile.rhtpa-operator.rh` build it alongside `manager` and install it at
  `/usr/local/bin/tls-configurator`. The image `ENTRYPOINT` stays `/manager`, so
  the Deployment selects the binary with an explicit `command:`.
- **Image wiring**: `watches.yaml` `overrideValues` expands
  `$RELATED_IMAGE_TLS_CONFIGURATOR` into
  `modules.tlsConfigurator.image.fullName`, so the configurator always runs the
  exact digest of the operator that rendered the chart. The var is set on the
  manager container (`config/manager/manager.yaml`, and the generated CSV) and
  mirrored into CSV `relatedImages` for disconnected installs. **When the
  operator digest changes, the manager `image:`, the env var and the
  `relatedImages` entry must move together.**
- **Chart module**: `modules.tlsConfigurator.enabled` (default `false`), with
  `pqc.enabled`, `targetDeployments` (default `[server]`) and `resyncPeriod`
  (default `5m`).
- **Rendered resources**: `templates/init/tls-configure/` — platform guard
  (`000`, renders nothing), ServiceAccount (`010`), ClusterRole (`015`), Role
  (`016`), RoleBinding (`017`), ClusterRoleBinding (`018`), Deployment (`020`).
  Everything from `010` on is a plain (non-hook) object living for the lifetime
  of the release in `.Release.Namespace`.

### Platform matrix

The module is OpenShift-only: the reconciler reads the cluster-wide TLS profile
from the `config.openshift.io` `APIServer` CR, which plain Kubernetes does not
have.

| Platform | `modules.tlsConfigurator.enabled` | Rationale |
| --- | --- | --- |
| OpenShift >= 4.22 | **required `true`** | The cluster-wide TLS profile can change at runtime and nothing else rolls the RHTPA workloads to pick it up. |
| OpenShift < 4.22 | optional | The `reconcile` action does not check the cluster version, so it works; it is just not mandatory. (The `update`/`validate` actions do gate on 4.22 via `ValidateMinimumVersion`, but the Deployment does not run those.) |
| plain Kubernetes | **required `false`** (default) | `config.openshift.io/v1` is absent; the reconciler would fail every attempt and retry forever. |

### Enabling it (OpenShift)

```yaml
modules:
  tlsConfigurator:
    enabled: true
    pqc:
      enabled: true
    targetDeployments:
      - server
    resyncPeriod: 5m
```

`targetDeployments` must match the rendered Deployment names of the TLS-serving
workloads (the server Deployment renders as `server`).

Under the operator, `image.fullName` is overridden with the operator's own
running image via `$RELATED_IMAGE_TLS_CONFIGURATOR` in `watches.yaml`, so
nothing needs pointing at a separate registry. The `values.yaml` default only
applies to a standalone `helm install`.

### Disabling it (plain Kubernetes)

`enabled: false` is the chart default, so a plain Kubernetes install needs no
action. Only set it explicitly if your values override it somewhere.

### Install-time enforcement

`templates/init/tls-configure/000-validate.yaml` renders no resources. It is
deliberately **not** gated on `.enabled` — it has to run in the disabled case
too — and fails the install when platform and setting do not match:

| Condition | Result | Override |
| --- | --- | --- |
| `enabled: true`, no OpenShift API detected | `fail` with an explanation | `openshift.enabled: true` if detection is wrong |
| `enabled: false`, cluster is OpenShift >= 4.22 | `fail` with an explanation | `modules.tlsConfigurator.allowDisabled: true` |

Implementation notes:

- Platform detection reuses the chart's existing
  `trustification.openshift.detect` helper (`route.openshift.io/v1` presence,
  or an explicit `openshift.enabled`).
- The version check reads the `ClusterVersion` CR with
  `lookup "config.openshift.io/v1" "ClusterVersion" "" "version"` and parses
  `.status.desired.version`. `lookup` returns nothing under `helm template` and
  `--dry-run`, so in those modes the version is unknown and the check is
  **skipped** rather than failing closed — otherwise every dry run would break.
- `allowDisabled` (default `false`) is consulted only when `enabled` is
  `false`. It is an explicit acknowledgement that a runtime TLS profile change
  will not roll the workloads.
- `allowDisabled` had to be added to `values.schema.json` as well as
  `values.schema.yaml` — the JSON file is what Helm actually enforces.

Verified against six `helm template` scenarios (plain/disabled, plain/enabled,
OpenShift/enabled, OpenShift/disabled, OpenShift/disabled+allowDisabled, and
the detection escape hatch); `helm lint` passes.

---

## RBAC

Cluster-scoped (`015-ClusterRole.yaml`):

```yaml
- apiGroups: ["config.openshift.io"]
  resources: ["apiservers"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["config.openshift.io"]
  resources: ["clusterversions"]
  verbs: ["get", "list"]
- apiGroups: ["operator.openshift.io"]
  resources: ["ingresscontrollers"]
  verbs: ["get", "list", "watch", "update", "patch"]
```

Namespaced (`016-Role.yaml`), in the release namespace:

```yaml
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "watch", "update", "patch"]
```

Because this is a Helm operator, it can only *grant* permissions it holds itself
(RBAC escalation prevention). `config/rbac/role_cluster_rbac_manager.yaml`
(`rhtpa-rbac-manager`) was therefore widened to also hold `apiservers` (watch),
`ingresscontrollers`, `apps/deployments`, and namespaced `roles`/`rolebindings`.

---

## Build and test

```bash
make build-tls-configurator   # standalone binary
make test                     # unit tests (requires envtest)
make podman-build             # operator image, which carries the configurator
```

There is no `make build` or standalone `make docker-build` for this component —
it is built as part of the operator image. Base image:
`registry.access.redhat.com/ubi10/ubi-minimal`; builder:
`registry.access.redhat.com/hi/go:1.26.7`.

### Test inventory

| Package | Top-level `Test*` funcs | Status |
|---|---|---|
| `pkg/tlsconfigurator/client` | 36 | ✅ pass |
| `pkg/tlsconfigurator/config` | 10 | ✅ pass |
| `pkg/tlsconfigurator/controller` | 13 | ✅ pass |
| `pkg/tlsconfigurator/crypto` | 30 | ✅ pass |
| `pkg/tlsconfigurator/reconcile` | 15 | ✅ pass |
| `test/tlsconfigurator` | 15 Ginkgo specs | ✅ pass |

`reconcile` splits into two files:

- `reconcile_test.go` — `TLSConfigHash`, the pure function behind the rollout
  decision: digest format, determinism across separately-constructed equal
  profiles (an unstable hash would roll the workloads on every reconcile), the
  PQC flag participating in the hash, and every profile change the reconciler is
  meant to react to producing a different hash.
- `reconcile_loop_test.go` — `Run`, `consumeWatch` and `reconcileOnce` driven by
  a fake OpenShift config clientset, a fake Kubernetes clientset and a
  `watch.FakeWatcher`. Covers: stamping, skipping up-to-date targets,
  idempotency, per-target error isolation, the Intermediate default, Added /
  Modified / Deleted / Error event handling, closed-channel and context-cancel
  exits, resync-tick rollouts, the startup reconcile, rollout on a runtime
  profile change, survival of an initial reconcile failure, and the
  watch-establishment retry path.

The loop tests queue events into a buffered fake watcher and close it before
running the loop, so a closed channel still drains its pending events — that
keeps `consumeWatch` assertions synchronous rather than timing-dependent. The
goroutine-driven `Run` tests poll for observable effects with a bounded timeout.

To inject the fakes, `client.NewAPIServerClientWithClientset` and
`client.NewWorkloadsClientWithClientset` build the client wrappers from an
existing clientset instead of a `*rest.Config`. Production code still goes
through the `*rest.Config` constructors.

---

## Version compatibility

Minimum OpenShift **4.22** (`client/version.go:31-33`), enforced in
`UpdateTLSProfile` before any change is applied. Lower versions are rejected
with a message naming the detected version. `check-version` reports it on
demand.

TLS versions `VersionTLS10`–`VersionTLS13` are accepted; `VersionTLS13` is the
CLI default and is required for PQC.

---

## Open items

1. **Router-level PQC is unblocked but not implemented.** The old blocker was
   that the pinned `openshift/api` exposed only `minTLSVersion` and `ciphers` on
   `TLSSecurityProfile`. The current dependency
   (`v0.0.0-20260924195948-0616345087ce`) adds `TLSProfileSpec.Groups []TLSGroup`
   including `TLSGroupX25519MLKEM768`, behind the `TLSGroupPreferences` feature
   gate. Nothing in `pkg/tlsconfigurator` writes that field yet. Remaining work:
   wire `Groups` into the `update` path and gate on `TLSGroupPreferences` being
   enabled on the cluster.
2. **Inert flags.** `--use-cluster-profile` and `--skip-version-check` are parsed
   but unread. Wire them up or remove them.
3. **Duplicate static bundle RBAC.** `config/rbac/role_cluster_tlsconfigurator.yaml`,
   `role_binding_tlsconfigurator.yaml` and the `tls-configurator` entry in
   `config/rbac/service_account.yaml` are leftovers from the hook-Job design.
   They duplicate by name the ClusterRole/ClusterRoleBinding/SA the chart
   creates, and the static binding still targets `openshift-ingress-operator`
   while the reconciler's SA now lives in the release namespace. Decide whether
   to drop them (chart owns them) or keep them as the bundle grant — a packaging
   call left open on purpose.

---

## What changed in the merge

For anyone reading the older docs in this directory
(`PR316_IMPLEMENTATION_SUMMARY.md`, `UPDATES_FROM_PR316.md`,
`VERIFICATION_REPORT.md`, `VERSION_CHECK_FEATURE.md`, `PROJECT_SUMMARY.md`),
these describe the standalone repo and are stale on the following points:

| Old doc says | Now |
|---|---|
| `pkg/crypto`, `pkg/client`, `test/suite` | `pkg/tlsconfigurator/...`, `test/tlsconfigurator` |
| Go 1.25.5 | Go 1.26.7 |
| UBI9 base | UBI10 base |
| `make build` / `make docker-build` | `make build-tls-configurator`; ships in the operator image |
| Image `openshift/tls-configurator:latest` | No separate image; operator's own digest |
| Deployed as a Job / CronJob | Deployment running `--action=reconcile` |
| Actions: 6 | 8 (adds `validate`, `reconcile`) |
| "Profile watcher not implemented" | Implemented (`pkg/tlsconfigurator/reconcile`) |
| "Hot reload not implemented; restart pods manually" | Automatic rolling restart via `rhtpa.io/tls-config-hash` |
| "Curves unsupported by `openshift/api` v0.0.0-20240830023148" | API now exposes `Groups`; the gap is that we don't write it |
| "Optional version-check bypass" | Bypass flag is inert; the gate always applies |

The PR #316 substance itself is intact: APIServer CR as source of truth,
`ConvertTLSProfile`, `TLSVersion`, `OpenSSLToIANACipherSuites`, `CipherSuites`,
`SecureTLSConfig`, and the `Get{Modern,Intermediate,Old}CipherSuites` helpers all
exist and are tested.
