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

package e2e

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	testAppDomain          = "test.example.com"
	kindDeployment         = "kind: Deployment"
	kindDeploymentResource = "Deployment"
)

func TestHelmChartRenderWithMinimalValues(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)

	// Create minimal values file
	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
	}

	// Render chart with minimal values
	rendered := renderHelmChart(t, chartPath, values)
	assert.NotEmpty(t, rendered, "chart should render with minimal values")

	// Verify rendered output contains expected resources
	assert.Contains(t, rendered, kindDeployment, "should contain Deployment")
	assert.Contains(t, rendered, "kind: Service", "should contain Service")
}

func TestHelmChartRenderWithFullValues(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)

	// Load full values from fixture
	cr := loadCRFixture(t, fixtureValidCR)
	spec, found, err := unstructured.NestedMap(cr.Object, fieldSpec)
	require.NoError(t, err, "should be able to get spec")
	require.True(t, found, "spec should exist")

	// Render chart with full values
	rendered := renderHelmChart(t, chartPath, spec)
	assert.NotEmpty(t, rendered, "chart should render with full values")

	// Verify modules are rendered
	assert.Contains(t, rendered, kindDeployment, "should contain Deployments")
}

func TestHelmChartRenderServerModule(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)

	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
		fieldModules: map[string]interface{}{
			fieldServer: map[string]interface{}{
				fieldEnabled:  true,
				fieldReplicas: 2,
			},
			fieldImporter: map[string]interface{}{
				fieldEnabled: false,
			},
		},
	}

	rendered := renderHelmChart(t, chartPath, values)
	assert.NotEmpty(t, rendered, "chart should render with server module enabled")

	docs := splitYAMLDocs(rendered)
	serverDeploymentFound := false

	for _, doc := range docs {
		var obj unstructured.Unstructured
		if err := yaml.Unmarshal([]byte(doc), &obj.Object); err != nil {
			continue
		}
		if obj.GetKind() != kindDeploymentResource || obj.GetName() != fieldServer {
			continue
		}
		serverDeploymentFound = true

		replicas, found, err := unstructured.NestedInt64(obj.Object, fieldSpec, fieldReplicas)
		require.NoError(t, err, "should be able to read replicas")
		require.True(t, found, "replicas field should exist")
		assert.Equal(t, int64(2), replicas, "server deployment should have 2 replicas")
	}

	assert.True(t, serverDeploymentFound, "server deployment should be rendered")
}

func TestHelmChartRenderImporterModule(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)

	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
		fieldModules: map[string]interface{}{
			fieldServer: map[string]interface{}{
				fieldEnabled: false,
			},
			fieldImporter: map[string]interface{}{
				fieldEnabled:  true,
				fieldReplicas: 1,
			},
		},
	}

	rendered := renderHelmChart(t, chartPath, values)
	assert.NotEmpty(t, rendered, "chart should render with importer module enabled")

	// Verify importer deployment exists
	docs := splitYAMLDocs(rendered)
	importerDeploymentFound := false

	for _, doc := range docs {
		if strings.Contains(doc, kindDeployment) && strings.Contains(doc, "importer") {
			importerDeploymentFound = true
			break
		}
	}

	assert.True(t, importerDeploymentFound, "importer deployment should be rendered")
}

func TestHelmChartRenderDatabaseJobs(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)

	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
		fieldModules: map[string]interface{}{
			"createDatabase": map[string]interface{}{
				fieldEnabled: true,
			},
			"migrateDatabase": map[string]interface{}{
				fieldEnabled: true,
			},
		},
	}

	rendered := renderHelmChart(t, chartPath, values)
	assert.NotEmpty(t, rendered, "chart should render with database jobs enabled")

	// Verify jobs are rendered
	docs := splitYAMLDocs(rendered)
	jobCount := 0

	for _, doc := range docs {
		if strings.Contains(doc, "kind: Job") {
			jobCount++
		}
	}

	assert.Greater(t, jobCount, 0, "database jobs should be rendered")
}

func TestHelmChartRenderWithOIDCConfig(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)

	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
		"oidc": map[string]interface{}{
			"clients": map[string]interface{}{
				"frontend": map[string]interface{}{
					"clientId": "test-frontend",
				},
				"cli": map[string]interface{}{
					"clientSecret": "test-secret",
				},
			},
		},
	}

	rendered := renderHelmChart(t, chartPath, values)
	assert.NotEmpty(t, rendered, "chart should render with OIDC config")

	// Verify OIDC configuration is included in rendered resources
	// This would typically be in ConfigMaps or environment variables
	assert.Contains(t, rendered, "test-frontend", "OIDC client ID should be in rendered output")
}

func TestHelmChartRenderWithResourceLimits(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)

	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
		fieldModules: map[string]interface{}{
			fieldServer: map[string]interface{}{
				fieldEnabled: true,
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    "500m",
						"memory": "512Mi",
					},
					"limits": map[string]interface{}{
						"cpu":    "1000m",
						"memory": "1Gi",
					},
				},
			},
		},
	}

	rendered := renderHelmChart(t, chartPath, values)
	assert.NotEmpty(t, rendered, "chart should render with resource limits")

	// Verify resource limits are in rendered output
	assert.Contains(t, rendered, "500m", "CPU request should be in rendered output")
	assert.Contains(t, rendered, "512Mi", "memory request should be in rendered output")
}

func TestHelmChartRenderWithIngress(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)

	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
		fieldIngress: map[string]interface{}{
			fieldEnabled: true,
		},
		fieldModules: map[string]interface{}{
			fieldServer: map[string]interface{}{
				fieldEnabled: true,
			},
		},
	}

	rendered := renderHelmChart(t, chartPath, values)
	assert.NotEmpty(t, rendered, "chart should render with ingress")

	// Verify Ingress resource is rendered
	docs := splitYAMLDocs(rendered)
	ingressFound := false

	for _, doc := range docs {
		if strings.Contains(doc, "kind: Ingress") || strings.Contains(doc, "kind: Route") {
			ingressFound = true
			assert.Contains(t, doc, testAppDomain, "ingress should use appDomain")
			break
		}
	}

	assert.True(t, ingressFound, "Ingress should be rendered when server module and ingress are enabled")
}

func TestHelmChartRenderInCRContext(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-helm-render"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR with specific configuration
	cr := loadCRFixture(t, fixtureValidCR)
	cr.SetNamespace(testNamespace)
	cr.SetName("test-helm-render-instance")

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, msgCreateCR)

	// Wait for Helm to render and create resources
	require.Eventually(t, func() bool {
		var getErr error
		var retrieved *unstructured.Unstructured
		retrieved, getErr = res.Get(ctx, cr.GetName(), metav1.GetOptions{})
		if getErr != nil {
			return false
		}
		_, found, _ := unstructured.NestedMap(retrieved.Object, "status")
		return found
	}, 45*time.Second, 1*time.Second, "CR should have status after reconciliation")

	// Check for Helm release secrets/configmaps
	configMaps, err := k8sClient.CoreV1().ConfigMaps(testNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "should be able to list ConfigMaps")

	helmReleaseFound := false
	for _, cm := range configMaps.Items {
		if cm.Labels["owner"] == fieldHelm {
			helmReleaseFound = true
			t.Logf("Found Helm release ConfigMap: %s", cm.Name)
			break
		}
	}

	// Helm release metadata might be stored as secrets in some versions
	if !helmReleaseFound {
		secrets, err := k8sClient.CoreV1().Secrets(testNamespace).List(ctx, metav1.ListOptions{})
		require.NoError(t, err, "should be able to list Secrets")

		for _, secret := range secrets.Items {
			if secret.Type == "helm.sh/release.v1" || secret.Labels["owner"] == fieldHelm {
				helmReleaseFound = true
				t.Logf("Found Helm release Secret: %s", secret.Name)
				break
			}
		}
	}

	assert.True(t, helmReleaseFound, "Helm release metadata should exist")
}

func TestHelmChartLint(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)

	cmd := exec.Command("helm", "lint", chartPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := err.Error() + " " + stderr.String()
		if strings.Contains(errMsg, "executable file not found") ||
			strings.Contains(errMsg, "helm: not found") {
			t.Skip("helm command not available in test environment")
		}
		t.Logf("helm lint stderr: %s", stderr.String())
		assert.NoError(t, err, "helm lint should pass")
		return
	}

	assert.Contains(t, stdout.String(), "1 chart(s) linted", "chart should be linted successfully")
}

// Helper functions

func getChartPath(t *testing.T) string {
	t.Helper()

	// Get path to Helm chart relative to test directory
	chartPath := filepath.Join("..", "..", "helm-charts", "redhat-trusted-profile-analyzer")

	// Verify chart exists
	chartYaml := filepath.Join(chartPath, "Chart.yaml")
	_, err := os.Stat(chartYaml)
	require.NoError(t, err, "Chart.yaml should exist at %s", chartPath)

	return chartPath
}

func renderHelmChart(t *testing.T, chartPath string, values map[string]interface{}) string {
	t.Helper()

	// Create temporary values file
	tmpDir := t.TempDir()
	valuesFile := filepath.Join(tmpDir, "values.yaml")

	valuesYAML, err := yaml.Marshal(values)
	require.NoError(t, err, "should be able to marshal values to YAML")

	err = os.WriteFile(valuesFile, valuesYAML, 0644)
	require.NoError(t, err, "should be able to write values file")

	// Run helm template
	cmd := exec.Command("helm", "template", "test-release", chartPath, "-f", valuesFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		t.Logf("helm template stderr: %s", stderr.String())
		// Helm might not be available in test environment
		stderrStr := stderr.String()
		if strings.Contains(stderrStr, "helm: not found") ||
			strings.Contains(stderrStr, "executable file not found") {
			t.Skip("helm command not available in test environment")
		}
		require.NoError(t, err, "helm template should succeed")
	}

	return stdout.String()
}

func splitYAMLDocs(yamlContent string) []string {
	separator := "\n---\n"
	docs := strings.Split(yamlContent, separator)

	// Filter out empty documents
	var result []string
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc != "" && doc != "---" {
			result = append(result, doc)
		}
	}

	return result
}
