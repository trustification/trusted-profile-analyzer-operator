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
	"os"
	"testing"
	"time"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// TestMain runs before all tests in the e2e package
func TestMain(m *testing.M) {
	// Parse flags first before calling testing.Short()
	// Note: flag.Parse() is called automatically before m.Run()

	// Setup: Verify cluster is accessible (skip in short mode)
	// We can't use testing.Short() here because flags haven't been parsed yet
	// Instead, we'll rely on each individual test to skip if needed
	skipSetup := false
	for _, arg := range os.Args {
		if arg == "-test.short" || arg == "--test.short" {
			skipSetup = true
			break
		}
	}

	if !skipSetup {
		if err := verifyClusterAccess(); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: Cannot access Kubernetes cluster: %v\n", err)
			fmt.Fprintf(os.Stderr, "E2E tests will be skipped. Run with -short flag to skip e2e tests explicitly.\n")
		}

		if err := verifyOperatorDeployed(); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: Operator may not be deployed: %v\n", err)
			fmt.Fprintf(os.Stderr, "Some tests may fail. Deploy the operator before running e2e tests.\n")
		}

		if err := verifyCRDInstalled(); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: CRD may not be installed: %v\n", err)
			fmt.Fprintf(os.Stderr, "Some tests may fail. Install CRDs with 'make install' before running e2e tests.\n")
		}
	}

	// Run tests
	code := m.Run()

	// Cleanup: Remove any leftover test resources
	if !skipSetup {
		cleanupTestResources()
	}

	os.Exit(code)
}

// verifyClusterAccess checks if we can connect to a Kubernetes cluster
func verifyClusterAccess() error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to list namespaces to verify connectivity
	_, err = clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	return nil
}

// verifyOperatorDeployed checks if the operator is deployed
func verifyOperatorDeployed() error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if operator namespace exists
	_, err = clientset.CoreV1().Namespaces().Get(ctx, operatorNamespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("operator namespace %s not found: %w", operatorNamespace, err)
	}

	// Check if operator deployment exists
	_, err = clientset.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("operator deployment %s not found: %w", operatorName, err)
	}

	return nil
}

// verifyCRDInstalled checks if the TrustedProfileAnalyzer CRD is installed
func verifyCRDInstalled() error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use apiextensions client for CRDs
	apiextensionsClient, err := apiextensionsclientset.NewForConfig(config)
	if err != nil {
		return err
	}

	_, err = apiextensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, "trustedprofileanalyzers.rhtpa.io", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("CRD trustedprofileanalyzers.rhtpa.io not found: %w", err)
	}

	return nil
}

// cleanupTestResources removes any leftover test resources
func cleanupTestResources() {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load kubeconfig for cleanup: %v\n", err)
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create kubernetes client for cleanup: %v\n", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// List all namespaces starting with e2e-test-
	namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list namespaces for cleanup: %v\n", err)
		return
	}

	for _, ns := range namespaces.Items {
		if len(ns.Name) >= 9 && ns.Name[:9] == "e2e-test-" {
			fmt.Printf("Cleaning up test namespace: %s\n", ns.Name)
			err := clientset.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to delete namespace %s: %v\n", ns.Name, err)
			}
		}
	}
}
