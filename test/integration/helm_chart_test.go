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

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ChartYaml represents the Chart.yaml structure
type ChartYaml struct {
	APIVersion  string `yaml:"apiVersion"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Type        string `yaml:"type"`
	Version     string `yaml:"version"`
	AppVersion  string `yaml:"appVersion"`
	KubeVersion string `yaml:"kubeVersion"`
	Maintainers []struct {
		Name  string `yaml:"name"`
		URL   string `yaml:"url"`
		Email string `yaml:"email"`
	} `yaml:"maintainers"`
}

// ValuesYaml represents a subset of the values.yaml structure
type ValuesYaml struct {
	AppDomain string `yaml:"appDomain"`
	PartOf    string `yaml:"partOf"`
	Replicas  int    `yaml:"replicas"`
	Modules   struct {
		Server struct {
			Enabled  bool `yaml:"enabled"`
			Replicas int  `yaml:"replicas"`
		} `yaml:"server"`
		Importer struct {
			Enabled  bool `yaml:"enabled"`
			Replicas int  `yaml:"replicas"`
		} `yaml:"importer"`
	} `yaml:"modules"`
}

func TestChartYamlValid(t *testing.T) {
	chartPath := filepath.Join("helm-charts", "redhat-trusted-profile-analyzer", "Chart.yaml")

	data, err := os.ReadFile(chartPath)
	require.NoError(t, err, "Chart.yaml should be readable")

	var chart ChartYaml
	err = yaml.Unmarshal(data, &chart)
	require.NoError(t, err, "Chart.yaml should be valid YAML")

	// Validate required fields
	assert.Equal(t, "v2", chart.APIVersion, "Chart API version should be v2")
	assert.Equal(t, "redhat-trusted-profile-analyzer", chart.Name, "Chart name should match")
	assert.Equal(t, "application", chart.Type, "Chart type should be application")
	assert.NotEmpty(t, chart.Version, "Chart version should be set")
	assert.NotEmpty(t, chart.AppVersion, "App version should be set")
	assert.NotEmpty(t, chart.KubeVersion, "Kubernetes version should be specified")
	assert.NotEmpty(t, chart.Maintainers, "Maintainers should be specified")
}

func TestValuesYamlValid(t *testing.T) {
	valuesPath := filepath.Join("helm-charts", "redhat-trusted-profile-analyzer", "values.yaml")

	data, err := os.ReadFile(valuesPath)
	require.NoError(t, err, "values.yaml should be readable")

	var values ValuesYaml
	err = yaml.Unmarshal(data, &values)
	require.NoError(t, err, "values.yaml should be valid YAML")

	// Validate default values
	assert.Equal(t, "change-me", values.AppDomain, "appDomain should have placeholder")
	assert.Equal(t, "trustify", values.PartOf, "partOf should be trustify")
	assert.Equal(t, 1, values.Replicas, "default replicas should be 1")

	// Validate module defaults
	assert.True(t, values.Modules.Server.Enabled, "server module should be enabled by default")
	assert.True(t, values.Modules.Importer.Enabled, "importer module should be enabled by default")
}

func TestValuesSchemaExists(t *testing.T) {
	schemaPath := filepath.Join("helm-charts", "redhat-trusted-profile-analyzer", "values.schema.json")

	_, err := os.Stat(schemaPath)
	assert.NoError(t, err, "values.schema.json should exist")

	// Verify it's valid JSON
	data, err := os.ReadFile(schemaPath)
	require.NoError(t, err, "values.schema.json should be readable")

	var schema map[string]interface{}
	err = yaml.Unmarshal(data, &schema)
	require.NoError(t, err, "values.schema.json should be valid JSON")

	// Verify required fields are defined
	properties, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok, "schema should have properties")

	assert.Contains(t, properties, "appDomain", "schema should require appDomain")
}

func TestHelmTemplateStructure(t *testing.T) {
	templatesPath := filepath.Join("helm-charts", "redhat-trusted-profile-analyzer", "templates")

	// Check for required template directories
	requiredDirs := []string{
		"services/server",
		"services/importer",
		"init/create-database",
		"init/migrate-database",
		"init/create-importers",
		"helpers",
		"tests",
	}

	for _, dir := range requiredDirs {
		dirPath := filepath.Join(templatesPath, dir)
		info, err := os.Stat(dirPath)
		require.NoError(t, err, "template directory %s should exist", dir)
		assert.True(t, info.IsDir(), "%s should be a directory", dir)
	}
}

func TestHelmTemplatesExist(t *testing.T) {
	templatesPath := filepath.Join("helm-charts", "redhat-trusted-profile-analyzer", "templates")

	// Check for required template files
	requiredTemplates := []string{
		"services/server/030-Deployment.yaml",
		"services/server/040-Service.yaml",
		"services/server/050-Ingress.yaml",
		"services/importer/030-Deployment.yaml",
		"tests/test-server-connection.yaml",
	}

	for _, template := range requiredTemplates {
		templatePath := filepath.Join(templatesPath, template)
		_, err := os.Stat(templatePath)
		assert.NoError(t, err, "template %s should exist", template)
	}
}

func TestHelmNotesExists(t *testing.T) {
	notesPath := filepath.Join("helm-charts", "redhat-trusted-profile-analyzer", "templates", "NOTES.txt")

	_, err := os.Stat(notesPath)
	assert.NoError(t, err, "NOTES.txt should exist")

	data, err := os.ReadFile(notesPath)
	require.NoError(t, err, "NOTES.txt should be readable")
	assert.NotEmpty(t, data, "NOTES.txt should not be empty")
}

func TestHelmIgnoreExists(t *testing.T) {
	helmignorePath := filepath.Join("helm-charts", "redhat-trusted-profile-analyzer", ".helmignore")

	_, err := os.Stat(helmignorePath)
	assert.NoError(t, err, ".helmignore should exist")
}

// TestImporterCredentialAllowlistsRenderOnImporterDeployment verifies configured allowlists reach only the importer.
func TestImporterCredentialAllowlistsRenderOnImporterDeployment(t *testing.T) {
	// Given importer allowlists containing multiple environment names and mounted credential paths.
	values := baseChartValuesForRenderTest()
	values["modules"] = map[string]interface{}{
		"importer": map[string]interface{}{
			"envPrefixes":     []string{"QUAY_API_TOKEN", "REGISTRY_PASSWORD"},
			"credentialPaths": []string{"/var/run/secrets/importer/token", "/var/run/secrets/importer/ca.pem"},
		},
	}

	// When Helm renders the chart with those values.
	rendered, err := renderChartForTest(t, values)
	require.NoError(t, err, "helm template should accept configured allowlists: %s", rendered)
	importerEnv := importerEnvironmentFromRenderedChart(t, rendered)

	// Then the importer receives comma-separated allowlists, and no other rendered resource gets them.
	assert.Equal(t, "QUAY_API_TOKEN,REGISTRY_PASSWORD", importerEnv["IMPORTER_ENV_PREFIXES"])
	assert.Equal(t, "/var/run/secrets/importer/token,/var/run/secrets/importer/ca.pem", importerEnv["IMPORTER_CREDENTIAL_PATHS"])
	assert.Equal(t, 1, strings.Count(rendered, "name: IMPORTER_ENV_PREFIXES"))
	assert.Equal(t, 1, strings.Count(rendered, "name: IMPORTER_CREDENTIAL_PATHS"))
}

// TestImporterCredentialAllowlistsAreUnsetByDefault verifies empty chart defaults preserve deny-by-default behavior.
func TestImporterCredentialAllowlistsAreUnsetByDefault(t *testing.T) {
	// Given the chart's default values with no importer allowlist overrides.
	rendered, err := renderChartForTest(t, baseChartValuesForRenderTest())
	require.NoError(t, err, "helm template should accept default allowlists: %s", rendered)
	importerEnv := importerEnvironmentFromRenderedChart(t, rendered)

	// When checking the importer and all rendered resources.
	// Then both credential source variables remain absent.
	assert.NotContains(t, importerEnv, "IMPORTER_ENV_PREFIXES")
	assert.NotContains(t, importerEnv, "IMPORTER_CREDENTIAL_PATHS")
	assert.NotContains(t, rendered, "IMPORTER_ENV_PREFIXES")
	assert.NotContains(t, rendered, "IMPORTER_CREDENTIAL_PATHS")
}

// TestImporterCredentialAllowlistSchemaRejectsInvalidTypes verifies both allowlists require arrays of strings.
func TestImporterCredentialAllowlistSchemaRejectsInvalidTypes(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value interface{}
	}{
		{name: "envPrefixes scalar", field: "envPrefixes", value: "QUAY_API_TOKEN"},
		{name: "envPrefixes non-string item", field: "envPrefixes", value: []int{42}},
		{name: "credentialPaths scalar", field: "credentialPaths", value: "/var/run/secrets/importer/token"},
		{name: "credentialPaths non-string item", field: "credentialPaths", value: []int{42}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// Given an importer allowlist with a value that violates its string-list contract.
			values := baseChartValuesForRenderTest()
			values["modules"] = map[string]interface{}{
				"importer": map[string]interface{}{
					testCase.field: testCase.value,
				},
			}

			// When Helm validates and renders the chart.
			output, err := renderChartForTest(t, values)

			// Then schema validation rejects the invalid allowlist value.
			require.Error(t, err, "invalid %s should be rejected", testCase.field)
			assert.Contains(t, output, testCase.field)
		})
	}
}

// baseChartValuesForRenderTest supplies required database and storage values for a valid chart render.
func baseChartValuesForRenderTest() map[string]interface{} {
	return map[string]interface{}{
		"appDomain": "test.example.com",
		"database": map[string]interface{}{
			"host":     "postgres.example.com",
			"name":     "trustify",
			"username": "trustify",
			"password": "test-only-placeholder",
		},
		"storage": map[string]interface{}{
			"type": "filesystem",
			"size": "1Gi",
		},
	}
}

// renderChartForTest renders the operator's embedded chart with supplied values for integration tests.
func renderChartForTest(t *testing.T, values map[string]interface{}) (string, error) {
	t.Helper()

	helmPath, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm command not available in test environment")
	}

	chartPath := filepath.Join("helm-charts", "redhat-trusted-profile-analyzer")
	args := []string{"template", "test-release", chartPath}
	valuesYAML, err := yaml.Marshal(values)
	require.NoError(t, err, "values should be valid YAML")

	valuesPath := filepath.Join(t.TempDir(), "values.yaml")
	err = os.WriteFile(valuesPath, valuesYAML, 0644)
	require.NoError(t, err, "values file should be writable")
	args = append(args, "-f", valuesPath)

	output, err := exec.Command(helmPath, args...).CombinedOutput()
	return string(output), err
}

// importerEnvironmentFromRenderedChart returns the environment on the importer container in rendered chart output.
func importerEnvironmentFromRenderedChart(t *testing.T, rendered string) map[string]string {
	t.Helper()

	var importerEnv map[string]string
	importerFound := false
	for _, doc := range strings.Split(rendered, "\n---\n") {
		if strings.TrimSpace(doc) == "" {
			continue
		}

		var deployment struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							Name string `yaml:"name"`
							Env  []struct {
								Name  string `yaml:"name"`
								Value string `yaml:"value"`
							} `yaml:"env"`
						} `yaml:"containers"`
					} `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(doc), &deployment), "rendered resource should be valid YAML")
		if deployment.Kind != "Deployment" || !strings.HasSuffix(deployment.Metadata.Name, "importer") {
			continue
		}

		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name != "service" {
				continue
			}
			importerFound = true
			importerEnv = make(map[string]string, len(container.Env))
			for _, variable := range container.Env {
				importerEnv[variable.Name] = variable.Value
			}
			break
		}
	}

	require.True(t, importerFound, "rendered chart should contain the importer service container")
	return importerEnv
}
