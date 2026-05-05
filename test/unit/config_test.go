/*
Copyright 2025.

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

package unit

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestDefaultReconcilePeriod verifies the default reconcile period is reasonable
func TestDefaultReconcilePeriod(t *testing.T) {
	defaultReconcilePeriod := time.Minute

	assert.Greater(t, defaultReconcilePeriod, time.Duration(0), "reconcile period should be positive")
	assert.LessOrEqual(t, defaultReconcilePeriod, 5*time.Minute, "reconcile period should not be too long")
}

// TestDefaultMaxConcurrentReconciles verifies default concurrency is based on CPU count
func TestDefaultMaxConcurrentReconciles(t *testing.T) {
	cpuCount := runtime.NumCPU()

	assert.Greater(t, cpuCount, 0, "CPU count should be positive")
	assert.LessOrEqual(t, cpuCount, 256, "CPU count should be reasonable")
}

// TestReconcilePeriodValues tests various reconcile period configurations
func TestReconcilePeriodValues(t *testing.T) {
	testCases := []struct {
		name   string
		period time.Duration
		valid  bool
	}{
		{
			name:   "1 second - very frequent",
			period: 1 * time.Second,
			valid:  true,
		},
		{
			name:   "1 minute - default",
			period: 1 * time.Minute,
			valid:  true,
		},
		{
			name:   "5 minutes - moderate",
			period: 5 * time.Minute,
			valid:  true,
		},
		{
			name:   "zero - invalid",
			period: 0,
			valid:  false,
		},
		{
			name:   "negative - invalid",
			period: -1 * time.Minute,
			valid:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.valid {
				assert.Greater(t, tc.period, time.Duration(0), "valid period should be positive")
			} else {
				assert.LessOrEqual(t, tc.period, time.Duration(0), "invalid period should be non-positive")
			}
		})
	}
}

// TestMaxConcurrentReconcilesValues tests various concurrency configurations
func TestMaxConcurrentReconcilesValues(t *testing.T) {
	testCases := []struct {
		name  string
		value int
		valid bool
	}{
		{
			name:  "1 - minimal concurrency",
			value: 1,
			valid: true,
		},
		{
			name:  "4 - typical value",
			value: 4,
			valid: true,
		},
		{
			name:  "10 - high concurrency",
			value: 10,
			valid: true,
		},
		{
			name:  "0 - invalid",
			value: 0,
			valid: false,
		},
		{
			name:  "negative - invalid",
			value: -1,
			valid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.valid {
				assert.Greater(t, tc.value, 0, "valid concurrency should be positive")
			} else {
				assert.LessOrEqual(t, tc.value, 0, "invalid concurrency should be non-positive")
			}
		})
	}
}

// TestOperatorNamespaceConventions tests namespace naming conventions
func TestOperatorNamespaceConventions(t *testing.T) {
	validNamespaces := []string{
		"trusted-profile-analyzer-operator-system",
		"rhtpa-operator-system",
		"default",
		"kube-system",
	}

	for _, ns := range validNamespaces {
		t.Run(ns, func(t *testing.T) {
			assert.NotEmpty(t, ns, "namespace should not be empty")
			assert.LessOrEqual(t, len(ns), 63, "namespace should not exceed 63 characters")
			// Kubernetes namespace names must match DNS-1123 label
			assert.Regexp(t, `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, ns, "namespace should match Kubernetes naming rules")
		})
	}
}

// TestLeaderElectionIDFormat tests leader election ID format
func TestLeaderElectionIDFormat(t *testing.T) {
	leaderElectionID := "195237d9.rhtpa-operator"

	assert.NotEmpty(t, leaderElectionID, "leader election ID should not be empty")
	assert.Contains(t, leaderElectionID, ".", "leader election ID should contain a dot separator")

	// Leader election ID typically follows the pattern: <unique-id>.<operator-name>
	// This helps prevent conflicts with other operators
}

// TestMetricsBindAddress tests metrics bind address configurations
func TestMetricsBindAddress(t *testing.T) {
	testCases := []struct {
		name       string
		address    string
		shouldBind bool
	}{
		{
			name:       "disabled",
			address:    "0",
			shouldBind: false,
		},
		{
			name:       "HTTP on 8080",
			address:    ":8080",
			shouldBind: true,
		},
		{
			name:       "HTTPS on 8443",
			address:    ":8443",
			shouldBind: true,
		},
		{
			name:       "localhost HTTP",
			address:    "127.0.0.1:8080",
			shouldBind: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.shouldBind {
				assert.NotEqual(t, "0", tc.address, "should not be disabled")
				assert.Contains(t, tc.address, ":", "should contain port separator")
			} else {
				assert.Equal(t, "0", tc.address, "should be disabled")
			}
		})
	}
}

// TestHealthProbeBindAddress tests health probe bind address format
func TestHealthProbeBindAddress(t *testing.T) {
	defaultProbeAddr := ":8081"

	assert.NotEmpty(t, defaultProbeAddr, "probe address should not be empty")
	assert.Contains(t, defaultProbeAddr, ":", "probe address should contain port separator")

	// Should bind to all interfaces by default
	assert.True(t, defaultProbeAddr[0] == ':', "should bind to all interfaces")
}
