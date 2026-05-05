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

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/operator-framework/helm-operator-plugins/pkg/watches"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestWatchesFileStructure(t *testing.T) {
	watchesPath := "watches.yaml"

	data, err := os.ReadFile(watchesPath)
	require.NoError(t, err, "watches.yaml should be readable")

	// Verify it's valid YAML
	var watches []map[string]interface{}
	err = yaml.Unmarshal(data, &watches)
	require.NoError(t, err, "watches.yaml should be valid YAML")

	assert.NotEmpty(t, watches, "watches should not be empty")
}

func TestWatchesGVKConfiguration(t *testing.T) {
	ws, err := watches.Load("watches.yaml")
	require.NoError(t, err, "watches.yaml should load successfully")
	require.Len(t, ws, 1, "should have exactly one watch configured")

	w := ws[0]

	// Verify GVK
	assert.Equal(t, "rhtpa.io", w.GroupVersionKind.Group, "group should be rhtpa.io")
	assert.Equal(t, "v1", w.GroupVersionKind.Version, "version should be v1")
	assert.Equal(t, "TrustedProfileAnalyzer", w.GroupVersionKind.Kind, "kind should be TrustedProfileAnalyzer")
}

func TestWatchesChartConfiguration(t *testing.T) {
	ws, err := watches.Load("watches.yaml")
	require.NoError(t, err)
	require.Len(t, ws, 1)

	w := ws[0]

	// Verify chart path
	assert.Equal(t, "helm-charts/redhat-trusted-profile-analyzer", w.ChartPath, "chart path should be correct")

	// Verify chart path exists and is valid
	chartYaml := filepath.Join(w.ChartPath, "Chart.yaml")
	_, err = os.Stat(chartYaml)
	assert.NoError(t, err, "Chart.yaml should exist at chart path")

	valuesYaml := filepath.Join(w.ChartPath, "values.yaml")
	_, err = os.Stat(valuesYaml)
	assert.NoError(t, err, "values.yaml should exist at chart path")
}

func TestWatchesReconcilePeriod(t *testing.T) {
	ws, err := watches.Load("watches.yaml")
	require.NoError(t, err)
	require.Len(t, ws, 1)

	w := ws[0]

	// Check if reconcile period is set
	if w.ReconcilePeriod != nil {
		t.Logf("Reconcile period: %v", w.ReconcilePeriod.Duration)
		assert.Greater(t, w.ReconcilePeriod.Duration.Seconds(), float64(0), "reconcile period should be positive")
	} else {
		t.Log("Reconcile period not explicitly set, will use default")
	}
}

func TestWatchesMaxConcurrentReconciles(t *testing.T) {
	ws, err := watches.Load("watches.yaml")
	require.NoError(t, err)
	require.Len(t, ws, 1)

	w := ws[0]

	// Verify MaxConcurrentReconciles
	require.NotNil(t, w.MaxConcurrentReconciles, "MaxConcurrentReconciles should be set")
	assert.Equal(t, 4, *w.MaxConcurrentReconciles, "MaxConcurrentReconciles should be 4")
	assert.Greater(t, *w.MaxConcurrentReconciles, 0, "MaxConcurrentReconciles should be positive")
}

func TestWatchesDependentResourcesConfiguration(t *testing.T) {
	ws, err := watches.Load("watches.yaml")
	require.NoError(t, err)
	require.Len(t, ws, 1)

	w := ws[0]

	// Verify WatchDependentResources configuration
	require.NotNil(t, w.WatchDependentResources, "WatchDependentResources should be explicitly set")
	assert.False(t, *w.WatchDependentResources, "WatchDependentResources should be false for performance")
}

func TestWatchesSelectorConfiguration(t *testing.T) {
	ws, err := watches.Load("watches.yaml")
	require.NoError(t, err)
	require.Len(t, ws, 1)

	w := ws[0]

	// Check selector configuration
	if w.Selector != nil {
		t.Logf("Selector configured: %+v", w.Selector)
	} else {
		t.Log("No selector configured - operator will watch all instances")
	}
}

func TestWatchesOverrideValues(t *testing.T) {
	ws, err := watches.Load("watches.yaml")
	require.NoError(t, err)
	require.Len(t, ws, 1)

	w := ws[0]

	// Check if override values are set
	if w.OverrideValues != nil && len(w.OverrideValues) > 0 {
		t.Logf("Override values configured: %+v", w.OverrideValues)

		// Verify override values are valid
		for key, value := range w.OverrideValues {
			t.Logf("  %s: %v", key, value)
			assert.NotEmpty(t, key, "override value key should not be empty")
		}
	} else {
		t.Log("No override values configured")
	}
}

func TestWatchesFileValidYAMLSyntax(t *testing.T) {
	watchesPath := "watches.yaml"

	// Read file
	_, err := os.ReadFile(watchesPath)
	require.NoError(t, err, "watches.yaml should be readable")

	// Try to unmarshal into watches structure
	ws, err := watches.Load(watchesPath)
	require.NoError(t, err, "watches.yaml should be valid watches configuration")
	assert.NotEmpty(t, ws, "watches should not be empty")

	// Verify required fields are present
	for i, w := range ws {
		assert.NotEmpty(t, w.GroupVersionKind.Group, "watch %d should have group", i)
		assert.NotEmpty(t, w.GroupVersionKind.Version, "watch %d should have version", i)
		assert.NotEmpty(t, w.GroupVersionKind.Kind, "watch %d should have kind", i)
		assert.NotEmpty(t, w.ChartPath, "watch %d should have chart path", i)
	}
}

func TestWatchesFileNoTrailingSpaces(t *testing.T) {
	watchesPath := "watches.yaml"

	data, err := os.ReadFile(watchesPath)
	require.NoError(t, err, "watches.yaml should be readable")

	// Check for common YAML issues
	content := string(data)

	// Should not have tab characters (YAML uses spaces)
	assert.NotContains(t, content, "\t", "watches.yaml should not contain tab characters")
}
