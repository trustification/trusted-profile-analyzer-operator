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
	fieldCloudProvider    = "cloudProvider"
	fieldCloudCredentials = "cloudCredentials"
	fieldCcoMode          = "ccoMode"
	fieldCcoRds           = "ccoRds"
	kindCredentialsReq    = "CredentialsRequest"
	ccoSecretName         = "test-release-cloud-creds"
	ccoNamespace          = "openshift-cloud-credential-operator"
	testBucket            = "trustify-storage"
	testRegionUSEast1     = "us-east-1"
	testSTSRoleARN        = "arn:aws:iam::123456789012:role/trustify-s3-role"
	testRdsHost           = "testdb.cluster-xyz.us-east-1.rds.amazonaws.com"
	fieldUsername         = "username"
	fieldTokenImage       = "tokenImage"
	testTokenImage        = "quay.io/example/rhtpa-rhel10-operator:test"
	tokenInitContainer    = "rds-auth-token"
	tokenFilePath         = "/var/run/rds-auth-token/token"
	jobCreateDb           = "create-db"
	jobCreateImporters    = "create-importers"
	cloudProviderAWS      = "aws"
	cloudProviderGCP      = "gcp"
)

func testDatabaseValues() map[string]interface{} {
	return map[string]interface{}{
		fieldHost:     "postgres.test.svc",
		fieldName:     "testdb",
		fieldUsername: "testuser",
		fieldPassword: "testpass",
	}
}

func awsValues() map[string]interface{} {
	return map[string]interface{}{
		fieldAppDomain:     testAppDomain,
		fieldCloudProvider: cloudProviderAWS,
		fieldCloudCredentials: map[string]interface{}{
			cloudProviderAWS: map[string]interface{}{
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
		fieldCloudProvider: cloudProviderGCP,
		fieldCloudCredentials: map[string]interface{}{
			"gcp": map[string]interface{}{
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

func awsManualValues() map[string]interface{} {
	return map[string]interface{}{
		fieldAppDomain:     testAppDomain,
		fieldCloudProvider: cloudProviderAWS,
		fieldCcoMode:       "manual",
		fieldCloudCredentials: map[string]interface{}{
			cloudProviderAWS: map[string]interface{}{
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

func assertServerDeploymentCCOEnvVars(
	t *testing.T, docs []string,
	accessKeyEnv, secretKeyEnv, accessKeyMsg, secretKeyMsg string,
) {
	t.Helper()
	for _, doc := range docs {
		obj, err := parseYAMLDoc(doc)
		if err != nil {
			continue
		}
		if obj["kind"] != kindDeploymentResource {
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
			if envName == accessKeyEnv {
				refName := nestedString(env, "valueFrom", "secretKeyRef", fieldName)
				refKey := nestedString(env, "valueFrom", "secretKeyRef", "key")
				assert.Equal(t, ccoSecretName, refName, accessKeyEnv+" should reference CCO secret")
				assert.Equal(t, "aws_access_key_id", refKey, accessKeyEnv+" should use correct key")
				accessKeyFound = true
			}
			if envName == secretKeyEnv {
				refName := nestedString(env, "valueFrom", "secretKeyRef", fieldName)
				refKey := nestedString(env, "valueFrom", "secretKeyRef", "key")
				assert.Equal(t, ccoSecretName, refName, secretKeyEnv+" should reference CCO secret")
				assert.Equal(t, "aws_secret_access_key", refKey, secretKeyEnv+" should use correct key")
				secretKeyFound = true
			}
		}
		assert.True(t, accessKeyFound, accessKeyMsg)
		assert.True(t, secretKeyFound, secretKeyMsg)
		return
	}
	t.Fatal("server deployment not found in rendered output")
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
	assertServerDeploymentCCOEnvVars(t, docs,
		"TRUSTD_S3_ACCESS_KEY", "TRUSTD_S3_SECRET_KEY",
		"TRUSTD_S3_ACCESS_KEY env var should be present",
		"TRUSTD_S3_SECRET_KEY env var should be present",
	)
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
	// The AWS SDK resolves the STS endpoint for AssumeRoleWithWebIdentity from the
	// credential chain's own region, which TRUSTD_S3_REGION does not feed.
	assert.Contains(t, rendered, "AWS_REGION",
		"manual mode should set AWS_REGION so STS can resolve its endpoint")
}

func TestHelmRenderMintModeNoAWSRegion(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsValues())

	assert.NotContains(t, rendered, "AWS_REGION",
		"mint mode uses static keys and should NOT set AWS_REGION")
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

func rdsIamDatabaseValues() map[string]interface{} {
	return map[string]interface{}{
		fieldHost:     testRdsHost,
		fieldName:     "testdb",
		fieldUsername: "testuser",
	}
}

// ccoRdsValues is the ccoRds block used by the RDS IAM tests. tokenImage is
// always set because the psql-based jobs refuse to render without it; in a
// cluster the operator injects it from RELATED_IMAGE_RDS_AUTH_TOKEN.
func ccoRdsValues() map[string]interface{} {
	return map[string]interface{}{
		fieldEnabled:    true,
		fieldRegion:     testRegionUSEast1,
		fieldTokenImage: testTokenImage,
	}
}

func awsRdsValues() map[string]interface{} {
	v := awsValues()
	v[fieldCcoRds] = ccoRdsValues()
	v[fieldDatabase] = rdsIamDatabaseValues()
	return v
}

func awsManualRdsValues() map[string]interface{} {
	v := awsManualValues()
	v[fieldCcoRds] = ccoRdsValues()
	v[fieldDatabase] = rdsIamDatabaseValues()
	return v
}

func TestHelmRenderRdsIamEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsRdsValues())

	assert.Contains(t, rendered, "TRUSTD_DB_IAM_AUTH",
		"RDS IAM mode should set TRUSTD_DB_IAM_AUTH")
	assert.Contains(t, rendered, "TRUSTD_DB_IAM_REGION",
		"RDS IAM mode should set TRUSTD_DB_IAM_REGION")
	assert.NotContains(t, rendered, "TRUSTD_DB_PASSWORD",
		"RDS IAM mode should NOT set TRUSTD_DB_PASSWORD")
}

func TestHelmRenderRdsIamDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsValues())

	assert.NotContains(t, rendered, "TRUSTD_DB_IAM_AUTH",
		"without ccoRds, TRUSTD_DB_IAM_AUTH should NOT be set")
	assert.NotContains(t, rendered, "TRUSTD_DB_IAM_REGION",
		"without ccoRds, TRUSTD_DB_IAM_REGION should NOT be set")
	assert.Contains(t, rendered, "TRUSTD_DB_PASSWORD",
		"without ccoRds, TRUSTD_DB_PASSWORD should be set")
}

func TestHelmRenderRdsIamNonManualAWSCredentials(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsRdsValues())
	docs := splitYAMLDocs(rendered)

	assertServerDeploymentCCOEnvVars(t, docs,
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
		"AWS_ACCESS_KEY_ID env var should be present for RDS IAM in mint mode",
		"AWS_SECRET_ACCESS_KEY env var should be present for RDS IAM in mint mode",
	)
}

func TestHelmRenderRdsIamManualMode(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsManualRdsValues())

	assert.Contains(t, rendered, "TRUSTD_DB_IAM_AUTH",
		"manual RDS IAM mode should set TRUSTD_DB_IAM_AUTH")
	assert.Contains(t, rendered, "TRUSTD_DB_IAM_REGION",
		"manual RDS IAM mode should set TRUSTD_DB_IAM_REGION")
	assert.NotContains(t, rendered, "TRUSTD_DB_PASSWORD",
		"manual RDS IAM mode should NOT set TRUSTD_DB_PASSWORD")

	assert.Contains(t, rendered, "AWS_WEB_IDENTITY_TOKEN_FILE",
		"manual mode should still set AWS_WEB_IDENTITY_TOKEN_FILE")
	assert.Contains(t, rendered, "AWS_ROLE_ARN",
		"manual mode should still set AWS_ROLE_ARN")
}

func TestHelmRenderRdsIamSSLMode(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsRdsValues())
	docs := splitYAMLDocs(rendered)

	for _, doc := range docs {
		obj, err := parseYAMLDoc(doc)
		if err != nil {
			continue
		}
		if obj["kind"] != kindDeploymentResource {
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

		for _, e := range envList {
			env, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			envName, _ := env[fieldName].(string)
			if envName == "TRUSTD_DB_SSLMODE" {
				envValue, _ := env["value"].(string)
				assert.Equal(t, "require", envValue,
					"RDS IAM mode should force TRUSTD_DB_SSLMODE to require")
				return
			}
		}
		t.Fatal("TRUSTD_DB_SSLMODE not found in server deployment env vars")
	}
	t.Fatal("server deployment not found in rendered output")
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

// enableMigrateDatabase turns on the migrate-database Job for the given values.
// The Job template requires both modules.migrateDatabase.enabled and a
// top-level .migrateDatabase value.
func enableMigrateDatabase(v map[string]interface{}) map[string]interface{} {
	v[fieldModules] = map[string]interface{}{
		"migrateDatabase": map[string]interface{}{fieldEnabled: true},
	}
	v["migrateDatabase"] = map[string]interface{}{}
	return v
}

// firstContainerEnv returns the env list of the first container of the first
// resource in docs whose kind matches and whose metadata.name contains
// nameContains. It fails the test if the resource or its containers are absent.
func firstContainerEnv(t *testing.T, docs []string, kind, nameContains string) []interface{} {
	t.Helper()
	for _, doc := range docs {
		obj, err := parseYAMLDoc(doc)
		if err != nil {
			continue
		}
		if obj["kind"] != kind {
			continue
		}
		metadata, _ := obj[fieldMetadata].(map[string]interface{})
		name, _ := metadata[fieldName].(string)
		if metadata == nil || !strings.Contains(name, nameContains) {
			continue
		}

		spec, _ := obj[fieldSpec].(map[string]interface{})
		template, _ := spec["template"].(map[string]interface{})
		templateSpec, _ := template[fieldSpec].(map[string]interface{})
		containers, _ := templateSpec["containers"].([]interface{})
		require.NotEmpty(t, containers, "%s/%s should have containers", kind, nameContains)

		container, _ := containers[0].(map[string]interface{})
		envList, _ := container["env"].([]interface{})
		require.NotEmpty(t, envList, "%s/%s container should have env vars", kind, nameContains)
		return envList
	}
	t.Fatalf("%s with name containing %q not found in rendered output", kind, nameContains)
	return nil
}

// envValue looks up the literal `value` of the named env var. The second
// return reports whether an env var with that name was found at all.
func envValue(envList []interface{}, name string) (string, bool) {
	for _, e := range envList {
		env, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if n, _ := env[fieldName].(string); n == name {
			v, _ := env["value"].(string)
			return v, true
		}
	}
	return "", false
}

// TestHelmRenderMigrateDbJobRdsIam guards the migrate-database Job — the init
// job that runs `trustd db migrate` and is the only one supporting RDS IAM
// auth. A committed merge conflict in this template previously dropped either
// the storage or the CCO includes; this test asserts both are present.
func TestHelmRenderMigrateDbJobRdsIam(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, enableMigrateDatabase(awsRdsValues()))
	docs := splitYAMLDocs(rendered)

	env := firstContainerEnv(t, docs, "Job", "migrate-db")

	// Storage env vars must still be emitted alongside the CCO/RDS ones.
	if _, ok := envValue(env, "TRUSTD_STORAGE_STRATEGY"); !ok {
		t.Error("migrate-db job should include storage env vars")
	}

	// RDS IAM env vars.
	iamAuth, ok := envValue(env, "TRUSTD_DB_IAM_AUTH")
	assert.True(t, ok, "migrate-db job should set TRUSTD_DB_IAM_AUTH for RDS IAM")
	assert.Equal(t, "true", iamAuth)

	region, ok := envValue(env, "TRUSTD_DB_IAM_REGION")
	assert.True(t, ok, "migrate-db job should set TRUSTD_DB_IAM_REGION for RDS IAM")
	assert.Equal(t, testRegionUSEast1, region)

	sslMode, ok := envValue(env, "TRUSTD_DB_SSLMODE")
	assert.True(t, ok, "migrate-db job should set TRUSTD_DB_SSLMODE")
	assert.Equal(t, "require", sslMode, "RDS IAM should force SSL mode to require")

	// AWS credentials are injected for non-manual modes so the SDK can mint
	// RDS auth tokens.
	if _, ok := envValue(env, "AWS_ACCESS_KEY_ID"); !ok {
		t.Error("migrate-db job should reference AWS_ACCESS_KEY_ID from the CCO secret in mint mode")
	}

	// Password must be omitted when RDS IAM auth is enabled.
	if _, ok := envValue(env, "TRUSTD_DB_PASSWORD"); ok {
		t.Error("migrate-db job should NOT set TRUSTD_DB_PASSWORD when RDS IAM is enabled")
	}
}

// TestHelmRenderMigrateDbJobManualVolumes asserts the migrate-database Job gets
// the CCO manual-mode volumes/mounts in addition to storage. This is the other
// half of the merge conflict that was previously mis-resolved.
func TestHelmRenderMigrateDbJobManualVolumes(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, enableMigrateDatabase(awsManualRdsValues()))
	docs := splitYAMLDocs(rendered)

	// Locate the migrate-db Job and inspect its pod spec directly.
	var found bool
	for _, doc := range docs {
		obj, err := parseYAMLDoc(doc)
		if err != nil || obj["kind"] != "Job" {
			continue
		}
		metadata, _ := obj[fieldMetadata].(map[string]interface{})
		name, _ := metadata[fieldName].(string)
		if !strings.Contains(name, "migrate-db") {
			continue
		}
		found = true

		spec, _ := obj[fieldSpec].(map[string]interface{})
		template, _ := spec["template"].(map[string]interface{})
		templateSpec, _ := template[fieldSpec].(map[string]interface{})
		volumes, _ := templateSpec["volumes"].([]interface{})

		names := map[string]bool{}
		for _, v := range volumes {
			vol, _ := v.(map[string]interface{})
			if n, _ := vol[fieldName].(string); n != "" {
				names[n] = true
			}
		}
		assert.True(t, names["cloud-credentials"],
			"migrate-db job should include cloud-credentials volume in manual mode")
		assert.True(t, names["bound-sa-token"],
			"migrate-db job should include bound-sa-token volume in manual mode")

		// Manual mode omits static S3/DB keys; STS env vars are used instead.
		env := firstContainerEnv(t, docs, "Job", "migrate-db")
		if _, ok := envValue(env, "AWS_WEB_IDENTITY_TOKEN_FILE"); !ok {
			t.Error("migrate-db job should set AWS_WEB_IDENTITY_TOKEN_FILE in manual mode")
		}
		break
	}
	require.True(t, found, "migrate-db job should be rendered")
}

// enablePsqlJobs turns on the two init jobs that shell out to psql. The
// create-database Job additionally requires a top-level `createDatabase`
// block holding the bootstrap (admin) connection settings.
func enablePsqlJobs(v map[string]interface{}, createDatabase map[string]interface{}) map[string]interface{} {
	modules, _ := v[fieldModules].(map[string]interface{})
	if modules == nil {
		modules = map[string]interface{}{}
		v[fieldModules] = modules
	}
	modules["createDatabase"] = map[string]interface{}{fieldEnabled: true}
	modules["createImporters"] = map[string]interface{}{fieldEnabled: true}
	v["createDatabase"] = createDatabase
	return v
}

// jobPodSpec returns the pod spec of the first Job whose name contains
// nameContains.
func jobPodSpec(t *testing.T, docs []string, nameContains string) map[string]interface{} {
	t.Helper()
	for _, doc := range docs {
		obj, err := parseYAMLDoc(doc)
		if err != nil || obj["kind"] != "Job" {
			continue
		}
		metadata, _ := obj[fieldMetadata].(map[string]interface{})
		if name, _ := metadata[fieldName].(string); !strings.Contains(name, nameContains) {
			continue
		}
		spec, _ := obj[fieldSpec].(map[string]interface{})
		template, _ := spec["template"].(map[string]interface{})
		podSpec, _ := template[fieldSpec].(map[string]interface{})
		require.NotNil(t, podSpec, "job %q should have a pod spec", nameContains)
		return podSpec
	}
	t.Fatalf("Job with name containing %q not found in rendered output", nameContains)
	return nil
}

// containerByName returns the named entry of the pod spec's `containers` or
// `initContainers` list, or nil when absent.
func containerByName(podSpec map[string]interface{}, list, name string) map[string]interface{} {
	entries, _ := podSpec[list].([]interface{})
	for _, e := range entries {
		container, _ := e.(map[string]interface{})
		if n, _ := container[fieldName].(string); n == name {
			return container
		}
	}
	return nil
}

// volumeNames returns the set of volume names on the pod spec.
func volumeNames(podSpec map[string]interface{}) map[string]bool {
	names := map[string]bool{}
	volumes, _ := podSpec["volumes"].([]interface{})
	for _, v := range volumes {
		vol, _ := v.(map[string]interface{})
		if n, _ := vol[fieldName].(string); n != "" {
			names[n] = true
		}
	}
	return names
}

// containerEnv returns the env list of a container map.
func containerEnv(container map[string]interface{}) []interface{} {
	env, _ := container["env"].([]interface{})
	return env
}

// containerScript joins a container's command into a single string so it can
// be searched for shell fragments.
func containerScript(container map[string]interface{}) string {
	parts, _ := container["command"].([]interface{})
	var sb strings.Builder
	for _, p := range parts {
		if s, ok := p.(string); ok {
			sb.WriteString(s)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// mountPaths returns the set of mount paths of a container.
func mountPaths(container map[string]interface{}) map[string]bool {
	paths := map[string]bool{}
	mounts, _ := container["volumeMounts"].([]interface{})
	for _, m := range mounts {
		mount, _ := m.(map[string]interface{})
		if p, _ := mount["mountPath"].(string); p != "" {
			paths[p] = true
		}
	}
	return paths
}

// assertMintsRdsToken checks the shape shared by both psql jobs when the
// connection they make uses RDS IAM auth: an `rds-auth-token` init container
// mints the token onto a shared volume, and the job itself reads that file
// into PGPASSWORD instead of receiving a static password.
func assertMintsRdsToken(t *testing.T, podSpec map[string]interface{}, jobName string) {
	t.Helper()

	init := containerByName(podSpec, "initContainers", tokenInitContainer)
	require.NotNil(t, init, "%s should have an %s init container", jobName, tokenInitContainer)

	image, _ := init["image"].(string)
	assert.Equal(t, testTokenImage, image,
		"%s init container should run the operator image carrying the helper", jobName)

	initEnv := containerEnv(init)
	region, ok := envValue(initEnv, "AWS_REGION")
	assert.True(t, ok, "%s init container should set AWS_REGION", jobName)
	assert.Equal(t, testRegionUSEast1, region)

	host, ok := envValue(initEnv, "PGHOST")
	assert.True(t, ok, "%s init container should set PGHOST", jobName)
	assert.Equal(t, testRdsHost, host, "the token is bound to the host it is minted for")

	// The token is bound to host+port+user, so the init container must mint it
	// for the very same user the job connects as.
	job := containerByName(podSpec, "containers", "job")
	require.NotNil(t, job, "%s should have a `job` container", jobName)
	initUser, _ := envValue(initEnv, "PGUSER")
	jobUser, _ := envValue(containerEnv(job), "PGUSER")
	assert.Equal(t, jobUser, initUser,
		"%s init container must mint the token for the user the job connects as", jobName)

	assert.True(t, volumeNames(podSpec)[tokenInitContainer],
		"%s should carry the %s volume", jobName, tokenInitContainer)
	assert.True(t, mountPaths(init)["/var/run/rds-auth-token"],
		"%s init container should mount the token volume", jobName)
	assert.True(t, mountPaths(job)["/var/run/rds-auth-token"],
		"%s job container should mount the token volume", jobName)

	assert.Contains(t, containerScript(job), tokenFilePath,
		"%s should read the minted token into PGPASSWORD", jobName)
	if _, ok := envValue(containerEnv(job), "PGPASSWORD"); ok {
		t.Errorf("%s should NOT set a static PGPASSWORD when using RDS IAM auth", jobName)
	}
}

// TestHelmRenderPsqlJobsRdsIamInitContainer covers the core of the feature:
// psql cannot mint an RDS IAM token itself, so both psql-based init jobs get
// an init container that does it for them.
func TestHelmRenderPsqlJobsRdsIamInitContainer(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	values := enablePsqlJobs(awsManualRdsValues(), map[string]interface{}{})
	docs := splitYAMLDocs(renderHelmChart(t, chartPath, values))

	for _, jobName := range []string{jobCreateDb, jobCreateImporters} {
		podSpec := jobPodSpec(t, docs, jobName)
		assertMintsRdsToken(t, podSpec, jobName)

		// Manual mode: the init container needs the STS volumes to assume the
		// role before it can sign the token request.
		names := volumeNames(podSpec)
		assert.True(t, names["cloud-credentials"],
			"%s should include the cloud-credentials volume in manual mode", jobName)
		assert.True(t, names["bound-sa-token"],
			"%s should include the bound-sa-token volume in manual mode", jobName)

		init := containerByName(podSpec, "initContainers", tokenInitContainer)
		if _, ok := envValue(containerEnv(init), "AWS_WEB_IDENTITY_TOKEN_FILE"); !ok {
			t.Errorf("%s init container should set AWS_WEB_IDENTITY_TOKEN_FILE in manual mode", jobName)
		}
	}
}

// TestHelmRenderPsqlJobsRdsIamMintMode asserts the init container gets static
// CCO credentials — rather than STS volumes — in the non-manual modes.
func TestHelmRenderPsqlJobsRdsIamMintMode(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	values := enablePsqlJobs(awsRdsValues(), map[string]interface{}{})
	docs := splitYAMLDocs(renderHelmChart(t, chartPath, values))

	podSpec := jobPodSpec(t, docs, jobCreateImporters)
	assertMintsRdsToken(t, podSpec, jobCreateImporters)

	init := containerByName(podSpec, "initContainers", tokenInitContainer)
	if _, ok := envValue(containerEnv(init), "AWS_ACCESS_KEY_ID"); !ok {
		t.Error("create-importers init container should reference AWS_ACCESS_KEY_ID from the CCO secret in mint mode")
	}
	assert.False(t, volumeNames(podSpec)["bound-sa-token"],
		"mint mode should not mount the projected SA token")
}

// TestHelmRenderRdsTokenImageSources covers the two ways the helper image is
// supplied: `defaultTokenImage`, which the operator injects from the optional
// RELATED_IMAGE_RDS_AUTH_TOKEN env var, and `tokenImage`, which the user sets
// and which must win so a deployment can pin a different image.
func TestHelmRenderRdsTokenImageSources(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	const pinnedImage = "quay.io/example/rhtpa-rhel10-operator:pinned"

	tests := []struct {
		name     string
		ccoRds   map[string]interface{}
		expected string
	}{
		{
			name: "operator injected only",
			ccoRds: map[string]interface{}{
				fieldEnabled:        true,
				fieldRegion:         testRegionUSEast1,
				"defaultTokenImage": testTokenImage,
			},
			expected: testTokenImage,
		},
		{
			name: "user value wins over operator injected",
			ccoRds: map[string]interface{}{
				fieldEnabled:        true,
				fieldRegion:         testRegionUSEast1,
				"defaultTokenImage": testTokenImage,
				fieldTokenImage:     pinnedImage,
			},
			expected: pinnedImage,
		},
		{
			name: "user value only - operator env var absent",
			ccoRds: map[string]interface{}{
				fieldEnabled:    true,
				fieldRegion:     testRegionUSEast1,
				fieldTokenImage: pinnedImage,
			},
			expected: pinnedImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := awsValues()
			values[fieldCcoRds] = tt.ccoRds
			values[fieldDatabase] = rdsIamDatabaseValues()
			docs := splitYAMLDocs(renderHelmChart(t, chartPath, enablePsqlJobs(values, map[string]interface{}{})))

			init := containerByName(jobPodSpec(t, docs, jobCreateImporters), "initContainers", tokenInitContainer)
			require.NotNil(t, init, "create-importers should have an %s init container", tokenInitContainer)
			image, _ := init["image"].(string)
			assert.Equal(t, tt.expected, image)
		})
	}
}

// TestHelmRenderCreateDbAdminIamOptOut covers the common bootstrap case: the
// admin connection create-database makes is the RDS master user on password
// auth, while the application user it creates gets rds_iam. The two identities
// must be able to differ.
func TestHelmRenderCreateDbAdminIamOptOut(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	values := enablePsqlJobs(awsRdsValues(), map[string]interface{}{
		fieldUsername: "postgres",
		fieldPassword: "adminpass",
		"iamAuth":     false,
	})
	docs := splitYAMLDocs(renderHelmChart(t, chartPath, values))

	podSpec := jobPodSpec(t, docs, jobCreateDb)
	assert.Nil(t, containerByName(podSpec, "initContainers", tokenInitContainer),
		"an admin connection opted out of IAM auth needs no token init container")

	job := containerByName(podSpec, "containers", "job")
	require.NotNil(t, job, "create-db should have a `job` container")
	if _, ok := envValue(containerEnv(job), "PGPASSWORD"); !ok {
		t.Error("create-db should keep PGPASSWORD for an admin connection opted out of IAM auth")
	}
	command, _ := job["command"].([]interface{})
	require.NotEmpty(t, command, "create-db should have a command")
	assert.Equal(t, "psql", command[0],
		"a connection opted out of IAM auth keeps the plain psql invocation")

	// The application user is still on IAM auth, so it has no password to set.
	if _, ok := envValue(containerEnv(job), "DB_PASSWORD"); ok {
		t.Error("create-db should not pass DB_PASSWORD for an application user on IAM auth")
	}

	// The opt-out is per connection: create-importers connects as the
	// application user and must still mint a token.
	assertMintsRdsToken(t, jobPodSpec(t, docs, jobCreateImporters), jobCreateImporters)
}

// assertPlainPsqlJob checks that a job is left exactly as deployments without
// RDS IAM auth have always had it: psql invoked directly, no shell wrapper, no
// init container, no token volume.
func assertPlainPsqlJob(t *testing.T, podSpec map[string]interface{}, jobName string) {
	t.Helper()

	assert.Nil(t, podSpec["initContainers"],
		"%s should have no init containers without RDS IAM auth", jobName)
	assert.False(t, volumeNames(podSpec)[tokenInitContainer],
		"%s should have no token volume without RDS IAM auth", jobName)

	job := containerByName(podSpec, "containers", "job")
	require.NotNil(t, job, "%s should have a `job` container", jobName)

	command, _ := job["command"].([]interface{})
	require.NotEmpty(t, command, "%s should have a command", jobName)
	assert.Equal(t, "psql", command[0],
		"%s should exec psql directly, not through a shell, without RDS IAM auth", jobName)

	if _, ok := envValue(containerEnv(job), "PGPASSWORD"); !ok {
		t.Errorf("%s should set PGPASSWORD without RDS IAM auth", jobName)
	}
	assert.NotContains(t, containerScript(job), tokenFilePath,
		"%s should not read a token file without RDS IAM auth", jobName)
}

// TestHelmRenderPsqlJobsWithoutRdsIam asserts the jobs are untouched when RDS
// IAM auth is off — the whole mechanism is opt-in.
func TestHelmRenderPsqlJobsWithoutRdsIam(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	values := enablePsqlJobs(awsValues(), map[string]interface{}{})
	docs := splitYAMLDocs(renderHelmChart(t, chartPath, values))

	for _, jobName := range []string{jobCreateDb, jobCreateImporters} {
		assertPlainPsqlJob(t, jobPodSpec(t, docs, jobName), jobName)
	}

	// The importer SQL keeps being passed as a psql argument rather than
	// through an env-var.
	importers := containerByName(jobPodSpec(t, docs, jobCreateImporters), "containers", "job")
	if _, ok := envValue(containerEnv(importers), "IMPORTERS_SQL"); ok {
		t.Error("create-importers should not use IMPORTERS_SQL without RDS IAM auth")
	}
	assert.Contains(t, containerScript(importers), "INSERT INTO IMPORTER",
		"create-importers should pass the SQL to psql directly without RDS IAM auth")
}

// TestHelmRenderPsqlJobsWithoutCloudProvider is the plain non-cloud case: no
// CCO at all, an in-cluster database, static passwords.
func TestHelmRenderPsqlJobsWithoutCloudProvider(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	values := enablePsqlJobs(map[string]interface{}{
		fieldAppDomain: testAppDomain,
		fieldStorage:   map[string]interface{}{fieldType: "filesystem", "size": "32Gi"},
		fieldDatabase:  testDatabaseValues(),
	}, map[string]interface{}{})
	docs := splitYAMLDocs(renderHelmChart(t, chartPath, values))

	for _, jobName := range []string{jobCreateDb, jobCreateImporters} {
		assertPlainPsqlJob(t, jobPodSpec(t, docs, jobName), jobName)
	}
}

// createDbInitSQL returns the init.sql the create-database Job runs.
func createDbInitSQL(t *testing.T, docs []string) string {
	t.Helper()
	for _, doc := range docs {
		obj, err := parseYAMLDoc(doc)
		if err != nil || obj["kind"] != "ConfigMap" {
			continue
		}
		metadata, _ := obj[fieldMetadata].(map[string]interface{})
		if name, _ := metadata[fieldName].(string); !strings.Contains(name, jobCreateDb) {
			continue
		}
		data, _ := obj["data"].(map[string]interface{})
		sql, _ := data["init.sql"].(string)
		require.NotEmpty(t, sql, "create-db ConfigMap should hold init.sql")
		return sql
	}
	t.Fatal("create-db ConfigMap not found in rendered output")
	return ""
}

// TestHelmRenderCreateDbInitSQLGrantsRdsIam asserts the created role is set up
// for the auth method it will actually use. Granting rds_iam is what enables
// token login — and it disables password login, so the two branches are
// mutually exclusive.
func TestHelmRenderCreateDbInitSQLGrantsRdsIam(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)

	withIam := enablePsqlJobs(awsRdsValues(), map[string]interface{}{})
	sql := createDbInitSQL(t, splitYAMLDocs(renderHelmChart(t, chartPath, withIam)))
	assert.Contains(t, sql, "GRANT rds_iam TO :db_user",
		"an application user on IAM auth should be granted rds_iam")
	assert.NotContains(t, sql, "ALTER USER :db_user WITH PASSWORD",
		"an application user on IAM auth has no password to set")

	withoutIam := enablePsqlJobs(awsValues(), map[string]interface{}{})
	sql = createDbInitSQL(t, splitYAMLDocs(renderHelmChart(t, chartPath, withoutIam)))
	assert.Contains(t, sql, "ALTER USER :db_user WITH PASSWORD",
		"without IAM auth the application user keeps a password")
	assert.NotContains(t, sql, "GRANT rds_iam",
		"rds_iam must not be granted when the user logs in with a password")
}
