# Configuring the operator per platform

How to configure the TLS configurator when the operator is deployed on
**OpenShift** and on **plain Kubernetes**.

The module is optional at every OpenShift version — there is no longer a
version gate that makes it mandatory. One version boundary does still matter,
but it is about what the cluster itself does rather than about this module:
from **OpenShift 5.1** the cluster-wide default TLS security profile is
`Modern` (`minTLSVersion: VersionTLS13`), where up to 5.0 it was
`Intermediate`. A cluster that sets no `.spec.tlsSecurityProfile` is therefore
on TLS 1.3 from 5.1 on, and the configurator resolves an unset profile against
the cluster version so it reports and publishes the right one.

Everything here is set on the `TrustedProfileAnalyzer` CR. The operator passes
`.spec` straight through to the Helm chart as values, so a chart value
`modules.tlsConfigurator.enabled` is CR field
`.spec.modules.tlsConfigurator.enabled`. The same keys work for a standalone
`helm install`.

For the *why* behind the module — the upstream packages it delegates to, how
`tlsAdherence` is interpreted, what goes into the rollout hash — see
[TLS_ADHERENCE.md](TLS_ADHERENCE.md). For a runnable walkthrough on CRC, see
[`devel/README.md`](../../devel/README.md).

## The short version

| Platform | `modules.tlsConfigurator.enabled` | Enforced how |
| --- | --- | --- |
| OpenShift (any version) | optional, recommended | Nothing |
| Plain Kubernetes | **`false` — required** (the chart default) | Install fails if enabled |

A second rule applies to `operandTls`, independently of the platform row:

| Cluster | `modules.tlsConfigurator.operandTls.enabled` | Enforced how |
| --- | --- | --- |
| `tlsAdherence: StrictAllComponents`, TLS-serving server | **`true` — required** | Install fails unless you also set `operandTls.allowDisabled: true` |
| Anything else | optional (default `false`) | Nothing |

Note that `enabled: true` on its own does **not** satisfy the second rule —
`operandTls.enabled` defaults to `false` and is a separate opt-in. See
[Strict adherence](#strict-adherence-and-operandtls).

The guard lives in
`helm-charts/redhat-trusted-profile-analyzer/templates/init/tls-configure/000-validate.yaml`.
It renders no resources and is deliberately *not* gated on `.enabled`, because
it has to catch the disabled case too.

> **Changed:** installing on OpenShift >= 4.22 with the module disabled used to
> fail unless `modules.tlsConfigurator.allowDisabled: true` acknowledged it.
> That requirement is gone, and the key has been removed. A CR or Subscription
> that still sets it is accepted and ignored, so nothing needs changing on
> upgrade. (`operandTls.allowDisabled` is a different key and still applies.)

## Why the platform matters

It is not only that `config.openshift.io/v1` is absent off OpenShift. The two
platforms give the RHTPA server a different role in TLS entirely, and that is
what decides whether a cluster TLS profile applies to it at all.

**On OpenShift**, `openshift.useServiceCa` defaults to `true`. The chart then:

- annotates the server Service with
  `service.beta.openshift.io/serving-cert-secret-name`, so the service-CA
  operator issues a certificate;
- mounts that secret and sets `HTTP_SERVER_TLS_ENABLED=true`,
  `HTTP_SERVER_TLS_KEY_FILE`, `HTTP_SERVER_TLS_CERTIFICATE_FILE`;
- annotates the Ingress with `route.openshift.io/termination: reencrypt`.

The server is therefore **a real TLS server** and is in scope for the
cluster-wide profile: it must honor it, and something has to restart it when
the profile changes. That something is the configurator — the restart with
`enabled`, and the honoring itself with
[`operandTls.enabled`](#honoring-the-profile-inside-the-workloads), which is a
separate opt-in.

**On plain Kubernetes**, `trustification.openshift.useServiceCa` resolves to
`false` regardless of the value you set. `HTTP_SERVER_TLS_ENABLED` is never
set, so the server listens on plain HTTP and TLS terminates at your ingress
controller. In the vocabulary of the TLS Profile Compliance reference this is
*"plaintext behind a TLS-terminating router: out of scope."* There is no
`tls.Config` in the RHTPA pods to apply a profile to, and no cluster-wide
profile API to read one from.

---

## OpenShift

### Minimum configuration

```yaml
apiVersion: rhtpa.io/v1
kind: TrustedProfileAnalyzer
metadata:
  name: rhtpa
  namespace: trustify
spec:
  appDomain: -trustify.apps.example.com
  modules:
    tlsConfigurator:
      enabled: true
```

That is enough. The defaults cover the rest:

| Key | Default | Meaning |
| --- | --- | --- |
| `targetDeployments` | `[server]` | Deployments in the release namespace to roll when the TLS configuration changes. Must match **rendered** Deployment names — the server renders as `server`. |
| `resyncPeriod` | `5m` | Periodic drift correction, in case a watch event is missed. |
| `pqc.enabled` | `false` | See [Post-quantum](#post-quantum-key-exchange) below. |
| `operandTls.enabled` | `false` | Whether the workloads' own TLS listener honors the cluster profile, or only gets restarted when it changes. See [Honoring the profile inside the workloads](#honoring-the-profile-inside-the-workloads) below. |
| `operandTls.allowDisabled` | `false` | Escape hatch for installing without operand publishing on a `StrictAllComponents` cluster. |
| `image.fullName` | *(empty)* | Filled in by the operator with its own running image via `RELATED_IMAGE_TLS_CONFIGURATOR`; do not set it. Only a standalone `helm install` has to supply one — rendering fails with a message if the module is enabled and it is empty. |

### Recommended configuration

The minimum above installs cleanly, but it only *restarts* the workloads on a
profile change — they come back serving the same TLS settings as before. For a
cluster you intend to keep adherent, turn on all three switches:

```yaml
spec:
  modules:
    tlsConfigurator:
      # 1. Run the reconciler: watch the cluster TLS profile and roll the
      #    workloads when it changes.
      enabled: true

      # 2. Make the workloads' own HTTPS listener honor the cluster profile,
      #    rather than only restarting them when it changes. This is what
      #    turns a restart into an actual configuration change.
      operandTls:
        enabled: true

      # 3. Always offer the hybrid post-quantum X25519MLKEM768 group, even
      #    on a cluster profile that omits it. This does not raise the
      #    minimum TLS version -- see the table below.
      pqc:
        enabled: true

      # Roll these Deployments when the cluster TLS configuration changes.
      # Add any other TLS-serving workload you introduce.
      targetDeployments:
        - server

      # Periodic drift correction. Lower it if you want faster recovery from a
      # missed watch event; the watch is the primary trigger either way.
      resyncPeriod: 5m
```

Why each one, and what it costs:

| Switch | What you gain | What it costs |
| --- | --- | --- |
| `enabled` | A runtime profile change rolls the workloads instead of being ignored until the next unrelated restart. | One extra Deployment plus its RBAC. |
| `operandTls.enabled` | The server's own listener actually follows the cluster profile, which is what makes RHTPA adherent. Required on a `StrictAllComponents` cluster. | On its own it can *loosen* the listener — see below. The reconciler owns a ConfigMap in the release namespace. |
| `pqc.enabled` | Guarantees the hybrid post-quantum group is offered rather than leaving it to OpenSSL's defaults, even on a profile that omits it. | The profile's other key-exchange groups are replaced by `X25519MLKEM768:X25519`. It does **not** raise the minimum TLS version. |

**`operandTls` tracks the cluster profile, including downwards.** A cluster
on `Old` or `Intermediate` drops the listener from TLS 1.3-only to TLS 1.0/1.2.
That is what honoring a cluster profile means, and `pqc.enabled` does **not**
override it: it pins `HTTP_SERVER_TLS_GROUPS` to `X25519MLKEM768:X25519` but
publishes `HTTP_SERVER_TLS_MIN_VERSION` exactly as the profile states it.

The protocol floor is the cluster administrator's decision. A cluster
deliberately on `Intermediate` to keep TLS 1.2 clients working keeps them
working; raising the floor underneath it would lock out the clients that
profile exists to admit. If you want the listener at TLS 1.3, set the cluster
profile to `Modern` — which is also the platform default from OpenShift 5.1,
so on 5.1+ a cluster that configures nothing is already there.

### What gets deployed

A `tls-configurator` Deployment in the release namespace with its own
ServiceAccount, a ClusterRole/ClusterRoleBinding for the cluster-scoped reads,
and a namespaced Role/RoleBinding to patch the target Deployments. It shares
the operator's *image* but not its pod.

The namespaced resources are named `tls-configurator`. The ClusterRole and
ClusterRoleBinding are cluster-scoped, so they are named
`<release-namespace>-<release-name>-tls-configurator` — a fixed name would be
owned by whichever release installed first, and every later install of the
chart would fail with *"ClusterRole \"tls-configurator\" … exists and cannot be
imported into the current release"*.

The container runs:

```
/usr/local/bin/tls-configurator \
  --action=reconcile \
  --enable-pqc=<pqc.enabled> \
  --target-namespace=<release namespace> \
  --target-deployments=<targetDeployments, comma separated> \
  --resync-period=<resyncPeriod> \
  [--operand-config-map=<operandTls.configMapName>]   # only with operandTls.enabled
```

It reconciles once on startup — which covers what the old pre-install hook Job
used to do — then watches `APIServer/cluster` and reconciles on every change.

### Verify it

```console
# The reconciler is up
oc -n trustify get deployment tls-configurator

# What it decided, including the cluster's adherence policy
oc -n trustify logs deployment/tls-configurator

# The hash it stamped on the operand
oc -n trustify get deployment server \
  -o jsonpath='{.spec.template.metadata.annotations.rhtpa\.io/tls-config-hash}{"\n"}'
```

### Reading the log

Every reconcile line is tagged with what triggered it: `[startup]`,
`[apiserver-event]` (the cluster `APIServer` CR changed) or `[resync]` (the
periodic drift check). On startup the reconciler prints the profile it found in
full, so the log records the cluster state the workloads were stamped from:

```text
Starting TLS reconciler (namespace=trustify, deployments=[server], pqc=false, resync=5m0s, annotation=rhtpa.io/tls-config-hash)
[startup] Cluster tlsAdherence="<unset>": honoring the cluster TLS profile is optional; this operator honors it regardless
[startup] Cluster TLS profile: type=Intermediate adherence=<unset> minVersion=TLS 1.2 pqcRequested=false hash=c998037ab4a9
[startup] Key-exchange groups: X25519MLKEM768, X25519, CurveP256, CurveP384
[startup] Ciphers from the profile (9): TLS_AES_128_GCM_SHA256, ...
[startup] Post-quantum key exchange is NOT active: MinVersion is TLS 1.2, must be TLS 1.3
[startup] TLS config changed for trustify/server (none -> c998037ab4a9), rolling out
[startup] Patched trustify/server pod template rhtpa.io/tls-config-hash=c998037ab4a9 (generation 4, 1 replica(s)): Kubernetes is restarting its pods so they re-read the TLS settings
[startup] Reconcile finished: 1 rolled out, 0 already up to date
```

Afterwards the log is driven by changes, with one deliberate exception: an
`APIServer` event that turns out to change nothing still logs a line saying so,
because an operator who just edited the CR is watching for a reaction and
silence would be indistinguishable from the watch being dead:

```text
[apiserver-event] Observed APIServer "cluster" MODIFIED event, reconciling
[apiserver-event] Cluster TLS profile unchanged (type=Intermediate hash=c998037ab4a9)
[apiserver-event] Reconcile finished: 0 rolled out, 1 already up to date
```

A cluster profile change prints the field-level diff, the rollout it causes, and
the restart landing:

```text
[apiserver-event] Observed APIServer "cluster" MODIFIED event, reconciling
[apiserver-event] Cluster TLS profile changed (hash c998037ab4a9 -> 81aa7cfd3171):
[apiserver-event]   profile type: Intermediate -> Modern
[apiserver-event]   minimum TLS version: TLS 1.2 -> TLS 1.3
[apiserver-event]   ciphers: 9 -> 3 (removed ECDHE-ECDSA-AES128-GCM-SHA256, ...)
[apiserver-event]   post-quantum key exchange: inactive -> active
[apiserver-event] TLS config changed for trustify/server (c998037ab4a9 -> 81aa7cfd3171), rolling out
[apiserver-event] Patched trustify/server pod template rhtpa.io/tls-config-hash=81aa7cfd3171 (generation 5, 1 replica(s)): Kubernetes is restarting its pods so they re-read the TLS settings
[resync] Deployment trustify/server is still restarting pods for the current TLS config (1/1 pods updated, 0 ready, 0 available)
[resync] Deployment trustify/server finished restarting with the current TLS config (1/1 pods updated, 1 ready, 1 available)
```

A resync that finds nothing to do logs nothing, so a steady cluster stays
quiet — silence between the `[startup]` block and the next event means the
profile has not moved. Rollout progress is only followed for restarts the
reconciler itself triggered; a workload that is unhealthy for its own reasons is
not reported here.

Inspect the cluster-side inputs directly with the same binary:

```console
oc -n trustify exec deployment/tls-configurator -- \
  /usr/local/bin/tls-configurator --action=get-cluster      # profile + adherence
oc -n trustify exec deployment/tls-configurator -- \
  /usr/local/bin/tls-configurator --action=get-adherence    # adherence only
oc -n trustify exec deployment/tls-configurator -- \
  /usr/local/bin/tls-configurator --action=show-tlsconfig   # resulting crypto/tls.Config
oc -n trustify exec deployment/tls-configurator -- \
  /usr/local/bin/tls-configurator --action=validate         # PQC / TLS 1.3 check, exits non-zero if not
```

End-to-end test — change the cluster profile and watch the operand roll:

```console
oc patch apiserver cluster --type=merge \
  -p '{"spec":{"tlsSecurityProfile":{"type":"Modern"}}}'
oc -n trustify rollout status deployment/server
```

### `tlsAdherence`

`APIServer/cluster` `.spec.tlsAdherence` (feature gate `TLSAdherence`) states
how strictly the cluster expects components to honor the profile:

| Value | Meaning |
| --- | --- |
| `""` (unset) | No opinion; treated as legacy today. Subject to change. |
| `LegacyAdheringComponentsOnly` | Only components that already honor the profile continue to. |
| `StrictAllComponents` | Every component must honor the profile. |

The configurator itself honors the cluster profile in every mode — it has no
TLS settings of its own that could compete — so no value of `tlsAdherence`
turns it off. The policy also participates in the rollout hash: moving the
cluster from unset to `StrictAllComponents` rolls the target Deployments even
though the profile itself did not change. It is logged and reported by the CLI
actions above.

`StrictAllComponents` does, however, change what the chart will install — see
the next section.

To set it cluster-wide (this is a cluster-admin action, not an RHTPA one):

```console
oc patch apiserver cluster --type=merge \
  -p '{"spec":{"tlsAdherence":"StrictAllComponents"}}'
```

If the gate is off, the field reads back as `""` and `--action=get-adherence`
says so.

### Strict adherence and `operandTls`

On a cluster set to `StrictAllComponents`, the chart **refuses to render**
unless the server's own listener is made to follow the cluster profile. The
check lives in the same `000-validate.yaml` as the platform guard and fires
when all of the following hold:

- the cluster is OpenShift, and
- the server is actually serving TLS (`openshift.useServiceCa`), and
- `APIServer/cluster` `.spec.tlsAdherence` is `StrictAllComponents`, and
- operand publishing is not effectively on — that is, **either**
  `modules.tlsConfigurator.enabled` **or**
  `modules.tlsConfigurator.operandTls.enabled` is `false`, and
- `modules.tlsConfigurator.operandTls.allowDisabled` is `false`.

The reason is that enabling the module is not the same as being adherent. The
reconciler rolls the workloads when the profile changes, but without
`operandTls` the server comes back serving the profile compiled into trustd —
TLS 1.3 only — whatever the cluster asked for. A strict cluster with publishing
off is quietly non-adherent, which is the exact state the policy exists to rule
out.

To satisfy it, use the [recommended configuration](#recommended-configuration)
— at minimum:

```yaml
spec:
  modules:
    tlsConfigurator:
      enabled: true
      operandTls:
        enabled: true
```

To install non-adherent anyway:

```yaml
spec:
  modules:
    tlsConfigurator:
      operandTls:
        allowDisabled: true
```

**Two things to know about the timing.**

First, `enabled: true` alone does not clear this check. An install that turned
the module on and left everything else at defaults is still caught —
`operandTls.enabled` defaults to `false` independently. That is the most common
configuration, so expect it to be the one that trips.

Second, the check runs on **every reconcile, not just at install**. It uses
Helm's `lookup`, which returns nothing under `helm template` and `--dry-run`
(so the check is skipped there), but returns the live CR on the operator's
render path. The operator re-renders roughly every minute. So an admin
tightening the cluster to `StrictAllComponents` will, within about a minute,
start failing reconciles on *every* existing RHTPA install on that cluster that
has not opted in or set the escape hatch.

That failure is not destructive — Helm stops applying, but the running pods
keep serving. What stops is drift correction, and no further CR edit can be
applied until the condition is resolved. It surfaces as an RHTPA operator
error rather than as a cluster-side compliance warning, so if you are planning
a move to `StrictAllComponents`, set `operandTls.enabled: true` (or
`operandTls.allowDisabled: true`) on the existing CRs **first**.

### Post-quantum key exchange

Two independent sources, and you usually want to understand both:

1. **The cluster profile already carries groups.** `TLSProfileSpec.groups`
   exists in the OpenShift API, and the built-in `Old`, `Intermediate` and
   `Modern` profiles all list `X25519MLKEM768` first. The configurator honors
   those whether or not you enable `pqc`.
2. **`pqc.enabled: true` guarantees the group.** It pins `CurvePreferences`
   to `[X25519MLKEM768, X25519]` even on a profile that omits the hybrid
   group, and the choice is part of the rollout hash so toggling it rolls the
   workloads. It does **not** raise `MinVersion`: hybrid key exchange is only
   defined for TLS 1.3, but the protocol floor belongs to the cluster profile,
   so on a profile that also permits older versions the group applies to the
   TLS 1.3 handshakes and the rest stay reachable.

A cluster on the `Modern` profile is already post-quantum without
`pqc.enabled`. Use `--action=validate` to see which of the two you are relying
on — it reports the cluster profile and the hardened variant separately, and
judges compliance on the cluster profile alone.

Both of those are about the configurator's *own* `tls.Config`, and about the
hash it stamps. Reaching the **workloads'** key exchange needs
[`operandTls.enabled`](#honoring-the-profile-inside-the-workloads), which
writes `HTTP_SERVER_TLS_GROUPS`. Without it the RHTPA pods fall back to
OpenSSL's default group list — which on the RHEL 10 images (OpenSSL 3.5)
already offers `X25519MLKEM768` first, so hybrid key exchange usually happens
anyway. It is just not *enforced*, and nothing reports on it.

Pushing the group onto the **router** is a separate, non-default action
(`--action=update --enable-pqc` against a `Custom` profile). It writes
`TLSProfileSpec.groups`, which is gated on `TLSGroupPreferences`; the
configurator reads the `FeatureGate` CR first and skips the field, with a log
line, when the gate is off. The reconcile Deployment never does this.

### Honoring the profile inside the workloads

Restarting a pod only helps if the pod reads something different when it comes
back. By default it does not: `trustd` has no way to reach the cluster
`APIServer` CR, and falls back to the TLS profile compiled into it — Mozilla
*modern*, TLS 1.3 only — whatever the cluster asked for. The reconciler rolls
the workloads on a profile change, and they come back serving exactly what
they served before.

`operandTls.enabled: true` closes that gap:

```yaml
spec:
  modules:
    tlsConfigurator:
      enabled: true
      operandTls:
        enabled: true
```

The reconciler then renders the resolved profile into a ConfigMap
(`tls-configurator-profile` by default, `operandTls.configMapName` to change
it) in the release namespace, which the server Deployment reads through an
optional `envFrom`:

| Variable | From |
| --- | --- |
| `HTTP_SERVER_TLS_SECURITY_PROFILE` | always `custom` — trustd's built-in `old`/`intermediate`/`modern` are Mozilla profiles that ignore the settings below |
| `HTTP_SERVER_TLS_MIN_VERSION` | `minTLSVersion`, exactly as the profile states it — `pqc.enabled` does not raise it |
| `HTTP_SERVER_TLS_CIPHERS` | the profile's TLS 1.2 ciphers; omitted when the minimum is 1.3, where they can never be negotiated |
| `HTTP_SERVER_TLS_CIPHERSUITES` | the profile's TLS 1.3 suites (the `TLS_*` entries) |
| `HTTP_SERVER_TLS_GROUPS` | `groups`, or `X25519MLKEM768:X25519` under `pqc.enabled` |

Five things are worth knowing before you turn it on.

- **It can loosen the listener, not only tighten it.** Out of the box the
  server is TLS 1.3-only. A cluster on `Old` or `Intermediate` will bring it
  down to TLS 1.0 or 1.2 with the matching cipher list — which is the point of
  honoring a cluster profile, but it is a real change to what the server
  accepts.
- **It is what `tlsAdherence: StrictAllComponents` asks for.** With it off,
  the server's own listener does not adhere, whatever the policy says — and on
  such a cluster the chart refuses to render at all. See [Strict adherence and
  `operandTls`](#strict-adherence-and-operandtls).
- **`pqc.enabled` does not prevent the loosening.** It pins the key-exchange
  groups, but the published minimum version stays the profile's. If you do not
  want the listener below TLS 1.3, set the cluster profile to `Modern` — the
  platform default from OpenShift 5.1 — rather than expecting this module to
  override the cluster.
- **The ConfigMap belongs to the reconciler.** It is created and rewritten at
  runtime, not templated by the chart, because the operator re-renders the
  chart periodically and would overwrite it. Hand edits are reverted on the
  next pass. The `envFrom` is `optional: true`, so pods start normally in the
  window before it first appears, and are rolled once it does.
- **Off OpenShift it does nothing**, for the reason in [Why the platform
  matters](#why-the-platform-matters): there is no TLS listener in the pod to
  configure.

Verify what the workloads actually got:

```console
oc -n trustify get configmap tls-configurator-profile -o jsonpath='{.data}' | jq
```

### Installing without the module

Supported, and nothing needs acknowledging — `enabled: false` is the chart
default:

```yaml
spec:
  modules:
    tlsConfigurator:
      enabled: false
```

You are accepting that a runtime change to the cluster-wide TLS profile will
not roll the RHTPA workloads — they keep serving with the settings they started
with until something else restarts them.

This used to fail on OpenShift >= 4.22 unless `allowDisabled: true` was also
set. It no longer does, and that key has been removed; a values file still
setting it is accepted and ignored.

### Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| Install fails: *"enabled is true, but this cluster does not expose the OpenShift API"* | `route.openshift.io/v1` not detected | If it really is OpenShift, force detection with `.spec.openshift.enabled: true` |
| Install fails: *"this cluster sets `tlsAdherence: StrictAllComponents`, but the RHTPA server would not honor the cluster TLS profile"* | Strict cluster with `operandTls.enabled` off — including when `enabled` is already `true` | Set `operandTls.enabled: true`, or `operandTls.allowDisabled: true` to accept non-adherence. See [Strict adherence](#strict-adherence-and-operandtls) |
| Reconciles start failing on **existing** installs, all at once | Someone just set the cluster to `StrictAllComponents`; the guard runs on every re-render, not only at install | Same fix, applied to each CR. Pods keep serving meanwhile — only drift correction stops |
| Logs: `failed to read cluster TLS settings: ... forbidden` | ClusterRole not bound, or the operator could not grant it | Check `oc get clusterrole <release-namespace>-<release-name>-tls-configurator` and the operator's own `rhtpa-rbac-manager` — a Helm operator can only grant permissions it holds |
| Install fails: *"ClusterRole \"tls-configurator\" … exists and cannot be imported into the current release"* (or the same for `ClusterRoleBinding`) | Left over from a release installed before the cluster-scoped resources were named per release | Delete both, as **two separate commands** — see below. Do this only after every release has been upgraded to a chart that uses the per-release names |
| Logs: `Custom TLS profile specified but Custom field is nil` | Cluster profile is `type: Custom` with no `custom:` block | Fix the cluster's `APIServer/cluster` — this is a malformed cluster config, not an RHTPA one |
| Logs: *"Not supported by Go's crypto/tls"* | Cluster profile names OpenSSL-only ciphers (`DHE-RSA-*`, `AES256-SHA256`, …) | Informational. Go cannot offer those; the rest of the profile still applies |
| Profile changed but `server` did not roll | `targetDeployments` does not match the rendered name | It is `server`, not `rhtpa-server` |

### Removing the legacy unprefixed cluster-scoped resources

Releases installed before the cluster-scoped resources were named per release
left behind a `ClusterRole` and a `ClusterRoleBinding` both plainly named
`tls-configurator`. Either one is enough to fail a later install. Delete them
with **two commands, one per resource type**:

```bash
oc delete clusterrole tls-configurator
oc delete clusterrolebinding tls-configurator
```

Do not combine them as `oc delete clusterrole tls-configurator clusterrolebinding
tls-configurator`. `oc delete` takes a single type followed by a list of names,
so that form asks for three *ClusterRoles* — `tls-configurator`,
`clusterrolebinding` and `tls-configurator` — and silently leaves the
ClusterRoleBinding in place. (`oc delete clusterrole/tls-configurator
clusterrolebinding/tls-configurator`, with slashes, also works, but the two
separate commands are harder to get wrong.)

Only do this once **every** release on the cluster has been upgraded to a chart
that uses the per-release names; a release still on the old chart is using
those objects.

---

## Plain Kubernetes (no OpenShift)

### Configuration

Leave the module off. `enabled: false` is the chart default, so a plain
Kubernetes install needs **no TLS-configurator configuration at all**:

```yaml
apiVersion: rhtpa.io/v1
kind: TrustedProfileAnalyzer
metadata:
  name: rhtpa
  namespace: trustify
spec:
  appDomain: -trustify.example.com
  ingress:
    className: nginx
  # modules.tlsConfigurator is omitted entirely -- it defaults to disabled.
```

Set it explicitly only if some layer of your values turned it on:

```yaml
spec:
  modules:
    tlsConfigurator:
      enabled: false
```

Enabling it fails the install with an explanatory message. That guard is doing
you a favour: the reconciler would fail every attempt against a non-existent
`config.openshift.io/v1` API and retry forever without ever accomplishing
anything.

### Where TLS actually lives here

As described above, the RHTPA server listens on **plain HTTP** on plain
Kubernetes, and TLS terminates at your ingress controller. So TLS is configured
in two places, neither of them the RHTPA CR:

**1. Per-Ingress, for certificates and hostnames** — standard
`networking.k8s.io/v1` `Ingress` TLS, passed through the CR:

```yaml
spec:
  ingress:
    className: nginx
    tls:
      - hosts:
          - server-trustify.example.com
        secretName: rhtpa-server-tls
    additionalAnnotations:
      cert-manager.io/cluster-issuer: letsencrypt-prod
```

**2. Per-ingress-controller, for protocol versions and ciphers** — this is the
plain-Kubernetes analogue of the cluster-wide TLS profile, and it is controller
specific. For ingress-nginx, for example, it is the controller's ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ingress-nginx-controller
  namespace: ingress-nginx
data:
  ssl-protocols: "TLSv1.3"
  ssl-ciphers: "ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384"
```

Consult your controller's documentation for the equivalent (HAProxy, Traefik,
Contour, Istio gateways and cloud load balancers all spell it differently).
Changing it rolls, or hot-reloads, the ingress controller — not the RHTPA
pods — which is precisely why no configurator is needed.

### What you give up, and what you do not

**Give up:** automatic propagation of a cluster-wide TLS policy change to the
RHTPA workloads. There is no such cluster-wide policy object on plain
Kubernetes, so there is nothing to propagate.

**Do not give up:** TLS itself, or post-quantum key exchange. Both are
properties of your ingress controller's TLS termination here. If the controller
negotiates `X25519MLKEM768`, your clients get hybrid PQC — the RHTPA pods are
not part of that handshake.

### If you terminate TLS at the pod anyway

`openshift.useServiceCa` only takes effect on OpenShift, so you cannot turn on
the server's own TLS listener through it on plain Kubernetes. If you need
pod-level TLS there, you are running a configuration the chart does not model;
treat the RHTPA server as a direct-Go TLS server and configure
`HTTP_SERVER_TLS_*` yourself. The TLS configurator will not help — there is
still no cluster-wide profile for it to read.

---

## The OpenShift 5.1 default profile

The reconciler has no version gate and behaves identically on every OpenShift
release. What does change at 5.1 is the cluster itself: an
`APIServer.spec.tlsSecurityProfile` that is left unset means `Intermediate`
(TLS 1.2) up to 5.0 and `Modern` (TLS 1.3 only) from 5.1.

That matters because the upstream resolver the configurator builds on
hard-codes the `Intermediate` fallback. The configurator therefore reads the
`ClusterVersion` CR and resolves an unset profile against it, so on a 5.1
cluster it reports and publishes `Modern`. Without that, enabling `operandTls`
on a 5.1 cluster with no explicit profile would publish an Intermediate
profile to the server and take its listener *down* to TLS 1.2 — the opposite
of honoring the cluster.

Everything else follows from the profile, not the version:

| Cluster profile | Published `HTTP_SERVER_TLS_MIN_VERSION` |
| --- | --- |
| unset, OpenShift >= 5.1 | `1.3` (the platform default, `Modern`) |
| unset, OpenShift <= 5.0 | `1.2` (the platform default, `Intermediate`) |
| `Modern` | `1.3` |
| `Intermediate` | `1.2` |
| `Old` | `1.0` |
| `Custom` | whatever its `minTLSVersion` says |

An explicitly configured profile is honored as written in every row — nothing
promotes `Old`, `Intermediate` or `Custom` to TLS 1.3, including
`pqc.enabled`.

If the cluster version cannot be read (RBAC, an API error), the configurator
logs it and falls back to `Intermediate`, which is the behaviour it had
before. It does not abort the pass.

## A note on detection

Platform detection keys off whether `route.openshift.io/v1` is present
(`trustification.openshift.detect`). Override it in either direction with:

```yaml
spec:
  openshift:
    enabled: true   # or false
```

The chart's adherence check reads the `APIServer` CR with Helm's `lookup`,
which returns nothing during `helm template` and `--dry-run`. In those modes
the cluster state is unknown and the check is **skipped** rather than failed —
do not "fix" that by failing closed, it would break every dry run and the
operator's own rendering paths.

The cluster *version* is no longer read at render time at all; the chart has
no version gate, and the 5.1 default above is resolved by the reconciler at
runtime, where it can read `ClusterVersion` properly.
