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

{{/*
Whether the cluster TLS profile is pushed into the workloads' own TLS listener.

Three things all have to be true, and each is a separate "no" rather than a
degraded "yes":

  * the module is enabled -- the reconciler is what writes the ConfigMap, so
    without it the ConfigMap is never created and the envFrom would reference
    something that will never exist;
  * operandTls.enabled -- an explicit opt-in, because this narrows what the
    server accepts from clients. Everything about this module is off by
    default;
  * the server is actually serving TLS (useServiceCa). Off OpenShift the
    server listens on plain HTTP and TLS terminates at the ingress controller,
    so there is no listener for a profile to apply to. Setting the variables
    anyway would be inert at best and misleading at worst.

Arguments: the root context (.)
*/}}
{{- define "trustification.tlsConfigurator.operandTls" -}}
{{- $module := .Values.modules.tlsConfigurator -}}
{{- if and $module.enabled $module.operandTls.enabled (eq (include "trustification.openshift.useServiceCa" .) "true") -}}
true
{{- else -}}
false
{{- end -}}
{{- end }}

{{/*
Name of the ConfigMap carrying the operand's TLS settings.

The reconciler owns this object outright -- it is created and rewritten at
runtime, and is deliberately not a chart template, because the operator
re-renders the chart periodically and would overwrite whatever the reconciler
had written. The chart only references it, as an optional envFrom source. This
is the same division as the pod-template hash annotation.

It is namespaced, so unlike the ClusterRole it keeps a plain name.

The name is validated here rather than at each use. It comes from user-supplied
values -- through the operator, from the CR .spec, which anyone who can create
a TrustedProfileAnalyzer controls -- and it is interpolated into a Deployment
argument, an envFrom reference and a Role resourceName. A value carrying a
quote or a newline would otherwise break out of those scalars and append
arbitrary YAML to the rendered pod spec or argument list. Rejecting anything
that is not a valid ConfigMap name closes that off at the source; every call
site quotes the result as well, so neither layer is load-bearing alone.

Quoting matters even for a legal name: go-yaml resolves bare "no", "y", "on"
and friends as booleans, so an unquoted name: no would render as a boolean and
be rejected by the API server.

The rule is Kubernetes' own for ConfigMap names, an RFC 1123 DNS subdomain of
at most 253 characters.

Arguments: the root context (.)
*/}}
{{- define "trustification.tlsConfigurator.operandConfigMapName" -}}
{{- $name := .Values.modules.tlsConfigurator.operandTls.configMapName | default "tls-configurator-profile" -}}
{{- if or (gt (len $name) 253) (not (regexMatch `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` $name)) -}}
{{- fail (printf `modules.tlsConfigurator.operandTls.configMapName is not a valid ConfigMap name: %q

It must be an RFC 1123 DNS subdomain of at most 253 characters: lowercase
alphanumerics, '-' and '.', starting and ending with an alphanumeric.

The name is rendered into the reconciler's arguments, the server's envFrom and
the reconciler's Role, so a value outside that alphabet is rejected here rather
than being interpolated into the manifests.` $name) -}}
{{- end -}}
{{- $name -}}
{{- end }}
