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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

const (
	testNamespace = "rhtpa"
	testConfigMap = "tls-configurator-profile"
)

// profileData is a representative published payload.
func profileData() map[string]string {
	return map[string]string{
		"HTTP_SERVER_TLS_SECURITY_PROFILE": "custom",
		"HTTP_SERVER_TLS_MIN_VERSION":      "1.3",
	}
}

// ownedConfigMap is what a previous pass of the reconciler would have left
// behind: the reconciler's own data under its ownership label.
func ownedConfigMap(data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfigMap,
			Namespace: testNamespace,
			Labels:    map[string]string{ManagedByLabel: ManagedByValue},
		},
		Data: data,
	}
}

func newWorkloads(objs ...runtime.Object) (*WorkloadsClient, *k8sfake.Clientset) {
	kube := k8sfake.NewClientset(objs...)
	return NewWorkloadsClientWithClientset(kube), kube
}

func readConfigMap(t *testing.T, kube *k8sfake.Clientset) *corev1.ConfigMap {
	t.Helper()
	cm, err := kube.CoreV1().ConfigMaps(testNamespace).
		Get(context.Background(), testConfigMap, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to read back configmap: %v", err)
	}
	return cm
}

// TestApplyProfileConfigMapCreates covers the first pass: nothing exists, so
// the ConfigMap is created carrying the ownership label every later pass
// checks for.
func TestApplyProfileConfigMapCreates(t *testing.T) {
	w, kube := newWorkloads()

	changed, err := w.ApplyProfileConfigMap(context.Background(), testNamespace, testConfigMap, profileData())
	if err != nil {
		t.Fatalf("ApplyProfileConfigMap() unexpected error = %v", err)
	}
	if !changed {
		t.Error("ApplyProfileConfigMap() changed = false, want true when it created the object")
	}

	cm := readConfigMap(t, kube)
	if got := cm.Labels[ManagedByLabel]; got != ManagedByValue {
		t.Errorf("created configmap %s = %q, want %q: later passes refuse to update an "+
			"object that does not carry it", ManagedByLabel, got, ManagedByValue)
	}
}

// TestApplyProfileConfigMapRefusesForeignObject is the collision guard.
// operandTls.configMapName is user-configurable, so it can name a ConfigMap
// another component owns -- and the update path replaces Data wholesale, which
// would erase it. An object without the ownership label must be left exactly
// as it was.
func TestApplyProfileConfigMapRefusesForeignObject(t *testing.T) {
	foreign := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfigMap,
			Namespace: testNamespace,
			Labels:    map[string]string{"app.kubernetes.io/name": "someone-else"},
		},
		Data: map[string]string{"IMPORTANT": "do not lose this"},
	}
	w, kube := newWorkloads(foreign)

	changed, err := w.ApplyProfileConfigMap(context.Background(), testNamespace, testConfigMap, profileData())
	if err == nil {
		t.Fatal("ApplyProfileConfigMap() = nil, want an error naming the collision")
	}
	if changed {
		t.Error("ApplyProfileConfigMap() changed = true, want false when it wrote nothing")
	}
	if !strings.Contains(err.Error(), ManagedByLabel) {
		t.Errorf("ApplyProfileConfigMap() error = %q, want it to name %s so the operator "+
			"can see what is wrong", err, ManagedByLabel)
	}

	cm := readConfigMap(t, kube)
	if got, want := cm.Data["IMPORTANT"], "do not lose this"; got != want {
		t.Errorf("foreign configmap data = %q, want %q left untouched", got, want)
	}
	if _, published := cm.Data["HTTP_SERVER_TLS_MIN_VERSION"]; published {
		t.Error("the reconciler's settings were written into a configmap it does not own")
	}
}

// TestApplyProfileConfigMapUpdatesOwnObject checks the normal repair path still
// works: an object carrying the ownership label is rewritten to match.
func TestApplyProfileConfigMapUpdatesOwnObject(t *testing.T) {
	w, kube := newWorkloads(ownedConfigMap(map[string]string{
		"HTTP_SERVER_TLS_MIN_VERSION": "1.0", // hand-edited
	}))

	changed, err := w.ApplyProfileConfigMap(context.Background(), testNamespace, testConfigMap, profileData())
	if err != nil {
		t.Fatalf("ApplyProfileConfigMap() unexpected error = %v", err)
	}
	if !changed {
		t.Error("ApplyProfileConfigMap() changed = false, want true when it repaired the data")
	}
	if got, want := readConfigMap(t, kube).Data["HTTP_SERVER_TLS_MIN_VERSION"], "1.3"; got != want {
		t.Errorf("configmap min version = %q, want the tampered value reverted to %q", got, want)
	}
}

// TestApplyProfileConfigMapIsIdempotent pins the no-op case: matching data must
// report changed = false, because the caller treats changed as a reason to roll
// the workloads.
func TestApplyProfileConfigMapIsIdempotent(t *testing.T) {
	w, _ := newWorkloads(ownedConfigMap(profileData()))

	changed, err := w.ApplyProfileConfigMap(context.Background(), testNamespace, testConfigMap, profileData())
	if err != nil {
		t.Fatalf("ApplyProfileConfigMap() unexpected error = %v", err)
	}
	if changed {
		t.Error("ApplyProfileConfigMap() changed = true, want false when the data already matched: " +
			"the caller would force a pointless rollout")
	}
}
