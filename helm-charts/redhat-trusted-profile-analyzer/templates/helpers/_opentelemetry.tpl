{{/*
Shares the OTEL_EXPORTER_OTLP_ENDPOINT env var with tracing and metrics signals

Arguments (dict):
  * root - .
*/}}
{{ define "trustification.application.collector"}}
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: {{ .root.Values.collector.endpoint | default ( include "_trustification.collector.defaultEndpoint" .root ) | quote }}
{{- end }}

{{/*
Default OTEL collector endpoint

Arguments (dict):
  * .
*/}}
{{ define "_trustification.collector.defaultEndpoint" -}}
{{- if and .Values.collector.deployment.enabled ( eq ( include "trustification.openshift.detect" . ) "true" ) -}}
http://otel-collector-collector:4317
{{- else -}}
http://infrastructure-otelcol:4317
{{- end }}
{{- end }}
