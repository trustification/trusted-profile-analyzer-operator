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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	testTimeout       = 5 * time.Minute
	operatorNamespace = "trusted-profile-analyzer-operator-system"
	operatorName      = "trusted-profile-analyzer-operator-controller-manager"

	// Label selector for operator pods.
	controlPlaneLabel         = "control-plane"
	controlPlaneValue         = "controller-manager"
	controlPlaneLabelSelector = "control-plane=controller-manager"

	// Fixture file names.
	fixtureMinimalCR = "minimal_cr.yaml"
	fixtureValidCR   = "valid_cr.yaml"
	fieldSpec        = "spec"
	fieldAPIVersion  = "apiVersion"
	fieldKind        = "kind"
	fieldMetadata    = "metadata"
	fieldName        = "name"
	fieldNamespace   = "namespace"
	fieldAppDomain   = "appDomain"
	fieldReplicas    = "replicas"
	fieldModules     = "modules"
	fieldServer      = "server"
	fieldEnabled     = "enabled"
	fieldImporter    = "importer"
	fieldIngress     = "ingress"
	fieldHelm        = "helm"
	fieldStorage     = "storage"
	fieldDatabase    = "database"
	fieldMetrics     = "metrics"
	fieldTracing     = "tracing"
	fieldType        = "type"
	fieldBucket      = "bucket"
	fieldRegion      = "region"

	// Common test skip messages.
	skipE2ETest         = "skipping e2e test in short mode"
	skipPerformanceTest = "skipping performance test in short mode"

	// Common assertion / error messages.
	msgCreateCR       = "should be able to create CR"
	msgGetCR          = "should be able to get CR"
	msgUpdateCR       = "should be able to update CR"
	msgDeleteCR       = "should be able to delete CR"
	msgGetSpec        = "should be able to get spec"
	msgSetField       = "should be able to set field"
	msgSpecExist      = "spec should exist"
	msgCRExist        = "CR should exist"
	msgCRNotNil       = "CR should not be nil"
	msgCRExistUpgrade = "CR should exist after upgrade"
)

// createCRAndVerifyExists creates a CR from a fixture in the given
// namespace, waits for reconciliation, and verifies it still exists.
// It returns the retrieved CR.
func createCRAndVerifyExists(
	t *testing.T,
	ctx context.Context,
	dynamicClient dynamic.Interface,
	k8sClient *kubernetes.Clientset,
	namespace, crName, fixture string,
	waitDuration time.Duration,
	createMsg, verifyMsg string,
) *unstructured.Unstructured {
	t.Helper()

	createNamespace(t, k8sClient, namespace)
	t.Cleanup(func() { deleteNamespace(t, k8sClient, namespace) })

	cr := loadCRFixture(t, fixture)
	cr.SetNamespace(namespace)
	cr.SetName(crName)

	res := dynamicClient.Resource(trustedProfileAnalyzerGVR).
		Namespace(namespace)
	_, err := res.Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, createMsg)

	time.Sleep(waitDuration)

	retrieved, err := res.Get(ctx, crName, metav1.GetOptions{})
	require.NoError(t, err, verifyMsg)

	return retrieved
}

// getKubernetesClient returns a Kubernetes clientset for testing
func getKubernetesClient(t *testing.T) *kubernetes.Clientset {
	t.Helper()

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	require.NoError(t, err, "should be able to load kubeconfig")

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err, "should be able to create kubernetes client")

	return clientset
}

// getDynamicClient returns a dynamic client for testing CRs
func getDynamicClient(t *testing.T) dynamic.Interface {
	t.Helper()

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	require.NoError(t, err, "should be able to load kubeconfig")

	dynamicClient, err := dynamic.NewForConfig(config)
	require.NoError(t, err, "should be able to create dynamic client")

	return dynamicClient
}

// createNamespace creates a test namespace
func createNamespace(t *testing.T, clientset *kubernetes.Clientset, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err, "should be able to create namespace")
	}
}

// deleteNamespace deletes a test namespace
func deleteNamespace(t *testing.T, clientset *kubernetes.Clientset, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := clientset.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Logf("Warning: failed to delete namespace %s: %v", name, err)
	}
}

// loadCRFixture loads a CR fixture from the fixtures directory
func loadCRFixture(t *testing.T, filename string) *unstructured.Unstructured {
	t.Helper()

	fixturePath := filepath.Join("..", "fixtures", filename)
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "should be able to read fixture file %s", filename)

	obj := &unstructured.Unstructured{}
	err = yaml.Unmarshal(data, obj)
	require.NoError(t, err, "should be able to unmarshal fixture YAML")

	return obj
}
