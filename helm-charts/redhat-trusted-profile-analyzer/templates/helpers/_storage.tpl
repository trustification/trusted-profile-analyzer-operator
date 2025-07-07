{{/*
Environment variables required to configure the S3 storage

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{ define "trustification.storage.envVars" -}}
{{- if .module.storage }}
{{- include "_trustification.storage.envVars" ( set (deepCopy .) "storage" .module.storage ) }}
{{- else }}
{{- include "_trustification.storage.envVars" ( set (deepCopy .) "storage" .root.Values.storage ) }}
{{- end }}
{{- end }}

{{/* Internal: env-vars for the evaluated storage */}}
{{ define "_trustification.storage.envVars"}}

{{- include "_trustification.storage.common.envVars" ( set (deepCopy .) "storage" .storage ) }}

{{- if eq .storage.type "filesystem" }}
{{ include "_trustification.storage.filesystem.envVars" . }}
{{- else if eq .storage.type "s3" }}
{{ include "_trustification.storage.s3.envVars" . }}
{{- else }}
{{- fail ".storage.type must either be set to 'filesystem' or 's3'" }}
{{- end }}
{{- end }}

{{/* common storage configuration */}}
{{- define "_trustification.storage.common.envVars" -}}
{{- with .storage.compression }}
- name: TRUSTD_STORAGE_COMPRESSION
  value: {{ . | quote }}
{{- end }}
{{- end }}

{{/* filesystem storage configuration */}}
{{- define "_trustification.storage.filesystem.envVars" -}}
- name: TRUSTD_STORAGE_STRATEGY
  value: fs

- name: TRUSTD_STORAGE_FS_PATH
  value: /data/storage
{{- end }}

{{/* S3 storage configuration */}}
{{- define "_trustification.storage.s3.envVars" -}}

- name: TRUSTD_STORAGE_STRATEGY
  value: s3

- name: TRUSTD_S3_ACCESS_KEY
  {{- include "trustification.common.envVarValue" .storage.accessKey | nindent 2 }}
- name: TRUSTD_S3_SECRET_KEY
  {{- include "trustification.common.envVarValue" .storage.secretKey | nindent 2 }}
- name: TRUSTD_S3_REGION
  {{- include "trustification.common.envVarValue" .storage.region | nindent 2 }}
- name: TRUSTD_S3_BUCKET
  value: {{ .storage.bucket | quote }}

{{- with .storage.pathStyle }}
- name: TRUSTD_S3_PATH_STYLE
  value: {{ . | quote }}
{{- end }}

{{- with .root.Values.tls.additionalTrustAnchor }}
- name: TRUSTD_S3_TRUST_ANCHORS
  value: {{ . | quote }}
{{- end }}

{{- end }}

{{/*
Volume mounts for the filesystem storage.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.storage.volumeMount" }}
{{ $storage := .module.storage | default .root.Values.storage }}
{{- if eq $storage.type "filesystem" }}
- name: storage
  mountPath: /data/storage
{{- end }}
{{- end }}

{{/*
Volume for the filesystem storage.

Arguments (dict):
  * root - .
  * name - name of the service
  * module - module object
*/}}
{{- define "trustification.storage.volume" }}
{{ $storage := .module.storage | default .root.Values.storage }}
{{- if eq $storage.type "filesystem" }}
- name: storage
  persistentVolumeClaim:
    claimName: {{ include "trustification.common.name" ( set (deepCopy .) "name" "storage" ) }}
{{- end }}
{{- end }}

{{/*
Default pod affinity when using the filesystem storage.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.storage.defaultPodAffinity" }}
{{ $storage := .module.storage | default .root.Values.storage }}
{{- if eq $storage.type "filesystem" }}
podAffinity:
  requiredDuringSchedulingIgnoredDuringExecution:
    - topologyKey: "kubernetes.io/hostname"
      labelSelector:
        matchLabels:
          {{/* we need to select the "server" pod and align with its host */}}
          {{- include "trustification.common.selectorLabels" ( mergeOverwrite (deepCopy .) (dict "name" "server" "component" "server") ) | nindent 10 }}
{{- end }}
{{- end }}
