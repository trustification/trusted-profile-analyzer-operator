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

// CRD represents a CustomResourceDefinition
type CRD struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Group string `yaml:"group"`
		Names struct {
			Kind       string   `yaml:"kind"`
			ListKind   string   `yaml:"listKind"`
			Plural     string   `yaml:"plural"`
			Singular   string   `yaml:"singular"`
			ShortNames []string `yaml:"shortNames"`
		} `yaml:"names"`
		Scope    string `yaml:"scope"`
		Versions []struct {
			Name    string `yaml:"name"`
			Served  bool   `yaml:"served"`
			Storage bool   `yaml:"storage"`
		} `yaml:"versions"`
	} `yaml:"spec"`
}

func TestCRDExists(t *testing.T) {
	crdPath := filepath.Join("config", "crd", "bases", "rhtpa.io_trustedprofileanalyzers.yaml")

	_, err := os.Stat(crdPath)
	require.NoError(t, err, "CRD file should exist")
}

func TestCRDValid(t *testing.T) {
	crdPath := filepath.Join("config", "crd", "bases", "rhtpa.io_trustedprofileanalyzers.yaml")

	data, err := os.ReadFile(crdPath)
	require.NoError(t, err, "CRD should be readable")

	var crd CRD
	err = yaml.Unmarshal(data, &crd)
	require.NoError(t, err, "CRD should be valid YAML")

	// Validate CRD metadata
	assert.Equal(t, "apiextensions.k8s.io/v1", crd.APIVersion, "CRD API version should be v1")
	assert.Equal(t, "CustomResourceDefinition", crd.Kind, "Kind should be CustomResourceDefinition")
	assert.Equal(t, "trustedprofileanalyzers.rhtpa.io", crd.Metadata.Name, "CRD name should be correct")

	// Validate spec
	assert.Equal(t, "rhtpa.io", crd.Spec.Group, "Group should be rhtpa.io")
	assert.Equal(t, "TrustedProfileAnalyzer", crd.Spec.Names.Kind, "Kind should be TrustedProfileAnalyzer")
	assert.Equal(t, "TrustedProfileAnalyzerList", crd.Spec.Names.ListKind, "ListKind should be correct")
	assert.Equal(t, "trustedprofileanalyzers", crd.Spec.Names.Plural, "Plural should be correct")
	assert.Equal(t, "trustedprofileanalyzer", crd.Spec.Names.Singular, "Singular should be correct")
	assert.Contains(t, crd.Spec.Names.ShortNames, "rhtpa", "ShortNames should include rhtpa")
	assert.Equal(t, "Namespaced", crd.Spec.Scope, "Scope should be Namespaced")

	// Validate versions
	require.NotEmpty(t, crd.Spec.Versions, "CRD should have versions")

	v1Found := false
	storageVersionFound := false

	for _, version := range crd.Spec.Versions {
		if version.Name == "v1" {
			v1Found = true
			assert.True(t, version.Served, "v1 should be served")
			if version.Storage {
				storageVersionFound = true
			}
		}
	}

	assert.True(t, v1Found, "v1 version should exist")
	assert.True(t, storageVersionFound, "One version should be storage version")
}

func TestSampleCRsValid(t *testing.T) {
	samplesPath := filepath.Join("config", "samples")

	samples, err := os.ReadDir(samplesPath)
	require.NoError(t, err, "samples directory should be readable")

	foundSample := false
	for _, sample := range samples {
		if sample.IsDir() || filepath.Ext(sample.Name()) != ".yaml" {
			continue
		}

		if sample.Name() == "kustomization.yaml" {
			continue
		}

		foundSample = true
		samplePath := filepath.Join(samplesPath, sample.Name())

		data, err := os.ReadFile(samplePath)
		require.NoError(t, err, "sample %s should be readable", sample.Name())

		var cr map[string]interface{}
		err = yaml.Unmarshal(data, &cr)
		require.NoError(t, err, "sample %s should be valid YAML", sample.Name())

		// Validate basic CR structure
		assert.Equal(t, "rhtpa.io/v1", cr["apiVersion"], "sample should have correct apiVersion")
		assert.Equal(t, "TrustedProfileAnalyzer", cr["kind"], "sample should have correct kind")
		assert.NotNil(t, cr["metadata"], "sample should have metadata")
		assert.NotNil(t, cr["spec"], "sample should have spec")
	}

	assert.True(t, foundSample, "at least one sample CR should exist")
}
