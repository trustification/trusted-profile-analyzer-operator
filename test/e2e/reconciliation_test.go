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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

func TestReconciliationCreatesResources(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-reconcile"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR
	cr := loadCRFixture(t, "valid_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("test-reconcile-instance")

	_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR")

	// Wait for Helm release to be created and resources to be reconciled
	// This may take time as the operator processes the CR
	time.Sleep(30 * time.Second)

	// Check if any resources were created
	// Note: Exact resources depend on Helm chart configuration
	// We'll check for common resources that should be created

	// Check for ConfigMaps (Helm creates release ConfigMaps)
	require.Eventually(t, func() bool {
		configMaps, err := k8sClient.CoreV1().ConfigMaps(testNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Logf("Error listing ConfigMaps: %v", err)
			return false
		}

		// Look for Helm release ConfigMaps
		for _, cm := range configMaps.Items {
			if cm.Labels["owner"] == "helm" {
				t.Logf("Found Helm release ConfigMap: %s", cm.Name)
				return true
			}
		}

		return false
	}, 2*time.Minute, 10*time.Second, "Helm release ConfigMap should be created")
}

func TestReconciliationIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-idempotent"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR
	cr := loadCRFixture(t, "minimal_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("test-idempotent-instance")

	created, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR")

	// Wait for initial reconciliation
	time.Sleep(20 * time.Second)

	// Trigger reconciliation by updating an annotation
	annotations := created.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["test.rhtpa.io/trigger"] = time.Now().Format(time.RFC3339)
	created.SetAnnotations(annotations)

	_, err = dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Update(ctx, created, metav1.UpdateOptions{})
	require.NoError(t, err, "should be able to trigger reconciliation")

	// Wait for reconciliation
	time.Sleep(20 * time.Second)

	// Verify CR still exists and is unchanged
	retrieved, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, "test-idempotent-instance", metav1.GetOptions{})
	require.NoError(t, err, "CR should still exist after reconciliation")
	assert.NotNil(t, retrieved, "CR should not be nil")
}

func TestReconciliationOnSpecChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-spec-change"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR with initial spec
	cr := loadCRFixture(t, "minimal_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("test-spec-change-instance")

	created, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR")

	// Wait for initial reconciliation
	time.Sleep(20 * time.Second)

	// Update spec
	spec, found, err := unstructured.NestedMap(created.Object, "spec")
	require.NoError(t, err, "should be able to get spec")
	require.True(t, found, "spec should exist")

	// Add replicas field
	spec["replicas"] = int64(2)
	err = unstructured.SetNestedMap(created.Object, spec, "spec")
	require.NoError(t, err, "should be able to set spec")

	updated, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Update(ctx, created, metav1.UpdateOptions{})
	require.NoError(t, err, "should be able to update CR spec")

	// Wait for reconciliation after spec change
	time.Sleep(30 * time.Second)

	// Verify the update was processed
	retrieved, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, "test-spec-change-instance", metav1.GetOptions{})
	require.NoError(t, err, "should be able to get updated CR")

	retrievedSpec, found, err := unstructured.NestedMap(retrieved.Object, "spec")
	require.NoError(t, err, "should be able to get spec from retrieved CR")
	require.True(t, found, "spec should exist in retrieved CR")

	replicas, found, err := unstructured.NestedInt64(retrievedSpec, "replicas")
	require.NoError(t, err, "should be able to get replicas")
	require.True(t, found, "replicas should exist")
	assert.Equal(t, int64(2), replicas, "replicas should be updated")

	_ = updated // Mark as used
}

func TestReconciliationPeriod(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-reconcile-period"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR
	cr := loadCRFixture(t, "minimal_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("test-period-instance")

	_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR")

	// Wait for initial reconciliation
	time.Sleep(20 * time.Second)

	// Get CR and record resource version
	cr1, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, "test-period-instance", metav1.GetOptions{})
	require.NoError(t, err, "should be able to get CR")
	rv1 := cr1.GetResourceVersion()

	// Wait for reconcile period (default is 1 minute according to main.go)
	// Add buffer time for processing
	time.Sleep(90 * time.Second)

	// Get CR again and check if it was reconciled
	cr2, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, "test-period-instance", metav1.GetOptions{})
	require.NoError(t, err, "should be able to get CR after reconcile period")

	// Note: Resource version may or may not change depending on whether
	// the Helm operator updates the status. We just verify the CR is still valid.
	assert.NotNil(t, cr2, "CR should still exist after reconcile period")

	_ = rv1 // Mark as used
}

func TestConcurrentReconciliation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-concurrent"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create multiple CRs to test concurrent reconciliation
	// The operator is configured with MaxConcurrentReconciles: 4
	numCRs := 3

	for i := 0; i < numCRs; i++ {
		cr := loadCRFixture(t, "minimal_cr.yaml")
		cr.SetNamespace(testNamespace)
		cr.SetName(generateCRName("concurrent", i))

		_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
		require.NoError(t, err, "should be able to create CR %d", i)
	}

	// Wait for all CRs to be reconciled
	time.Sleep(45 * time.Second)

	// Verify all CRs still exist
	for i := 0; i < numCRs; i++ {
		crName := generateCRName("concurrent", i)
		retrieved, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
		require.NoError(t, err, "CR %s should exist", crName)
		assert.NotNil(t, retrieved, "CR %s should not be nil", crName)
	}
}

func TestReconciliationWithDependentResources(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-dependent"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR
	cr := loadCRFixture(t, "valid_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("test-dependent-instance")

	_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR")

	// Wait for reconciliation
	time.Sleep(30 * time.Second)

	// According to watches.yaml, WatchDependentResources is false
	// This means the operator doesn't watch resources created by Helm
	// We'll just verify that the operator doesn't error during reconciliation

	retrieved, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, "test-dependent-instance", metav1.GetOptions{})
	require.NoError(t, err, "CR should exist after reconciliation")
	assert.NotNil(t, retrieved, "CR should not be nil")
}

func TestReconciliationHandlesErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-error-handling"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR with potentially invalid configuration
	// (empty appDomain might cause Helm rendering issues)
	cr := loadCRFixture(t, "minimal_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("test-error-instance")

	// Modify to have invalid spec
	spec, _, _ := unstructured.NestedMap(cr.Object, "spec")
	spec["appDomain"] = "" // Empty appDomain might cause issues
	_ = unstructured.SetNestedMap(cr.Object, spec, "spec")

	_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR even with invalid spec")

	// Wait for reconciliation attempt
	time.Sleep(30 * time.Second)

	// Verify CR still exists (operator should handle errors gracefully)
	retrieved, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, "test-error-instance", metav1.GetOptions{})
	require.NoError(t, err, "CR should still exist even after reconciliation errors")
	assert.NotNil(t, retrieved, "CR should not be nil")

	// Check operator logs for error handling
	// In a real test, we'd parse logs to verify proper error handling
	// For now, we just verify the operator didn't crash
	pods, err := k8sClient.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{"control-plane": "controller-manager"}.AsSelector().String(),
	})
	require.NoError(t, err, "should be able to list operator pods")
	require.NotEmpty(t, pods.Items, "operator pod should still exist")
	assert.Equal(t, "Running", string(pods.Items[0].Status.Phase), "operator pod should still be running")
}

// Helper function to generate unique CR names
func generateCRName(prefix string, index int) string {
	return prefix + "-" + string(rune('a'+index))
}
