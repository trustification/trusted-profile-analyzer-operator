{{/*
Evaluate the infrastructure port.

Arguments: (dict)
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.infrastructure.port"}}
{{- $infra := merge (deepCopy .module.infrastructure ) .root.Values.infrastructure }}
{{- $infra.port | default 9010 -}}
{{- end }}

{{/*
Evaluate the infrastructure initial delay seconds.

Arguments: (dict)
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.infrastructure.initialDelaySeconds"}}
{{- $infra := merge (deepCopy .module.infrastructure ) .root.Values.infrastructure }}
{{- $infra.initialDelaySeconds | default 2 -}}
{{- end }}

{{/*
Additional env-vars for configuring the infrastructure endpoint.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.infrastructure.envVars"}}
- name: INFRASTRUCTURE_ENABLED
  value: "true"
- name: INFRASTRUCTURE_BIND
  value: "[::]:{{- include "trustification.application.infrastructure.port" . }}"

{{- if eq ( include "trustification.application.metrics.enabled" .root ) "true" }}
- name: METRICS
  value: "enabled"
{{ with .root.Values.metrics.otelMetricExportInterval }}
- name: OTEL_METRIC_EXPORT_INTERVAL
  value: {{ . | quote }}
{{ end }}
{{- end }}

{{- if eq ( include "trustification.application.tracing.enabled" .root ) "true" }}
- name: TRACING
  value: "enabled"
- name: OTEL_BSP_MAX_EXPORT_BATCH_SIZE
  value: {{ .root.Values.tracing.otelBspMaxExportBatchSize | default 512 | quote }}
- name: OTEL_TRACES_SAMPLER
  value: {{ .root.Values.tracing.otelTracesSampler | default "parentbased_traceidratio" | quote }}
- name: OTEL_TRACES_SAMPLER_ARG
  value: {{ .root.Values.tracing.otelTracesSamplerArg | default "" | quote }}
{{- end }}

{{- if eq ( include "trustification.application.collector.enabled" .root ) "true" }}
{{ include "trustification.application.collector" . }}
{{- end }}

{{- end }}

{{/*
Pod port definition for the infrastructure endpoint.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.infrastructure.podPorts" }}
- containerPort: {{ include "trustification.application.infrastructure.port" . }}
  protocol: TCP
  name: infra
{{- end}}

{{/*
Standard infrastructure probe definitions.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{ define "trustification.application.infrastructure.probes" }}
livenessProbe:
  initialDelaySeconds: {{ include "trustification.application.infrastructure.initialDelaySeconds" . }}
  httpGet:
    path: /health/live
    port: {{ include "trustification.application.infrastructure.port" . }}

readinessProbe:
  initialDelaySeconds: {{ include "trustification.application.infrastructure.initialDelaySeconds" . }}
  httpGet:
    path: /health/ready
    port: {{ include "trustification.application.infrastructure.port" . }}

{{- end }}
