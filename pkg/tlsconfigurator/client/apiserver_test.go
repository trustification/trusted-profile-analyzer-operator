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
package client

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func apiServer(spec configv1.APIServerSpec) *configv1.APIServer {
	return &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       spec,
	}
}

func TestGetClusterTLSSettingsResolvesProfileAndAdherence(t *testing.T) {
	intermediate := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]

	tests := []struct {
		name          string
		spec          configv1.APIServerSpec
		wantSpec      configv1.TLSProfileSpec
		wantAdherence configv1.TLSAdherencePolicy
		wantHonor     bool
	}{
		{
			name:          "nothing set defaults to intermediate and legacy adherence",
			spec:          configv1.APIServerSpec{},
			wantSpec:      intermediate,
			wantAdherence: configv1.TLSAdherencePolicyNoOpinion,
			wantHonor:     false,
		},
		{
			name: "built-in type expands to its concrete spec",
			spec: configv1.APIServerSpec{
				TLSSecurityProfile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
			},
			wantSpec:      *configv1.TLSProfiles[configv1.TLSProfileModernType],
			wantAdherence: configv1.TLSAdherencePolicyNoOpinion,
			wantHonor:     false,
		},
		{
			name: "strict adherence is reported and requires honoring",
			spec: configv1.APIServerSpec{
				TLSSecurityProfile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType},
				TLSAdherence:       configv1.TLSAdherencePolicyStrictAllComponents,
			},
			wantSpec:      intermediate,
			wantAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
			wantHonor:     true,
		},
		{
			name: "legacy adherence does not require honoring",
			spec: configv1.APIServerSpec{
				TLSAdherence: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
			},
			wantSpec:      intermediate,
			wantAdherence: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
			wantHonor:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewAPIServerClientWithClientset(configfake.NewClientset(apiServer(tt.spec)))

			got, err := c.GetClusterTLSSettings(context.Background())
			if err != nil {
				t.Fatalf("GetClusterTLSSettings() unexpected error = %v", err)
			}

			if got.Spec.MinTLSVersion != tt.wantSpec.MinTLSVersion {
				t.Errorf("MinTLSVersion = %q, want %q", got.Spec.MinTLSVersion, tt.wantSpec.MinTLSVersion)
			}
			if len(got.Spec.Ciphers) != len(tt.wantSpec.Ciphers) {
				t.Errorf("Ciphers = %v, want %v", got.Spec.Ciphers, tt.wantSpec.Ciphers)
			}
			if len(got.Spec.Groups) != len(tt.wantSpec.Groups) {
				t.Errorf("Groups = %v, want %v", got.Spec.Groups, tt.wantSpec.Groups)
			}
			if got.Adherence != tt.wantAdherence {
				t.Errorf("Adherence = %q, want %q", got.Adherence, tt.wantAdherence)
			}
			if got.ShouldHonor() != tt.wantHonor {
				t.Errorf("ShouldHonor() = %v, want %v", got.ShouldHonor(), tt.wantHonor)
			}
		})
	}
}

// A Custom profile naming no Custom block is a malformed cluster config, not
// something to silently default away.
func TestGetClusterTLSSettingsRejectsCustomWithoutSpec(t *testing.T) {
	c := NewAPIServerClientWithClientset(configfake.NewClientset(apiServer(configv1.APIServerSpec{
		TLSSecurityProfile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType},
	})))

	if _, err := c.GetClusterTLSSettings(context.Background()); err == nil {
		t.Fatal("GetClusterTLSSettings() error = nil, want an error for Custom with no custom spec")
	}
}

func TestGetTLSAdherencePolicy(t *testing.T) {
	c := NewAPIServerClientWithClientset(configfake.NewClientset(apiServer(configv1.APIServerSpec{
		TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
	})))

	got, err := c.GetTLSAdherencePolicy(context.Background())
	if err != nil {
		t.Fatalf("GetTLSAdherencePolicy() unexpected error = %v", err)
	}
	if got != configv1.TLSAdherencePolicyStrictAllComponents {
		t.Errorf("GetTLSAdherencePolicy() = %q, want %q", got, configv1.TLSAdherencePolicyStrictAllComponents)
	}
}

func TestGetTLSAdherencePolicyWithoutAPIServer(t *testing.T) {
	c := NewAPIServerClientWithClientset(configfake.NewClientset())

	if _, err := c.GetTLSAdherencePolicy(context.Background()); err == nil {
		t.Fatal("GetTLSAdherencePolicy() error = nil, want a not-found error")
	}
}
