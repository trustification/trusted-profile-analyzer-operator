{{/*
Are metrics enabled?

Arguments (dict):
  * .
*/}}
{{ define "trustification.application.metrics.enabled"}}
{{- .Values.metrics.enabled }}
{{- end }}

{{/*
Pod labels for metrics.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{ define "trustification.application.metrics.podLabels" }}
{{- if eq (include "trustification.application.metrics.enabled" .root ) "true" }}
metrics: "true"
{{ end }}
{{- end }}
