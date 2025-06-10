# Build the manager binary
FROM quay.io/operator-framework/helm-operator:v1.39.2

LABEL com.redhat.component ="RHTPA"
LABEL description ="Red Hat Trusted Profile Analyzer"
LABEL io.k8s.description ="Red Hat Trusted Profile Analyzer"
LABEL io.k8s.display-name ="RHTPA Operator"
LABEL io.openshift.tags ="RHTPA"
LABEL name ="RHTPA Operator"
LABEL org.opencontainers.image.source="https://github.com/trustification/trusted-profile-analyzer-operator"
LABEL summary ="RHTPA Operator"

ENV HOME=/opt/helm
COPY watches.yaml ${HOME}/watches.yaml
COPY helm-charts  ${HOME}/helm-charts
WORKDIR ${HOME}
