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
	ocptls "github.com/openshift/controller-runtime-common/pkg/tls"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/crypto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

// ClusterTLSSettings is the full cluster-wide TLS contract: the resolved
// profile plus the adherence policy that says whether components are required
// to honor it. The two are always read from the same APIServer snapshot so
// they cannot disagree.
type ClusterTLSSettings struct {
	// Profile is the raw .spec.tlsSecurityProfile, nil when unset.
	Profile *configv1.TLSSecurityProfile
	// Spec is Profile resolved to concrete ciphers, groups and minimum
	// version (built-ins expanded, nil/unknown defaulted to Intermediate).
	Spec configv1.TLSProfileSpec
	// Adherence is .spec.tlsAdherence, "" when unset.
	Adherence configv1.TLSAdherencePolicy
}

// ShouldHonor reports whether components must apply this profile, per the
// cluster's tlsAdherence policy.
func (s ClusterTLSSettings) ShouldHonor() bool {
	return crypto.ShouldHonorClusterTLSProfile(s.Adherence)
}

// APIServerClient wraps the OpenShift config client for APIServer resources
type APIServerClient struct {
	configClient configclient.Interface
}

// NewAPIServerClient creates a new APIServer client
func NewAPIServerClient(config *rest.Config) (*APIServerClient, error) {
	configClient, err := configclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create config client: %w", err)
	}

	return &APIServerClient{
		configClient: configClient,
	}, nil
}

// NewAPIServerClientWithClientset creates an APIServer client around an existing
// config clientset. It exists so callers can inject an alternative
// implementation -- notably a fake clientset in tests -- without going through a
// *rest.Config.
func NewAPIServerClientWithClientset(configClient configclient.Interface) *APIServerClient {
	return &APIServerClient{configClient: configClient}
}

// GetAPIServer retrieves the cluster APIServer resource
// The APIServer resource is always named "cluster" in OpenShift
func (c *APIServerClient) GetAPIServer(ctx context.Context) (*configv1.APIServer, error) {
	apiServer, err := c.configClient.ConfigV1().APIServers().Get(ctx, ocptls.APIServerName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get APIServer cluster: %w", err)
	}
	return apiServer, nil
}

// GetClusterTLSSettings reads the profile and the adherence policy from a
// single APIServer snapshot and resolves the profile to a concrete
// TLSProfileSpec. This is the entry point callers should use; the individual
// getters below exist for the CLI's single-value output.
func (c *APIServerClient) GetClusterTLSSettings(ctx context.Context) (ClusterTLSSettings, error) {
	apiServer, err := c.GetAPIServer(ctx)
	if err != nil {
		return ClusterTLSSettings{}, err
	}

	spec, err := crypto.ResolveProfileSpec(apiServer.Spec.TLSSecurityProfile)
	if err != nil {
		return ClusterTLSSettings{}, err
	}

	return ClusterTLSSettings{
		Profile:   apiServer.Spec.TLSSecurityProfile,
		Spec:      spec,
		Adherence: apiServer.Spec.TLSAdherence,
	}, nil
}

// GetTLSAdherencePolicy returns the cluster's .spec.tlsAdherence value, which
// says how strictly components must honor the cluster-wide TLS profile. It is
// "" on clusters where the TLSAdherence feature gate is off.
func (c *APIServerClient) GetTLSAdherencePolicy(ctx context.Context) (configv1.TLSAdherencePolicy, error) {
	apiServer, err := c.GetAPIServer(ctx)
	if err != nil {
		return configv1.TLSAdherencePolicyNoOpinion, err
	}
	return apiServer.Spec.TLSAdherence, nil
}

// WatchAPIServer returns a watch on the cluster APIServer resource so callers
// can react to runtime changes of the cluster-wide TLS security profile.
func (c *APIServerClient) WatchAPIServer(ctx context.Context) (watch.Interface, error) {
	w, err := c.configClient.ConfigV1().APIServers().Watch(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", ocptls.APIServerName).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to watch APIServer cluster: %w", err)
	}
	return w, nil
}

// GetClusterTLSProfile retrieves the cluster-wide TLS security profile
// This is the recommended approach per OpenShift best practices
func (c *APIServerClient) GetClusterTLSProfile(ctx context.Context) (*configv1.TLSSecurityProfile, error) {
	apiServer, err := c.GetAPIServer(ctx)
	if err != nil {
		return nil, err
	}

	// If no TLS profile is specified, return nil (will use default Intermediate)
	if apiServer.Spec.TLSSecurityProfile == nil {
		return nil, nil
	}

	return apiServer.Spec.TLSSecurityProfile, nil
}

// UpdateClusterTLSProfile updates the cluster-wide TLS security profile
func (c *APIServerClient) UpdateClusterTLSProfile(ctx context.Context, profile *configv1.TLSSecurityProfile) error {
	apiServer, err := c.GetAPIServer(ctx)
	if err != nil {
		return err
	}

	// Update the TLS security profile
	apiServer.Spec.TLSSecurityProfile = profile

	_, err = c.configClient.ConfigV1().APIServers().Update(ctx, apiServer, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update cluster TLS security profile: %w", err)
	}

	return nil
}

// GetEffectiveTLSProfile returns the effective TLS profile for the cluster
// If no profile is configured, it returns the default Intermediate profile
func (c *APIServerClient) GetEffectiveTLSProfile(ctx context.Context) (*configv1.TLSSecurityProfile, error) {
	profile, err := c.GetClusterTLSProfile(ctx)
	if err != nil {
		return nil, err
	}

	// If no profile is set, return default Intermediate profile
	if profile == nil {
		return &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileIntermediateType,
		}, nil
	}

	return profile, nil
}
