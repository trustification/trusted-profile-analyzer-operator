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
{{- if not (eq (include "trustification.cco.rds.isEnabled" (dict "root" .root)) "true") }}
- name: {{ .prefix | default "" }}PGPASSWORD
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.password "msg" "Missing value for database password" ) | nindent 2 }}
{{- end }}
- name: {{ .prefix | default "" }}PGSSLMODE
{{- if eq (include "trustification.cco.rds.isEnabled" (dict "root" .root)) "true" }}
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
- name: {{ .prefix | default "TRUSTD_DB_" }}HOST
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.host "msg" "Missing value for database host" ) | nindent 2 }}
- name: {{ .prefix | default "TRUSTD_DB_" }}PORT
  {{- include "trustification.common.envVarValue" (.database.port | default "5432") | nindent 2 }}
- name: {{ .prefix | default "TRUSTD_DB_" }}NAME
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.name "msg" "Missing value for database name" ) | nindent 2 }}
- name: {{ .prefix | default "TRUSTD_DB_" }}USER
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.username "msg" "Missing value for database username" ) | nindent 2 }}
{{- if not (eq (include "trustification.cco.rds.isEnabled" (dict "root" .root)) "true") }}
- name: {{ .prefix | default "TRUSTD_DB_" }}PASSWORD
  {{- include "trustification.common.requiredEnvVarValue" (dict "value" .database.password "msg" "Missing value for database password" ) | nindent 2 }}
{{- end }}
- name: {{ .prefix | default "TRUSTD_DB_" }}SSLMODE
{{- if eq (include "trustification.cco.rds.isEnabled" (dict "root" .root)) "true" }}
  value: require
{{- else }}
  value: {{ .database.sslMode | default "allow" }}
{{- end }}

{{- $prefix := (.prefix | default "TRUSTD_DB_") }}
{{- with .database.minimumConnections }}
- name: {{ $prefix }}MIN_CONN
  value: {{ . | quote }}
{{- end }}
{{- with .database.maximumConnections }}
- name: {{ $prefix }}MAX_CONN
  value: {{ . | quote }}
{{- end }}

{{- end }}
