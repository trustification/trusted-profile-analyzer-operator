{{/*
Default host part of the Server endpoint.

Arguments (dict):
  * root - .
*/}}
{{- define "trustification.host.server" }}
{{- include "trustification.ingress.host" (dict "root" .root "ingress" .root.Values.modules.server.ingress "defaultHost" "server") }}
{{- end }}
