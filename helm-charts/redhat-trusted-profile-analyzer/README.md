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
| OpenShift >= 4.22 | **`enabled: true` (required)** | The cluster-wide TLS profile can change at runtime; without the configurator the RHTPA workloads keep serving with the old settings until something else rolls them. The chart refuses to install with the module disabled. |
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
    # Optional: periodic drift correction.
    resyncPeriod: 5m
```

Installed through the operator, the configurator runs the operator's own image —
`watches.yaml` expands `$RELATED_IMAGE_TLS_CONFIGURATOR` into
`modules.tlsConfigurator.image.fullName`, so there is no separate image to
point at. The value in `values.yaml` only applies to a standalone
`helm install`.

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
explanatory message when they do not match. Both guards have an escape hatch:

- **Enabled on a cluster with no OpenShift API.** Detection keys off
  `route.openshift.io/v1`. If detection is wrong for your cluster, force it with
  `openshift.enabled: true`.
- **Disabled on OpenShift 4.22+.** To install without the configurator anyway,
  acknowledge the choice with `modules.tlsConfigurator.allowDisabled: true`.
  You are accepting that a runtime TLS profile change will not roll the
  workloads.

The 4.22 check reads the `ClusterVersion` CR with Helm's `lookup`, which returns
nothing during `helm template` and `--dry-run`. In those modes the version is
unknown and the check is skipped, so rendering never fails on a version it
cannot read.

More detail: [`docs/tls-configurator/`](../../docs/tls-configurator/).
