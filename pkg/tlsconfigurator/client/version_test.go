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
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name       string
		versionStr string
		wantMajor  int
		wantMinor  int
		wantPatch  int
		wantErr    bool
	}{
		{
			name:       "simple version",
			versionStr: "4.22.0",
			wantMajor:  4,
			wantMinor:  22,
			wantPatch:  0,
			wantErr:    false,
		},
		{
			name:       "version with patch",
			versionStr: "4.22.5",
			wantMajor:  4,
			wantMinor:  22,
			wantPatch:  5,
			wantErr:    false,
		},
		{
			name:       "version with suffix",
			versionStr: "4.22.0-rc.1",
			wantMajor:  4,
			wantMinor:  22,
			wantPatch:  0,
			wantErr:    false,
		},
		{
			name:       "nightly version",
			versionStr: "4.23.0-0.nightly-2024-01-01-123456",
			wantMajor:  4,
			wantMinor:  23,
			wantPatch:  0,
			wantErr:    false,
		},
		{
			name:       "version 4.21",
			versionStr: "4.21.0",
			wantMajor:  4,
			wantMinor:  21,
			wantPatch:  0,
			wantErr:    false,
		},
		{
			name:       "version 5.0",
			versionStr: "5.0.0",
			wantMajor:  5,
			wantMinor:  0,
			wantPatch:  0,
			wantErr:    false,
		},
		{
			name:       "invalid - no minor version",
			versionStr: "4",
			wantErr:    true,
		},
		{
			name:       "invalid - not a number",
			versionStr: "abc.def.ghi",
			wantErr:    true,
		},
		{
			name:       "invalid - empty",
			versionStr: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVersion(tt.versionStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Major != tt.wantMajor {
				t.Errorf("parseVersion() Major = %d, want %d", got.Major, tt.wantMajor)
			}
			if got.Minor != tt.wantMinor {
				t.Errorf("parseVersion() Minor = %d, want %d", got.Minor, tt.wantMinor)
			}
			if got.Patch != tt.wantPatch {
				t.Errorf("parseVersion() Patch = %d, want %d", got.Patch, tt.wantPatch)
			}
		})
	}
}

func TestOpenShiftVersion_IsAtLeast(t *testing.T) {
	tests := []struct {
		name     string
		version  *OpenShiftVersion
		reqMajor int
		reqMinor int
		want     bool
	}{
		{
			name:     "exactly minimum version",
			version:  &OpenShiftVersion{Major: 4, Minor: 22, Patch: 0},
			reqMajor: 4,
			reqMinor: 22,
			want:     true,
		},
		{
			name:     "newer minor version",
			version:  &OpenShiftVersion{Major: 4, Minor: 23, Patch: 0},
			reqMajor: 4,
			reqMinor: 22,
			want:     true,
		},
		{
			name:     "newer major version",
			version:  &OpenShiftVersion{Major: 5, Minor: 0, Patch: 0},
			reqMajor: 4,
			reqMinor: 22,
			want:     true,
		},
		{
			name:     "older minor version",
			version:  &OpenShiftVersion{Major: 4, Minor: 21, Patch: 0},
			reqMajor: 4,
			reqMinor: 22,
			want:     false,
		},
		{
			name:     "older major version",
			version:  &OpenShiftVersion{Major: 3, Minor: 11, Patch: 0},
			reqMajor: 4,
			reqMinor: 22,
			want:     false,
		},
		{
			name:     "patch version doesn't matter",
			version:  &OpenShiftVersion{Major: 4, Minor: 22, Patch: 10},
			reqMajor: 4,
			reqMinor: 22,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.version.IsAtLeast(tt.reqMajor, tt.reqMinor)
			if got != tt.want {
				t.Errorf("IsAtLeast(%d, %d) = %v, want %v for version %s",
					tt.reqMajor, tt.reqMinor, got, tt.want, tt.version.String())
			}
		})
	}
}

func TestOpenShiftVersion_String(t *testing.T) {
	tests := []struct {
		name    string
		version *OpenShiftVersion
		want    string
	}{
		{
			name:    "with full version",
			version: &OpenShiftVersion{Major: 4, Minor: 22, Patch: 0, Full: "4.22.0-rc.1"},
			want:    "4.22.0-rc.1",
		},
		{
			name:    "without full version",
			version: &OpenShiftVersion{Major: 4, Minor: 22, Patch: 5},
			want:    "4.22.5",
		},
		{
			name:    "zero patch",
			version: &OpenShiftVersion{Major: 4, Minor: 22, Patch: 0},
			want:    "4.22.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.version.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
