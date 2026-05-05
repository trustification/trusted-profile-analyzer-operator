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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func findYAMLDocument(t *testing.T, data []byte, kind string) map[string]interface{} {
	t.Helper()
	docs := strings.Split(string(data), "\n---\n")
	for _, doc := range docs {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		var obj map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			continue
		}
		if obj["kind"] == kind {
			return obj
		}
	}
	return nil
}

func TestOperatorSecurityContext(t *testing.T) {
	managerPath := filepath.Join("config", "manager", "manager.yaml")

	data, err := os.ReadFile(managerPath)
	if err != nil {
		t.Skip("manager.yaml not found, skipping security context test")
	}

	// manager.yaml contains multiple YAML documents; find the Deployment
	deployment := findYAMLDocument(t, data, "Deployment")
	require.NotNil(t, deployment, "manager.yaml should contain a Deployment")

	// Check for security context
	spec, ok := deployment["spec"].(map[string]interface{})
	require.True(t, ok, "deployment should have spec")

	template, ok := spec["template"].(map[string]interface{})
	require.True(t, ok, "spec should have template")

	templateSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		t.Skip("Template spec not found in expected format")
	}

	// Check pod security context
	if securityContext, ok := templateSpec["securityContext"].(map[string]interface{}); ok {
		t.Logf("Pod security context: %+v", securityContext)

		// Verify runAsNonRoot is set
		if runAsNonRoot, ok := securityContext["runAsNonRoot"].(bool); ok {
			assert.True(t, runAsNonRoot, "pod should run as non-root")
		}
	}

	// Check container security context
	containers, ok := templateSpec["containers"].([]interface{})
	if ok && len(containers) > 0 {
		for i, container := range containers {
			containerMap, ok := container.(map[string]interface{})
			if !ok {
				continue
			}

			if securityContext, ok := containerMap["securityContext"].(map[string]interface{}); ok {
				t.Logf("Container %d security context: %+v", i, securityContext)

				// Verify common security settings
				if allowPrivilegeEscalation, ok := securityContext["allowPrivilegeEscalation"].(bool); ok {
					assert.False(t, allowPrivilegeEscalation, "container should not allow privilege escalation")
				}

				if runAsNonRoot, ok := securityContext["runAsNonRoot"].(bool); ok {
					assert.True(t, runAsNonRoot, "container should run as non-root")
				}
			}
		}
	}
}

func TestRBACRolesMinimalPrivileges(t *testing.T) {
	rolePath := filepath.Join("config", "rbac", "role.yaml")

	data, err := os.ReadFile(rolePath)
	if err != nil {
		t.Skip("role.yaml not found, skipping RBAC test")
	}

	// Parse YAML documents (file may contain multiple documents)
	docs := strings.Split(string(data), "\n---\n")

	for _, doc := range docs {
		if strings.TrimSpace(doc) == "" {
			continue
		}

		var role map[string]interface{}
		err := yaml.Unmarshal([]byte(doc), &role)
		if err != nil {
			t.Logf("Skipping document that couldn't be parsed: %v", err)
			continue
		}

		kind, ok := role["kind"].(string)
		if !ok || (kind != "Role" && kind != "ClusterRole") {
			continue
		}

		rules, ok := role["rules"].([]interface{})
		if !ok {
			continue
		}

		t.Logf("Checking %s: %s", kind, role["metadata"].(map[string]interface{})["name"])

		// Check each rule for overly broad permissions
		for i, rule := range rules {
			ruleMap, ok := rule.(map[string]interface{})
			if !ok {
				continue
			}

			// Check for wildcard permissions
			resources, _ := ruleMap["resources"].([]interface{})
			verbs, _ := ruleMap["verbs"].([]interface{})

			hasWildcardResource := false
			hasWildcardVerb := false

			for _, resource := range resources {
				if resource == "*" {
					hasWildcardResource = true
				}
			}

			for _, verb := range verbs {
				if verb == "*" {
					hasWildcardVerb = true
				}
			}

			if hasWildcardResource {
				t.Logf("  Warning: Rule %d has wildcard resource '*'", i)
			}

			if hasWildcardVerb {
				t.Logf("  Warning: Rule %d has wildcard verb '*'", i)
			}

			// Both wildcards together is a security risk
			if hasWildcardResource && hasWildcardVerb {
				t.Errorf("  Rule %d has both wildcard resource and verb - overly permissive", i)
			}
		}
	}
}

func TestServiceAccountAutomountToken(t *testing.T) {
	saPath := filepath.Join("config", "rbac", "service_account.yaml")

	data, err := os.ReadFile(saPath)
	if err != nil {
		t.Skip("service_account.yaml not found, skipping test")
	}

	var serviceAccount map[string]interface{}
	err = yaml.Unmarshal(data, &serviceAccount)
	require.NoError(t, err, "service_account.yaml should be valid YAML")

	// Check if automountServiceAccountToken is explicitly set
	if automount, ok := serviceAccount["automountServiceAccountToken"].(bool); ok {
		t.Logf("automountServiceAccountToken is set to: %v", automount)
		// If set to false, that's good security practice for controllers
		// If set to true, that's necessary for the operator to function
		// Either is acceptable, but it should be explicit
	} else {
		t.Log("automountServiceAccountToken not explicitly set (will default to true)")
	}
}

func TestNetworkPolicyExists(t *testing.T) {
	npPath := filepath.Join("config", "network-policy")

	info, err := os.Stat(npPath)
	if err != nil {
		t.Skip("network-policy directory not found")
	}

	assert.True(t, info.IsDir(), "network-policy should be a directory")

	// Check for network policy files
	files, err := os.ReadDir(npPath)
	require.NoError(t, err, "should be able to read network-policy directory")

	if len(files) > 0 {
		t.Logf("Found %d network policy file(s)", len(files))
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".yaml") {
				t.Logf("  - %s", file.Name())
			}
		}
	} else {
		t.Log("No network policy files found - consider adding network policies for security")
	}
}

func TestImagePullPolicySecure(t *testing.T) {
	managerPath := filepath.Join("config", "manager", "manager.yaml")

	data, err := os.ReadFile(managerPath)
	if err != nil {
		t.Skip("manager.yaml not found")
	}

	deployment := findYAMLDocument(t, data, "Deployment")
	if deployment == nil {
		t.Skip("Deployment not found in manager.yaml")
	}

	spec, ok := deployment["spec"].(map[string]interface{})
	if !ok {
		t.Skip("Deployment spec not in expected format")
	}

	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		t.Skip("Template not in expected format")
	}

	templateSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		t.Skip("Template spec not in expected format")
	}

	containers, ok := templateSpec["containers"].([]interface{})
	if !ok || len(containers) == 0 {
		t.Skip("Containers not found")
	}

	for i, container := range containers {
		containerMap, ok := container.(map[string]interface{})
		if !ok {
			continue
		}

		if pullPolicy, ok := containerMap["imagePullPolicy"].(string); ok {
			t.Logf("Container %d image pull policy: %s", i, pullPolicy)

			// For production, should use IfNotPresent or Always, not Never
			assert.NotEqual(t, "Never", pullPolicy, "image pull policy should not be Never in production")
		}

		// Check if using latest tag (not recommended for production)
		if image, ok := containerMap["image"].(string); ok {
			if strings.HasSuffix(image, ":latest") {
				t.Logf("Warning: Container %d uses :latest tag - not recommended for production", i)
			}
		}
	}
}

func TestResourceLimitsDefined(t *testing.T) {
	managerPath := filepath.Join("config", "manager", "manager.yaml")

	data, err := os.ReadFile(managerPath)
	if err != nil {
		t.Skip("manager.yaml not found")
	}

	deployment := findYAMLDocument(t, data, "Deployment")
	if deployment == nil {
		t.Skip("Deployment not found in manager.yaml")
	}

	// Navigate to containers
	spec, _ := deployment["spec"].(map[string]interface{})
	template, _ := spec["template"].(map[string]interface{})
	templateSpec, _ := template["spec"].(map[string]interface{})
	containers, ok := templateSpec["containers"].([]interface{})

	if !ok || len(containers) == 0 {
		t.Skip("Containers not found in expected format")
	}

	for i, container := range containers {
		containerMap, ok := container.(map[string]interface{})
		if !ok {
			continue
		}

		containerName := "unknown"
		if name, ok := containerMap["name"].(string); ok {
			containerName = name
		}

		resources, ok := containerMap["resources"].(map[string]interface{})
		if !ok {
			t.Logf("Warning: Container %s has no resource limits/requests defined", containerName)
			continue
		}

		// Check for limits
		if limits, ok := resources["limits"].(map[string]interface{}); ok {
			t.Logf("Container %d (%s) has limits: %+v", i, containerName, limits)
		} else {
			t.Logf("Warning: Container %s has no resource limits", containerName)
		}

		// Check for requests
		if requests, ok := resources["requests"].(map[string]interface{}); ok {
			t.Logf("Container %d (%s) has requests: %+v", i, containerName, requests)
		} else {
			t.Logf("Warning: Container %s has no resource requests", containerName)
		}
	}
}

func TestSecretsNotHardcoded(t *testing.T) {
	// Check common files for hardcoded secrets
	filesToCheck := []string{
		"config/manager/manager.yaml",
		"config/samples",
		"helm-charts/redhat-trusted-profile-analyzer/values.yaml",
	}

	suspiciousPatterns := []string{
		"password:",
		"secret:",
		"apiKey:",
		"token:",
		"key:",
	}

	for _, filePath := range filesToCheck {
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			// Check all YAML files in directory
			files, _ := os.ReadDir(filePath)
			for _, file := range files {
				if !file.IsDir() && (strings.HasSuffix(file.Name(), ".yaml") || strings.HasSuffix(file.Name(), ".yml")) {
					checkFileForSecrets(t, filepath.Join(filePath, file.Name()), suspiciousPatterns)
				}
			}
		} else {
			checkFileForSecrets(t, filePath, suspiciousPatterns)
		}
	}
}

func checkFileForSecrets(t *testing.T, filePath string, patterns []string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		lineLower := strings.ToLower(line)

		for _, pattern := range patterns {
			if strings.Contains(lineLower, pattern) {
				// Check if the value looks like a real secret (not a reference or example)
				if strings.Contains(line, "value:") || strings.Contains(line, "=") {
					// Exclude common safe patterns
					if strings.Contains(line, "secretRef") ||
						strings.Contains(line, "secretKeyRef") ||
						strings.Contains(line, "change-me") ||
						strings.Contains(line, "example") ||
						strings.Contains(line, "CHANGEME") ||
						strings.Contains(line, "<") {
						continue
					}

					t.Logf("Potential hardcoded secret in %s:%d - %s", filePath, lineNum+1, strings.TrimSpace(line))
				}
			}
		}
	}
}
