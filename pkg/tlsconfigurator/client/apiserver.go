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
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

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

// GetAPIServer retrieves the cluster APIServer resource
// The APIServer resource is always named "cluster" in OpenShift
func (c *APIServerClient) GetAPIServer(ctx context.Context) (*configv1.APIServer, error) {
	apiServer, err := c.configClient.ConfigV1().APIServers().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get APIServer cluster: %w", err)
	}
	return apiServer, nil
}

// WatchAPIServer returns a watch on the cluster APIServer resource so callers
// can react to runtime changes of the cluster-wide TLS security profile.
func (c *APIServerClient) WatchAPIServer(ctx context.Context) (watch.Interface, error) {
	w, err := c.configClient.ConfigV1().APIServers().Watch(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", "cluster").String(),
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
