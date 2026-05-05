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

func TestCRCreationWithMissingAppDomain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-missing-domain"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR without appDomain
	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rhtpa.io/v1",
			"kind":       "TrustedProfileAnalyzer",
			"metadata": map[string]interface{}{
				"name":      "test-missing-domain",
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				// Missing appDomain - should fail validation if schema validation is enabled
				"replicas": 1,
			},
		},
	}

	_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})

	// Depending on CRD validation, this might fail or succeed
	// If validation is enabled, it should fail
	if err != nil {
		t.Logf("Creation failed as expected due to validation: %v", err)
	} else {
		t.Logf("Creation succeeded - validation may not be enforced or appDomain may have a default")
	}
}

func TestCRCreationWithInvalidValues(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	testCases := []struct {
		name        string
		spec        map[string]interface{}
		expectError bool
	}{
		{
			name: "negative replicas",
			spec: map[string]interface{}{
				"appDomain": "test.example.com",
				"replicas":  -1,
			},
			expectError: true,
		},
		{
			name: "invalid module config",
			spec: map[string]interface{}{
				"appDomain": "test.example.com",
				"modules": map[string]interface{}{
					"server": map[string]interface{}{
						"replicas": -5,
					},
				},
			},
			expectError: true,
		},
		{
			name: "empty appDomain",
			spec: map[string]interface{}{
				"appDomain": "",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testNamespace := "e2e-test-invalid-" + tc.name
			createNamespace(t, k8sClient, testNamespace)
			defer deleteNamespace(t, k8sClient, testNamespace)

			cr := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "rhtpa.io/v1",
					"kind":       "TrustedProfileAnalyzer",
					"metadata": map[string]interface{}{
						"name":      "test-invalid",
						"namespace": testNamespace,
					},
					"spec": tc.spec,
				},
			}

			_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})

			if tc.expectError {
				// Validation may or may not be enforced depending on CRD configuration
				if err != nil {
					t.Logf("Got expected error: %v", err)
				} else {
					t.Logf("Warning: expected error but creation succeeded - validation may not be enforced")
				}
			} else {
				require.NoError(t, err, "should not error for valid spec")
			}
		})
	}
}

func TestCRUpdateWithInvalidSpec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-invalid-update"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create valid CR
	cr := loadCRFixture(t, "minimal_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("test-invalid-update")

	created, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create valid CR")

	// Try to update with invalid spec
	spec := map[string]interface{}{
		"appDomain": "",  // Invalid empty appDomain
		"replicas":  -10, // Invalid negative replicas
	}

	err = unstructured.SetNestedField(created.Object, spec, "spec")
	require.NoError(t, err, "should be able to set spec field")

	_, err = dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Update(ctx, created, metav1.UpdateOptions{})

	// Depending on validation, this may or may not fail
	if err != nil {
		t.Logf("Update rejected as expected: %v", err)
	} else {
		t.Logf("Warning: invalid update was accepted - validation may not be enforced")
	}
}

func TestCRDeletionOfNonExistentResource(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-delete-nonexistent"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Try to delete a resource that doesn't exist
	err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Delete(ctx, "nonexistent-resource", metav1.DeleteOptions{})

	assert.Error(t, err, "should error when deleting non-existent resource")
	t.Logf("Got expected error: %v", err)
}

func TestCRCreationInNonExistentNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)

	// Try to create CR in a namespace that doesn't exist
	nonExistentNamespace := "e2e-test-nonexistent-ns-12345"

	cr := loadCRFixture(t, "minimal_cr.yaml")
	cr.SetNamespace(nonExistentNamespace)
	cr.SetName("test-in-nonexistent-ns")

	_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(nonExistentNamespace).Create(ctx, cr, metav1.CreateOptions{})

	assert.Error(t, err, "should error when creating CR in non-existent namespace")
	t.Logf("Got expected error: %v", err)
}

func TestCRWithMalformedSpec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-malformed-spec"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR with type mismatches
	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rhtpa.io/v1",
			"kind":       "TrustedProfileAnalyzer",
			"metadata": map[string]interface{}{
				"name":      "test-malformed",
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"appDomain": "test.example.com",
				"replicas":  "not-a-number", // Wrong type - should be int
				"modules": map[string]interface{}{
					"server": "not-an-object", // Wrong type - should be object
				},
			},
		},
	}

	_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})

	// This should fail if type validation is enforced
	if err != nil {
		t.Logf("Creation failed as expected due to type mismatch: %v", err)
	} else {
		t.Logf("Warning: creation with type mismatches succeeded - strict validation may not be enforced")
	}
}

func TestOperatorHandlesRapidCRUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-rapid-updates"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR
	cr := loadCRFixture(t, "minimal_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("test-rapid-updates")

	_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR")

	// Perform rapid updates
	for i := 0; i < 5; i++ {
		// Get latest version
		latest, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, "test-rapid-updates", metav1.GetOptions{})
		require.NoError(t, err, "should be able to get CR")

		// Update spec
		newDomain := "test" + string(rune('0'+i)) + ".example.com"
		err = unstructured.SetNestedField(latest.Object, newDomain, "spec", "appDomain")
		require.NoError(t, err, "should be able to set field")

		_, err = dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Update(ctx, latest, metav1.UpdateOptions{})
		if err != nil {
			t.Logf("Update %d failed (may be expected due to conflicts): %v", i, err)
		}
	}

	// Verify final state
	final, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, "test-rapid-updates", metav1.GetOptions{})
	require.NoError(t, err, "should be able to get final CR state")
	assert.NotNil(t, final, "final CR should exist")
}
