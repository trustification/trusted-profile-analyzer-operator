{{/*
Get value from a resource. Currenlty only from secrets.

Arguments (dict):
  * .root - .
  * .valueFrom - The "valueFrom" object
*/}}
{{- define "_trustify.valueFrom" }}

{{- if .valueFrom.secretKeyRef }}
{{- $secret := lookup "v1" "Secret" .root.Release.Namespace .valueFrom.secretKeyRef.name }}
{{- required "Key not present in Secret" (index $secret.data .valueFrom.secretKeyRef.key ) | b64dec }}

{{- else if .valueFrom.configMapKeyRef }}
{{- $config := lookup "v1" "ConfigMap" .root.Release.Namespace .valueFrom.configMapKeyRef.name }}
{{- required "Key not present in ConfigMap" (index $config.data .valueFrom.configMapKeyRef.key ) }}

{{- else }}
{{- fail "valueFrom can only use .secretKeyRef or .configMapRef" }}
{{- end }}

{{- end }}
