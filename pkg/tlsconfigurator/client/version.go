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
package client

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	configclient "github.com/openshift/client-go/config/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const (
	// MinimumOpenShiftMajorVersion is the minimum required OpenShift major version
	MinimumOpenShiftMajorVersion = 4
	// MinimumOpenShiftMinorVersion is the minimum required OpenShift minor version
	MinimumOpenShiftMinorVersion = 22
)

// VersionChecker handles OpenShift version detection and validation
type VersionChecker struct {
	configClient configclient.Interface
}

// OpenShiftVersion represents an OpenShift version
type OpenShiftVersion struct {
	Major int
	Minor int
	Patch int
	Full  string
}

// NewVersionChecker creates a new version checker
func NewVersionChecker(config *rest.Config) (*VersionChecker, error) {
	configClient, err := configclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create config client: %w", err)
	}

	return &VersionChecker{
		configClient: configClient,
	}, nil
}

// GetOpenShiftVersion retrieves the OpenShift cluster version
func (v *VersionChecker) GetOpenShiftVersion(ctx context.Context) (*OpenShiftVersion, error) {
	// Get ClusterVersion resource (always named "version")
	clusterVersion, err := v.configClient.ConfigV1().ClusterVersions().Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster version: %w", err)
	}

	// Get the current version from status
	if len(clusterVersion.Status.History) == 0 {
		return nil, fmt.Errorf("no version history available")
	}

	// The first entry is the current/target version
	currentVersion := clusterVersion.Status.History[0].Version
	if currentVersion == "" {
		return nil, fmt.Errorf("cluster version is empty")
	}

	// Parse the version string (format: X.Y.Z or X.Y.Z-suffix)
	version, err := parseVersion(currentVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version %s: %w", currentVersion, err)
	}

	version.Full = currentVersion
	return version, nil
}

// IsMinimumVersion checks if the cluster meets the minimum version requirement
func (v *VersionChecker) IsMinimumVersion(ctx context.Context) (bool, *OpenShiftVersion, error) {
	version, err := v.GetOpenShiftVersion(ctx)
	if err != nil {
		return false, nil, err
	}

	meetsRequirement := version.IsAtLeast(MinimumOpenShiftMajorVersion, MinimumOpenShiftMinorVersion)
	return meetsRequirement, version, nil
}

// ValidateMinimumVersion validates that the cluster meets minimum version requirements
// Returns an error if the version is too old
func (v *VersionChecker) ValidateMinimumVersion(ctx context.Context) error {
	meets, version, err := v.IsMinimumVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster version: %w", err)
	}

	if !meets {
		return fmt.Errorf(
			"OpenShift version %s does not meet minimum requirement (4.22+). "+
				"This tool requires OpenShift 4.22 or later for proper TLS profile support",
			version.Full,
		)
	}

	return nil
}

// IsAtLeast checks if this version is at least the specified major.minor version
func (v *OpenShiftVersion) IsAtLeast(major, minor int) bool {
	if v.Major > major {
		return true
	}
	if v.Major == major && v.Minor >= minor {
		return true
	}
	return false
}

// String returns a string representation of the version
func (v *OpenShiftVersion) String() string {
	if v.Full != "" {
		return v.Full
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// parseVersion parses a version string like "4.22.0" or "4.22.0-rc.1"
func parseVersion(versionStr string) (*OpenShiftVersion, error) {
	// Remove any suffix (e.g., -rc.1, -0.nightly-2024-01-01)
	parts := strings.Split(versionStr, "-")
	mainVersion := parts[0]

	// Split into major.minor.patch
	versionParts := strings.Split(mainVersion, ".")
	if len(versionParts) < 2 {
		return nil, fmt.Errorf("invalid version format: %s", versionStr)
	}

	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %s", versionParts[0])
	}

	minor, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version: %s", versionParts[1])
	}

	patch := 0
	if len(versionParts) >= 3 {
		patch, err = strconv.Atoi(versionParts[2])
		if err != nil {
			// Patch might have suffix, ignore error and use 0
			patch = 0
		}
	}

	return &OpenShiftVersion{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}
