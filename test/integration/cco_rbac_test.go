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

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/yaml.v3"
)

type ClusterRoleBinding struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name   string            `yaml:"name"`
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	RoleRef struct {
		APIGroup string `yaml:"apiGroup"`
		Kind     string `yaml:"kind"`
		Name     string `yaml:"name"`
	} `yaml:"roleRef"`
	Subjects []struct {
		Kind      string `yaml:"kind"`
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"subjects"`
}

func TestCCOClusterRoleExists(t *testing.T) {
	rolePath := filepath.Join("config", "rbac", "clusterrole.yaml")

	data, err := os.ReadFile(rolePath)
	require.NoError(t, err, "CCO ClusterRole file should exist and be readable")

	var role Role
	err = yaml.Unmarshal(data, &role)
	require.NoError(t, err, "CCO ClusterRole should be valid YAML")

	assert.Equal(t, "rbac.authorization.k8s.io/v1", role.APIVersion, "should use correct API version")
	assert.Equal(t, "ClusterRole", role.Kind, "should be a ClusterRole")
	assert.Equal(t, "cco-credentialsrequest-access", role.Metadata.Name, "name should match")
}

func TestCCOClusterRolePermissions(t *testing.T) {
	rolePath := filepath.Join("config", "rbac", "clusterrole.yaml")

	data, err := os.ReadFile(rolePath)
	require.NoError(t, err, "CCO ClusterRole should be readable")

	var role Role
	err = yaml.Unmarshal(data, &role)
	require.NoError(t, err, "CCO ClusterRole should be valid YAML")

	foundCCORule := false
	for _, rule := range role.Rules {
		if contains(rule.APIGroups, "cloudcredential.openshift.io") &&
			contains(rule.Resources, "credentialsrequests") {
			foundCCORule = true
			requiredVerbs := []string{"create", "get", "list", "watch", "update", "delete"}
			for _, verb := range requiredVerbs {
				assert.Contains(t, rule.Verbs, verb,
					"CCO ClusterRole should have '%s' verb on credentialsrequests", verb)
			}
		}
	}

	assert.True(t, foundCCORule,
		"CCO ClusterRole should have a rule for cloudcredential.openshift.io/credentialsrequests")
}

func TestCCOClusterRoleBindingExists(t *testing.T) {
	bindingPath := filepath.Join("config", "rbac", "clusterrolebinding_cco.yaml")

	data, err := os.ReadFile(bindingPath)
	require.NoError(t, err, "CCO ClusterRoleBinding file should exist and be readable")

	var binding ClusterRoleBinding
	err = yaml.Unmarshal(data, &binding)
	require.NoError(t, err, "CCO ClusterRoleBinding should be valid YAML")

	assert.Equal(t, "rbac.authorization.k8s.io/v1", binding.APIVersion, "should use correct API version")
	assert.Equal(t, "ClusterRoleBinding", binding.Kind, "should be a ClusterRoleBinding")
	assert.Equal(t, "cco-credentialsrequest-rolebinding", binding.Metadata.Name, "name should match")

	assert.Equal(t, "rbac.authorization.k8s.io", binding.RoleRef.APIGroup, "roleRef apiGroup should match")
	assert.Equal(t, "ClusterRole", binding.RoleRef.Kind, "roleRef kind should be ClusterRole")
	assert.Equal(t, "cco-credentialsrequest-access", binding.RoleRef.Name, "roleRef name should match the CCO ClusterRole")
}

func TestCCOClusterRoleBindingSubjects(t *testing.T) {
	bindingPath := filepath.Join("config", "rbac", "clusterrolebinding_cco.yaml")

	data, err := os.ReadFile(bindingPath)
	require.NoError(t, err, "CCO ClusterRoleBinding should be readable")

	var binding ClusterRoleBinding
	err = yaml.Unmarshal(data, &binding)
	require.NoError(t, err, "CCO ClusterRoleBinding should be valid YAML")

	require.NotEmpty(t, binding.Subjects, "ClusterRoleBinding should have at least one subject")

	assert.Equal(t, "ServiceAccount", binding.Subjects[0].Kind,
		"subject should be a ServiceAccount")

	// The subject must be written the way kustomize can rewrite it, not the
	// way it ends up on the cluster. config/rbac/service_account.yaml declares
	// the account as "controller-manager" in namespace "system"; the
	// nameReference transformer then applies the namePrefix and the namespace
	// transformer substitutes the real namespace, giving
	// rhtpa-operator-controller-manager in rhtpa-operator-system.
	//
	// Spelling the final name here instead is what this test used to assert,
	// and it is a silent failure: kustomize leaves an unrecognised literal
	// alone, so the binding is created pointing at a ServiceAccount that does
	// not exist, and RBAC simply never matches it. Verified on a live cluster
	// -- the operator was denied every permission this ClusterRole grants.
	assert.Equal(t, "controller-manager", binding.Subjects[0].Name,
		"subject must use the unprefixed name so kustomize's nameReference transformer rewrites it")
	assert.Equal(t, "system", binding.Subjects[0].Namespace,
		"subject must use the \"system\" namespace so kustomize's namespace transformer rewrites it")
}

func TestCCOResourcesInKustomization(t *testing.T) {
	kustomizationPath := filepath.Join("config", "rbac", "kustomization.yaml")

	data, err := os.ReadFile(kustomizationPath)
	require.NoError(t, err, "kustomization.yaml should be readable")

	content := string(data)
	assert.True(t, strings.Contains(content, "clusterrole.yaml"),
		"kustomization.yaml should include clusterrole.yaml")
	assert.True(t, strings.Contains(content, "clusterrolebinding_cco.yaml"),
		"kustomization.yaml should include clusterrolebinding_cco.yaml")
}

// operatorServiceAccountNames are the two spellings of the operator's own
// ServiceAccount: the one declared in config/rbac/service_account.yaml and the
// one it ends up with after the namePrefix is applied. A binding subject naming
// either is meant to point at the operator.
var operatorServiceAccountNames = map[string]bool{
	"controller-manager":                true,
	"rhtpa-operator-controller-manager": true,
}

// TestBindingSubjectsUseKustomizeConvention guards every (Cluster)RoleBinding
// under config/rbac, not just the CCO one.
//
// Kustomize rewrites a subject only when it is written the way the generated
// ServiceAccount is: the unprefixed name (so nameReference matches it) in
// namespace "system" (so the namespace transformer substitutes it). A literal
// post-prefix name with a "placeholder" namespace is passed through untouched,
// producing a binding whose subject refers to a ServiceAccount that never
// exists. Nothing reports that -- the API accepts the binding and the
// authorizer simply never matches it, so the operator just gets "forbidden" on
// every permission the role was supposed to grant.
//
// Found on a live OpenShift cluster: three ClusterRoleBindings were inert this
// way, and the operator could not create the TLS configurator's ServiceAccount,
// Role or RoleBinding, so the whole module failed to install.
func TestBindingSubjectsUseKustomizeConvention(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("config", "rbac", "*.yaml"))
	require.NoError(t, err, "should be able to list config/rbac")
	require.NotEmpty(t, paths, "config/rbac should contain manifests")

	checked := 0
	for _, path := range paths {
		data, err := os.ReadFile(path) //nolint:gosec // fixed, repo-relative path
		require.NoError(t, err, "%s should be readable", path)

		for _, doc := range strings.Split(string(data), "\n---") {
			var binding ClusterRoleBinding
			if err := yaml.Unmarshal([]byte(doc), &binding); err != nil {
				continue
			}
			if !strings.HasSuffix(binding.Kind, "RoleBinding") {
				continue
			}

			for _, subject := range binding.Subjects {
				if subject.Kind != "ServiceAccount" || !operatorServiceAccountNames[subject.Name] {
					continue
				}
				checked++
				name := filepath.Base(path)
				assert.Equal(t, "controller-manager", subject.Name,
					"%s: subject must use the unprefixed name so kustomize rewrites it", name)
				assert.Equal(t, "system", subject.Namespace,
					"%s: subject must use the \"system\" namespace so kustomize rewrites it", name)
			}
		}
	}

	assert.NotZero(t, checked, "should have found at least one binding for the operator ServiceAccount")
}
