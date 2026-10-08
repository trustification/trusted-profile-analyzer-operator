# Configuring the operator per platform

How to configure the TLS configurator when the operator is deployed on
**OpenShift 4.22 and above** and on **plain Kubernetes**.

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
| OpenShift >= 4.22 | **`true` — required** | Install fails unless you also set `allowDisabled: true` |
| OpenShift < 4.22 | optional (works, not mandatory) | Nothing |
| Plain Kubernetes | **`false` — required** (the chart default) | Install fails if enabled |

The guard lives in
`helm-charts/redhat-trusted-profile-analyzer/templates/init/tls-configure/000-validate.yaml`.
It renders no resources and is deliberately *not* gated on `.enabled`, because
it has to catch the disabled case too.

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
the profile changes. That something is the configurator.

**On plain Kubernetes**, `trustification.openshift.useServiceCa` resolves to
`false` regardless of the value you set. `HTTP_SERVER_TLS_ENABLED` is never
set, so the server listens on plain HTTP and TLS terminates at your ingress
controller. In the vocabulary of the TLS Profile Compliance reference this is
*"plaintext behind a TLS-terminating router: out of scope."* There is no
`tls.Config` in the RHTPA pods to apply a profile to, and no cluster-wide
profile API to read one from.

---

## OpenShift 4.22 and above

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
| `allowDisabled` | `false` | Escape hatch for installing *without* the module. |
| `image.fullName` | the operator's own image | Overridden by the operator via `RELATED_IMAGE_TLS_CONFIGURATOR`; do not set it. |

### Full configuration

```yaml
spec:
  modules:
    tlsConfigurator:
      enabled: true

      # Roll these Deployments when the cluster TLS configuration changes.
      # Add any other TLS-serving workload you introduce.
      targetDeployments:
        - server

      # Force TLS 1.3 and the hybrid post-quantum X25519MLKEM768 group,
      # regardless of what the cluster profile asks for.
      pqc:
        enabled: true

      # Periodic drift correction. Lower it if you want faster recovery from a
      # missed watch event; the watch is the primary trigger either way.
      resyncPeriod: 5m
```

### What gets deployed

A `tls-configurator` Deployment in the release namespace with its own
ServiceAccount, a ClusterRole/ClusterRoleBinding for the cluster-scoped reads,
and a namespaced Role/RoleBinding to patch the target Deployments. It shares
the operator's *image* but not its pod.

The container runs:

```
/usr/local/bin/tls-configurator \
  --action=reconcile \
  --enable-pqc=<pqc.enabled> \
  --target-namespace=<release namespace> \
  --target-deployments=<targetDeployments, comma separated> \
  --resync-period=<resyncPeriod>
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

**There is nothing to configure on the operator side.** The configurator honors
the cluster profile in every mode — it has no TLS settings of its own that
could compete — so no value of `tlsAdherence` turns it off. What the policy
does do is participate in the rollout hash: moving the cluster from unset to
`StrictAllComponents` rolls the target Deployments even though the profile
itself did not change. It is also logged and reported by the CLI actions above.

To set it cluster-wide (this is a cluster-admin action, not an RHTPA one):

```console
oc patch apiserver cluster --type=merge \
  -p '{"spec":{"tlsAdherence":"StrictAllComponents"}}'
```

If the gate is off, the field reads back as `""` and `--action=get-adherence`
says so.

### Post-quantum key exchange

Two independent sources, and you usually want to understand both:

1. **The cluster profile already carries groups.** `TLSProfileSpec.groups`
   exists in the OpenShift API, and the built-in `Old`, `Intermediate` and
   `Modern` profiles all list `X25519MLKEM768` first. The configurator honors
   those whether or not you enable `pqc`.
2. **`pqc.enabled: true` is a floor.** It additionally forces `MinVersion` to
   TLS 1.3 and pins `CurvePreferences` to `[X25519MLKEM768, X25519]`, and the
   choice is part of the rollout hash so toggling it rolls the workloads.
   Hybrid key exchange is only defined for TLS 1.3, which is why the version
   floor comes with it.

A cluster on the `Modern` profile is already post-quantum without
`pqc.enabled`. Use `--action=validate` to see which of the two you are relying
on — it reports the cluster profile and the hardened variant separately, and
judges compliance on the cluster profile alone.

Pushing the group onto the **router** is a separate, non-default action
(`--action=update --enable-pqc` against a `Custom` profile). It writes
`TLSProfileSpec.groups`, which is gated on `TLSGroupPreferences`; the
configurator reads the `FeatureGate` CR first and skips the field, with a log
line, when the gate is off. The reconcile Deployment never does this.

### Installing without the module on 4.22+

Supported, but you have to say so:

```yaml
spec:
  modules:
    tlsConfigurator:
      enabled: false
      allowDisabled: true
```

You are accepting that a runtime change to the cluster-wide TLS profile will
not roll the RHTPA workloads — they keep serving with the settings they started
with until something else restarts them.

### Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| Install fails: *"enabled is false, but this is OpenShift 4.x where the TLS configurator is required"* | 4.22+ with the module off | Set `enabled: true`, or `allowDisabled: true` to accept the consequence |
| Install fails: *"enabled is true, but this cluster does not expose the OpenShift API"* | `route.openshift.io/v1` not detected | If it really is OpenShift, force detection with `.spec.openshift.enabled: true` |
| Logs: `failed to read cluster TLS settings: ... forbidden` | ClusterRole not bound, or the operator could not grant it | Check `oc get clusterrole tls-configurator` and the operator's own `rhtpa-rbac-manager` — a Helm operator can only grant permissions it holds |
| Logs: `Custom TLS profile specified but Custom field is nil` | Cluster profile is `type: Custom` with no `custom:` block | Fix the cluster's `APIServer/cluster` — this is a malformed cluster config, not an RHTPA one |
| Logs: *"Not supported by Go's crypto/tls"* | Cluster profile names OpenSSL-only ciphers (`DHE-RSA-*`, `AES256-SHA256`, …) | Informational. Go cannot offer those; the rest of the profile still applies |
| Profile changed but `server` did not roll | `targetDeployments` does not match the rendered name | It is `server`, not `rhtpa-server` |

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

## OpenShift below 4.22

Optional. The reconciler has no version gate, so `enabled: true` works and
behaves exactly as on 4.22+; it is simply not mandatory, because the
cluster-wide TLS profile is not expected to change under you. The chart does
not force either choice.

Turning it on early is harmless and means one less thing to remember at
upgrade time.

## A note on detection

Platform detection keys off whether `route.openshift.io/v1` is present
(`trustification.openshift.detect`). Override it in either direction with:

```yaml
spec:
  openshift:
    enabled: true   # or false
```

The 4.22 version check reads the `ClusterVersion` CR with Helm's `lookup`,
which returns nothing during `helm template` and `--dry-run`. In those modes
the version is unknown and the check is **skipped** rather than failed — do not
"fix" that by failing closed, it would break every dry run and the operator's
own rendering paths.
