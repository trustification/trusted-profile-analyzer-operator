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
  helm upgrade --install --dependency-update -n $NAMESPACE infrastructure charts/trustify-infrastructure --values values-ocp-no-aws-crc.yaml  --set-string keycloak.ingress.hostname=sso$APP_DOMAIN --set-string appDomain=$APP_DOMAIN
```

# RBAC
```console
  kubectl apply -f role-ingress.yaml
  kubectl apply -f rolebinding-ingress.yaml
  kubectl apply -f role-job.yaml
  kubectl apply -f rolebinding-job.yaml
```

# Container repository
- Replace ```registry.redhat.io/rhtpa/rhtpa-rhel9-operator``` occurrences with your registry like quay.io/<your_username>/rhtpa-rhel9-operator 
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
        - quay.io/<your_username>/rhtpa-trustification-service-rhel9
      source: registry.redhat.io/rhtpa/rhtpa-trustification-service-rhel9
 ```
  

- Replace IF NEEDED the image ```registry.redhat.io/rhtpa/rhtpa-trustification-service-rhel9``` in the makefile 

# Builds the operator
```console
  make podman-build
  make podman-push
  make bundle-build
  make bundle-push
  operator-sdk run bundle -n trustify quay.io/<your_username>/trustification-operator-bundle:v1.0.0
```

# Deploy an instance
From the UI or from cli with the values of trustify of namespace and services configured from helm-chart infrastructure
```console
kubectl apply -f trusted-profile-analyzer-demo.yaml
```
