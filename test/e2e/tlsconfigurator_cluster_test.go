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

// OpenShift-only e2e tests for the TLS configurator: the ones that cannot run
// on the minikube job.
//
// Everything else in this package either renders the chart or inspects objects
// the operator created, both of which work anywhere. These tests observe a real
// TLS handshake with trustd, which exists only on OpenShift: off OpenShift
// `trustification.openshift.useServiceCa` is false, the chart never sets
// HTTP_SERVER_TLS_ENABLED, the server listens on plain HTTP and TLS terminates
// at the ingress controller. The chart also refuses to enable the module there
// at all. So the minikube workflow cannot cover TLS 1.3 or PQC even in
// principle -- run these against CRC instead, see `make e2e-crc`.
//
// They are opt-in (E2E_OPENSHIFT=1) and assume a cluster prepared as
// devel/trusted-profile-analyzer-ocp-5-crc.yaml describes: the tlsConfigurator
// module enabled with pqc.enabled and operandTls.enabled.
package e2e

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	tlsclient "github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/client"
	tlscrypto "github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/crypto"
)

const (
	// envOpenShiftE2E opts in to this file. Without it every test here skips,
	// so `go test ./test/e2e/...` against minikube is unaffected.
	envOpenShiftE2E = "E2E_OPENSHIFT"
	// envE2ENamespace is the namespace the CR was installed into.
	envE2ENamespace = "E2E_NAMESPACE"
	// envClusterMutation additionally opts in to the one test that patches
	// cluster-wide state. See TestOpenShiftProfileChangeRollsServer.
	envClusterMutation = "E2E_CLUSTER_MUTATION"

	// defaultE2ENamespace matches devel/trusted-profile-analyzer-ocp-5-crc.yaml.
	defaultE2ENamespace = "trustify"

	// serverDeploymentName is the rendered name of the TLS-serving workload,
	// and the default entry in modules.tlsConfigurator.targetDeployments.
	serverDeploymentName = "server"
	// reconcilerDeploymentName is the TLS reconciler's rendered name.
	reconcilerDeploymentName = "tls-configurator"
	// serverPodPort is the container port trustd listens on. The Service
	// publishes 443 in front of it, but port-forward addresses the pod, and
	// going through the Route instead would measure the router's TLS settings
	// rather than trustd's.
	serverPodPort = 8080

	// serviceCAConfigMap is the ConfigMap OpenShift injects into every
	// namespace holding the service-ca bundle, used to verify the server's
	// serving certificate.
	serviceCAConfigMap = "openshift-service-ca.crt"
	serviceCAKey       = "service-ca.crt"

	// pqcGroupName is how the cluster profile and the operand env spell the
	// hybrid post-quantum key exchange.
	pqcGroupName = "X25519MLKEM768"

	// skipNotOpenShift explains the opt-out.
	skipNotOpenShift = "skipping OpenShift-only e2e test; set " + envOpenShiftE2E + "=1 to run it against CRC/OpenShift"

	// rolloutTimeout bounds a workload rollout triggered by a TLS change.
	rolloutTimeout = 10 * time.Minute
)

// clusterVersionGVR identifies the OpenShift ClusterVersion CR, used both to
// confirm this is OpenShift and to report the version the gate turns on.
var clusterVersionGVR = schema.GroupVersionResource{
	Group:    "config.openshift.io",
	Version:  "v1",
	Resource: "clusterversions",
}

// apiServerGVR identifies the cluster APIServer CR, the authoritative source of
// the cluster-wide TLS profile and the adherence policy.
var apiServerGVR = schema.GroupVersionResource{
	Group:    "config.openshift.io",
	Version:  "v1",
	Resource: "apiservers",
}

// openShiftCluster is the handle each test in this file works through.
type openShiftCluster struct {
	namespace string
	config    *rest.Config
	kube      *kubernetes.Clientset
	dynamic   dynamic.Interface
}

// requireOpenShiftCluster skips unless the OpenShift e2e run was asked for and
// the cluster really is OpenShift.
func requireOpenShiftCluster(t *testing.T) *openShiftCluster {
	t.Helper()

	if testing.Short() || os.Getenv(envOpenShiftE2E) != "1" {
		t.Skip(skipNotOpenShift)
	}

	namespace := os.Getenv(envE2ENamespace)
	if namespace == "" {
		namespace = defaultE2ENamespace
	}

	cluster := &openShiftCluster{
		namespace: namespace,
		config:    getRESTConfig(t),
		kube:      getKubernetesClient(t),
		dynamic:   getDynamicClient(t),
	}

	version := cluster.version(t)
	t.Logf("OpenShift %s, namespace %s", version, namespace)

	return cluster
}

// version returns the cluster's OpenShift version, failing if the
// ClusterVersion CR is not readable -- which is also how "this is not
// OpenShift" shows up.
func (c *openShiftCluster) version(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cv, err := c.dynamic.Resource(clusterVersionGVR).Get(ctx, "version", metav1.GetOptions{})
	require.NoError(t, err, "cluster must expose config.openshift.io/v1 ClusterVersion; this suite is OpenShift-only")

	version, found, err := unstructured.NestedString(cv.Object, "status", "desired", "version")
	require.NoError(t, err, "should be able to read .status.desired.version")
	require.True(t, found, "ClusterVersion should report a desired version")

	return version
}

// atLeast422 reports whether the cluster is on a release where the chart makes
// the module mandatory. The comparison mirrors the one in
// templates/init/tls-configure/000-validate.yaml, which is why a major above 4
// (5.x) counts unconditionally.
func atLeast422(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return major > 4 || (major == 4 && minor >= 22)
}

// TestOpenShiftVersionGateMatchesChart records which side of the 4.22 gate the
// cluster is on. On 4.22+ -- so on any 5.x -- the reconciler is not optional,
// and a run that found it missing there would be testing a cluster the chart
// should have refused to install on.
func TestOpenShiftVersionGateMatchesChart(t *testing.T) {
	cluster := requireOpenShiftCluster(t)

	version := cluster.version(t)
	if !atLeast422(version) {
		t.Logf("OpenShift %s is below 4.22: the module is optional here, but this suite still needs it enabled", version)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := cluster.kube.AppsV1().Deployments(cluster.namespace).
		Get(ctx, reconcilerDeploymentName, metav1.GetOptions{})
	require.NoError(t, err,
		"deployment/%s must exist: enable modules.tlsConfigurator in the CR "+
			"(see devel/trusted-profile-analyzer-ocp-5-crc.yaml)",
		reconcilerDeploymentName)
}

// TestOpenShiftTLSReconcilerIsRunning checks the reconciler Deployment came up,
// and that it is the singleton the design requires: two overlapping instances
// can interleave their passes and let a stale profile snapshot win, which is
// why the chart sets strategy Recreate rather than the default RollingUpdate.
func TestOpenShiftTLSReconcilerIsRunning(t *testing.T) {
	cluster := requireOpenShiftCluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	deployment, err := cluster.kube.AppsV1().Deployments(cluster.namespace).
		Get(ctx, reconcilerDeploymentName, metav1.GetOptions{})
	require.NoError(t, err, "should be able to get the reconciler Deployment")

	assert.EqualValues(t, 1, deployment.Status.AvailableReplicas,
		"the reconciler should have exactly one available replica")
	assert.Equal(t, "Recreate", string(deployment.Spec.Strategy.Type),
		"the reconciler must not overlap with itself across an upgrade")
}

// TestOpenShiftOperandProfileIsPublished checks the ConfigMap the reconciler
// writes the resolved cluster profile into. It is owned by the reconciler
// rather than templated by the chart -- the operator's periodic re-render would
// otherwise overwrite it -- so the managed-by label is the evidence that it is
// the reconciler's work and not a leftover.
func TestOpenShiftOperandProfileIsPublished(t *testing.T) {
	cluster := requireOpenShiftCluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// The ConfigMap is written on the reconciler's first pass, which on a fresh
	// install happens after the workloads are already up.
	var cm *corev1.ConfigMap
	require.Eventually(t, func() bool {
		var err error
		cm, err = cluster.kube.CoreV1().ConfigMaps(cluster.namespace).
			Get(ctx, operandConfigMapName, metav1.GetOptions{})
		return err == nil
	}, 2*time.Minute, 5*time.Second,
		"ConfigMap %q should exist: set modules.tlsConfigurator.operandTls.enabled in the CR",
		operandConfigMapName)

	assert.Equal(t, tlsclient.ManagedByValue, cm.Labels[tlsclient.ManagedByLabel],
		"the operand ConfigMap must be owned by the reconciler")

	assert.Equal(t, "custom", cm.Data[tlscrypto.EnvProfile],
		"%s selects trustd's custom mapping; the built-in names ignore the min version and ciphers",
		tlscrypto.EnvProfile)

	assert.Equal(t, "1.3", cm.Data[tlscrypto.EnvMinVersion],
		"%s should be 1.3 with pqc.enabled: hybrid key exchange is only defined for TLS 1.3",
		tlscrypto.EnvMinVersion)

	groups := cm.Data[tlscrypto.EnvGroups]
	assert.Contains(t, strings.Split(groups, ":"), pqcGroupName,
		"%s should offer the hybrid post-quantum group, got %q", tlscrypto.EnvGroups, groups)

	// An empty cipher list is not the same as an absent one: OpenSSL reads an
	// empty HTTP_SERVER_TLS_CIPHERSUITES as "disable every TLS 1.3 suite",
	// which would leave a TLS 1.3-only listener unable to handshake at all.
	for _, key := range []string{tlscrypto.EnvCiphers, tlscrypto.EnvCipherSuites, tlscrypto.EnvGroups} {
		if value, present := cm.Data[key]; present {
			assert.NotEmpty(t, value, "%s is published, so it must not be empty", key)
		}
	}
}

// TestOpenShiftServerConsumesOperandProfile checks the other half of the wiring:
// the server reads that ConfigMap, and does so through envFrom with optional
// set. envFrom rather than env because a name in env wins, so the keys the
// chart sets directly -- HTTP_SERVER_TLS_ENABLED and the certificate paths --
// stay authoritative and the reconciler cannot take the listener down. Optional
// because the ConfigMap is written at runtime and may not exist when the pods
// first start.
func TestOpenShiftServerConsumesOperandProfile(t *testing.T) {
	cluster := requireOpenShiftCluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	deployment, err := cluster.kube.AppsV1().Deployments(cluster.namespace).
		Get(ctx, serverDeploymentName, metav1.GetOptions{})
	require.NoError(t, err, "should be able to get the server Deployment")

	var envFromFound bool
	for _, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name != serverContainerName {
			continue
		}
		for _, source := range c.EnvFrom {
			if source.ConfigMapRef == nil || source.ConfigMapRef.Name != operandConfigMapName {
				continue
			}
			envFromFound = true
			require.NotNil(t, source.ConfigMapRef.Optional, "the envFrom source must be marked optional")
			assert.True(t, *source.ConfigMapRef.Optional,
				"the ConfigMap is written at runtime, so the pods must start without it")
		}
	}
	assert.True(t, envFromFound,
		"the %s container should read %q through envFrom", serverContainerName, operandConfigMapName)

	// Stamped by the reconciler rather than the chart -- deliberately, so the
	// operator's periodic re-render does not fight it -- so on a fresh install
	// it appears a moment after the Deployment does.
	require.Eventually(t, func() bool {
		hash, err := cluster.serverTLSHash(ctx)
		return err == nil && hash != ""
	}, 2*time.Minute, 5*time.Second,
		"the reconciler should stamp %s on the server pod template", tlsclient.TLSConfigHashAnnotation)
}

// TestOpenShiftServerNegotiatesTLS13WithPQC is the assertion the minikube job
// cannot make: a real handshake with trustd, over a port-forward to the pod so
// that the Route's own TLS settings are not what gets measured.
//
// Go's client offers X25519MLKEM768 by default from 1.24, so a plain dial is
// enough to show the server selected the hybrid post-quantum group rather than
// merely tolerating a client that insisted on it.
func TestOpenShiftServerNegotiatesTLS13WithPQC(t *testing.T) {
	cluster := requireOpenShiftCluster(t)

	addr := cluster.forwardServerPort(t)

	conn, err := tls.Dial("tcp", addr, cluster.clientTLSConfig(t, tls.VersionTLS12, tls.VersionTLS13))
	require.NoError(t, err, "should be able to complete a TLS handshake with trustd")
	defer func() { _ = conn.Close() }()

	state := conn.ConnectionState()
	assert.Equal(t, uint16(tls.VersionTLS13), state.Version,
		"server should negotiate TLS 1.3, got %s", tls.VersionName(state.Version))
	assert.Equal(t, tls.X25519MLKEM768, state.CurveID,
		"server should negotiate the hybrid post-quantum key exchange, got %s", state.CurveID)
}

// TestOpenShiftServerRejectsTLS12 is the negative half: with pqc.enabled the
// published floor is TLS 1.3, so a client that cannot go above 1.2 must fail.
// Without this, a server that quietly ignored the published profile and
// accepted everything would still pass the test above.
func TestOpenShiftServerRejectsTLS12(t *testing.T) {
	cluster := requireOpenShiftCluster(t)

	addr := cluster.forwardServerPort(t)

	conn, err := tls.Dial("tcp", addr, cluster.clientTLSConfig(t, tls.VersionTLS12, tls.VersionTLS12))
	if err == nil {
		_ = conn.Close()
		t.Fatalf("TLS 1.2 handshake succeeded, but the published minimum version is 1.3")
	}
}

// TestOpenShiftProfileChangeRollsServer exercises the runtime loop end to end:
// change the cluster profile, and the reconciler must republish it and roll the
// targets, because Kubernetes does not restart pods on a ConfigMap change by
// itself.
//
// It patches `apiserver/cluster`, which is cluster-wide and rolls the
// kube-apiserver as well -- minutes of churn on a single-node CRC, with the API
// briefly unavailable. Hence the second opt-in. The original profile is
// restored on cleanup.
func TestOpenShiftProfileChangeRollsServer(t *testing.T) {
	cluster := requireOpenShiftCluster(t)

	if os.Getenv(envClusterMutation) != "1" {
		t.Skipf("skipping cluster-mutating test; set %s=1 to allow patching apiserver/cluster", envClusterMutation)
	}

	ctx, cancel := context.WithTimeout(context.Background(), rolloutTimeout)
	defer cancel()

	before, err := cluster.serverTLSHash(ctx)
	require.NoError(t, err, "should be able to read the server's TLS hash")
	require.NotEmpty(t, before, "the server should carry a TLS hash before the profile changes")

	original, wanted := cluster.swapTLSProfile(t, ctx)
	t.Logf("cluster TLS profile %s -> %s", original, wanted)

	// Hashing the *resolved* profile is what keeps equivalent spellings from
	// churning the workloads, so the hash must move only because the profile
	// genuinely differs.
	require.Eventually(t, func() bool {
		hash, err := cluster.serverTLSHash(ctx)
		return err == nil && hash != before
	}, rolloutTimeout, 10*time.Second,
		"%s on the server pod template should change after the cluster profile does",
		tlsclient.TLSConfigHashAnnotation)
}

// serverTLSHash reads the rollout hash off the server pod template.
//
// It returns an error rather than failing the test because it is called from
// inside require.Eventually conditions, which testify runs on their own
// goroutine -- a t.FailNow from there does not fail the test cleanly.
func (c *openShiftCluster) serverTLSHash(ctx context.Context) (string, error) {
	deployment, err := c.kube.AppsV1().Deployments(c.namespace).
		Get(ctx, serverDeploymentName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get server Deployment: %w", err)
	}

	return deployment.Spec.Template.Annotations[tlsclient.TLSConfigHashAnnotation], nil
}

// profileSubFields are the per-type sub-objects of TLSSecurityProfile. Every
// merge patch below names all of them so that switching type also clears the
// previous type's object: a merge patch only adds and overwrites, so leaving
// them out would accumulate an "intermediate" alongside a "modern".
var profileSubFields = []string{"old", "intermediate", "modern", "custom"}

// tlsProfilePatch builds a merge patch body that makes .spec.tlsSecurityProfile
// exactly profile, clearing any field profile does not set. A nil profile
// removes the whole stanza.
func tlsProfilePatch(profile map[string]interface{}) ([]byte, error) {
	if profile == nil {
		return []byte(`{"spec":{"tlsSecurityProfile":null}}`), nil
	}

	stanza := map[string]interface{}{fieldType: nil}
	for _, field := range profileSubFields {
		stanza[field] = nil
	}
	for key, value := range profile {
		stanza[key] = value
	}

	return json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{"tlsSecurityProfile": stanza},
	})
}

// swapTLSProfile flips the cluster between two built-in profiles and registers
// a cleanup that puts the original back verbatim, returning both type names.
// Intermediate is the effective default when the field is unset, so an absent
// profile is reported as that and restored by removing the field again.
//
// The API requires the sub-object matching the type to be present -- setting
// `type: Modern` alone is rejected with "type set to Modern, but the
// corresponding field is unset" -- so the patch always carries an empty one.
func (c *openShiftCluster) swapTLSProfile(t *testing.T, ctx context.Context) (original, wanted string) {
	t.Helper()

	apiServer, err := c.dynamic.Resource(apiServerGVR).Get(ctx, "cluster", metav1.GetOptions{})
	require.NoError(t, err, "should be able to read the cluster APIServer CR")

	// Captured whole rather than rebuilt from the type name, so that a Custom
	// profile with hand-set ciphers is restored as it was.
	existing, present, err := unstructured.NestedMap(apiServer.Object, "spec", "tlsSecurityProfile")
	require.NoError(t, err, "should be able to read .spec.tlsSecurityProfile")

	original = "Intermediate"
	if present {
		if name, ok := existing[fieldType].(string); ok && name != "" {
			original = name
		}
	} else {
		existing = nil
	}

	wanted = "Modern"
	if original == "Modern" {
		wanted = "Intermediate"
	}

	restore, err := tlsProfilePatch(existing)
	require.NoError(t, err, "should be able to build the restore patch")
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_, err := c.dynamic.Resource(apiServerGVR).
			Patch(cleanupCtx, "cluster", types.MergePatchType, restore, metav1.PatchOptions{})
		if err != nil {
			t.Logf("Warning: failed to restore the cluster TLS profile: %v", err)
			return
		}
		t.Logf("restored cluster TLS profile to %s", original)
	})

	patch, err := tlsProfilePatch(map[string]interface{}{
		fieldType:               wanted,
		strings.ToLower(wanted): map[string]interface{}{},
	})
	require.NoError(t, err, "should be able to build the profile patch")

	_, err = c.dynamic.Resource(apiServerGVR).
		Patch(ctx, "cluster", types.MergePatchType, patch, metav1.PatchOptions{})
	require.NoError(t, err, "should be able to patch the cluster TLS profile")

	return original, wanted
}

// clientTLSConfig builds a client that verifies the server's service-ca
// certificate when the bundle is available, and falls back to skipping
// verification when it is not -- these tests are about the negotiated protocol
// and key exchange, and a missing bundle should not turn into a confusing
// handshake failure.
func (c *openShiftCluster) clientTLSConfig(t *testing.T, minVersion, maxVersion uint16) *tls.Config {
	t.Helper()

	cfg := &tls.Config{
		MinVersion: minVersion,
		MaxVersion: maxVersion,
		// The connection goes through a local port-forward, so the dialled
		// address is 127.0.0.1 and the name to verify has to be given here.
		ServerName: fmt.Sprintf("%s.%s.svc", serverDeploymentName, c.namespace),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cm, err := c.kube.CoreV1().ConfigMaps(c.namespace).Get(ctx, serviceCAConfigMap, metav1.GetOptions{})
	if err != nil {
		t.Logf("Warning: no %s ConfigMap (%v); not verifying the serving certificate", serviceCAConfigMap, err)
		cfg.InsecureSkipVerify = true
		return cfg
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(cm.Data[serviceCAKey])) {
		t.Logf("Warning: %s/%s held no usable certificate; not verifying the serving certificate",
			serviceCAConfigMap, serviceCAKey)
		cfg.InsecureSkipVerify = true
		return cfg
	}
	cfg.RootCAs = pool

	return cfg
}

// forwardServerPort opens a port-forward to a running server pod and returns the
// local address to dial. The forward is closed when the test ends.
func (c *openShiftCluster) forwardServerPort(t *testing.T) string {
	t.Helper()

	// Outlives the pod search below, which polls for up to testTimeout.
	ctx, cancel := context.WithTimeout(context.Background(), rolloutTimeout)
	defer cancel()

	pod := c.runningServerPod(t, ctx)

	transport, upgrader, err := spdy.RoundTripperFor(c.config)
	require.NoError(t, err, "should be able to build an SPDY round tripper")

	url := c.kube.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(c.namespace).
		Name(pod).
		SubResource("portforward").
		URL()

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})

	// Port 0 asks for any free local port, so concurrent tests do not collide.
	forwarder, err := portforward.New(dialer,
		[]string{fmt.Sprintf("0:%d", serverPodPort)}, stopCh, readyCh, io.Discard, io.Discard)
	require.NoError(t, err, "should be able to create the port forwarder")

	errCh := make(chan error, 1)
	go func() { errCh <- forwarder.ForwardPorts() }()
	t.Cleanup(func() { close(stopCh) })

	select {
	case <-readyCh:
	case err := <-errCh:
		require.NoError(t, err, "port-forward to pod %s failed", pod)
	case <-time.After(time.Minute):
		t.Fatalf("port-forward to pod %s did not become ready", pod)
	}

	ports, err := forwarder.GetPorts()
	require.NoError(t, err, "should be able to read the forwarded ports")
	require.Len(t, ports, 1, "exactly one port should be forwarded")

	return fmt.Sprintf("127.0.0.1:%d", ports[0].Local)
}

// runningServerPod returns the name of a ready server pod that is running the
// *current* pod template, selected through the Deployment's own selector rather
// than a hardcoded label set.
//
// Matching on the rollout hash matters here. The reconciler stamps it after the
// pods are already up on a fresh install, so there is a window in which a ready
// pod exists that started before the TLS profile was published — dialling that
// one would observe trustd's compiled-in defaults and fail for the wrong
// reason. Pods inherit their template's annotations, so comparing them is
// enough to tell the generations apart.
func (c *openShiftCluster) runningServerPod(t *testing.T, ctx context.Context) string {
	t.Helper()

	var name string
	require.Eventually(t, func() bool {
		var err error
		name, err = c.findReadyServerPod(ctx)
		return err == nil && name != ""
	}, testTimeout, 10*time.Second,
		"no ready server pod running the current pod template in namespace %s", c.namespace)

	return name
}

// findReadyServerPod returns a matching pod name, or "" if there is none yet.
// Like serverTLSHash it reports errors rather than failing the test, because it
// runs inside a require.Eventually condition.
func (c *openShiftCluster) findReadyServerPod(ctx context.Context) (string, error) {
	deployment, err := c.kube.AppsV1().Deployments(c.namespace).
		Get(ctx, serverDeploymentName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get server Deployment: %w", err)
	}

	wantHash := deployment.Spec.Template.Annotations[tlsclient.TLSConfigHashAnnotation]

	selector := labels.Set(deployment.Spec.Selector.MatchLabels).String()
	pods, err := c.kube.CoreV1().Pods(c.namespace).
		List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("list server pods: %w", err)
	}

	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Annotations[tlsclient.TLSConfigHashAnnotation] != wantHash {
			continue
		}
		if isPodReady(pod) {
			return pod.Name, nil
		}
	}

	return "", nil
}

// isPodReady reports whether the pod carries a true Ready condition.
func isPodReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
