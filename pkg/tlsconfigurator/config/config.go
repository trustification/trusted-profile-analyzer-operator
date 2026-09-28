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
package config

import (
	"fmt"
	"os"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Config represents the application configuration
type Config struct {
	Kubeconfig            string
	IngressControllerName string
	Namespace             string
	TLSProfile            *configv1.TLSSecurityProfile

	// EnablePQC requests post-quantum, TLS 1.3-only key exchange for converted
	// crypto/tls.Config values and for reconciliation.
	EnablePQC bool

	// Runtime reconciliation settings (used by the "reconcile" action).
	TargetNamespace   string        // namespace of the workloads to roll out
	TargetDeployments []string      // deployment names to annotate on TLS change
	ResyncPeriod      time.Duration // periodic drift-correction interval
}

// TLSConfig represents the desired TLS configuration
type TLSConfig struct {
	Type          configv1.TLSProfileType
	Ciphers       []string
	MinTLSVersion configv1.TLSProtocolVersion

	// EnablePQC forces TLS 1.3 and advertises the post-quantum key-exchange
	// group. Note: the pinned OpenShift TLSSecurityProfile API cannot express
	// key-exchange groups, so this only affects converted crypto/tls.Config
	// values and the compliance hash, not the IngressController profile fields.
	EnablePQC bool
}

// NewConfig creates a new Config with default values
func NewConfig() *Config {
	return &Config{
		Kubeconfig:            os.Getenv("KUBECONFIG"),
		IngressControllerName: "default",
		Namespace:             "openshift-ingress-operator",
	}
}

// GetKubeConfig returns a Kubernetes REST config (method)
func (c *Config) GetKubeConfig() (*rest.Config, error) {
	return GetKubeConfig(c.Kubeconfig)
}

// GetKubeConfig returns a Kubernetes REST config from kubeconfig path (standalone function)
func GetKubeConfig(kubeconfigPath string) (*rest.Config, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig from %s: %w", kubeconfigPath, err)
		}
	} else {
		// Try in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load in-cluster config: %w", err)
		}
	}

	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.IngressControllerName == "" {
		return fmt.Errorf("ingress controller name cannot be empty")
	}
	if c.Namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}
	return nil
}

// BuildTLSProfile creates a TLS security profile from TLSConfig
func BuildTLSProfile(tlsConfig *TLSConfig) *configv1.TLSSecurityProfile {
	if tlsConfig == nil {
		return nil
	}

	profile := &configv1.TLSSecurityProfile{
		Type: tlsConfig.Type,
	}

	if tlsConfig.Type == configv1.TLSProfileCustomType {
		customProfile := &configv1.CustomTLSProfile{
			TLSProfileSpec: configv1.TLSProfileSpec{
				Ciphers:       tlsConfig.Ciphers,
				MinTLSVersion: tlsConfig.MinTLSVersion,
			},
		}

		profile.Custom = customProfile
	}

	return profile
}

// GetScheme returns a runtime scheme with OpenShift API types registered
func GetScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := operatorv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add operator scheme: %w", err)
	}
	if err := configv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add config scheme: %w", err)
	}
	return scheme, nil
}
