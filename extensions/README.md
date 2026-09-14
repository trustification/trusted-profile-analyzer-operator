# Tokenize with Cloud Credentials Operator

Cloud Credentials Operator (CCO) is installed by default on OCP.
To check the CCO status 

```console 
oc get clusteroperator cloud-credential
```
it shows something like

```console
NAME               VERSION   AVAILABLE   PROGRESSING   DEGRADED   SINCE
cloud-credential   4.x.x     True        False         False      ...
```
Pod status
```console 
oc get pods -n openshift-cloud-credential-operator
```

Credential requests checks
```console 
oc get credentialsrequests -n openshift-cloud-credential-operator
```

CCO details
```console 
oc describe clusteroperator cloud-credential
```

CCO Modality
```console 
oc get cloudcredential cluster -o yaml
```
On spec.credentialsMode will be the configured setting (Mint,
Passthrough, Manual, or empty for default).

## How the Operator interacts with the CCO

1. Operator declares permissions needed in a CredentialsRequest CR in the namespace openshift-cloud-credential-operator

```console 
  apiVersion: cloudcredential.openshift.io/v1
  kind: CredentialsRequest
  metadata:
    name: my-operator-credentials
    namespace: openshift-cloud-credential-operator
  spec:
    secretRef:
      name: my-cloud-creds
      namespace: my-operator-namespace
    providerSpec:
      apiVersion: cloudcredential.openshift.io/v1
      kind: AWSProviderSpec   # esempio per AWS
      statementEntries:
        - effect: Allow
          action:
            - "s3:GetObject"
            - "s3:PutObject"
          resource: "*"
```

2. CCO processes the CR and create a  Kubernetes Secret with the cloud credentails 
   in the  namespace specified in spec.secretRef.

3. Operator reads the Secret and uses the credentials to interact with the cloud API. 
   The Operator must tolerate the non-immediate availability of the Secret because it takes time to create.

## How to integrate the CCO with the Helm Chart Operator

1. Define Credential Request in the chart
2. 
2. Configure Deployment to use the secret created by the CCO

3. Handling the delay of the creation of the secret
   first approach: init container
```console
 initContainers:
    - name: wait-for-creds
      image: registry.redhat.io/openshift4/ose-cli
      command:
        - /bin/bash
        - -c
        - |
          until oc get secret {{ .Release.Name }}-cloud-creds -n {{ 
  .Release.Namespace }} 2>/dev/null; do
            echo "Waiting for cloud credentials..."
            sleep 5
          done
```
   second approach Retry in the Operator's code, but this is available on a full Go Operator

4. Support different cloud providers with values configurations

5. RBAC needed to create the CredentialsRequest
6. Support Manual Mode (STS/WIF): if the cluster uses STS mode, CCO doesn’t create the secret automatically.  
   In STS mode the user must :
   1. Extract CredentialsRequest from chart
   2. Use ccoctl tool to generate the credentials
   3. Create the secrets manually before installing the chart.

