{{/*
Environment variables for the TLS security profile.

Precedence:
  1. User-specified via .Values.tls.securityProfile (string or object)
  2. Auto-detected from OpenShift APIServer CR via lookup
  3. Nothing (trustify server uses its compiled default: modern)

Arguments (dict):
  * root - .
*/}}
{{- define "trustification.tls.securityProfile.envVars" -}}

{{- $profile := (.root.Values.tls).securityProfile -}}

{{- if $profile -}}
  {{/* User explicitly set tls.securityProfile */}}
  {{- if kindIs "string" $profile }}
- name: HTTP_SERVER_TLS_SECURITY_PROFILE
  value: {{ $profile | quote }}
  {{- else if kindIs "map" $profile }}
- name: HTTP_SERVER_TLS_SECURITY_PROFILE
  value: {{ $profile.type | quote }}
    {{- if eq $profile.type "custom" }}
      {{- with $profile.minTLSVersion }}
- name: HTTP_SERVER_TLS_MIN_VERSION
  value: {{ . | quote }}
      {{- end }}
      {{- with $profile.ciphers }}
- name: HTTP_SERVER_TLS_CIPHERS
  value: {{ join ":" . | quote }}
      {{- end }}
      {{- with $profile.ciphersuites }}
- name: HTTP_SERVER_TLS_CIPHERSUITES
  value: {{ join ":" . | quote }}
      {{- end }}
    {{- end }}
  {{- end }}

{{- else if eq (include "trustification.openshift.detect" .root) "true" -}}
  {{/* Auto-detect from OpenShift APIServer CR */}}
  {{- $apiserver := (lookup "config.openshift.io/v1" "APIServer" "" "cluster") -}}
  {{- $tlsProfile := dig "spec" "tlsSecurityProfile" dict $apiserver -}}
  {{- with $tlsProfile -}}
    {{- $type := .type | lower }}
- name: HTTP_SERVER_TLS_SECURITY_PROFILE
  value: {{ $type | quote }}
    {{- if eq $type "custom" }}
      {{- with .custom -}}
        {{- with .minTLSVersion }}
- name: HTTP_SERVER_TLS_MIN_VERSION
  value: {{ include "trustification.tls.securityProfile.mapVersion" . | quote }}
        {{- end }}
        {{- with .ciphers }}
- name: HTTP_SERVER_TLS_CIPHERS
  value: {{ join ":" . | quote }}
        {{- end }}
      {{- end }}
    {{- end }}
  {{- end }}

{{- end }}

{{- end }}

{{/*
Map OpenShift TLS version names to trustify version strings.

VersionTLS10 -> 1.0, VersionTLS11 -> 1.1, VersionTLS12 -> 1.2, VersionTLS13 -> 1.3

Arguments: version string (e.g. "VersionTLS12")
*/}}
{{- define "trustification.tls.securityProfile.mapVersion" -}}
{{- $map := dict "VersionTLS10" "1.0" "VersionTLS11" "1.1" "VersionTLS12" "1.2" "VersionTLS13" "1.3" -}}
{{- get $map . | default . -}}
{{- end }}
