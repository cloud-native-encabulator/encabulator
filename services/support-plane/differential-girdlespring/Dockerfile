# syntax=docker/dockerfile:1
#
# differential-girdlespring: the differential girdlespring on the up end of the grammeters.
#
# Build context is the repository root, because the Go service kit lives in
# libs/go/kit and is referenced by a replace directive:
#   docker build -f services/support-plane/differential-girdlespring/Dockerfile -t differential-girdlespring .

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
COPY services/support-plane/differential-girdlespring ./services/support-plane/differential-girdlespring

WORKDIR /src/services/support-plane/differential-girdlespring
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/differential-girdlespring ./cmd/server

# Distroless static, nonroot: no shell, no package manager, UID/GID 65532.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/differential-girdlespring /usr/local/bin/differential-girdlespring

USER 65532:65532
EXPOSE 8080
# No HEALTHCHECK: Kubernetes probes own health (/healthz and /readyz).
ENTRYPOINT ["/usr/local/bin/differential-girdlespring"]
