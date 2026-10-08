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
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
)

func TestNewConfig(t *testing.T) {
	// Set environment variable (t.Setenv restores it automatically)
	t.Setenv("KUBECONFIG", "/tmp/kubeconfig")

	cfg := NewConfig()

	if cfg.Kubeconfig != "/tmp/kubeconfig" {
		t.Errorf("expected kubeconfig to be /tmp/kubeconfig, got %s", cfg.Kubeconfig)
	}

	if cfg.IngressControllerName != "default" {
		t.Errorf("expected default ingress controller name, got %s", cfg.IngressControllerName)
	}

	if cfg.Namespace != "openshift-ingress-operator" {
		t.Errorf("expected openshift-ingress-operator namespace, got %s", cfg.Namespace)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				IngressControllerName: "default",
				Namespace:             "openshift-ingress-operator",
			},
			expectError: false,
		},
		{
			name: "empty ingress controller name",
			config: &Config{
				IngressControllerName: "",
				Namespace:             "openshift-ingress-operator",
			},
			expectError: true,
		},
		{
			name: "empty namespace",
			config: &Config{
				IngressControllerName: "default",
				Namespace:             "",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildTLSProfile(t *testing.T) {
	tests := []struct {
		name      string
		tlsConfig *TLSConfig
		expected  *configv1.TLSSecurityProfile
	}{
		{
			name:      "nil config",
			tlsConfig: nil,
			expected:  nil,
		},
		{
			name: "custom TLS 1.3 profile",
			tlsConfig: &TLSConfig{
				Type:          configv1.TLSProfileCustomType,
				Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
				MinTLSVersion: configv1.VersionTLS13,
			},
			expected: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			},
		},
		{
			name: "custom profile without curves",
			tlsConfig: &TLSConfig{
				Type:          configv1.TLSProfileCustomType,
				Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
				MinTLSVersion: configv1.VersionTLS12,
			},
			expected: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildTLSProfile(tt.tlsConfig)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result.Type != tt.expected.Type {
				t.Errorf("expected type %v, got %v", tt.expected.Type, result.Type)
			}

			if tt.expected.Custom != nil {
				if result.Custom == nil {
					t.Error("expected custom profile but got nil")
					return
				}

				if result.Custom.MinTLSVersion != tt.expected.Custom.MinTLSVersion {
					t.Errorf("expected MinTLSVersion %v, got %v",
						tt.expected.Custom.MinTLSVersion, result.Custom.MinTLSVersion)
				}

				if len(result.Custom.Ciphers) != len(tt.expected.Custom.Ciphers) {
					t.Errorf("expected %d ciphers, got %d",
						len(tt.expected.Custom.Ciphers), len(result.Custom.Ciphers))
				}

			}
		})
	}
}

func TestGetScheme(t *testing.T) {
	scheme, err := GetScheme()
	if err != nil {
		t.Fatalf("failed to get scheme: %v", err)
	}

	if scheme == nil {
		t.Error("expected scheme to not be nil")
	}

	// Verify that OpenShift operator types are registered
	gvk := operatorv1.GroupVersion.WithKind("IngressController")
	if !scheme.Recognizes(gvk) {
		t.Error("scheme does not recognize IngressController type")
	}
}
