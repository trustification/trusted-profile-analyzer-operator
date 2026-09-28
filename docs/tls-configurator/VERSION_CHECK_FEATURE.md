# OpenShift Version Check Feature

**Status**: ✅ Implemented
**Minimum Required Version**: OpenShift 4.22
**Date**: 2026-03-10

---

## Overview

This feature ensures that TLS configuration changes are only applied to OpenShift clusters running version 4.22 or later. This protects against applying incompatible configurations to older cluster versions.

## Why Version 4.22?

OpenShift 4.22 introduced enhanced TLS profile support and improved APIServer configuration capabilities. Earlier versions may not support all TLS configuration options properly, particularly:

- Advanced cipher suite configurations
- TLS 1.3 specific settings
- Certain custom TLS profile features

## How It Works

### Automatic Version Check

When you attempt to update TLS configurations using the `update` action, the application automatically:

1. Queries the OpenShift ClusterVersion resource
2. Parses the current cluster version
3. Compares it against the minimum requirement (4.22)
4. Blocks the update if the version is too old

### Manual Version Check

You can manually check your cluster version:

```bash
./bin/tls-configurator --action check-version
```

**Example Output (Compatible):**
```
OpenShift Cluster Version Check
================================
Detected Version: 4.22.5
Major: 4
Minor: 22
Patch: 5

Minimum Required: 4.22

✅ Status: COMPATIBLE
   This cluster meets the minimum version requirement.
   TLS configuration changes can be safely applied.
```

**Example Output (Incompatible):**
```
OpenShift Cluster Version Check
================================
Detected Version: 4.21.0
Major: 4
Minor: 21
Patch: 0

Minimum Required: 4.22

❌ Status: INCOMPATIBLE
   This cluster does NOT meet the minimum version requirement.
   TLS configuration changes will be blocked.

   Please upgrade to OpenShift 4.22 or later before applying
   TLS configuration changes.
```

---

## Implementation Details

### New Components

#### 1. **Version Checker** (`pkg/client/version.go`)

Main functionality:
- `NewVersionChecker()` - Creates version checker client
- `GetOpenShiftVersion()` - Retrieves current cluster version
- `IsMinimumVersion()` - Checks if version meets requirements
- `ValidateMinimumVersion()` - Validates and returns error if too old

#### 2. **Version Parsing**

Handles various OpenShift version formats:
- Simple: `4.22.0`
- With suffix: `4.22.0-rc.1`
- Nightly: `4.23.0-0.nightly-2024-01-01-123456`

#### 3. **Integration with Controller**

The `TLSController` now includes:
- Version checker instance
- Automatic validation before updates
- Logging of version check results

### Version String Parsing

The system can parse various version formats:

```
Input                              → Parsed as
────────────────────────────────────────────────
4.22.0                            → 4.22.0
4.22.5                            → 4.22.5
4.22.0-rc.1                       → 4.22.0
4.23.0-0.nightly-2024-01-01       → 4.23.0
5.0.0                             → 5.0.0
```

---

## Usage Examples

### Check Version Before Update

```bash
# First, check if your cluster is compatible
./bin/tls-configurator --action check-version

# If compatible, proceed with update
./bin/tls-configurator --action update \
  --type Custom \
  --min-tls-version VersionTLS13 \
  --ciphers "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384"
```

### Automatic Protection

When updating, version check happens automatically:

```bash
$ ./bin/tls-configurator --action update --type Custom ...

2026/03/10 12:00:00 Updating TLS profile for IngressController: openshift-ingress-operator/default
2026/03/10 12:00:00 OpenShift version 4.22.5 meets minimum requirement (4.22+)
2026/03/10 12:00:00 Successfully updated TLS profile
```

If version is too old:

```bash
$ ./bin/tls-configurator --action update --type Custom ...

2026/03/10 12:00:00 Updating TLS profile for IngressController: openshift-ingress-operator/default
2026/03/10 12:00:00 Failed to update TLS profile: version check failed:
  OpenShift version 4.21.0 does not meet minimum requirement (4.22+).
  This tool requires OpenShift 4.22 or later for proper TLS profile support
```

### Skipping Version Check (Not Recommended)

In rare cases where you need to bypass the version check:

```bash
./bin/tls-configurator --action update \
  --skip-version-check \
  --type Custom \
  --min-tls-version VersionTLS13 \
  --ciphers "TLS_AES_128_GCM_SHA256"
```

**⚠️ Warning**: Skipping the version check may result in:
- Incompatible configurations
- Unexpected behavior
- Cluster instability
- Only use if you absolutely know what you're doing

---

## API Access Requirements

### Required RBAC Permissions

The version checker needs access to the ClusterVersion resource:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: tls-configurator
rules:
# Existing permissions
- apiGroups: ["operator.openshift.io"]
  resources: ["ingresscontrollers"]
  verbs: ["get", "list", "update", "patch"]
- apiGroups: ["config.openshift.io"]
  resources: ["apiservers"]
  verbs: ["get", "list"]

# NEW: Version checking
- apiGroups: ["config.openshift.io"]
  resources: ["clusterversions"]
  verbs: ["get", "list"]
```

### ClusterVersion Resource

The checker reads from the `version` ClusterVersion resource:

```bash
# Check cluster version manually with oc
oc get clusterversion version -o yaml
```

---

## Testing

### Unit Tests

Comprehensive tests for version parsing and comparison:

```bash
go test -v ./pkg/client/... -run TestVersion
```

**Test Coverage**:
- Version string parsing (9 test cases)
- Version comparison (6 test cases)
- Version string formatting (3 test cases)

**All tests passing**: ✅

### Test Cases

1. **Version Parsing**
   - Simple versions: `4.22.0`
   - Versions with patch: `4.22.5`
   - Versions with suffix: `4.22.0-rc.1`
   - Nightly versions: `4.23.0-0.nightly-...`
   - Invalid formats

2. **Version Comparison**
   - Exactly minimum: `4.22.0` ≥ `4.22` → ✅
   - Newer minor: `4.23.0` ≥ `4.22` → ✅
   - Newer major: `5.0.0` ≥ `4.22` → ✅
   - Older minor: `4.21.0` ≥ `4.22` → ❌
   - Older major: `3.11.0` ≥ `4.22` → ❌

---

## Error Handling

### Version Detection Failures

If the version cannot be detected:

```
Error: failed to check cluster version: failed to get cluster version:
  clusterversions.config.openshift.io "version" not found
```

**Possible causes**:
- Not running on OpenShift
- Insufficient RBAC permissions
- Network connectivity issues

### Invalid Version Format

If version parsing fails:

```
Error: failed to parse version X.Y.Z: invalid version format
```

**Resolution**: Report as a bug - all standard OpenShift version formats should parse correctly.

---

## Configuration Constants

Located in `pkg/client/version.go`:

```go
const (
    // MinimumOpenShiftMajorVersion is the minimum required OpenShift major version
    MinimumOpenShiftMajorVersion = 4

    // MinimumOpenShiftMinorVersion is the minimum required OpenShift minor version
    MinimumOpenShiftMinorVersion = 22
)
```

To change the minimum version requirement, update these constants and rebuild.

---

## Compatibility Matrix

| OpenShift Version | Compatible | Notes |
|-------------------|------------|-------|
| 3.11 | ❌ | Too old, not supported |
| 4.10 | ❌ | Below minimum |
| 4.21 | ❌ | Below minimum |
| **4.22** | ✅ | **Minimum required** |
| 4.23 | ✅ | Fully supported |
| 4.24+ | ✅ | Fully supported |
| 5.0+ | ✅ | Forward compatible |

---

## FAQ

### Q: Why can't I update TLS on OpenShift 4.21?

**A**: OpenShift 4.21 lacks certain TLS configuration features that were introduced in 4.22. Applying advanced TLS configurations to 4.21 could result in unexpected behavior or instability.

### Q: Can I change the minimum version requirement?

**A**: Yes, but not recommended. Edit `MinimumOpenShiftMajorVersion` and `MinimumOpenShiftMinorVersion` in `pkg/client/version.go` and rebuild. Only do this if you fully understand the implications.

### Q: What if I need to test on an older cluster?

**A**: Use the `--skip-version-check` flag. Be prepared for potential issues and do not use in production.

### Q: Does this work with OKD?

**A**: Yes, OKD uses the same ClusterVersion resource format. The version numbering might be different, but the parsing should work.

### Q: What happens if the version check fails due to network issues?

**A**: The update will be blocked. The version check must succeed before changes are applied. Ensure network connectivity to the API server.

---

## Future Enhancements

Potential improvements for future versions:

1. **Version-Specific Feature Detection**
   - Detect which TLS features are available in each version
   - Provide warnings for features that might not work

2. **Soft Warning Mode**
   - Allow updates with warnings on older versions
   - Log warnings but don't block

3. **Version Compatibility Database**
   - Maintain database of known issues per version
   - Provide specific guidance based on detected version

4. **Automatic Version Upgrade Suggestions**
   - Detect outdated clusters
   - Provide upgrade recommendations

---

## Related Documentation

- **VERSION_CHECK_FEATURE.md** - This file
- **UPDATES_FROM_PR316.md** - OpenShift best practices
- **PROJECT_SUMMARY.md** - Overall project documentation
- **README.md** - User guide and usage examples

---

## Conclusion

The version check feature provides an essential safety mechanism to prevent incompatible TLS configurations from being applied to older OpenShift clusters. It's automatic, transparent, and can be bypassed if absolutely necessary (though not recommended).

**Key Benefits**:
- ✅ Prevents configuration errors
- ✅ Protects cluster stability
- ✅ Clear error messages
- ✅ Easy to verify compatibility
- ✅ Minimal performance impact

**Recommendation**: Always check version compatibility before deploying to a new cluster.
