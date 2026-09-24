{{/*
The SQL seeding the importer table, shared by the two ways the create-importers
job can run it: as a `psql -c` argument (the plain path) or through the
IMPORTERS_SQL env-var (when the job is wrapped in a shell to pick up an RDS IAM
token).

Arguments: .
*/}}
{{- define "trustification.createImporters.sql" -}}
{{ range $key, $value := .Values.modules.createImporters.importers }}
INSERT INTO IMPORTER
  (name, revision, state, last_change, configuration)
VALUES
  ({{ $key | squote }}, gen_random_uuid(), 0, now(), '{{ $value | toJson }}'::jsonb)
ON CONFLICT (name) DO UPDATE
SET
  revision=EXCLUDED.revision,
  configuration=EXCLUDED.configuration
;
{{ end }}
{{- end -}}
