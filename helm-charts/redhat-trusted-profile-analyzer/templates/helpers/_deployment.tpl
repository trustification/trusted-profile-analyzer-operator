{{/*
Number of replicas.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.replicas" }}
{{- .module.replicas | default .root.Values.replicas | default 1 }}
{{- end }}

{{/*
Additional pod labels for applications.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.podLabels" }}
{{- include "trustification.application.metrics.podLabels" . }}
{{- end }}

{{/*
Additional pod annotations for applications.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.podAnnotations" }}
{{- include "trustification.application.collector.podAnnotations" . }}
{{- end }}

{{/*
Pod settings

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.pod" }}

{{- with .module.serviceAccountName }}
serviceAccountName: {{ . | quote }}
{{- end }}

{{- end }}

{{/*
Pod affinity settings

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.podAffinity" }}
affinity:
{{- with .module.affinity }}
  {{- . | toYaml | nindent 2 }}
{{- else }}
  {{- include "trustification.storage.defaultPodAffinity" . | nindent 2 }}
{{- end }}

{{- end }}

{{/*
Default container settings

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.container" }}

{{- with .module.resources }}
resources:
  {{- . | toYaml | nindent 2 }}
{{ end }}

{{- end }}
