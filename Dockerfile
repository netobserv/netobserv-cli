# We do not use --platform feature to auto fill this ARG because of incompatibility between podman and docker
ARG TARGETARCH=amd64

# Build the manager binary
FROM docker.io/library/golang:1.26 AS builder

ARG TARGETARCH
ARG LDFLAGS

WORKDIR /opt/app-root

COPY cmd cmd
COPY internal internal
COPY main.go main.go
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/

# Build collector
RUN GOARCH=$TARGETARCH go build -ldflags "$LDFLAGS" -mod vendor -a -o build/network-observability-cli

# We still need Makefile & resources for oc-commands; copy them after go build for caching
COPY commands/ commands/
COPY res/ res/
COPY scripts/ scripts/
COPY Makefile Makefile
COPY .mk/ .mk/

# Embed commands in case users want to pull it from collector image
RUN USER=netobserv VERSION=main make oc-commands

# Prepare output dir
RUN mkdir -p output

# Create final image from ubi + built binary and command
FROM --platform=linux/$TARGETARCH registry.access.redhat.com/ubi9/ubi-minimal:1789639833

RUN microdnf install -y tar && \
    microdnf clean all

WORKDIR /

COPY --from=builder /opt/app-root/build .
COPY --from=builder --chown=65532:65532 /opt/app-root/output /output
USER 65532:65532

ENTRYPOINT ["/network-observability-cli"]
