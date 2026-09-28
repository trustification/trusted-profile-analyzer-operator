# README Addition - Version Check Feature

Add this section to the README.md after the "Quick Start" section:

---

## Version Requirements

### Minimum OpenShift Version: 4.22

This tool requires **OpenShift 4.22 or later** for proper TLS profile support. The application automatically checks the cluster version before applying changes.

### Check Your Cluster Version

Before making any TLS configuration changes, verify your cluster version:

```bash
# Check cluster version
./bin/tls-configurator --action check-version
```

**Example Output (Compatible Cluster):**
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

**Example Output (Incompatible Cluster):**
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

### Automatic Protection

When you attempt to update TLS configurations, the version is checked automatically:

```bash
$ ./bin/tls-configurator --action update \
    --type Custom \
    --min-tls-version VersionTLS13 \
    --ciphers "TLS_AES_128_GCM_SHA256"

2026/03/10 12:00:00 Updating TLS profile for IngressController...
2026/03/10 12:00:00 OpenShift version 4.22.5 meets minimum requirement (4.22+)
2026/03/10 12:00:00 Successfully updated TLS profile
```

If the cluster version is too old, the update will be blocked:

```bash
$ ./bin/tls-configurator --action update ...

Error: version check failed: OpenShift version 4.21.0 does not meet
minimum requirement (4.22+). This tool requires OpenShift 4.22 or
later for proper TLS profile support
```

### Version Compatibility Matrix

| OpenShift Version | Compatible | Notes |
|-------------------|------------|-------|
| 3.11 and earlier | ❌ | Not supported |
| 4.10 - 4.21 | ❌ | Below minimum requirement |
| **4.22** | ✅ | **Minimum required version** |
| 4.23+ | ✅ | Fully supported |
| 5.0+ | ✅ | Forward compatible |

### Bypassing Version Check (Not Recommended)

In exceptional cases where you need to bypass the version check:

```bash
./bin/tls-configurator --action update \
  --skip-version-check \
  --type Custom \
  --min-tls-version VersionTLS13 \
  --ciphers "TLS_AES_128_GCM_SHA256"
```

**⚠️ Warning**: Using `--skip-version-check` may result in:
- Incompatible configurations
- Unexpected cluster behavior
- Potential instability
- Configuration errors

Only use this flag if you absolutely know what you're doing and understand the risks.

### Required RBAC Permissions

For version checking to work, the application needs additional RBAC permissions:

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

### Why OpenShift 4.22?

OpenShift 4.22 introduced enhanced TLS profile support including:
- Improved APIServer TLS configuration
- Better cipher suite support
- Enhanced TLS 1.3 capabilities
- More stable custom TLS profile handling

Applying advanced TLS configurations to earlier versions could result in:
- Unexpected behavior
- Configuration errors
- Cluster instability
- Incomplete feature support

For detailed information about the version check feature, see [VERSION_CHECK_FEATURE.md](VERSION_CHECK_FEATURE.md).

---
