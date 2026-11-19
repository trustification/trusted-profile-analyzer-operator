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

func TestEnablePQC(t *testing.T) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	EnablePQC(cfg)

	if len(cfg.CurvePreferences) == 0 || cfg.CurvePreferences[0] != tls.X25519MLKEM768 {
		t.Errorf("EnablePQC CurvePreferences = %v, want X25519MLKEM768 first", cfg.CurvePreferences)
	}
}

// The protocol floor belongs to the cluster profile. A cluster on Old,
// Intermediate or a Custom profile with a lower minimum has decided that
// pre-1.3 clients must still connect, and asking for the hybrid group does not
// revoke that decision -- Go just will not select the group below TLS 1.3.
func TestEnablePQCLeavesTheProfileMinimumVersionAlone(t *testing.T) {
	for _, minVersion := range []uint16{
		tls.VersionTLS10, tls.VersionTLS12, tls.VersionTLS13,
	} {
		t.Run(tls.VersionName(minVersion), func(t *testing.T) {
			cfg := &tls.Config{MinVersion: minVersion}
			EnablePQC(cfg)
			if cfg.MinVersion != minVersion {
				t.Errorf("MinVersion = %s, want it left at %s",
					tls.VersionName(cfg.MinVersion), tls.VersionName(minVersion))
			}
		})
	}
}

// Config.CipherSuites only governs the pre-1.3 handshakes, which are now
// reachable under PQC on a profile that permits them. Clearing it would hand
// those Go's defaults instead of the profile's list.
func TestEnablePQCKeepsTheProfileCiphers(t *testing.T) {
	suites := []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, CipherSuites: suites}
	EnablePQC(cfg)

	if len(cfg.CipherSuites) != len(suites) {
		t.Errorf("CipherSuites = %v, want the profile's list %v kept", cfg.CipherSuites, suites)
	}
}

func TestIsPQCCompliant(t *testing.T) {
	tests := []struct {
		name string
		cfg  *tls.Config
		want bool
	}{
		{
			name: "compliant",
			cfg:  &tls.Config{MinVersion: tls.VersionTLS13, CurvePreferences: PQCCurvePreferences()},
			want: true,
		},
		{
			name: "missing pqc group",
			cfg:  &tls.Config{MinVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{tls.X25519}},
			want: false,
		},
		{
			name: "tls 1.2",
			cfg:  &tls.Config{MinVersion: tls.VersionTLS12, CurvePreferences: PQCCurvePreferences()},
			want: false,
		},
		{
			name: "nil",
			cfg:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reasons := IsPQCCompliant(tt.cfg)
			if got != tt.want {
				t.Errorf("IsPQCCompliant() = %v (reasons=%v), want %v", got, reasons, tt.want)
			}
			if !got && len(reasons) == 0 {
				t.Errorf("expected reasons for non-compliant config")
			}
		})
	}
}

// On a Modern profile the floor is already TLS 1.3, so PQC is fully in effect
// with nothing forced.
func TestBuildTLSConfigWithPQCOnModern(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}

	cfg, _, err := BuildTLSConfig(profile, Options{EnablePQC: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	compliant, reasons := IsPQCCompliant(cfg)
	if !compliant {
		t.Errorf("expected PQC-compliant config, reasons=%v", reasons)
	}
}

// On a profile that permits TLS 1.2 the hybrid group is offered but the floor
// stays where the cluster put it, so the result is deliberately *not*
// PQC-compliant: compliance is the cluster profile's to grant.
func TestBuildTLSConfigWithPQCKeepsALowerProfileFloor(t *testing.T) {
	profiles := map[string]*configv1.TLSSecurityProfile{
		"Old":          {Type: configv1.TLSProfileOldType},
		"Intermediate": {Type: configv1.TLSProfileIntermediateType},
		"Custom": {
			Type: configv1.TLSProfileCustomType,
			Custom: &configv1.CustomTLSProfile{
				TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{tls12CipherAES128},
					MinTLSVersion: configv1.VersionTLS11,
				},
			},
		},
	}

	for profileType, profile := range profiles {
		t.Run(profileType, func(t *testing.T) {
			plain, _, err := BuildTLSConfig(profile, Options{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			cfg, _, err := BuildTLSConfig(profile, Options{EnablePQC: true})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cfg.MinVersion != plain.MinVersion {
				t.Errorf("MinVersion = %s with PQC, want the profile's own %s",
					TLSVersionName(cfg.MinVersion), TLSVersionName(plain.MinVersion))
			}
			if len(cfg.CipherSuites) != len(plain.CipherSuites) {
				t.Errorf("CipherSuites = %v with PQC, want the profile's own %v",
					cfg.CipherSuites, plain.CipherSuites)
			}
			if len(cfg.CurvePreferences) == 0 || cfg.CurvePreferences[0] != tls.X25519MLKEM768 {
				t.Errorf("CurvePreferences = %v, want X25519MLKEM768 first", cfg.CurvePreferences)
			}
			if compliant, _ := IsPQCCompliant(cfg); compliant {
				t.Errorf("a %s profile permits pre-1.3 handshakes, so it must not be "+
					"reported as PQC compliant", profileType)
			}
		})
	}
}

// The built-in profiles shipped by openshift/api already list X25519MLKEM768
// in Groups, so a cluster on Modern is post-quantum without --enable-pqc. The
// old hand-rolled converter dropped Groups entirely and could not see this.
func TestBuiltInProfilesCarryPQCGroupFromTheClusterProfile(t *testing.T) {
	for _, profileType := range []configv1.TLSProfileType{
		configv1.TLSProfileOldType,
		configv1.TLSProfileIntermediateType,
		configv1.TLSProfileModernType,
	} {
		t.Run(string(profileType), func(t *testing.T) {
			cfg, _, err := BuildTLSConfig(&configv1.TLSSecurityProfile{Type: profileType}, Options{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cfg.CurvePreferences) == 0 || cfg.CurvePreferences[0] != tls.X25519MLKEM768 {
				t.Errorf("CurvePreferences = %v, want X25519MLKEM768 first", cfg.CurvePreferences)
			}
		})
	}

	// Only Modern is TLS 1.3-only, so only Modern is fully PQC compliant.
	modern, _, err := BuildTLSConfig(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compliant, reasons := IsPQCCompliant(modern); !compliant {
		t.Errorf("Modern profile should be PQC compliant without --enable-pqc, reasons=%v", reasons)
	}
}

func TestPQCGroups(t *testing.T) {
	groups := PQCGroups()
	if len(groups) == 0 || groups[0] != configv1.TLSGroupX25519MLKEM768 {
		t.Errorf("PQCGroups() = %v, want X25519MLKEM768 first", groups)
	}

	// The API vocabulary and the crypto/tls vocabulary must agree, otherwise
	// a profile written by --action=update would not match what a Go service
	// negotiates.
	curves := PQCCurvePreferences()
	if len(curves) != len(groups) {
		t.Fatalf("PQCGroups() has %d entries, PQCCurvePreferences() has %d", len(groups), len(curves))
	}
	for i := range groups {
		if CurveName(curves[i]) != string(groups[i]) {
			t.Errorf("entry %d: curve %q != group %q", i, CurveName(curves[i]), groups[i])
		}
	}
}
