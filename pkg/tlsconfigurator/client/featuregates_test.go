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

func featureGate(enabled, disabled []configv1.FeatureGateName) *configv1.FeatureGate {
	details := configv1.FeatureGateDetails{Version: version422}
	for _, name := range enabled {
		details.Enabled = append(details.Enabled, configv1.FeatureGateAttributes{Name: name})
	}
	for _, name := range disabled {
		details.Disabled = append(details.Disabled, configv1.FeatureGateAttributes{Name: name})
	}

	return &configv1.FeatureGate{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Status:     configv1.FeatureGateStatus{FeatureGates: []configv1.FeatureGateDetails{details}},
	}
}

func TestFeatureGateCheckerIsEnabled(t *testing.T) {
	tests := []struct {
		name string
		gate *configv1.FeatureGate
		want bool
	}{
		{
			name: "enabled",
			gate: featureGate([]configv1.FeatureGateName{FeatureGateTLSGroupPreferences}, nil),
			want: true,
		},
		{
			name: "explicitly disabled",
			gate: featureGate(nil, []configv1.FeatureGateName{FeatureGateTLSGroupPreferences}),
			want: false,
		},
		{
			// A cluster too old to know the gate lists it nowhere.
			name: "absent",
			gate: featureGate([]configv1.FeatureGateName{"SomeOtherGate"}, nil),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewFeatureGateCheckerWithClientset(configfake.NewClientset(tt.gate))

			got, err := c.IsEnabled(context.Background(), FeatureGateTLSGroupPreferences)
			if err != nil {
				t.Fatalf("IsEnabled() unexpected error = %v", err)
			}
			if got != tt.want {
				t.Errorf("IsEnabled(%q) = %v, want %v", FeatureGateTLSGroupPreferences, got, tt.want)
			}
		})
	}
}

// Failing to read the FeatureGate CR must be an error rather than a silent
// "disabled": the caller uses it to decide whether writing Groups is safe.
func TestFeatureGateCheckerErrorsWhenUnreadable(t *testing.T) {
	c := NewFeatureGateCheckerWithClientset(configfake.NewClientset())

	if _, err := c.IsEnabled(context.Background(), FeatureGateTLSGroupPreferences); err == nil {
		t.Fatal("IsEnabled() error = nil, want a not-found error")
	}
}
