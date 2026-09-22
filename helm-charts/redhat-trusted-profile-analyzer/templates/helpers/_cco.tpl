{{/*
Cloud Credentials Operator (CCO) helpers for manual/STS/WIF mode.

These templates emit volumes, volume mounts, and environment variables
needed when ccoMode is "manual". For mint/passthrough/default modes
they emit nothing — credentials are handled via _storage.tpl.

Arguments (dict):
  * root - .
*/}}

{{/*
Returns "true" when CCO manual mode is active (cloudProvider set AND ccoMode is "manual").
*/}}
{{- define "trustification.cco.isManualMode" -}}
{{- if and .root.Values.cloudProvider (eq (toString .root.Values.ccoMode) "manual") -}}true{{- end -}}
{{- end -}}

{{/*
Returns the CCO secret name.
*/}}
{{- define "trustification.cco.secretName" -}}
{{ .root.Release.Name }}-cloud-creds
{{- end -}}

{{/*
Volumes for CCO manual mode: CCO credentials secret + projected SA token.
*/}}
{{- define "trustification.cco.volumes" -}}
{{- if eq (include "trustification.cco.isManualMode" .) "true" }}
- name: cloud-credentials
  secret:
    secretName: {{ include "trustification.cco.secretName" . }}
- name: bound-sa-token
  projected:
    sources:
      - serviceAccountToken:
          audience: openshift
          expirationSeconds: 3600
          path: token
{{- end }}
{{- end -}}

{{/*
Volume mounts for CCO manual mode.
*/}}
{{- define "trustification.cco.volumeMounts" -}}
{{- if eq (include "trustification.cco.isManualMode" .) "true" }}
- name: cloud-credentials
  mountPath: /var/run/secrets/cloud
  readOnly: true
- name: bound-sa-token
  mountPath: /var/run/secrets/openshift/serviceaccount
  readOnly: true
{{- end }}
{{- end -}}

{{/*
Environment variables for CCO manual mode, branched by cloud provider.
For mint/passthrough/default modes this emits nothing.
*/}}
{{- define "trustification.cco.envVars" -}}
{{- if eq (include "trustification.cco.isManualMode" .) "true" }}
{{- if eq .root.Values.cloudProvider "aws" }}
- name: AWS_SHARED_CREDENTIALS_FILE
  value: /var/run/secrets/cloud/credentials
- name: AWS_WEB_IDENTITY_TOKEN_FILE
  value: /var/run/secrets/openshift/serviceaccount/token
{{- with .root.Values.cloudCredentials.aws.stsIAMRoleARN }}
- name: AWS_ROLE_ARN
  value: {{ . | quote }}
{{- end }}
{{- end }}
{{- end }}
{{- end -}}

{{/*
Returns "true" when CCO RDS IAM authentication is active
(cloudProvider set AND ccoRds.enabled is true).
*/}}
{{- define "trustification.cco.rds.isEnabled" -}}
{{- if and .root.Values.cloudProvider .root.Values.ccoRds -}}
  {{- if .root.Values.ccoRds.enabled -}}true{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Returns "true" when a *specific* database connection should authenticate with an
RDS IAM token.

`ccoRds.enabled` is the default for every connection, but an individual
connection can opt out by setting `iamAuth: false` on its database block. That
matters for the bootstrap admin connection of the create-database job: the RDS
master user commonly keeps password authentication, while the application user
it creates is granted `rds_iam`.

Arguments (dict):
  * root - .
  * database - database object
*/}}
{{- define "trustification.cco.rds.isConnectionIamAuth" -}}
{{- if and (kindIs "map" .database) (hasKey .database "iamAuth") -}}
{{- if .database.iamAuth -}}true{{- end -}}
{{- else -}}
{{- include "trustification.cco.rds.isEnabled" (dict "root" .root) -}}
{{- end -}}
{{- end -}}

{{/*
Static AWS credentials from the CCO secret, for the non-manual CCO modes
(default/mint/passthrough). In manual mode the projected service-account token
and AWS_SHARED_CREDENTIALS_FILE from "trustification.cco.envVars" take over, so
this emits nothing.

Arguments (dict):
  * root - .
*/}}
{{- define "trustification.cco.rds.awsCredentialEnvVars" -}}
{{- if not (eq (include "trustification.cco.isManualMode" .) "true") }}
- name: AWS_ACCESS_KEY_ID
  valueFrom:
    secretKeyRef:
      name: {{ include "trustification.cco.secretName" . }}
      key: aws_access_key_id
- name: AWS_SECRET_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "trustification.cco.secretName" . }}
      key: aws_secret_access_key
{{- end }}
{{- end -}}

{{/*
Environment variables for RDS IAM authentication via CCO.
Emits TRUSTD_DB_IAM_AUTH, TRUSTD_DB_IAM_REGION, and — for non-manual modes —
AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY from the CCO secret so the
AWS SDK can generate RDS auth tokens.

Arguments (dict):
  * root - .
  * database - (optional) database object, to honour a per-connection opt-out
*/}}
{{- define "trustification.cco.rds.envVars" -}}
{{- if eq (include "trustification.cco.rds.isConnectionIamAuth" (dict "root" .root "database" (.database | default .root.Values.database))) "true" }}
- name: TRUSTD_DB_IAM_AUTH
  value: "true"
{{/* trustd reads the region from TRUSTD_DB_IAM_REGION (clap arg `--db-region`) */}}
{{- with .root.Values.ccoRds.region }}
- name: TRUSTD_DB_IAM_REGION
  value: {{ . | quote }}
{{- end }}
{{- include "trustification.cco.rds.awsCredentialEnvVars" (dict "root" .root) }}
{{- end }}
{{- end -}}

{{/*
Volumes needed by the token init container: the in-memory volume carrying the
minted token to the job container, plus the CCO credential volumes the init
container itself needs in manual mode.

Only for pods that do not already include "trustification.cco.volumes" — that
is, the psql-based jobs. Rendered only when the connection uses IAM
authentication.

Arguments (dict):
  * root - .
  * database - database object
*/}}
{{- define "trustification.cco.rds.tokenVolumes" -}}
{{- if eq (include "trustification.cco.rds.isConnectionIamAuth" .) "true" }}
- name: rds-auth-token
  emptyDir:
    medium: Memory
    sizeLimit: 1Mi
{{- include "trustification.cco.volumes" (dict "root" .root) }}
{{- end }}
{{- end -}}

{{/*
Mount for the token volume.

Arguments (dict):
  * root - .
  * database - database object
*/}}
{{- define "trustification.cco.rds.tokenVolumeMount" -}}
{{- if eq (include "trustification.cco.rds.isConnectionIamAuth" .) "true" }}
- name: rds-auth-token
  mountPath: /var/run/rds-auth-token
{{- end }}
{{- end -}}

{{/*
Image running the `rds-auth-token` helper.

Two values feed it, in precedence order:

  * `.Values.ccoRds.tokenImage` — set by the user, in the CR or in values.yaml.
  * `.Values.ccoRds.defaultTokenImage` — injected by the operator from the
    RELATED_IMAGE_RDS_AUTH_TOKEN environment variable (see watches.yaml), which
    the config/rds-auth-token kustomize component puts on the manager. That
    component is optional, so this may be empty or absent.

Fails only when a connection actually needs a token and neither is set.

Arguments (dict):
  * root - .
*/}}
{{- define "trustification.cco.rds.tokenImage" -}}
{{- $ccoRds := .root.Values.ccoRds | default dict -}}
{{- $image := $ccoRds.tokenImage | default $ccoRds.defaultTokenImage -}}
{{- required "RDS IAM authentication for the psql-based jobs requires an image carrying the `rds-auth-token` helper: set .Values.ccoRds.tokenImage, or deploy the operator with RELATED_IMAGE_RDS_AUTH_TOKEN set (the config/rds-auth-token kustomize component)" $image -}}
{{- end -}}

{{/*
Init container that mints an RDS IAM authentication token.

`psql` cannot generate a token itself and the trustd image ships no AWS tooling,
so the jobs that shell out to psql (create-database, create-importers) get the
token from this init container instead. It runs the `rds-auth-token` helper from
the operator image — the same image that carries this chart — and resolves
credentials through the standard AWS chain, so it works in every CCO mode.

The connection parameters come from the very same "trustification.psql.envVars"
block the job container uses, so the token can never be minted for a different
endpoint or user than the one psql connects as.

Arguments (dict):
  * root - .
  * database - database object
*/}}
{{- define "trustification.cco.rds.tokenInitContainer" -}}
{{- if eq (include "trustification.cco.rds.isConnectionIamAuth" .) "true" }}
- name: rds-auth-token
  image: {{ include "trustification.cco.rds.tokenImage" (dict "root" .root) | quote }}
  imagePullPolicy: {{ .root.Values.image.pullPolicy | default "IfNotPresent" }}
  command:
    - /usr/local/bin/rds-auth-token
  env:
    {{- include "trustification.psql.envVars" (dict "root" .root "database" .database) | nindent 4 }}
    - name: AWS_REGION
      value: {{ required "RDS IAM authentication requires .Values.ccoRds.region" .root.Values.ccoRds.region | quote }}
    {{- include "trustification.cco.envVars" (dict "root" .root) | nindent 4 }}
    {{- include "trustification.cco.rds.awsCredentialEnvVars" (dict "root" .root) | nindent 4 }}
  volumeMounts:
    {{- include "trustification.cco.rds.tokenVolumeMount" . | nindent 4 }}
    {{- include "trustification.cco.volumeMounts" (dict "root" .root) | nindent 4 }}
  {{- with .root.Values.ccoRds.resources }}
  resources:
    {{- . | toYaml | nindent 4 }}
  {{- end }}
{{- end }}
{{- end -}}

{{/*
Shell prologue that loads the minted token into PGPASSWORD for psql.
Emits nothing when the connection does not use IAM authentication.

Arguments (dict):
  * root - .
  * database - database object
*/}}
{{- define "trustification.cco.rds.psqlPasswordPrologue" -}}
{{- if eq (include "trustification.cco.rds.isConnectionIamAuth" .) "true" }}
PGPASSWORD="$(cat /var/run/rds-auth-token/token)"
export PGPASSWORD
{{- end }}
{{- end -}}
