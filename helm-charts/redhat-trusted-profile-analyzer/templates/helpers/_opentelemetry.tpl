{{/*
Shares the OTEL_EXPORTER_OTLP_ENDPOINT env var with tracing and metrics signals

Arguments (dict):
  * root - .
*/}}
{{ define "trustification.application.collector"}}
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: {{ .root.Values.collector.endpoint }}
{{- end }}

