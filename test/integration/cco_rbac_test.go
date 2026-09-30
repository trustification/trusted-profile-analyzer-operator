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
	assert.Equal(t, "rhtpa-operator-controller-manager", binding.Subjects[0].Name,
		"subject should reference the operator's service account")
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
