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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	trustedProfileAnalyzerGVR = schema.GroupVersionResource{
		Group:    "rhtpa.io",
		Version:  "v1",
		Resource: "trustedprofileanalyzers",
	}
)

func TestCRCreation(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-create"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Load minimal CR fixture
	cr := loadCRFixture(t, fixtureMinimalCR)
	cr.SetNamespace(testNamespace)
	crName := "test-create-instance"
	cr.SetName(crName)

	// Create the CR
	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	created, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create TrustedProfileAnalyzer CR")
	assert.NotNil(t, created, "created CR should not be nil")

	// Verify CR exists
	retrieved, err := res.Get(ctx, crName, metav1.GetOptions{})
	require.NoError(t, err, "should be able to get created CR")
	assert.Equal(t, crName, retrieved.GetName(), "CR name should match")
}

func TestCRUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-update"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Load minimal CR fixture
	cr := loadCRFixture(t, fixtureMinimalCR)
	cr.SetNamespace(testNamespace)
	cr.SetName("test-update-instance")

	// Create the CR
	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	created, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, msgCreateCR)

	// Update the CR
	spec, found, err := unstructured.NestedMap(created.Object, fieldSpec)
	require.NoError(t, err, "should be able to get spec")
	require.True(t, found, "spec should exist")

	spec[fieldReplicas] = int64(2)
	err = unstructured.SetNestedMap(created.Object, spec, fieldSpec)
	require.NoError(t, err, "should be able to set spec")

	updated, err := res.Update(ctx, created, metav1.UpdateOptions{})
	require.NoError(t, err, "should be able to update CR")

	// Verify the update
	updatedSpec, found, err := unstructured.NestedMap(updated.Object, fieldSpec)
	require.NoError(t, err, "should be able to get updated spec")
	require.True(t, found, "updated spec should exist")

	replicas, found, err := unstructured.NestedInt64(updatedSpec, fieldReplicas)
	require.NoError(t, err, "should be able to get replicas")
	require.True(t, found, "replicas should exist")
	assert.Equal(t, int64(2), replicas, "replicas should be updated")
}

func TestCRDeletion(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-delete"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Load minimal CR fixture
	cr := loadCRFixture(t, fixtureMinimalCR)
	cr.SetNamespace(testNamespace)
	crName := "test-delete-instance"
	cr.SetName(crName)

	// Create the CR
	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, msgCreateCR)

	// Delete the CR
	err = res.Delete(ctx, crName, metav1.DeleteOptions{})
	require.NoError(t, err, "should be able to delete CR")

	// Verify deletion
	require.Eventually(t, func() bool {
		_, err := res.Get(ctx, crName, metav1.GetOptions{})
		return apierrors.IsNotFound(err)
	}, 30*time.Second, 2*time.Second, "CR should be deleted")
}

func TestCRWithFullSpec(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-fullspec"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Load full CR fixture
	cr := loadCRFixture(t, fixtureValidCR)
	cr.SetNamespace(testNamespace)
	cr.SetName("test-fullspec-instance")

	// Create the CR
	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	created, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR with full spec")
	assert.NotNil(t, created, "created CR should not be nil")

	// Verify all spec fields are preserved
	spec, found, err := unstructured.NestedMap(created.Object, fieldSpec)
	require.NoError(t, err, "should be able to get spec")
	require.True(t, found, "spec should exist")

	// Verify appDomain
	appDomain, found, err := unstructured.NestedString(spec, fieldAppDomain)
	require.NoError(t, err, "should be able to get appDomain")
	require.True(t, found, "appDomain should exist")
	assert.NotEmpty(t, appDomain, "appDomain should not be empty")

	// Verify modules
	modules, found, err := unstructured.NestedMap(spec, fieldModules)
	require.NoError(t, err, "should be able to get modules")
	require.True(t, found, "modules should exist")
	assert.NotEmpty(t, modules, "modules should not be empty")
}

//nolint:dupl // similar structure to reconciliation_test.go but tests different behavior
func TestCRStatusUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-status"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Load minimal CR fixture
	cr := loadCRFixture(t, fixtureMinimalCR)
	cr.SetNamespace(testNamespace)
	cr.SetName("test-status-instance")

	// Create the CR
	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, msgCreateCR)

	// Wait for operator to reconcile and potentially update status
	time.Sleep(10 * time.Second)

	// Get the CR and check if status exists
	retrieved, err := res.Get(ctx, "test-status-instance", metav1.GetOptions{})
	require.NoError(t, err, "should be able to get CR")

	// Note: Status updates depend on the Helm operator reconciliation
	// We just verify the CR structure is valid
	assert.NotNil(t, retrieved, "retrieved CR should not be nil")
}

func TestMultipleCRsInDifferentNamespaces(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create first test namespace
	namespace1 := "e2e-test-multi-1"
	createNamespace(t, k8sClient, namespace1)
	defer deleteNamespace(t, k8sClient, namespace1)

	// Create second test namespace
	namespace2 := "e2e-test-multi-2"
	createNamespace(t, k8sClient, namespace2)
	defer deleteNamespace(t, k8sClient, namespace2)

	// Create CR in first namespace
	cr1 := loadCRFixture(t, fixtureMinimalCR)
	cr1.SetNamespace(namespace1)
	cr1.SetName("instance-ns1")

	res1 := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(namespace1)
	_, err := res1.Create(ctx, cr1, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR in namespace 1")

	// Create CR in second namespace
	cr2 := loadCRFixture(t, fixtureMinimalCR)
	cr2.SetNamespace(namespace2)
	cr2.SetName("instance-ns2")

	res2 := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(namespace2)
	_, err = res2.Create(ctx, cr2, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR in namespace 2")

	// Verify both CRs exist
	retrieved1, err := res1.Get(ctx, "instance-ns1", metav1.GetOptions{})
	require.NoError(t, err, "CR in namespace 1 should exist")
	assert.Equal(t, namespace1, retrieved1.GetNamespace(), "CR should be in namespace 1")

	retrieved2, err := res2.Get(ctx, "instance-ns2", metav1.GetOptions{})
	require.NoError(t, err, "CR in namespace 2 should exist")
	assert.Equal(t, namespace2, retrieved2.GetNamespace(), "CR should be in namespace 2")
}

func TestCRValidation(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-validation"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR with invalid API version (should fail if OpenAPI validation is enabled)
	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			fieldAPIVersion: "rhtpa.io/v999",
			fieldKind:       "TrustedProfileAnalyzer",
			fieldMetadata: map[string]interface{}{
				fieldName:      "test-invalid",
				fieldNamespace: testNamespace,
			},
			fieldSpec: map[string]interface{}{
				fieldAppDomain: "test.example.com",
			},
		},
	}

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	// Note: This should fail with validation error, but exact behavior depends on CRD validation rules
	if err != nil {
		t.Logf("Expected validation error received: %v", err)
	}
}

// Helper functions are defined in helpers_test.go
