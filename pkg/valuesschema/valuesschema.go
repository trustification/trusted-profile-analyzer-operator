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

// Package valuesschema resolves the external JSON schemas that the chart's
// values.schema.json refers to by `urn:`.
//
// Helm validates values against the chart's schema before rendering. That
// schema needs definitions owned by other projects -- the trustify importer
// and auth configs, and a few Kubernetes core types -- which it used to `$ref`
// over the network. Those URLs made `helm template`/`install` fail outright in
// an air-gapped cluster.
//
// The refs are now `urn:` instead. Helm (>= 3.20) resolves that scheme through
// the pluggable chartutil.URNResolver, which Register installs from copies
// embedded in the operator binary, so validation is complete and offline.
//
// An unresolved `urn:` is deliberately not fatal: Helm logs a warning and
// substitutes a permissive schema. So a standalone `helm install` of this
// chart, with no operator to register anything, still works offline -- it just
// does not validate those subtrees. That graceful degradation is the point of
// using `urn:` rather than a remote or relative `$ref`.
package valuesschema

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"helm.sh/helm/v3/pkg/chartutil"
)

// The vendored copies and the manifest describing them. Refresh with
// hack/vendor-values-schemas.py --fetch.
//
//go:embed schemas/*.json
var schemas embed.FS

const schemasDir = "schemas"

const sourcesFile = "schemas/SOURCES.json"

type sources struct {
	Schemas map[string]struct {
		URN    string `json:"urn"`
		SHA256 string `json:"sha256"`
	} `json:"schemas"`
}

// Load parses the embedded schemas and returns them keyed by URN.
//
// It verifies each file against the digest recorded in SOURCES.json, so a
// vendored copy edited by hand instead of re-vendored is caught at startup
// rather than silently changing what the chart accepts.
func Load() (map[string]any, error) {
	raw, err := schemas.ReadFile(sourcesFile)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sourcesFile, err)
	}
	var src sources
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", sourcesFile, err)
	}
	if len(src.Schemas) == 0 {
		return nil, fmt.Errorf("%s declares no schemas", sourcesFile)
	}

	// Every embedded file must be accounted for in SOURCES.json. Without this
	// an added-but-unregistered schema would resolve to nothing at runtime and
	// silently fall back to permissive validation.
	entries, err := schemas.ReadDir(schemasDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", schemasDir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == path.Base(sourcesFile) {
			continue
		}
		if _, ok := src.Schemas[name]; !ok {
			return nil, fmt.Errorf("%s: embedded but not listed in %s", name, sourcesFile)
		}
	}

	out := make(map[string]any, len(src.Schemas))
	for name, meta := range src.Schemas {
		body, err := schemas.ReadFile(path.Join(schemasDir, name))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		digest := hex.EncodeToString(sha256Sum(body))
		if digest != meta.SHA256 {
			return nil, fmt.Errorf(
				"%s: sha256 %s does not match %s (%s); re-run hack/vendor-values-schemas.py",
				name, digest, sourcesFile, meta.SHA256)
		}
		if meta.URN == "" {
			return nil, fmt.Errorf("%s: no urn in %s", name, sourcesFile)
		}
		// UnmarshalJSON, not encoding/json: the validator needs numbers kept
		// as json.Number for the precision its comparisons rely on.
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}
		if _, dup := out[meta.URN]; dup {
			return nil, fmt.Errorf("%s: urn %s is claimed by more than one schema", name, meta.URN)
		}
		out[meta.URN] = doc
	}
	return out, nil
}

// URNs lists the registered URNs, sorted. Used for logging and in tests.
func URNs(docs map[string]any) []string {
	urns := make([]string, 0, len(docs))
	for urn := range docs {
		urns = append(urns, urn)
	}
	sort.Strings(urns)
	return urns
}

// Register installs the embedded schemas as Helm's URN resolver and returns
// the URNs it registered.
//
// This mutates package-level state in Helm, which is the only hook Helm
// exposes for this; call it once during startup, before any chart is rendered.
func Register() ([]string, error) {
	docs, err := Load()
	if err != nil {
		return nil, err
	}
	chartutil.URNResolver = func(urn string) (any, error) {
		doc, ok := docs[urn]
		if !ok {
			// Returning an error lets Helm apply its permissive fallback, which
			// is what we want for a URN this build does not know about.
			return nil, fmt.Errorf("no vendored schema for %s", urn)
		}
		return doc, nil
	}
	return URNs(docs), nil
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}
