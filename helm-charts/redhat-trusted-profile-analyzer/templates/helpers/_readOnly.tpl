{{/*
Resolve the effective read-only flag: module-level overrides the global default.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.readOnly.envVars" }}
{{- if hasKey .module "readOnly" -}}
{{- with .module.readOnly }}
- name: TRUSTD_READ_ONLY
  value: {{ . | quote }}
{{- end }}
{{- else -}}
{{- with .root.Values.readOnly }}
- name: TRUSTD_READ_ONLY
  value: {{ . | quote }}
{{- end }}
{{- end }}
{{- end }}
