# We do not use --platform feature to auto fill this ARG because of incompatibility between podman and docker
ARG TARGETARCH=amd64

# Build the manager binary
FROM docker.io/library/golang:1.26 AS builder

ARG TARGETARCH
ARG LDFLAGS

WORKDIR /opt/app-root

COPY cmd cmd
COPY internal internal
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/

# Build the in-pod collector/agent binary
RUN GOARCH=$TARGETARCH go build -ldflags "$LDFLAGS" -mod vendor -a -o build/network-observability-cli ./cmd/collector

# Also build the client plugin (oc/kubectl-netobserv) and embed it in the image so users without
# access to the GitHub release/krew can `oc cp` it out of a running pod. internal/plugin/manifests'
# go:embed'd res/ directory is already present under internal/, copied above.
RUN CGO_ENABLED=0 GOARCH=$TARGETARCH go build -ldflags "$LDFLAGS" -mod vendor -a -o build/oc-netobserv ./cmd/plugin && \
    cp build/oc-netobserv build/kubectl-netobserv

# Prepare output dir
RUN mkdir -p output

# Create final image from ubi + built binary and command
FROM --platform=linux/$TARGETARCH registry.access.redhat.com/ubi9/ubi-minimal:1790754202

RUN microdnf install -y tar && \
    microdnf clean all

WORKDIR /

COPY --from=builder /opt/app-root/build .
COPY --from=builder --chown=65532:65532 /opt/app-root/output /output
USER 65532:65532

ENTRYPOINT ["/network-observability-cli"]
