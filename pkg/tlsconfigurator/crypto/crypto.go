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

// Package crypto turns an OpenShift TLSSecurityProfile into a crypto/tls.Config.
//
// It is a thin adapter over the two packages Red Hat documents for this in the
// "TLS Profile Compliance -- Implementation Reference":
//
//   - github.com/openshift/controller-runtime-common/pkg/tls -- profile
//     resolution (GetTLSProfileSpec) and tls.Config construction
//     (NewTLSConfigFromProfile, SetNextProtos).
//   - github.com/openshift/library-go/pkg/crypto -- TLS version and cipher
//     name conversion, and ShouldHonorClusterTLSProfile.
//
// Nothing here re-implements those conversions. The local additions are the
// post-quantum helpers, which the upstream packages do not provide because the
// PQC key-exchange group now arrives through the profile's Groups field rather
// than through a separate switch.
package crypto

import (
	"crypto/tls"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	ocptls "github.com/openshift/controller-runtime-common/pkg/tls"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
)

// DefaultNextProtos is the ALPN list applied when a caller does not pick one.
// The cluster TLS profile deliberately says nothing about ALPN, so every server
// has to set NextProtos itself.
var DefaultNextProtos = ocptls.HTTP2NextProtos

// Options are the local hardening choices layered on top of the cluster
// profile. Everything that the cluster profile itself expresses -- minimum
// version, ciphers, key-exchange groups -- comes from the profile, not from
// here.
type Options struct {
	// EnablePQC forces TLS 1.3 and guarantees that the hybrid post-quantum
	// group X25519MLKEM768 is offered even if the cluster profile omits it.
	EnablePQC bool

	// NextProtos is the ALPN list to advertise. Nil means DefaultNextProtos;
	// an explicitly empty, non-nil slice leaves ALPN unset.
	NextProtos []string
}

// PQCCurvePreferences returns the post-quantum key-exchange groups to
// negotiate, most-preferred first. X25519MLKEM768 is the hybrid group
// (classical X25519 + ML-KEM-768, NIST FIPS 203) enabled by default in Go's
// crypto/tls since Go 1.24; plain X25519 is kept as the classical fallback for
// peers without PQC support.
func PQCCurvePreferences() []tls.CurveID {
	return []tls.CurveID{
		tls.X25519MLKEM768,
		tls.X25519,
	}
}

// PQCGroups returns the same preference list in the OpenShift API's vocabulary,
// for writing into a TLSProfileSpec.Groups field.
func PQCGroups() []configv1.TLSGroup {
	return []configv1.TLSGroup{
		configv1.TLSGroupX25519MLKEM768,
		configv1.TLSGroupX25519,
	}
}

// ResolveProfileSpec resolves a TLSSecurityProfile to the concrete
// TLSProfileSpec it stands for: built-in types expand from
// configv1.TLSProfiles, Custom returns its embedded spec, and nil or an
// unknown type falls back to Intermediate.
func ResolveProfileSpec(profile *configv1.TLSSecurityProfile) (configv1.TLSProfileSpec, error) {
	spec, err := ocptls.GetTLSProfileSpec(profile)
	if err != nil {
		return configv1.TLSProfileSpec{}, fmt.Errorf("failed to resolve TLS security profile: %w", err)
	}
	return spec, nil
}

// BuildTLSConfig resolves a TLSSecurityProfile and converts it to a
// crypto/tls.Config.
//
// The returned unsupported slice lists cipher and group names present in the
// profile that Go's crypto/tls cannot honor. That is informational, not an
// error: a cluster profile may legitimately name ciphers (DHE-RSA-*,
// AES256-SHA256, ...) that only OpenSSL-based servers can offer. Callers should
// log it.
func BuildTLSConfig(profile *configv1.TLSSecurityProfile, opts Options) (
	tlsConfig *tls.Config, unsupported []string, err error,
) {
	spec, err := ResolveProfileSpec(profile)
	if err != nil {
		return nil, nil, err
	}
	cfg, unsupported := BuildTLSConfigFromSpec(spec, opts)
	return cfg, unsupported, nil
}

// BuildTLSConfigFromSpec converts an already-resolved TLSProfileSpec to a
// crypto/tls.Config. Use this when the spec was obtained from a watch event or
// cached elsewhere, so the profile is resolved exactly once.
func BuildTLSConfigFromSpec(spec configv1.TLSProfileSpec, opts Options) (tlsConfig *tls.Config, unsupported []string) {
	apply, unsupported := ocptls.NewTLSConfigFromProfile(spec)

	cfg := &tls.Config{}
	apply(cfg)

	nextProtos := opts.NextProtos
	if nextProtos == nil {
		nextProtos = DefaultNextProtos
	}
	ocptls.SetNextProtos(nextProtos...)(cfg)

	SecureTLSConfig(cfg)

	if opts.EnablePQC {
		EnablePQC(cfg)
	}

	return cfg, unsupported
}

// TLSVersion converts a TLS version string ("VersionTLS12") to the
// corresponding crypto/tls constant.
func TLSVersion(version string) (uint16, error) {
	return libgocrypto.TLSVersion(version)
}

// OpenSSLToIANACipherSuites converts OpenSSL cipher names to IANA names.
// Names with no OpenSSL spelling are passed through unchanged, so already-IANA
// input round-trips.
func OpenSSLToIANACipherSuites(opensslNames []string) []string {
	return libgocrypto.OpenSSLToIANACipherSuites(opensslNames)
}

// CipherSuites converts IANA cipher suite names to crypto/tls constants. It
// fails on the first name Go does not know; use BuildTLSConfig instead when
// unknown ciphers should be reported rather than rejected.
func CipherSuites(ianaNames []string) ([]uint16, error) {
	suites := make([]uint16, 0, len(ianaNames))
	for _, name := range ianaNames {
		id, err := libgocrypto.CipherSuite(name)
		if err != nil {
			return nil, fmt.Errorf("unknown cipher suite: %s", name)
		}
		suites = append(suites, id)
	}
	return suites, nil
}

// CipherSuitesOrDie is like CipherSuites but panics on error.
func CipherSuitesOrDie(ianaNames []string) []uint16 {
	suites, err := CipherSuites(ianaNames)
	if err != nil {
		panic(fmt.Sprintf("failed to convert cipher suites: %v", err))
	}
	return suites
}

// ShouldHonorClusterTLSProfile reports whether a component must apply the
// cluster-wide TLS profile given the cluster's .spec.tlsAdherence policy.
// Unknown values return true, so a future, stricter policy is honored by
// default.
func ShouldHonorClusterTLSProfile(adherence configv1.TLSAdherencePolicy) bool {
	return libgocrypto.ShouldHonorClusterTLSProfile(adherence)
}

// SecureTLSConfig applies secure baseline settings to a TLS config.
func SecureTLSConfig(config *tls.Config) {
	if config == nil {
		return
	}

	// Set secure defaults.
	// PreferServerCipherSuites is deliberately not set: Go has ignored the field
	// since 1.18 and always applies its own cipher preference order.
	config.SessionTicketsDisabled = false
	config.Renegotiation = tls.RenegotiateNever

	// Note: We don't override MinVersion here to allow Old profile
	// to use TLS 1.0/1.1 if explicitly configured
}

// EnablePQC hardens a crypto/tls.Config for post-quantum key exchange.
// It forces the minimum protocol version to TLS 1.3 (hybrid PQC key exchange is
// only defined for TLS 1.3) and sets the key-exchange group preferences to the
// post-quantum list. The symmetric cipher suites are left untouched: for TLS 1.3
// Go selects its AEAD suites internally and ignores Config.CipherSuites, and
// those suites are already quantum-resistant at the symmetric level.
func EnablePQC(config *tls.Config) {
	if config == nil {
		return
	}
	if config.MinVersion < tls.VersionTLS13 {
		config.MinVersion = tls.VersionTLS13
	}
	// Go ignores CipherSuites for TLS 1.3; leaving stale TLS 1.2 suites behind
	// is just misleading.
	config.CipherSuites = nil
	config.CurvePreferences = PQCCurvePreferences()
}

// IsPQCCompliant reports whether a crypto/tls.Config negotiates post-quantum,
// TLS 1.3-only key exchange. When it is not compliant, the returned reasons
// explain what is missing.
func IsPQCCompliant(config *tls.Config) (bool, []string) {
	if config == nil {
		return false, []string{"tls config is nil"}
	}

	var reasons []string
	if config.MinVersion < tls.VersionTLS13 {
		reasons = append(reasons, fmt.Sprintf("MinVersion is %s, must be TLS 1.3", TLSVersionName(config.MinVersion)))
	}

	hasPQCGroup := false
	for _, c := range config.CurvePreferences {
		if c == tls.X25519MLKEM768 {
			hasPQCGroup = true
			break
		}
	}
	if !hasPQCGroup {
		reasons = append(reasons, "CurvePreferences does not include X25519MLKEM768")
	}

	return len(reasons) == 0, reasons
}

// CurveName returns a human-readable name for a TLS key-exchange group.
func CurveName(id tls.CurveID) string {
	switch id {
	case tls.X25519MLKEM768:
		return string(configv1.TLSGroupX25519MLKEM768)
	case tls.X25519:
		return string(configv1.TLSGroupX25519)
	case tls.CurveP256:
		return "CurveP256"
	case tls.CurveP384:
		return "CurveP384"
	case tls.CurveP521:
		return "CurveP521"
	default:
		return fmt.Sprintf("Unknown(0x%04x)", uint16(id))
	}
}

// TLSVersionName renders a crypto/tls version constant for humans. The
// library-go helper panics on unknown input, so this uses the stdlib one.
func TLSVersionName(version uint16) string {
	if version == 0 {
		return "unset"
	}
	return tls.VersionName(version)
}

// CipherSuiteName renders a crypto/tls cipher suite constant for humans. The
// library-go helper panics on unknown input, so this uses the stdlib one.
func CipherSuiteName(id uint16) string {
	return tls.CipherSuiteName(id)
}

// DefaultTLSConfig returns the TLS configuration for the cluster default
// profile (Intermediate), for callers that cannot reach the API server.
func DefaultTLSConfig() *tls.Config {
	cfg, _ := BuildTLSConfigFromSpec(*configv1.TLSProfiles[configv1.TLSProfileIntermediateType], Options{})
	return cfg
}

// DefaultTLSVersion returns the minimum TLS version of the cluster default
// profile.
func DefaultTLSVersion() uint16 {
	return libgocrypto.TLSVersionOrDie(string(ocptls.DefaultMinTLSVersion))
}
