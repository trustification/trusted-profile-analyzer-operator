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
// The cluster APIServer CR ("cluster") .spec.tlsSecurityProfile is treated as
// the source of truth. Whenever it changes, the reconciler recomputes a hash of
// the effective (optionally post-quantum) TLS configuration and stamps it onto
// the target Deployments' pod template, which triggers a rolling restart so the
// pods pick up the new TLS settings.
package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/client"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/config"
	"k8s.io/apimachinery/pkg/watch"
)

// Reconciler watches the cluster TLS profile and rolls target workloads.
type Reconciler struct {
	apiServerClient *client.APIServerClient
	workloadsClient *client.WorkloadsClient
	cfg             *config.Config
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
	log.Printf("Starting TLS reconciler (namespace=%s, deployments=%v, pqc=%t, resync=%s)",
		r.cfg.TargetNamespace, r.cfg.TargetDeployments, r.cfg.EnablePQC, r.cfg.ResyncPeriod)

	if err := r.reconcileOnce(ctx); err != nil {
		// Do not exit on the initial failure; the watch/resync loop retries.
		log.Printf("Initial reconcile failed: %v", err)
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
			log.Printf("Failed to establish watch, retrying: %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}

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
			if err := r.reconcileOnce(ctx); err != nil {
				log.Printf("Periodic reconcile failed: %v", err)
			}
		case event, ok := <-w.ResultChan():
			if !ok {
				return nil // channel closed; caller re-establishes the watch
			}
			switch event.Type {
			case watch.Added, watch.Modified:
				log.Printf("Observed APIServer %s event, reconciling", event.Type)
				if err := r.reconcileOnce(ctx); err != nil {
					log.Printf("Reconcile failed: %v", err)
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
func (r *Reconciler) reconcileOnce(ctx context.Context) error {
	profile, err := r.apiServerClient.GetEffectiveTLSProfile(ctx)
	if err != nil {
		return fmt.Errorf("failed to read cluster TLS profile: %w", err)
	}

	hash, err := TLSConfigHash(profile, r.cfg.EnablePQC)
	if err != nil {
		return err
	}

	var errs []error
	for _, name := range r.cfg.TargetDeployments {
		current, err := r.workloadsClient.GetDeploymentTLSHash(ctx, r.cfg.TargetNamespace, name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if current == hash {
			continue // already up to date, no rollout
		}
		log.Printf("TLS config changed for %s/%s (%.12s -> %.12s), rolling out",
			r.cfg.TargetNamespace, name, orNone(current), hash)
		if err := r.workloadsClient.SetDeploymentTLSHash(ctx, r.cfg.TargetNamespace, name, hash); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("reconcile completed with errors: %v", errs)
	}
	return nil
}

// TLSConfigHash returns a stable hash of the effective TLS configuration. The
// PQC flag is part of the hash so that toggling post-quantum forces a rollout.
func TLSConfigHash(profile *configv1.TLSSecurityProfile, enablePQC bool) (string, error) {
	payload := struct {
		Profile *configv1.TLSSecurityProfile `json:"profile"`
		PQC     bool                         `json:"pqc"`
	}{
		Profile: profile,
		PQC:     enablePQC,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal TLS config for hashing: %w", err)
	}

	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
