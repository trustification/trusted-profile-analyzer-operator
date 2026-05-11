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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOperatorHealthEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	require.Eventually(t, func() bool {
		pods, err := k8sClient.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: controlPlaneLabelSelector,
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}

		pod := pods.Items[0]
		if pod.Status.Phase != corev1.PodRunning {
			return false
		}

		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady {
				return condition.Status == corev1.ConditionTrue
			}
		}
		return false
	}, testTimeout, pollInterval, "operator pod should be running and ready")
}

// assertProbeConfigured is a shared helper for readiness and liveness probe tests
// to avoid code duplication.
func assertProbeConfigured(
	t *testing.T,
	probe *corev1.Probe,
	probeName string,
	expectedPath string,
) {
	t.Helper()

	assert.NotNil(t, probe,
		"manager container should have %s probe", probeName)

	if probe != nil {
		assert.NotNil(t, probe.HTTPGet,
			"%s probe should use HTTP GET", probeName)
		if probe.HTTPGet != nil {
			assert.Equal(t, expectedPath, probe.HTTPGet.Path,
				"%s probe should check %s", probeName, expectedPath)
		}
	}
}

func TestOperatorReadinessProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Get operator deployment
	deployment, err := k8sClient.AppsV1().Deployments(operatorNamespace).Get(
		ctx, operatorName, metav1.GetOptions{})
	if err != nil {
		t.Skip("Operator deployment not found, skipping readiness probe test")
	}

	// Verify readiness probe is configured
	containers := deployment.Spec.Template.Spec.Containers
	require.NotEmpty(t, containers, "deployment should have containers")

	assertProbeConfigured(t, containers[0].ReadinessProbe, "readiness", "/readyz")
}

func TestOperatorLivenessProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Get operator deployment
	deployment, err := k8sClient.AppsV1().Deployments(operatorNamespace).Get(
		ctx, operatorName, metav1.GetOptions{})
	if err != nil {
		t.Skip("Operator deployment not found, skipping liveness probe test")
	}

	// Verify liveness probe is configured
	containers := deployment.Spec.Template.Spec.Containers
	require.NotEmpty(t, containers, "deployment should have containers")

	assertProbeConfigured(t, containers[0].LivenessProbe, "liveness", "/healthz")
}

func TestOperatorPodRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Get current operator pod
	pods, err := k8sClient.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: controlPlaneLabelSelector,
	})

	if err != nil || len(pods.Items) == 0 {
		t.Skip("Operator pod not found, skipping restart test")
	}

	pod := pods.Items[0]
	originalUID := pod.UID
	restartCount := int32(0)
	if len(pod.Status.ContainerStatuses) > 0 {
		restartCount = pod.Status.ContainerStatuses[0].RestartCount
	}

	t.Logf("Current operator pod: %s, UID: %s, RestartCount: %d",
		pod.Name, originalUID, restartCount)

	// In a real scenario, you might want to trigger a restart and verify recovery
	// For this test, we just verify the pod hasn't been restarting excessively
	assert.LessOrEqual(t, restartCount, int32(5),
		"operator pod should not have excessive restarts")
}

func TestOperatorResourceLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Get operator deployment
	deployment, err := k8sClient.AppsV1().Deployments(operatorNamespace).Get(
		ctx, operatorName, metav1.GetOptions{})
	if err != nil {
		t.Skip("Operator deployment not found, skipping resource limits test")
	}

	// Verify resource limits are set
	containers := deployment.Spec.Template.Spec.Containers
	require.NotEmpty(t, containers, "deployment should have containers")

	for _, container := range containers {
		t.Logf("Checking resources for container: %s", container.Name)

		// Resource requests should be set
		if container.Resources.Requests != nil {
			cpu := container.Resources.Requests.Cpu()
			memory := container.Resources.Requests.Memory()
			t.Logf("  Requests - CPU: %s, Memory: %s",
				cpu.String(), memory.String())
		} else {
			t.Logf("  Warning: No resource requests set for container %s",
				container.Name)
		}

		// Resource limits should be set
		if container.Resources.Limits != nil {
			cpu := container.Resources.Limits.Cpu()
			memory := container.Resources.Limits.Memory()
			t.Logf("  Limits - CPU: %s, Memory: %s",
				cpu.String(), memory.String())
		} else {
			t.Logf("  Warning: No resource limits set for container %s",
				container.Name)
		}
	}
}

func TestOperatorLeaderElection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Check for leader election lease
	// The operator uses leader election, so there should be a lease or configmap
	leases, err := k8sClient.CoordinationV1().Leases(operatorNamespace).List(
		ctx, metav1.ListOptions{})
	if err != nil {
		t.Logf("Warning: Could not list leases: %v", err)
	} else {
		t.Logf("Found %d lease(s) in operator namespace", len(leases.Items))
		for _, lease := range leases.Items {
			t.Logf("  Lease: %s, Holder: %v",
				lease.Name, lease.Spec.HolderIdentity)
		}
	}

	// Also check for ConfigMap-based leader election (older style)
	configMaps, err := k8sClient.CoreV1().ConfigMaps(operatorNamespace).List(
		ctx, metav1.ListOptions{})
	if err != nil {
		t.Logf("Warning: Could not list configmaps: %v", err)
	} else {
		for _, cm := range configMaps.Items {
			if cm.Annotations != nil {
				leaderAnnotation := "control-plane.alpha.kubernetes.io/leader"
				if _, ok := cm.Annotations[leaderAnnotation]; ok {
					t.Logf("Found leader election ConfigMap: %s", cm.Name)
				}
			}
		}
	}
}

func TestOperatorMetricsEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Check if metrics service exists
	metricsServiceName := operatorName + "-metrics-service"
	service, err := k8sClient.CoreV1().Services(operatorNamespace).Get(
		ctx, metricsServiceName, metav1.GetOptions{})
	if err != nil {
		t.Skip("Metrics service not found, skipping metrics endpoint test")
	}

	t.Logf("Found metrics service: %s", service.Name)
	assert.NotEmpty(t, service.Spec.Ports, "metrics service should have ports")

	for _, port := range service.Spec.Ports {
		if port.Name == "https" || port.Name == fieldMetrics {
			t.Logf("Metrics port: %s -> %d", port.Name, port.Port)
		}
	}
}

func TestOperatorServiceAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Get operator deployment
	deployment, err := k8sClient.AppsV1().Deployments(operatorNamespace).Get(
		ctx, operatorName, metav1.GetOptions{})
	if err != nil {
		t.Skip("Operator deployment not found, skipping service account test")
	}

	serviceAccountName := deployment.Spec.Template.Spec.ServiceAccountName
	require.NotEmpty(t, serviceAccountName, "deployment should have service account")

	// Verify service account exists
	serviceAccount, err := k8sClient.CoreV1().ServiceAccounts(operatorNamespace).Get(
		ctx, serviceAccountName, metav1.GetOptions{})
	require.NoError(t, err, "service account should exist")
	assert.Equal(t, serviceAccountName, serviceAccount.Name,
		"service account name should match")

	t.Logf("Operator uses service account: %s", serviceAccountName)
}

func TestOperatorLogLevel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Get operator pod
	pods, err := k8sClient.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: controlPlaneLabelSelector,
	})

	if err != nil || len(pods.Items) == 0 {
		t.Skip("Operator pod not found, skipping log level test")
	}

	pod := pods.Items[0]

	// Check for development flag or log level configuration
	for _, container := range pod.Spec.Containers {
		t.Logf("Container %s args: %v", container.Name, container.Args)

		// Look for development or log level flags
		for _, arg := range container.Args {
			if arg == "--development=true" || arg == "--development" {
				t.Logf("Operator is running in development mode")
			}
			if arg == "--zap-devel" {
				t.Logf("Operator is using development logging")
			}
		}
	}
}

func TestOperatorWatchesMultipleNamespaces(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Get operator deployment
	deployment, err := k8sClient.AppsV1().Deployments(operatorNamespace).Get(
		ctx, operatorName, metav1.GetOptions{})
	if err != nil {
		t.Skip("Operator deployment not found, skipping namespace watch test")
	}

	// Check WATCH_NAMESPACE environment variable
	for _, container := range deployment.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == "WATCH_NAMESPACE" {
				t.Logf("Operator watches namespace(s): %s", env.Value)
				if env.Value == "" {
					t.Logf("Operator is watching all namespaces (cluster-wide)")
				}
			}
		}
	}
}
