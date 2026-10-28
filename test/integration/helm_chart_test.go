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
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	chartImporterModuleName     = "importer"
	chartServerModuleName       = "server"
	chartEnvPrefixesField       = "envPrefixes"
	chartCredentialPathsField   = "credentialPaths"
	chartRecommendPatternsField = "recommendPatterns"
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

// TestChartHasNoMergeConflictMarkers walks the entire Helm chart and fails if
// any file contains an unresolved git merge conflict marker. A committed
// conflict in values.schema.json once broke every `helm template` render, so
// this guards the whole tree cheaply rather than relying on rendering coverage.
func TestChartHasNoMergeConflictMarkers(t *testing.T) {
	chartRoot := filepath.Join("helm-charts", "redhat-trusted-profile-analyzer")

	// Line prefixes that unambiguously indicate a conflict marker.
	markers := []string{"<<<<<<< ", ">>>>>>> "}

	err := filepath.WalkDir(chartRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr, "should be able to read %s", path)

		for lineNum, line := range strings.Split(string(data), "\n") {
			for _, marker := range markers {
				if strings.HasPrefix(line, marker) {
					t.Errorf("%s:%d contains a merge conflict marker: %q",
						path, lineNum+1, line)
				}
			}
		}
		return nil
	})
	require.NoError(t, err, "walking the chart directory should succeed")
}

func TestValuesSchemaContainsCCOFields(t *testing.T) {
	schemaPath := filepath.Join("helm-charts", "redhat-trusted-profile-analyzer", "values.schema.json")

	data, err := os.ReadFile(schemaPath)
	require.NoError(t, err, "values.schema.json should be readable")

	var schema map[string]interface{}
	err = yaml.Unmarshal(data, &schema)
	require.NoError(t, err, "values.schema.json should be valid JSON")

	properties, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok, "schema should have properties")

	assert.Contains(t, properties, "cloudProvider", "schema should define cloudProvider")
	assert.Contains(t, properties, "ccoMode", "schema should define ccoMode")
	assert.Contains(t, properties, "cloudCredentials", "schema should define cloudCredentials")

	cloudProvider, ok := properties["cloudProvider"].(map[string]interface{})
	require.True(t, ok, "cloudProvider should be an object")
	assert.Equal(t, "string", cloudProvider["type"], "cloudProvider should be a string type")

	cpEnum, ok := cloudProvider["enum"].([]interface{})
	require.True(t, ok, "cloudProvider should have enum values")
	assert.Contains(t, cpEnum, "aws", "cloudProvider enum should include aws")
	assert.Contains(t, cpEnum, "gcp", "cloudProvider enum should include gcp")

	ccoMode, ok := properties["ccoMode"].(map[string]interface{})
	require.True(t, ok, "ccoMode should be an object")
	assert.Equal(t, "string", ccoMode["type"], "ccoMode should be a string type")

	modeEnum, ok := ccoMode["enum"].([]interface{})
	require.True(t, ok, "ccoMode should have enum values")
	assert.Contains(t, modeEnum, "default", "ccoMode enum should include default")
	assert.Contains(t, modeEnum, "mint", "ccoMode enum should include mint")
	assert.Contains(t, modeEnum, "passthrough", "ccoMode enum should include passthrough")
	assert.Contains(t, modeEnum, "manual", "ccoMode enum should include manual")

	cloudCreds, ok := properties["cloudCredentials"].(map[string]interface{})
	require.True(t, ok, "cloudCredentials should be an object")
	ccProps, ok := cloudCreds["properties"].(map[string]interface{})
	require.True(t, ok, "cloudCredentials should have properties")
	assert.Contains(t, ccProps, "aws", "cloudCredentials should have aws")
	assert.Contains(t, ccProps, "gcp", "cloudCredentials should have gcp")

	awsCreds, ok := ccProps["aws"].(map[string]interface{})
	require.True(t, ok, "aws credentials should be an object")
	awsProps, ok := awsCreds["properties"].(map[string]interface{})
	require.True(t, ok, "aws should have properties")
	assert.Contains(t, awsProps, "stsIAMRoleARN", "aws should have stsIAMRoleARN property")
}

// TestImporterCredentialAllowlistsRenderOnImporterDeployment verifies configured allowlists reach only the importer.
func TestImporterCredentialAllowlistsRenderOnImporterDeployment(t *testing.T) {
	// Given importer allowlists containing multiple environment names and mounted credential paths.
	values := baseChartValuesForRenderTest()
	values["modules"] = map[string]interface{}{
		chartImporterModuleName: map[string]interface{}{
			chartEnvPrefixesField:     []string{"QUAY_API_TOKEN", "REGISTRY_PASSWORD"},
			chartCredentialPathsField: []string{"/var/run/secrets/importer/token", "/var/run/secrets/importer/ca.pem"},
		},
	}

	// When Helm renders the chart with those values.
	rendered, err := renderChartForTest(t, values)
	require.NoError(t, err, "helm template should accept configured allowlists: %s", rendered)
	importerEnv := importerEnvironmentFromRenderedChart(t, rendered)

	// Then the importer receives comma-separated allowlists, and no other rendered resource gets them.
	assert.Equal(t, "QUAY_API_TOKEN,REGISTRY_PASSWORD", importerEnv["IMPORTER_ENV_PREFIXES"])
	assert.Equal(t,
		"/var/run/secrets/importer/token,/var/run/secrets/importer/ca.pem",
		importerEnv["IMPORTER_CREDENTIAL_PATHS"],
	)
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
		{name: "envPrefixes scalar", field: chartEnvPrefixesField, value: "QUAY_API_TOKEN"},
		{name: "envPrefixes non-string item", field: chartEnvPrefixesField, value: []int{42}},
		{name: "credentialPaths scalar", field: chartCredentialPathsField, value: "/var/run/secrets/importer/token"},
		{name: "credentialPaths non-string item", field: chartCredentialPathsField, value: []int{42}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// Given an importer allowlist with a value that violates its string-list contract.
			values := baseChartValuesForRenderTest()
			values["modules"] = map[string]interface{}{
				chartImporterModuleName: map[string]interface{}{
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

// TestCommaDelimitedChartListItemsRejectCommas verifies comma-bearing entries cannot be misparsed.
func TestCommaDelimitedChartListItemsRejectCommas(t *testing.T) {
	tests := []struct {
		name   string
		module string
		field  string
		value  string
	}{
		{
			name:   "environment prefix",
			module: chartImporterModuleName,
			field:  chartEnvPrefixesField,
			value:  "QUAY_API_TOKEN,REGISTRY_",
		},
		{
			name:   "credential path",
			module: chartImporterModuleName,
			field:  chartCredentialPathsField,
			value:  "/var/run/secrets/quay,token",
		},
		{
			name:   "recommendation pattern",
			module: chartServerModuleName,
			field:  chartRecommendPatternsField,
			value:  "^pkg,foo$",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// Given a comma-containing item in a comma-delimited chart list.
			values := baseChartValuesForRenderTest()
			values["modules"] = map[string]interface{}{
				testCase.module: map[string]interface{}{
					testCase.field: []string{testCase.value},
				},
			}

			// When Helm validates and renders the chart.
			output, err := renderChartForTest(t, values)

			// Then schema validation rejects the ambiguous list item.
			require.Error(t, err, "comma-containing %s should be rejected", testCase.field)
			assert.Contains(t, output, testCase.field)
		})
	}
}

// TestCommaFreeServerRecommendationPatternsStillRender verifies valid patterns remain configurable.
func TestCommaFreeServerRecommendationPatternsStillRender(t *testing.T) {
	// Given comma-free recommendation patterns.
	values := baseChartValuesForRenderTest()
	values["modules"] = map[string]interface{}{
		chartServerModuleName: map[string]interface{}{
			chartRecommendPatternsField: []string{"^foo$", "^bar$"},
		},
	}

	// When Helm renders the chart.
	rendered, err := renderChartForTest(t, values)
	require.NoError(t, err, "helm template should accept comma-free recommendation patterns: %s", rendered)

	// Then the server receives the list in its existing comma-separated format.
	assert.Contains(t, rendered, "name: TRUSTD_RECOMMEND_PATTERNS")
	assert.Contains(t, rendered, `value: "^foo$,^bar$"`)
}

// TestChartListSchemaRejectsEmptyItems verifies empty strings cannot broaden credential or recommendation access.
func TestChartListSchemaRejectsEmptyItems(t *testing.T) {
	tests := []struct {
		name   string
		module string
		field  string
	}{
		{name: "environment prefix", module: chartImporterModuleName, field: chartEnvPrefixesField},
		{name: "credential path", module: chartImporterModuleName, field: chartCredentialPathsField},
		{name: "recommendation pattern", module: chartServerModuleName, field: chartRecommendPatternsField},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// Given a chart list containing an empty string item.
			values := baseChartValuesForRenderTest()
			values["modules"] = map[string]interface{}{
				testCase.module: map[string]interface{}{
					testCase.field: []string{""},
				},
			}

			// When Helm validates and renders the chart.
			output, err := renderChartForTest(t, values)

			// Then schema validation rejects the empty item.
			require.Error(t, err, "empty %s item should be rejected", testCase.field)
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
	args := make([]string, 0, 5)
	args = append(args, "template", "test-release", chartPath)
	valuesYAML, err := yaml.Marshal(values)
	require.NoError(t, err, "values should be valid YAML")

	valuesPath := filepath.Join(t.TempDir(), "values.yaml")
	err = os.WriteFile(valuesPath, valuesYAML, 0644)
	require.NoError(t, err, "values file should be writable")
	args = append(args, "-f", valuesPath)

	command := exec.Command(helmPath, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if err != nil {
		return stdout.String() + stderr.String(), err
	}
	return stdout.String(), nil
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
