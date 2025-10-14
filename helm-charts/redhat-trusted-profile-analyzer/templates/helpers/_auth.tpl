{{/*
Volume mounts for the authenticator configuration.

Arguments (dict):
  * root - .
  * module - module object
*/}}
{{- define "trustification.authenticator.volumeMount" }}
- name: config-auth
  mountPath: /etc/config/auth.yaml

{{- if ((.module.authenticator).configMapRef).key }}
  subPath: {{ .module.authenticator.configMapRef.key }}
{{- else if ((.root.Values.authenticator).configMapRef).key }}
  subPath: {{ .root.Values.authenticator.configMapRef.key }}
{{- else }}
  subPath: auth.yaml
{{ end }}
{{- end }}

{{/*
Volume for the authenticator configuration.

Arguments (dict):
  * root - .
  * name - name of the service
  * module - module object
*/}}
{{- define "trustification.authenticator.volume" }}
- name: config-auth
  configMap:
{{- if (.module.authenticator).configMapRef }}
    name: {{ .root.Values.authenticator.configMapRef.name }}
{{- else if (.root.Values.authenticator).configMapRef }}
    name: {{ .root.Values.authenticator.configMapRef.name }}
{{- else }}
    name: {{ include "trustification.common.name" (set (deepCopy .) "name" (printf "%s-auth" .name ) ) }}
{{- end }}
{{- end }}

{{/*
Create a default config map for the authenticator.

Arguments (dict):
  * root - .
  * name - name of the service
  * module - module object
*/}}
{{- define "trustification.authenticator.defaultConfigMap" }}
{{- if and (not (.root.Values.authenticator).configMapRef) (not (.module.authenticator).configMapRef) }}

kind: ConfigMap
apiVersion: v1
metadata:
  name: {{ include "trustification.common.name" (set (deepCopy .) "name" (printf "%s-auth" .name ) ) }}
  labels:
    {{- include "trustification.common.labels" (set (deepCopy .) "name" (printf "%s-auth" .name ) ) | nindent 4 }}

data:

{{ $auth := .module.authenticator | default .root.Values.authenticator }}

{{- if kindIs "string" $auth }} {{/* Plain string */}}
  auth.yaml: {{ $auth | quote }}

{{- else if hasKey $auth "content" }} {{/* Structured content */}}
  auth.yaml: |
    {{ $auth | toYaml | nindent 4 }}

{{- else }} {{/* Empty */}}
  auth.yaml: |
    {{- include "trustification.authenticator.defaultContent" . | nindent 4 }}

{{- end }}

{{- end }}{{/* external referenced config */}}
{{- end }}

{{/*
Create the authenticator default configuration.

Arguments: (dict)
  * root - .
  * module - module object
*/}}
{{- define "trustification.authenticator.defaultContent" }}

{{- if eq (.root.Values.authenticator).type "cognito" -}}
authentication:
  clients:
    - clientId: {{ include "trustification.oidc.clientId" (dict "root" .root "clientId" "frontend" ) }}
      issuerUrl: {{ include "trustification.oidc.issuerUrlForClient" (dict "root" .root "clientId" "frontend" ) }}

      {{- with .root.Values.tls.additionalTrustAnchor }}
      tlsCaCertificates:
        - {{ . | quote }}
      {{- end }}

      additionalPermissions:
        - "ai"
        - "read.advisory"
        - "read.importer"
        - "read.metadata"
        - "read.sbom"
        - "read.weakness"

      groupSelector: "$['cognito:groups'][*]"

      groupMappings:
        manager:
          - "create.advisory"
          - "create.importer"
          - "create.metadata"
          - "create.sbom"
          - "create.weakness"

          - "upload.dataset"
          - "update.advisory"
          - "update.importer"
          - "update.metadata"
          - "update.sbom"
          - "update.weakness"

          - "delete.advisory"
          - "delete.importer"
          - "delete.metadata"
          - "delete.sbom"
          - "delete.vulnerability"
          - "delete.weakness"

    - clientId: {{ include "trustification.oidc.clientId" (dict "root" .root "clientId" "cli" ) }}
      issuerUrl: {{ include "trustification.oidc.issuerUrlForClient" (dict "root" .root "clientId" "cli" ) }}

      {{- with .root.Values.tls.additionalTrustAnchor }}
      tlsCaCertificates:
        - {{ . | quote }}
      {{- end }}

      scopeMappings:
        "trustify/advisory":
          - "create.advisory"
          - "read.advisory"
          - "update.advisory"
          - "delete.advisory"
        "trustify/importer":
          - "create.importer"
          - "read.importer"
          - "update.importer"
          - "delete.importer"
        "trustify/metadata":
          - "create.metadata"
          - "read.metadata"
          - "update.metadata"
          - "delete.metadata"
        "trustify/sbom":
          - "create.sbom"
          - "read.sbom"
          - "update.sbom"
          - "delete.sbom"
        "trustify/ai":
          - "ai"
        "trustify/admin":
          - "upload.dataset"
          - "delete.vulnerability"


{{- else -}}{{/* Keycloak is the default */}}
authentication:
  clients:

    - clientId: {{ include "trustification.oidc.clientId" (dict "root" .root "clientId" "frontend" ) }}
      issuerUrl: {{ include "trustification.oidc.issuerUrlForClient" (dict "root" .root "clientId" "frontend" ) }}
      scopeMappings: &keycloakScopeMappings
        "create:document": [ "create.advisory", "create.importer", "create.metadata", "create.sbom", "create.weakness", "upload.dataset" ]
        "read:document": [ "ai", "read.advisory", "read.importer", "read.metadata", "read.sbom", "read.weakness" ]
        "update:document": [ "update.advisory", "update.importer", "update.metadata", "update.sbom", "update.weakness" ]
        "delete:document": [ "delete.advisory", "delete.importer", "delete.metadata", "delete.sbom", "delete.vulnerability", "delete.weakness" ]
      {{- with .root.Values.tls.additionalTrustAnchor }}
      tlsCaCertificates:
        - {{ . | quote }}
      {{- end }}

    - clientId: {{ include "trustification.oidc.clientId" (dict "root" .root "clientId" "cli" ) }}
      issuerUrl: {{ include "trustification.oidc.issuerUrlForClient" (dict "root" .root "clientId" "cli" ) }}
      scopeMappings: *keycloakScopeMappings
      {{- with .root.Values.tls.additionalTrustAnchor }}
      tlsCaCertificates:
        - {{ . | quote }}
      {{- end }}

{{- end }}
{{- end }}
