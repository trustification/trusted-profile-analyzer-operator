{{/*
Is tracing enabled?

Arguments (dict):
  * .
*/}}
{{ define "trustification.application.tracing.enabled"}}
{{- .Values.tracing.enabled }}
{{- end }}
