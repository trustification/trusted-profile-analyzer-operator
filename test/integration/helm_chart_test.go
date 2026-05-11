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
	"path/filepath"
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
