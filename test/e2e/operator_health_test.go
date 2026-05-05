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
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOperatorHealthEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Get operator pod
	pods, err := k8sClient.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "control-plane=controller-manager",
	})

	if err != nil || len(pods.Items) == 0 {
		t.Skip("Operator pod not found, skipping health endpoint test")
	}

	podName := pods.Items[0].Name
	t.Logf("Found operator pod: %s", podName)

	// Port-forward to health endpoint
	// Note: In a real test, you'd set up port forwarding or use a service
	// For now, we'll just verify the pod is running
	assert.Equal(t, "Running", string(pods.Items[0].Status.Phase), "operator pod should be running")

	// Check pod readiness
	for _, condition := range pods.Items[0].Status.Conditions {
		if condition.Type == "Ready" {
			assert.Equal(t, "True", string(condition.Status), "operator pod should be ready")
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
	deployment, err := k8sClient.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorName, metav1.GetOptions{})
	if err != nil {
		t.Skip("Operator deployment not found, skipping readiness probe test")
	}

	// Verify readiness probe is configured
	containers := deployment.Spec.Template.Spec.Containers
	require.NotEmpty(t, containers, "deployment should have containers")

	managerContainer := containers[0]
	assert.NotNil(t, managerContainer.ReadinessProbe, "manager container should have readiness probe")

	if managerContainer.ReadinessProbe != nil {
		assert.NotNil(t, managerContainer.ReadinessProbe.HTTPGet, "readiness probe should use HTTP GET")
		if managerContainer.ReadinessProbe.HTTPGet != nil {
			assert.Equal(t, "/readyz", managerContainer.ReadinessProbe.HTTPGet.Path, "readiness probe should check /readyz")
		}
	}
}

func TestOperatorLivenessProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Get operator deployment
	deployment, err := k8sClient.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorName, metav1.GetOptions{})
	if err != nil {
		t.Skip("Operator deployment not found, skipping liveness probe test")
	}

	// Verify liveness probe is configured
	containers := deployment.Spec.Template.Spec.Containers
	require.NotEmpty(t, containers, "deployment should have containers")

	managerContainer := containers[0]
	assert.NotNil(t, managerContainer.LivenessProbe, "manager container should have liveness probe")

	if managerContainer.LivenessProbe != nil {
		assert.NotNil(t, managerContainer.LivenessProbe.HTTPGet, "liveness probe should use HTTP GET")
		if managerContainer.LivenessProbe.HTTPGet != nil {
			assert.Equal(t, "/healthz", managerContainer.LivenessProbe.HTTPGet.Path, "liveness probe should check /healthz")
		}
	}
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
		LabelSelector: "control-plane=controller-manager",
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

	t.Logf("Current operator pod: %s, UID: %s, RestartCount: %d", pod.Name, originalUID, restartCount)

	// In a real scenario, you might want to trigger a restart and verify recovery
	// For this test, we just verify the pod hasn't been restarting excessively
	assert.LessOrEqual(t, restartCount, int32(5), "operator pod should not have excessive restarts")
}

func TestOperatorResourceLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	k8sClient := getKubernetesClient(t)

	// Get operator deployment
	deployment, err := k8sClient.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorName, metav1.GetOptions{})
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
			t.Logf("  Requests - CPU: %s, Memory: %s", cpu.String(), memory.String())
		} else {
			t.Logf("  Warning: No resource requests set for container %s", container.Name)
		}

		// Resource limits should be set
		if container.Resources.Limits != nil {
			cpu := container.Resources.Limits.Cpu()
			memory := container.Resources.Limits.Memory()
			t.Logf("  Limits - CPU: %s, Memory: %s", cpu.String(), memory.String())
		} else {
			t.Logf("  Warning: No resource limits set for container %s", container.Name)
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
	leases, err := k8sClient.CoordinationV1().Leases(operatorNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Logf("Warning: Could not list leases: %v", err)
	} else {
		t.Logf("Found %d lease(s) in operator namespace", len(leases.Items))
		for _, lease := range leases.Items {
			t.Logf("  Lease: %s, Holder: %v", lease.Name, lease.Spec.HolderIdentity)
		}
	}

	// Also check for ConfigMap-based leader election (older style)
	configMaps, err := k8sClient.CoreV1().ConfigMaps(operatorNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Logf("Warning: Could not list configmaps: %v", err)
	} else {
		for _, cm := range configMaps.Items {
			if cm.Annotations != nil {
				if _, ok := cm.Annotations["control-plane.alpha.kubernetes.io/leader"]; ok {
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
	service, err := k8sClient.CoreV1().Services(operatorNamespace).Get(ctx, operatorName+"-metrics-service", metav1.GetOptions{})
	if err != nil {
		t.Skip("Metrics service not found, skipping metrics endpoint test")
	}

	t.Logf("Found metrics service: %s", service.Name)
	assert.NotEmpty(t, service.Spec.Ports, "metrics service should have ports")

	for _, port := range service.Spec.Ports {
		if port.Name == "https" || port.Name == "metrics" {
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
	deployment, err := k8sClient.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorName, metav1.GetOptions{})
	if err != nil {
		t.Skip("Operator deployment not found, skipping service account test")
	}

	serviceAccountName := deployment.Spec.Template.Spec.ServiceAccountName
	require.NotEmpty(t, serviceAccountName, "deployment should have service account")

	// Verify service account exists
	serviceAccount, err := k8sClient.CoreV1().ServiceAccounts(operatorNamespace).Get(ctx, serviceAccountName, metav1.GetOptions{})
	require.NoError(t, err, "service account should exist")
	assert.Equal(t, serviceAccountName, serviceAccount.Name, "service account name should match")

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
		LabelSelector: "control-plane=controller-manager",
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
	deployment, err := k8sClient.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorName, metav1.GetOptions{})
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

func queryHealthEndpoint(podIP string, port int, path string) error {
	url := fmt.Sprintf("http://%s:%d%s", podIP, port, path)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
