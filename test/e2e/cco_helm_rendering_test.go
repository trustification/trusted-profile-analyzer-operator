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

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	fieldCloudProvider         = "cloudProvider"
	fieldCloudCredentials      = "cloudCredentials"
	fieldCcoMode               = "ccoMode"
	kindCredentialsReq         = "CredentialsRequest"
	ccoSecretName              = "test-release-cloud-creds"
	ccoNamespace               = "openshift-cloud-credential-operator"
	testBucket                 = "trustify-storage"
	testRegionUSEast1          = "us-east-1"
	testSTSRoleARN             = "arn:aws:iam::123456789012:role/trustify-s3-role"
	testGCPServiceAccountEmail = "trustify-sa@my-project.iam.gserviceaccount.com"
	testAzureClientID          = "11111111-1111-1111-1111-111111111111"
	testAzureTenantID          = "22222222-2222-2222-2222-222222222222"
	testAzureSubscriptionID    = "00000000-0000-0000-0000-000000000000"
	cloudAWS                   = "aws"
	cloudGCP                   = "gcp"
	cloudAzure                 = "azure"
	ccoModeManual              = "manual"
	testAzureRegion            = "eastus"
)

func testDatabaseValues() map[string]interface{} {
	return map[string]interface{}{
		"host":     "postgres.test.svc",
		fieldName:  "testdb",
		"username": "testuser",
		"password": "testpass",
	}
}

func awsValues() map[string]interface{} {
	return map[string]interface{}{
		fieldAppDomain:     testAppDomain,
		fieldCloudProvider: cloudAWS,
		fieldCloudCredentials: map[string]interface{}{
			cloudAWS: map[string]interface{}{
				"statementEntries": []interface{}{
					map[string]interface{}{
						"effect":   "Allow",
						"action":   []interface{}{"s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:ListBucket"},
						"resource": "*",
					},
				},
			},
		},
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: testBucket,
			fieldRegion: testRegionUSEast1,
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}
}

func gcpValues() map[string]interface{} {
	return map[string]interface{}{
		fieldAppDomain:     testAppDomain,
		fieldCloudProvider: cloudGCP,
		fieldCloudCredentials: map[string]interface{}{
			cloudGCP: map[string]interface{}{
				"permissions": []interface{}{
					"storage.objects.get",
					"storage.objects.create",
					"storage.objects.delete",
					"storage.buckets.get",
				},
			},
		},
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: testBucket,
			fieldRegion: "us-central1",
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}
}

func azureValues() map[string]interface{} {
	return map[string]interface{}{
		fieldAppDomain:     testAppDomain,
		fieldCloudProvider: cloudAzure,
		fieldCloudCredentials: map[string]interface{}{
			cloudAzure: map[string]interface{}{
				"roleBindings": []interface{}{
					map[string]interface{}{
						"role":  "Contributor",
						"scope": "/subscriptions/" + testAzureSubscriptionID,
					},
				},
				"azureSubscriptionID": testAzureSubscriptionID,
				"azureRegion":         testAzureRegion,
			},
		},
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: testBucket,
			fieldRegion: testAzureRegion,
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}
}

func azureManualValues() map[string]interface{} {
	return map[string]interface{}{
		fieldAppDomain:     testAppDomain,
		fieldCloudProvider: cloudAzure,
		fieldCcoMode:       ccoModeManual,
		fieldCloudCredentials: map[string]interface{}{
			cloudAzure: map[string]interface{}{
				"roleBindings": []interface{}{
					map[string]interface{}{
						"role":  "Contributor",
						"scope": "/subscriptions/" + testAzureSubscriptionID,
					},
				},
				"azureClientID":       testAzureClientID,
				"azureTenantID":       testAzureTenantID,
				"azureSubscriptionID": testAzureSubscriptionID,
				"azureRegion":         testAzureRegion,
			},
		},
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: testBucket,
			fieldRegion: testAzureRegion,
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}
}

func gcpManualValues() map[string]interface{} {
	return map[string]interface{}{
		fieldAppDomain:     testAppDomain,
		fieldCloudProvider: cloudGCP,
		fieldCcoMode:       ccoModeManual,
		fieldCloudCredentials: map[string]interface{}{
			cloudGCP: map[string]interface{}{
				"permissions": []interface{}{
					"storage.objects.get",
					"storage.objects.create",
					"storage.objects.delete",
					"storage.buckets.get",
				},
				"serviceAccountEmail": testGCPServiceAccountEmail,
			},
		},
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: testBucket,
			fieldRegion: "us-central1",
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}
}

func awsManualValues() map[string]interface{} {
	return map[string]interface{}{
		fieldAppDomain:     testAppDomain,
		fieldCloudProvider: cloudAWS,
		fieldCcoMode:       ccoModeManual,
		fieldCloudCredentials: map[string]interface{}{
			cloudAWS: map[string]interface{}{
				"statementEntries": []interface{}{
					map[string]interface{}{
						"effect":   "Allow",
						"action":   []interface{}{"s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:ListBucket"},
						"resource": "*",
					},
				},
				"stsIAMRoleARN": testSTSRoleARN,
			},
		},
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: testBucket,
			fieldRegion: testRegionUSEast1,
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}
}

func awsPassthroughValues() map[string]interface{} {
	v := awsValues()
	v[fieldCcoMode] = "passthrough"
	return v
}

func awsDefaultValues() map[string]interface{} {
	v := awsValues()
	v[fieldCcoMode] = "default"
	return v
}

// parseYAMLDoc parses a YAML document via JSON round-trip so that numeric
// types are float64 (compatible with k8s unstructured helpers).
func parseYAMLDoc(doc string) (map[string]interface{}, error) {
	var intermediate interface{}
	if err := yaml.Unmarshal([]byte(doc), &intermediate); err != nil {
		return nil, err
	}
	jsonBytes, err := json.Marshal(intermediate)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func findCredentialsRequest(t *testing.T, docs []string) map[string]interface{} {
	t.Helper()
	for _, doc := range docs {
		obj, err := parseYAMLDoc(doc)
		if err != nil {
			continue
		}
		if obj["kind"] == kindCredentialsReq {
			return obj
		}
	}
	return nil
}

func nestedString(obj map[string]interface{}, keys ...string) string {
	current := obj
	for i, key := range keys {
		if i == len(keys)-1 {
			v, _ := current[key].(string)
			return v
		}
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

func TestHelmRenderAWSCredentialsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsValues())
	docs := splitYAMLDocs(rendered)

	cr := findCredentialsRequest(t, docs)
	require.NotNil(t, cr, "CredentialsRequest should be rendered for AWS")

	metadata, _ := cr[fieldMetadata].(map[string]interface{})
	require.NotNil(t, metadata, "metadata should exist")
	assert.Equal(t, ccoSecretName, metadata[fieldName], "CredentialsRequest name should match release")
	assert.Equal(t, ccoNamespace, metadata[fieldNamespace], "CredentialsRequest should be in CCO namespace")

	spec, _ := cr[fieldSpec].(map[string]interface{})
	require.NotNil(t, spec, "spec should exist")

	assert.Equal(t, ccoSecretName, nestedString(spec, "secretRef", fieldName), "secretRef.name should match")

	providerSpec, _ := spec["providerSpec"].(map[string]interface{})
	require.NotNil(t, providerSpec, "providerSpec should exist")
	assert.Equal(t, "AWSProviderSpec", providerSpec["kind"], "providerSpec should be AWSProviderSpec")
	assert.Equal(t, "cloudcredential.openshift.io/v1",
		providerSpec[fieldAPIVersion], "providerSpec apiVersion should match")

	entries, ok := providerSpec["statementEntries"].([]interface{})
	assert.True(t, ok, "statementEntries should be a list")
	assert.NotEmpty(t, entries, "statementEntries should not be empty")

	assert.NotContains(t, rendered, "GCPProviderSpec", "AWS config should not contain GCP provider")
	assert.NotContains(t, rendered, "predefinedRoles", "AWS config should not contain predefinedRoles")
}

func TestHelmRenderGCPCredentialsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, gcpValues())
	docs := splitYAMLDocs(rendered)

	cr := findCredentialsRequest(t, docs)
	require.NotNil(t, cr, "CredentialsRequest should be rendered for GCP")

	metadata, _ := cr[fieldMetadata].(map[string]interface{})
	require.NotNil(t, metadata, "metadata should exist")
	assert.Equal(t, ccoSecretName, metadata[fieldName], "CredentialsRequest name should match release")
	assert.Equal(t, ccoNamespace, metadata[fieldNamespace], "CredentialsRequest should be in CCO namespace")

	spec, _ := cr[fieldSpec].(map[string]interface{})
	require.NotNil(t, spec, "spec should exist")

	providerSpec, _ := spec["providerSpec"].(map[string]interface{})
	require.NotNil(t, providerSpec, "providerSpec should exist")
	assert.Equal(t, "GCPProviderSpec", providerSpec["kind"], "providerSpec should be GCPProviderSpec")

	roles, ok := providerSpec["predefinedRoles"].([]interface{})
	assert.True(t, ok, "predefinedRoles should be a list")
	assert.NotEmpty(t, roles, "predefinedRoles should not be empty")

	assert.NotContains(t, rendered, "AWSProviderSpec", "GCP config should not contain AWS provider")
	assert.NotContains(t, rendered, "statementEntries", "GCP config should not contain statementEntries")
}

func TestHelmRenderNoCredentialsRequestWithoutCloudProvider(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: "test-bucket",
			fieldRegion: testRegionUSEast1,
			"accessKey": "test-key",
			"secretKey": "test-secret",
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}

	rendered := renderHelmChart(t, chartPath, values)
	assert.NotEmpty(t, rendered, "chart should render without cloudProvider")

	docs := splitYAMLDocs(rendered)
	cr := findCredentialsRequest(t, docs)
	assert.Nil(t, cr, "CredentialsRequest should NOT be rendered without cloudProvider")
}

func TestHelmRenderStorageEnvVarsWithCCO(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsValues())

	assert.Contains(t, rendered, ccoSecretName,
		"rendered output should reference CCO secret")
	assert.Contains(t, rendered, "aws_access_key_id",
		"rendered output should reference aws_access_key_id key")
	assert.Contains(t, rendered, "aws_secret_access_key",
		"rendered output should reference aws_secret_access_key key")

	docs := splitYAMLDocs(rendered)
	for _, doc := range docs {
		obj, err := parseYAMLDoc(doc)
		if err != nil {
			continue
		}
		if obj["kind"] != "Deployment" {
			continue
		}
		metadata, _ := obj[fieldMetadata].(map[string]interface{})
		if metadata == nil || !strings.Contains(metadata[fieldName].(string), fieldServer) {
			continue
		}

		spec, _ := obj[fieldSpec].(map[string]interface{})
		template, _ := spec["template"].(map[string]interface{})
		templateSpec, _ := template[fieldSpec].(map[string]interface{})
		containers, _ := templateSpec["containers"].([]interface{})
		require.NotEmpty(t, containers, "server deployment should have containers")

		container, _ := containers[0].(map[string]interface{})
		envList, _ := container["env"].([]interface{})
		require.NotEmpty(t, envList, "container should have env vars")

		accessKeyFound := false
		secretKeyFound := false
		for _, e := range envList {
			env, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			envName, _ := env[fieldName].(string)
			if envName == "TRUSTD_S3_ACCESS_KEY" {
				refName := nestedString(env, "valueFrom", "secretKeyRef", fieldName)
				refKey := nestedString(env, "valueFrom", "secretKeyRef", "key")
				assert.Equal(t, ccoSecretName, refName, "ACCESS_KEY should reference CCO secret")
				assert.Equal(t, "aws_access_key_id", refKey, "ACCESS_KEY should use correct key")
				accessKeyFound = true
			}
			if envName == "TRUSTD_S3_SECRET_KEY" {
				refName := nestedString(env, "valueFrom", "secretKeyRef", fieldName)
				refKey := nestedString(env, "valueFrom", "secretKeyRef", "key")
				assert.Equal(t, ccoSecretName, refName, "SECRET_KEY should reference CCO secret")
				assert.Equal(t, "aws_secret_access_key", refKey, "SECRET_KEY should use correct key")
				secretKeyFound = true
			}
		}
		assert.True(t, accessKeyFound, "TRUSTD_S3_ACCESS_KEY env var should be present")
		assert.True(t, secretKeyFound, "TRUSTD_S3_SECRET_KEY env var should be present")
		return
	}
	t.Fatal("server deployment not found in rendered output")
}

func TestHelmRenderStorageEnvVarsWithoutCCO(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: testBucket,
			fieldRegion: testRegionUSEast1,
			"accessKey": "explicit-access-key",
			"secretKey": "explicit-secret-key",
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}

	rendered := renderHelmChart(t, chartPath, values)

	assert.NotContains(t, rendered, "cloud-creds",
		"rendered output should NOT reference CCO secret when cloudProvider is not set")
	assert.Contains(t, rendered, "explicit-access-key",
		"rendered output should contain explicit access key")
	assert.Contains(t, rendered, "explicit-secret-key",
		"rendered output should contain explicit secret key")
}

func TestHelmRenderAWSManualCredentialsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsManualValues())
	docs := splitYAMLDocs(rendered)

	cr := findCredentialsRequest(t, docs)
	require.NotNil(t, cr, "CredentialsRequest should be rendered for AWS manual mode")

	spec, _ := cr[fieldSpec].(map[string]interface{})
	require.NotNil(t, spec, "spec should exist")

	assert.Equal(t, "/var/run/secrets/openshift/serviceaccount/token",
		spec["cloudTokenPath"], "cloudTokenPath should be set for manual mode")

	providerSpec, _ := spec["providerSpec"].(map[string]interface{})
	require.NotNil(t, providerSpec, "providerSpec should exist")
	assert.Equal(t, "AWSProviderSpec", providerSpec["kind"])
	assert.Equal(t, testSTSRoleARN, providerSpec["stsIAMRoleARN"],
		"stsIAMRoleARN should be set for manual mode")
}

func TestHelmRenderAWSManualDeploymentVolumes(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsManualValues())

	assert.Contains(t, rendered, "cloud-credentials",
		"manual mode should include cloud-credentials volume")
	assert.Contains(t, rendered, "bound-sa-token",
		"manual mode should include bound-sa-token volume")
	assert.Contains(t, rendered, "/var/run/secrets/cloud",
		"manual mode should mount cloud credentials")
	assert.Contains(t, rendered, "/var/run/secrets/openshift/serviceaccount",
		"manual mode should mount projected SA token")
}

func TestHelmRenderAWSManualEnvVars(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsManualValues())

	assert.Contains(t, rendered, "AWS_SHARED_CREDENTIALS_FILE",
		"manual mode should set AWS_SHARED_CREDENTIALS_FILE")
	assert.Contains(t, rendered, "AWS_WEB_IDENTITY_TOKEN_FILE",
		"manual mode should set AWS_WEB_IDENTITY_TOKEN_FILE")
	assert.Contains(t, rendered, "AWS_ROLE_ARN",
		"manual mode should set AWS_ROLE_ARN")
	assert.Contains(t, rendered, testSTSRoleARN,
		"AWS_ROLE_ARN should contain the STS role ARN value")
}

func TestHelmRenderManualModeNoS3Keys(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsManualValues())

	assert.NotContains(t, rendered, "TRUSTD_S3_ACCESS_KEY",
		"manual mode should NOT set TRUSTD_S3_ACCESS_KEY")
	assert.NotContains(t, rendered, "TRUSTD_S3_SECRET_KEY",
		"manual mode should NOT set TRUSTD_S3_SECRET_KEY")
	assert.NotContains(t, rendered, "aws_access_key_id",
		"manual mode should NOT reference aws_access_key_id secret key")
}

func TestHelmRenderMintModeHasS3Keys(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsValues())

	assert.Contains(t, rendered, "TRUSTD_S3_ACCESS_KEY",
		"mint mode should set TRUSTD_S3_ACCESS_KEY")
	assert.Contains(t, rendered, "TRUSTD_S3_SECRET_KEY",
		"mint mode should set TRUSTD_S3_SECRET_KEY")
	assert.Contains(t, rendered, "aws_access_key_id",
		"mint mode should reference aws_access_key_id from CCO secret")

	assert.NotContains(t, rendered, "AWS_WEB_IDENTITY_TOKEN_FILE",
		"mint mode should NOT set AWS_WEB_IDENTITY_TOKEN_FILE")
	assert.NotContains(t, rendered, "bound-sa-token",
		"mint mode should NOT include bound-sa-token volume")
}

func TestHelmRenderAWSPassthroughCredentialsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsPassthroughValues())
	docs := splitYAMLDocs(rendered)

	cr := findCredentialsRequest(t, docs)
	require.NotNil(t, cr, "CredentialsRequest should be rendered for passthrough mode")

	spec, _ := cr[fieldSpec].(map[string]interface{})
	require.NotNil(t, spec, "spec should exist")

	assert.Nil(t, spec["cloudTokenPath"],
		"passthrough mode should NOT set cloudTokenPath")

	providerSpec, _ := spec["providerSpec"].(map[string]interface{})
	require.NotNil(t, providerSpec, "providerSpec should exist")
	assert.Equal(t, "AWSProviderSpec", providerSpec["kind"])
	assert.Empty(t, providerSpec["stsIAMRoleARN"],
		"passthrough mode should NOT set stsIAMRoleARN")

	assert.Contains(t, rendered, "TRUSTD_S3_ACCESS_KEY",
		"passthrough mode should set TRUSTD_S3_ACCESS_KEY from CCO secret")
	assert.Contains(t, rendered, "aws_access_key_id",
		"passthrough mode should reference aws_access_key_id from CCO secret")
}

func TestHelmRenderAWSDefaultCredentialsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsDefaultValues())
	docs := splitYAMLDocs(rendered)

	cr := findCredentialsRequest(t, docs)
	require.NotNil(t, cr, "CredentialsRequest should be rendered for default mode")

	spec, _ := cr[fieldSpec].(map[string]interface{})
	require.NotNil(t, spec, "spec should exist")

	assert.Nil(t, spec["cloudTokenPath"],
		"default mode should NOT set cloudTokenPath")

	assert.Contains(t, rendered, "TRUSTD_S3_ACCESS_KEY",
		"default mode should set TRUSTD_S3_ACCESS_KEY from CCO secret")
}

func TestHelmRenderGCPManualCredentialsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, gcpManualValues())
	docs := splitYAMLDocs(rendered)

	cr := findCredentialsRequest(t, docs)
	require.NotNil(t, cr, "CredentialsRequest should be rendered for GCP manual mode")

	spec, _ := cr[fieldSpec].(map[string]interface{})
	require.NotNil(t, spec, "spec should exist")

	assert.Equal(t, "/var/run/secrets/openshift/serviceaccount/token",
		spec["cloudTokenPath"], "cloudTokenPath should be set for GCP manual mode")

	providerSpec, _ := spec["providerSpec"].(map[string]interface{})
	require.NotNil(t, providerSpec, "providerSpec should exist")
	assert.Equal(t, "GCPProviderSpec", providerSpec["kind"])
	assert.Equal(t, testGCPServiceAccountEmail, providerSpec["serviceAccountEmail"],
		"serviceAccountEmail should be set for GCP manual mode")

	roles, ok := providerSpec["predefinedRoles"].([]interface{})
	assert.True(t, ok, "predefinedRoles should be a list")
	assert.NotEmpty(t, roles, "predefinedRoles should not be empty")
}

func TestHelmRenderGCPManualDeploymentVolumes(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, gcpManualValues())

	assert.Contains(t, rendered, "cloud-credentials",
		"GCP manual mode should include cloud-credentials volume")
	assert.Contains(t, rendered, "bound-sa-token",
		"GCP manual mode should include bound-sa-token volume")
	assert.Contains(t, rendered, "/var/run/secrets/cloud",
		"GCP manual mode should mount cloud credentials")
	assert.Contains(t, rendered, "/var/run/secrets/openshift/serviceaccount",
		"GCP manual mode should mount projected SA token")
}

func TestHelmRenderGCPManualEnvVars(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, gcpManualValues())

	assert.Contains(t, rendered, "GOOGLE_APPLICATION_CREDENTIALS",
		"GCP manual mode should set GOOGLE_APPLICATION_CREDENTIALS")
	assert.Contains(t, rendered, "/var/run/secrets/cloud/service_account.json",
		"GOOGLE_APPLICATION_CREDENTIALS should point to the credentials file")

	assert.NotContains(t, rendered, "AWS_SHARED_CREDENTIALS_FILE",
		"GCP manual mode should NOT set AWS env vars")
	assert.NotContains(t, rendered, "AWS_WEB_IDENTITY_TOKEN_FILE",
		"GCP manual mode should NOT set AWS env vars")
}

func TestHelmRenderGCPManualModeNoS3Keys(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, gcpManualValues())

	assert.NotContains(t, rendered, "TRUSTD_S3_ACCESS_KEY",
		"GCP manual mode should NOT set TRUSTD_S3_ACCESS_KEY")
	assert.NotContains(t, rendered, "TRUSTD_S3_SECRET_KEY",
		"GCP manual mode should NOT set TRUSTD_S3_SECRET_KEY")
}

func TestHelmRenderAzureCredentialsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, azureValues())
	docs := splitYAMLDocs(rendered)

	cr := findCredentialsRequest(t, docs)
	require.NotNil(t, cr, "CredentialsRequest should be rendered for Azure")

	metadata, _ := cr[fieldMetadata].(map[string]interface{})
	require.NotNil(t, metadata, "metadata should exist")
	assert.Equal(t, ccoSecretName, metadata[fieldName], "CredentialsRequest name should match release")
	assert.Equal(t, ccoNamespace, metadata[fieldNamespace], "CredentialsRequest should be in CCO namespace")

	spec, _ := cr[fieldSpec].(map[string]interface{})
	require.NotNil(t, spec, "spec should exist")

	providerSpec, _ := spec["providerSpec"].(map[string]interface{})
	require.NotNil(t, providerSpec, "providerSpec should exist")
	assert.Equal(t, "AzureProviderSpec", providerSpec["kind"], "providerSpec should be AzureProviderSpec")
	assert.Equal(t, "cloudcredential.openshift.io/v1",
		providerSpec[fieldAPIVersion], "providerSpec apiVersion should match")

	bindings, ok := providerSpec["roleBindings"].([]interface{})
	assert.True(t, ok, "roleBindings should be a list")
	assert.NotEmpty(t, bindings, "roleBindings should not be empty")

	assert.Equal(t, testAzureSubscriptionID, providerSpec["azureSubscriptionID"],
		"azureSubscriptionID should be set")
	assert.Equal(t, testAzureRegion, providerSpec["azureRegion"],
		"azureRegion should be set")

	assert.NotContains(t, rendered, "AWSProviderSpec", "Azure config should not contain AWS provider")
	assert.NotContains(t, rendered, "GCPProviderSpec", "Azure config should not contain GCP provider")
}

func TestHelmRenderAzureManualCredentialsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, azureManualValues())
	docs := splitYAMLDocs(rendered)

	cr := findCredentialsRequest(t, docs)
	require.NotNil(t, cr, "CredentialsRequest should be rendered for Azure manual mode")

	spec, _ := cr[fieldSpec].(map[string]interface{})
	require.NotNil(t, spec, "spec should exist")

	assert.Equal(t, "/var/run/secrets/openshift/serviceaccount/token",
		spec["cloudTokenPath"], "cloudTokenPath should be set for Azure manual mode")

	providerSpec, _ := spec["providerSpec"].(map[string]interface{})
	require.NotNil(t, providerSpec, "providerSpec should exist")
	assert.Equal(t, "AzureProviderSpec", providerSpec["kind"])
	assert.Equal(t, testAzureClientID, providerSpec["azureClientID"],
		"azureClientID should be set for Azure manual mode")
	assert.Equal(t, testAzureTenantID, providerSpec["azureTenantID"],
		"azureTenantID should be set for Azure manual mode")
}

func TestHelmRenderAzureManualDeploymentVolumes(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, azureManualValues())

	assert.Contains(t, rendered, "cloud-credentials",
		"Azure manual mode should include cloud-credentials volume")
	assert.Contains(t, rendered, "bound-sa-token",
		"Azure manual mode should include bound-sa-token volume")
	assert.Contains(t, rendered, "/var/run/secrets/cloud",
		"Azure manual mode should mount cloud credentials")
	assert.Contains(t, rendered, "/var/run/secrets/openshift/serviceaccount",
		"Azure manual mode should mount projected SA token")
}

func TestHelmRenderAzureManualEnvVars(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, azureManualValues())

	assert.Contains(t, rendered, "AZURE_CLIENT_ID",
		"Azure manual mode should set AZURE_CLIENT_ID")
	assert.Contains(t, rendered, "AZURE_TENANT_ID",
		"Azure manual mode should set AZURE_TENANT_ID")
	assert.Contains(t, rendered, "AZURE_SUBSCRIPTION_ID",
		"Azure manual mode should set AZURE_SUBSCRIPTION_ID")
	assert.Contains(t, rendered, "AZURE_FEDERATED_TOKEN_FILE",
		"Azure manual mode should set AZURE_FEDERATED_TOKEN_FILE")

	assert.Contains(t, rendered, "azure_client_id",
		"AZURE_CLIENT_ID should reference CCO secret key")
	assert.Contains(t, rendered, "azure_tenant_id",
		"AZURE_TENANT_ID should reference CCO secret key")

	assert.NotContains(t, rendered, "AWS_SHARED_CREDENTIALS_FILE",
		"Azure manual mode should NOT set AWS env vars")
	assert.NotContains(t, rendered, "GOOGLE_APPLICATION_CREDENTIALS",
		"Azure manual mode should NOT set GCP env vars")
}

func TestHelmRenderAzureManualModeNoS3Keys(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, azureManualValues())

	assert.NotContains(t, rendered, "TRUSTD_S3_ACCESS_KEY",
		"Azure manual mode should NOT set TRUSTD_S3_ACCESS_KEY")
	assert.NotContains(t, rendered, "TRUSTD_S3_SECRET_KEY",
		"Azure manual mode should NOT set TRUSTD_S3_SECRET_KEY")
}
