# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **Kubernetes Helm Operator** that manages the deployment and lifecycle of Red Hat Trusted Profile Analyzer (RHTPA) on OpenShift. It's built using the Operator SDK with the Helm plugin framework.

**Key Facts:**
- Uses the Helm Operator pattern (operator-framework/helm-operator-plugins)
- Manages a single Custom Resource: `TrustedProfileAnalyzer` (group: rhtpa.io/v1)
- Written in Go 1.26
- Deploys via Helm charts located in `helm-charts/redhat-trusted-profile-analyzer/`
- The operator reconciles the CRD by applying Helm chart templates

## Architecture

### Operator Structure

The operator uses a **watch-based reconciliation** pattern:

1. **Main Entry Point** (`main.go`): Sets up the controller manager and loads watches from `watches.yaml`
2. **Watches Configuration** (`watches.yaml`): Defines which CRDs to watch and which Helm charts to apply
3. **Helm Chart** (`helm-charts/redhat-trusted-profile-analyzer/`): Contains templates for all RHTPA components
4. **CRD Definition** (`config/crd/bases/rhtpa.io_trustedprofileanalyzers.yaml`): Defines the TrustedProfileAnalyzer resource

### Key Components Managed by Helm Chart

The Helm chart deploys multiple modules (configurable via `.spec.modules`):
- **Server**: Main RHTPA server deployment
- **Importer**: Imports security data (SBOMs, CSAFs, CVEs, OSV)
- **Database Jobs**: Create/migrate database
- **Importer Jobs**: Create importers for various data sources (Red Hat SBOMs, CSAF, CVE, OSV, Quay)

The operator reconciles the CRD by rendering the Helm chart with values from the CR's `.spec` field.

## Development Commands

### Building and Testing

```bash
# Format code
make fmt

# Lint code
make vet

# Run tests
make test

# Generate manifests (CRDs, RBAC)
make manifests

# Generate DeepCopy code
make generate
```

### Container Image Management

```bash
# Build operator image (default uses podman)
make podman-build

# Push operator image
make podman-push

# Override image tag
make podman-build IMG=quay.io/yourusername/rhtpa-rhel10-operator:latest
```

### Bundle Management (OLM)

```bash
# Generate OLM bundle
make bundle VERSION=1.1.1

# Build bundle image
make bundle-build

# Push bundle image
make bundle-push
```

### Local Development with CRC

See `devel/README.md` for detailed CRC setup. Quick overview:

```bash
# Start CRC cluster
crc start --cpus 8 --memory 32768 --disk-size 80

# Deploy infrastructure (PostgreSQL, Keycloak, OpenTelemetry)
# First, clone https://github.com/trustification/trustify-helm-charts/
NAMESPACE=trustify
APP_DOMAIN=-$NAMESPACE.$(oc -n openshift-ingress-operator get ingresscontrollers.operator.openshift.io default -o jsonpath='{.status.domain}')
helm upgrade --install --dependency-update -n $NAMESPACE infrastructure charts/trustify-infrastructure \
  --values devel/values-ocp-no-aws-crc.yaml \
  --set-string keycloak.ingress.hostname=sso$APP_DOMAIN \
  --set-string appDomain=$APP_DOMAIN

# Deploy operator bundle
operator-sdk run bundle -n trustify <bundle-image>

# Create TrustedProfileAnalyzer instance
kubectl apply -f devel/trusted-profile-analyzer-demo.yaml
```

### Deployment

```bash
# Install CRDs
make install

# Uninstall CRDs
make uninstall

# Deploy operator to cluster
make deploy

# Undeploy operator
make undeploy

# Run operator locally against configured cluster
make run
```

## Configuration

### Important Files

- **`watches.yaml`**: Maps the TrustedProfileAnalyzer CRD to the Helm chart path
  - `MaxConcurrentReconciles: 4` - controls parallelism
  - `WatchDependentResources: false` - operator doesn't watch chart-created resources

- **`Makefile`**:
  - `VERSION`: Operator version (default: 1.1.1)
  - `IMAGE_TAG_BASE`: Container registry path
  - `BUILDER`: Container tool (podman or docker)
  - `OPERATOR_SDK_VERSION`: v1.42.0

- **`helm-charts/redhat-trusted-profile-analyzer/values.yaml`**: Default Helm values
  - Must set `appDomain` for deployments
  - Module-based architecture with `modules.server`, `modules.importer`, etc.

### Custom Resource Spec

The TrustedProfileAnalyzer CR spec uses `x-kubernetes-preserve-unknown-fields: true`, meaning it accepts arbitrary fields that are passed through to the Helm chart. Key fields:

- `appDomain`: Required, sets ingress domain
- `modules.server.enabled`: Enable/disable server component
- `modules.importer.enabled`: Enable/disable importer component
- `modules.createDatabase.enabled`: Run database creation job
- `modules.migrateDatabase.enabled`: Run database migration job
- `modules.createImporters.enabled`: Create importer jobs
- `oidc.clients.frontend`: OIDC frontend configuration
- `database`: Database connection settings
- `storage`: Storage configuration
- `metrics.enabled`: Enable metrics collection
- `tracing.enabled`: Enable distributed tracing

## TLS Configurator & Post-Quantum Cryptography (PQC)

### How the TLS Configurator is wired in

The chart ships an optional `tlsConfigurator` module (disabled by default). It is
external tooling packaged as a container image, not part of this operator's Go
code; the source lives in the sibling `tls-configurator` repo (see its
`CLAUDE.md`).

- Toggle: `modules.tlsConfigurator.enabled` (`values.yaml`, default `false`)
- Image: `modules.tlsConfigurator.image.fullName`
  (dev default: `quay.io/mdessi/tls-configurator:latest`)
- Rendered resources live under
  `helm-charts/redhat-trusted-profile-analyzer/templates/init/tls-configure/`:
  ServiceAccount (`010`), ClusterRole (`015`), namespaced Role (`016`),
  RoleBinding (`017`), ClusterRoleBinding (`018`), and a **Deployment** (`020`).

**Architecture change (runtime reconciliation).** The module used to be a Helm
`pre-install,pre-upgrade` hook **Job** that ran `--action=update` once. It is now
a long-running **Deployment** that runs `--action=reconcile`: it reconciles once
on startup (covering the old install-time behaviour) and then watches the
cluster-wide TLS profile so a change **at runtime** rolls the affected
workloads. The RBAC resources are therefore plain (non-hook) objects that live
for the lifetime of the release, in `.Release.Namespace`.

The Deployment invokes:

```
--action=reconcile
--enable-pqc={{ .Values.modules.tlsConfigurator.pqc.enabled }}
--target-namespace={{ .Release.Namespace }}
--target-deployments={{ join "," .Values.modules.tlsConfigurator.targetDeployments }}
--resync-period={{ .Values.modules.tlsConfigurator.resyncPeriod }}
```

### Runtime update flow (TLS change → workload rollout)

1. The reconciler watches the cluster `APIServer` CR (`cluster`)
   `.spec.tlsSecurityProfile` — the authoritative cluster-wide TLS config.
2. On any change it computes a hash of the effective (optionally post-quantum)
   TLS config and compares it to each target Deployment's
   `rhtpa.io/tls-config-hash` pod-template annotation.
3. Deployments whose hash differs are patched, changing the pod template and
   triggering a **rolling restart** so pods re-read the new TLS settings. This
   reuses the same idea as the chart's existing `configHash/auth` annotation on
   the server Deployment. Kubernetes does not restart pods on ConfigMap/Secret
   change by itself, so this explicit hash bump is required.
4. `rhtpa.io/tls-config-hash` is intentionally **not** in the Helm templates so
   the operator's periodic re-render does not fight the reconciler.

`targetDeployments` (default `[server]`) must match the rendered Deployment
names of the TLS-serving workloads (the server Deployment renders as `server`).

### Enabling Post-Quantum Cryptography

PQC in TLS 1.3 is delivered through the hybrid **key-exchange group**
`X25519MLKEM768` (X25519 + ML-KEM-768, NIST FIPS 203) — **not** through the
cipher suites, which stay the same. Hybrid PQC key exchange requires TLS 1.3.

**Did the `tls-configurator` image need updating? Yes — and it has been.** PQC
support (a `--enable-pqc` flag, `CurvePreferences=[X25519MLKEM768, X25519]`,
forced TLS 1.3, a `validate` action) and the runtime `reconcile` mode were added
in the `tls-configurator` repo; the image must be **rebuilt/republished** for
the operator to consume it (the chart pins it by tag). One limitation remains:
the pinned OpenShift API cannot store a key-exchange group on
`TLSSecurityProfile`, so router-level PQC needs an `openshift/api` bump — see
`tls-configurator/CLAUDE.md`.

To enable from the operator side:

1. Point `modules.tlsConfigurator.image.fullName` at the rebuilt PQC-capable
   image and set `modules.tlsConfigurator.enabled: true`.
2. Set `modules.tlsConfigurator.pqc.enabled: true`.
3. Confirm `modules.tlsConfigurator.targetDeployments` lists the TLS-serving
   Deployments to roll on change.

### RBAC

The reconciler needs: `watch` on `config.openshift.io/apiservers`, `get,list` on
`clusterversions`, `get,list,watch,update,patch` on
`operator.openshift.io/ingresscontrollers` (ClusterRole `015`), and
`get,list,watch,update,patch` on `apps/deployments` in the release namespace
(Role `016`). These are provided by the chart.

Because this is a Helm operator, it can only *grant* permissions it holds
itself (RBAC escalation prevention). `config/rbac/role_cluster_rbac_manager.yaml`
(`rhtpa-rbac-manager`) was therefore widened to also hold `apiservers` (watch),
`ingresscontrollers`, `apps/deployments`, and namespaced `roles`/`rolebindings`
so the operator can create the reconciler's RBAC and Deployment.

**Follow-up / known issue:** `config/rbac/role_cluster_tlsconfigurator.yaml` +
`role_binding_tlsconfigurator.yaml` + the `tls-configurator` entry in
`config/rbac/service_account.yaml` are static bundle copies from the old hook-Job
design. They now duplicate (by name) the ClusterRole/ClusterRoleBinding/SA the
Helm chart creates, and the static binding still targets
`openshift-ingress-operator` while the reconciler SA now lives in the release
namespace. Decide whether to remove the static copies (let the chart own them) or
keep them as the bundle grant — this is a packaging call left open on purpose.

## Linting

The project uses golangci-lint with configuration in `.golangci.yml`. Enabled linters include:
- Standard Go tools: `gofmt`, `goimports`, `govet`, `staticcheck`
- Code quality: `dupl`, `errcheck`, `goconst`, `gocyclo`, `ineffassign`, `unused`
- Best practices: `revive`, `gosimple`

Run linting: `golangci-lint run` (not wrapped in Makefile)

## Release Process

1. Update `VERSION` in Makefile
2. Build and push operator image: `make podman-build podman-push`
3. Update bundle: `make bundle`
4. Build and push bundle: `make bundle-build bundle-push`
5. The bundle contains OLM metadata with channels: `stable`, `stable-v1.0`, `stable-v1.1`
6. Default channel: `stable-v1.1`

## Testing Strategy

The operator is tested via:
1. Unit tests: `make test` (requires envtest)
2. Integration testing with actual clusters (CRC or OpenShift)
3. The Helm chart itself should be tested separately

## Common Issues and Notes

- **Image Registry**: By default uses `registry.redhat.io/rhtpa/`. For development, override with your own registry or use ImageDigestMirrorSet on OpenShift (see `devel/README.md`)
- **Dependencies**: The operator requires infrastructure components (PostgreSQL, Keycloak) deployed separately via the trustify-infrastructure Helm chart
- **Reconcile Period**: Default is 1 minute, configurable per watch in `watches.yaml`
- **Resource Requirements**: Server and Importer default to 1 CPU / 8Gi memory each
