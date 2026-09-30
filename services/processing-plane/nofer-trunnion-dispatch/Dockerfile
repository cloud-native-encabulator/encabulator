# nofer-trunnion-dispatch
#
# Build context is the repository root, so the shared kit can be copied in:
#   docker build -f services/processing-plane/nofer-trunnion-dispatch/Dockerfile -t nofer-trunnion-dispatch .
#
# The image mirrors the repository layout under /app. That is not cosmetic: the service's
# node_modules/@encabulator/kit is a relative symlink into libs/ts/kit, and the kit's own
# compiled code resolves fastify, pino and the OTel packages from libs/ts/kit/node_modules.
# Reproducing the dev tree means `make run` and the container behave identically.

ARG NODE_VERSION=24

# ---------- stage 1: build the shared kit, then drop its dev dependencies ----------
FROM node:${NODE_VERSION}-slim AS kit
WORKDIR /app/libs/ts/kit
COPY libs/ts/kit/package.json libs/ts/kit/package-lock.json ./
RUN npm ci
COPY libs/ts/kit/tsconfig.json libs/ts/kit/tsconfig.build.json ./
COPY libs/ts/kit/src ./src
RUN npm run build && npm prune --omit=dev && rm -rf src

# ---------- stage 2: build the service ----------
FROM node:${NODE_VERSION}-slim AS build
WORKDIR /app/services/processing-plane/nofer-trunnion-dispatch
COPY --from=kit /app/libs/ts/kit /app/libs/ts/kit
COPY services/processing-plane/nofer-trunnion-dispatch/package.json \
     services/processing-plane/nofer-trunnion-dispatch/package-lock.json ./
RUN npm ci
COPY services/processing-plane/nofer-trunnion-dispatch/tsconfig.json \
     services/processing-plane/nofer-trunnion-dispatch/tsconfig.build.json ./
COPY services/processing-plane/nofer-trunnion-dispatch/src ./src
RUN npm run build

# ---------- stage 3: production dependencies only ----------
FROM node:${NODE_VERSION}-slim AS deps
WORKDIR /app/services/processing-plane/nofer-trunnion-dispatch
COPY --from=kit /app/libs/ts/kit /app/libs/ts/kit
COPY services/processing-plane/nofer-trunnion-dispatch/package.json \
     services/processing-plane/nofer-trunnion-dispatch/package-lock.json ./
RUN npm ci --omit=dev

# ---------- stage 4: runtime ----------
FROM gcr.io/distroless/nodejs24-debian13:nonroot
WORKDIR /app/services/processing-plane/nofer-trunnion-dispatch

COPY --from=kit --chown=65532:65532 /app/libs/ts/kit /app/libs/ts/kit
COPY --from=deps --chown=65532:65532 \
     /app/services/processing-plane/nofer-trunnion-dispatch/node_modules ./node_modules
COPY --from=build --chown=65532:65532 \
     /app/services/processing-plane/nofer-trunnion-dispatch/dist ./dist
COPY --chown=65532:65532 services/processing-plane/nofer-trunnion-dispatch/package.json ./package.json

ENV NODE_ENV=production \
    PORT=8080 \
    NODE_OPTIONS=--enable-source-maps

USER 65532:65532
EXPOSE 8080

# No HEALTHCHECK: Kubernetes probes own health (design section 7, "Containers").
CMD ["dist/index.js"]
