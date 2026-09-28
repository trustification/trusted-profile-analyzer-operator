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
package crypto

import (
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

func TestTLSVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    uint16
		wantErr bool
	}{
		{
			name:    "TLS 1.0",
			version: "VersionTLS10",
			want:    tls.VersionTLS10,
			wantErr: false,
		},
		{
			name:    "TLS 1.1",
			version: "VersionTLS11",
			want:    tls.VersionTLS11,
			wantErr: false,
		},
		{
			name:    "TLS 1.2",
			version: "VersionTLS12",
			want:    tls.VersionTLS12,
			wantErr: false,
		},
		{
			name:    "TLS 1.3",
			version: "VersionTLS13",
			want:    tls.VersionTLS13,
			wantErr: false,
		},
		{
			name:    "invalid version",
			version: "VersionTLS99",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TLSVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("TLSVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TLSVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertTLSProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile *configv1.TLSSecurityProfile
		wantErr bool
		checkFn func(*testing.T, *tls.Config)
	}{
		{
			name:    "nil profile returns default",
			profile: nil,
			wantErr: false,
			checkFn: func(t *testing.T, config *tls.Config) {
				if config.MinVersion != tls.VersionTLS12 {
					t.Errorf("Expected TLS 1.2, got %d", config.MinVersion)
				}
			},
		},
		{
			name: "Modern profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			wantErr: false,
			checkFn: func(t *testing.T, config *tls.Config) {
				if config.MinVersion != tls.VersionTLS13 {
					t.Errorf("Expected TLS 1.3 for Modern profile, got %d", config.MinVersion)
				}
			},
		},
		{
			name: "Intermediate profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			wantErr: false,
			checkFn: func(t *testing.T, config *tls.Config) {
				if config.MinVersion != tls.VersionTLS12 {
					t.Errorf("Expected TLS 1.2 for Intermediate profile, got %d", config.MinVersion)
				}
			},
		},
		{
			name: "Old profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
			wantErr: false,
			checkFn: func(t *testing.T, config *tls.Config) {
				if config.MinVersion != tls.VersionTLS10 {
					t.Errorf("Expected TLS 1.0 for Old profile, got %d", config.MinVersion)
				}
			},
		},
		{
			name: "Custom profile with TLS 1.3",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS13,
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
					},
				},
			},
			wantErr: false,
			checkFn: func(t *testing.T, config *tls.Config) {
				if config.MinVersion != tls.VersionTLS13 {
					t.Errorf("Expected TLS 1.3, got %d", config.MinVersion)
				}
				if len(config.CipherSuites) == 0 {
					t.Error("Expected cipher suites to be set")
				}
			},
		},
		{
			name: "Custom profile without custom config",
			profile: &configv1.TLSSecurityProfile{
				Type:   configv1.TLSProfileCustomType,
				Custom: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertTLSProfile(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertTLSProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkFn != nil {
				tt.checkFn(t, got)
			}
		})
	}
}

func TestSecureTLSConfig(t *testing.T) {
	config := &tls.Config{}
	SecureTLSConfig(config)

	// SecureTLSConfig applies baseline security settings but doesn't override MinVersion
	// to allow Old profile to use lower TLS versions if explicitly configured
	if config.Renegotiation != tls.RenegotiateNever {
		t.Error("SecureTLSConfig should disable renegotiation")
	}

	if config.SessionTicketsDisabled {
		t.Error("SecureTLSConfig should enable session tickets for performance")
	}
}

func TestGetModernCipherSuites(t *testing.T) {
	suites := GetModernCipherSuites()
	if len(suites) == 0 {
		t.Error("Modern cipher suites should not be empty")
	}

	// Check for TLS 1.3 ciphers
	found := false
	for _, suite := range suites {
		if suite == tls.TLS_AES_128_GCM_SHA256 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Modern profile should include TLS_AES_128_GCM_SHA256")
	}
}

func TestGetIntermediateCipherSuites(t *testing.T) {
	suites := GetIntermediateCipherSuites()
	if len(suites) == 0 {
		t.Error("Intermediate cipher suites should not be empty")
	}

	// Should include both TLS 1.3 and TLS 1.2 ciphers
	has13 := false
	has12 := false

	for _, suite := range suites {
		if suite == tls.TLS_AES_128_GCM_SHA256 {
			has13 = true
		}
		if suite == tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 {
			has12 = true
		}
	}

	if !has13 {
		t.Error("Intermediate profile should include TLS 1.3 ciphers")
	}
	if !has12 {
		t.Error("Intermediate profile should include TLS 1.2 ciphers")
	}
}

func TestDefaultTLSVersion(t *testing.T) {
	version := DefaultTLSVersion()
	if version != tls.VersionTLS12 {
		t.Errorf("DefaultTLSVersion() = %d, want TLS 1.2 (%d)", version, tls.VersionTLS12)
	}
}

func TestCipherSuitesFallback(t *testing.T) {
	tests := []struct {
		name      string
		ianaNames []string
		wantErr   bool
	}{
		{
			name:      "valid TLS 1.3 cipher",
			ianaNames: []string{"TLS_AES_128_GCM_SHA256"},
			wantErr:   false,
		},
		{
			name:      "valid TLS 1.2 cipher",
			ianaNames: []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
			wantErr:   false,
		},
		{
			name:      "multiple valid ciphers",
			ianaNames: []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384"},
			wantErr:   false,
		},
		{
			name:      "invalid cipher",
			ianaNames: []string{"INVALID_CIPHER"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertCipherSuitesFallback(tt.ianaNames)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertCipherSuitesFallback() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != len(tt.ianaNames) {
				t.Errorf("convertCipherSuitesFallback() returned %d suites, want %d", len(got), len(tt.ianaNames))
			}
		})
	}
}
