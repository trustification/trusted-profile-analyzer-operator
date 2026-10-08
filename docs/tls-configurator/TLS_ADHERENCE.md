# TLS Profile Compliance and `tlsAdherence`

How this operator implements Red Hat's *TLS Profile Compliance — Implementation
Reference*, and where the code lives.

## Which recipe this operator follows

The reference's approach-selection table has a row for exactly this shape:

> **Downstream operator for upstream operand** — the downstream operator uses
> one of the other approaches to find the TLS settings of the cluster, then
> configures the upstream operand via the operand's own TLS configuration
> mechanism (flags, env, configmap, …).

That is what the TLS configurator does. It reads the cluster TLS settings from
`apiservers.config.openshift.io/cluster` and reconfigures the operand — the
RHTPA Deployments — through the operand's mechanism, which here is a pod
template rollout driven by the `rhtpa.io/tls-config-hash` annotation.

The cluster-read half uses the reference's documented packages rather than
local reimplementations:

| Concern | Package | Used in |
| --- | --- | --- |
| Resolve a `TLSSecurityProfile` to a concrete `TLSProfileSpec` | `controller-runtime-common/pkg/tls.GetTLSProfileSpec` | `crypto.ResolveProfileSpec` |
| Build a `crypto/tls.Config` from the spec | `controller-runtime-common/pkg/tls.NewTLSConfigFromProfile` | `crypto.BuildTLSConfigFromSpec` |
| ALPN, which the cluster profile never sets | `controller-runtime-common/pkg/tls.SetNextProtos` | `crypto.BuildTLSConfigFromSpec` |
| OpenSSL ↔ IANA cipher names, TLS version parsing | `library-go/pkg/crypto` | `crypto.TLSVersion`, `crypto.OpenSSLToIANACipherSuites`, `crypto.CipherSuites` |
| Interpret `tlsAdherence` | `library-go/pkg/crypto.ShouldHonorClusterTLSProfile` | `crypto.ShouldHonorClusterTLSProfile` |

The watch itself is the reference's "Direct Go → Watch for Profile Changes"
pattern: a typed `APIServers().Watch` filtered with
`FieldSelector: metadata.name=cluster`. It is not the `SecurityProfileWatcher`
from `controller-runtime-common`, because that watcher's contract is
*exit-on-change so my own pod restarts with the new profile* — the configurator
does not serve TLS, it rolls other people's pods, and must stay alive to do it.

## `tlsAdherence`

`apiservers.config.openshift.io/cluster` `.spec.tlsAdherence` says how strictly
components must honor the cluster-wide profile. It is gated on the
`TLSAdherence` feature gate, so it reads back as `""` on clusters without it.

| Value | Meaning | `ShouldHonorClusterTLSProfile` |
| --- | --- | --- |
| `""` (unset) | No opinion; currently treated as legacy. Subject to change. | `false` |
| `LegacyAdheringComponentsOnly` | Only components that already honor the profile continue to. | `false` |
| `StrictAllComponents` | Every component must honor the profile. | `true` |
| anything else | Unknown — assume a future, stricter policy. | `true` |

### What this operator does with it

The configurator **always propagates the cluster profile**, in every adherence
mode. It is an adhering component by construction: it has no TLS settings of
its own that could compete with the cluster's, so there is nothing for
`LegacyAdheringComponentsOnly` to let it keep. Switching it off in legacy mode
would only make the operator less compliant for no gain.

What the policy *does* affect:

- **It is part of the rollout hash.** `reconcile.TLSConfigHash` hashes the
  resolved profile spec, the adherence policy, and the PQC flag together.
  Tightening `tlsAdherence` from unset to `StrictAllComponents` therefore rolls
  the target Deployments even though the profile itself did not move — the
  pods restart into a cluster whose TLS contract has changed.
- **It is logged**, once per distinct value, so an operator reading the
  configurator's logs can see whether the cluster considers adherence
  mandatory.
- **It is reported** by `--action=get-adherence`, and included in the output of
  `--action=get-cluster`, `--action=show-tlsconfig` and `--action=validate`.

## Hashing the resolved spec, not the profile

The hash covers the *resolved* `TLSProfileSpec`, not the raw
`TLSSecurityProfile`. Profiles that differ only in spelling — unset versus an
explicit `Intermediate`, or a `Custom` profile that writes out exactly what a
built-in already means — produce the same hash and do not roll the workloads.
A real change to ciphers, groups, minimum version, adherence, or the PQC flag
does.

## Post-quantum key exchange

PQC in TLS 1.3 is delivered by the hybrid key-exchange group `X25519MLKEM768`
(X25519 + ML-KEM-768, NIST FIPS 203) — a **group**, not a cipher suite.

Two things changed here relative to the previous implementation:

1. **Groups now arrive from the cluster profile.** `TLSProfileSpec.Groups`
   exists in the current `openshift/api`, and the built-in `Old`,
   `Intermediate` and `Modern` profiles all already list `X25519MLKEM768`
   first. Because the config now comes from `NewTLSConfigFromProfile`, those
   groups land in `tls.Config.CurvePreferences` automatically. The previous
   hand-rolled converter ignored `Groups` entirely, so a cluster on `Modern`
   looked non-post-quantum to this operator when it was not.
2. **`--action=update` can write groups.** With `--enable-pqc` and a `Custom`
   profile, the configurator writes `Groups: [X25519MLKEM768, X25519]` so the
   router advertises the hybrid group. `TLSProfileSpec.Groups` is gated on
   `TLSGroupPreferences`; writing it on a cluster without the gate is rejected
   by the API server, so the configurator reads the `FeatureGate` CR first
   (`client.FeatureGateChecker`) and skips the field — with a log line —
   when the gate is off.

`--enable-pqc` still additionally forces `MinVersion` to TLS 1.3 locally, since
hybrid key exchange is only defined for TLS 1.3.

## Scope notes from the reference

- *All TLS servers must honor the profile.* The operator manager serves no
  TLS: `--metrics-bind-address` defaults to `0` and controller-runtime's
  metrics server is not configured for secure serving, so there is no
  `tls.Config` to apply. If metrics are ever switched to HTTPS, wire
  `tlspkg.FetchAPIServerTLSProfile` + `NewTLSConfigFromProfile` into
  `server.Options.TLSOpts` and register a `SecurityProfileWatcher` that
  cancels the manager context — the reference's controller-runtime recipe.
- *Never use `InsecureSkipVerify`.* Nothing in `pkg/tlsconfigurator` sets it.
- *Set ALPN yourself.* `crypto.BuildTLSConfig` defaults `NextProtos` to
  `h2, http/1.1`; pass `Options{NextProtos: []string{"http/1.1"}}` for servers
  that should not offer HTTP/2.

## RBAC

Beyond the existing grants, the configurator reads the `FeatureGate` CR:

```yaml
- apiGroups: ["config.openshift.io"]
  resources: ["featuregates"]
  verbs: ["get", "list"]
```

Because this is a Helm operator it can only grant permissions it holds itself,
so the same rule was added to `config/rbac/role_cluster_rbac_manager.yaml`.
Reading `.spec.tlsAdherence` needs no new permission — it comes from the same
`APIServer` get the profile already uses.
