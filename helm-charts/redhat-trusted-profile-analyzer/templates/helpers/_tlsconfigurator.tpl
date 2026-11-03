{{/*
Name for the TLS configurator's *cluster-scoped* resources (ClusterRole,
ClusterRoleBinding).

These objects live outside any namespace, so a fixed name collides as soon as a
second release of this chart is installed anywhere on the same cluster: Helm
refuses to adopt an object whose meta.helm.sh/release-name and
-release-namespace annotations point at a different release, and the install
fails with "exists and cannot be imported into the current release".

Release namespace + release name is unique cluster-wide (Helm release names are
unique within a namespace), so it is the natural prefix. The namespaced
resources (ServiceAccount, Role, RoleBinding, Deployment) keep the plain
"tls-configurator" name, like the rest of the chart.

Arguments: the root context (.)
*/}}
{{- define "trustification.tlsConfigurator.clusterScopedName" -}}
{{ printf "%s-%s-tls-configurator" .Release.Namespace .Release.Name | trunc 253 | trimSuffix "-" }}
{{- end }}
