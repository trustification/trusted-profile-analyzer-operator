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
	operatorv1 "github.com/openshift/api/operator/v1"
	operatorclient "github.com/openshift/client-go/operator/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// Client wraps the OpenShift operator client
type Client struct {
	operatorClient operatorclient.Interface
	namespace      string
}

// NewClient creates a new OpenShift client
func NewClient(config *rest.Config, namespace string) (*Client, error) {
	operatorClient, err := operatorclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create operator client: %w", err)
	}

	return &Client{
		operatorClient: operatorClient,
		namespace:      namespace,
	}, nil
}

// GetIngressController retrieves an IngressController by name
func (c *Client) GetIngressController(ctx context.Context, name string) (*operatorv1.IngressController, error) {
	ic, err := c.operatorClient.OperatorV1().IngressControllers(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get IngressController %s/%s: %w", c.namespace, name, err)
	}
	return ic, nil
}

// UpdateIngressController updates an IngressController
func (c *Client) UpdateIngressController(
	ctx context.Context, ic *operatorv1.IngressController,
) (*operatorv1.IngressController, error) {
	updated, err := c.operatorClient.OperatorV1().IngressControllers(c.namespace).Update(ctx, ic, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update IngressController %s/%s: %w", c.namespace, ic.Name, err)
	}
	return updated, nil
}

// ListIngressControllers lists all IngressControllers in the namespace
func (c *Client) ListIngressControllers(ctx context.Context) (*operatorv1.IngressControllerList, error) {
	list, err := c.operatorClient.OperatorV1().IngressControllers(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list IngressControllers in %s: %w", c.namespace, err)
	}
	return list, nil
}

// GetTLSSecurityProfile retrieves the TLS security profile from an IngressController
func (c *Client) GetTLSSecurityProfile(ctx context.Context, name string) (*configv1.TLSSecurityProfile, error) {
	ic, err := c.GetIngressController(ctx, name)
	if err != nil {
		return nil, err
	}

	if ic.Spec.TLSSecurityProfile == nil {
		return nil, fmt.Errorf("IngressController %s/%s has no TLS security profile", c.namespace, name)
	}

	return ic.Spec.TLSSecurityProfile, nil
}

// UpdateTLSSecurityProfile updates the TLS security profile of an IngressController
func (c *Client) UpdateTLSSecurityProfile(
	ctx context.Context, name string, profile *configv1.TLSSecurityProfile,
) error {
	ic, err := c.GetIngressController(ctx, name)
	if err != nil {
		return err
	}

	// Update the TLS security profile
	ic.Spec.TLSSecurityProfile = profile

	_, err = c.UpdateIngressController(ctx, ic)
	if err != nil {
		return fmt.Errorf("failed to update TLS security profile: %w", err)
	}

	return nil
}

// ValidateTLSProfile validates a TLS security profile
func ValidateTLSProfile(profile *configv1.TLSSecurityProfile) error {
	if profile == nil {
		return fmt.Errorf("TLS security profile cannot be nil")
	}

	switch profile.Type {
	case configv1.TLSProfileCustomType:
		if profile.Custom == nil {
			return fmt.Errorf("custom TLS profile type requires custom configuration")
		}
		if err := validateCustomProfile(profile.Custom); err != nil {
			return err
		}
	case configv1.TLSProfileOldType, configv1.TLSProfileIntermediateType, configv1.TLSProfileModernType:
		// These are predefined profiles, no additional validation needed
	default:
		return fmt.Errorf("unknown TLS profile type: %s", profile.Type)
	}

	return nil
}

func validateCustomProfile(custom *configv1.CustomTLSProfile) error {
	if custom == nil {
		return fmt.Errorf("custom profile cannot be nil")
	}

	if len(custom.Ciphers) == 0 {
		return fmt.Errorf("custom profile must specify at least one cipher")
	}

	if custom.MinTLSVersion == "" {
		return fmt.Errorf("custom profile must specify minimum TLS version")
	}

	// Validate TLS version
	validVersions := map[configv1.TLSProtocolVersion]bool{
		configv1.VersionTLS10: true,
		configv1.VersionTLS11: true,
		configv1.VersionTLS12: true,
		configv1.VersionTLS13: true,
	}

	if !validVersions[custom.MinTLSVersion] {
		return fmt.Errorf("invalid minimum TLS version: %s", custom.MinTLSVersion)
	}

	return nil
}
