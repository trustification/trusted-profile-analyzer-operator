/*
Copyright 2025.

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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Role represents a Kubernetes Role
type Role struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Rules []struct {
		APIGroups []string `yaml:"apiGroups"`
		Resources []string `yaml:"resources"`
		Verbs     []string `yaml:"verbs"`
	} `yaml:"rules"`
}

func TestRBACConfigurationExists(t *testing.T) {
	rbacPath := filepath.Join("config", "rbac")

	info, err := os.Stat(rbacPath)
	require.NoError(t, err, "RBAC directory should exist")
	assert.True(t, info.IsDir(), "RBAC path should be a directory")
}

func TestManagerRoleExists(t *testing.T) {
	rolePath := filepath.Join("config", "rbac", "role.yaml")

	data, err := os.ReadFile(rolePath)
	require.NoError(t, err, "manager role should exist and be readable")

	var role Role
	err = yaml.Unmarshal(data, &role)
	require.NoError(t, err, "manager role should be valid YAML")

	assert.Equal(t, "rbac.authorization.k8s.io/v1", role.APIVersion, "role should use correct API version")
	assert.Contains(t, []string{"Role", "ClusterRole"}, role.Kind, "should be a Role or ClusterRole")
}

func TestJobRoleConfiguration(t *testing.T) {
	rolePath := filepath.Join("config", "rbac", "role_job.yaml")

	data, err := os.ReadFile(rolePath)
	require.NoError(t, err, "job role should exist")

	var role Role
	err = yaml.Unmarshal(data, &role)
	require.NoError(t, err, "job role should be valid YAML")

	// Verify job permissions
	foundJobPermissions := false
	for _, rule := range role.Rules {
		if contains(rule.APIGroups, "batch") && contains(rule.Resources, "jobs") {
			foundJobPermissions = true
			assert.Contains(t, rule.Verbs, "create", "should be able to create jobs")
			assert.Contains(t, rule.Verbs, "get", "should be able to get jobs")
			assert.Contains(t, rule.Verbs, "list", "should be able to list jobs")
			assert.Contains(t, rule.Verbs, "watch", "should be able to watch jobs")
		}
	}

	assert.True(t, foundJobPermissions, "role should have job permissions")
}

func TestIngressRoleConfiguration(t *testing.T) {
	rolePath := filepath.Join("config", "rbac", "role_ingress.yaml")

	data, err := os.ReadFile(rolePath)
	require.NoError(t, err, "ingress role should exist")

	var role Role
	err = yaml.Unmarshal(data, &role)
	require.NoError(t, err, "ingress role should be valid YAML")

	// Verify ingress permissions
	foundIngressPermissions := false
	for _, rule := range role.Rules {
		if contains(rule.APIGroups, "networking.k8s.io") && contains(rule.Resources, "ingresses") {
			foundIngressPermissions = true
			assert.Contains(t, rule.Verbs, "create", "should be able to create ingresses")
			assert.Contains(t, rule.Verbs, "get", "should be able to get ingresses")
		}
	}

	assert.True(t, foundIngressPermissions, "role should have ingress permissions")
}

func TestServiceAccountExists(t *testing.T) {
	saPath := filepath.Join("config", "rbac", "service_account.yaml")

	data, err := os.ReadFile(saPath)
	require.NoError(t, err, "service account should exist")

	var sa map[string]interface{}
	err = yaml.Unmarshal(data, &sa)
	require.NoError(t, err, "service account should be valid YAML")

	assert.Equal(t, "v1", sa["apiVersion"], "service account should use v1 API")
	assert.Equal(t, "ServiceAccount", sa["kind"], "kind should be ServiceAccount")
}

func TestLeaderElectionRBAC(t *testing.T) {
	rolePath := filepath.Join("config", "rbac", "leader_election_role.yaml")

	data, err := os.ReadFile(rolePath)
	require.NoError(t, err, "leader election role should exist")

	var role Role
	err = yaml.Unmarshal(data, &role)
	require.NoError(t, err, "leader election role should be valid YAML")

	// Verify leader election permissions
	foundLeasePermissions := false
	for _, rule := range role.Rules {
		if contains(rule.Resources, "leases") {
			foundLeasePermissions = true
			assert.Contains(t, rule.Verbs, "get", "should be able to get leases")
			assert.Contains(t, rule.Verbs, "create", "should be able to create leases")
			assert.Contains(t, rule.Verbs, "update", "should be able to update leases")
		}
	}

	assert.True(t, foundLeasePermissions, "role should have lease permissions for leader election")
}

func TestRoleBindingsExist(t *testing.T) {
	rbacPath := filepath.Join("config", "rbac")

	files, err := os.ReadDir(rbacPath)
	require.NoError(t, err, "should be able to read RBAC directory")

	foundRoleBinding := false
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".yaml" &&
			(filepath.Base(file.Name()) == "role_binding.yaml" ||
				filepath.Base(file.Name()) == "rolebinding_job.yaml" ||
				filepath.Base(file.Name()) == "rolebinding_ingress.yaml") {
			foundRoleBinding = true

			data, err := os.ReadFile(filepath.Join(rbacPath, file.Name()))
			require.NoError(t, err, "role binding should be readable: %s", file.Name())

			var rb map[string]interface{}
			err = yaml.Unmarshal(data, &rb)
			require.NoError(t, err, "role binding should be valid YAML: %s", file.Name())

			assert.Contains(t, []string{"RoleBinding", "ClusterRoleBinding"}, rb["kind"],
				"should be a RoleBinding or ClusterRoleBinding: %s", file.Name())
		}
	}

	assert.True(t, foundRoleBinding, "at least one role binding should exist")
}

// Helper function to check if a slice contains a string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
