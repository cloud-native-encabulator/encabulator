# syntax=docker/dockerfile:1.7
#
# spurving-bearing-quoter (Rust)
#
# Build context is the repository root (design section 7, "Containers"), so every COPY path here is
# repository-relative. From the repository root:
#
#   docker buildx build \
#     --platform linux/amd64,linux/arm64 \
#     -f services/processing-plane/spurving-bearing-quoter/Dockerfile \
#     -t ghcr.io/jlbutler/cloud-native-encabulator/spurving-bearing-quoter:1.0.0 .
#
# or just `make image` from the service directory.
#
# Multi-arch: every stage builds natively for the target platform under buildx/QEMU rather than
# cross-compiling. Cross-compiling Rust needs a second toolchain and linker per target, and the
# quoter has no C dependencies to make that worthwhile.
#
# No HEALTHCHECK: Kubernetes probes own health (design section 7).

ARG RUST_VERSION=1.98
ARG RUNTIME_IMAGE=gcr.io/distroless/cc-debian13:nonroot
ARG CARGO_CHEF_VERSION=0.1.78

# ---------------------------------------------------------------------------------------------
# chef: the toolchain plus cargo-chef, shared by the planner and the builder.
# ---------------------------------------------------------------------------------------------
FROM rust:${RUST_VERSION}-slim-trixie AS chef
ARG CARGO_CHEF_VERSION
ENV CARGO_TERM_COLOR=never
WORKDIR /src
RUN cargo install cargo-chef --locked --version ${CARGO_CHEF_VERSION}

# ---------------------------------------------------------------------------------------------
# planner: reduce the manifests to a recipe, so a source-only change does not rebuild the world.
# ---------------------------------------------------------------------------------------------
FROM chef AS planner
COPY services/processing-plane/spurving-bearing-quoter/Cargo.toml ./Cargo.toml
COPY services/processing-plane/spurving-bearing-quoter/Cargo.lock ./Cargo.lock
COPY services/processing-plane/spurving-bearing-quoter/src ./src
COPY services/processing-plane/spurving-bearing-quoter/tests ./tests
RUN cargo chef prepare --recipe-path recipe.json

# ---------------------------------------------------------------------------------------------
# builder: dependencies first (cached), then the service itself.
# ---------------------------------------------------------------------------------------------
FROM chef AS builder
COPY --from=planner /src/recipe.json recipe.json
RUN cargo chef cook --release --locked --recipe-path recipe.json

COPY services/processing-plane/spurving-bearing-quoter/Cargo.toml ./Cargo.toml
COPY services/processing-plane/spurving-bearing-quoter/Cargo.lock ./Cargo.lock
COPY services/processing-plane/spurving-bearing-quoter/src ./src
RUN cargo build --release --locked --bin spurving-bearing-quoter \
 && strip target/release/spurving-bearing-quoter

# ---------------------------------------------------------------------------------------------
# runtime: distroless, nonroot (UID/GID 65532), read-only root filesystem compatible.
# ---------------------------------------------------------------------------------------------
FROM ${RUNTIME_IMAGE} AS runtime

ARG VERSION=1.0.0
LABEL org.opencontainers.image.title="spurving-bearing-quoter" \
      org.opencontainers.image.description="Pricing engine for the Cloud Native Encabulator: aligns the spurving bearings with the pentametric fan, economically." \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.source="https://github.com/jlbutler/cloud-native-encabulator"

COPY --from=builder /src/target/release/spurving-bearing-quoter /usr/local/bin/spurving-bearing-quoter

# The binary writes nothing outside /tmp, which the chart mounts as an emptyDir.
ENV PORT=8080 \
    LOG_LEVEL=info \
    OTEL_SERVICE_NAME=spurving-bearing-quoter \
    TMPDIR=/tmp

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/spurving-bearing-quoter"]
