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
{{- else if eq .root.Values.cloudProvider "gcp" }}
- name: GOOGLE_APPLICATION_CREDENTIALS
  value: /var/run/secrets/cloud/service_account.json
{{- else if eq .root.Values.cloudProvider "azure" }}
- name: AZURE_CLIENT_ID
  valueFrom:
    secretKeyRef:
      name: {{ include "trustification.cco.secretName" . }}
      key: azure_client_id
- name: AZURE_TENANT_ID
  valueFrom:
    secretKeyRef:
      name: {{ include "trustification.cco.secretName" . }}
      key: azure_tenant_id
- name: AZURE_SUBSCRIPTION_ID
  valueFrom:
    secretKeyRef:
      name: {{ include "trustification.cco.secretName" . }}
      key: azure_subscription_id
- name: AZURE_FEDERATED_TOKEN_FILE
  value: /var/run/secrets/openshift/serviceaccount/token
{{- end }}
{{- end }}
{{- end -}}
