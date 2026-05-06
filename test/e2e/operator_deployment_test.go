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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	pollInterval = 5 * time.Second
)

func TestOperatorDeploymentExists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	clientset := getKubernetesClient(t)

	deployment, err := clientset.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorName, metav1.GetOptions{})
	require.NoError(t, err, "operator deployment should exist")

	assert.NotNil(t, deployment, "deployment should not be nil")
	assert.Equal(t, operatorName, deployment.Name, "deployment name should match")
}

func TestOperatorDeploymentReady(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	clientset := getKubernetesClient(t)

	// Wait for deployment to be ready
	require.Eventually(t, func() bool {
		deployment, err := clientset.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorName, metav1.GetOptions{})
		if err != nil {
			t.Logf("Error getting deployment: %v", err)
			return false
		}

		// Check if deployment is ready
		if deployment.Status.ReadyReplicas >= 1 && deployment.Status.Replicas == deployment.Status.ReadyReplicas {
			return true
		}

		t.Logf("Deployment not ready yet: %d/%d replicas ready", deployment.Status.ReadyReplicas, deployment.Status.Replicas)
		return false
	}, testTimeout, pollInterval, "operator deployment should be ready")
}

func TestOperatorPodRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	clientset := getKubernetesClient(t)

	// Get pods with operator label
	labelSelector := labels.Set{
		controlPlaneLabel: controlPlaneValue,
	}.AsSelector()

	require.Eventually(t, func() bool {
		pods, err := clientset.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		})
		if err != nil {
			t.Logf("Error listing pods: %v", err)
			return false
		}

		if len(pods.Items) == 0 {
			t.Logf("No operator pods found")
			return false
		}

		// Check if at least one pod is running
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				// Verify all containers are ready
				allReady := true
				for _, status := range pod.Status.ContainerStatuses {
					if !status.Ready {
						allReady = false
						break
					}
				}
				if allReady {
					return true
				}
			}
		}

		t.Logf("No running pods found yet")
		return false
	}, testTimeout, pollInterval, "operator pod should be running")
}

func TestOperatorPodPhase(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	clientset := getKubernetesClient(t)

	// Get operator pod
	labelSelector := labels.Set{
		controlPlaneLabel: controlPlaneValue,
	}.AsSelector()

	pods, err := clientset.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})
	require.NoError(t, err, "should be able to list operator pods")
	require.NotEmpty(t, pods.Items, "at least one operator pod should exist")

	pod := pods.Items[0]
	assert.Equal(t, corev1.PodRunning, pod.Status.Phase, "operator pod should be running")
}

func TestOperatorMetricsService(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	clientset := getKubernetesClient(t)

	// Check if metrics service exists
	services, err := clientset.CoreV1().Services(operatorNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "should be able to list services")

	metricsServiceFound := false
	for _, svc := range services.Items {
		if svc.Name == "rhtpa-operator-controller-manager-metrics-service" {
			metricsServiceFound = true
			assert.NotEmpty(t, svc.Spec.Ports, "metrics service should have ports")
			break
		}
	}

	assert.True(t, metricsServiceFound, "metrics service should exist")
}

func TestOperatorRBAC(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	clientset := getKubernetesClient(t)

	// Check if ServiceAccount exists
	saName := "rhtpa-operator-controller-manager"
	sa, err := clientset.CoreV1().ServiceAccounts(operatorNamespace).Get(
		ctx, saName, metav1.GetOptions{})
	require.NoError(t, err, "operator service account should exist")
	assert.NotNil(t, sa, "service account should not be nil")

	// Check if ClusterRole exists
	crName := "rhtpa-operator-manager-role"
	cr, err := clientset.RbacV1().ClusterRoles().Get(
		ctx, crName, metav1.GetOptions{})
	require.NoError(t, err, "operator cluster role should exist")
	assert.NotNil(t, cr, "cluster role should not be nil")
	assert.NotEmpty(t, cr.Rules, "cluster role should have rules")

	// Check if ClusterRoleBinding exists
	crbName := "rhtpa-operator-manager-rolebinding"
	crb, err := clientset.RbacV1().ClusterRoleBindings().Get(
		ctx, crbName, metav1.GetOptions{})
	require.NoError(t, err, "operator cluster role binding should exist")
	assert.NotNil(t, crb, "cluster role binding should not be nil")
}

func TestOperatorLeaderElectionLease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	clientset := getKubernetesClient(t)

	// Wait for leader election lease to be created
	require.Eventually(t, func() bool {
		leases, err := clientset.CoordinationV1().Leases(operatorNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Logf("Error listing leases: %v", err)
			return false
		}

		for _, lease := range leases.Items {
			if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != "" {
				t.Logf("Found lease with holder: %s", *lease.Spec.HolderIdentity)
				return true
			}
		}

		t.Logf("No active leader election lease found yet")
		return false
	}, testTimeout, pollInterval, "leader election lease should be created")
}

func TestOperatorScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	clientset := getKubernetesClient(t)

	deployment, err := clientset.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorName, metav1.GetOptions{})
	require.NoError(t, err, "operator deployment should exist")

	// Verify deployment has desired replicas set
	assert.NotNil(t, deployment.Spec.Replicas, "deployment should have replicas specified")
	assert.Greater(t, *deployment.Spec.Replicas, int32(0), "deployment should have at least 1 replica")

	// Verify replicas match status
	assert.Equal(t, *deployment.Spec.Replicas, deployment.Status.Replicas, "deployment status should match spec")
}

func TestOperatorLogs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	clientset := getKubernetesClient(t)

	// Get operator pod
	labelSelector := labels.Set{
		controlPlaneLabel: controlPlaneValue,
	}.AsSelector()

	pods, err := clientset.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})
	require.NoError(t, err, "should be able to list operator pods")
	require.NotEmpty(t, pods.Items, "at least one operator pod should exist")

	pod := pods.Items[0]

	// Get pod logs
	logOptions := &corev1.PodLogOptions{
		TailLines: ptr(int64(100)),
	}

	logs := clientset.CoreV1().Pods(operatorNamespace).GetLogs(pod.Name, logOptions)
	logStream, err := logs.Stream(ctx)
	require.NoError(t, err, "should be able to get operator logs")
	defer func() { _ = logStream.Close() }()

	// Just verify we can read logs
	buf := make([]byte, 1024)
	n, _ := logStream.Read(buf)
	assert.Greater(t, n, 0, "should be able to read operator logs")
}

// Helper functions

func ptr[T any](v T) *T {
	return &v
}
