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

# RustFS (S3-compatible storage)
RustFS is an S3-compatible object storage used as the storage backend. It replaces the local filesystem storage, which is not suitable for production or upgrades between versions.

Deploy RustFS into the trustify namespace:
```console
  kubectl apply -f rustfs.yaml -n trustify
  kubectl wait --for=condition=ready pod -l app=rustfs -n trustify --timeout=120s
```

Create the storage bucket using the MinIO mc client:
```console
  kubectl run rustfs-mc --rm -i --restart=Never --image=minio/mc:latest -- sh -c \
    "mc alias set rustfs http://rustfs.trustify.svc.cluster.local:9000 rustfsadmin rustfsadmin && \
     mc mb --ignore-existing rustfs/trustify"
```

The RustFS service will be available at `http://rustfs.trustify.svc.cluster.local:9000` with:
- Access key: `rustfsadmin`
- Secret key: `rustfsadmin`
- Bucket: `trustify`
- Console: port 9001 (accessible via port-forward for debugging)

Note: RustFS uses `emptyDir` for data storage, meaning data is lost when the pod restarts. This is acceptable for development and demo purposes.

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
        - quay.io/<your_username>/rhtpa-trustification-service-rhel10
      source: registry.redhat.io/rhtpa/rhtpa-trustification-service-rhel10
 ```
  

- Replace IF NEEDED the image ```registry.redhat.io/rhtpa/rhtpa-trustification-service-rhel10``` in the makefile 

# Builds the operator
```console
  make podman-build
  make podman-push
 ```
update the operator sha and then run
```console
  make bundle-build
  make bundle-push
  operator-sdk run bundle -n trustify quay.io/<your_username>/rhtpa-rhel10-operator-bundle:v3.0.0
```

# Deploy an instance for development or demo
From the UI or from cli with the values of trustify of namespace and services configured from helm-chart infrastructure.
Make sure RustFS is deployed and the bucket is created (see RustFS section above) before applying:

```console
kubectl apply -f trusted-profile-analyzer-demo.yaml
```

# Cleanup an instance
From the UI
- Delete deployment rhtpa-operator-controller-manager 
- Delete subscription rhtpa-operator-v1-0-0-sub
- Delete catalogSource rhtpa-operator-catalog

