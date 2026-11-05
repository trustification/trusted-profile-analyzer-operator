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

package reconcile

import (
	"context"
	"errors"
	"testing"

	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/client"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/crypto"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

const (
	testConfigMap = "tls-configurator-profile"
	// configMapsResource is the resource name the fake clientset records
	// actions against, used to assert what the reconciler did or did not touch.
	configMapsResource = "configmaps"
	// wantOperandProfile is the only trustd profile that honors an explicit
	// minimum version and cipher list; the reconciler always publishes it.
	wantOperandProfile = "custom"
	// wantMinVersion is the minimum TLS version the fixtures below resolve to,
	// in trustd's spelling of it.
	wantMinVersion = "1.3"
)

func (h *harness) configMapData(t *testing.T) map[string]string {
	t.Helper()
	cm, err := h.kube.CoreV1().ConfigMaps(testNamespace).
		Get(context.Background(), testConfigMap, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to read back configmap %q: %v", testConfigMap, err)
	}
	return cm.Data
}

// TestPublishingIsOffByDefault is the guarantee the whole feature rests on:
// with no ConfigMap configured, the reconciler behaves exactly as it did
// before it could configure the operand -- it rolls the workloads and writes
// nothing else.
func TestPublishingIsOffByDefault(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:     modernProfile(),
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
	})

	if err := h.r.reconcileOnce(context.Background(), triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() unexpected error = %v", err)
	}

	for _, a := range h.kube.Actions() {
		if a.GetResource().Resource == configMapsResource {
			t.Errorf("reconcileOnce() performed a %q on configmaps, want the operand "+
				"left alone when no ConfigMap is configured", a.GetVerb())
		}
	}
	assertPatched(t, h.patchedDeployments(), []string{testDeployment})
}

// TestPublishWritesTheProfile checks the happy path: the resolved profile
// lands in the ConfigMap in the operand's own vocabulary.
func TestPublishWritesTheProfile(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:          modernProfile(),
		deployments:      []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
		operandConfigMap: testConfigMap,
	})

	if err := h.r.reconcileOnce(context.Background(), triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() unexpected error = %v", err)
	}

	data := h.configMapData(t)
	if got, want := data[crypto.EnvProfile], wantOperandProfile; got != want {
		t.Errorf("configmap %s = %q, want %q", crypto.EnvProfile, got, want)
	}
	if got, want := data[crypto.EnvMinVersion], wantMinVersion; got != want {
		t.Errorf("configmap %s = %q, want %q for the Modern profile",
			crypto.EnvMinVersion, got, want)
	}
	assertPatched(t, h.patchedDeployments(), []string{testDeployment})
}

// TestPublishHappensBeforeTheRollout pins the ordering. A pod reads the
// ConfigMap when it starts, so writing it after the restart has been triggered
// is a race the pods can lose.
func TestPublishHappensBeforeTheRollout(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:          modernProfile(),
		deployments:      []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
		operandConfigMap: testConfigMap,
	})

	if err := h.r.reconcileOnce(context.Background(), triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() unexpected error = %v", err)
	}

	var sawConfigMapWrite bool
	for _, a := range h.kube.Actions() {
		switch {
		case a.GetResource().Resource == configMapsResource && (a.GetVerb() == "create" || a.GetVerb() == "update"):
			sawConfigMapWrite = true
		case a.GetResource().Resource == "deployments" && a.GetVerb() == "patch":
			if !sawConfigMapWrite {
				t.Fatal("the deployment was patched before the operand settings were written: " +
					"the restarting pods would read the old ConfigMap")
			}
		}
	}
	if !sawConfigMapWrite {
		t.Fatal("no ConfigMap write happened at all")
	}
}

// TestPublishFailureBlocksTheRollout checks that a workload is never stamped
// with a configuration that was not successfully published: the hash is what
// says "these pods are running the current profile".
func TestPublishFailureBlocksTheRollout(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:          modernProfile(),
		deployments:      []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
		operandConfigMap: testConfigMap,
	})
	h.kube.PrependReactor("create", configMapsResource,
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("boom")
		})

	err := h.r.reconcileOnce(context.Background(), triggerStartup)
	if err == nil {
		t.Fatal("reconcileOnce() = nil, want the publish failure reported")
	}
	if got := h.patchedDeployments(); len(got) != 0 {
		t.Errorf("reconcileOnce() patched %v, want no rollout when publishing failed", got)
	}
}

// TestEnablingPublishingRollsTheWorkloads is why the published settings are
// part of the rollout hash. Switching publishing on changes what the pods must
// read without touching the cluster profile, so a workload already carrying
// the profile's hash still has to restart -- otherwise it would keep running
// with no TLS settings from the ConfigMap it now references.
func TestEnablingPublishingRollsTheWorkloads(t *testing.T) {
	profile := modernProfile()

	// The hash a workload carries after a reconcile with publishing off.
	withoutPublishing := newHarness(t, harnessOpts{
		profile:     profile,
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
	})
	if err := withoutPublishing.r.reconcileOnce(context.Background(), triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() unexpected error = %v", err)
	}
	stamped := withoutPublishing.storedHash(t, testDeployment)

	// The same profile, now with publishing on, against a workload that
	// already carries that hash.
	h := newHarness(t, harnessOpts{
		profile:          profile,
		deployments:      []*appsv1.Deployment{deploymentWithHash(testDeployment, stamped)},
		operandConfigMap: testConfigMap,
	})
	if err := h.r.reconcileOnce(context.Background(), triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() unexpected error = %v", err)
	}

	assertPatched(t, h.patchedDeployments(), []string{testDeployment})
	if got := h.storedHash(t, testDeployment); got == stamped {
		t.Errorf("stored hash = %q, want it to differ from the publishing-off hash %q",
			got, stamped)
	}
}

// TestPublishIsIdempotent checks that a steady cluster stays quiet: a second
// pass must not rewrite the ConfigMap, which would be a pointless write on
// every resync tick.
func TestPublishIsIdempotent(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:          modernProfile(),
		deployments:      []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
		operandConfigMap: testConfigMap,
	})

	ctx := context.Background()
	if err := h.r.reconcileOnce(ctx, triggerStartup); err != nil {
		t.Fatalf("first reconcileOnce() unexpected error = %v", err)
	}
	h.kube.ClearActions()
	if err := h.r.reconcileOnce(ctx, triggerResync); err != nil {
		t.Fatalf("second reconcileOnce() unexpected error = %v", err)
	}

	for _, a := range h.kube.Actions() {
		if a.GetResource().Resource == configMapsResource && a.GetVerb() != "get" {
			t.Errorf("second reconcileOnce() performed a %q on configmaps, want no rewrite "+
				"when nothing changed", a.GetVerb())
		}
	}
	assertPatched(t, h.patchedDeployments(), nil)
}

// TestPublishRepairsAnEditedConfigMap checks that the reconciler owns the
// ConfigMap outright. A hand-edited value is reverted on the next pass, so the
// cluster profile remains the single source of truth for what the operand
// serves.
//
// Reverting the ConfigMap is only half the repair. A pod that started while the
// edit was live read the tampered settings and is still serving them, and its
// TLS hash is the current one -- the hash describes the cluster profile, which
// never moved. So the repair has to roll the workloads as well, under a stamp
// that differs from what the pod template already carries.
func TestPublishRepairsAnEditedConfigMap(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:          modernProfile(),
		deployments:      []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
		operandConfigMap: testConfigMap,
	})

	ctx := context.Background()
	if err := h.r.reconcileOnce(ctx, triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() unexpected error = %v", err)
	}
	hashBefore := h.storedHash(t, testDeployment)

	cm, err := h.kube.CoreV1().ConfigMaps(testNamespace).Get(ctx, testConfigMap, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to read back the configmap: %v", err)
	}
	cm.Data[crypto.EnvMinVersion] = "1.0"
	if _, err := h.kube.CoreV1().ConfigMaps(testNamespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("failed to tamper with the configmap: %v", err)
	}
	h.kube.ClearActions()

	if err := h.r.reconcileOnce(ctx, triggerResync); err != nil {
		t.Fatalf("second reconcileOnce() unexpected error = %v", err)
	}

	if got, want := h.configMapData(t)[crypto.EnvMinVersion], wantMinVersion; got != want {
		t.Errorf("configmap %s = %q after reconcile, want the tampered value reverted to %q",
			crypto.EnvMinVersion, got, want)
	}
	assertPatched(t, h.patchedDeployments(), []string{testDeployment})
	if got := h.storedHash(t, testDeployment); got != hashBefore {
		t.Errorf("stored hash = %q, want it unchanged at %q: the cluster profile did not move, "+
			"only the configmap did", got, hashBefore)
	}
	if h.restartStamp(t, testDeployment) == "" {
		t.Errorf("deployment carries no %s annotation, so re-patching the unchanged hash "+
			"would have been a no-op and the pods would keep the tampered settings",
			client.TLSRestartAnnotation)
	}
}

// TestPublishRecreatesADeletedConfigMap is the same repair seen from the other
// side: the ConfigMap is gone rather than wrong. A pod that started in that
// window has no TLS settings from it at all -- the chart's envFrom is optional
// precisely so that starting without it works -- and again its hash is current,
// so only the restart stamp gets it back onto the cluster profile.
func TestPublishRecreatesADeletedConfigMap(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:          modernProfile(),
		deployments:      []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
		operandConfigMap: testConfigMap,
	})

	ctx := context.Background()
	if err := h.r.reconcileOnce(ctx, triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() unexpected error = %v", err)
	}

	if err := h.kube.CoreV1().ConfigMaps(testNamespace).
		Delete(ctx, testConfigMap, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("failed to delete the configmap: %v", err)
	}
	h.kube.ClearActions()

	if err := h.r.reconcileOnce(ctx, triggerResync); err != nil {
		t.Fatalf("second reconcileOnce() unexpected error = %v", err)
	}

	if got, want := h.configMapData(t)[crypto.EnvMinVersion], wantMinVersion; got != want {
		t.Errorf("configmap %s = %q after reconcile, want it recreated with %q",
			crypto.EnvMinVersion, got, want)
	}
	assertPatched(t, h.patchedDeployments(), []string{testDeployment})
	if h.restartStamp(t, testDeployment) == "" {
		t.Errorf("deployment carries no %s annotation, so the pods that started without the "+
			"configmap would never be restarted", client.TLSRestartAnnotation)
	}
}

// TestSteadyStateLeavesTheRestartStampAlone guards the other direction: the
// stamp is a repair tool, so a cluster where nothing drifts must never acquire
// one. Writing a fresh timestamp on every pass would roll the workloads on
// every resync tick.
func TestSteadyStateLeavesTheRestartStampAlone(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:          modernProfile(),
		deployments:      []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
		operandConfigMap: testConfigMap,
	})

	ctx := context.Background()
	for _, trigger := range []string{triggerStartup, triggerResync, triggerResync} {
		if err := h.r.reconcileOnce(ctx, trigger); err != nil {
			t.Fatalf("reconcileOnce(%s) unexpected error = %v", trigger, err)
		}
	}

	// Only the first pass, which created the ConfigMap and stamped the hash.
	assertPatched(t, h.patchedDeployments(), []string{testDeployment})
	if got := h.restartStamp(t, testDeployment); got != "" {
		t.Errorf("deployment carries %s=%q, want none: nothing drifted, so the hash alone "+
			"was enough to roll it", client.TLSRestartAnnotation, got)
	}
}
