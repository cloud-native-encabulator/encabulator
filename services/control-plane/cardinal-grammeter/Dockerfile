# syntax=docker/dockerfile:1
# cardinal-grammeter
#
# Build context is the repository root, so that libs/go/kit can be copied in and
# resolved through the module's replace directive:
#
#   docker build -f services/control-plane/cardinal-grammeter/Dockerfile .
#
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

ARG TARGETOS
ARG TARGETARCH

# Overridable so builders behind a corporate resolver that cannot see
# proxy.golang.org can substitute a reachable proxy, e.g.
#   --build-arg GOPROXY=https://goproxy.io
ARG GOPROXY
ENV GOPROXY=${GOPROXY:-https://proxy.golang.org,direct}

WORKDIR /src

# The kit first, so a change to the service does not invalidate its download.
COPY libs/go/kit/go.mod libs/go/kit/go.sum ./libs/go/kit/
COPY services/control-plane/cardinal-grammeter/go.mod services/control-plane/cardinal-grammeter/go.sum ./services/control-plane/cardinal-grammeter/

WORKDIR /src/services/control-plane/cardinal-grammeter
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY libs/go/kit /src/libs/go/kit
COPY services/control-plane/cardinal-grammeter /src/services/control-plane/cardinal-grammeter

# Static binary: distroless static carries no libc.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /out/cardinal-grammeter .

FROM gcr.io/distroless/static-debian12:nonroot

# UID/GID 65532 (distroless "nonroot"). Read-only root filesystem compatible:
# the grammeter writes nothing to disk.
USER 65532:65532
WORKDIR /
COPY --from=build /out/cardinal-grammeter /usr/local/bin/cardinal-grammeter

EXPOSE 8080

# No HEALTHCHECK: Kubernetes probes own health.
ENTRYPOINT ["/usr/local/bin/cardinal-grammeter"]
