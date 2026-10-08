{{/*
PSQL configuration env-vars.

Arguments (dict):

  * root - .
  * database - database object
  * prefix - (optional) prefix to the env-var names
*/}}
{{- define "trustification.psql.envVars" }}
- name: {{ .prefix | default "" }}PGHOST
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.host "msg" "Missing value for database host" ) | nindent 2 }}
- name: {{ .prefix | default "" }}PGPORT
  {{- include "trustification.common.envVarValue" (.database.port | default "5432") | nindent 2 }}
- name: {{ .prefix | default "" }}PGDATABASE
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.name "msg" "Missing value for database name" ) | nindent 2 }}
- name: {{ .prefix | default "" }}PGUSER
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.username "msg" "Missing value for database username" ) | nindent 2 }}
{{- if not (eq (include "trustification.cco.rds.isConnectionIamAuth" (dict "root" .root "database" .database)) "true") }}
- name: {{ .prefix | default "" }}PGPASSWORD
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.password "msg" "Missing value for database password" ) | nindent 2 }}
{{- end }}
- name: {{ .prefix | default "" }}PGSSLMODE
{{- if eq (include "trustification.cco.rds.isConnectionIamAuth" (dict "root" .root "database" .database)) "true" }}
  value: require
{{- else }}
  value: {{ .database.sslMode | default "allow" }}
{{- end }}
{{- end }}

{{/*
Postgres configuration env-vars.

Arguments (dict):

  * root - .
  * database - database object
  * prefix - (optional) prefix to the env-var names
*/}}
{{- define "trustification.postgres.envVars" }}
{{- $iamAuth := eq (include "trustification.cco.rds.isConnectionIamAuth" (dict "root" .root "database" .database)) "true" }}
- name: {{ .prefix | default "TRUSTD_DB_" }}HOST
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.host "msg" "Missing value for database host" ) | nindent 2 }}
- name: {{ .prefix | default "TRUSTD_DB_" }}PORT
  {{- include "trustification.common.envVarValue" (.database.port | default "5432") | nindent 2 }}
- name: {{ .prefix | default "TRUSTD_DB_" }}NAME
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.name "msg" "Missing value for database name" ) | nindent 2 }}
- name: {{ .prefix | default "TRUSTD_DB_" }}USER
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.username "msg" "Missing value for database username" ) | nindent 2 }}
{{- if not $iamAuth }}
- name: {{ .prefix | default "TRUSTD_DB_" }}PASSWORD
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.password "msg" "Missing value for database password" ) | nindent 2 }}
{{- end }}
- name: {{ .prefix | default "TRUSTD_DB_" }}SSLMODE
{{- if $iamAuth }}
  value: require
{{- else }}
  value: {{ .database.sslMode | default "allow" }}
{{- end }}

{{- $prefix := (.prefix | default "TRUSTD_DB_") }}
{{- if .database.minimumConnections }}
- name: {{ $prefix }}MIN_CONN
  value: {{ .database.minimumConnections | quote }}
{{- else if $iamAuth }}
{{- /*
  trustd defaults to a minimum of 25 pooled connections. Over RDS IAM
  authentication every connection costs a PAM round-trip, and opening 25 of
  them at once starves them all inside trustd's connection acquire timeout — the
  pool then never yields a single connection and the process fails to start.
  The timeout is not configurable from this chart, so cap the eager fill
  instead and let the pool grow on demand. An explicit minimumConnections
  always wins.
*/}}
- name: {{ $prefix }}MIN_CONN
  value: "1"
{{- end }}
{{- with .database.maximumConnections }}
- name: {{ $prefix }}MAX_CONN
  value: {{ . | quote }}
{{- end }}

{{- end }}
