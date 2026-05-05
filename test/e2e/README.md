# End-to-End Tests

This directory contains end-to-end tests for the RHTPA Operator.

## Prerequisites

To run e2e tests, you need:
- A Kubernetes cluster (can be local like kind, k3d, or CRC)
- kubectl configured to access the cluster
- The operator deployed to the cluster

## Running E2E Tests

```bash
# Install CRDs
make install

# Deploy the operator
make deploy

# Run e2e tests
go test -v ./test/e2e/...

# Cleanup
make undeploy
```

## Test Structure

E2E tests should:
1. Deploy a TrustedProfileAnalyzer CR
2. Wait for reconciliation
3. Verify expected resources are created
4. Test functionality
5. Clean up resources

## Available Test Files

### Core Tests
- **`suite_test.go`** - Test suite setup, cluster verification, cleanup
- **`helpers_test.go`** - Common helper functions and utilities
- **`cr_lifecycle_test.go`** - CR creation, update, deletion, validation
- **`reconciliation_test.go`** - Reconciliation behavior and idempotency
- **`helm_rendering_test.go`** - Helm chart rendering with various configs
- **`upgrade_test.go`** - Operator upgrade scenarios
- **`operator_deployment_test.go`** - Operator deployment verification

### Advanced Tests
- **`error_handling_test.go`** - Invalid configurations, edge cases, error recovery
- **`operator_health_test.go`** - Health checks, metrics, probes, leader election
- **`performance_test.go`** - Load testing, concurrent operations, performance validation
- **`module_configuration_test.go`** - Server, importer, database job configurations

## Running Specific Test Categories

```bash
# Run only CR lifecycle tests
go test -v ./test/e2e -run TestCR

# Run only error handling tests
go test -v ./test/e2e -run TestCR.*Invalid

# Run only performance tests
go test -v ./test/e2e -run TestMultiple

# Run only operator health tests
go test -v ./test/e2e -run TestOperator

# Skip long-running tests
go test -v -short ./test/e2e/...
```

## Test Fixtures

Test fixtures are located in `../fixtures/`:
- `minimal_cr.yaml` - Minimal valid CR configuration
- `valid_cr.yaml` - Standard valid CR with common options
- `full_cr.yaml` - Full-featured CR with all options
- `invalid_cr.yaml` - Invalid CR for negative testing
- `server_only_cr.yaml` - Server module only
- `importer_only_cr.yaml` - Importer module only

## Troubleshooting

### Tests Fail with "cluster not accessible"
Ensure your kubeconfig is set up correctly:
```bash
kubectl cluster-info
```

### Tests Timeout
Increase timeout:
```bash
go test -v -timeout 30m ./test/e2e/...
```

### CRD Not Found
Install CRDs first:
```bash
make install
```

### Operator Not Found
Deploy the operator:
```bash
make deploy
```

### Cleanup After Failed Tests
Delete test namespaces manually:
```bash
kubectl delete namespace -l e2e-test=true
# Or delete specific namespaces
kubectl get ns | grep e2e-test | awk '{print $1}' | xargs kubectl delete ns
```
