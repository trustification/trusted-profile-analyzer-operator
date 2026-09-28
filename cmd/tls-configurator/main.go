/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/client"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/config"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/controller"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/crypto"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/reconcile"
)

var (
	kubeconfig        = flag.String("kubeconfig", "", "Path to kubeconfig file (uses in-cluster config if not set)")
	ingressController = flag.String("ingress-controller", "default", "Name of the IngressController to modify")
	namespace         = flag.String("namespace", "openshift-ingress-operator", "Namespace of the IngressController")
	action            = flag.String("action", "get",
		"Action to perform: get, update, list, get-cluster, show-tlsconfig, check-version, validate, reconcile")
	tlsType       = flag.String("type", "Custom", "TLS profile type: Custom, Intermediate, Modern, Old")
	minTLSVersion = flag.String("min-tls-version", "VersionTLS13",
		"Minimum TLS version: VersionTLS10, VersionTLS11, VersionTLS12, VersionTLS13")
	ciphers = flag.String("ciphers", "", "Comma-separated list of ciphers")
	// TODO: these two flags are accepted for CLI compatibility but are currently
	// inert -- nothing reads them. The OpenShift 4.22 gate they relate to
	// (controller.ValidateOpenShiftVersion) also has no callers, so there is
	// presently nothing for --skip-version-check to skip. Kept as-is by the
	// operator merge to avoid changing runtime behaviour; wire up or remove
	// separately.
	//nolint:unused // accepted for CLI compatibility; see TODO above
	useCluster = flag.Bool("use-cluster-profile", false, "Use cluster-wide APIServer TLS profile (recommended)")
	//nolint:unused // accepted for CLI compatibility; see TODO above
	skipVersionCheck = flag.Bool("skip-version-check", false, "Skip OpenShift version check (not recommended)")
	enablePQC        = flag.Bool("enable-pqc", false,
		"Enforce post-quantum, TLS 1.3-only key exchange (X25519MLKEM768)")
	targetNamespace = flag.String("target-namespace", "",
		"Namespace of the workloads to roll out on TLS change (reconcile mode)")
	targetDeployments = flag.String("target-deployments", "",
		"Comma-separated Deployment names to roll out on TLS change (reconcile mode)")
	resyncPeriod = flag.Duration("resync-period", 5*time.Minute,
		"Periodic drift-correction interval (reconcile mode)")
)

func main() {
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create configuration
	cfg := config.NewConfig()
	cfg.Kubeconfig = *kubeconfig
	cfg.IngressControllerName = *ingressController
	cfg.Namespace = *namespace
	cfg.EnablePQC = *enablePQC
	cfg.TargetNamespace = *targetNamespace
	cfg.TargetDeployments = parseCiphers(*targetDeployments)
	cfg.ResyncPeriod = *resyncPeriod

	// The reconcile action runs its own long-lived controller and does not use
	// the one-shot TLSController.
	if *action == "reconcile" {
		if err := reconcileAction(ctx, cfg); err != nil {
			log.Fatalf("Reconciler failed: %v", err)
		}
		return
	}

	// Create controller
	ctrl, err := controller.NewTLSController(cfg)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}

	// Perform action
	switch *action {
	case "get":
		if err := getAction(ctx, ctrl); err != nil {
			log.Fatalf("Failed to get TLS profile: %v", err)
		}
	case "update":
		if err := updateAction(ctx, ctrl); err != nil {
			log.Fatalf("Failed to update TLS profile: %v", err)
		}
	case "list":
		if err := listAction(ctx, ctrl); err != nil {
			log.Fatalf("Failed to list IngressControllers: %v", err)
		}
	case "get-cluster":
		if err := getClusterAction(ctx, cfg); err != nil {
			log.Fatalf("Failed to get cluster TLS profile: %v", err)
		}
	case "show-tlsconfig":
		if err := showTLSConfigAction(ctx, cfg); err != nil {
			log.Fatalf("Failed to show TLS config: %v", err)
		}
	case "check-version":
		if err := checkVersionAction(ctx, ctrl); err != nil {
			log.Fatalf("Failed to check version: %v", err)
		}
	case "validate":
		if err := validateAction(ctx, cfg); err != nil {
			log.Fatalf("PQC/TLS 1.3 compliance check failed: %v", err)
		}
	default:
		log.Fatalf("Unknown action: %s. Valid actions are: get, update, list, get-cluster, "+
			"show-tlsconfig, check-version, validate, reconcile", *action)
	}
}

// reconcileAction runs the long-lived reconciler that rolls target workloads
// whenever the cluster-wide TLS profile changes at runtime.
func reconcileAction(ctx context.Context, cfg *config.Config) error {
	r, err := reconcile.NewReconciler(cfg)
	if err != nil {
		return err
	}
	return r.Run(ctx)
}

// validateAction reports whether the effective cluster TLS configuration is
// post-quantum and TLS 1.3-only compliant. It exits non-zero when it is not.
func validateAction(ctx context.Context, cfg *config.Config) error {
	k8sConfig, err := config.GetKubeConfig(cfg.Kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	apiServerClient, err := client.NewAPIServerClient(k8sConfig)
	if err != nil {
		return fmt.Errorf("failed to create APIServer client: %w", err)
	}

	profile, err := apiServerClient.GetEffectiveTLSProfile(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster TLS profile: %w", err)
	}

	// Evaluate the profile as it would be enforced with PQC enabled.
	tlsConfig, err := crypto.ConvertTLSProfileWithPQC(profile, true)
	if err != nil {
		return fmt.Errorf("failed to convert TLS profile: %w", err)
	}

	compliant, reasons := crypto.IsPQCCompliant(tlsConfig)

	fmt.Println("Post-Quantum / TLS 1.3 Compliance Check")
	fmt.Println("=======================================")
	fmt.Printf("MinVersion:       %s\n", tlsVersionName(tlsConfig.MinVersion))
	fmt.Printf("Key-exchange:     ")
	for i, c := range tlsConfig.CurvePreferences {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(crypto.CurveName(c))
	}
	fmt.Println()

	if compliant {
		fmt.Println("\n✅ Status: COMPLIANT (post-quantum, TLS 1.3)")
		return nil
	}

	fmt.Println("\n❌ Status: NON-COMPLIANT")
	for _, r := range reasons {
		fmt.Printf("   - %s\n", r)
	}
	return fmt.Errorf("configuration is not PQC/TLS 1.3 compliant")
}

func getAction(ctx context.Context, ctrl *controller.TLSController) error {
	profile, err := ctrl.GetCurrentTLSProfile(ctx)
	if err != nil {
		return err
	}

	// Pretty print the profile
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	fmt.Println("Current TLS Security Profile:")
	fmt.Println(string(data))
	return nil
}

func updateAction(ctx context.Context, ctrl *controller.TLSController) error {
	// Parse TLS profile type
	profileType, err := parseTLSProfileType(*tlsType)
	if err != nil {
		return err
	}

	// Build TLS configuration
	tlsConfig := &config.TLSConfig{
		Type: profileType,
	}

	// For custom profiles, parse additional parameters
	if profileType == configv1.TLSProfileCustomType {
		if *ciphers == "" {
			return fmt.Errorf("ciphers must be specified for custom TLS profile")
		}

		tlsConfig.Ciphers = parseCiphers(*ciphers)
		tlsConfig.MinTLSVersion = configv1.TLSProtocolVersion(*minTLSVersion)
	}

	// Post-quantum key exchange is only defined for TLS 1.3, so enforce it.
	if *enablePQC {
		tlsConfig.EnablePQC = true
		if profileType == configv1.TLSProfileCustomType {
			tlsConfig.MinTLSVersion = configv1.VersionTLS13
		}
		log.Printf("PQC enabled: enforcing TLS 1.3 and X25519MLKEM768 key exchange")
		log.Printf("Note: the OpenShift TLSSecurityProfile API cannot store key-exchange groups; " +
			"the PQC group is negotiated by the Go runtime (>=1.24) and recorded in the workload rollout hash")
	}

	// Apply the configuration
	if err := ctrl.ApplyTLSConfiguration(ctx, tlsConfig); err != nil {
		return err
	}

	fmt.Println("Successfully updated TLS configuration")

	// Show the new configuration
	return getAction(ctx, ctrl)
}

func listAction(ctx context.Context, ctrl *controller.TLSController) error {
	ingressControllers, err := ctrl.ListIngressControllers(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Found %d IngressController(s):\n\n", len(ingressControllers))

	for _, ic := range ingressControllers {
		fmt.Printf("Name: %s/%s\n", ic.Namespace, ic.Name)
		if ic.HasTLSProfile {
			fmt.Printf("  TLS Profile Type: %s\n", ic.TLSProfile.Type)
			if ic.TLSProfile.Custom != nil {
				fmt.Printf("  Min TLS Version: %s\n", ic.TLSProfile.Custom.MinTLSVersion)
				fmt.Printf("  Ciphers: %v\n", ic.TLSProfile.Custom.Ciphers)
			}
		} else {
			fmt.Println("  TLS Profile: Not configured")
		}
		fmt.Println()
	}

	return nil
}

func parseTLSProfileType(typeStr string) (configv1.TLSProfileType, error) {
	switch strings.ToLower(typeStr) {
	case "custom":
		return configv1.TLSProfileCustomType, nil
	case "intermediate":
		return configv1.TLSProfileIntermediateType, nil
	case "modern":
		return configv1.TLSProfileModernType, nil
	case "old":
		return configv1.TLSProfileOldType, nil
	default:
		return "", fmt.Errorf("unknown TLS profile type: %s", typeStr)
	}
}

func parseCiphers(cipherStr string) []string {
	if cipherStr == "" {
		return nil
	}
	ciphers := strings.Split(cipherStr, ",")
	for i := range ciphers {
		ciphers[i] = strings.TrimSpace(ciphers[i])
	}
	return ciphers
}

// getClusterAction retrieves the cluster-wide TLS profile from APIServer (recommended approach)
func getClusterAction(ctx context.Context, cfg *config.Config) error {
	// Create k8s config
	k8sConfig, err := config.GetKubeConfig(cfg.Kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	// Create APIServer client
	apiServerClient, err := client.NewAPIServerClient(k8sConfig)
	if err != nil {
		return fmt.Errorf("failed to create APIServer client: %w", err)
	}

	// Get cluster TLS profile
	profile, err := apiServerClient.GetEffectiveTLSProfile(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster TLS profile: %w", err)
	}

	// Pretty print the profile
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	fmt.Println("Cluster-wide TLS Security Profile (from APIServer):")
	fmt.Println("===================================================")
	fmt.Println(string(data))
	fmt.Println()
	fmt.Println("ℹ️  This is the authoritative cluster-wide TLS configuration.")
	fmt.Println("   Per OpenShift best practices, read from APIServer CR 'cluster'.")

	return nil
}

// showTLSConfigAction demonstrates converting OpenShift profile to crypto/tls.Config
func showTLSConfigAction(ctx context.Context, cfg *config.Config) error {
	// Create k8s config
	k8sConfig, err := config.GetKubeConfig(cfg.Kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	// Create APIServer client
	apiServerClient, err := client.NewAPIServerClient(k8sConfig)
	if err != nil {
		return fmt.Errorf("failed to create APIServer client: %w", err)
	}

	// Get cluster TLS profile
	profile, err := apiServerClient.GetEffectiveTLSProfile(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster TLS profile: %w", err)
	}

	fmt.Println("OpenShift TLS Security Profile:")
	fmt.Println("================================")
	profileData, _ := json.MarshalIndent(profile, "", "  ")
	fmt.Println(string(profileData))
	fmt.Println()

	// Convert to crypto/tls.Config
	tlsConfig, err := crypto.ConvertTLSProfileWithPQC(profile, *enablePQC)
	if err != nil {
		return fmt.Errorf("failed to convert TLS profile: %w", err)
	}

	// Display crypto/tls.Config
	fmt.Println("Converted to crypto/tls.Config:")
	fmt.Println("================================")
	fmt.Printf("MinVersion: %s\n", tlsVersionName(tlsConfig.MinVersion))
	fmt.Printf("MaxVersion: %s\n", tlsVersionName(tlsConfig.MaxVersion))
	fmt.Printf("SessionTicketsDisabled: %v\n", tlsConfig.SessionTicketsDisabled)
	fmt.Printf("Renegotiation: %s\n", renegotiationName(tlsConfig.Renegotiation))
	if len(tlsConfig.CurvePreferences) > 0 {
		fmt.Printf("Key-exchange groups: ")
		for i, c := range tlsConfig.CurvePreferences {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(crypto.CurveName(c))
		}
		fmt.Println()
	}
	fmt.Printf("\nCipher Suites (%d configured):\n", len(tlsConfig.CipherSuites))
	for i, suite := range tlsConfig.CipherSuites {
		fmt.Printf("  %2d. %s (0x%04x)\n", i+1, cipherSuiteName(suite), suite)
	}

	fmt.Println()
	fmt.Println("ℹ️  This configuration can be directly used with:")
	fmt.Println("   - Kubernetes webhook servers")
	fmt.Println("   - Metrics endpoints")
	fmt.Println("   - HTTP/gRPC servers")
	fmt.Println("   - Any Go application using crypto/tls")

	return nil
}

// Helper functions for pretty printing
func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	case 0:
		return "Not specified"
	default:
		return fmt.Sprintf("Unknown (0x%04x)", version)
	}
}

func renegotiationName(r tls.RenegotiationSupport) string {
	switch r {
	case tls.RenegotiateNever:
		return "Never"
	case tls.RenegotiateOnceAsClient:
		return "Once as client"
	case tls.RenegotiateFreelyAsClient:
		return "Freely as client"
	default:
		return fmt.Sprintf("Unknown (%d)", r)
	}
}

func cipherSuiteName(id uint16) string {
	names := map[uint16]string{
		tls.TLS_AES_128_GCM_SHA256:                        "TLS_AES_128_GCM_SHA256",
		tls.TLS_AES_256_GCM_SHA384:                        "TLS_AES_256_GCM_SHA384",
		tls.TLS_CHACHA20_POLY1305_SHA256:                  "TLS_CHACHA20_POLY1305_SHA256",
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256:       "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:         "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384:       "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:         "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256: "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305",
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256:   "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256:       "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256",
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256:         "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA:          "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA:            "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA:          "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:            "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256:               "TLS_RSA_WITH_AES_128_GCM_SHA256",
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384:               "TLS_RSA_WITH_AES_256_GCM_SHA384",
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256:               "TLS_RSA_WITH_AES_128_CBC_SHA256",
		tls.TLS_RSA_WITH_AES_128_CBC_SHA:                  "TLS_RSA_WITH_AES_128_CBC_SHA",
		tls.TLS_RSA_WITH_AES_256_CBC_SHA:                  "TLS_RSA_WITH_AES_256_CBC_SHA",
	}

	if name, ok := names[id]; ok {
		return name
	}
	return fmt.Sprintf("Unknown_Cipher_0x%04x", id)
}

// checkVersionAction checks the OpenShift cluster version
func checkVersionAction(ctx context.Context, ctrl *controller.TLSController) error {
	version, err := ctrl.CheckOpenShiftVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster version: %w", err)
	}

	fmt.Println("OpenShift Cluster Version Check")
	fmt.Println("================================")
	fmt.Printf("Detected Version: %s\n", version.String())
	fmt.Printf("Major: %d\n", version.Major)
	fmt.Printf("Minor: %d\n", version.Minor)
	fmt.Printf("Patch: %d\n", version.Patch)
	fmt.Println()

	// Check if version meets requirements
	meetsRequirement := version.IsAtLeast(client.MinimumOpenShiftMajorVersion, client.MinimumOpenShiftMinorVersion)

	fmt.Printf("Minimum Required: %d.%d\n", client.MinimumOpenShiftMajorVersion, client.MinimumOpenShiftMinorVersion)

	if meetsRequirement {
		fmt.Println()
		fmt.Println("✅ Status: COMPATIBLE")
		fmt.Println("   This cluster meets the minimum version requirement.")
		fmt.Println("   TLS configuration changes can be safely applied.")
	} else {
		fmt.Println()
		fmt.Println("❌ Status: INCOMPATIBLE")
		fmt.Println("   This cluster does NOT meet the minimum version requirement.")
		fmt.Println("   TLS configuration changes will be blocked.")
		fmt.Println()
		fmt.Println("   Please upgrade to OpenShift 4.22 or later before applying")
		fmt.Println("   TLS configuration changes.")
		return fmt.Errorf("cluster version %s is below minimum requirement (4.22)", version.String())
	}

	return nil
}
