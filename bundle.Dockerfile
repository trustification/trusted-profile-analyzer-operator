FROM scratch

LABEL com.redhat.component="rhtpa-operator-bundle"
LABEL description="The bundle image for the rhtas-operator, containing manifests, metadata and testing scorecard."
LABEL io.k8s.description="The bundle image for the rhtas-operator, containing manifests, metadata and testing scorecard."
LABEL io.k8s.display-name="RHTPA operator bundle container image for Red Hat Trusted Profile Analyzer."
LABEL io.openshift.tags="rhtpa-operator-bundle, Red Hat, rhtpa-operator, rhtpa, Red Hat Trusted Profile Analyzer."
LABEL name="rhtpa/rhtpa-operator-bundle"
LABEL org.opencontainers.image.source="https://github.com/trustification/trusted-profile-analyzer-operator"
LABEL summary="Operator Bundle for the rhtpa-operator."
LABEL maintainer="Red Hat"
LABEL vendor="Red Hat, Inc."
LABEL distribution-scope="public"
LABEL url="https://www.redhat.com"
LABEL version="1.0.3"
LABEL release=1.0.3
LABEL cpe="cpe:/a:redhat:trusted_profile_analyzer:2.1::el9"
LABEL org.opencontainers.image.created="${SOURCE_DATE_EPOCH}"

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
# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=rhtpa-operator
LABEL operators.operatorframework.io.bundle.channels.v1=stable,stable-v1.0
LABEL operators.operatorframework.io.bundle.channel.default.v1=stable
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.41.1
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=helm.sdk.operatorframework.io/v1
LABEL operators.openshift.io/valid-subscription="Red Hat Trusted Profile Analyzer"

# Labels for testing.
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/

# Copy files to locations specified by labels.
COPY bundle/manifests /manifests/
COPY bundle/metadata /metadata/
COPY bundle/tests/scorecard /tests/scorecard/
COPY LICENSE /licenses/license.txt
