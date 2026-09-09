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
Environment variables for RDS IAM authentication via CCO.
Emits TRUSTD_DB_IAM_AUTH, TRUSTD_DB_REGION, and — for non-manual modes —
AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY from the CCO secret so the
AWS SDK can generate RDS auth tokens.

Arguments (dict):
  * root - .
*/}}
{{- define "trustification.cco.rds.envVars" -}}
{{- if eq (include "trustification.cco.rds.isEnabled" .) "true" }}
- name: TRUSTD_DB_IAM_AUTH
  value: "true"
{{- with .root.Values.ccoRds.region }}
- name: TRUSTD_DB_REGION
  value: {{ . | quote }}
{{- end }}
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
{{- end }}
{{- end -}}
