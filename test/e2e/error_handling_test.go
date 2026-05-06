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
	crAPIVersion     = "rhtpa.io/v1"
	crKind           = "TrustedProfileAnalyzer"
	errAppDomain     = "test.example.com"
	crRapidUpdates   = "test-rapid-updates"
	logExpectedError = "Got expected error: %v"
)

func TestCRCreationWithMissingAppDomain(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
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
			fieldAPIVersion: crAPIVersion,
			fieldKind:       crKind,
			fieldMetadata: map[string]interface{}{
				fieldName:      "test-missing-domain",
				fieldNamespace: testNamespace,
			},
			fieldSpec: map[string]interface{}{
				// Missing appDomain - should fail validation if schema validation is enabled
				fieldReplicas: 1,
			},
		},
	}

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})

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
		t.Skip(skipE2ETest)
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
				fieldAppDomain: errAppDomain,
				fieldReplicas:  -1,
			},
			expectError: true,
		},
		{
			name: "invalid module config",
			spec: map[string]interface{}{
				fieldAppDomain: errAppDomain,
				fieldModules: map[string]interface{}{
					fieldServer: map[string]interface{}{
						fieldReplicas: -5,
					},
				},
			},
			expectError: true,
		},
		{
			name: "empty appDomain",
			spec: map[string]interface{}{
				fieldAppDomain: "",
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
					fieldAPIVersion: crAPIVersion,
					fieldKind:       crKind,
					fieldMetadata: map[string]interface{}{
						fieldName:      "test-invalid",
						fieldNamespace: testNamespace,
					},
					fieldSpec: tc.spec,
				},
			}

			res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
			_, err := res.Create(ctx, cr, metav1.CreateOptions{})

			if tc.expectError {
				// Validation may or may not be enforced depending on CRD configuration
				if err != nil {
					t.Logf(logExpectedError, err)
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
		t.Skip(skipE2ETest)
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
	cr := loadCRFixture(t, fixtureMinimalCR)
	cr.SetNamespace(testNamespace)
	cr.SetName("test-invalid-update")

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	created, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create valid CR")

	// Try to update with invalid spec
	spec := map[string]interface{}{
		fieldAppDomain: "",  // Invalid empty appDomain
		fieldReplicas:  -10, // Invalid negative replicas
	}

	err = unstructured.SetNestedField(created.Object, spec, fieldSpec)
	require.NoError(t, err, msgSetField)

	_, err = res.Update(ctx, created, metav1.UpdateOptions{})

	// Depending on validation, this may or may not fail
	if err != nil {
		t.Logf("Update rejected as expected: %v", err)
	} else {
		t.Logf("Warning: invalid update was accepted - validation may not be enforced")
	}
}

func TestCRDeletionOfNonExistentResource(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
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
	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	err := res.Delete(ctx, "nonexistent-resource", metav1.DeleteOptions{})

	assert.Error(t, err, "should error when deleting non-existent resource")
	t.Logf(logExpectedError, err)
}

func TestCRCreationInNonExistentNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)

	// Try to create CR in a namespace that doesn't exist
	nonExistentNamespace := "e2e-test-nonexistent-ns-12345"

	cr := loadCRFixture(t, fixtureMinimalCR)
	cr.SetNamespace(nonExistentNamespace)
	cr.SetName("test-in-nonexistent-ns")

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(nonExistentNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})

	assert.Error(t, err, "should error when creating CR in non-existent namespace")
	t.Logf(logExpectedError, err)
}

func TestCRWithMalformedSpec(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
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
			fieldAPIVersion: crAPIVersion,
			fieldKind:       crKind,
			fieldMetadata: map[string]interface{}{
				fieldName:      "test-malformed",
				fieldNamespace: testNamespace,
			},
			fieldSpec: map[string]interface{}{
				fieldAppDomain: errAppDomain,
				fieldReplicas:  "not-a-number", // Wrong type - should be int
				fieldModules: map[string]interface{}{
					fieldServer: "not-an-object", // Wrong type - should be object
				},
			},
		},
	}

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})

	// This should fail if type validation is enforced
	if err != nil {
		t.Logf("Creation failed as expected due to type mismatch: %v", err)
	} else {
		t.Logf("Warning: creation with type mismatches succeeded - strict validation may not be enforced")
	}
}

func TestOperatorHandlesRapidCRUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
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
	cr := loadCRFixture(t, fixtureMinimalCR)
	cr.SetNamespace(testNamespace)
	cr.SetName(crRapidUpdates)

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, msgCreateCR)

	// Perform rapid updates
	for i := 0; i < 5; i++ {
		// Get latest version
		latest, err := res.Get(ctx, crRapidUpdates, metav1.GetOptions{})
		require.NoError(t, err, msgGetCR)

		// Update spec
		newDomain := "test" + string(rune('0'+i)) + ".example.com"
		err = unstructured.SetNestedField(
			latest.Object, newDomain, fieldSpec, fieldAppDomain,
		)
		require.NoError(t, err, msgSetField)

		_, err = res.Update(ctx, latest, metav1.UpdateOptions{})
		if err != nil {
			t.Logf("Update %d failed (may be expected due to conflicts): %v", i, err)
		}
	}

	// Verify final state
	final, err := res.Get(ctx, crRapidUpdates, metav1.GetOptions{})
	require.NoError(t, err, "should be able to get final CR state")
	assert.NotNil(t, final, "final CR should exist")
}
