/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	// fieldOpenshift is the values key holding the platform overrides.
	fieldOpenshift = "openshift"
	// operandConfigMapName is the default name of the ConfigMap through which
	// the reconciler hands the cluster TLS profile to the workloads.
	operandConfigMapName = "tls-configurator-profile"
	// operandConfigMapFlag is how that name reaches the reconciler.
	operandConfigMapFlag = "--operand-config-map=" + operandConfigMapName
	// serverContainerName is the container in the server Deployment that runs
	// trustd -- the one that has to receive the operand TLS settings.
	serverContainerName = "service"
	// testOperatorImage stands in for the operator image the operator itself
	// injects via RELATED_IMAGE_TLS_CONFIGURATOR; a standalone render has to
	// supply one.
	testOperatorImage = "quay.io/example/rhtpa-operator:test"
)

// tlsConfiguratorValues renders the chart on a cluster that looks like
// OpenShift, with the TLS configurator on and operandTls set as asked.
//
// openshift.enabled forces platform detection: `helm template` cannot call
// lookup, so the chart would otherwise take this for plain Kubernetes and
// refuse to install with the module enabled.
func tlsConfiguratorValues(operandTLS bool) map[string]interface{} {
	values := minimalValues()
	values[fieldOpenshift] = openshiftValues(true)
	values[fieldModules] = map[string]interface{}{
		"server": map[string]interface{}{fieldEnabled: true},
		"tlsConfigurator": map[string]interface{}{
			fieldEnabled: true,
			"image":      map[string]interface{}{"fullName": testOperatorImage},
			"operandTls": map[string]interface{}{
				fieldEnabled: operandTLS,
			},
		},
	}
	return values
}

// openshiftValues forces OpenShift detection, and says whether the server
// terminates TLS itself. `helm template` cannot call lookup, so without
// openshift.enabled the chart takes any render for plain Kubernetes.
func openshiftValues(useServiceCA bool) map[string]interface{} {
	return map[string]interface{}{
		fieldEnabled:   true,
		"useServiceCa": useServiceCA,
	}
}

// TestTLSConfiguratorOperandTLSIsOffByDefault is the guarantee that matters
// most here: enabling the module must not, on its own, change how the RHTPA
// workloads serve TLS. Honoring the cluster profile in the operand's own
// listener can relax it -- a cluster on the Old profile takes the server down
// to TLS 1.0 -- so it has to be a separate, explicit opt-in.
func TestTLSConfiguratorOperandTLSIsOffByDefault(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	rendered := renderHelmChart(t, getChartPath(t), tlsConfiguratorValues(false))

	assert.NotContains(t, rendered, operandConfigMapName,
		"nothing should reference the operand ConfigMap when operandTls is off")
	assert.NotContains(t, rendered, "envFrom",
		"the server should not read an operand ConfigMap when operandTls is off")
	assert.NotContains(t, rendered, "--operand-config-map",
		"the reconciler should not be told to publish when operandTls is off")
	// The module itself is still on, so its Deployment must be there: this
	// test is about the opt-in, not about the module.
	assert.Contains(t, rendered, "--action=reconcile",
		"the reconciler should still be deployed")
}

// TestTLSConfiguratorOperandTLSWiring checks the three pieces that have to
// agree on the ConfigMap name: the reconciler writes it, the server reads it,
// and the Role allows the write.
//
// The Deployments are parsed rather than grepped. A substring search over the
// whole document cannot say *where* it matched: an envFrom attached to the
// wrong container, or an `optional: true` belonging to some unrelated field,
// would satisfy it while trustd never sees the settings.
func TestTLSConfiguratorOperandTLSWiring(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	rendered := renderHelmChart(t, getChartPath(t), tlsConfiguratorValues(true))

	configurator := parseDeployment(t, findDocument(t, rendered, "Deployment", "tls-configurator"))
	assert.Contains(t, findContainer(t, configurator, "tls-configurator").Args, operandConfigMapFlag,
		"the reconciler should be told which ConfigMap to publish to")

	server := parseDeployment(t, findDocument(t, rendered, "Deployment", "server"))
	source := operandEnvFromSource(t, findContainer(t, server, serverContainerName))
	require.NotNil(t, source.ConfigMapRef,
		"the server's operand envFrom entry should be a configMapRef")
	assert.Equal(t, operandConfigMapName, source.ConfigMapRef.Name)
	require.NotNil(t, source.ConfigMapRef.Optional,
		"the reference must set optional explicitly")
	assert.True(t, *source.ConfigMapRef.Optional,
		"the reference must be optional: the ConfigMap is written at runtime, "+
			"so pods can legitimately start before it exists")

	// The settings only reach trustd through the serving container. An
	// envFrom on an init container would be invisible to it.
	for _, c := range server.Spec.Template.Spec.InitContainers {
		assert.Empty(t, c.EnvFrom,
			"init container %q carries envFrom; the operand settings belong on the "+
				"serving container, where trustd can read them", c.Name)
	}

	role := findDocument(t, rendered, "Role", "tls-configurator")
	assert.Contains(t, role, "configmaps",
		"the reconciler needs permission to write the ConfigMap it publishes")
	assert.Contains(t, role, "resourceNames",
		"ConfigMap access should be pinned to the one name, not the whole namespace")
}

// TestTLSConfiguratorRejectsInvalidConfigMapName covers the injection guard.
// operandTls.configMapName is user-supplied -- through the operator it comes
// straight from the CR .spec -- and is interpolated into the reconciler's
// arguments, the server's envFrom and the reconciler's Role. A value carrying
// a quote and a newline would otherwise close the scalar it sits in and append
// arbitrary YAML to the rendered manifests.
func TestTLSConfiguratorRejectsInvalidConfigMapName(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	tests := []struct {
		name  string
		given string
	}{
		{
			name:  "argument injection",
			given: "pwned\"\n            - \"--target-namespace=kube-system",
		},
		{
			name:  "pod spec injection",
			given: "pwned\n  - secretRef:\n      name: auth-secret",
		},
		{name: "uppercase", given: "NotADNSName"},
		{name: "leading dash", given: "-leading-dash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := tlsConfiguratorValues(true)
			modules, ok := values[fieldModules].(map[string]interface{})
			require.True(t, ok, "modules should be a map")
			configurator, ok := modules["tlsConfigurator"].(map[string]interface{})
			require.True(t, ok, "tlsConfigurator should be a map")
			configurator["operandTls"] = map[string]interface{}{
				fieldEnabled:    true,
				"configMapName": tt.given,
			}

			_, stderr, err := tryRenderHelmChart(t, getChartPath(t), values)
			require.Error(t, err,
				"the chart should refuse to render %q rather than interpolate it", tt.given)
			assert.Contains(t, stderr, "configMapName",
				"the rejection should name the offending value")
		})
	}
}

// parseDeployment unmarshals a rendered document into a typed Deployment, so
// assertions can name the container and field they mean.
func parseDeployment(t *testing.T, doc string) *appsv1.Deployment {
	t.Helper()

	var dep appsv1.Deployment
	require.NoError(t, k8syaml.Unmarshal([]byte(doc), &dep), "should parse the Deployment")
	return &dep
}

// findContainer returns the named container of a Deployment's pod template.
func findContainer(t *testing.T, dep *appsv1.Deployment, name string) corev1.Container {
	t.Helper()

	for _, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("deployment %q has no container named %q", dep.Name, name)
	return corev1.Container{}
}

// operandEnvFromSource returns the container's envFrom entry referencing the
// operand ConfigMap, failing when it is absent or duplicated.
func operandEnvFromSource(t *testing.T, c corev1.Container) corev1.EnvFromSource {
	t.Helper()

	var matches []corev1.EnvFromSource
	for _, source := range c.EnvFrom {
		if source.ConfigMapRef != nil && source.ConfigMapRef.Name == operandConfigMapName {
			matches = append(matches, source)
		}
	}

	require.Lenf(t, matches, 1,
		"want exactly one envFrom entry on container %q referencing configmap %q, got %d",
		c.Name, operandConfigMapName, len(matches))
	return matches[0]
}

// TestTLSConfiguratorConfigMapIsNotTemplated pins the ownership split. The
// ConfigMap is written by the reconciler at runtime; templating it would mean
// the operator's periodic re-render overwrites whatever the reconciler last
// published, which is exactly the fight the hash annotation already avoids.
func TestTLSConfiguratorConfigMapIsNotTemplated(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	rendered := renderHelmChart(t, getChartPath(t), tlsConfiguratorValues(true))

	for _, doc := range splitYAMLDocs(rendered) {
		if !strings.Contains(doc, "kind: ConfigMap") {
			continue
		}
		assert.NotContains(t, doc, "name: "+operandConfigMapName,
			"the operand ConfigMap must not be a chart template: the operator would "+
				"overwrite what the reconciler publishes into it")
	}
}

// TestTLSConfiguratorNoOperandTLSOffOpenShift checks the platform guard. Off
// OpenShift the server listens on plain HTTP and TLS terminates at the ingress
// controller, so there is no listener for a profile to apply to and the
// variables would be inert.
func TestTLSConfiguratorNoOperandTLSWithoutServiceCA(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	values := tlsConfiguratorValues(true)
	values[fieldOpenshift] = openshiftValues(false)

	rendered := renderHelmChart(t, getChartPath(t), values)

	assert.NotContains(t, rendered, operandConfigMapName,
		"operandTls should be inert when the server is not serving TLS itself")
}

// TestTLSConfiguratorIsASingleton pins the Deployment strategy. The reconciler
// does no leader election: each pass reads the cluster profile and then writes
// the operand ConfigMap and the workloads' hash from that snapshot, so a second
// instance running concurrently can land a stale snapshot over a fresh one. The
// default RollingUpdate strategy would produce exactly that overlap during an
// upgrade, because maxSurge rounds up to 1 even at replicas: 1.
func TestTLSConfiguratorIsASingleton(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	rendered := renderHelmChart(t, getChartPath(t), tlsConfiguratorValues(true))

	deployment := findDocument(t, rendered, "Deployment", "tls-configurator")
	assert.Contains(t, deployment, "replicas: 1",
		"a second replica would be a second concurrent writer")
	assert.Contains(t, deployment, "type: Recreate",
		"RollingUpdate surges a second pod before the old one goes away, so an old "+
			"reconciler could publish its stale profile after the new one published the "+
			"current one")
}

// TestTLSConfiguratorAdherenceGuardIsSkippedOnDryRun pins the one half of the
// adherence guard that is reachable without a cluster.
//
// 000-validate.yaml refuses to install when the cluster sets tlsAdherence
// StrictAllComponents and operandTls is off. It reads the policy with lookup,
// which returns nothing under `helm template` and --dry-run, so the check is
// skipped there. That skip is deliberate and must stay: failing closed on an
// unreadable policy would break every dry run and the operator's own rendering
// paths. The fail path itself needs a live cluster, like the 4.22 version
// guard above it.
func TestTLSConfiguratorAdherenceGuardIsSkippedOnDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	// The exact shape the guard targets: TLS-serving server, module on,
	// publishing off. It must still render.
	rendered := renderHelmChart(t, getChartPath(t), tlsConfiguratorValues(false))
	assert.Contains(t, rendered, "--action=reconcile",
		"the adherence guard must not fire when the policy cannot be read")

	// And the escape hatch has to be a known value, or setting it would be
	// rejected by values.schema.json before the guard ever consults it.
	values := tlsConfiguratorValues(false)
	modules, ok := values[fieldModules].(map[string]interface{})
	require.True(t, ok, "modules should be a map")
	configurator, ok := modules["tlsConfigurator"].(map[string]interface{})
	require.True(t, ok, "tlsConfigurator should be a map")
	configurator["operandTls"] = map[string]interface{}{
		fieldEnabled:    false,
		"allowDisabled": true,
	}

	rendered = renderHelmChart(t, getChartPath(t), values)
	assert.Contains(t, rendered, "--action=reconcile",
		"operandTls.allowDisabled should be accepted by the values schema")
	assert.NotContains(t, rendered, operandConfigMapFlag,
		"acknowledging non-adherence must not switch publishing on")
}

// findDocument returns the single rendered document of the given kind and
// metadata name, failing the test when it is absent or ambiguous.
//
// Both kind and name are matched against the document's own top-level fields
// rather than anywhere in its text. Nested occurrences are common enough to
// make a loose match useless: a RoleBinding names its Role under roleRef, and
// the importer Deployment names the server Deployment in its pod affinity
// selector.
func findDocument(t *testing.T, rendered, kind, name string) string {
	t.Helper()

	var matches []string
	for _, doc := range splitYAMLDocs(rendered) {
		if hasTopLevelField(doc, "kind: "+kind) &&
			strings.Contains(doc, "metadata:\n  name: "+name+"\n") {
			matches = append(matches, doc)
		}
	}

	require.Lenf(t, matches, 1, "want exactly one %s named %q in the rendered chart", kind, name)
	return matches[0]
}

// hasTopLevelField reports whether a rendered document carries the given
// unindented line. Documents arrive trimmed, so the field may be the first
// line or follow a newline.
func hasTopLevelField(doc, field string) bool {
	return strings.HasPrefix(doc, field+"\n") || strings.Contains(doc, "\n"+field+"\n")
}
