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
	"fmt"
	"sort"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
)

// Environment variables understood by trustd's HTTP server (see
// common/infrastructure/src/app/http.rs in the trustify repository). They are
// the operand's own configuration mechanism, which is what the TLS Profile
// Compliance reference asks a downstream operator to drive.
//
// trustd is OpenSSL-based, not Go-based, so the profile is handed over in
// OpenSSL's vocabulary rather than converted to IANA names: a cipher list for
// TLS 1.2 and below, a separate ciphersuite list for TLS 1.3, and a group list
// for key exchange. That is exactly how configv1.TLSProfileSpec already spells
// its contents, so no name translation is involved.
const (
	// EnvProfile selects trustd's profile mapping. Always "custom" here: the
	// built-in old/intermediate/modern mappings are Mozilla profiles compiled
	// into trustd and ignore the min version and cipher settings.
	EnvProfile = "HTTP_SERVER_TLS_SECURITY_PROFILE"
	// EnvMinVersion is the minimum protocol version, as "1.0".."1.3".
	EnvMinVersion = "HTTP_SERVER_TLS_MIN_VERSION"
	// EnvCiphers is the TLS 1.2 and below cipher list, OpenSSL names, ":"-separated.
	EnvCiphers = "HTTP_SERVER_TLS_CIPHERS"
	// EnvCipherSuites is the TLS 1.3 ciphersuite list, ":"-separated.
	EnvCipherSuites = "HTTP_SERVER_TLS_CIPHERSUITES"
	// EnvGroups is the key-exchange group list, in preference order, ":"-separated.
	EnvGroups = "HTTP_SERVER_TLS_GROUPS"
)

// operandProfile is the value of EnvProfile. See the constant's comment.
const operandProfile = "custom"

// minVersionTLS13 is trustd's spelling of TLS 1.3. It is singled out because
// four decisions turn on it: PQC raises the minimum to it, a profile with no
// usable pre-1.3 cipher raises the minimum to it, a TLS 1.3-only listener
// suppresses the TLS 1.2 cipher list, and -- being the ceiling of the version
// map -- it is what an unrecognised version falls back to.
const minVersionTLS13 = "1.3"

// minVersionTLS12 is trustd's spelling of TLS 1.2.
const minVersionTLS12 = "1.2"

// openSSLGroups is the allowlist of key-exchange groups that may be handed to
// the operand, mapping the OpenShift API spelling to OpenSSL's.
//
// This is an allowlist rather than a pass-through on purpose. OpenSSL's
// SSL_CTX_set1_groups_list rejects the whole list when it contains one name it
// does not know, and trustd treats that as a fatal startup error -- so a group
// added to the OpenShift API in a future release, or one an older OpenSSL has
// never heard of, would take the server down rather than degrade. Unknown
// entries are dropped and reported instead.
//
// The spellings happen to be identical on both sides; the map is kept explicit
// so that a future divergence has somewhere to live.
var openSSLGroups = map[configv1.TLSGroup]string{
	configv1.TLSGroupX25519:             "X25519",
	configv1.TLSGroupSecP256r1:          "secp256r1",
	configv1.TLSGroupSecP384r1:          "secp384r1",
	configv1.TLSGroupSecP521r1:          "secp521r1",
	configv1.TLSGroupX25519MLKEM768:     "X25519MLKEM768",
	configv1.TLSGroupSecP256r1MLKEM768:  "SecP256r1MLKEM768",
	configv1.TLSGroupSecP384r1MLKEM1024: "SecP384r1MLKEM1024",
}

// minVersionNames maps the OpenShift protocol version enum to the spelling
// trustd's --http-server-tls-min-version accepts.
var minVersionNames = map[configv1.TLSProtocolVersion]string{
	configv1.VersionTLS10: "1.0",
	configv1.VersionTLS11: "1.1",
	configv1.VersionTLS12: minVersionTLS12,
	configv1.VersionTLS13: minVersionTLS13,
}

// OperandEnv renders a resolved cluster TLS profile as the environment the
// RHTPA workloads read at startup, so that their own TLS listener serves the
// cluster profile rather than trustd's compiled-in default.
//
// The returned dropped slice lists profile entries that were left out, for the
// caller to log. It is informational: an unusable entry must not stop the rest
// of the profile from being applied.
//
// Empty lists are omitted rather than emitted as empty strings. An empty
// ciphersuite list passed to OpenSSL disables every TLS 1.3 suite and leaves a
// TLS 1.3-only server unable to complete a handshake, so "the profile says
// nothing" has to mean "leave OpenSSL's default alone". Where that fallback
// would widen what the listener accepts beyond the profile, the minimum
// version is raised instead -- see the cipher handling below.
func OperandEnv(spec configv1.TLSProfileSpec, opts Options) (env map[string]string, dropped []string) {
	minVersion, versionDropped := operandMinVersion(spec.MinTLSVersion, opts.EnablePQC)
	dropped = append(dropped, versionDropped...)

	suites, ciphers := splitCiphers(spec.Ciphers)

	// A profile that names ciphers but none that TLS 1.2 and below can
	// negotiate cannot be expressed as a cipher list: OpenSSL rejects a list
	// that selects nothing, so trustd would fail to start. Omitting the
	// variable is worse -- the older versions would fall back to OpenSSL's
	// default list, quietly accepting suites the profile never allowed.
	//
	// Raising the floor to TLS 1.3 says the same thing and is enforceable:
	// with no permitted pre-1.3 cipher there is nothing those versions could
	// have completed a handshake with anyway. A profile naming no ciphers at
	// all is a different case -- it expresses no opinion, so OpenSSL's
	// defaults stand and the minimum is left alone.
	if minVersion != minVersionTLS13 && len(ciphers) == 0 && len(spec.Ciphers) > 0 {
		dropped = append(dropped, fmt.Sprintf(
			"profile lists no cipher usable below TLS 1.3, so the minimum version is "+
				"raised from %s to %s rather than letting OpenSSL's defaults apply",
			minVersion, minVersionTLS13))
		minVersion = minVersionTLS13
	}

	env = map[string]string{
		EnvProfile:    operandProfile,
		EnvMinVersion: minVersion,
	}

	// The TLS 1.3 suites are passed through when the profile names them, but
	// their absence is deliberately *not* treated like the case above. The
	// OpenShift API is explicit that "TLS 1.3 cipher suites are not
	// configurable and are always enabled when TLS 1.3 is negotiated"
	// (TLSProfileSpec.Ciphers), so falling back to OpenSSL's TLS 1.3 defaults
	// is the specified behaviour, not a gap. Forcing an empty list here would
	// disable TLS 1.3 outright on a profile that permits it.
	if len(suites) > 0 {
		env[EnvCipherSuites] = strings.Join(suites, ":")
	}
	// Below TLS 1.3 only; a TLS 1.3-only server never negotiates these, and
	// emitting them would imply a fallback that cannot happen.
	if len(ciphers) > 0 && minVersion != minVersionTLS13 {
		env[EnvCiphers] = strings.Join(ciphers, ":")
	}

	groups, groupsDropped := operandGroups(spec.Groups, opts.EnablePQC)
	dropped = append(dropped, groupsDropped...)
	if len(groups) > 0 {
		env[EnvGroups] = strings.Join(groups, ":")
	}

	return env, dropped
}

// operandMinVersion renders the profile's minimum protocol version, raising it
// to TLS 1.3 under PQC because hybrid key exchange is only defined there.
//
// An unrecognised version is reported and treated as TLS 1.3, the highest this
// build can name. minTLSVersion is a required field whose CRD pins it to an
// enum, so a value that is not in the map cannot be a typo or an unset field:
// it is a version added to the OpenShift API after this build, and therefore
// one *above* everything here can spell. Falling back to a lower version would
// publish a weaker floor than the cluster policy states and let the operand
// negotiate protocols the cluster has forbidden -- the one outcome this
// function must not produce. Refusing to configure the operand at all is no
// better, since that leaves it on its compiled-in default; TLS 1.3 is the
// closest this build can come to enforcing what was asked.
func operandMinVersion(version configv1.TLSProtocolVersion, enablePQC bool) (minVersion string, dropped []string) {
	name, known := minVersionNames[version]
	if !known {
		name = minVersionTLS13
		dropped = append(dropped, fmt.Sprintf(
			"minTLSVersion %q is newer than this build recognises, so the minimum version is "+
				"set to %s -- the highest it can name -- rather than a weaker floor than the "+
				"cluster asks for", version, name))
	}
	if enablePQC && name != minVersionTLS13 {
		return minVersionTLS13, dropped
	}
	return name, dropped
}

// splitCiphers separates a profile's cipher list into the TLS 1.3 ciphersuites
// and the ciphers for TLS 1.2 and below, which OpenSSL configures through two
// different calls. TLSProfileSpec.Ciphers holds both in one list, the 1.3
// suites first, distinguished by their IANA "TLS_" prefix -- the 1.2 entries
// use OpenSSL's hyphenated spelling, which never starts with "TLS_".
func splitCiphers(all []string) (suites, ciphers []string) {
	for _, c := range all {
		if strings.HasPrefix(c, "TLS_") {
			suites = append(suites, c)
			continue
		}
		ciphers = append(ciphers, c)
	}
	return suites, ciphers
}

// operandGroups renders the profile's key-exchange groups in preference order,
// dropping any this build cannot name for OpenSSL.
//
// Under PQC the profile's list is replaced rather than extended, matching what
// EnablePQC does to a crypto/tls.Config: the point of the flag is that the
// hybrid group is the only key exchange on offer besides its classical
// fallback.
//
// Dropping some of the profile's groups is safe -- what remains is a subset of
// what the cluster permits. Dropping *all* of them is not: the list would then
// be omitted, and OpenSSL's defaults are wider than any profile, so the operand
// would negotiate key exchanges the cluster has ruled out. That case falls back
// to pqcGroupNames instead; see the comment there.
func operandGroups(groups []configv1.TLSGroup, enablePQC bool) (names, dropped []string) {
	if enablePQC {
		return pqcGroupNames(), nil
	}

	for _, g := range groups {
		name, known := openSSLGroups[g]
		if !known {
			dropped = append(dropped, fmt.Sprintf("key-exchange group %q is not supported by this build", g))
			continue
		}
		names = append(names, name)
	}

	// The profile named groups and this build could name none of them. The
	// three ways out are all imperfect, so the least bad one is taken
	// deliberately: publishing the unknown names is fatal (OpenSSL rejects a
	// list containing a single name it does not know, and trustd will not
	// start), and publishing nothing is a silent widening to OpenSSL's
	// defaults. Substituting the narrowest list this build can name -- the
	// hybrid post-quantum group and its classical partner -- keeps the operand
	// serving under a group list no wider than a modern profile's. It is not
	// what the cluster asked for, which is why it is reported in dropped.
	if len(names) == 0 && len(groups) > 0 {
		names = pqcGroupNames()
		dropped = append(dropped, fmt.Sprintf(
			"no key-exchange group named by the profile is supported by this build, so %s is "+
				"published instead of letting OpenSSL's wider defaults apply",
			strings.Join(names, ":")))
	}
	return names, dropped
}

// pqcGroupNames renders PQCGroups in OpenSSL's vocabulary.
func pqcGroupNames() []string {
	groups := PQCGroups()
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, openSSLGroups[g])
	}
	return names
}

// OperandEnvSummary renders an environment map as sorted "KEY=value" lines, for
// logging a configuration that is otherwise only visible inside a ConfigMap.
func OperandEnvSummary(env map[string]string) []string {
	lines := make([]string, 0, len(env))
	for k, v := range env {
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(lines)
	return lines
}
