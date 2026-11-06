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
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// TLSRestartAnnotation is stamped alongside TLSConfigHashAnnotation to force a
// rolling restart when the hash itself has not moved.
//
// The hash records which cluster TLS profile a pod was started for. It cannot
// record whether the ConfigMap that profile was published to still holds what
// the reconciler wrote -- so a pod that started while that ConfigMap was
// hand-edited or missing is running the wrong settings under the right hash,
// and re-patching the same hash is a no-op that never becomes a restart. This
// annotation gives the repair a value that actually changes, the same device
// `kubectl rollout restart` uses.
const TLSRestartAnnotation = "rhtpa.io/tls-restarted-at"

// ManagedByLabel marks the objects this reconciler owns outright, so that one
// found in the namespace is recognisable as its work rather than a user's.
const ManagedByLabel = "app.kubernetes.io/managed-by"

// ManagedByValue is the ManagedByLabel value the reconciler stamps.
const ManagedByValue = "tls-configurator"

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

// SetDeploymentTLSAnnotations patches the TLS config hash -- and, when
// restartedAt is non-empty, a restart stamp -- onto a deployment's pod template
// annotations, returning the patched deployment, whose generation and replica
// count let callers report the rollout they just triggered. Changing either
// annotation rolls the deployment so its pods re-read the new TLS
// configuration.
//
// Pass a restartedAt only to force a restart the hash cannot express: the patch
// is a no-op at the API level when every value is unchanged, so a repair that
// leaves the hash alone needs something else to move. See TLSRestartAnnotation.
func (w *WorkloadsClient) SetDeploymentTLSAnnotations(
	ctx context.Context, namespace, name, hash, restartedAt string,
) (*appsv1.Deployment, error) {
	annotations := map[string]string{TLSConfigHashAnnotation: hash}
	if restartedAt != "" {
		annotations[TLSRestartAnnotation] = restartedAt
	}

	patch := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": annotations,
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

// ApplyProfileConfigMap creates or updates the ConfigMap through which the
// resolved cluster TLS profile reaches the operand's own TLS listener, and
// reports whether its contents actually moved.
//
// The ConfigMap is owned by this reconciler rather than templated by the Helm
// chart, for the same reason the pod-template hash annotation is: the
// operator re-renders the chart periodically and would overwrite anything the
// reconciler had written into a chart-managed object. The chart references it
// as an optional envFrom source, so the Deployment stays chart-owned and the
// two never fight over the same field.
//
// The changed return matters for correctness, not just logging. A pod only
// picks up new ConfigMap values when it restarts, and the restart is triggered
// by the pod-template hash -- so a ConfigMap edited out from under the
// reconciler has to force a rollout even though the cluster profile, and
// therefore the hash, is unchanged.
//
// Ownership is enforced, not assumed: an existing ConfigMap without this
// reconciler's ManagedByLabel is left strictly alone and reported as an error.
func (w *WorkloadsClient) ApplyProfileConfigMap(
	ctx context.Context, namespace, name string, data map[string]string,
) (changed bool, err error) {
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": ManagedByValue,
				ManagedByLabel:           ManagedByValue,
			},
			Annotations: map[string]string{
				"rhtpa.io/description": "Cluster TLS profile, rendered as the RHTPA " +
					"workloads' HTTP server settings. Managed by the TLS reconciler; " +
					"manual edits are reverted.",
			},
		},
		Data: data,
	}

	cms := w.kube.CoreV1().ConfigMaps(namespace)

	existing, err := cms.Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if _, err := cms.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return false, fmt.Errorf("failed to create configmap %s/%s: %w", namespace, name, err)
		}
		return true, nil
	case err != nil:
		return false, fmt.Errorf("failed to read configmap %s/%s: %w", namespace, name, err)
	}

	// Refuse to touch a ConfigMap this reconciler did not create. The name is
	// user-configurable (operandTls.configMapName), so it can be pointed -- by
	// a typo or by a copied values file -- at an object another component owns,
	// and the update below replaces Data wholesale rather than merging into it.
	// Nothing visible here distinguishes a misconfiguration from a deliberate
	// hand-over, so the only safe reading of an unlabelled object is that the
	// name is already taken. Failing is also recoverable in a way that a
	// silently erased ConfigMap is not.
	if owner := existing.Labels[ManagedByLabel]; owner != ManagedByValue {
		return false, fmt.Errorf(
			"configmap %s/%s already exists and is not managed by the TLS reconciler "+
				"(%s=%q, want %q): refusing to overwrite data this reconciler does not own; "+
				"point operandTls.configMapName at an unused name",
			namespace, name, ManagedByLabel, owner, ManagedByValue)
	}

	if maps.Equal(existing.Data, data) {
		return false, nil
	}

	// The DeepCopy carries resourceVersion, so this Update is conflict-checked:
	// a competing writer between the Get above and the Update here loses with a
	// 409 rather than silently clobbering. Do not strip it. The chart keeps the
	// reconciler a singleton (Deployment strategy Recreate) so there should be
	// no competing writer at all; this is the second line of defence.
	updated := existing.DeepCopy()
	updated.Data = data
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	maps.Copy(updated.Labels, desired.Labels)
	if _, err := cms.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return false, fmt.Errorf("failed to update configmap %s/%s: %w", namespace, name, err)
	}
	return true, nil
}
