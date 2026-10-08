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
package controller

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

func TestCompareTLSProfiles(t *testing.T) {
	tests := []struct {
		name      string
		profile1  *configv1.TLSSecurityProfile
		profile2  *configv1.TLSSecurityProfile
		different bool
	}{
		{
			name:      "both nil",
			profile1:  nil,
			profile2:  nil,
			different: false,
		},
		{
			name:      "one nil",
			profile1:  &configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType},
			profile2:  nil,
			different: true,
		},
		{
			name: "same custom profiles",
			profile1: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			},
			profile2: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			},
			different: false,
		},
		{
			name: "different TLS versions",
			profile1: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			},
			profile2: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
			different: true,
		},
		{
			name: "different ciphers",
			profile1: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			},
			profile2: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_256_GCM_SHA384"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			},
			different: true,
		},
		{
			name: "different profile types",
			profile1: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
			},
			profile2: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			different: true,
		},
		{
			name: "same intermediate profiles",
			profile1: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			profile2: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			different: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareTLSProfiles(tt.profile1, tt.profile2)
			if result != tt.different {
				t.Errorf("expected different=%v, got %v", tt.different, result)
			}
		})
	}
}

func TestCompareCustomProfiles(t *testing.T) {
	tests := []struct {
		name      string
		custom1   *configv1.CustomTLSProfile
		custom2   *configv1.CustomTLSProfile
		different bool
	}{
		{
			name:      "both nil",
			custom1:   nil,
			custom2:   nil,
			different: false,
		},
		{
			name: "one nil",
			custom1: &configv1.CustomTLSProfile{
				TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
					MinTLSVersion: configv1.VersionTLS13,
				},
			},
			custom2:   nil,
			different: true,
		},
		{
			name: "same profiles",
			custom1: &configv1.CustomTLSProfile{
				TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
					MinTLSVersion: configv1.VersionTLS13,
				},
			},
			custom2: &configv1.CustomTLSProfile{
				TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
					MinTLSVersion: configv1.VersionTLS13,
				},
			},
			different: false,
		},
		{
			name: "different number of ciphers",
			custom1: &configv1.CustomTLSProfile{
				TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
					MinTLSVersion: configv1.VersionTLS13,
				},
			},
			custom2: &configv1.CustomTLSProfile{
				TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384"},
					MinTLSVersion: configv1.VersionTLS13,
				},
			},
			different: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareCustomProfiles(tt.custom1, tt.custom2)
			if result != tt.different {
				t.Errorf("expected different=%v, got %v", tt.different, result)
			}
		})
	}
}
