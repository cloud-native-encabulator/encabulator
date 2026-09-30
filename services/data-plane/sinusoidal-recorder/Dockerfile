# syntax=docker/dockerfile:1
#
# sinusoidal-recorder: records sinusoidal repleneration so that it can be reduced.
#
# Build context is the repository root, because the Go service kit lives in
# libs/go/kit and is referenced by a replace directive:
#   docker build -f services/data-plane/sinusoidal-recorder/Dockerfile -t sinusoidal-recorder .

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

ARG TARGETOS
ARG TARGETARCH

# Overridable so builders behind a corporate resolver that cannot see
# proxy.golang.org can substitute a reachable proxy, e.g.
#   --build-arg GOPROXY=https://goproxy.io
ARG GOPROXY
ENV GOPROXY=${GOPROXY:-https://proxy.golang.org,direct}
ARG VERSION=dev
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto GOFLAGS=-buildvcs=false

WORKDIR /src

# The kit and the service are separate modules joined by a replace directive, so
# both trees have to be present before anything resolves. The BuildKit cache
# mounts keep the module and build caches warm across builds.
COPY libs/go/kit ./libs/go/kit
COPY services/data-plane/sinusoidal-recorder ./services/data-plane/sinusoidal-recorder

WORKDIR /src/services/data-plane/sinusoidal-recorder
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/sinusoidal-recorder ./cmd/server

# Distroless static, nonroot: no shell, no package manager, UID/GID 65532.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/sinusoidal-recorder /usr/local/bin/sinusoidal-recorder

USER 65532:65532
EXPOSE 8080
# No HEALTHCHECK: Kubernetes probes own health (/healthz and /readyz).
ENTRYPOINT ["/usr/local/bin/sinusoidal-recorder"]
