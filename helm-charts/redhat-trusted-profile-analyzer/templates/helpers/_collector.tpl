{{/*
Check if the collector is enabled.
*/}}
{{ define "trustification.application.collector.enabled" }}
{{- or ( eq ( include "trustification.application.metrics.enabled" . ) "true" ) ( eq ( include "trustification.application.tracing.enabled" . ) "true" ) }}
{{- end }}


{{/*
Annotations for collector.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{ define "trustification.application.collector.annotations" }}
{{- end }}

{{/*
Pod annotations for collector.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{ define "trustification.application.collector.podAnnotations" }}
{{- if eq (include "trustification.application.collector.enabled" .root ) "true" }}
sidecar.opentelemetry.io/inject: "true"
{{end }}
{{- end }}
