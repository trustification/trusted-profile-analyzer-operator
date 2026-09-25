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
package client

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// TLSConfigHashAnnotation is stamped onto a workload's pod template so that a
// change to the resolved TLS configuration produces a new pod spec and triggers
// a rolling restart. It is deliberately not managed by the Helm chart so that
// the operator's periodic re-render does not fight the reconciler.
const TLSConfigHashAnnotation = "rhtpa.io/tls-config-hash"

// WorkloadsClient wraps the core Kubernetes clientset for rolling out workloads.
type WorkloadsClient struct {
	kube kubernetes.Interface
}

// NewWorkloadsClient creates a new WorkloadsClient.
func NewWorkloadsClient(config *rest.Config) (*WorkloadsClient, error) {
	kube, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	return &WorkloadsClient{kube: kube}, nil
}

// GetDeploymentTLSHash returns the current TLS config hash annotation on a
// deployment's pod template, or "" if it is not set.
func (w *WorkloadsClient) GetDeploymentTLSHash(ctx context.Context, namespace, name string) (string, error) {
	dep, err := w.kube.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}
	return dep.Spec.Template.Annotations[TLSConfigHashAnnotation], nil
}

// SetDeploymentTLSHash patches the TLS config hash onto a deployment's pod
// template annotations. Changing the annotation rolls the deployment so its
// pods re-read the new TLS configuration. The patch is a no-op at the API level
// when the value is unchanged.
func (w *WorkloadsClient) SetDeploymentTLSHash(ctx context.Context, namespace, name, hash string) error {
	patch := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]string{
						TLSConfigHashAnnotation: hash,
					},
				},
			},
		},
	}

	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal deployment patch: %w", err)
	}

	if _, err := w.kube.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.StrategicMergePatchType, data, metav1.PatchOptions{},
	); err != nil {
		return fmt.Errorf("failed to patch deployment %s/%s: %w", namespace, name, err)
	}

	return nil
}
