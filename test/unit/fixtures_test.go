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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestFixturesExist(t *testing.T) {
	fixtures := []string{
		"minimal_cr.yaml",
		"valid_cr.yaml",
		"server_only_cr.yaml",
		"importer_only_cr.yaml",
	}

	fixturesDir := filepath.Join("..", "fixtures")

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			path := filepath.Join(fixturesDir, fixture)
			_, err := os.Stat(path)
			assert.NoError(t, err, "fixture file should exist: %s", fixture)
		})
	}
}

func TestMinimalCRFixture(t *testing.T) {
	cr := loadFixture(t, "minimal_cr.yaml")

	assert.Equal(t, "rhtpa.io/v1", cr.GetAPIVersion())
	assert.Equal(t, "TrustedProfileAnalyzer", cr.GetKind())
	assert.NotEmpty(t, cr.GetName())

	spec, found, err := unstructured.NestedMap(cr.Object, "spec")
	require.NoError(t, err)
	require.True(t, found, "spec should exist")

	appDomain, found, err := unstructured.NestedString(spec, "appDomain")
	require.NoError(t, err)
	require.True(t, found, "appDomain should exist")
	assert.NotEmpty(t, appDomain, "appDomain should not be empty")
}

func TestValidCRFixture(t *testing.T) {
	cr := loadFixture(t, "valid_cr.yaml")

	spec, found, err := unstructured.NestedMap(cr.Object, "spec")
	require.NoError(t, err)
	require.True(t, found)

	// Check modules
	modules, found, err := unstructured.NestedMap(spec, "modules")
	require.NoError(t, err)
	require.True(t, found, "modules should exist")

	// Check server module
	server, found, err := unstructured.NestedMap(modules, "server")
	require.NoError(t, err)
	require.True(t, found, "server module should exist")

	serverEnabled, found, err := unstructured.NestedBool(server, "enabled")
	require.NoError(t, err)
	require.True(t, found)
	assert.True(t, serverEnabled, "server should be enabled")
}

func TestServerOnlyCRFixture(t *testing.T) {
	cr := loadFixture(t, "server_only_cr.yaml")

	spec, _, _ := unstructured.NestedMap(cr.Object, "spec")
	modules, _, _ := unstructured.NestedMap(spec, "modules")

	server, found, _ := unstructured.NestedMap(modules, "server")
	require.True(t, found, "server module should exist")

	serverEnabled, _, _ := unstructured.NestedBool(server, "enabled")
	assert.True(t, serverEnabled)

	serverReplicas, _, _ := unstructured.NestedInt64(server, "replicas")
	assert.Equal(t, int64(1), serverReplicas)

	importer, found, _ := unstructured.NestedMap(modules, "importer")
	if found {
		importerEnabled, _, _ := unstructured.NestedBool(importer, "enabled")
		assert.False(t, importerEnabled, "importer should be disabled")
	}
}

func TestImporterOnlyCRFixture(t *testing.T) {
	cr := loadFixture(t, "importer_only_cr.yaml")

	spec, _, _ := unstructured.NestedMap(cr.Object, "spec")
	modules, _, _ := unstructured.NestedMap(spec, "modules")

	importer, found, _ := unstructured.NestedMap(modules, "importer")
	require.True(t, found, "importer module should exist")

	importerEnabled, _, _ := unstructured.NestedBool(importer, "enabled")
	assert.True(t, importerEnabled)

	server, found, _ := unstructured.NestedMap(modules, "server")
	if found {
		serverEnabled, _, _ := unstructured.NestedBool(server, "enabled")
		assert.False(t, serverEnabled, "server should be disabled")
	}
}

func TestCRMustHaveAppDomain(t *testing.T) {
	fixtures := []string{
		"minimal_cr.yaml",
		"valid_cr.yaml",
		"server_only_cr.yaml",
		"importer_only_cr.yaml",
	}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			cr := loadFixture(t, fixture)
			spec, found, err := unstructured.NestedMap(cr.Object, "spec")
			require.NoError(t, err)
			require.True(t, found)

			appDomain, found, err := unstructured.NestedString(spec, "appDomain")
			require.NoError(t, err)
			require.True(t, found, "appDomain is required in all CRs")
			assert.NotEmpty(t, appDomain, "appDomain should not be empty")
		})
	}
}

// Helper function to load a fixture file
func loadFixture(t *testing.T, filename string) *unstructured.Unstructured {
	t.Helper()

	fixturesDir := filepath.Join("..", "fixtures")
	fixturePath := filepath.Join(fixturesDir, filename)

	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "should be able to read fixture file")

	obj := &unstructured.Unstructured{}
	err = yaml.Unmarshal(data, &obj.Object)
	require.NoError(t, err, "should be able to unmarshal fixture YAML")

	return obj
}
