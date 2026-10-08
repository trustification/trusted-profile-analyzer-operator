# crc
```console 
  crc start --cpus 8 --memory 32768 --disk-size 80
  oc login -u kubeadmin https://api.crc.testing:6443
  oc new-project trustify
  oc get secret -n openshift-ingress router-certs-default -o go-template='{{index .data "tls.crt"}}' | base64 -d > tls.crt
  oc create configmap crc-trust-anchor --from-file=tls.crt -n trustify
  rm tls.crt
```

# Infrastructure deployment with helm-chart
  This will deploy postgresql, keycloak and otelcol
- Download the repo https://github.com/trustification/trustify-helm-charts/ 
```console 
  NAMESPACE=trustify APP_DOMAIN=-$NAMESPACE.$(oc -n openshift-ingress-operator get ingresscontrollers.operator.openshift.io default -o jsonpath='{.status.domain}')
```
then to install helm-chart
```console 
  helm upgrade --install --dependency-update -n $NAMESPACE infrastructure charts/trustify-infrastructure --values values-ocp-no-aws-crc.yaml  --set-string keycloak.ingress.hostname=sso$APP_DOMAIN --set-string appDomain=$APP_DOMAIN
```
or if you want to enable metrics and tracing
```console 
  helm upgrade --install --dependency-update -n $NAMESPACE infrastructure charts/trustify-infrastructure --values values-ocp-no-aws-crc.yaml  --set-string keycloak.ingress.hostname=sso$APP_DOMAIN --set-string appDomain=$APP_DOMAIN --set tracing.enabled=true --set metrics.enabled=true --set-string collector.endpoint="http://infrastructure-otelcol:4317"
```

# Container repository
- Replace ```registry.redhat.io/rhtpa/rhtpa-rhel10-operator``` occurrences with your registry like quay.io/<your_username>/rhtpa-rhel10-operator 
  or map on the crc/ocp with a registry mirroring 
  
```console
apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: rhtap-tp
spec:
  imageDigestMirrors:
    - mirrorSourcePolicy: AllowContactingSource
      mirrors:
        - quay.io/<your_username>/rhtpa-rhel10
      source: registry.redhat.io/rhtpa/rhtpa-rhel10
 ```
  

- Replace IF NEEDED the image ```registry.redhat.io/rhtpa/rhtpa-rhel10``` in the makefile 

# Builds the operator
```console
  make podman-build
  make podman-push
 ```
update the operator sha and then run
```console
  make bundle-build
  make bundle-push
  operator-sdk run bundle -n trustify quay.io/<your_username>/rhtpa-rhel10-operator-bundle:v3.2.0
```

# Deploy an instance for development or demo
From the UI or from cli with the values of trustify of namespace and services configured from helm-chart infrastructure
Note: Storage filesystem is only for development/demo installation purposes, storage filesystem isn't designed for production or upgrades between different versions

```console
kubectl apply -f trusted-profile-analyzer-demo.yaml
```

# TLS configurator — configuring it per platform

The `tlsConfigurator` module is OpenShift-only. Which setting is correct depends
on where you deployed the operator, and the chart **enforces** the matrix at
install time rather than just documenting it.

| Platform | `.spec.modules.tlsConfigurator.enabled` | What happens otherwise |
|---|---|---|
| OpenShift >= 4.22 | `true` — required | Install fails unless you also set `allowDisabled: true` |
| OpenShift < 4.22 | optional | Nothing — it works there, it just isn't mandatory |
| Plain Kubernetes | `false` — the default | Install fails if you enable it |

Full reference, including `tlsAdherence` and post-quantum:
[`docs/tls-configurator/DEPLOYMENT.md`](../docs/tls-configurator/DEPLOYMENT.md).

## Which platform am I on?

```console
# OpenShift if this returns a version; empty/error means plain Kubernetes
kubectl get clusterversion version -o jsonpath='{.status.desired.version}{"\n"}'

# The cluster-wide TLS profile and adherence policy (OpenShift only)
kubectl get apiserver cluster -o jsonpath='{.spec.tlsSecurityProfile}{"\n"}'
kubectl get apiserver cluster -o jsonpath='{.spec.tlsAdherence}{"\n"}'
```

## OpenShift 4.22+

Add this to your `TrustedProfileAnalyzer` CR (`trusted-profile-analyzer-demo.yaml`
already uses the `modules:` block — put it alongside `server:` and `importer:`):

```yaml
spec:
  modules:
    tlsConfigurator:
      enabled: true
      # Deployments to roll when the cluster TLS configuration changes.
      # These are *rendered* names: the server Deployment renders as "server".
      targetDeployments:
        - server
      # Optional: force TLS 1.3 + hybrid post-quantum X25519MLKEM768.
      pqc:
        enabled: true
      # Optional: periodic drift correction (default 5m).
      resyncPeriod: 5m
```

Apply and check the reconciler came up:

```console
kubectl apply -f devel/trusted-profile-analyzer-demo.yaml
oc -n trustify rollout status deployment/tls-configurator
oc -n trustify logs deployment/tls-configurator
```

The log reports the cluster's `tlsAdherence` policy on the first reconcile.
Inspect the inputs and the resulting `crypto/tls.Config` with the same binary:

```console
oc -n trustify exec deployment/tls-configurator -- \
  /usr/local/bin/tls-configurator --action=get-cluster
oc -n trustify exec deployment/tls-configurator -- \
  /usr/local/bin/tls-configurator --action=get-adherence
oc -n trustify exec deployment/tls-configurator -- \
  /usr/local/bin/tls-configurator --action=show-tlsconfig
oc -n trustify exec deployment/tls-configurator -- \
  /usr/local/bin/tls-configurator --action=validate
```

Verify the whole loop — change the cluster profile, watch the operand roll:

```console
oc -n trustify get deployment server \
  -o jsonpath='{.spec.template.metadata.annotations.rhtpa\.io/tls-config-hash}{"\n"}'

oc patch apiserver cluster --type=merge \
  -p '{"spec":{"tlsSecurityProfile":{"type":"Modern"}}}'

oc -n trustify rollout status deployment/server
oc -n trustify get deployment server \
  -o jsonpath='{.spec.template.metadata.annotations.rhtpa\.io/tls-config-hash}{"\n"}'   # changed
```

Tightening the cluster adherence policy rolls the workloads too, even though
the profile itself is unchanged:

```console
oc patch apiserver cluster --type=merge \
  -p '{"spec":{"tlsAdherence":"StrictAllComponents"}}'
oc -n trustify rollout status deployment/server
```

To install on 4.22+ *without* the module, acknowledge it explicitly — otherwise
the install is refused:

```yaml
spec:
  modules:
    tlsConfigurator:
      enabled: false
      allowDisabled: true
```

CRC note: `crc` tracks a recent OpenShift, so check `clusterversion` above. If
it is 4.22+, the module is required and an install without it will fail with an
explanatory message.

## Plain Kubernetes (kind, minikube, EKS/GKE/AKS)

Leave the module out of the CR entirely — disabled is the default, so **no TLS
configurator configuration is needed**:

```yaml
spec:
  appDomain: -trustify.example.com
  ingress:
    className: nginx
  # no modules.tlsConfigurator block
```

Enabling it fails the install: there is no `config.openshift.io/v1` APIServer CR
to read, so the reconciler would retry forever without accomplishing anything.

There is a second reason it would be pointless. Off OpenShift the chart resolves
`openshift.useServiceCa` to `false`, so the server never gets
`HTTP_SERVER_TLS_ENABLED` and listens on plain HTTP — TLS terminates at your
ingress controller, and the RHTPA pods are not part of the handshake at all.

So on plain Kubernetes, TLS is configured in two places, neither of them the
TLS configurator:

```yaml
# 1. Certificates and hostnames -- via the CR, standard Ingress TLS
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

```yaml
# 2. Protocol versions and ciphers -- on the ingress controller itself.
#    This is the plain-Kubernetes analogue of OpenShift's cluster-wide TLS
#    profile. Example for ingress-nginx; other controllers differ.
apiVersion: v1
kind: ConfigMap
metadata:
  name: ingress-nginx-controller
  namespace: ingress-nginx
data:
  ssl-protocols: "TLSv1.3"
  ssl-ciphers: "ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384"
```

Changing that rolls the ingress controller, not the RHTPA pods — which is
exactly why no configurator is needed here.

If detection is wrong for your cluster (it keys off `route.openshift.io/v1`),
override it in either direction with `.spec.openshift.enabled: true|false`.

# Cleanup an instance
From the UI
- Delete deployment rhtpa-operator-controller-manager 
- Delete subscription rhtpa-operator-v1-0-0-sub
- Delete catalogSource rhtpa-operator-catalog

