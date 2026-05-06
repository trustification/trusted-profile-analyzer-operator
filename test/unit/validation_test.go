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

package unit

import (
	"os"
	"testing"

	"github.com/operator-framework/helm-operator-plugins/pkg/watches"
	"github.com/stretchr/testify/assert"
)

func TestWatchLoadFromValidYAML(t *testing.T) {
	// This test verifies watches.Load validates chart paths
	// We expect it to fail because the chart doesn't exist
	yamlContent := `
- group: test.io
  version: v1
  kind: TestKind
  chart: /path/to/chart
`
	tmpfile, err := createTempYAMLFile(t, yamlContent)
	if err != nil {
		t.Skip("cannot create temp file")
	}

	_, err = watches.Load(tmpfile.Name())
	// Should error because chart path doesn't exist
	assert.Error(t, err, "should error on non-existent chart path")
}

func TestWatchLoadNonExistentFile(t *testing.T) {
	_, err := watches.Load("/nonexistent/path/watches.yaml")
	assert.Error(t, err, "non-existent file should return error")
}

// Helper function to create temporary YAML file for testing
func createTempYAMLFile(t *testing.T, content string) (*os.File, error) {
	t.Helper()

	tmpfile, err := os.CreateTemp("", "watches-*.yaml")
	if err != nil {
		return nil, err
	}

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		_ = tmpfile.Close()
		_ = os.Remove(tmpfile.Name())
		return nil, err
	}

	if err := tmpfile.Close(); err != nil {
		_ = os.Remove(tmpfile.Name())
		return nil, err
	}

	// Reopen for reading
	tmpfile, err = os.Open(tmpfile.Name())
	if err != nil {
		_ = os.Remove(tmpfile.Name())
		return nil, err
	}

	// Clean up on test completion
	t.Cleanup(func() {
		_ = tmpfile.Close()
		_ = os.Remove(tmpfile.Name())
	})

	return tmpfile, nil
}
