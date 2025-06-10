# Build the manager binary
FROM quay.io/operator-framework/helm-operator:v1.40.0

LABEL com.redhat.component="rhtpa-operator"
LABEL description="Red Hat Trusted Profile Analyzer Operator image"
LABEL io.k8s.description="Red Hat Trusted Profile Analyzer Operator image"
LABEL io.k8s.display-name="RHTPA operator container image for Red Hat Trusted Profile Analyzer"
LABEL io.openshift.tags="RHTPA, rhtpa-operator, Red Hat Trusted Profile Analyzer"
LABEL name="rhtpa-operator"
LABEL org.opencontainers.image.source="https://github.com/trustification/trusted-profile-analyzer-operator"
LABEL summary="RHTPA Operator"
LABEL release=1.0.0
LABEL maintainer="Red Hat"

LABEL features.operators.openshift.io/cni="false"
LABEL features.operators.openshift.io/disconnected="false"
LABEL features.operators.openshift.io/fips-compliant="false"
LABEL features.operators.openshift.io/proxy-aware="false"
LABEL features.operators.openshift.io/cnf="false"
LABEL features.operators.openshift.io/csi="false"
LABEL features.operators.openshift.io/tls-profiles="false"
LABEL features.operators.openshift.io/token-auth-aws="false"
LABEL features.operators.openshift.io/token-auth-azure="false"
LABEL features.operators.openshift.io/token-auth-gcp="false"

ENV HOME=/opt/helm
COPY watches.yaml ${HOME}/watches.yaml
COPY helm-charts  ${HOME}/helm-charts
WORKDIR ${HOME}
