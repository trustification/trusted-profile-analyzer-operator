# Test Suite Documentation

This directory contains comprehensive tests for the Trusted Profile Analyzer Operator.

## Test Structure

The test suite is organized into three main categories:

### 1. Unit Tests (`test/unit/`)
Unit tests that don't require cluster access or external dependencies.

**Test Files:**
- `config_test.go` - Tests for operator configuration values
- `validation_test.go` - Tests for watches file loading
- `fixtures_test.go` - Tests for CR fixture files

**Run unit tests:**
```bash
go test -v -short ./test/unit/...
```

### 2. Integration Tests (`test/integration/`)
Integration tests that verify file structures and configurations.

**Run integration tests (from project root):**
```bash
go test -v -short ./test/integration/...
```

### 3. E2E Tests (`test/e2e/`)
E2E tests requiring a running Kubernetes cluster.

**Run E2E tests:**
```bash
go test -v -short ./test/e2e/...  # Skips cluster tests
go test -v ./test/e2e/...          # Runs all E2E tests
```

## Test Statistics

- **Total test files**: 19
- **Total test functions**: 122+

## Running Tests

### All tests (short mode)
```bash
go test -v -short ./...
```

### Main operator tests
```bash
go test -v .
```

### With coverage
```bash
go test -v -short -cover ./...
```

## Test Fixtures

Located in `test/fixtures/`:
- `minimal_cr.yaml` - Minimal valid CR
- `valid_cr.yaml` - Full CR configuration
- `server_only_cr.yaml` - Server module only
- `importer_only_cr.yaml` - Importer module only
