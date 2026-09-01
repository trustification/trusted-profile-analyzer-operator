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
  operator-sdk run bundle -n trustify quay.io/<your_username>/rhtpa-rhel10-operator-bundle:v3.1.0
```

# Deploy an instance for development or demo
From the UI or from cli with the values of trustify of namespace and services configured from helm-chart infrastructure
Note: Storage filesystem is only for development/demo installation purposes, storage filesystem isn't designed for production or upgrades between different versions

```console
kubectl apply -f trusted-profile-analyzer-demo.yaml
```

# Cloud Credential Operator (CCO) integration

Use this when you want the operator to obtain S3 (and optionally RDS IAM) credentials
through OpenShift CCO instead of static access keys in the CR.

## Check the cluster's CCO mode

```console
oc get cloudcredential cluster -o jsonpath='{.spec.credentialsMode}'
```

If it returns `Manual` (the default on STS/WIF clusters), CCO does **not** turn
`CredentialsRequest` objects into Secrets automatically. The operator still renders a
`CredentialsRequest` when `spec.cloudProvider` is set, but CCO ignores it — you must
pre-provision the IAM role and the Secret yourself with `ccoctl` before deploying the CR.
This matches `ccoMode: manual` (STS) in the CR.

## Naming constraint

The pods mount a Secret named `<cr-name>-cloud-creds` in the CR's namespace. For the demo
CR `rhtpa-demo` in namespace `trustify`, the Secret must be `rhtpa-demo-cloud-creds`.
The standalone `devel/cco/credentialRequest.yaml` must produce exactly that `secretRef`.

## 1. Prepare devel/cco/credentialRequest.yaml

Set the `secretRef` to match `<cr-name>-cloud-creds` / the CR namespace, and keep
`cloudTokenPath` for manual/STS mode:

```yaml
apiVersion: cloudcredential.openshift.io/v1
kind: CredentialsRequest
metadata:
  name: rhtpa-demo-cloud-creds
  namespace: openshift-cloud-credential-operator
spec:
  secretRef:
    name: rhtpa-demo-cloud-creds      # must equal <cr-name>-cloud-creds
    namespace: trustify               # the CR's namespace
  cloudTokenPath: /var/run/secrets/openshift/serviceaccount/token
  providerSpec:
    apiVersion: cloudcredential.openshift.io/v1
    kind: AWSProviderSpec
    statementEntries:
      - effect: Allow
        action:
          - "s3:GetObject"
          - "s3:PutObject"
          - "s3:DeleteObject"
          - "s3:ListBucket"
          - "s3:GetBucketLocation"
          - "s3:ListBucketMultipartUploads"
          - "s3:AbortMultipartUpload"
          - "s3:ListMultipartUploadParts"
        resource: "*"
```

## 2. Provision the IAM role and Secret with ccoctl (before deploying the CR)

```console
# Extract the ccoctl binary matching your cluster (once)
RELEASE_IMAGE=$(oc get clusterversion version -o jsonpath='{.status.desired.image}')
CCO_IMAGE=$(oc adm release info --image-for='cloud-credential-operator' "$RELEASE_IMAGE")
oc image extract "$CCO_IMAGE" --file="/usr/bin/ccoctl" --confirm && chmod +x ccoctl

# Point ccoctl at a directory containing the CredentialsRequest above
mkdir -p /tmp/credreqs && cp devel/cco/credentialRequest.yaml /tmp/credreqs/

# Create IAM role(s) + the Secret manifest, reusing the cluster's OIDC provider
./ccoctl aws create-iam-roles \
  --name=rhtpa-demo \
  --region=<your-region> \
  --credentials-requests-dir=/tmp/credreqs \
  --identity-provider-arn=<cluster-oidc-provider-arn> \
  --output-dir=/tmp/cco-output

# Apply the generated Secret manifest to the cluster
oc apply -f /tmp/cco-output/manifests/trustify-rhtpa-demo-cloud-creds-credentials.yaml
```

`ccoctl` prints the **role ARN** it created — copy it into `spec.cloudCredentials.aws.stsIAMRoleARN`
in the CR (next step).

## 3. Enable CCO in devel/trusted-profile-analyzer-ocp.yaml

Add the CCO block to `spec` and switch storage from `filesystem` to `s3` (CCO credentials
are only used by S3/RDS). Do **not** set `storage.accessKey`/`storage.secretKey` — in
manual/STS mode the AWS SDK uses the projected SA token and role instead.

```yaml
spec:
  appDomain: -change-me

  cloudProvider: aws
  ccoMode: manual
  cloudCredentials:
    aws:
      statementEntries:
        - effect: Allow
          action:
            - "s3:GetObject"
            - "s3:PutObject"
            - "s3:DeleteObject"
            - "s3:ListBucket"
            - "s3:GetBucketLocation"
            - "s3:ListBucketMultipartUploads"
            - "s3:AbortMultipartUpload"
            - "s3:ListMultipartUploadParts"
          resource: "*"
      stsIAMRoleARN: "arn:aws:iam::<ACCOUNT_ID>:role/<role-name>"   # from ccoctl output

  storage:
    type: s3
    bucket: <your-bucket-name>
    region: <your-region>
```

The operator automatically injects the projected SA token volume, the
`/var/run/secrets/cloud` mount, and `AWS_SHARED_CREDENTIALS_FILE`,
`AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_ROLE_ARN` — no further changes needed.

## 4. Deploy the CR

```console
kubectl apply -f trusted-profile-analyzer-ocp.yaml
```

## Ordering summary

1. Edit `devel/cco/credentialRequest.yaml` (`secretRef` → `rhtpa-demo-cloud-creds` / `trustify`).
2. Run `ccoctl create-iam-roles` → note the role ARN, apply the generated Secret.
3. Edit `devel/trusted-profile-analyzer-ocp.yaml` (add `cloudProvider`/`ccoMode`/`stsIAMRoleARN`, storage → s3).
4. `kubectl apply` the CR.

> Optional: to also use RDS IAM auth for the database, add `ccoRds.enabled: true` and
> `ccoRds.region`, include `rds-db:connect` in the `statementEntries`, and drop the DB
> password. Note the `create-database`/`create-importers` init jobs use `psql` and still
> require a static DB password; only `migrate-database` (`trustd db migrate`) supports RDS IAM.

# Cleanup an instance
From the UI
- Delete deployment rhtpa-operator-controller-manager 
- Delete subscription rhtpa-operator-v1-0-0-sub
- Delete catalogSource rhtpa-operator-catalog

