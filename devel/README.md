# crc
- crc start --cpus 8 --memory 32768 --disk-size 80
- oc new-project trustify
- NAMESPACE=trustify APP_DOMAIN=-$NAMESPACE.$(oc -n openshift-ingress-operator get ingresscontrollers.operator.openshift.io default -o jsonpath='{.status.domain}')
- oc get secret -n openshift-ingress router-certs-default -o go-template='{{index .data "tls.crt"}}' | base64 -d > tls.crt
- oc create configmap crc-trust-anchor --from-file=tls.crt -n trustify
- rm tls.crt
- 
# Infrastructure with helm
- helm upgrade --install --dependency-update -n $NAMESPACE infrastructure charts/trustify-infrastructure --values values-ocp-no-aws-crc.yaml  --set-string keycloak.ingress.hostname=sso$APP_DOMAIN --set-string appDomain=$APP_DOMAIN

# Rbac
- kubectl apply -f role-ingress.yaml
- kubectl apply -f rolebinding-ingress.yaml
- kubectl apply -f role-job.yaml
- kubectl apply -f rolebinding-job.yaml
