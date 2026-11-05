{{/*
Environment variables required to configure the HTTP server side.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.httpServer.envVars" -}}
- name: HTTP_SERVER_BIND_ADDR
  value: "::"

{{ if eq ( include "trustification.openshift.useServiceCa" .root ) "true" }}
- name: HTTP_SERVER_TLS_ENABLED
  value: "true"
- name: HTTP_SERVER_TLS_KEY_FILE
  value: "/etc/tls/tls.key"
- name: HTTP_SERVER_TLS_CERTIFICATE_FILE
  value: "/etc/tls/tls.crt"
{{ end }}

{{- with .module.requestLimit }}
- name: HTTP_SERVER_REQUEST_LIMIT
  value: {{ include "trustification.common.byteSizeValue" . }}
{{- end }}

{{- with .module.jsonLimit }}
- name: HTTP_SERVER_JSON_LIMIT
  value: {{ include "trustification.common.byteSizeValue" . }}
{{- end }}

{{- with .module.workers }}
- name: HTTP_SERVER_WORKERS
  value: {{ . | quote }}
{{- end }}

{{- end }}

{{/*
envFrom sources for the HTTP server side.

The cluster-wide TLS profile, rendered by the TLS reconciler as the HTTP
server's own TLS settings (minimum version, cipher list, TLS 1.3 ciphersuites,
key-exchange groups). Without this the server honors none of them: it has no
way to read the cluster profile itself, and falls back to the "modern" profile
compiled into trustd whatever the cluster says.

Two deliberate choices:

  * optional: true -- the ConfigMap is written by the reconciler at runtime, so
    on a fresh install the pods may well start before it exists. Starting on
    trustd's own defaults and being restarted a moment later with the real
    settings is strictly better than refusing to start.
  * envFrom, not env -- a variable set in env wins over the same name from
    envFrom, so the keys the chart sets directly (HTTP_SERVER_TLS_ENABLED and
    the certificate paths) stay authoritative and the reconciler cannot
    accidentally take the listener down by writing them.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.application.httpServer.envFrom" -}}

{{- if eq ( include "trustification.tlsConfigurator.operandTls" .root ) "true" }}
- configMapRef:
    name: {{ include "trustification.tlsConfigurator.operandConfigMapName" .root | quote }}
    optional: true
{{- end }}

{{- end }}

{{/*
Volume mounts matching the HTTP server side configuration.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{ define "trustification.application.httpServerVolumesMounts" -}}

{{ if eq ( include "trustification.openshift.useServiceCa" .root ) "true" }}
- mountPath: /etc/tls
  name: tls
  readOnly: true
{{ end }}

{{- end }}

{{/*
Volumes matching the HTTP server side configuration.

Arguments (dict):
  * root - .
  * name - name of the deployment
  * module - module object
*/}}
{{ define "trustification.application.httpServerVolumes" -}}

{{ if eq ( include "trustification.openshift.useServiceCa" .root ) "true" }}
- name: tls
  secret:
    secretName: {{ .name }}-tls
{{- end }}

{{- end }}
