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
// Package reconcile provides a long-running controller that keeps operator
// workloads in sync with the cluster-wide TLS security profile at runtime.
//
// The cluster APIServer CR ("cluster") is treated as the source of truth for
// both .spec.tlsSecurityProfile and .spec.tlsAdherence. Whenever either
// changes, the reconciler recomputes a hash of the effective (optionally
// post-quantum) TLS configuration and stamps it onto the target Deployments'
// pod template, which triggers a rolling restart so the pods pick up the new
// TLS settings.
//
// This is the "downstream operator for upstream operand" shape from the TLS
// Profile Compliance implementation reference: the operator reads the cluster
// TLS settings through the documented APIs and then reconfigures its operand
// through the operand's own mechanism -- here, a pod-template rollout.
package reconcile

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	ocptls "github.com/openshift/controller-runtime-common/pkg/tls"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/client"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/config"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/crypto"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// Reconcile triggers, used to label log lines so an operator reading the log
// can tell a startup reconcile from one caused by a cluster change or by the
// periodic drift check.
const (
	triggerStartup = "startup"
	triggerEvent   = "apiserver-event"
	triggerResync  = "resync"
)

// Reconciler watches the cluster TLS profile and rolls target workloads.
type Reconciler struct {
	apiServerClient *client.APIServerClient
	workloadsClient *client.WorkloadsClient
	cfg             *config.Config

	// lastAdherence suppresses repeat logging of an unchanged tlsAdherence
	// policy across resync ticks. Only touched from the single Run goroutine.
	lastAdherence *configv1.TLSAdherencePolicy

	// lastSummary is the TLS configuration seen by the previous successful
	// reconcile. It is what a change is logged against: nil means nothing has
	// been observed yet, so the next reconcile logs the full profile instead of
	// a diff. Only touched from the single Run goroutine.
	lastSummary *tlsSummary

	// rollingOut records, per target deployment, that a rollout this reconciler
	// triggered has not been seen to finish yet, so that its completion is
	// reported once. Only touched from the single Run goroutine.
	rollingOut map[string]bool
}

// NewReconciler creates a Reconciler from the application configuration.
func NewReconciler(cfg *config.Config) (*Reconciler, error) {
	if cfg.TargetNamespace == "" {
		return nil, fmt.Errorf("target namespace must be set for reconcile mode")
	}
	if len(cfg.TargetDeployments) == 0 {
		return nil, fmt.Errorf("at least one target deployment must be set for reconcile mode")
	}

	restConfig, err := cfg.GetKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	apiServerClient, err := client.NewAPIServerClient(restConfig)
	if err != nil {
		return nil, err
	}

	workloadsClient, err := client.NewWorkloadsClient(restConfig)
	if err != nil {
		return nil, err
	}

	return &Reconciler{
		apiServerClient: apiServerClient,
		workloadsClient: workloadsClient,
		cfg:             cfg,
	}, nil
}

// Run performs an initial reconcile and then reconciles on every change to the
// cluster APIServer TLS profile, plus a periodic resync to correct drift. It
// blocks until the context is cancelled.
func (r *Reconciler) Run(ctx context.Context) error {
	log.Printf("Starting TLS reconciler (namespace=%s, deployments=%v, pqc=%t, resync=%s, annotation=%s)",
		r.cfg.TargetNamespace, r.cfg.TargetDeployments, r.cfg.EnablePQC, r.cfg.ResyncPeriod,
		client.TLSConfigHashAnnotation)

	if err := r.reconcileOnce(ctx, triggerStartup); err != nil {
		// Do not exit on the initial failure; the watch/resync loop retries.
		log.Printf("[%s] Initial reconcile failed: %v", triggerStartup, err)
	}

	resync := r.cfg.ResyncPeriod
	if resync <= 0 {
		resync = 5 * time.Minute
	}
	ticker := time.NewTicker(resync)
	defer ticker.Stop()

	for {
		w, err := r.apiServerClient.WatchAPIServer(ctx)
		if err != nil {
			log.Printf("Failed to establish watch on APIServer %q, retrying in 5s: %v",
				ocptls.APIServerName, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}
		log.Printf("Watching APIServer %q for TLS profile changes (resync every %s)",
			ocptls.APIServerName, resync)

		if err := r.consumeWatch(ctx, w, ticker); err != nil {
			return err
		}
		// Watch channel closed (server timeout); loop re-establishes it.
		log.Printf("Watch closed by server, re-establishing")
	}
}

// consumeWatch drains a watch until it closes or the context/ticker fires.
// It returns a non-nil error only when the reconciler should stop.
func (r *Reconciler) consumeWatch(ctx context.Context, w watch.Interface, ticker *time.Ticker) error {
	defer w.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.reconcileOnce(ctx, triggerResync); err != nil {
				log.Printf("[%s] Periodic reconcile failed: %v", triggerResync, err)
			}
		case event, ok := <-w.ResultChan():
			if !ok {
				return nil // channel closed; caller re-establishes the watch
			}
			switch event.Type {
			case watch.Added, watch.Modified:
				log.Printf("[%s] Observed APIServer %q %s event, reconciling",
					triggerEvent, ocptls.APIServerName, event.Type)
				if err := r.reconcileOnce(ctx, triggerEvent); err != nil {
					log.Printf("[%s] Reconcile failed: %v", triggerEvent, err)
				}
			case watch.Error:
				log.Printf("Watch error event: %v", event.Object)
				return nil // re-establish the watch
			}
		}
	}
}

// reconcileOnce resolves the desired TLS configuration and ensures every target
// deployment carries the matching hash, rolling out only those that differ.
// trigger names what caused this pass and prefixes the log lines it writes.
func (r *Reconciler) reconcileOnce(ctx context.Context, trigger string) error {
	settings, err := r.apiServerClient.GetClusterTLSSettings(ctx)
	if err != nil {
		return fmt.Errorf("failed to read cluster TLS settings: %w", err)
	}

	r.logAdherence(settings.Adherence)

	hash, err := TLSConfigHash(settings, r.cfg.EnablePQC)
	if err != nil {
		return err
	}

	r.logProfile(trigger, r.summarize(settings, hash))

	var errs []error
	var rolled, upToDate int
	for _, name := range r.cfg.TargetDeployments {
		dep, err := r.workloadsClient.GetDeployment(ctx, r.cfg.TargetNamespace, name)
		if err != nil {
			log.Printf("[%s] Cannot read the TLS hash of deployment %s/%s: %v",
				trigger, r.cfg.TargetNamespace, name, err)
			errs = append(errs, err)
			continue
		}
		current := client.TLSHashOf(dep)
		if current == hash {
			upToDate++
			r.logRolloutProgress(trigger, name, dep)
			continue // already up to date, no rollout
		}
		log.Printf("[%s] TLS config changed for %s/%s (%.12s -> %.12s), rolling out",
			trigger, r.cfg.TargetNamespace, name, orNone(current), hash)
		patched, err := r.workloadsClient.SetDeploymentTLSHash(ctx, r.cfg.TargetNamespace, name, hash)
		if err != nil {
			log.Printf("[%s] Failed to roll out deployment %s/%s: %v",
				trigger, r.cfg.TargetNamespace, name, err)
			errs = append(errs, err)
			continue
		}
		rolled++
		r.markRollingOut(name)
		log.Printf("[%s] Patched %s/%s pod template %s=%.12s (generation %d, %d replica(s)): "+
			"Kubernetes is restarting its pods so they re-read the TLS settings",
			trigger, r.cfg.TargetNamespace, name, client.TLSConfigHashAnnotation, hash,
			patched.Generation, replicaCount(patched.Spec.Replicas))
	}

	if len(errs) > 0 {
		log.Printf("[%s] Reconcile finished with errors: %d rolled out, %d already up to date, %d failed",
			trigger, rolled, upToDate, len(errs))
		return fmt.Errorf("reconcile completed with errors: %v", errs)
	}
	// A quiet resync is the normal case; logging it every tick would bury the
	// passes that actually did something.
	if rolled > 0 || trigger != triggerResync {
		log.Printf("[%s] Reconcile finished: %d rolled out, %d already up to date",
			trigger, rolled, upToDate)
	}
	return nil
}

// markRollingOut records that a rollout was just triggered for a deployment,
// so that logRolloutProgress reports when it lands.
func (r *Reconciler) markRollingOut(name string) {
	if r.rollingOut == nil {
		r.rollingOut = map[string]bool{}
	}
	r.rollingOut[name] = true
}

// logRolloutProgress reports a deployment that already carries the current TLS
// hash. Carrying the hash only means the pod template was patched -- the pods
// may still be restarting -- so this is where the restart itself becomes
// visible: progress while it runs, and one completion line when it lands.
//
// Progress is only followed for rollouts this reconciler triggered. A workload
// that is unhealthy for reasons of its own is not this component's news to
// report on every resync tick.
func (r *Reconciler) logRolloutProgress(trigger, name string, dep *appsv1.Deployment) {
	progress, rolling := rolloutStatus(dep)

	switch {
	case r.rollingOut[name] && rolling:
		log.Printf("[%s] Deployment %s/%s is still restarting pods for the current TLS config (%s)",
			trigger, r.cfg.TargetNamespace, name, progress)
	case r.rollingOut[name]:
		delete(r.rollingOut, name)
		log.Printf("[%s] Deployment %s/%s finished restarting with the current TLS config (%s)",
			trigger, r.cfg.TargetNamespace, name, progress)
	case trigger == triggerStartup:
		log.Printf("[%s] Deployment %s/%s already carries TLS config hash %.12s, no rollout (%s)",
			trigger, r.cfg.TargetNamespace, name, client.TLSHashOf(dep), progress)
	}
}

// rolloutStatus renders a Deployment's replica counts and reports whether its
// pods are still being replaced. A rollout is in flight while any replica is
// not yet updated to the current pod template, or an updated one is not yet
// available.
func rolloutStatus(dep *appsv1.Deployment) (summary string, rolling bool) {
	desired := replicaCount(dep.Spec.Replicas)
	status := dep.Status

	rolling = status.UpdatedReplicas < desired ||
		status.Replicas > status.UpdatedReplicas ||
		status.AvailableReplicas < desired

	return fmt.Sprintf("%d/%d pods updated, %d ready, %d available",
		status.UpdatedReplicas, desired, status.ReadyReplicas, status.AvailableReplicas), rolling
}

// replicaCount renders a Deployment's desired replica count, defaulting the
// unset pointer to the Kubernetes default of 1.
func replicaCount(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}

// logAdherence reports the cluster's tlsAdherence policy once per distinct
// value. The reconciler always propagates the profile -- it is an adhering
// component and has no TLS settings of its own to prefer -- but the policy is
// worth surfacing because it tells operators whether the cluster considers
// adherence mandatory.
func (r *Reconciler) logAdherence(adherence configv1.TLSAdherencePolicy) {
	if r.lastAdherence != nil && *r.lastAdherence == adherence {
		return
	}
	r.lastAdherence = &adherence

	switch {
	case crypto.ShouldHonorClusterTLSProfile(adherence):
		log.Printf("Cluster tlsAdherence=%q: all components are required to honor the cluster TLS profile",
			orUnset(adherence))
	default:
		log.Printf("Cluster tlsAdherence=%q: honoring the cluster TLS profile is optional; "+
			"this operator honors it regardless", orUnset(adherence))
	}
}

// tlsSummary is the human-readable projection of one observation of the
// cluster TLS settings: what the profile resolves to, how it lands in a
// crypto/tls.Config once this reconciler's own options are applied, and the
// hash the workloads are stamped with. It exists purely for logging -- the
// rollout decision is made on hash alone.
type tlsSummary struct {
	hash        string
	profileType string
	adherence   string
	minVersion  string
	ciphers     []string
	groups      []string
	pqc         bool
	pqcReasons  []string
	unsupported []string
}

// summarize resolves settings into the form logProfile reports.
func (r *Reconciler) summarize(settings client.ClusterTLSSettings, hash string) tlsSummary {
	tlsConfig, unsupported := crypto.BuildTLSConfigFromSpec(settings.Spec, crypto.Options{
		EnablePQC: r.cfg.EnablePQC,
	})
	pqc, reasons := crypto.IsPQCCompliant(tlsConfig)

	return tlsSummary{
		hash:        hash,
		profileType: profileTypeName(settings.Profile),
		adherence:   orUnset(settings.Adherence),
		minVersion:  crypto.TLSVersionName(tlsConfig.MinVersion),
		ciphers:     settings.Spec.Ciphers,
		groups:      groupNames(tlsConfig.CurvePreferences),
		pqc:         pqc,
		pqcReasons:  reasons,
		unsupported: unsupported,
	}
}

// logProfile reports what the cluster TLS profile resolves to. The first
// observation (startup, or the first success after a failed start) is logged in
// full so the log says what the pod found; later passes log only what changed,
// so a steady cluster stays quiet.
func (r *Reconciler) logProfile(trigger string, summary tlsSummary) {
	switch {
	case r.lastSummary == nil:
		log.Printf("[%s] Cluster TLS profile: type=%s adherence=%s minVersion=%s pqcRequested=%t hash=%.12s",
			trigger, summary.profileType, summary.adherence, summary.minVersion, r.cfg.EnablePQC, summary.hash)
		log.Printf("[%s] Key-exchange groups: %s", trigger, orNone(strings.Join(summary.groups, ", ")))
		log.Printf("[%s] Ciphers from the profile (%d): %s",
			trigger, len(summary.ciphers), orNone(strings.Join(summary.ciphers, ", ")))
		if summary.pqc {
			log.Printf("[%s] Post-quantum key exchange is active (TLS 1.3 + %s)",
				trigger, configv1.TLSGroupX25519MLKEM768)
		} else {
			log.Printf("[%s] Post-quantum key exchange is NOT active: %s",
				trigger, strings.Join(summary.pqcReasons, "; "))
		}
		if len(summary.unsupported) > 0 {
			log.Printf("[%s] Dropped, not supported by Go's crypto/tls: %s",
				trigger, strings.Join(summary.unsupported, ", "))
		}

	case r.lastSummary.hash != summary.hash:
		log.Printf("[%s] Cluster TLS profile changed (hash %.12s -> %.12s):",
			trigger, r.lastSummary.hash, summary.hash)
		for _, change := range r.lastSummary.diff(summary) {
			log.Printf("[%s]   %s", trigger, change)
		}
		if len(summary.unsupported) > 0 {
			log.Printf("[%s] Dropped, not supported by Go's crypto/tls: %s",
				trigger, strings.Join(summary.unsupported, ", "))
		}

	case trigger != triggerResync:
		// An event that turned out to change nothing relevant: say so, since an
		// operator who just edited the APIServer CR is watching for a reaction.
		log.Printf("[%s] Cluster TLS profile unchanged (type=%s hash=%.12s)",
			trigger, summary.profileType, summary.hash)
	}

	r.lastSummary = &summary
}

// diff lists the field-level changes from s to next, in the order an operator
// is most likely to care about. The hash already told us something changed;
// this says what.
func (s tlsSummary) diff(next tlsSummary) []string {
	var changes []string
	add := func(field, from, to string) {
		if from != to {
			changes = append(changes, fmt.Sprintf("%s: %s -> %s", field, orNone(from), orNone(to)))
		}
	}

	add("profile type", s.profileType, next.profileType)
	add("tlsAdherence", s.adherence, next.adherence)
	add("minimum TLS version", s.minVersion, next.minVersion)
	add("key-exchange groups", strings.Join(s.groups, ", "), strings.Join(next.groups, ", "))
	// Cipher lists are long enough that printing both in full drowns the rest of
	// the diff, so report the counts and what moved.
	if strings.Join(s.ciphers, ",") != strings.Join(next.ciphers, ",") {
		changes = append(changes, fmt.Sprintf("ciphers: %d -> %d%s",
			len(s.ciphers), len(next.ciphers), listDelta(s.ciphers, next.ciphers)))
	}
	if s.pqc != next.pqc {
		add("post-quantum key exchange", pqcState(s.pqc), pqcState(next.pqc))
	}

	if len(changes) == 0 {
		// The hash covers the whole resolved spec, so it can move on a field
		// this summary does not render. Do not claim nothing changed.
		changes = append(changes, "no change in the reported fields; "+
			"the resolved profile differs elsewhere -- run 'tls-configurator --action=get-cluster' for the full spec")
	}
	return changes
}

// listDelta renders what was added to and removed from a list, as a suffix to
// append to a "x -> y" line. Two lists with the same members in a different
// order report as reordered -- cipher order is preference order, so it matters.
func listDelta(from, to []string) string {
	added := missingFrom(to, from)
	removed := missingFrom(from, to)

	var parts []string
	if len(added) > 0 {
		parts = append(parts, "added "+strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, "removed "+strings.Join(removed, ", "))
	}
	if len(parts) == 0 {
		return " (reordered)"
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

// missingFrom returns the entries of list that other does not contain.
func missingFrom(list, other []string) []string {
	have := make(map[string]struct{}, len(other))
	for _, s := range other {
		have[s] = struct{}{}
	}

	var missing []string
	for _, s := range list {
		if _, ok := have[s]; !ok {
			missing = append(missing, s)
		}
	}
	return missing
}

func pqcState(active bool) string {
	if active {
		return "active"
	}
	return "inactive"
}

// profileTypeName names the configured profile type, or reports that none is
// set and the cluster default applies.
func profileTypeName(profile *configv1.TLSSecurityProfile) string {
	if profile == nil || profile.Type == "" {
		return fmt.Sprintf("<unset> (defaults to %s)", configv1.TLSProfileIntermediateType)
	}
	return string(profile.Type)
}

// groupNames renders key-exchange groups in preference order.
func groupNames(curves []tls.CurveID) []string {
	names := make([]string, 0, len(curves))
	for _, c := range curves {
		names = append(names, crypto.CurveName(c))
	}
	return names
}

// TLSConfigHash returns a stable hash of the effective TLS configuration.
//
// It hashes the *resolved* TLSProfileSpec rather than the raw
// TLSSecurityProfile, so that renaming the same settings -- unset to
// "Intermediate", or a Custom profile spelling out exactly what a built-in
// already says -- does not roll the workloads. The adherence policy and the
// PQC flag are part of the hash so that flipping either one does.
func TLSConfigHash(settings client.ClusterTLSSettings, enablePQC bool) (string, error) {
	payload := struct {
		Spec      configv1.TLSProfileSpec     `json:"spec"`
		Adherence configv1.TLSAdherencePolicy `json:"adherence"`
		PQC       bool                        `json:"pqc"`
	}{
		Spec:      settings.Spec,
		Adherence: settings.Adherence,
		PQC:       enablePQC,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal TLS config for hashing: %w", err)
	}

	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

func orUnset(adherence configv1.TLSAdherencePolicy) string {
	if adherence == configv1.TLSAdherencePolicyNoOpinion {
		return "<unset>"
	}
	return string(adherence)
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
