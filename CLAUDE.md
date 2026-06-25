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
