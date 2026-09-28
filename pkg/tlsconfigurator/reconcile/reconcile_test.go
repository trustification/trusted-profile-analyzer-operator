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
package reconcile

import (
	"regexp"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

// sha256Hex matches the lowercase hex encoding of a 32-byte digest.
var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

func modernProfile() *configv1.TLSSecurityProfile {
	return &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}
}

func intermediateProfile() *configv1.TLSSecurityProfile {
	return &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}
}

func customProfile(minVersion configv1.TLSProtocolVersion, ciphers ...string) *configv1.TLSSecurityProfile {
	return &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{
			TLSProfileSpec: configv1.TLSProfileSpec{
				Ciphers:       ciphers,
				MinTLSVersion: minVersion,
			},
		},
	}
}

func mustHash(t *testing.T, profile *configv1.TLSSecurityProfile, enablePQC bool) string {
	t.Helper()
	hash, err := TLSConfigHash(profile, enablePQC)
	if err != nil {
		t.Fatalf("TLSConfigHash() unexpected error = %v", err)
	}
	return hash
}

func TestTLSConfigHashFormat(t *testing.T) {
	tests := []struct {
		name      string
		profile   *configv1.TLSSecurityProfile
		enablePQC bool
	}{
		{name: "nil profile", profile: nil, enablePQC: false},
		{name: "nil profile with pqc", profile: nil, enablePQC: true},
		{name: "modern", profile: modernProfile(), enablePQC: false},
		{name: "intermediate", profile: intermediateProfile(), enablePQC: false},
		{name: "old", profile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType}},
		{
			name:      "custom",
			profile:   customProfile(configv1.VersionTLS13, "ECDHE-RSA-AES128-GCM-SHA256"),
			enablePQC: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustHash(t, tt.profile, tt.enablePQC)
			if !sha256Hex.MatchString(got) {
				t.Errorf("TLSConfigHash() = %q, want a 64-char lowercase hex digest", got)
			}
		})
	}
}

// The hash is stamped onto a pod template, so an unstable hash would roll the
// workloads on every reconcile.
func TestTLSConfigHashIsDeterministic(t *testing.T) {
	tests := []struct {
		name      string
		profile   func() *configv1.TLSSecurityProfile
		enablePQC bool
	}{
		{name: "nil profile", profile: func() *configv1.TLSSecurityProfile { return nil }},
		{name: "modern", profile: modernProfile},
		{name: "modern with pqc", profile: modernProfile, enablePQC: true},
		{
			name: "custom with multiple ciphers",
			profile: func() *configv1.TLSSecurityProfile {
				return customProfile(configv1.VersionTLS12,
					"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Separately constructed but equal values must hash identically,
			// otherwise the reconciler would roll workloads on every restart.
			first := mustHash(t, tt.profile(), tt.enablePQC)
			for i := 0; i < 3; i++ {
				if got := mustHash(t, tt.profile(), tt.enablePQC); got != first {
					t.Fatalf("TLSConfigHash() = %v on call %d, want stable %v", got, i+2, first)
				}
			}
		})
	}
}

// Toggling PQC must force a rollout even when the cluster profile is unchanged.
func TestTLSConfigHashIncludesPQCFlag(t *testing.T) {
	tests := []struct {
		name    string
		profile *configv1.TLSSecurityProfile
	}{
		{name: "nil profile", profile: nil},
		{name: "modern", profile: modernProfile()},
		{name: "intermediate", profile: intermediateProfile()},
		{name: "custom", profile: customProfile(configv1.VersionTLS13)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withPQC := mustHash(t, tt.profile, true)
			withoutPQC := mustHash(t, tt.profile, false)
			if withPQC == withoutPQC {
				t.Errorf("TLSConfigHash() = %v for both pqc=true and pqc=false, want different hashes",
					withPQC)
			}
		})
	}
}

// Any change the reconciler is meant to react to must move the hash.
func TestTLSConfigHashDistinguishesProfiles(t *testing.T) {
	tests := []struct {
		name string
		a    *configv1.TLSSecurityProfile
		b    *configv1.TLSSecurityProfile
	}{
		{
			name: "nil vs empty profile",
			a:    nil,
			b:    &configv1.TLSSecurityProfile{},
		},
		{
			name: "modern vs intermediate",
			a:    modernProfile(),
			b:    intermediateProfile(),
		},
		{
			name: "intermediate vs old",
			a:    intermediateProfile(),
			b:    &configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType},
		},
		{
			name: "custom differing min TLS version",
			a:    customProfile(configv1.VersionTLS12, "ECDHE-RSA-AES128-GCM-SHA256"),
			b:    customProfile(configv1.VersionTLS13, "ECDHE-RSA-AES128-GCM-SHA256"),
		},
		{
			name: "custom differing ciphers",
			a:    customProfile(configv1.VersionTLS13, "ECDHE-RSA-AES128-GCM-SHA256"),
			b:    customProfile(configv1.VersionTLS13, "ECDHE-RSA-AES256-GCM-SHA384"),
		},
		{
			name: "custom with an extra cipher",
			a:    customProfile(configv1.VersionTLS13, "ECDHE-RSA-AES128-GCM-SHA256"),
			b: customProfile(configv1.VersionTLS13,
				"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"),
		},
		{
			// Cipher order is a preference order, so reordering is a real change.
			name: "custom with reordered ciphers",
			a: customProfile(configv1.VersionTLS13,
				"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"),
			b: customProfile(configv1.VersionTLS13,
				"ECDHE-RSA-AES256-GCM-SHA384", "ECDHE-RSA-AES128-GCM-SHA256"),
		},
		{
			name: "custom vs modern",
			a:    customProfile(configv1.VersionTLS13),
			b:    modernProfile(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashA := mustHash(t, tt.a, false)
			hashB := mustHash(t, tt.b, false)
			if hashA == hashB {
				t.Errorf("TLSConfigHash() = %v for both profiles, want different hashes", hashA)
			}
		})
	}
}

func TestOrNone(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "none"},
		{name: "non-empty", input: "abc123", want: "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := orNone(tt.input); got != tt.want {
				t.Errorf("orNone(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
