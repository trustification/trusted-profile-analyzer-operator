# Updates Based on OpenShift TLS Profile Best Practices

**Source**: https://github.com/openshift-eng/ai-helpers/pull/316

## Summary of Changes

This document summarizes the updates made to align with OpenShift best practices for TLS profile implementation as outlined in PR #316 of the ai-helpers repository.

---

## New Features Added

### 1. **APIServer Client Support** ✅

Added support for reading cluster-wide TLS profiles from the `APIServer` CR (recommended approach):

**New Package**: `pkg/client/apiserver.go`
- `NewAPIServerClient()` - Create APIServer client
- `GetAPIServer()` - Get cluster APIServer resource
- `GetClusterTLSProfile()` - Get cluster-wide TLS profile
- `GetEffectiveTLSProfile()` - Get profile with defaults applied

**Why**: According to OpenShift best practices, the `APIServer` CR named "cluster" is the authoritative source for cluster-wide TLS security profiles.

### 2. **crypto/tls Conversion Utilities** ✅

Implemented conversion from OpenShift TLS profiles to Go's `crypto/tls.Config`:

**New Package**: `pkg/crypto/crypto.go`
- `ConvertTLSProfile()` - Convert OpenShift profile to tls.Config
- `TLSVersion()` - Map version strings to Go constants
- `OpenSSLToIANACipherSuites()` - Convert OpenSSL to IANA names
- `CipherSuites()` - Convert cipher names to Go constants
- `SecureTLSConfig()` - Apply secure baseline settings
- `GetDefaultTLSConfig()` - Get default (Intermediate) config

**Why**: Enables actual TLS configuration for webhook servers, metrics endpoints, and HTTP/GRPC services.

### 3. **New CLI Actions** ✅

Added commands following OpenShift best practices:

#### `--action get-cluster`
Retrieves the cluster-wide TLS profile from APIServer CR:
```bash
./bin/tls-configurator --action get-cluster
```

#### `--action show-tlsconfig`
Demonstrates conversion to crypto/tls.Config:
```bash
./bin/tls-configurator --action show-tlsconfig
```

**Output includes**:
- OpenShift TLS Security Profile (JSON)
- Converted crypto/tls.Config details
- TLS version, cipher suites, security settings
- Usage guidance

### 4. **Enhanced Documentation** ✅

Created comprehensive documentation:
- `IMPROVEMENTS_ANALYSIS.md` - Gap analysis and recommendations
- `UPDATES_FROM_PR316.md` - This file
- Updated inline code documentation
- Added usage examples

---

## Implementation Details

### APIServer Profile Reading Pattern

Following the recommended pattern from the PR:

```go
// 1. Create APIServer client
apiServerClient, err := client.NewAPIServerClient(k8sConfig)

// 2. Get cluster TLS profile
profile, err := apiServerClient.GetEffectiveTLSProfile(ctx)

// 3. Convert to crypto/tls.Config
tlsConfig, err := crypto.ConvertTLSProfile(profile)
```

### Conversion Example

```go
// OpenShift profile
profile := &configv1.TLSSecurityProfile{
    Type: configv1.TLSProfileModernType,
}

// Convert to Go's crypto/tls.Config
tlsConfig, err := crypto.ConvertTLSProfile(profile)

// Result:
// tlsConfig.MinVersion = tls.VersionTLS13
// tlsConfig.CipherSuites = [TLS 1.3 ciphers]
// tlsConfig.PreferServerCipherSuites = true
// tlsConfig.Renegotiation = tls.RenegotiateNever
```

---

## TLS Profile Types

| Type | TLS Version | Description | Use Case |
|------|-------------|-------------|----------|
| **Modern** | TLS 1.3+ | Highest security | New applications |
| **Intermediate** | TLS 1.2+ | Default, balanced | General use (default) |
| **Old** | TLS 1.0+ | Legacy support | Legacy clients only |
| **Custom** | User-defined | Specific requirements | Special configurations |

**Default**: When `spec.tlsSecurityProfile` is unspecified, OpenShift uses **Intermediate** (TLS 1.2+).

---

## Diagnostic Commands

### Check Cluster TLS Profile
```bash
# Using oc CLI
oc get apiserver cluster -o jsonpath='{.spec.tlsSecurityProfile}' | jq .

# Using tls-configurator
./bin/tls-configurator --action get-cluster
```

### Check IngressController TLS Profile
```bash
# Using oc CLI
oc get ingresscontroller default -n openshift-ingress-operator \
  -o jsonpath='{.spec.tlsSecurityProfile}'

# Using tls-configurator (original method)
./bin/tls-configurator --action get --ingress-controller default
```

### Show crypto/tls.Config
```bash
./bin/tls-configurator --action show-tlsconfig
```

---

## Usage Examples

### 1. Get Cluster-Wide TLS Profile (Recommended)

```bash
./bin/tls-configurator --action get-cluster
```

**Output**:
```json
Cluster-wide TLS Security Profile (from APIServer):
===================================================
{
  "type": "Intermediate"
}

ℹ️  This is the authoritative cluster-wide TLS configuration.
   Per OpenShift best practices, read from APIServer CR 'cluster'.
```

### 2. Show Converted TLS Configuration

```bash
./bin/tls-configurator --action show-tlsconfig
```

**Output**:
```
OpenShift TLS Security Profile:
================================
{
  "type": "Intermediate"
}

Converted to crypto/tls.Config:
================================
MinVersion: TLS 1.2
MaxVersion: Not specified
PreferServerCipherSuites: true
SessionTicketsDisabled: false
Renegotiation: Never

Cipher Suites (9 configured):
   1. TLS_AES_128_GCM_SHA256 (0x1301)
   2. TLS_AES_256_GCM_SHA384 (0x1302)
   3. TLS_CHACHA20_POLY1305_SHA256 (0x1303)
   4. TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 (0xc02b)
   5. TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 (0xc02f)
   ... (continues)

ℹ️  This configuration can be directly used with:
   - Kubernetes webhook servers
   - Metrics endpoints
   - HTTP/gRPC servers
   - Any Go application using crypto/tls
```

### 3. Update IngressController (Original Method)

```bash
./bin/tls-configurator --action update \
  --type Custom \
  --min-tls-version VersionTLS13 \
  --ciphers "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384"
```

---

## Code Structure

```
pkg/
├── client/
│   ├── client.go        # IngressController client (original)
│   └── apiserver.go     # APIServer client (NEW)
├── crypto/              # NEW package
│   ├── crypto.go        # TLS conversion utilities
│   └── crypto_test.go   # Comprehensive tests
├── config/
│   └── config.go        # Enhanced with GetKubeConfig()
└── controller/
    └── controller.go    # TLS controller logic
```

---

## Benefits of Updates

### ✅ Compliance with OpenShift Best Practices
- Uses `APIServer` CR as authoritative source
- Follows recommended conversion patterns
- Implements library-go-style utilities

### ✅ Enhanced Functionality
- Actual TLS configuration, not just API updates
- Support for webhook servers and metrics endpoints
- Proper cipher suite handling and version mapping

### ✅ Dual Approach Support
- **Recommended**: APIServer cluster-wide profile
- **Legacy**: IngressController-specific profile
- Both approaches available for flexibility

### ✅ Production Ready
- Comprehensive test coverage
- Proper error handling
- Clear documentation and examples

### ✅ Future Proof
- Aligned with OpenShift engineering practices
- Easy to extend with profile watchers
- Ready for graceful restart patterns

---

## Testing

All new functionality is fully tested:

### Crypto Package Tests
```bash
go test -v ./pkg/crypto/...
```

**Coverage**:
- TLS version conversion
- Profile conversion (Modern, Intermediate, Old, Custom)
- Cipher suite conversion
- Secure TLS configuration
- OpenSSL to IANA mapping

### Integration
```bash
make test
```

**Results**:
- ✅ All unit tests pass
- ✅ Integration tests pass
- ✅ Build successful

---

## Comparison: Before vs After

### Before (Original Implementation)
```go
// Only supported IngressController resources
client := NewClient(config, namespace)
profile, err := client.GetTLSSecurityProfile(ctx, "default")
// Could only read/update OpenShift resources
```

### After (Enhanced Implementation)
```go
// Recommended: APIServer approach
apiClient := client.NewAPIServerClient(config)
profile, err := apiClient.GetEffectiveTLSProfile(ctx)

// Convert to actual TLS configuration
tlsConfig, err := crypto.ConvertTLSProfile(profile)

// Use in your application
server := &http.Server{
    TLSConfig: tlsConfig,
}
```

---

## Migration Guide

### For New Applications
**Use the APIServer approach** (recommended):
```bash
./bin/tls-configurator --action get-cluster
./bin/tls-configurator --action show-tlsconfig
```

### For Existing Applications
Both approaches are supported:
1. **Cluster-wide**: Use `--action get-cluster` for APIServer
2. **IngressController**: Use `--action get` for IngressController (legacy)

### No Breaking Changes
All existing functionality remains available and working.

---

## References

- **Source PR**: https://github.com/openshift-eng/ai-helpers/pull/316
- **OpenShift API**: https://github.com/openshift/api
- **Mozilla TLS Guidelines**: https://wiki.mozilla.org/Security/Server_Side_TLS
- **Go crypto/tls**: https://pkg.go.dev/crypto/tls

---

## Next Steps

Potential future enhancements (not implemented yet):

1. **Profile Watcher**: Monitor TLS profile changes
2. **Graceful Restart**: Reload on configuration updates
3. **controller-runtime-common**: Integration with official package
4. **configobserver Pattern**: Watch for profile changes

These can be added incrementally as needed.

---

## Conclusion

The project now fully implements OpenShift TLS profile best practices as outlined in PR #316:

✅ APIServer CR support (cluster-wide profiles)
✅ crypto/tls.Config conversion
✅ OpenSSL to IANA cipher mapping
✅ TLS version utilities
✅ Comprehensive testing
✅ Enhanced documentation
✅ Production-ready implementation

The application can now be used as both a CLI tool and a reference implementation for OpenShift TLS profile handling in Go applications.
