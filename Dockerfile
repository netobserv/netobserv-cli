# We do not use --platform feature to auto fill this ARG because of incompatibility between podman and docker
ARG TARGETARCH=amd64

# Build the collector and standalone Go plugin
# Match the target platform so the collector retains SQLite CGO support.
FROM --platform=linux/$TARGETARCH docker.io/library/golang:1.26 AS builder

ARG TARGETARCH
ARG LDFLAGS
ARG VERSION=main
ARG COLLECTOR_IMAGE=quay.io/netobserv/network-observability-cli:${VERSION}
ARG AGENT_IMAGE=quay.io/netobserv/netobserv-ebpf-agent:main
ARG PULL_POLICY=Always

WORKDIR /opt/app-root

COPY cmd cmd
COPY internal internal
COPY main.go main.go
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/

# Build collector
RUN GOARCH=$TARGETARCH go build -ldflags "$LDFLAGS" -mod vendor -a -o build/network-observability-cli

# Build the native plugin through the shared Makefile target.
COPY res/ res/
COPY Makefile Makefile
COPY .mk/ .mk/
RUN make oc-commands GOARCH=$TARGETARCH PLUGIN_GOOS=linux \
    VERSION="$VERSION" IMAGE="$COLLECTOR_IMAGE" AGENT_IMAGE="$AGENT_IMAGE" \
    PULL_POLICY="$PULL_POLICY"

# Prepare output dir
RUN mkdir -p output && chmod 0775 output

# Create final image from ubi + built binary and command
FROM --platform=linux/$TARGETARCH registry.access.redhat.com/ubi9/ubi-minimal:1789639833

RUN microdnf install -y tar && \
    microdnf clean all

WORKDIR /

COPY --from=builder /opt/app-root/build/network-observability-cli /network-observability-cli
COPY --from=builder /opt/app-root/build/oc-netobserv /oc-netobserv
# OpenShift runs containers with an arbitrary UID in group 0.
COPY --from=builder --chown=65532:0 /opt/app-root/output /output
RUN chmod 0775 /output
USER 65532:65532

ENTRYPOINT ["/network-observability-cli"]
