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
	"fmt"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

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
				// Go ignores Config.CipherSuites for TLS 1.3
				// (golang/go#29349), so leaving it unset is correct.
				if config.CipherSuites != nil {
					t.Errorf("Expected CipherSuites to be left unset for TLS 1.3, got %v", config.CipherSuites)
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
			got, _, err := BuildTLSConfig(tt.profile, Options{})
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkFn != nil {
				tt.checkFn(t, got)
			}
		})
	}
}

// A cluster profile may name ciphers that only OpenSSL-based servers can
// offer. Those must be reported, not rejected: the old converter returned an
// error here and the configurator would have refused to reconcile.
func TestBuildTLSConfigReportsUnsupportedCiphersInsteadOfFailing(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{
			TLSProfileSpec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers: []string{
					tls12CipherAES128,           // supported by Go
					"DHE-RSA-AES128-GCM-SHA256", // OpenSSL only
				},
			},
		},
	}

	cfg, unsupported, err := BuildTLSConfig(profile, Options{})
	if err != nil {
		t.Fatalf("BuildTLSConfig() error = %v, want nil", err)
	}
	if len(cfg.CipherSuites) != 1 || cfg.CipherSuites[0] != tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 {
		t.Errorf("CipherSuites = %v, want only TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", cfg.CipherSuites)
	}
	if len(unsupported) != 1 || unsupported[0] != "DHE-RSA-AES128-GCM-SHA256" {
		t.Errorf("unsupported = %v, want [DHE-RSA-AES128-GCM-SHA256]", unsupported)
	}
}

// The cluster profile says nothing about ALPN, so every server has to set it.
func TestBuildTLSConfigSetsALPN(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}

	cfg, _, err := BuildTLSConfig(profile, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.NextProtos) == 0 {
		t.Error("NextProtos is empty; the cluster profile does not set ALPN, so we must")
	}

	cfg, _, err = BuildTLSConfig(profile, Options{NextProtos: []string{"http/1.1"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != "http/1.1" {
		t.Errorf("NextProtos = %v, want [http/1.1]", cfg.NextProtos)
	}
}

func TestShouldHonorClusterTLSProfile(t *testing.T) {
	tests := []struct {
		adherence configv1.TLSAdherencePolicy
		want      bool
	}{
		{configv1.TLSAdherencePolicyNoOpinion, false},
		{configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly, false},
		{configv1.TLSAdherencePolicyStrictAllComponents, true},
		// Unknown values must honor the profile, for forward compatibility
		// with a stricter policy added later.
		{configv1.TLSAdherencePolicy("SomethingStricterAddedLater"), true},
	}

	for _, tt := range tests {
		t.Run(string(tt.adherence), func(t *testing.T) {
			if got := ShouldHonorClusterTLSProfile(tt.adherence); got != tt.want {
				t.Errorf("ShouldHonorClusterTLSProfile(%q) = %v, want %v", tt.adherence, got, tt.want)
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

// The cluster-wide default profile is not a constant: OpenShift applies
// Intermediate when .spec.tlsSecurityProfile is unset up to 5.0, and Modern
// from 5.1. Resolving an unset profile to Intermediate on 5.1 would describe
// the cluster as weaker than it is -- and publishing that to the operand would
// genuinely drop its listener from TLS 1.3 to TLS 1.2.
func TestClusterDefaultProfileType(t *testing.T) {
	tests := []struct {
		major, minor int
		want         configv1.TLSProfileType
	}{
		{4, 21, configv1.TLSProfileIntermediateType},
		{4, 22, configv1.TLSProfileIntermediateType},
		{4, 99, configv1.TLSProfileIntermediateType},
		{5, 0, configv1.TLSProfileIntermediateType},
		{5, 1, configv1.TLSProfileModernType},
		{5, 2, configv1.TLSProfileModernType},
		{6, 0, configv1.TLSProfileModernType},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d.%d", tt.major, tt.minor), func(t *testing.T) {
			if got := ClusterDefaultProfileType(tt.major, tt.minor); got != tt.want {
				t.Errorf("ClusterDefaultProfileType(%d, %d) = %q, want %q",
					tt.major, tt.minor, got, tt.want)
			}
		})
	}
}

// The 5.1 default has to be TLS 1.3 all the way down to the resolved spec,
// since that is what reaches the operand and the rollout hash.
func TestClusterDefaultProfileTypeResolvesToTLS13From51(t *testing.T) {
	spec, err := ResolveProfileSpecWithDefault(nil, ClusterDefaultProfileType(5, 1))
	if err != nil {
		t.Fatalf("ResolveProfileSpecWithDefault() error = %v", err)
	}
	if spec.MinTLSVersion != configv1.VersionTLS13 {
		t.Errorf("MinTLSVersion = %q, want %q", spec.MinTLSVersion, configv1.VersionTLS13)
	}
}

func TestResolveProfileSpecWithDefault(t *testing.T) {
	modern := *configv1.TLSProfiles[configv1.TLSProfileModernType]
	intermediate := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	old := *configv1.TLSProfiles[configv1.TLSProfileOldType]

	tests := []struct {
		name        string
		profile     *configv1.TLSSecurityProfile
		defaultType configv1.TLSProfileType
		want        configv1.TLSProfileSpec
	}{
		{
			name:        "nil profile takes the supplied default",
			profile:     nil,
			defaultType: configv1.TLSProfileModernType,
			want:        modern,
		},
		{
			name:        "empty type is also no opinion",
			profile:     &configv1.TLSSecurityProfile{},
			defaultType: configv1.TLSProfileModernType,
			want:        modern,
		},
		{
			name:        "no default supplied keeps the upstream fallback",
			profile:     nil,
			defaultType: "",
			want:        intermediate,
		},
		// The whole point of the default is that it only applies where the
		// cluster expressed nothing. A profile the administrator chose is
		// honored as written, which is also why nothing here forces TLS 1.3
		// onto Old, Intermediate or Custom.
		{
			name:        "an explicit Old profile is not overridden by a Modern default",
			profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType},
			defaultType: configv1.TLSProfileModernType,
			want:        old,
		},
		{
			name:        "an explicit Intermediate profile is not overridden either",
			profile:     &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType},
			defaultType: configv1.TLSProfileModernType,
			want:        intermediate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveProfileSpecWithDefault(tt.profile, tt.defaultType)
			if err != nil {
				t.Fatalf("ResolveProfileSpecWithDefault() error = %v", err)
			}
			if got.MinTLSVersion != tt.want.MinTLSVersion {
				t.Errorf("MinTLSVersion = %q, want %q", got.MinTLSVersion, tt.want.MinTLSVersion)
			}
			if len(got.Ciphers) != len(tt.want.Ciphers) {
				t.Errorf("Ciphers = %v, want %v", got.Ciphers, tt.want.Ciphers)
			}
		})
	}
}

// A Custom profile is taken exactly as written, default or not: its
// minTLSVersion is the administrator's and nothing raises it.
func TestResolveProfileSpecWithDefaultKeepsCustomAsWritten(t *testing.T) {
	custom := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{
			TLSProfileSpec: configv1.TLSProfileSpec{
				Ciphers:       []string{tls12CipherAES128},
				MinTLSVersion: configv1.VersionTLS11,
			},
		},
	}

	got, err := ResolveProfileSpecWithDefault(custom, configv1.TLSProfileModernType)
	if err != nil {
		t.Fatalf("ResolveProfileSpecWithDefault() error = %v", err)
	}
	if got.MinTLSVersion != configv1.VersionTLS11 {
		t.Errorf("MinTLSVersion = %q, want the custom profile's own %q",
			got.MinTLSVersion, configv1.VersionTLS11)
	}
}
