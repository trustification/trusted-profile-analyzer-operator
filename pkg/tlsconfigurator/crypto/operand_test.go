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
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

// tls13SuiteAES128 is one of the three TLS 1.3 ciphersuites every built-in
// OpenShift profile lists, used here as the marker that the suites made it
// through the split intact.
const tls13SuiteAES128 = "TLS_AES_128_GCM_SHA256"

// tls12CipherAES128 is a representative cipher for TLS 1.2 and below: an
// OpenSSL-spelled name, which is how the profile distinguishes them from the
// IANA-spelled TLS 1.3 suites.
const tls12CipherAES128 = "ECDHE-RSA-AES128-GCM-SHA256"

// TestOperandEnvBuiltInProfiles checks the three built-in profiles end to end,
// because they are what almost every cluster actually runs.
func TestOperandEnvBuiltInProfiles(t *testing.T) {
	tests := []struct {
		profileType    configv1.TLSProfileType
		wantMinVersion string
		// wantCiphers is whether a TLS 1.2 cipher list should be emitted at all.
		wantCiphers bool
	}{
		{configv1.TLSProfileOldType, "1.0", true},
		{configv1.TLSProfileIntermediateType, minVersionTLS12, true},
		// Modern is TLS 1.3-only, so the 1.2 list would never be negotiated.
		{configv1.TLSProfileModernType, minVersionTLS13, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.profileType), func(t *testing.T) {
			spec := *configv1.TLSProfiles[tt.profileType]

			env, dropped := OperandEnv(spec, Options{})
			if len(dropped) != 0 {
				t.Errorf("OperandEnv() dropped %v, want nothing dropped for a built-in profile", dropped)
			}

			if got := env[EnvProfile]; got != "custom" {
				t.Errorf("%s = %q, want \"custom\": the built-in trustd profiles ignore "+
					"the cipher and version settings", EnvProfile, got)
			}
			if got := env[EnvMinVersion]; got != tt.wantMinVersion {
				t.Errorf("%s = %q, want %q", EnvMinVersion, got, tt.wantMinVersion)
			}

			if _, ok := env[EnvCiphers]; ok != tt.wantCiphers {
				t.Errorf("%s present = %t, want %t", EnvCiphers, ok, tt.wantCiphers)
			}

			// Every built-in profile lists the three TLS 1.3 suites.
			suites := env[EnvCipherSuites]
			if !strings.Contains(suites, tls13SuiteAES128) {
				t.Errorf("%s = %q, want it to carry the profile's TLS 1.3 suites",
					EnvCipherSuites, suites)
			}
			// ... and those must not leak into the TLS 1.2 list, which OpenSSL
			// parses with a different grammar and would reject.
			if ciphers := env[EnvCiphers]; strings.Contains(ciphers, "TLS_") {
				t.Errorf("%s = %q, want no TLS 1.3 suite names in the TLS 1.2 cipher list",
					EnvCiphers, ciphers)
			}

			// All three built-ins lead with the hybrid PQC group.
			if got := env[EnvGroups]; !strings.HasPrefix(got, "X25519MLKEM768:") {
				t.Errorf("%s = %q, want it to lead with X25519MLKEM768", EnvGroups, got)
			}
		})
	}
}

// TestOperandEnvPQCOverridesProfile checks that --enable-pqc forces TLS 1.3 and
// the hybrid group even when the cluster profile asks for neither.
func TestOperandEnvPQCOverridesProfile(t *testing.T) {
	spec := configv1.TLSProfileSpec{
		Ciphers:       []string{tls13SuiteAES128, tls12CipherAES128},
		Groups:        []configv1.TLSGroup{configv1.TLSGroupSecP256r1},
		MinTLSVersion: configv1.VersionTLS10,
	}

	env, dropped := OperandEnv(spec, Options{EnablePQC: true})
	if len(dropped) != 0 {
		t.Errorf("OperandEnv() dropped %v, want nothing dropped", dropped)
	}

	if got := env[EnvMinVersion]; got != minVersionTLS13 {
		t.Errorf("%s = %q, want \"1.3\": hybrid key exchange is only defined for TLS 1.3",
			EnvMinVersion, got)
	}
	if got := env[EnvGroups]; got != "X25519MLKEM768:X25519" {
		t.Errorf("%s = %q, want the profile's groups replaced by the PQC list", EnvGroups, got)
	}
	if got, ok := env[EnvCiphers]; ok {
		t.Errorf("%s = %q, want it omitted: a TLS 1.3-only server never negotiates these",
			EnvCiphers, got)
	}
}

// TestOperandEnvOmitsEmptyLists is the important safety property: handing
// OpenSSL an empty ciphersuite list disables every TLS 1.3 suite, which leaves
// a TLS 1.3-only server unable to complete a handshake at all. "The profile
// says nothing" has to mean "leave the default alone".
func TestOperandEnvOmitsEmptyLists(t *testing.T) {
	spec := configv1.TLSProfileSpec{MinTLSVersion: configv1.VersionTLS12}

	env, _ := OperandEnv(spec, Options{})

	for _, key := range []string{EnvCiphers, EnvCipherSuites, EnvGroups} {
		if got, ok := env[key]; ok {
			t.Errorf("%s = %q, want the key omitted entirely when the profile lists none", key, got)
		}
	}
	// The two that are always meaningful are still there.
	if env[EnvProfile] == "" || env[EnvMinVersion] == "" {
		t.Errorf("OperandEnv() = %v, want the profile and min version always set", env)
	}
}

// TestOperandEnvNoCipherBelowTLS13RaisesTheFloor covers the case where
// omitting a cipher variable would widen the listener rather than leave it
// alone: the profile names ciphers, but every one of them is a TLS 1.3 suite,
// while the minimum version still permits TLS 1.2.
//
// Omitting HTTP_SERVER_TLS_CIPHERS there hands TLS 1.2 OpenSSL's default
// cipher list -- suites the profile never allowed. An empty list cannot be
// published instead, because OpenSSL rejects a cipher list that selects
// nothing and trustd would fail to start, so the minimum version is raised to
// 1.3 to say the same thing enforceably.
func TestOperandEnvNoCipherBelowTLS13RaisesTheFloor(t *testing.T) {
	spec := configv1.TLSProfileSpec{
		Ciphers:       []string{tls13SuiteAES128, "TLS_AES_256_GCM_SHA384"},
		MinTLSVersion: configv1.VersionTLS12,
	}

	env, dropped := OperandEnv(spec, Options{})

	if got := env[EnvMinVersion]; got != minVersionTLS13 {
		t.Errorf("%s = %q, want \"1.3\": TLS 1.2 is permitted by the profile but has "+
			"no cipher it could negotiate", EnvMinVersion, got)
	}
	if got, ok := env[EnvCiphers]; ok {
		t.Errorf("%s = %q, want it omitted once the floor is TLS 1.3", EnvCiphers, got)
	}
	if got, want := env[EnvCipherSuites], tls13SuiteAES128+":TLS_AES_256_GCM_SHA384"; got != want {
		t.Errorf("%s = %q, want %q", EnvCipherSuites, got, want)
	}
	if len(dropped) != 1 || !strings.Contains(dropped[0], "raised") {
		t.Errorf("OperandEnv() dropped = %v, want one entry reporting the raised minimum", dropped)
	}
}

// TestOperandEnvNoCiphersAtAllKeepsTheFloor is the other side of that rule. A
// profile naming no ciphers expresses no opinion about them, so OpenSSL's
// defaults are the specified behaviour and the minimum version must be left
// where the profile put it.
func TestOperandEnvNoCiphersAtAllKeepsTheFloor(t *testing.T) {
	spec := configv1.TLSProfileSpec{MinTLSVersion: configv1.VersionTLS12}

	env, dropped := OperandEnv(spec, Options{})

	if got := env[EnvMinVersion]; got != minVersionTLS12 {
		t.Errorf("%s = %q, want \"1.2\" left untouched when the profile names no ciphers",
			EnvMinVersion, got)
	}
	if len(dropped) != 0 {
		t.Errorf("OperandEnv() dropped = %v, want nothing reported", dropped)
	}
}

// TestOperandEnvMissingTLS13SuitesIsNotRestricted pins the deliberate
// asymmetry. A profile that lists only pre-1.3 ciphers leaves the TLS 1.3
// suites to OpenSSL's defaults, which is what the OpenShift API specifies:
// "TLS 1.3 cipher suites are not configurable and are always enabled when TLS
// 1.3 is negotiated". Publishing an empty ciphersuite list to "close" this
// would disable TLS 1.3 on a profile that permits it.
func TestOperandEnvMissingTLS13SuitesIsNotRestricted(t *testing.T) {
	spec := configv1.TLSProfileSpec{
		Ciphers:       []string{tls12CipherAES128},
		MinTLSVersion: configv1.VersionTLS12,
	}

	env, dropped := OperandEnv(spec, Options{})

	if got, ok := env[EnvCipherSuites]; ok {
		t.Errorf("%s = %q, want it omitted so OpenSSL keeps all TLS 1.3 suites enabled",
			EnvCipherSuites, got)
	}
	if got := env[EnvMinVersion]; got != minVersionTLS12 {
		t.Errorf("%s = %q, want \"1.2\": the pre-1.3 ciphers are enforceable, so there is "+
			"no reason to raise the floor", EnvMinVersion, got)
	}
	if got, want := env[EnvCiphers], tls12CipherAES128; got != want {
		t.Errorf("%s = %q, want %q", EnvCiphers, got, want)
	}
	if len(dropped) != 0 {
		t.Errorf("OperandEnv() dropped = %v, want nothing reported", dropped)
	}
}

// TestOperandEnvDropsUnknownGroup covers the reason the group map is an
// allowlist: OpenSSL rejects a whole group list containing one name it does not
// know, and trustd treats that as a fatal startup error. A group this build
// cannot name must be dropped, not passed through.
func TestOperandEnvDropsUnknownGroup(t *testing.T) {
	spec := configv1.TLSProfileSpec{
		Groups: []configv1.TLSGroup{
			configv1.TLSGroupX25519,
			configv1.TLSGroup("MLKEM4096-FromTheFuture"),
			configv1.TLSGroupSecP256r1,
		},
		MinTLSVersion: configv1.VersionTLS12,
	}

	env, dropped := OperandEnv(spec, Options{})

	if got, want := env[EnvGroups], "X25519:secp256r1"; got != want {
		t.Errorf("%s = %q, want %q: the unknown group dropped, the rest kept in order",
			EnvGroups, got, want)
	}
	if len(dropped) != 1 || !strings.Contains(dropped[0], "MLKEM4096-FromTheFuture") {
		t.Errorf("OperandEnv() dropped = %v, want one entry naming the unknown group", dropped)
	}
}

// TestOperandEnvAllGroupsUnknownSubstitutes is the boundary of the rule above.
// Dropping some of a profile's groups narrows the list, which is safe. Dropping
// all of them would omit the variable entirely, and OpenSSL's default groups
// are wider than any profile -- so the operand would negotiate key exchanges
// the cluster ruled out. A known-narrow list is substituted instead, and the
// substitution is reported.
func TestOperandEnvAllGroupsUnknownSubstitutes(t *testing.T) {
	spec := configv1.TLSProfileSpec{
		Groups: []configv1.TLSGroup{
			configv1.TLSGroup("MLKEM4096-FromTheFuture"),
			configv1.TLSGroup("SomethingElseEntirely"),
		},
		MinTLSVersion: configv1.VersionTLS12,
	}

	env, dropped := OperandEnv(spec, Options{})

	want := strings.Join(pqcGroupNames(), ":")
	if got, ok := env[EnvGroups]; !ok || got != want {
		t.Errorf("%s = %q (present=%t), want %q: omitting it would hand the operand "+
			"OpenSSL's defaults, which are wider than the profile", EnvGroups, got, ok, want)
	}
	if len(dropped) != 3 {
		t.Errorf("OperandEnv() dropped = %v, want both unknown groups plus the substitution", dropped)
	}
	if !strings.Contains(dropped[len(dropped)-1], want) {
		t.Errorf("OperandEnv() last dropped entry = %q, want it to name the substituted list %q",
			dropped[len(dropped)-1], want)
	}
}

// TestOperandEnvNoGroupsAtAllIsLeftAlone separates the substitution above from
// the case it must not swallow: a profile that names no groups expresses no
// opinion, so OpenSSL's defaults are the right answer and nothing is published.
func TestOperandEnvNoGroupsAtAllIsLeftAlone(t *testing.T) {
	spec := configv1.TLSProfileSpec{MinTLSVersion: configv1.VersionTLS12}

	env, dropped := OperandEnv(spec, Options{})

	if got, ok := env[EnvGroups]; ok {
		t.Errorf("%s = %q, want it omitted when the profile names no groups", EnvGroups, got)
	}
	if len(dropped) != 0 {
		t.Errorf("OperandEnv() dropped = %v, want nothing reported", dropped)
	}
}

// TestOperandEnvUnknownMinVersion checks the other fail-soft path, and that it
// fails soft *upwards*. minTLSVersion is a required enum field, so a value this
// build cannot name came from a newer OpenShift API and is higher than anything
// in the map. Falling back to the highest known version keeps the operand
// inside the cluster policy; falling back to a lower one would hand it
// protocols the cluster has forbidden.
func TestOperandEnvUnknownMinVersion(t *testing.T) {
	spec := configv1.TLSProfileSpec{MinTLSVersion: configv1.TLSProtocolVersion("VersionTLS99")}

	env, dropped := OperandEnv(spec, Options{})

	if got := env[EnvMinVersion]; got != minVersionTLS13 {
		t.Errorf("%s = %q, want the %q fallback: an unrecognised version is newer than "+
			"this build knows, so the floor must not drop below the highest it can name",
			EnvMinVersion, got, minVersionTLS13)
	}
	if len(dropped) != 1 || !strings.Contains(dropped[0], "VersionTLS99") {
		t.Errorf("OperandEnv() dropped = %v, want one entry naming the unrecognised version", dropped)
	}
}

// TestSplitCiphers pins the rule the split relies on: TLS 1.3 suites carry the
// IANA "TLS_" prefix, and the TLS 1.2 entries use OpenSSL's hyphenated
// spelling, which never does.
func TestSplitCiphers(t *testing.T) {
	suites, ciphers := splitCiphers([]string{
		tls13SuiteAES128,
		"ECDHE-ECDSA-AES128-GCM-SHA256",
		"TLS_CHACHA20_POLY1305_SHA256",
		"DHE-RSA-AES256-GCM-SHA384",
	})

	if want := []string{tls13SuiteAES128, "TLS_CHACHA20_POLY1305_SHA256"}; !equal(suites, want) {
		t.Errorf("splitCiphers() suites = %v, want %v", suites, want)
	}
	if want := []string{"ECDHE-ECDSA-AES128-GCM-SHA256", "DHE-RSA-AES256-GCM-SHA384"}; !equal(ciphers, want) {
		t.Errorf("splitCiphers() ciphers = %v, want %v", ciphers, want)
	}
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
