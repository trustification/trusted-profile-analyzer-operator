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
	"time"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/client"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/config"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	testNamespace = "rhtpa"
	// eventBuffer lets tests queue watch events without a reader, so the loop
	// tests stay synchronous.
	eventBuffer = 8
	// waitTimeout bounds the goroutine-driven tests (Run, ticker resync).
	waitTimeout = 5 * time.Second
)

func apiServerCR(profile *configv1.TLSSecurityProfile) *configv1.APIServer {
	return &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       configv1.APIServerSpec{TLSSecurityProfile: profile},
	}
}

func deploymentWithHash(name, hash string) *appsv1.Deployment {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
	}
	if hash != "" {
		dep.Spec.Template.Annotations = map[string]string{client.TLSConfigHashAnnotation: hash}
	}
	return dep
}

type harnessOpts struct {
	profile     *configv1.TLSSecurityProfile
	noAPIServer bool
	deployments []*appsv1.Deployment
	// targets defaults to the names of deployments; set explicitly to reference
	// deployments that do not exist.
	targets   []string
	enablePQC bool
	resync    time.Duration
	watchErr  error
}

type harness struct {
	r            *Reconciler
	configClient *configfake.Clientset
	kube         *k8sfake.Clientset
	watcher      *watch.FakeWatcher
}

func newHarness(t *testing.T, o harnessOpts) *harness {
	t.Helper()

	var configObjs []runtime.Object
	if !o.noAPIServer {
		configObjs = append(configObjs, apiServerCR(o.profile))
	}
	configClient := configfake.NewClientset(configObjs...)

	w := watch.NewFakeWithChanSize(eventBuffer, false)
	configClient.PrependWatchReactor("apiservers", k8stesting.DefaultWatchReactor(w, o.watchErr))

	kubeObjs := make([]runtime.Object, 0, len(o.deployments))
	targets := o.targets
	for _, dep := range o.deployments {
		kubeObjs = append(kubeObjs, dep)
		if o.targets == nil {
			targets = append(targets, dep.Name)
		}
	}
	kube := k8sfake.NewClientset(kubeObjs...)

	resync := o.resync
	if resync == 0 {
		// Long enough that the resync ticker never fires unless a test asks it to.
		resync = time.Hour
	}

	h := &harness{
		configClient: configClient,
		kube:         kube,
		watcher:      w,
		r: &Reconciler{
			apiServerClient: client.NewAPIServerClientWithClientset(configClient),
			workloadsClient: client.NewWorkloadsClientWithClientset(kube),
			cfg: &config.Config{
				TargetNamespace:   testNamespace,
				TargetDeployments: targets,
				EnablePQC:         o.enablePQC,
				ResyncPeriod:      resync,
			},
		},
	}
	// Drop the actions generated while seeding the tracker.
	kube.ClearActions()
	return h
}

// patchedDeployments returns the names of deployments patched so far, in order.
func (h *harness) patchedDeployments() []string {
	var names []string
	for _, a := range h.kube.Actions() {
		if a.GetVerb() == "patch" && a.GetResource().Resource == "deployments" {
			names = append(names, a.(k8stesting.PatchAction).GetName())
		}
	}
	return names
}

func (h *harness) storedHash(t *testing.T, name string) string {
	t.Helper()
	dep, err := h.kube.AppsV1().Deployments(testNamespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to read back deployment %q: %v", name, err)
	}
	return dep.Spec.Template.Annotations[client.TLSConfigHashAnnotation]
}

func waitFor(t *testing.T, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", waitTimeout, desc)
}

func TestNewReconcilerRejectsIncompleteConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
	}{
		{
			name: "missing target namespace",
			cfg:  &config.Config{TargetDeployments: []string{"server"}},
		},
		{
			name: "missing target deployments",
			cfg:  &config.Config{TargetNamespace: testNamespace},
		},
		{
			name: "empty target deployments",
			cfg:  &config.Config{TargetNamespace: testNamespace, TargetDeployments: []string{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validation must happen before any cluster access, otherwise this
			// would fail on kubeconfig resolution instead.
			if _, err := NewReconciler(tt.cfg); err == nil {
				t.Fatal("NewReconciler() error = nil, want a validation error")
			}
		})
	}
}

func TestReconcileOnce(t *testing.T) {
	modern := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}

	tests := []struct {
		name        string
		opts        harnessOpts
		wantErr     bool
		wantPatched []string
		// wantHashOf names a deployment whose stored hash must equal the hash of
		// the profile the reconciler should have resolved.
		wantHashOf     string
		wantHashSource *configv1.TLSSecurityProfile
		wantPQC        bool
	}{
		{
			name: "stamps an unannotated deployment",
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "")},
			},
			wantPatched:    []string{"server"},
			wantHashOf:     "server",
			wantHashSource: modern,
		},
		{
			name: "rewrites a stale hash",
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "stale")},
			},
			wantPatched:    []string{"server"},
			wantHashOf:     "server",
			wantHashSource: modern,
		},
		{
			name: "leaves an up-to-date deployment alone",
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", mustHashFor(modern, false))},
			},
			wantPatched: nil,
		},
		{
			name: "patches only the drifted deployment",
			opts: harnessOpts{
				profile: modern,
				deployments: []*appsv1.Deployment{
					deploymentWithHash("server", mustHashFor(modern, false)),
					deploymentWithHash("importer", "stale"),
				},
			},
			wantPatched: []string{"importer"},
		},
		{
			name: "pqc changes the stamped hash",
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", mustHashFor(modern, false))},
				enablePQC:   true,
			},
			wantPatched:    []string{"server"},
			wantHashOf:     "server",
			wantHashSource: modern,
			wantPQC:        true,
		},
		{
			name: "defaults to Intermediate when the cluster sets no profile",
			opts: harnessOpts{
				profile:     nil,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "")},
			},
			wantPatched:    []string{"server"},
			wantHashOf:     "server",
			wantHashSource: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType},
		},
		{
			name: "errors when the APIServer CR is missing",
			opts: harnessOpts{
				noAPIServer: true,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "")},
			},
			wantErr:     true,
			wantPatched: nil,
		},
		{
			name: "errors when a target deployment does not exist",
			opts: harnessOpts{
				profile: modern,
				targets: []string{"ghost"},
			},
			wantErr:     true,
			wantPatched: nil,
		},
		{
			// One bad target must not stop the others from being reconciled.
			name: "reports an error but still patches healthy targets",
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "")},
				targets:     []string{"ghost", "server"},
			},
			wantErr:        true,
			wantPatched:    []string{"server"},
			wantHashOf:     "server",
			wantHashSource: modern,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.opts)

			err := h.r.reconcileOnce(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("reconcileOnce() error = %v, wantErr %v", err, tt.wantErr)
			}

			assertPatched(t, h.patchedDeployments(), tt.wantPatched)

			if tt.wantHashOf != "" {
				want := mustHashFor(tt.wantHashSource, tt.wantPQC)
				if got := h.storedHash(t, tt.wantHashOf); got != want {
					t.Errorf("stored hash on %q = %q, want %q", tt.wantHashOf, got, want)
				}
			}
		})
	}
}

// A second pass over unchanged state must not roll the workload again.
func TestReconcileOnceIsIdempotent(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		deployments: []*appsv1.Deployment{deploymentWithHash("server", "")},
	})

	for i := 0; i < 3; i++ {
		if err := h.r.reconcileOnce(context.Background()); err != nil {
			t.Fatalf("reconcileOnce() call %d error = %v", i+1, err)
		}
	}

	assertPatched(t, h.patchedDeployments(), []string{"server"})
}

func TestConsumeWatch(t *testing.T) {
	modern := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}

	tests := []struct {
		name string
		// queue buffers events before consumeWatch runs; the watcher is then
		// closed so the loop drains, observes the closed channel and returns.
		queue       func(w *watch.FakeWatcher)
		opts        harnessOpts
		wantPatched []string
	}{
		{
			name: "modified event triggers a reconcile",
			queue: func(w *watch.FakeWatcher) {
				w.Modify(apiServerCR(modern))
			},
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "stale")},
			},
			wantPatched: []string{"server"},
		},
		{
			name: "added event triggers a reconcile",
			queue: func(w *watch.FakeWatcher) {
				w.Add(apiServerCR(modern))
			},
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "stale")},
			},
			wantPatched: []string{"server"},
		},
		{
			name: "deleted event is ignored",
			queue: func(w *watch.FakeWatcher) {
				w.Delete(apiServerCR(modern))
			},
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "stale")},
			},
			wantPatched: nil,
		},
		{
			// An Error event must abandon the watch immediately so the caller
			// re-establishes it; the queued Modify must not be processed.
			name: "error event returns before later events",
			queue: func(w *watch.FakeWatcher) {
				w.Error(&metav1.Status{Reason: metav1.StatusReasonExpired})
				w.Modify(apiServerCR(modern))
			},
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "stale")},
			},
			wantPatched: nil,
		},
		{
			name:  "closed channel returns without reconciling",
			queue: func(w *watch.FakeWatcher) {},
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "stale")},
			},
			wantPatched: nil,
		},
		{
			// A failing reconcile is logged, not fatal: the second event is
			// still processed and the healthy target still rolls.
			name: "keeps consuming after a reconcile error",
			queue: func(w *watch.FakeWatcher) {
				w.Modify(apiServerCR(modern))
				w.Modify(apiServerCR(modern))
			},
			opts: harnessOpts{
				profile:     modern,
				deployments: []*appsv1.Deployment{deploymentWithHash("server", "stale")},
				targets:     []string{"ghost", "server"},
			},
			wantPatched: []string{"server"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.opts)
			tt.queue(h.watcher)
			// Closing after queueing keeps the test synchronous: a closed
			// buffered channel still drains its pending events first.
			h.watcher.Stop()

			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()

			if err := h.r.consumeWatch(context.Background(), h.watcher, ticker); err != nil {
				t.Fatalf("consumeWatch() error = %v, want nil so the caller re-establishes the watch", err)
			}

			assertPatched(t, h.patchedDeployments(), tt.wantPatched)
		})
	}
}

func TestConsumeWatchReturnsOnContextCancel(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		deployments: []*appsv1.Deployment{deploymentWithHash("server", "stale")},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	// The watcher stays open and empty, so ctx.Done is the only ready case.
	err := h.r.consumeWatch(ctx, h.watcher, ticker)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("consumeWatch() error = %v, want context.Canceled", err)
	}
	if got := h.patchedDeployments(); len(got) != 0 {
		t.Errorf("patched %v, want no rollout on a cancelled context", got)
	}
	if !h.watcher.IsStopped() {
		t.Error("consumeWatch() did not stop the watch on return")
	}
}

func TestConsumeWatchReconcilesOnResyncTick(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		deployments: []*appsv1.Deployment{deploymentWithHash("server", "stale")},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// No watch events: the rollout below can only come from the ticker.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	done := make(chan error, 1)
	go func() { done <- h.r.consumeWatch(ctx, h.watcher, ticker) }()

	waitFor(t, "the resync ticker to drive a rollout", func() bool {
		return len(h.patchedDeployments()) > 0
	})

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("consumeWatch() error = %v, want context.Canceled", err)
	}
}

func TestRunReconcilesOnStartup(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		deployments: []*appsv1.Deployment{deploymentWithHash("server", "")},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- h.r.Run(ctx) }()

	// The startup reconcile covers what the old pre-install hook Job did, so it
	// must happen without any watch event arriving.
	waitFor(t, "the startup reconcile to roll the deployment", func() bool {
		return len(h.patchedDeployments()) > 0
	})

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunRollsOnProfileChange(t *testing.T) {
	modern := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}
	old := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType}

	h := newHarness(t, harnessOpts{
		profile:     modern,
		deployments: []*appsv1.Deployment{deploymentWithHash("server", "")},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- h.r.Run(ctx) }()

	waitFor(t, "the startup reconcile", func() bool {
		return len(h.patchedDeployments()) == 1
	})
	if got, want := h.storedHash(t, "server"), mustHashFor(modern, false); got != want {
		t.Fatalf("hash after startup = %q, want %q", got, want)
	}

	// Change the cluster-wide profile, then announce it on the watch.
	updated := apiServerCR(old)
	if _, err := h.configClient.ConfigV1().APIServers().Update(
		ctx, updated, metav1.UpdateOptions{},
	); err != nil {
		t.Fatalf("failed to update the APIServer CR: %v", err)
	}
	h.watcher.Modify(updated)

	waitFor(t, "the watch event to drive a second rollout", func() bool {
		return len(h.patchedDeployments()) == 2
	})
	if got, want := h.storedHash(t, "server"), mustHashFor(old, false); got != want {
		t.Errorf("hash after profile change = %q, want %q", got, want)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

// A cluster that is not ready yet must not kill the reconciler on startup.
func TestRunSurvivesInitialReconcileFailure(t *testing.T) {
	h := newHarness(t, harnessOpts{
		noAPIServer: true,
		deployments: []*appsv1.Deployment{deploymentWithHash("server", "")},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- h.r.Run(ctx) }()

	// The startup reconcile fails, but Run must reach the watch loop and keep
	// going until the context is cancelled.
	waitFor(t, "the reconciler to establish its watch", func() bool {
		for _, a := range h.configClient.Actions() {
			if a.GetVerb() == "watch" {
				return true
			}
		}
		return false
	})

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := h.patchedDeployments(); len(got) != 0 {
		t.Errorf("patched %v, want no rollout when the profile cannot be read", got)
	}
}

func TestRunRetriesWhenTheWatchCannotBeEstablished(t *testing.T) {
	h := newHarness(t, harnessOpts{
		profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		deployments: []*appsv1.Deployment{deploymentWithHash("server", "")},
		watchErr:    errors.New("watch refused"),
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- h.r.Run(ctx) }()

	// The startup reconcile still runs even though the watch cannot be set up.
	waitFor(t, "the startup reconcile", func() bool {
		return len(h.patchedDeployments()) > 0
	})

	// Run is now in its retry backoff; cancelling must unblock it rather than
	// leaving it to sleep out the full interval.
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func assertPatched(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("patched deployments = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("patched deployments = %v, want %v", got, want)
		}
	}
}

// mustHashFor is the table-friendly form of TLSConfigHash; the inputs are all
// static, so a failure here is a programming error in the test.
func mustHashFor(profile *configv1.TLSSecurityProfile, enablePQC bool) string {
	hash, err := TLSConfigHash(profile, enablePQC)
	if err != nil {
		panic(err)
	}
	return hash
}
