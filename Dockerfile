# Build the manager binary
FROM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
#TODO uncomment after adding golang api
#COPY api/ api/
#COPY controllers/ controllers/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
#FROM registry.access.redhat.com/ubi9/ubi:latest
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

LABEL com.redhat.component="rhtpa-operator"
LABEL description="Red Hat Trusted Profile Analyzer Operator"
LABEL io.k8s.description="Red Hat Trusted Profile Analyzer Operator"
LABEL io.k8s.display-name="RHTPA operator for Red Hat Trusted Profile Analyzer"
LABEL io.openshift.tags="RHTPA, rhtpa-operator, Red Hat Trusted Profile Analyzer"
LABEL name="rhtpa/rhtpa-rhel9-operator"
LABEL org.opencontainers.image.source="https://github.com/trustification/trusted-profile-analyzer-operator"
LABEL summary="RHTPA Operator"
LABEL version="1.0.3"
LABEL release=1.0.3
LABEL maintainer="Red Hat"

RUN microdnf update -y && microdnf clean all -y

ENV HOME=/opt/helm \
    USER_NAME=helm \
    USER_UID=1001

RUN echo "${USER_NAME}:x:${USER_UID}:0:${USER_NAME} user:${HOME}:/sbin/nologin" >> /etc/passwd

# Copy necessary files with the right permissions
COPY --chown=${USER_UID}:0 watches.yaml ${HOME}/watches.yaml
COPY --chown=${USER_UID}:0 helm-charts  ${HOME}/helm-charts

# Copy manager binary
COPY --from=builder /workspace/manager .

USER ${USER_UID}

WORKDIR ${HOME}

ENTRYPOINT ["/manager"]
