# Updated README Section - Add After Installation

## New Features (Based on OpenShift Best Practices)

### Cluster-Wide TLS Profile Support

Following OpenShift best practices, the application now supports reading cluster-wide TLS profiles from the `APIServer` CR:

```bash
# Get cluster-wide TLS profile (recommended approach)
./bin/tls-configurator --action get-cluster

# Show actual crypto/tls.Config conversion
./bin/tls-configurator --action show-tlsconfig
```

### crypto/tls Integration

Convert OpenShift TLS profiles to Go's `crypto/tls.Config` for use in:
- Kubernetes webhook servers
- Metrics endpoints  
- HTTP/gRPC servers
- Any Go application

**Example Output**:
```
MinVersion: TLS 1.3
CipherSuites: TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384, ...
PreferServerCipherSuites: true
Renegotiation: Never
```

---

## Usage

### Option 1: Cluster-Wide Profile (Recommended)

Read from the authoritative APIServer CR:

```bash
# View cluster TLS profile
./bin/tls-configurator --action get-cluster

# Convert to crypto/tls.Config format
./bin/tls-configurator --action show-tlsconfig
```

### Option 2: IngressController Profile (Legacy)

Read from specific IngressController:

```bash
# Get IngressController TLS profile
./bin/tls-configurator --action get --ingress-controller default

# Update IngressController TLS profile  
./bin/tls-configurator --action update \
  --type Custom \
  --min-tls-version VersionTLS13 \
  --ciphers "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384"

# List all IngressControllers
./bin/tls-configurator --action list
```

---

## TLS Profile Types

| Type | TLS Version | Description |
|------|-------------|-------------|
| Modern | TLS 1.3+ | Highest security, recommended for new applications |
| Intermediate | TLS 1.2+ | **Default**, balanced security and compatibility |
| Old | TLS 1.0+ | Legacy support only, not recommended |
| Custom | User-defined | Specific cipher and version requirements |

**Note**: When no profile is specified, OpenShift uses **Intermediate** (TLS 1.2+).

---

## Architecture

```
┌─────────────────────┐
│ OpenShift Cluster   │
├─────────────────────┤
│ APIServer CR        │ ← Cluster-wide TLS (Recommended)
│  └─ cluster         │
│                     │
│ IngressController   │ ← Per-controller TLS (Legacy)
│  └─ default         │
└─────────────────────┘
         ↓
┌─────────────────────┐
│ tls-configurator    │
├─────────────────────┤
│ pkg/client/         │
│  ├─ apiserver.go    │ ← NEW: APIServer client
│  └─ client.go       │ ← IngressController client
│                     │
│ pkg/crypto/         │ ← NEW: TLS conversion
│  └─ crypto.go       │
└─────────────────────┘
         ↓
┌─────────────────────┐
│ crypto/tls.Config   │ ← Go native TLS config
└─────────────────────┘
```

---

## Examples

### Example 1: Check Cluster TLS Configuration

```bash
$ ./bin/tls-configurator --action get-cluster

Cluster-wide TLS Security Profile (from APIServer):
===================================================
{
  "type": "Intermediate"
}

ℹ️  This is the authoritative cluster-wide TLS configuration.
   Per OpenShift best practices, read from APIServer CR 'cluster'.
```

### Example 2: View TLS Configuration Details

```bash
$ ./bin/tls-configurator --action show-tlsconfig

OpenShift TLS Security Profile:
================================
{
  "type": "Modern"
}

Converted to crypto/tls.Config:
================================
MinVersion: TLS 1.3
PreferServerCipherSuites: true
Renegotiation: Never

Cipher Suites (3 configured):
   1. TLS_AES_128_GCM_SHA256 (0x1301)
   2. TLS_AES_256_GCM_SHA384 (0x1302)
   3. TLS_CHACHA20_POLY1305_SHA256 (0x1303)
```

### Example 3: Programmatic Usage

```go
import (
    "context"
    "github.com/openshift/tls-configurator/pkg/client"
    "github.com/openshift/tls-configurator/pkg/crypto"
)

// Get cluster TLS profile
apiClient, _ := client.NewAPIServerClient(k8sConfig)
profile, _ := apiClient.GetEffectiveTLSProfile(context.Background())

// Convert to crypto/tls.Config
tlsConfig, _ := crypto.ConvertTLSProfile(profile)

// Use in your application
server := &http.Server{
    TLSConfig: tlsConfig,
    // ... other settings
}
```

---

## Documentation

- **[UPDATES_FROM_PR316.md](UPDATES_FROM_PR316.md)** - Detailed changelog and improvements
- **[IMPROVEMENTS_ANALYSIS.md](IMPROVEMENTS_ANALYSIS.md)** - Gap analysis and recommendations
- **[VERIFICATION_REPORT.md](VERIFICATION_REPORT.md)** - Build and test verification
- **[PROJECT_SUMMARY.md](PROJECT_SUMMARY.md)** - Technical overview

