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
package controller

import (
	"context"
	"fmt"
	"log"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/client"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/config"
)

// TLSController manages TLS configurations for IngressControllers
type TLSController struct {
	client         *client.Client
	config         *config.Config
	versionChecker *client.VersionChecker
}

// NewTLSController creates a new TLSController
func NewTLSController(cfg *config.Config) (*TLSController, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	restConfig, err := cfg.GetKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	c, err := client.NewClient(restConfig, cfg.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	versionChecker, err := client.NewVersionChecker(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create version checker: %w", err)
	}

	return &TLSController{
		client:         c,
		config:         cfg,
		versionChecker: versionChecker,
	}, nil
}

// GetCurrentTLSProfile retrieves the current TLS profile from the IngressController
func (tc *TLSController) GetCurrentTLSProfile(ctx context.Context) (*configv1.TLSSecurityProfile, error) {
	log.Printf("Retrieving current TLS profile for IngressController: %s/%s",
		tc.config.Namespace, tc.config.IngressControllerName)

	profile, err := tc.client.GetTLSSecurityProfile(ctx, tc.config.IngressControllerName)
	if err != nil {
		return nil, err
	}

	log.Printf("Current TLS profile type: %s", profile.Type)
	if profile.Custom != nil {
		log.Printf("  MinTLSVersion: %s", profile.Custom.MinTLSVersion)
		log.Printf("  Ciphers: %v", profile.Custom.Ciphers)
	}

	return profile, nil
}

// UpdateTLSProfile updates the TLS profile of the IngressController
func (tc *TLSController) UpdateTLSProfile(ctx context.Context, profile *configv1.TLSSecurityProfile) error {
	log.Printf("Updating TLS profile for IngressController: %s/%s",
		tc.config.Namespace, tc.config.IngressControllerName)

	// Check OpenShift version before applying changes
	if err := tc.versionChecker.ValidateMinimumVersion(ctx); err != nil {
		return fmt.Errorf("version check failed: %w", err)
	}

	version, _ := tc.versionChecker.GetOpenShiftVersion(ctx)
	log.Printf("OpenShift version %s meets minimum requirement (4.22+)", version.String())

	// Validate the profile before applying
	if err := client.ValidateTLSProfile(profile); err != nil {
		return fmt.Errorf("invalid TLS profile: %w", err)
	}

	// Update the IngressController
	if err := tc.client.UpdateTLSSecurityProfile(ctx, tc.config.IngressControllerName, profile); err != nil {
		return fmt.Errorf("failed to update TLS profile: %w", err)
	}

	log.Printf("Successfully updated TLS profile")
	return nil
}

// ApplyTLSConfiguration applies a TLS configuration to the IngressController
func (tc *TLSController) ApplyTLSConfiguration(ctx context.Context, tlsConfig *config.TLSConfig) error {
	log.Printf("Applying TLS configuration to IngressController: %s/%s",
		tc.config.Namespace, tc.config.IngressControllerName)

	// Build the TLS profile from the configuration
	profile := config.BuildTLSProfile(tlsConfig)
	if profile == nil {
		return fmt.Errorf("failed to build TLS profile from configuration")
	}

	// Apply the profile
	return tc.UpdateTLSProfile(ctx, profile)
}

// ListIngressControllers lists all IngressControllers and their TLS configurations
func (tc *TLSController) ListIngressControllers(ctx context.Context) ([]IngressControllerInfo, error) {
	log.Printf("Listing IngressControllers in namespace: %s", tc.config.Namespace)

	list, err := tc.client.ListIngressControllers(ctx)
	if err != nil {
		return nil, err
	}

	var result []IngressControllerInfo
	for _, ic := range list.Items {
		info := IngressControllerInfo{
			Name:      ic.Name,
			Namespace: ic.Namespace,
		}

		if ic.Spec.TLSSecurityProfile != nil {
			info.TLSProfile = ic.Spec.TLSSecurityProfile
			info.HasTLSProfile = true
		}

		result = append(result, info)
	}

	log.Printf("Found %d IngressControllers", len(result))
	return result, nil
}

// IngressControllerInfo contains information about an IngressController
type IngressControllerInfo struct {
	Name          string
	Namespace     string
	TLSProfile    *configv1.TLSSecurityProfile
	HasTLSProfile bool
}

// CompareTLSProfiles compares two TLS profiles and returns true if they are different
func CompareTLSProfiles(profile1, profile2 *configv1.TLSSecurityProfile) bool {
	if profile1 == nil && profile2 == nil {
		return false
	}
	if profile1 == nil || profile2 == nil {
		return true
	}

	if profile1.Type != profile2.Type {
		return true
	}

	// Compare custom profiles if both are custom type
	if profile1.Type == configv1.TLSProfileCustomType {
		return compareCustomProfiles(profile1.Custom, profile2.Custom)
	}

	return false
}

func compareCustomProfiles(custom1, custom2 *configv1.CustomTLSProfile) bool {
	if custom1 == nil && custom2 == nil {
		return false
	}
	if custom1 == nil || custom2 == nil {
		return true
	}

	if custom1.MinTLSVersion != custom2.MinTLSVersion {
		return true
	}

	if len(custom1.Ciphers) != len(custom2.Ciphers) {
		return true
	}

	// Check ciphers
	cipherMap := make(map[string]bool)
	for _, cipher := range custom1.Ciphers {
		cipherMap[cipher] = true
	}
	for _, cipher := range custom2.Ciphers {
		if !cipherMap[cipher] {
			return true
		}
	}

	return false
}

// CheckOpenShiftVersion checks and returns the OpenShift cluster version
func (tc *TLSController) CheckOpenShiftVersion(ctx context.Context) (*client.OpenShiftVersion, error) {
	return tc.versionChecker.GetOpenShiftVersion(ctx)
}

// ValidateOpenShiftVersion validates that the cluster meets minimum version requirements
func (tc *TLSController) ValidateOpenShiftVersion(ctx context.Context) error {
	return tc.versionChecker.ValidateMinimumVersion(ctx)
}
