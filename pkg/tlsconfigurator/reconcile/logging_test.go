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
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// captureLog redirects the standard logger for the duration of a test and
// returns what was written to it.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	buf := &bytes.Buffer{}
	flags := log.Flags()
	log.SetOutput(buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flags)
	})
	return buf
}

func assertLogged(t *testing.T, logged string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(logged, want) {
			t.Errorf("log does not contain %q; got:\n%s", want, logged)
		}
	}
}

func assertNotLogged(t *testing.T, logged string, unwanted ...string) {
	t.Helper()
	for _, bad := range unwanted {
		if strings.Contains(logged, bad) {
			t.Errorf("log unexpectedly contains %q; got:\n%s", bad, logged)
		}
	}
}

// setProfile rewrites the cluster APIServer CR so the next reconcile sees a
// different TLS configuration.
func setProfile(t *testing.T, h *harness, profile *configv1.TLSSecurityProfile,
	adherence configv1.TLSAdherencePolicy) {
	t.Helper()

	ctx := context.Background()
	api, err := h.configClient.ConfigV1().APIServers().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to read the APIServer CR: %v", err)
	}
	api.Spec.TLSSecurityProfile = profile
	api.Spec.TLSAdherence = adherence
	if _, err := h.configClient.ConfigV1().APIServers().Update(ctx, api, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("failed to update the APIServer CR: %v", err)
	}
}

// The first observation must say what the pod found, since that is the only
// record of the cluster state the workloads were stamped from.
func TestReconcileLogsTheResolvedProfileOnStartup(t *testing.T) {
	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType},
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
	})

	if err := h.r.reconcileOnce(context.Background(), triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() error = %v", err)
	}

	assertLogged(t, buf.String(),
		"[startup] Cluster TLS profile: type=Intermediate",
		"minVersion=TLS 1.2",
		"pqcRequested=false",
		"Key-exchange groups: "+string(configv1.TLSGroupX25519MLKEM768),
		"Ciphers from the profile (9):",
		"Post-quantum key exchange is NOT active",
	)
}

// An unset profile is reported as the default it resolves to, not as "<nil>".
func TestReconcileLogsTheDefaultProfileWhenUnset(t *testing.T) {
	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
	})

	if err := h.r.reconcileOnce(context.Background(), triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() error = %v", err)
	}

	assertLogged(t, buf.String(), "type=<unset> (defaults to Intermediate)")
}

func TestReconcileLogsPQCWhenRequested(t *testing.T) {
	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType},
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
		enablePQC:   true,
	})

	if err := h.r.reconcileOnce(context.Background(), triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() error = %v", err)
	}

	assertLogged(t, buf.String(),
		"pqcRequested=true",
		"minVersion=TLS 1.3",
		"Post-quantum key exchange is active",
	)
}

// The rollout is the part an operator correlates with restarting pods, so it
// has to name the deployment, both hashes and the annotation that moved.
func TestReconcileLogsTheRollout(t *testing.T) {
	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, "stale-hash")},
	})

	if err := h.r.reconcileOnce(context.Background(), triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() error = %v", err)
	}

	assertLogged(t, buf.String(),
		"TLS config changed for "+testNamespace+"/"+testDeployment+" (stale-hash ->",
		"Patched "+testNamespace+"/"+testDeployment+" pod template rhtpa.io/tls-config-hash=",
		"restarting its pods",
		"Reconcile finished: 1 rolled out, 0 already up to date",
	)
}

func TestReconcileLogsAnUpToDateDeploymentOnStartup(t *testing.T) {
	modern := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}

	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		profile:     modern,
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, mustHashFor(modern, false))},
	})

	if err := h.r.reconcileOnce(context.Background(), triggerStartup); err != nil {
		t.Fatalf("reconcileOnce() error = %v", err)
	}

	assertLogged(t, buf.String(),
		"already carries TLS config hash",
		"Reconcile finished: 0 rolled out, 1 already up to date",
	)
}

func TestReconcileLogsTheDiffWhenTheProfileChanges(t *testing.T) {
	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType},
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
	})

	ctx := context.Background()
	if err := h.r.reconcileOnce(ctx, triggerStartup); err != nil {
		t.Fatalf("initial reconcileOnce() error = %v", err)
	}

	buf.Reset()
	setProfile(t, h, &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		configv1.TLSAdherencePolicyStrictAllComponents)
	if err := h.r.reconcileOnce(ctx, triggerEvent); err != nil {
		t.Fatalf("second reconcileOnce() error = %v", err)
	}

	assertLogged(t, buf.String(),
		"Cluster TLS profile changed (hash ",
		"profile type: Intermediate -> Modern",
		"tlsAdherence: <unset> -> StrictAllComponents",
		"minimum TLS version: TLS 1.2 -> TLS 1.3",
		"ciphers: 9 -> 3 (removed ",
		"post-quantum key exchange: inactive -> active",
	)
	// The full profile dump belongs to the first observation only.
	assertNotLogged(t, buf.String(), "Ciphers from the profile")
}

// A resync that finds nothing to do must stay silent, or the steady state
// buries the passes that actually did something.
func TestReconcileIsQuietOnAnUnchangedResync(t *testing.T) {
	modern := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}

	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		profile:     modern,
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, mustHashFor(modern, false))},
	})

	ctx := context.Background()
	if err := h.r.reconcileOnce(ctx, triggerStartup); err != nil {
		t.Fatalf("initial reconcileOnce() error = %v", err)
	}

	buf.Reset()
	if err := h.r.reconcileOnce(ctx, triggerResync); err != nil {
		t.Fatalf("resync reconcileOnce() error = %v", err)
	}

	if got := buf.String(); got != "" {
		t.Errorf("unchanged resync logged %q, want silence", got)
	}
}

// An APIServer event that changes nothing relevant is still worth a line: the
// operator who just edited the CR is watching for a reaction.
func TestReconcileReportsAnEventThatChangedNothing(t *testing.T) {
	modern := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}

	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		profile:     modern,
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, mustHashFor(modern, false))},
	})

	ctx := context.Background()
	if err := h.r.reconcileOnce(ctx, triggerStartup); err != nil {
		t.Fatalf("initial reconcileOnce() error = %v", err)
	}

	buf.Reset()
	if err := h.r.reconcileOnce(ctx, triggerEvent); err != nil {
		t.Fatalf("event reconcileOnce() error = %v", err)
	}

	assertLogged(t, buf.String(), "[apiserver-event] Cluster TLS profile unchanged (type=Modern hash=")
}

func TestReconcileLogsTargetFailures(t *testing.T) {
	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, "")},
		targets:     []string{missingDeployment, testDeployment},
	})

	if err := h.r.reconcileOnce(context.Background(), triggerStartup); err == nil {
		t.Fatal("reconcileOnce() error = nil, want an error for the missing deployment")
	}

	assertLogged(t, buf.String(),
		"Cannot read the TLS hash of deployment "+testNamespace+"/"+missingDeployment,
		"Reconcile finished with errors: 1 rolled out, 0 already up to date, 1 failed",
	)
}

// withStatus fills in a deployment's observed replica counts, which is what
// the rollout-progress logging reads.
func withStatus(dep *appsv1.Deployment, updated, ready, available int32) *appsv1.Deployment {
	dep.Status = appsv1.DeploymentStatus{
		Replicas:          updated,
		UpdatedReplicas:   updated,
		ReadyReplicas:     ready,
		AvailableReplicas: available,
	}
	return dep
}

// Patching the pod template only starts the restart; the pass that sees it
// land is the one that can say the pods came back with the new TLS config.
func TestReconcileLogsRolloutProgressAndCompletion(t *testing.T) {
	modern := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}
	hash := mustHashFor(modern, false)

	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		profile:     modern,
		deployments: []*appsv1.Deployment{deploymentWithHash(testDeployment, "stale")},
	})

	ctx := context.Background()
	if err := h.r.reconcileOnce(ctx, triggerStartup); err != nil {
		t.Fatalf("initial reconcileOnce() error = %v", err)
	}

	// Mid-rollout: the new pod exists but is not available yet.
	buf.Reset()
	replaceDeployment(t, h, withStatus(deploymentWithHash(testDeployment, hash), 1, 0, 0))
	if err := h.r.reconcileOnce(ctx, triggerResync); err != nil {
		t.Fatalf("mid-rollout reconcileOnce() error = %v", err)
	}
	assertLogged(t, buf.String(),
		"is still restarting pods for the current TLS config (1/1 pods updated, 0 ready, 0 available)")

	// Rollout landed.
	buf.Reset()
	replaceDeployment(t, h, withStatus(deploymentWithHash(testDeployment, hash), 1, 1, 1))
	if err := h.r.reconcileOnce(ctx, triggerResync); err != nil {
		t.Fatalf("settled reconcileOnce() error = %v", err)
	}
	assertLogged(t, buf.String(), "finished restarting with the current TLS config")

	// And then it goes quiet again.
	buf.Reset()
	if err := h.r.reconcileOnce(ctx, triggerResync); err != nil {
		t.Fatalf("steady reconcileOnce() error = %v", err)
	}
	if got := buf.String(); got != "" {
		t.Errorf("settled resync logged %q, want silence", got)
	}
}

// A workload that is unhealthy for its own reasons, with no rollout from this
// reconciler, is not reported on every resync tick.
func TestReconcileDoesNotFollowRolloutsItDidNotTrigger(t *testing.T) {
	modern := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}

	buf := captureLog(t)
	h := newHarness(t, harnessOpts{
		profile: modern,
		deployments: []*appsv1.Deployment{
			withStatus(deploymentWithHash(testDeployment, mustHashFor(modern, false)), 0, 0, 0),
		},
	})

	ctx := context.Background()
	if err := h.r.reconcileOnce(ctx, triggerStartup); err != nil {
		t.Fatalf("initial reconcileOnce() error = %v", err)
	}

	buf.Reset()
	if err := h.r.reconcileOnce(ctx, triggerResync); err != nil {
		t.Fatalf("resync reconcileOnce() error = %v", err)
	}
	if got := buf.String(); got != "" {
		t.Errorf("resync over an untouched deployment logged %q, want silence", got)
	}
}

// replaceDeployment swaps the stored deployment for one with a new status, the
// way the Deployment controller would as a rollout proceeds.
func replaceDeployment(t *testing.T, h *harness, dep *appsv1.Deployment) {
	t.Helper()
	if _, err := h.kube.AppsV1().Deployments(testNamespace).
		Update(context.Background(), dep, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("failed to update deployment %q: %v", dep.Name, err)
	}
}

// The hash covers the whole resolved spec, so it can move on something the
// summary does not render. The diff must say so rather than print nothing.
func TestSummaryDiffFallsBackWhenNoRenderedFieldChanged(t *testing.T) {
	summary := tlsSummary{hash: "a", profileType: "Custom", minVersion: "TLS 1.3"}
	next := summary
	next.hash = "b"

	changes := summary.diff(next)
	if len(changes) != 1 || !strings.Contains(changes[0], "differs elsewhere") {
		t.Errorf("diff() = %v, want a single 'differs elsewhere' line", changes)
	}
}

func TestListDelta(t *testing.T) {
	tests := []struct {
		name string
		from []string
		to   []string
		want string
	}{
		{name: "added", from: []string{"a"}, to: []string{"a", "b"}, want: " (added b)"},
		{name: "removed", from: []string{"a", "b"}, to: []string{"a"}, want: " (removed b)"},
		{
			name: "added and removed",
			from: []string{"a", "b"},
			to:   []string{"a", "c"},
			want: " (added c; removed b)",
		},
		{name: "reordered", from: []string{"a", "b"}, to: []string{"b", "a"}, want: " (reordered)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := listDelta(tt.from, tt.to); got != tt.want {
				t.Errorf("listDelta(%v, %v) = %q, want %q", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestReplicaCount(t *testing.T) {
	three := int32(3)
	if got := replicaCount(nil); got != 1 {
		t.Errorf("replicaCount(nil) = %d, want 1 (the Kubernetes default)", got)
	}
	if got := replicaCount(&three); got != 3 {
		t.Errorf("replicaCount(&3) = %d, want 3", got)
	}
}
