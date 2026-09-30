# logarithmic-casing-line. Build context is the repository root:
#   docker build -f services/processing-plane/logarithmic-casing-line/Dockerfile .
# syntax=docker/dockerfile:1
ARG PYTHON_VERSION=3.14
ARG UV_VERSION=0.11

FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv

FROM python:${PYTHON_VERSION}-slim AS builder
COPY --from=uv /uv /usr/local/bin/uv
ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never \
    UV_PROJECT_ENVIRONMENT=/app/.venv

# numpy ships wheels for both architectures, so no toolchain is needed.
WORKDIR /src
COPY libs/python/encabulator-kit /src/libs/python/encabulator-kit
COPY services/processing-plane/logarithmic-casing-line \
     /src/services/processing-plane/logarithmic-casing-line

WORKDIR /src/services/processing-plane/logarithmic-casing-line
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-editable

FROM python:${PYTHON_VERSION}-slim AS runtime
LABEL org.opencontainers.image.title="logarithmic-casing-line" \
      org.opencontainers.image.description="Fabrication workers for the Encabulator Works." \
      org.opencontainers.image.source="https://github.com/jlbutler/cloud-native-encabulator"

ENV PATH="/app/.venv/bin:${PATH}" \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    HOME=/tmp \
    TMPDIR=/tmp \
    PORT=8080

# Only the virtual environment: it already contains the service and the kit as wheels.
COPY --from=builder /app/.venv /app/.venv

USER 65532:65532
WORKDIR /tmp
EXPOSE 8080
# No HEALTHCHECK: Kubernetes probes own health.
ENTRYPOINT ["/app/.venv/bin/python", "-m", "casing_line"]
