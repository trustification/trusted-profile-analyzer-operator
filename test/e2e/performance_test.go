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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

func TestMultipleCRsInSameNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-multiple-crs"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create multiple CRs
	numCRs := 3
	for i := 0; i < numCRs; i++ {
		cr := loadCRFixture(t, "minimal_cr.yaml")
		cr.SetNamespace(testNamespace)
		cr.SetName(fmt.Sprintf("test-instance-%d", i))

		_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
		require.NoError(t, err, "should be able to create CR %d", i)
	}

	// Verify all CRs exist
	list, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "should be able to list CRs")
	assert.GreaterOrEqual(t, len(list.Items), numCRs, "should have created at least %d CRs", numCRs)

	// Clean up
	for i := 0; i < numCRs; i++ {
		err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Delete(ctx, fmt.Sprintf("test-instance-%d", i), metav1.DeleteOptions{})
		if err != nil {
			t.Logf("Warning: failed to delete CR %d: %v", i, err)
		}
	}
}

func TestConcurrentCRCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespaces
	numNamespaces := 3
	var wg sync.WaitGroup
	errors := make(chan error, numNamespaces)

	for i := 0; i < numNamespaces; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			testNamespace := fmt.Sprintf("e2e-test-concurrent-%d", index)
			createNamespace(t, k8sClient, testNamespace)
			defer deleteNamespace(t, k8sClient, testNamespace)

			cr := loadCRFixture(t, "minimal_cr.yaml")
			cr.SetNamespace(testNamespace)
			cr.SetName(fmt.Sprintf("concurrent-instance-%d", index))

			_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
			if err != nil {
				errors <- fmt.Errorf("failed to create CR in namespace %s: %w", testNamespace, err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent creation error: %v", err)
	}
}

func TestReconciliationPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-reconcile-perf"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR and measure time
	startTime := time.Now()

	cr := loadCRFixture(t, "minimal_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("perf-test-instance")

	_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR")

	creationDuration := time.Since(startTime)
	t.Logf("CR creation took: %v", creationDuration)

	// Perform update and measure time
	updateStartTime := time.Now()

	retrieved, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, "perf-test-instance", metav1.GetOptions{})
	require.NoError(t, err, "should be able to get CR")

	// Update appDomain
	err = unstructured.SetNestedField(retrieved.Object, "updated.example.com", "spec", "appDomain")
	require.NoError(t, err, "should be able to set field")

	_, err = dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Update(ctx, retrieved, metav1.UpdateOptions{})
	require.NoError(t, err, "should be able to update CR")

	updateDuration := time.Since(updateStartTime)
	t.Logf("CR update took: %v", updateDuration)

	// Performance assertions
	assert.Less(t, creationDuration.Seconds(), 10.0, "CR creation should complete within 10 seconds")
	assert.Less(t, updateDuration.Seconds(), 10.0, "CR update should complete within 10 seconds")
}

func TestCRDeletionPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-delete-perf"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create CR
	cr := loadCRFixture(t, "minimal_cr.yaml")
	cr.SetNamespace(testNamespace)
	cr.SetName("delete-perf-instance")

	_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "should be able to create CR")

	// Give it time to reconcile
	time.Sleep(10 * time.Second)

	// Measure deletion time
	startTime := time.Now()

	err = dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Delete(ctx, "delete-perf-instance", metav1.DeleteOptions{})
	require.NoError(t, err, "should be able to delete CR")

	// Wait for deletion to complete
	err = waitForCRDeletion(ctx, dynamicClient, testNamespace, "delete-perf-instance", 2*time.Minute)
	require.NoError(t, err, "CR should be deleted")

	deletionDuration := time.Since(startTime)
	t.Logf("CR deletion took: %v", deletionDuration)

	// Performance assertion
	assert.Less(t, deletionDuration.Seconds(), 120.0, "CR deletion should complete within 2 minutes")
}

func TestOperatorUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dynamicClient := getDynamicClient(t)
	k8sClient := getKubernetesClient(t)

	// Create test namespace
	testNamespace := "e2e-test-load"
	createNamespace(t, k8sClient, testNamespace)
	defer deleteNamespace(t, k8sClient, testNamespace)

	// Create and update multiple CRs rapidly
	numCRs := 5
	numUpdatesPerCR := 3

	startTime := time.Now()

	for i := 0; i < numCRs; i++ {
		cr := loadCRFixture(t, "minimal_cr.yaml")
		cr.SetNamespace(testNamespace)
		crName := fmt.Sprintf("load-test-%d", i)
		cr.SetName(crName)

		_, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
		require.NoError(t, err, "should be able to create CR %s", crName)

		// Perform rapid updates
		for j := 0; j < numUpdatesPerCR; j++ {
			retrieved, err := dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
			if err != nil {
				t.Logf("Warning: failed to get CR %s for update %d: %v", crName, j, err)
				continue
			}

			newDomain := fmt.Sprintf("load-%d-%d.example.com", i, j)
			err = unstructured.SetNestedField(retrieved.Object, newDomain, "spec", "appDomain")
			require.NoError(t, err, "should be able to set field")

			_, err = dynamicClient.Resource(trustedProfileAnalyzerGVR).Namespace(testNamespace).Update(ctx, retrieved, metav1.UpdateOptions{})
			if err != nil {
				t.Logf("Warning: failed to update CR %s (update %d): %v", crName, j, err)
			}
		}
	}

	totalDuration := time.Since(startTime)
	t.Logf("Created and updated %d CRs (%d updates each) in %v", numCRs, numUpdatesPerCR, totalDuration)

	// Verify operator is still healthy
	time.Sleep(30 * time.Second)

	// Check operator pod status
	pods, err := k8sClient.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "control-plane=controller-manager",
	})

	if err == nil && len(pods.Items) > 0 {
		pod := pods.Items[0]
		assert.Equal(t, "Running", string(pod.Status.Phase), "operator should still be running after load test")

		// Check for excessive restarts
		if len(pod.Status.ContainerStatuses) > 0 {
			restartCount := pod.Status.ContainerStatuses[0].RestartCount
			assert.LessOrEqual(t, restartCount, int32(2), "operator should not have restarted excessively during load test")
			t.Logf("Operator restart count after load test: %d", restartCount)
		}
	}
}

// Helper function to wait for CR deletion
func waitForCRDeletion(ctx context.Context, client dynamic.Interface, namespace, name string, timeout time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for CR deletion")
		case <-ticker.C:
			_, err := client.Resource(trustedProfileAnalyzerGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				// CR not found - deletion complete
				return nil
			}
		}
	}
}
