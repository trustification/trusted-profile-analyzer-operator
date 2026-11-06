# Red Hat Trusted Profile Analyzer Helm Chart

Red Hat Trusted Profile Analyzer is a product that provides a single pane of glass for aggregating, managing, and analyzing the composition and security documentation of custom, 3rd party, and open source software without slowing down development or increasing operational complexity.  

Designed for development and security teams to gain visibility and insights into the risk profiles of the codebases comprising their application portfolio, ensuring security threats and vulnerabilities are minimized and caught promptly.  

This Helm chart requires some Amazon Web Services resources to be provided by the user.

For detailed instructions on how to deploy this Helm chart, please check [Red Hat Trusted Profile Analyzer documentation](https://docs.redhat.com/en/documentation/red_hat_trusted_profile_analyzer/3.0/html/deployment_guide/index).

## TLS configurator (`modules.tlsConfigurator`)

The TLS configurator is an OpenShift-only module. It watches the cluster-wide
TLS security profile (the `config.openshift.io` `APIServer` CR named `cluster`)
and, when that profile changes at runtime, stamps a new
`rhtpa.io/tls-config-hash` annotation on the pod template of each target
Deployment so Kubernetes rolls it and the pods re-read the new TLS settings.
Nothing else in the chart does this — Kubernetes does not restart pods when a
cluster configuration object changes.

| Platform | Setting | Why |
| --- | --- | --- |
| OpenShift >= 4.22 (incl. 5.x) | **`enabled: true` (required)** | The cluster-wide TLS profile can change at runtime; without the configurator the RHTPA workloads keep serving with the old settings until something else rolls them. The chart refuses to install with the module disabled. |
| OpenShift < 4.22 | optional | The reconciler works there too — it simply is not mandatory, because the cluster-wide TLS profile is not expected to change under you. |
| Plain Kubernetes | **`enabled: false` (required)** | `config.openshift.io/v1` does not exist, so the reconciler would fail every attempt and retry forever. The chart refuses to install with the module enabled. |

### Enable it (OpenShift)

```yaml
modules:
  tlsConfigurator:
    enabled: true
    # Deployments in the release namespace to roll when the TLS profile changes.
    targetDeployments:
      - server
    # Optional: force TLS 1.3 + the hybrid post-quantum X25519MLKEM768 key
    # exchange, and include that choice in the rollout hash.
    pqc:
      enabled: true
    # Optional: make the workloads' own HTTPS listener honor the cluster
    # profile, instead of only being restarted when it changes.
    operandTls:
      enabled: true
    # Optional: periodic drift correction.
    resyncPeriod: 5m
```

`operandTls.enabled` is the difference between restarting the workloads and
*reconfiguring* them. With it off — the default — the RHTPA server keeps
serving with the TLS profile compiled into `trustd` (Mozilla modern: TLS 1.3
only) no matter what the cluster profile says; a profile change rolls the pods
and they come back identical. With it on, the reconciler writes the resolved
profile into a ConfigMap that the server reads as
`HTTP_SERVER_TLS_MIN_VERSION`, `HTTP_SERVER_TLS_CIPHERS`,
`HTTP_SERVER_TLS_CIPHERSUITES` and `HTTP_SERVER_TLS_GROUPS`, so the listener
tracks the cluster. Note that this can *relax* the listener as well as tighten
it: a cluster on the `Old` profile will take the server down to TLS 1.0.
Enabling `pqc` alongside it prevents that — PQC raises the published minimum
back to TLS 1.3 and pins the groups — at the cost of rejecting TLS 1.2 clients
the cluster profile would have allowed.
See `docs/tls-configurator/DEPLOYMENT.md` for the details.

`pqc.enabled` is a floor, not the only source of post-quantum settings. The
cluster's own TLS profile carries key-exchange groups
(`TLSProfileSpec.groups`), and the built-in `Old`, `Intermediate` and `Modern`
profiles all list `X25519MLKEM768` first — the configurator honors those
whether or not `pqc.enabled` is set. Turning it on additionally forces TLS 1.3,
which is what makes the hybrid group mandatory rather than merely preferred.

Installed through the operator, the configurator runs the operator's own image —
`watches.yaml` expands `$RELATED_IMAGE_TLS_CONFIGURATOR` into
`modules.tlsConfigurator.image.fullName`, so there is no separate image to
point at. `values.yaml` therefore pins no default of its own; a standalone
`helm install` with the module enabled has to set
`modules.tlsConfigurator.image.fullName` to an RHTPA operator image, and
rendering fails with a message if it does not.

### Disable it (plain Kubernetes)

`enabled: false` is the chart default, so a plain Kubernetes install needs no
action. If it has been switched on somewhere in your values, turn it back off:

```yaml
modules:
  tlsConfigurator:
    enabled: false
```

### Install-time guards

`templates/init/tls-configure/000-validate.yaml` renders no resources; it only
validates the combination of platform and setting, and fails the install with an
explanatory message when they do not match. Every guard has an escape hatch:

- **Enabled on a cluster with no OpenShift API.** Detection keys off
  `route.openshift.io/v1`. If detection is wrong for your cluster, force it with
  `openshift.enabled: true`.
- **Disabled on OpenShift 4.22+.** To install without the configurator anyway,
  acknowledge the choice with `modules.tlsConfigurator.allowDisabled: true`.
  You are accepting that a runtime TLS profile change will not roll the
  workloads.
- **`operandTls` off on a cluster that requires adherence.** When the
  `APIServer` CR sets `tlsAdherence: StrictAllComponents`, every component has
  to honor the cluster TLS profile — and the RHTPA server only does that when
  `operandTls.enabled` is true; otherwise it serves `trustd`'s compiled-in
  profile. Checked only where the server terminates TLS itself. To install
  non-adherent anyway, acknowledge the choice with
  `modules.tlsConfigurator.operandTls.allowDisabled: true`. The reconciler then
  logs a warning on every pass.

The 4.22 check reads the `ClusterVersion` CR with Helm's `lookup`, and the
adherence check reads the `APIServer` CR the same way. `lookup` returns nothing
during `helm template` and `--dry-run`, so in those modes the cluster state is
unknown and both checks are skipped — rendering never fails on something it
cannot read.

### Why the platform matters

It is not only that `config.openshift.io/v1` is absent off OpenShift. On
OpenShift, `openshift.useServiceCa` (default `true`) makes the chart request a
service-CA certificate and set `HTTP_SERVER_TLS_ENABLED=true`, so the server is
a real TLS server and is in scope for the cluster-wide profile. On plain
Kubernetes that resolves to `false`, the server listens on plain HTTP, and TLS
terminates at your ingress controller — there is no `tls.Config` in the RHTPA
pods for a profile to apply to. Protocol versions and ciphers are configured on
the ingress controller there (for ingress-nginx, `ssl-protocols` /
`ssl-ciphers` in its ConfigMap), and changing those rolls the controller rather
than the RHTPA pods.

More detail:

- [`docs/tls-configurator/DEPLOYMENT.md`](../../docs/tls-configurator/DEPLOYMENT.md)
  — per-platform configuration, verification and troubleshooting.
- [`docs/tls-configurator/TLS_ADHERENCE.md`](../../docs/tls-configurator/TLS_ADHERENCE.md)
  — `tlsAdherence`, the rollout hash, and the upstream packages used.
- [`devel/README.md`](../../devel/README.md) — runnable CRC walkthrough.
