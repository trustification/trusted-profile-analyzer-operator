# Using TLS Configurator as a Go Library

**Version**: 1.0.0
**Go Module**: `github.com/openshift/tls-configurator`
**Minimum Go Version**: 1.25

---

## Table of Contents

1. [Installation](#installation)
2. [Quick Start](#quick-start)
3. [Package Overview](#package-overview)
4. [Common Use Cases](#common-use-cases)
5. [API Reference](#api-reference)
6. [Complete Examples](#complete-examples)
7. [Error Handling](#error-handling)
8. [Best Practices](#best-practices)

---

## Installation

### Add as Dependency

```bash
# Add to your Go module
go get github.com/openshift/tls-configurator

# Or add to go.mod manually
# require github.com/openshift/tls-configurator v1.0.0
```

### Import in Your Code

```go
import (
    "github.com/openshift/tls-configurator/pkg/client"
    "github.com/openshift/tls-configurator/pkg/config"
    "github.com/openshift/tls-configurator/pkg/controller"
    "github.com/openshift/tls-configurator/pkg/crypto"

    configv1 "github.com/openshift/api/config/v1"
)
```

---

## Quick Start

### Example 1: Check OpenShift Version

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/openshift/tls-configurator/pkg/client"
    "k8s.io/client-go/tools/clientcmd"
)

func main() {
    // Load kubeconfig
    config, err := clientcmd.BuildConfigFromFlags("", "/path/to/kubeconfig")
    if err != nil {
        log.Fatal(err)
    }

    // Create version checker
    checker, err := client.NewVersionChecker(config)
    if err != nil {
        log.Fatal(err)
    }

    // Get cluster version
    version, err := checker.GetOpenShiftVersion(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("OpenShift Version: %s\n", version.String())
    fmt.Printf("Compatible: %v\n", version.IsAtLeast(4, 22))
}
```

### Example 2: Get Cluster TLS Profile

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"

    "github.com/openshift/tls-configurator/pkg/client"
    "k8s.io/client-go/tools/clientcmd"
)

func main() {
    // Load kubeconfig
    config, err := clientcmd.BuildConfigFromFlags("", "/path/to/kubeconfig")
    if err != nil {
        log.Fatal(err)
    }

    // Create APIServer client
    apiClient, err := client.NewAPIServerClient(config)
    if err != nil {
        log.Fatal(err)
    }

    // Get cluster-wide TLS profile
    profile, err := apiClient.GetEffectiveTLSProfile(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    // Display profile
    data, _ := json.MarshalIndent(profile, "", "  ")
    fmt.Println(string(data))
}
```

### Example 3: Convert to crypto/tls.Config

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/openshift/tls-configurator/pkg/client"
    "github.com/openshift/tls-configurator/pkg/crypto"
    "k8s.io/client-go/tools/clientcmd"
)

func main() {
    // Load kubeconfig
    config, err := clientcmd.BuildConfigFromFlags("", "/path/to/kubeconfig")
    if err != nil {
        log.Fatal(err)
    }

    // Get cluster TLS profile
    apiClient, err := client.NewAPIServerClient(config)
    if err != nil {
        log.Fatal(err)
    }

    profile, err := apiClient.GetEffectiveTLSProfile(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    // Convert to crypto/tls.Config
    tlsConfig, err := crypto.ConvertTLSProfile(profile)
    if err != nil {
        log.Fatal(err)
    }

    // Use in your application
    fmt.Printf("TLS Version: %x\n", tlsConfig.MinVersion)
    fmt.Printf("Cipher Suites: %d configured\n", len(tlsConfig.CipherSuites))
}
```

---

## Package Overview

### pkg/client

**Purpose**: OpenShift API clients

#### IngressController Client

```go
// Create client for IngressController operations
restConfig, _ := clientcmd.BuildConfigFromFlags("", kubeconfig)
client, err := client.NewClient(restConfig, "openshift-ingress-operator")

// Get TLS profile
profile, err := client.GetTLSSecurityProfile(ctx, "default")

// Update TLS profile
err = client.UpdateTLSSecurityProfile(ctx, "default", newProfile)
```

#### APIServer Client (Recommended)

```go
// Create APIServer client
apiClient, err := client.NewAPIServerClient(restConfig)

// Get cluster-wide TLS profile
profile, err := apiClient.GetClusterTLSProfile(ctx)

// Get effective profile (with defaults)
profile, err := apiClient.GetEffectiveTLSProfile(ctx)

// Update cluster profile
err = apiClient.UpdateClusterTLSProfile(ctx, newProfile)
```

#### Version Checker

```go
// Create version checker
checker, err := client.NewVersionChecker(restConfig)

// Get OpenShift version
version, err := checker.GetOpenShiftVersion(ctx)

// Check minimum version
meetsReq, version, err := checker.IsMinimumVersion(ctx)

// Validate (returns error if too old)
err = checker.ValidateMinimumVersion(ctx)
```

### pkg/crypto

**Purpose**: TLS profile conversion utilities

```go
import "github.com/openshift/tls-configurator/pkg/crypto"

// Convert OpenShift profile to crypto/tls.Config
tlsConfig, err := crypto.ConvertTLSProfile(osProfile)

// Convert TLS version string
version, err := crypto.TLSVersion("VersionTLS13")

// Convert cipher names
ianaNames := crypto.OpenSSLToIANACipherSuites(opensslNames)
cipherIDs, err := crypto.CipherSuites(ianaNames)

// Apply secure baseline
crypto.SecureTLSConfig(tlsConfig)

// Get predefined cipher suites
modernCiphers := crypto.GetModernCipherSuites()
intermediateCiphers := crypto.GetIntermediateCipherSuites()
```

### pkg/config

**Purpose**: Configuration management

```go
import "github.com/openshift/tls-configurator/pkg/config"

// Create configuration
cfg := config.NewConfig()
cfg.Kubeconfig = "/path/to/kubeconfig"
cfg.IngressControllerName = "default"
cfg.Namespace = "openshift-ingress-operator"

// Get Kubernetes config
restConfig, err := cfg.GetKubeConfig()

// Build TLS profile
tlsConfig := &config.TLSConfig{
    Type:          configv1.TLSProfileCustomType,
    Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
    MinTLSVersion: configv1.VersionTLS13,
}
profile := config.BuildTLSProfile(tlsConfig)
```

### pkg/controller

**Purpose**: High-level TLS controller

```go
import "github.com/openshift/tls-configurator/pkg/controller"

// Create controller
cfg := config.NewConfig()
ctrl, err := controller.NewTLSController(cfg)

// Get current profile
profile, err := ctrl.GetCurrentTLSProfile(ctx)

// Update profile
err = ctrl.UpdateTLSProfile(ctx, newProfile)

// Apply TLS configuration
tlsConfig := &config.TLSConfig{...}
err = ctrl.ApplyTLSConfiguration(ctx, tlsConfig)

// Check version
version, err := ctrl.CheckOpenShiftVersion(ctx)
```

---

## Common Use Cases

### Use Case 1: Kubernetes Operator with TLS Configuration

```go
package main

import (
    "context"
    "net/http"

    "github.com/openshift/tls-configurator/pkg/client"
    "github.com/openshift/tls-configurator/pkg/crypto"
    "sigs.k8s.io/controller-runtime/pkg/manager"
)

type MyOperator struct {
    apiClient *client.APIServerClient
}

func (o *MyOperator) SetupWebhookServer(mgr manager.Manager) error {
    // Get cluster TLS profile
    profile, err := o.apiClient.GetEffectiveTLSProfile(context.Background())
    if err != nil {
        return err
    }

    // Convert to crypto/tls.Config
    tlsConfig, err := crypto.ConvertTLSProfile(profile)
    if err != nil {
        return err
    }

    // Apply to webhook server
    server := mgr.GetWebhookServer()
    server.TLSOpts = []func(*tls.Config){
        func(cfg *tls.Config) {
            cfg.MinVersion = tlsConfig.MinVersion
            cfg.CipherSuites = tlsConfig.CipherSuites
            cfg.PreferServerCipherSuites = tlsConfig.PreferServerCipherSuites
        },
    }

    return nil
}
```

### Use Case 2: Metrics Server with Cluster TLS Settings

```go
package main

import (
    "context"
    "crypto/tls"
    "net/http"

    "github.com/openshift/tls-configurator/pkg/client"
    "github.com/openshift/tls-configurator/pkg/crypto"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func StartMetricsServer(kubeconfig string) error {
    // Get Kubernetes config
    restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
    if err != nil {
        return err
    }

    // Get cluster TLS profile
    apiClient, err := client.NewAPIServerClient(restConfig)
    if err != nil {
        return err
    }

    profile, err := apiClient.GetEffectiveTLSProfile(context.Background())
    if err != nil {
        return err
    }

    // Convert to TLS config
    tlsConfig, err := crypto.ConvertTLSProfile(profile)
    if err != nil {
        return err
    }

    // Create HTTPS server with cluster TLS settings
    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.Handler())

    server := &http.Server{
        Addr:      ":8443",
        Handler:   mux,
        TLSConfig: tlsConfig,
    }

    return server.ListenAndServeTLS("server.crt", "server.key")
}
```

### Use Case 3: TLS Configuration Validator

```go
package main

import (
    "context"
    "fmt"

    "github.com/openshift/tls-configurator/pkg/client"
    configv1 "github.com/openshift/api/config/v1"
)

type TLSValidator struct {
    client *client.Client
}

func (v *TLSValidator) ValidateIngressController(ctx context.Context, name string) error {
    // Get current profile
    profile, err := v.client.GetTLSSecurityProfile(ctx, name)
    if err != nil {
        return fmt.Errorf("failed to get profile: %w", err)
    }

    // Validate profile
    if err := client.ValidateTLSProfile(profile); err != nil {
        return fmt.Errorf("invalid TLS profile: %w", err)
    }

    // Check for insecure configurations
    if profile.Type == configv1.TLSProfileOldType {
        return fmt.Errorf("Old TLS profile is not recommended")
    }

    if profile.Type == configv1.TLSProfileCustomType {
        if profile.Custom.MinTLSVersion == configv1.VersionTLS10 ||
           profile.Custom.MinTLSVersion == configv1.VersionTLS11 {
            return fmt.Errorf("TLS 1.0/1.1 are deprecated and insecure")
        }
    }

    return nil
}
```

### Use Case 4: Automated TLS Profile Updater

```go
package main

import (
    "context"
    "time"

    "github.com/openshift/tls-configurator/pkg/config"
    "github.com/openshift/tls-configurator/pkg/controller"
    configv1 "github.com/openshift/api/config/v1"
)

type TLSUpdater struct {
    controller *controller.TLSController
}

func (u *TLSUpdater) EnforceModernTLS(ctx context.Context) error {
    // Check version first
    if err := u.controller.ValidateOpenShiftVersion(ctx); err != nil {
        return err
    }

    // Create Modern TLS configuration
    tlsConfig := &config.TLSConfig{
        Type:          configv1.TLSProfileModernType,
        MinTLSVersion: configv1.VersionTLS13,
        Ciphers: []string{
            "TLS_AES_128_GCM_SHA256",
            "TLS_AES_256_GCM_SHA384",
            "TLS_CHACHA20_POLY1305_SHA256",
        },
    }

    // Apply configuration
    return u.controller.ApplyTLSConfiguration(ctx, tlsConfig)
}

func (u *TLSUpdater) RunPeriodically() {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()

    for range ticker.C {
        ctx := context.Background()
        if err := u.EnforceModernTLS(ctx); err != nil {
            // Log error
            continue
        }
    }
}
```

---

## API Reference

### client.NewClient

```go
func NewClient(config *rest.Config, namespace string) (*Client, error)
```

Creates a new IngressController client.

**Parameters:**
- `config`: Kubernetes REST config
- `namespace`: Namespace containing IngressControllers (typically `openshift-ingress-operator`)

**Returns:** Client instance or error

### client.NewAPIServerClient

```go
func NewAPIServerClient(config *rest.Config) (*APIServerClient, error)
```

Creates a new APIServer client for cluster-wide TLS profiles.

**Parameters:**
- `config`: Kubernetes REST config

**Returns:** APIServerClient instance or error

### client.NewVersionChecker

```go
func NewVersionChecker(config *rest.Config) (*VersionChecker, error)
```

Creates a new version checker for OpenShift version detection.

**Parameters:**
- `config`: Kubernetes REST config

**Returns:** VersionChecker instance or error

### crypto.ConvertTLSProfile

```go
func ConvertTLSProfile(profile *configv1.TLSSecurityProfile) (*tls.Config, error)
```

Converts an OpenShift TLS security profile to crypto/tls.Config.

**Parameters:**
- `profile`: OpenShift TLS security profile (nil returns default Intermediate)

**Returns:** crypto/tls.Config or error

### crypto.TLSVersion

```go
func TLSVersion(version string) (uint16, error)
```

Converts a TLS version string to Go constant.

**Parameters:**
- `version`: TLS version string (e.g., "VersionTLS13")

**Returns:** TLS version constant or error

**Supported Values:**
- `"VersionTLS10"` → `tls.VersionTLS10`
- `"VersionTLS11"` → `tls.VersionTLS11`
- `"VersionTLS12"` → `tls.VersionTLS12`
- `"VersionTLS13"` → `tls.VersionTLS13`

### controller.NewTLSController

```go
func NewTLSController(cfg *config.Config) (*TLSController, error)
```

Creates a new TLS controller with version checking.

**Parameters:**
- `cfg`: Configuration with kubeconfig, namespace, etc.

**Returns:** TLSController instance or error

---

## Complete Examples

### Example: Full Operator Integration

```go
package main

import (
    "context"
    "crypto/tls"
    "fmt"
    "net/http"
    "os"

    "github.com/openshift/tls-configurator/pkg/client"
    "github.com/openshift/tls-configurator/pkg/crypto"
    "k8s.io/client-go/rest"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
    // Setup logging
    ctrl.SetLogger(zap.New())
    log := ctrl.Log.WithName("tls-operator")

    // Get Kubernetes config
    restConfig, err := ctrl.GetConfig()
    if err != nil {
        log.Error(err, "failed to get kube config")
        os.Exit(1)
    }

    // Check OpenShift version
    if err := checkVersion(restConfig); err != nil {
        log.Error(err, "version check failed")
        os.Exit(1)
    }

    // Get cluster TLS configuration
    tlsConfig, err := getClusterTLSConfig(restConfig)
    if err != nil {
        log.Error(err, "failed to get TLS config")
        os.Exit(1)
    }

    // Start metrics server with cluster TLS settings
    if err := startMetricsServer(tlsConfig); err != nil {
        log.Error(err, "failed to start metrics server")
        os.Exit(1)
    }

    log.Info("Operator started successfully")
    select {} // Block forever
}

func checkVersion(config *rest.Config) error {
    checker, err := client.NewVersionChecker(config)
    if err != nil {
        return err
    }

    version, err := checker.GetOpenShiftVersion(context.Background())
    if err != nil {
        return err
    }

    fmt.Printf("OpenShift version: %s\n", version.String())

    if !version.IsAtLeast(4, 22) {
        return fmt.Errorf("requires OpenShift 4.22+, found %s", version.String())
    }

    return nil
}

func getClusterTLSConfig(config *rest.Config) (*tls.Config, error) {
    apiClient, err := client.NewAPIServerClient(config)
    if err != nil {
        return nil, err
    }

    profile, err := apiClient.GetEffectiveTLSProfile(context.Background())
    if err != nil {
        return nil, err
    }

    tlsConfig, err := crypto.ConvertTLSProfile(profile)
    if err != nil {
        return nil, err
    }

    return tlsConfig, nil
}

func startMetricsServer(tlsConfig *tls.Config) error {
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("ok"))
    })
    mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
        // Add your metrics handler
        w.WriteHeader(http.StatusOK)
    })

    server := &http.Server{
        Addr:      ":8443",
        Handler:   mux,
        TLSConfig: tlsConfig,
    }

    go func() {
        if err := server.ListenAndServeTLS("/etc/tls/tls.crt", "/etc/tls/tls.key"); err != nil {
            fmt.Printf("Metrics server error: %v\n", err)
        }
    }()

    return nil
}
```

### Example: Configuration Sync Service

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/openshift/tls-configurator/pkg/client"
    "github.com/openshift/tls-configurator/pkg/controller"
    configv1 "github.com/openshift/api/config/v1"
)

type ConfigSyncer struct {
    apiClient  *client.APIServerClient
    ingressClient *client.Client
}

func NewConfigSyncer(config *rest.Config) (*ConfigSyncer, error) {
    apiClient, err := client.NewAPIServerClient(config)
    if err != nil {
        return nil, err
    }

    ingressClient, err := client.NewClient(config, "openshift-ingress-operator")
    if err != nil {
        return nil, err
    }

    return &ConfigSyncer{
        apiClient:     apiClient,
        ingressClient: ingressClient,
    }, nil
}

func (s *ConfigSyncer) SyncTLSProfiles(ctx context.Context) error {
    // Get cluster-wide profile
    clusterProfile, err := s.apiClient.GetEffectiveTLSProfile(ctx)
    if err != nil {
        return fmt.Errorf("failed to get cluster profile: %w", err)
    }

    // List all IngressControllers
    list, err := s.ingressClient.ListIngressControllers(ctx)
    if err != nil {
        return fmt.Errorf("failed to list ingress controllers: %w", err)
    }

    // Sync each IngressController
    for _, ic := range list.Items {
        currentProfile := ic.Spec.TLSSecurityProfile

        // Compare profiles
        if !controller.CompareTLSProfiles(currentProfile, clusterProfile) {
            // Profiles match, skip
            continue
        }

        // Update IngressController to match cluster profile
        fmt.Printf("Syncing IngressController %s/%s\n", ic.Namespace, ic.Name)
        if err := s.ingressClient.UpdateTLSSecurityProfile(ctx, ic.Name, clusterProfile); err != nil {
            fmt.Printf("Failed to sync %s: %v\n", ic.Name, err)
            continue
        }
    }

    return nil
}

func (s *ConfigSyncer) RunPeriodically(interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for range ticker.C {
        ctx := context.Background()
        if err := s.SyncTLSProfiles(ctx); err != nil {
            fmt.Printf("Sync error: %v\n", err)
        }
    }
}
```

---

## Error Handling

### Common Errors and Solutions

#### Version Check Failed

```go
err := checker.ValidateMinimumVersion(ctx)
if err != nil {
    // Handle version error
    if strings.Contains(err.Error(), "does not meet minimum requirement") {
        log.Error("Cluster version too old, upgrade to OpenShift 4.22+")
        return err
    }
}
```

#### Profile Validation Failed

```go
err := client.ValidateTLSProfile(profile)
if err != nil {
    // Handle validation error
    log.Error(err, "Invalid TLS profile configuration")
    // Don't apply invalid profile
    return err
}
```

#### Conversion Error

```go
tlsConfig, err := crypto.ConvertTLSProfile(profile)
if err != nil {
    // Handle conversion error
    log.Error(err, "Failed to convert TLS profile")
    // Fall back to default
    tlsConfig = crypto.GetDefaultTLSConfig()
}
```

### Error Wrapping

```go
import "fmt"

func MyFunction() error {
    profile, err := apiClient.GetClusterTLSProfile(ctx)
    if err != nil {
        return fmt.Errorf("failed to get cluster profile: %w", err)
    }

    tlsConfig, err := crypto.ConvertTLSProfile(profile)
    if err != nil {
        return fmt.Errorf("failed to convert profile: %w", err)
    }

    return nil
}
```

---

## Best Practices

### 1. Always Check Version First

```go
// Good
func UpdateTLS(ctx context.Context) error {
    // Check version before any updates
    if err := checker.ValidateMinimumVersion(ctx); err != nil {
        return err
    }

    // Proceed with updates
    return applyUpdates(ctx)
}
```

### 2. Use APIServer for Cluster-Wide Settings

```go
// Recommended
apiClient, _ := client.NewAPIServerClient(config)
profile, _ := apiClient.GetEffectiveTLSProfile(ctx)

// Instead of
ingressClient, _ := client.NewClient(config, namespace)
profile, _ := ingressClient.GetTLSSecurityProfile(ctx, "default")
```

### 3. Cache TLS Configuration

```go
type MyService struct {
    tlsConfig     *tls.Config
    lastRefresh   time.Time
    refreshInterval time.Duration
}

func (s *MyService) GetTLSConfig() (*tls.Config, error) {
    if time.Since(s.lastRefresh) > s.refreshInterval {
        // Refresh from cluster
        profile, err := s.apiClient.GetEffectiveTLSProfile(ctx)
        if err != nil {
            // Return cached config on error
            return s.tlsConfig, nil
        }

        s.tlsConfig, _ = crypto.ConvertTLSProfile(profile)
        s.lastRefresh = time.Now()
    }

    return s.tlsConfig, nil
}
```

### 4. Handle nil Profiles Gracefully

```go
// ConvertTLSProfile handles nil profiles
tlsConfig, err := crypto.ConvertTLSProfile(nil)
// Returns default Intermediate profile

// When profile might be nil
profile, err := apiClient.GetClusterTLSProfile(ctx)
if profile == nil {
    // Use effective profile (applies defaults)
    profile, err = apiClient.GetEffectiveTLSProfile(ctx)
}
```

### 5. Use Context Properly

```go
// Create context with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// Pass context to all operations
profile, err := apiClient.GetEffectiveTLSProfile(ctx)
```

### 6. Validate Before Applying

```go
func ApplyProfile(ctx context.Context, profile *configv1.TLSSecurityProfile) error {
    // Always validate first
    if err := client.ValidateTLSProfile(profile); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // Then apply
    return ctrl.UpdateTLSProfile(ctx, profile)
}
```

---

## Integration Patterns

### Pattern 1: Initialization

```go
type MyApp struct {
    tlsConfig *tls.Config
}

func (a *MyApp) Initialize() error {
    // Get Kubernetes config
    config, err := ctrl.GetConfig()
    if err != nil {
        return err
    }

    // Check OpenShift version
    checker, err := client.NewVersionChecker(config)
    if err != nil {
        return err
    }

    if err := checker.ValidateMinimumVersion(context.Background()); err != nil {
        return err
    }

    // Get and convert TLS profile
    apiClient, err := client.NewAPIServerClient(config)
    if err != nil {
        return err
    }

    profile, err := apiClient.GetEffectiveTLSProfile(context.Background())
    if err != nil {
        return err
    }

    a.tlsConfig, err = crypto.ConvertTLSProfile(profile)
    return err
}
```

### Pattern 2: Periodic Refresh

```go
func (a *MyApp) StartTLSRefresher(interval time.Duration) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()

        for range ticker.C {
            if err := a.refreshTLSConfig(); err != nil {
                log.Error(err, "failed to refresh TLS config")
            }
        }
    }()
}
```

### Pattern 3: Fallback on Error

```go
func (a *MyApp) GetTLSConfig() *tls.Config {
    // Try to get from cluster
    profile, err := a.apiClient.GetEffectiveTLSProfile(context.Background())
    if err != nil {
        // Fall back to cached or default
        if a.tlsConfig != nil {
            return a.tlsConfig
        }
        return crypto.GetDefaultTLSConfig()
    }

    config, err := crypto.ConvertTLSProfile(profile)
    if err != nil {
        return crypto.GetDefaultTLSConfig()
    }

    return config
}
```

---

## Testing Your Integration

### Unit Test Example

```go
package myapp_test

import (
    "testing"

    "github.com/openshift/tls-configurator/pkg/crypto"
    configv1 "github.com/openshift/api/config/v1"
)

func TestTLSConfiguration(t *testing.T) {
    // Create test profile
    profile := &configv1.TLSSecurityProfile{
        Type: configv1.TLSProfileModernType,
    }

    // Convert
    tlsConfig, err := crypto.ConvertTLSProfile(profile)
    if err != nil {
        t.Fatalf("conversion failed: %v", err)
    }

    // Verify
    if tlsConfig.MinVersion != tls.VersionTLS13 {
        t.Errorf("expected TLS 1.3, got %x", tlsConfig.MinVersion)
    }
}
```

---

## Troubleshooting

### Issue: Import Error

**Problem:**
```
cannot find package "github.com/openshift/tls-configurator/pkg/client"
```

**Solution:**
```bash
go get github.com/openshift/tls-configurator
go mod tidy
```

### Issue: Version Check Fails

**Problem:**
```
Error: failed to get cluster version: clusterversions.config.openshift.io "version" not found
```

**Solution:**
- Verify running on OpenShift (not vanilla Kubernetes)
- Check RBAC permissions for ClusterVersion resource
- Verify kubeconfig is correct

### Issue: nil Pointer Error

**Problem:**
```
panic: runtime error: invalid memory address or nil pointer dereference
```

**Solution:**
```go
// Always check for nil
if profile == nil {
    profile, _ = apiClient.GetEffectiveTLSProfile(ctx)
}

if profile.Custom == nil {
    // Handle appropriately
}
```

---

## Additional Resources

- **Main Documentation**: [README.md](README.md)
- **Version Check Feature**: [VERSION_CHECK_FEATURE.md](VERSION_CHECK_FEATURE.md)
- **PR #316 Updates**: [UPDATES_FROM_PR316.md](UPDATES_FROM_PR316.md)
- **Project Summary**: [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md)

---

## License

Apache License 2.0 - See LICENSE file for details

---

## Support

For issues and questions:
- GitHub Issues: https://github.com/openshift/tls-configurator/issues
- Documentation: This file and related docs in the repository
