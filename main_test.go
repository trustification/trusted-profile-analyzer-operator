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

package main

import (
	"os"
	"testing"

	"github.com/operator-framework/helm-operator-plugins/pkg/watches"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchesFileLoad(t *testing.T) {
	// Test that watches.yaml exists and is valid
	ws, err := watches.Load("watches.yaml")
	require.NoError(t, err, "watches.yaml should be loadable")
	require.NotEmpty(t, ws, "watches should not be empty")

	// Verify the watch configuration
	assert.Equal(t, 1, len(ws), "should have exactly one watch")

	w := ws[0]
	assert.Equal(t, "rhtpa.io", w.Group, "group should be rhtpa.io")
	assert.Equal(t, "v1", w.Version, "version should be v1")
	assert.Equal(t, "TrustedProfileAnalyzer", w.Kind, "kind should be TrustedProfileAnalyzer")
	assert.Equal(t, "helm-charts/redhat-trusted-profile-analyzer", w.ChartPath, "chart path should be correct")
}

func TestWatchesConfiguration(t *testing.T) {
	ws, err := watches.Load("watches.yaml")
	require.NoError(t, err)

	w := ws[0]

	// Test MaxConcurrentReconciles configuration
	require.NotNil(t, w.MaxConcurrentReconciles, "MaxConcurrentReconciles should be set")
	assert.Equal(t, 4, *w.MaxConcurrentReconciles, "MaxConcurrentReconciles should be 4")

	// Test WatchDependentResources configuration
	require.NotNil(t, w.WatchDependentResources, "WatchDependentResources should be set")
	assert.False(t, *w.WatchDependentResources, "WatchDependentResources should be false")
}

func TestChartPathExists(t *testing.T) {
	ws, err := watches.Load("watches.yaml")
	require.NoError(t, err)

	chartPath := ws[0].ChartPath

	// Verify chart directory exists
	info, err := os.Stat(chartPath)
	require.NoError(t, err, "chart path should exist")
	assert.True(t, info.IsDir(), "chart path should be a directory")

	// Verify Chart.yaml exists
	chartYaml := chartPath + "/Chart.yaml"
	_, err = os.Stat(chartYaml)
	require.NoError(t, err, "Chart.yaml should exist")

	// Verify values.yaml exists
	valuesYaml := chartPath + "/values.yaml"
	_, err = os.Stat(valuesYaml)
	require.NoError(t, err, "values.yaml should exist")

	// Verify templates directory exists
	templatesDir := chartPath + "/templates"
	info, err = os.Stat(templatesDir)
	require.NoError(t, err, "templates directory should exist")
	assert.True(t, info.IsDir(), "templates should be a directory")
}

func TestDefaultConfiguration(t *testing.T) {
	// Test default values
	assert.Greater(t, defaultMaxConcurrentReconciles, 0, "default max concurrent reconciles should be positive")
	assert.NotZero(t, defaultReconcilePeriod, "default reconcile period should be set")
}

func TestInvalidWatchesFile(t *testing.T) {
	// Test loading a non-existent watches file
	_, err := watches.Load("nonexistent-watches.yaml")
	assert.Error(t, err, "should error on non-existent file")
}
