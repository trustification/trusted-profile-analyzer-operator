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
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const (
	// FeatureGateTLSAdherence gates the APIServer .spec.tlsAdherence field.
	FeatureGateTLSAdherence configv1.FeatureGateName = "TLSAdherence"

	// FeatureGateTLSGroupPreferences gates TLSProfileSpec.Groups. Writing
	// Groups on a cluster without this gate is rejected by the API server.
	FeatureGateTLSGroupPreferences configv1.FeatureGateName = "TLSGroupPreferences"
)

// FeatureGateChecker reports which OpenShift feature gates are enabled on the
// cluster, so callers can avoid writing fields the API server will reject.
type FeatureGateChecker struct {
	configClient configclient.Interface
}

// NewFeatureGateChecker creates a FeatureGateChecker.
func NewFeatureGateChecker(config *rest.Config) (*FeatureGateChecker, error) {
	configClient, err := configclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create config client: %w", err)
	}
	return &FeatureGateChecker{configClient: configClient}, nil
}

// NewFeatureGateCheckerWithClientset creates a FeatureGateChecker around an
// existing config clientset, so tests can inject a fake.
func NewFeatureGateCheckerWithClientset(configClient configclient.Interface) *FeatureGateChecker {
	return &FeatureGateChecker{configClient: configClient}
}

// IsEnabled reports whether the named feature gate is enabled on this cluster.
// It reads the FeatureGate CR "cluster", whose status lists the gates resolved
// for the current cluster version.
func (f *FeatureGateChecker) IsEnabled(ctx context.Context, name configv1.FeatureGateName) (bool, error) {
	gate, err := f.configClient.ConfigV1().FeatureGates().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get FeatureGate cluster: %w", err)
	}

	for _, details := range gate.Status.FeatureGates {
		for _, enabled := range details.Enabled {
			if enabled.Name == name {
				return true, nil
			}
		}
	}

	return false, nil
}
