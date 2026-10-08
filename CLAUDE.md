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

## Cloud Credential Operator (CCO) Integration

The operator supports OpenShift Cloud Credential Operator integration for automatic cloud credential provisioning. CCO eliminates the need to manually create and manage S3 access keys by delegating credential lifecycle to the platform.

### Enabling CCO

Set `cloudProvider` in the CR spec. This is the master toggle — when absent, no CCO resources are created and credentials must be supplied manually via `storage.accessKey`/`storage.secretKey`.

```yaml
spec:
  cloudProvider: aws        # "aws" or "gcp"
  ccoMode: mint             # optional: "default", "mint", "passthrough", or "manual"
  cloudCredentials:
    aws:
      statementEntries:
        - effect: Allow
          action: ["s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:ListBucket"]
          resource: "*"
```

When `cloudProvider` is set:
- A `CredentialsRequest` resource is created in the `openshift-cloud-credential-operator` namespace
- CCO provisions a Secret named `<release-name>-cloud-creds` in the deployment namespace
- `storage.accessKey` and `storage.secretKey` become **optional** (auto-populated from the CCO secret)

### Disabling CCO

Remove or leave `cloudProvider` unset in the CR spec. When unset:
- No `CredentialsRequest` is created
- No CCO volumes or environment variables are injected into pods
- S3 credentials must be provided explicitly via `storage.accessKey` and `storage.secretKey`

### CCO Modes

| Mode | `ccoMode` value | Description |
|------|----------------|-------------|
| **Default** | `default` | CCO auto-determines provisioning method. |
| **Mint** | `mint` | CCO creates new IAM credentials with least-privilege permissions from `statementEntries`. |
| **Passthrough** | `passthrough` | CCO copies cluster admin credentials to the target namespace. |
| **Manual** | `manual` | Credentials pre-provisioned via `ccoctl` tool (STS/WIF). Requires `stsIAMRoleARN` for AWS. |

### Manual Mode (STS)

Manual mode uses short-lived token-based authentication instead of static access keys. It requires additional configuration:

```yaml
spec:
  cloudProvider: aws
  ccoMode: manual
  cloudCredentials:
    aws:
      statementEntries:
        - effect: Allow
          action: ["s3:*"]
          resource: "*"
      stsIAMRoleARN: "arn:aws:iam::123456789012:role/trustify-s3-role"
```

When manual mode is active, the operator automatically:
- Mounts a projected ServiceAccount token at `/var/run/secrets/openshift/serviceaccount`
- Mounts the CCO credentials secret at `/var/run/secrets/cloud`
- Sets `AWS_SHARED_CREDENTIALS_FILE`, `AWS_WEB_IDENTITY_TOKEN_FILE`, and `AWS_ROLE_ARN` environment variables
- Omits `TRUSTD_S3_ACCESS_KEY` / `TRUSTD_S3_SECRET_KEY` (the AWS SDK uses STS instead)

### RDS IAM Authentication via CCO

The operator supports using CCO-provisioned credentials for RDS IAM authentication, eliminating the need for static database passwords. This is controlled by `ccoRds.enabled` and requires `cloudProvider` to be set.

```yaml
spec:
  cloudProvider: aws
  ccoRds:
    enabled: true
    region: us-east-1
  cloudCredentials:
    aws:
      statementEntries:
        - effect: Allow
          action: ["s3:*", "rds-db:connect"]
          resource: "*"
  database:
    host: mydb.cluster-xyz.us-east-1.rds.amazonaws.com
    name: trustify
    username: trustify_user
    # password is NOT required when ccoRds.enabled is true
```

When `ccoRds.enabled` is true:
- `TRUSTD_DB_IAM_AUTH=true` is set on all trustd pods
- `TRUSTD_DB_IAM_REGION` is set from `ccoRds.region`
- `database.password` becomes optional (omitted from env vars)
- SSL mode is forced to `require` (RDS IAM auth mandates TLS)
- For **mint/passthrough/default** modes: `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` are populated from the CCO secret
- For **manual** (STS) mode: the existing STS volumes/env vars are sufficient — no extra credentials needed

When `ccoRds.enabled` is false or absent, behavior is unchanged — `database.password` is required and SSL mode uses the configured value.

**Init jobs:** `migrate-database` runs `trustd db migrate`, which speaks RDS IAM auth natively. The `create-database` and `create-importers` jobs shell out to `psql`, which cannot mint an IAM token, so when the connection they make uses IAM auth the chart prepends an `rds-auth-token` init container that mints the token onto a shared in-memory volume; the job is then wrapped in `bash` so it can read that file into `PGPASSWORD`. That init container runs the operator image itself (it carries the `cmd/rds-auth-token` helper).

**Where the helper image comes from (optional, configurable):** the chart resolves it from `ccoRds.tokenImage` first, falling back to `ccoRds.defaultTokenImage`, which the operator injects from the `RELATED_IMAGE_RDS_AUTH_TOKEN` environment variable via `overrideValues` in `watches.yaml`. Rendering fails only when a connection actually needs a token and both are empty.

That environment variable is added by the optional `config/rds-auth-token` kustomize component, enabled from the `# [RDS_AUTH_TOKEN]` block in `config/default/kustomization.yaml`:

| To… | Do this |
|-----|---------|
| Disable it | Comment out the `[RDS_AUTH_TOKEN]` block in `config/default/kustomization.yaml`, then `make bundle`. The env var and the `rds-auth-token` `relatedImages` entry disappear; clusters that need the helper set `ccoRds.tokenImage` in the CR. |
| Pin a different image cluster-wide | Edit the value in `config/rds-auth-token/manager_rds_auth_token_patch.yaml` and comment out the `replacements` block in that component (it otherwise forces the manager image), or set `spec.config.env` on the OLM Subscription. |
| Pin a different image per instance | Set `ccoRds.tokenImage` in the CR — a user value always wins over the operator-injected one. |

Because the override always wins over the CR, the operator writes to `defaultTokenImage` and never to `tokenImage`; leaving the user key free is what makes the per-instance override possible.

**The whole mechanism is opt-in.** A connection that does not use IAM auth renders exactly as it always did: `psql` exec'd directly with the SQL as an argument, no init container, no shell wrapper, no token volume, `PGPASSWORD` from the configured password. Deployments that are not on AWS, or not using `ccoRds`, are unaffected.

**Per-connection opt-out:** IAM auth is selected per database connection, not globally, and `iamAuth` is the switch. Any database block (`database`, `createDatabase`, `modules.createImporters.database`) may set `iamAuth: false` to keep password authentication while `ccoRds.enabled` is true, or `iamAuth: true` to opt a single connection in. This matters for `create-database`, whose bootstrap connection is typically the RDS master user on password auth, while the application role it creates is granted `rds_iam`.

### Key Files

| File | Purpose |
|------|---------|
| `helm-charts/.../templates/credentialrequest.yaml` | CredentialsRequest template (conditional on `cloudProvider`) |
| `helm-charts/.../templates/helpers/_cco.tpl` | Helper templates for manual mode volumes/mounts/env vars, RDS IAM auth, and the `rds-auth-token` init container |
| `cmd/rds-auth-token/main.go` | Helper binary shipped in the operator image; mints an RDS IAM token to a file |
| `helm-charts/.../templates/helpers/_storage.tpl` | S3 env vars — branches on CCO vs manual credentials |
| `helm-charts/.../templates/helpers/_postgres.tpl` | Database env vars — branches on ccoRds for password/SSL |
| `config/rbac/clusterrole.yaml` | ClusterRole granting access to `credentialsrequests` API |
| `config/rbac/clusterrolebinding_cco.yaml` | Binds the CCO ClusterRole to the operator ServiceAccount |
| `test/fixtures/aws_cco_*.yaml` | Example CRs for each CCO mode |
| `test/fixtures/aws_cco_rds_cr.yaml` | Example CR for RDS IAM auth (mint mode) |
| `test/fixtures/aws_cco_manual_rds_cr.yaml` | Example CR for RDS IAM auth (manual/STS mode) |
| `test/e2e/cco_helm_rendering_test.go` | E2E tests for CCO template rendering |

### RBAC

The operator requires a ClusterRole with permissions on `cloudcredential.openshift.io/credentialsrequests` (create, delete, get, list, patch, update, watch). This is configured in `config/rbac/clusterrole.yaml` and bound via `config/rbac/clusterrolebinding_cco.yaml`.

## TLS Configurator & Post-Quantum Cryptography (PQC)

### How the TLS Configurator is wired in

Detailed documentation lives in `docs/tls-configurator/`:

- `DEPLOYMENT.md` — per-platform configuration (OpenShift 4.22+, OpenShift
  < 4.22, plain Kubernetes), verification and troubleshooting. `devel/README.md`
  has the runnable CRC version.
- `TLS_ADHERENCE.md` — how the component implements Red Hat's *TLS Profile
  Compliance — Implementation Reference*.
- `FINAL_PROJECT_STATUS.md` — component inventory (predates the adherence
  work).

The chart ships an optional `tlsConfigurator` module (disabled by default).

**The configurator now lives in this repo and ships in the operator image.** It
was previously an external component built from the sibling
`tls-openshift-configurator` repo and published as its own image. Its source is
now:

```
cmd/tls-configurator/        CLI entry point, flag parsing, action dispatch
pkg/tlsconfigurator/
  client/                    IngressController, APIServer, ClusterVersion, Deployment clients
  config/                    Config building / kubeconfig handling
  controller/                One-shot TLS controller orchestration
  crypto/                    OpenShift TLSSecurityProfile -> crypto/tls.Config (+ PQC)
  reconcile/                 Long-running runtime reconciler (watch -> roll workloads)
test/tlsconfigurator/        Ginkgo/Gomega integration suite
```

Both `Dockerfile` and `Dockerfile.rhtpa-operator.rh` build a second binary
alongside `manager` and install it at `/usr/local/bin/tls-configurator`. The
image `ENTRYPOINT` remains `/manager`, so the configurator Deployment selects it
with an explicit `command:`. `make build-tls-configurator` builds it standalone.

- Toggle: `modules.tlsConfigurator.enabled` (`values.yaml`, default `false`)
- Image: `modules.tlsConfigurator.image.fullName` — the **operator's own image**.
  `watches.yaml` sets `overrideValues` to expand
  `$RELATED_IMAGE_TLS_CONFIGURATOR` into this key, so the configurator always
  runs the exact digest of the operator that rendered the chart.
  `RELATED_IMAGE_TLS_CONFIGURATOR` is set on the manager container
  (`config/manager/manager.yaml`, and the generated CSV) and mirrored into the
  CSV `relatedImages` under the name `tls-configurator` so disconnected installs
  pull it. **When the operator image digest changes, all three must move
  together**: the manager `image:`, the env var, and the `relatedImages` entry.
- Rendered resources live under
  `helm-charts/redhat-trusted-profile-analyzer/templates/init/tls-configure/`:
  ServiceAccount (`010`), ClusterRole (`015`), namespaced Role (`016`),
  RoleBinding (`017`), ClusterRoleBinding (`018`), and a **Deployment** (`020`).

Sharing an image does **not** mean sharing a pod: the configurator still runs as
its own Deployment with its own ServiceAccount and cluster RBAC.

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

### Upstream packages (do not reimplement these)

Profile resolution and `tls.Config` construction are delegated to the packages
Red Hat documents for this, not hand-rolled:

- `github.com/openshift/controller-runtime-common/pkg/tls` —
  `GetTLSProfileSpec`, `NewTLSConfigFromProfile`, `SetNextProtos`,
  `APIServerName`.
- `github.com/openshift/library-go/pkg/crypto` — TLS version and cipher name
  conversion, and `ShouldHonorClusterTLSProfile`.

`pkg/tlsconfigurator/crypto` is a thin adapter over those two. It must not
regrow local cipher-name tables or per-profile cipher lists: the built-in
profiles live in `configv1.TLSProfiles` and already carry `Groups`, including
`X25519MLKEM768`. `BuildTLSConfig` returns an `unsupported []string` rather
than erroring on ciphers Go cannot offer — a cluster profile may legitimately
name OpenSSL-only suites, and failing there would stop reconciliation.

### `tlsAdherence`

`APIServer.spec.tlsAdherence` (feature gate `TLSAdherence`) says whether
components *must* honor the cluster profile. The configurator honors it in
every mode — it has no competing TLS settings of its own — but the policy is
part of the rollout hash, so tightening it to `StrictAllComponents` rolls the
workloads even when the profile is unchanged. It is logged once per distinct
value and reported by `--action=get-adherence`, `get-cluster`,
`show-tlsconfig` and `validate`.

### Runtime update flow (TLS change → workload rollout)

1. The reconciler watches the cluster `APIServer` CR (`cluster`) — both
   `.spec.tlsSecurityProfile` and `.spec.tlsAdherence`, the authoritative
   cluster-wide TLS config.
2. On any change it computes a hash of the **resolved** `TLSProfileSpec` plus
   the adherence policy and the PQC flag, and compares it to each target
   Deployment's `rhtpa.io/tls-config-hash` pod-template annotation. Hashing the
   resolved spec means equivalent spellings (unset vs. explicit
   `Intermediate`, a `Custom` profile that restates a built-in) do not churn
   the workloads.
3. Deployments whose hash differs are patched, changing the pod template and
   triggering a **rolling restart** so pods re-read the new TLS settings. This
   reuses the same idea as the chart's existing `configHash/auth` annotation on
   the server Deployment. Kubernetes does not restart pods on ConfigMap/Secret
   change by itself, so this explicit hash bump is required.
4. `rhtpa.io/tls-config-hash` is intentionally **not** in the Helm templates so
   the operator's periodic re-render does not fight the reconciler.

`targetDeployments` (default `[server]`) must match the rendered Deployment
names of the TLS-serving workloads (the server Deployment renders as `server`).

### When the module must be on, and when it must be off

The module is OpenShift-only, and the chart *enforces* the matrix rather than
just documenting it:

| Platform | `modules.tlsConfigurator.enabled` | Enforced by |
| --- | --- | --- |
| OpenShift >= 4.22 | **required `true`** | install fails unless `allowDisabled: true` |
| OpenShift < 4.22 | optional | nothing — `reconcile` has no version gate, so it runs fine, it is just not mandatory |
| plain Kubernetes | **required `false`** (the default) | install fails if enabled |

The deeper reason for the plain-Kubernetes row is not just the missing
`config.openshift.io/v1` API. `trustification.openshift.useServiceCa` resolves
to `false` off OpenShift, so the chart never sets `HTTP_SERVER_TLS_ENABLED` and
the server listens on **plain HTTP** — TLS terminates at the ingress
controller. There is no `tls.Config` in the RHTPA pods for a cluster profile to
apply to, which is the reference's "plaintext behind a TLS-terminating router:
out of scope". User-facing per-platform configuration (including where TLS
*is* configured on plain Kubernetes) is in
`docs/tls-configurator/DEPLOYMENT.md` and `devel/README.md`.

Enforcement lives in
`helm-charts/redhat-trusted-profile-analyzer/templates/init/tls-configure/000-validate.yaml`.
It renders no resources and is deliberately **not** gated on
`.enabled` — it has to run in the disabled case too. Details:

- Platform detection reuses the chart's existing
  `trustification.openshift.detect` helper (`route.openshift.io/v1` +
  `openshift.enabled`). Escape hatch for wrong detection:
  `openshift.enabled: true`.
- The 4.22 check uses `lookup "config.openshift.io/v1" "ClusterVersion" ""
  "version"` and parses `.status.desired.version`. `lookup` returns nothing
  under `helm template` / `--dry-run`, so the check is **skipped** there rather
  than failing on an unreadable version. Do not "fix" that by failing closed —
  it would break every dry run and the operator's own rendering paths.
- `modules.tlsConfigurator.allowDisabled` (default `false`) is consulted only
  when `enabled` is `false`. It is an explicit acknowledgement that a runtime
  TLS profile change will not roll the workloads.

User-facing versions of this live in the chart `README.md` and in
`values.yaml` comments; `values.schema.json` is the schema Helm actually
enforces, so `allowDisabled` had to be added there (the `.yaml` schema is the
source but is not what Helm reads).

### Enabling Post-Quantum Cryptography

PQC in TLS 1.3 is delivered through the hybrid **key-exchange group**
`X25519MLKEM768` (X25519 + ML-KEM-768, NIST FIPS 203) — **not** through the
cipher suites, which stay the same. Hybrid PQC key exchange requires TLS 1.3.

PQC support (a `--enable-pqc` flag, `CurvePreferences=[X25519MLKEM768, X25519]`,
forced TLS 1.3, a `validate` action) and the runtime `reconcile` mode are built
into the configurator. Because it now ships in the operator image, there is no
separate image to rebuild or republish — an operator build carries it.

**Groups come from the cluster profile.** `TLSProfileSpec.Groups []TLSGroup`
exists in the `openshift/api` this repo depends on, and the built-in `Old`,
`Intermediate` and `Modern` profiles all list `TLSGroupX25519MLKEM768` first.
Because the config is now built by `NewTLSConfigFromProfile`, those groups land
in `CurvePreferences` automatically — a cluster on `Modern` is post-quantum
without `--enable-pqc`.

**Router-level PQC is implemented, gated on `TLSGroupPreferences`.**
`--action=update --enable-pqc` with a `Custom` profile writes
`Groups: [X25519MLKEM768, X25519]` onto the IngressController profile. The
field is behind the `TLSGroupPreferences` feature gate and the API server
rejects it when the gate is off, so `controller.SupportsTLSGroups` reads the
`FeatureGate` CR first and the field is skipped (with a log line) on clusters
without it. `--enable-pqc` additionally forces TLS 1.3 locally, since hybrid
key exchange is only defined for TLS 1.3.

To enable from the operator side:

1. Set `modules.tlsConfigurator.enabled: true`. The image is the operator's own;
   no value needs pointing at a separate registry.
2. Set `modules.tlsConfigurator.pqc.enabled: true`.
3. Confirm `modules.tlsConfigurator.targetDeployments` lists the TLS-serving
   Deployments to roll on change.

### RBAC

The reconciler needs: `watch` on `config.openshift.io/apiservers`, `get,list` on
`clusterversions` and on `featuregates`, `get,list,watch,update,patch` on
`operator.openshift.io/ingresscontrollers` (ClusterRole `015`), and
`get,list,watch,update,patch` on `apps/deployments` in the release namespace
(Role `016`). These are provided by the chart. Reading `.spec.tlsAdherence`
needs nothing extra — it rides on the same `APIServer` get as the profile.

Because this is a Helm operator, it can only *grant* permissions it holds
itself (RBAC escalation prevention). `config/rbac/role_cluster_rbac_manager.yaml`
(`rhtpa-rbac-manager`) was therefore widened to also hold `apiservers` (watch),
`featuregates`, `ingresscontrollers`, `apps/deployments`, and namespaced
`roles`/`rolebindings` so the operator can create the reconciler's RBAC and
Deployment.

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
