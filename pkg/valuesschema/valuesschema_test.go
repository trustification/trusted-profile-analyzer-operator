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

package valuesschema

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"sigs.k8s.io/yaml"
)

const chartDir = "../../helm-charts/redhat-trusted-profile-analyzer"

// restoreResolver puts Helm's package-level resolver back after a test that
// swaps it, so ordering between tests cannot leak.
func restoreResolver(t *testing.T) {
	t.Helper()
	saved := chartutil.URNResolver
	t.Cleanup(func() { chartutil.URNResolver = saved })
}

func TestLoadVerifiesDigestsAndURNs(t *testing.T) {
	docs, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{
		"urn:k8s:v1.29.0:EnvVarSource",
		"urn:k8s:v1.29.0:Quantity",
		"urn:k8s:v1.29.0:Volume",
		"urn:k8s:v1.29.0:VolumeMount",
		"urn:trustify:auth",
		"urn:trustify:importer",
	}
	got := URNs(docs)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("registered URNs:\n got %v\nwant %v", got, want)
	}
}

// The chart must not reach the network to validate values. This is the whole
// point of the vendoring, and it is a one-character edit away from regressing.
func TestChartSchemasHaveNoRemoteRefs(t *testing.T) {
	docs, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	remote := regexp.MustCompile(`\$ref"?\s*:\s*"(https?://[^"]+)"`)
	urnRef := regexp.MustCompile(`"(urn:[^"]*)"`)

	for _, name := range []string{"values.schema.json", "values.schema.yaml"} {
		body, err := os.ReadFile(filepath.Join(chartDir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for _, m := range remote.FindAllStringSubmatch(string(body), -1) {
			t.Errorf("%s: remote $ref reintroduced: %s", name, m[1])
		}
		for _, m := range urnRef.FindAllStringSubmatch(string(body), -1) {
			urn := m[1]
			// A URN carrying a JSON-pointer fragment breaks the offline
			// fallback: Helm substitutes a bare `true` schema for an
			// unregistered URN, and `#/...` cannot resolve against it.
			if strings.Contains(urn, "#") {
				t.Errorf("%s: %s has a JSON-pointer fragment", name, urn)
				continue
			}
			if _, ok := docs[urn]; !ok {
				t.Errorf("%s: %s is referenced but not vendored", name, urn)
			}
		}
	}
}

// validateChart runs the chart's real values schema over the chart defaults
// coalesced with the given user values, the same way Helm does when rendering.
//
// The user values come from test/fixtures/valid_cr.yaml rather than being
// written out here: the chart's own values.yaml is not a valid input on its
// own (storage and database carry required fields that only a CR supplies).
func validateChart(t *testing.T, mutate func(spec map[string]any)) error {
	t.Helper()
	chrt, err := loader.Load(chartDir)
	if err != nil {
		t.Fatalf("loading chart: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "valid_cr.yaml"))
	if err != nil {
		t.Fatalf("reading valid_cr.yaml: %v", err)
	}
	var cr struct {
		Spec map[string]any `yaml:"spec"`
	}
	if err := yaml.Unmarshal(raw, &cr); err != nil {
		t.Fatalf("parsing valid_cr.yaml: %v", err)
	}
	if mutate != nil {
		mutate(cr.Spec)
	}

	values, err := chartutil.CoalesceValues(chrt, cr.Spec)
	if err != nil {
		t.Fatalf("coalescing values: %v", err)
	}
	return chartutil.ValidateAgainstSchema(chrt, values)
}

func TestChartDefaultsValidateWithRegisteredSchemas(t *testing.T) {
	restoreResolver(t)
	if _, err := Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := validateChart(t, nil); err != nil {
		t.Fatalf("valid CR rejected by chart schema: %v", err)
	}
}

// Registering the schemas must actually tighten validation -- otherwise the
// whole mechanism could be silently inert and the tests above would still pass.
func TestRegisteredSchemasAreEnforced(t *testing.T) {
	restoreResolver(t)
	if _, err := Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// `period` is a trustify HumantimeSerde (a string); an object is not
	// valid. This can only be rejected if urn:trustify:importer resolved.
	err := validateChart(t, func(spec map[string]any) {
		spec["modules"] = map[string]any{
			"createImporters": map[string]any{
				"importers": map[string]any{
					"bad": map[string]any{
						"sbom": map[string]any{
							"source": "https://example.com",
							"period": map[string]any{"not": "a duration"},
						},
					},
				},
			},
		}
	})
	if err == nil {
		t.Fatal("invalid importer config accepted: urn:trustify:importer was not enforced")
	}
	if !strings.Contains(err.Error(), "importers") {
		t.Errorf("rejected, but not for the importer: %v", err)
	}
}

// A standalone `helm install` has no operator to register anything. That must
// degrade to permissive validation rather than failing, which is why the refs
// are `urn:` and not remote or relative.
func TestUnregisteredURNsDegradeGracefully(t *testing.T) {
	restoreResolver(t)
	chartutil.URNResolver = func(urn string) (any, error) {
		return nil, errUnresolved(urn)
	}
	if err := validateChart(t, nil); err != nil {
		t.Fatalf("valid CR rejected with no resolver registered: %v", err)
	}
}

type errUnresolved string

func (e errUnresolved) Error() string { return "unresolved: " + string(e) }
