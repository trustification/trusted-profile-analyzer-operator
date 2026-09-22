# crc
```console 
  crc start --cpus 8 --memory 32768 --disk-size 80
  oc login -u kubeadmin https://api.crc.testing:6443
  oc new-project trustify
  oc get secret -n openshift-ingress router-certs-default -o go-template='{{index .data "tls.crt"}}' | base64 -d > tls.crt
  oc create configmap crc-trust-anchor --from-file=tls.crt -n trustify
  rm tls.crt
```

# Ingress trust anchor (OCP)

On a cluster whose default ingress certificate is self-signed, the Keycloak route
(`sso<APP_DOMAIN>`) is served with a certificate that trustd does not trust: it only loads
the OpenShift Service CA plus the system roots. The server then fails to start with

```
Error: error sending request for url (https://sso-.../realms/trustify/.well-known/openid-configuration)
Caused by:
  client error (Connect)
  ... certificate verify failed ... (self-signed certificate in certificate chain)
```

Export the router CA and hand it to the chart as an additional trust anchor:

```console
  oc get secret router-ca -n openshift-ingress-operator -o go-template='{{index .data "tls.crt"}}' | base64 -d > tls.crt
  # If that secret does not exist (custom ingress certificate), take the serving chain instead:
  #   oc extract secret/router-certs-default -n openshift-ingress --keys=tls.crt --to=.
  oc create configmap ingress-trust-anchor --from-file=tls.crt -n trustify
  rm tls.crt
```

`trusted-profile-analyzer-ocp-cco-manual-aws-s3.yaml` already references it:

```yaml
  tls:
    additionalTrustAnchor: /etc/trust-anchor/tls.crt

  extraVolumes:
  - name: trust-anchor
    configMap:
      name: ingress-trust-anchor

  extraVolumeMounts:
  - name: trust-anchor
    readOnly: true
    mountPath: /etc/trust-anchor
```

`extraVolumes`/`extraVolumeMounts` are chart-wide, so the server, the importer and the init
jobs all get the mount. Set this in the CR, not on the Deployment — the operator reconciles
the Deployment back every minute. On a cluster with a properly trusted ingress certificate
none of this is needed. `oidc.insecure: true` unblocks the same failure by skipping TLS
verification against the issuer entirely — development only.

Verify the chain with:

```console
  openssl s_client -connect sso$APP_DOMAIN:443 -showcerts </dev/null 2>/dev/null | grep -E "^(depth|verify)"
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
  operator-sdk run bundle -n trustify quay.io/<your_username>/rhtpa-rhel10-operator-bundle:v3.1.1
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

## CCO disabled (the default)

CCO is entirely opt-in: `spec.cloudProvider` is the master toggle. Leave it unset — on a
non-STS cluster, or on plain Kubernetes — and the whole feature renders to nothing:

- No `CredentialsRequest` is created. The template is wrapped in
  `{{- if .Values.cloudProvider }}`, which also keeps the chart installable on clusters
  where the `cloudcredential.openshift.io` CRD does not exist.
- No CCO volumes, mounts or environment variables are injected. `ccoMode: manual` alone
  does nothing: the manual/STS path requires *both* `cloudProvider` and `ccoMode: manual`,
  so there is no `cloud-credentials` volume, no projected `bound-sa-token`, and no
  `AWS_SHARED_CREDENTIALS_FILE` / `AWS_WEB_IDENTITY_TOKEN_FILE` / `AWS_ROLE_ARN` /
  `AWS_REGION`.
- No RDS IAM auth. That path also requires both `cloudProvider` and `ccoRds.enabled`, so
  no `TRUSTD_DB_IAM_AUTH` / `TRUSTD_DB_IAM_REGION` is set.

The pre-CCO configuration contract is unchanged:

- With `storage.type: s3` you must supply `storage.accessKey` / `storage.secretKey`
  yourself (literal values or your own `valueFrom` secret reference); they are rendered
  into `TRUSTD_S3_ACCESS_KEY` / `TRUSTD_S3_SECRET_KEY` as before.
- `database.password` is still required — omitting it fails the render with
  `Missing value for database password`.
- `TRUSTD_DB_SSLMODE` follows `database.sslMode` (default `allow`); it is **not** forced
  to `require`.

Two pieces of the feature ship unconditionally in the bundle but stay inert: the
ClusterRole granting access to `credentialsrequests` (an RBAC rule for an API group that
is absent simply matches nothing) and the `features.operators.openshift.io/token-auth-aws`
annotation (the console only renders the role-ARN field on clusters whose credentials mode
is `Manual`).

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
The standalone `cco/aws-credentialRequest.yaml` must produce exactly that `secretRef`.

## 1. Prepare devel/cco/credentialRequest.yaml

Set the `secretRef` to match `<cr-name>-cloud-creds` / the CR namespace, list the
ServiceAccounts the role may be assumed by, and keep `cloudTokenPath` for manual/STS mode:

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
  serviceAccountNames:
    - default                         # the SA the trustd pods run as
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
          - "rds-db:connect"
        resource: "*"
```

`serviceAccountNames` is what `ccoctl` turns into the role's trust policy — one
`system:serviceaccount:<secretRef.namespace>:<name>` condition per entry. The chart
creates no ServiceAccount and sets no `modules.*.serviceAccountName`, so the pods run as
`default` in the release namespace. If you do set `modules.<name>.serviceAccountName` in
the CR, list those names here instead — a mismatch is not caught at deploy time, it shows
up later as `AccessDenied`/`InvalidIdentityToken` when a pod calls S3.

## 2. Provision the IAM role and Secret with ccoctl (before deploying the CR)

```console
# Extract the ccoctl binary matching your cluster (once)

oc extract secret/pull-secret -n openshift-config --to=- > ~/pull-secret.json
 chmod 600 ~/pull-secret.json

oc get secret/pull-secret -n openshift-config \
    -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d > ~/pull-secret.json

RELEASE_IMAGE=$(oc get clusterversion version -o jsonpath='{.status.desired.image}')

CCO_IMAGE=$(oc adm release info --image-for='cloud-credential-operator' "$RELEASE_IMAGE")

oc image extract $CCO_IMAGE --file="/usr/bin/ccoctl" -a ~/pull-secret.json
  chmod 775 ccoctl

# Point ccoctl at a directory containing the CredentialsRequest above
mkdir -p /tmp/credreqs && cp devel/cco/aws-credentialRequest.yaml /tmp/credreqs/

# Derive the cluster's region and IAM OIDC provider ARN.
# The ARN is arn:aws:iam::<account>:oidc-provider/<issuer-host>, where <issuer-host>
# is the serviceAccountIssuer with the https:// prefix stripped and no trailing slash.
REGION=$(oc get infrastructure cluster -o jsonpath='{.status.platformStatus.aws.region}')

ISSUER=$(oc get authentication cluster -o jsonpath='{.spec.serviceAccountIssuer}')

ACCOUNT=$(oc get secret installer-cloud-credentials -n openshift-image-registry \
    -o jsonpath='{.data.credentials}' | base64 -d | sed -n 's/.*iam::\([0-9]*\):.*/\1/p')

OIDC_ARN="arn:aws:iam::${ACCOUNT}:oidc-provider/${ISSUER#https://}"

# Sanity check (optional, needs the aws CLI): the ARN must appear in this list
aws iam list-open-id-connect-providers

# Create IAM role(s) + the Secret manifest, reusing the cluster's OIDC provider.
# Requires AWS credentials in the environment (AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY
# or AWS_PROFILE) with IAM write permissions.
./ccoctl aws create-iam-roles \
  --name=rhtpa-demo \
  --region="$REGION" \
  --credentials-requests-dir=/tmp/credreqs \
  --identity-provider-arn="$OIDC_ARN" \
  --output-dir=/tmp/cco-output

# Apply the generated Secret manifest to the cluster
oc apply -f /tmp/cco-output/manifests/trustify-rhtpa-demo-cloud-creds-credentials.yaml
```

> **Troubleshooting**
>
> - `failed to get IAM Identity Provider: ... ValidationError: Invalid resource type in
>   ARN` — `--identity-provider-arn` was not an `oidc-provider` ARN; most often the raw
>   issuer URL (with `https://`), a role ARN, or a bucket ARN was passed instead.
> - `error while creating Role policy document ...: CredentialsRequest must provide
>   ServieAccounts to bind the Role policy to` (sic) — `spec.serviceAccountNames` is
>   missing or empty in the CredentialsRequest from step 1.

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

1. Edit `cco/aws-credentialRequest.yaml` (`secretRef` → `rhtpa-demo-cloud-creds` / `trustify`).
2. Run `ccoctl create-iam-roles` → note the role ARN, apply the generated Secret.
3. Edit `trusted-profile-analyzer-ocp-cco-manual-aws-s3.yaml` (add `cloudProvider`/`ccoMode`/`stsIAMRoleARN`, storage → s3).
4. `kubectl apply` the CR.

## Optional: RDS IAM auth on the same role

To reach an RDS database with IAM authentication instead of a static password, use
`trusted-profile-analyzer-ocp-cco-manual-aws-s3-rds.yaml` — a ready-made variant of the CR above. It
adds `ccoRds.enabled: true` / `ccoRds.region`, includes `rds-db:connect` in the
`statementEntries`, and drops `database.password`. Add `rds-db:connect` to
`cco/aws-credentialRequest.yaml` as well, before running `ccoctl`, so the role can
actually log in.

How the three init jobs authenticate:

- `migrate-database` runs `trustd db migrate`, which speaks IAM auth natively. It
  connects as `database.username`, which must therefore own the schema.
- `create-database` and `create-importers` shell out to `psql`, which cannot mint an RDS
  IAM token. Whenever the connection they make uses IAM auth, the chart prepends an
  `rds-auth-token` init container — the operator image, which carries a small helper of
  that name — that mints a token onto a shared in-memory volume; the job reads it into
  `PGPASSWORD`. The image is `ccoRds.tokenImage` if you set it, otherwise
  `ccoRds.defaultTokenImage`, which the operator injects from the optional
  `RELATED_IMAGE_RDS_AUTH_TOKEN` env var (added by the `config/rds-auth-token` kustomize
  component, enabled from `config/default/kustomization.yaml`; see `watches.yaml` for the
  injection). Set `ccoRds.tokenImage` by hand to pin a specific image, to run the chart
  standalone, or when the operator was deployed with that component disabled.

None of this touches a deployment that is not using RDS IAM auth: those jobs still exec
`psql` directly, exactly as before.

IAM auth is chosen per connection, not globally, and `iamAuth` is the switch — set it on
any database block to override `ccoRds.enabled` for that one connection.
`create-database` bootstraps the database and the login role, and the RDS master user it
connects as normally still has a
password — so the CR sets `createDatabase.iamAuth: false` and points the password at a
Secret. The `trustify` role that job creates is granted `rds_iam` instead of a password,
which is why `create-importers`, connecting as that role, does need the init container.

# Cleanup an instance
From the UI
- Delete deployment rhtpa-operator-controller-manager 
- Delete subscription rhtpa-operator-v1-0-0-sub
- Delete catalogSource rhtpa-operator-catalog

