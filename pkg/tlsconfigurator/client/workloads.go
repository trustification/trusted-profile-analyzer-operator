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

	appsv1 "k8s.io/api/apps/v1"
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

// NewWorkloadsClientWithClientset creates a WorkloadsClient around an existing
// Kubernetes clientset. It exists so callers can inject an alternative
// implementation -- notably a fake clientset in tests -- without going through a
// *rest.Config.
func NewWorkloadsClientWithClientset(kube kubernetes.Interface) *WorkloadsClient {
	return &WorkloadsClient{kube: kube}
}

// GetDeployment returns a target deployment. Callers read both its stamped TLS
// hash (see TLSHashOf) and its rollout status from it, so it is fetched whole
// rather than projected down to the annotation.
func (w *WorkloadsClient) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	dep, err := w.kube.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}
	return dep, nil
}

// TLSHashOf returns the TLS config hash stamped on a deployment's pod template,
// or "" if it carries none.
func TLSHashOf(dep *appsv1.Deployment) string {
	if dep == nil {
		return ""
	}
	return dep.Spec.Template.Annotations[TLSConfigHashAnnotation]
}

// SetDeploymentTLSHash patches the TLS config hash onto a deployment's pod
// template annotations and returns the patched deployment, whose generation
// and replica count let callers report the rollout they just triggered.
// Changing the annotation rolls the deployment so its pods re-read the new TLS
// configuration. The patch is a no-op at the API level when the value is
// unchanged.
func (w *WorkloadsClient) SetDeploymentTLSHash(
	ctx context.Context, namespace, name, hash string,
) (*appsv1.Deployment, error) {
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
		return nil, fmt.Errorf("failed to marshal deployment patch: %w", err)
	}

	dep, err := w.kube.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.StrategicMergePatchType, data, metav1.PatchOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to patch deployment %s/%s: %w", namespace, name, err)
	}

	return dep, nil
}
