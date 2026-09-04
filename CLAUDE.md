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
- `TRUSTD_DB_REGION` is set from `ccoRds.region`
- `database.password` becomes optional (omitted from env vars)
- SSL mode is forced to `require` (RDS IAM auth mandates TLS)
- For **mint/passthrough/default** modes: `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` are populated from the CCO secret
- For **manual** (STS) mode: the existing STS volumes/env vars are sufficient — no extra credentials needed

When `ccoRds.enabled` is false or absent, behavior is unchanged — `database.password` is required and SSL mode uses the configured value.

**Note:** The `create-database` and `create-importers` init jobs use `psql` directly and cannot generate RDS IAM tokens. These jobs continue requiring static credentials via `createDatabase.database`. The `migrate-database` job runs `trustd db migrate` and supports RDS IAM auth.

### Key Files

| File | Purpose |
|------|---------|
| `helm-charts/.../templates/credentialrequest.yaml` | CredentialsRequest template (conditional on `cloudProvider`) |
| `helm-charts/.../templates/helpers/_cco.tpl` | Helper templates for manual mode volumes/mounts/env vars and RDS IAM auth |
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
