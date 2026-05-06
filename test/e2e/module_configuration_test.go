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

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	fixtureFullCR   = "full_cr.yaml"
	msgRetrievedNil = "retrieved CR should not be nil"
	msgGetAppDomain = "should be able to get appDomain"
	msgGetModules   = "should be able to get modules"
)

func TestServerModuleConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-server-module"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR with server-only configuration
	cr := loadCRFixture(t, "server_only_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("server-module-test")

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create server-only CR")

	// Verify CR was created
	retrieved, err := res.Get(ctx, "server-module-test", metav1.GetOptions{})
	require.NoError(t, err, msgGetCR)
	assert.NotNil(t, retrieved, msgRetrievedNil)
}

func TestImporterModuleConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-importer-module"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR with importer-only configuration
	cr := loadCRFixture(t, "importer_only_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("importer-module-test")

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create importer-only CR")

	// Verify CR was created
	retrieved, err := res.Get(ctx, "importer-module-test", metav1.GetOptions{})
	require.NoError(t, err, msgGetCR)
	assert.NotNil(t, retrieved, msgRetrievedNil)
}

func TestFullConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-full-config"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR with full configuration
	cr := loadCRFixture(t, fixtureFullCR)
	cr.SetNamespace(testNamespace)
	cr.SetName("full-config-test")

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create full configuration CR")

	// Verify CR was created with all fields
	retrieved, err := res.Get(ctx, "full-config-test", metav1.GetOptions{})
	require.NoError(t, err, msgGetCR)

	// Verify spec fields
	spec, found, err := unstructured.NestedMap(retrieved.Object, fieldSpec)
	require.NoError(t, err, msgGetSpec)
	require.True(t, found, msgSpecExist)

	// Check appDomain
	appDomain, found, err := unstructured.NestedString(spec, fieldAppDomain)
	require.NoError(t, err, msgGetAppDomain)
	require.True(t, found, "appDomain should exist")
	assert.Equal(t, "full.example.com", appDomain, "appDomain should match")

	// Check modules
	modules, found, err := unstructured.NestedMap(spec, fieldModules)
	require.NoError(t, err, msgGetModules)
	require.True(t, found, "modules should exist")
	assert.NotEmpty(t, modules, "modules should not be empty")
}

func TestModuleReplicasConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	testCases := []struct {
		name             string
		serverReplicas   int
		importerReplicas int
	}{
		{
			name:             "single-replica",
			serverReplicas:   1,
			importerReplicas: 1,
		},
		{
			name:             "multi-replica-server",
			serverReplicas:   3,
			importerReplicas: 1,
		},
		{
			name:             "multi-replica-importer",
			serverReplicas:   1,
			importerReplicas: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testNamespace := "e2e-test-replicas-" + tc.name
			createNamespace(t, k8sClient, testNamespace)
			defer deleteNamespace(t, k8sClient, testNamespace)

			cr := loadCRFixture(t, fixtureValidCR)
			cr.SetNamespace(testNamespace)
			cr.SetName("replicas-test")

			// Set replica counts
			spec, _, _ := unstructured.NestedMap(cr.Object, fieldSpec)
			modules := map[string]interface{}{
				fieldServer: map[string]interface{}{
					fieldEnabled:  true,
					fieldReplicas: tc.serverReplicas,
				},
				fieldImporter: map[string]interface{}{
					fieldEnabled:  true,
					fieldReplicas: tc.importerReplicas,
				},
			}
			spec[fieldModules] = modules
			_ = unstructured.SetNestedMap(cr.Object, spec, fieldSpec)

			res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
			_, err := res.Create(ctx, cr, metav1.CreateOptions{})
			require.NoError(t, err, "should be able to create CR with replica config")

			// Verify CR was created
			retrieved, err := res.Get(ctx, "replicas-test", metav1.GetOptions{})
			require.NoError(t, err, msgGetCR)
			assert.NotNil(t, retrieved, msgRetrievedNil)
		})
	}
}

func TestOIDCConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-oidc"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	cr := loadCRFixture(t, fixtureFullCR)
	cr.SetNamespace(testNamespace)
	cr.SetName("oidc-test")

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR with OIDC configuration")

	// Verify OIDC configuration
	retrieved, err := res.Get(ctx, "oidc-test", metav1.GetOptions{})
	require.NoError(t, err, msgGetCR)

	oidc, found, err := unstructured.NestedMap(retrieved.Object, fieldSpec, "oidc")
	require.NoError(t, err, "should be able to get OIDC config")
	if found {
		assert.NotEmpty(t, oidc, "OIDC configuration should not be empty")
		t.Logf("OIDC configuration: %+v", oidc)
	}
}

func TestDatabaseConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-database"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	cr := loadCRFixture(t, fixtureFullCR)
	cr.SetNamespace(testNamespace)
	cr.SetName("database-test")

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR with database configuration")

	// Verify database configuration
	retrieved, err := res.Get(ctx, "database-test", metav1.GetOptions{})
	require.NoError(t, err, msgGetCR)

	database, found, err := unstructured.NestedMap(retrieved.Object, fieldSpec, "database")
	require.NoError(t, err, "should be able to get database config")
	if found {
		assert.NotEmpty(t, database, "database configuration should not be empty")

		// Check for expected database fields
		if host, found, _ := unstructured.NestedString(database, "host"); found {
			assert.NotEmpty(t, host, "database host should not be empty")
		}
	}
}

func TestStorageConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-storage"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	cr := loadCRFixture(t, fixtureFullCR)
	cr.SetNamespace(testNamespace)
	cr.SetName("storage-test")

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR with storage configuration")

	// Verify storage configuration
	retrieved, err := res.Get(ctx, "storage-test", metav1.GetOptions{})
	require.NoError(t, err, msgGetCR)

	storage, found, err := unstructured.NestedMap(retrieved.Object, fieldSpec, "storage")
	require.NoError(t, err, "should be able to get storage config")
	if found {
		assert.NotEmpty(t, storage, "storage configuration should not be empty")
		t.Logf("Storage configuration present with %d fields", len(storage))
	}
}

func TestMetricsAndTracingConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-metrics-tracing"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	cr := loadCRFixture(t, fixtureFullCR)
	cr.SetNamespace(testNamespace)
	cr.SetName("metrics-tracing-test")

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR with metrics/tracing config")

	// Verify metrics and tracing configuration
	retrieved, err := res.Get(ctx, "metrics-tracing-test", metav1.GetOptions{})
	require.NoError(t, err, msgGetCR)

	spec, found, err := unstructured.NestedMap(retrieved.Object, fieldSpec)
	require.NoError(t, err, msgGetSpec)
	require.True(t, found, msgSpecExist)

	if metrics, found, _ := unstructured.NestedMap(spec, "metrics"); found {
		t.Logf("Metrics configuration present")
		assert.NotEmpty(t, metrics, "metrics configuration should not be empty")
	}

	if tracing, found, _ := unstructured.NestedMap(spec, "tracing"); found {
		t.Logf("Tracing configuration present")
		assert.NotEmpty(t, tracing, "tracing configuration should not be empty")
	}
}
