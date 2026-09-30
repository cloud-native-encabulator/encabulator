# The Encabulation Control Console: a Vue 3 SPA served by a Fastify BFF in one
# container.
#
# Build context is the repository ROOT, not this directory, so the shared
# TypeScript kit can be copied in (design section 7, "Containers"):
#
#   docker build -f services/control-plane/encabulation-control-console/Dockerfile \
#     -t ghcr.io/jlbutler/cloud-native-encabulator/encabulation-control-console:1.0.0 .
#
# Four stages: the kit, the installed dependency tree, the two builds, and a
# runtime that carries compiled JavaScript and production dependencies only.
# Every COPY names explicit paths, so `dev/` — the fixture server, the fake
# cluster reader, dev.env — cannot reach the image even by accident.

# ─────────────────────────────────────────────────────────────── stage 1: kit ──
FROM node:24-slim AS kit
WORKDIR /repo/libs/ts/kit

COPY libs/ts/kit/package.json libs/ts/kit/package-lock.json ./
RUN npm ci --ignore-scripts

COPY libs/ts/kit/tsconfig.json libs/ts/kit/tsconfig.build.json ./
COPY libs/ts/kit/src ./src
RUN npm run build && npm prune --omit=dev

# ────────────────────────────────────────────────────────────── stage 2: deps ──
FROM node:24-slim AS deps
ENV NPM_CONFIG_FUND=false NPM_CONFIG_AUDIT=false

# The kit is a `file:` dependency, so it must sit at the path the console's
# package.json names (../../../libs/ts/kit) before `npm ci` runs.
COPY --from=kit /repo/libs/ts/kit /repo/libs/ts/kit

WORKDIR /repo/services/control-plane/encabulation-control-console
COPY services/control-plane/encabulation-control-console/package.json \
     services/control-plane/encabulation-control-console/package-lock.json ./
RUN npm ci --ignore-scripts && npm rebuild esbuild

# ───────────────────────────────────────────────────────────── stage 3: build ──
FROM deps AS build
WORKDIR /repo/services/control-plane/encabulation-control-console

COPY services/control-plane/encabulation-control-console/tsconfig.json \
     services/control-plane/encabulation-control-console/tsconfig.bff.json \
     services/control-plane/encabulation-control-console/tsconfig.web.json \
     services/control-plane/encabulation-control-console/vite.config.ts \
     services/control-plane/encabulation-control-console/env.d.ts \
     services/control-plane/encabulation-control-console/index.html ./
COPY services/control-plane/encabulation-control-console/public ./public
COPY services/control-plane/encabulation-control-console/src ./src

# `vite build` emits the SPA to dist/web with no source maps; `tsc` emits the BFF
# to dist/bff with no source maps and no declarations. Neither ships a map.
RUN npx vue-tsc -p tsconfig.web.json --noEmit \
 && npx vite build \
 && npx tsc -p tsconfig.bff.json \
 && find dist -name '*.map' -delete

# ───────────────────────────────────────────────────── stage 4: prod modules ──
FROM deps AS prod-modules
WORKDIR /repo/services/control-plane/encabulation-control-console
RUN npm prune --omit=dev

# ─────────────────────────────────────────────────────────── stage 5: runtime ──
FROM node:24-slim AS runtime

ENV NODE_ENV=production \
    TMPDIR=/tmp \
    PORT=8080

WORKDIR /app

COPY --from=prod-modules --chown=65532:65532 \
     /repo/services/control-plane/encabulation-control-console/node_modules /app/node_modules
COPY --from=prod-modules --chown=65532:65532 \
     /repo/services/control-plane/encabulation-control-console/package.json /app/package.json
COPY --from=build --chown=65532:65532 \
     /repo/services/control-plane/encabulation-control-console/dist /app/dist

# `npm ci` linked @encabulator/kit with a relative symlink out to libs/ts/kit,
# which no longer exists at that depth here. Replace the link with the kit's real
# runtime tree so module resolution needs nothing outside /app.
RUN rm -rf /app/node_modules/@encabulator/kit
COPY --from=kit --chown=65532:65532 \
     /repo/libs/ts/kit /app/node_modules/@encabulator/kit

# Where the BFF finds the built SPA. Set explicitly rather than inferred, so a
# future layout change fails loudly instead of serving a blank page.
ENV WEB_ROOT=/app/dist/web

# Fail the build, not the deployment, if the import graph is broken or the SPA is
# missing. Cheap, and it has caught a bad COPY path more than once.
RUN node --input-type=module -e "\
  import { access } from 'node:fs/promises'; \
  const kit = await import('@encabulator/kit'); \
  if (typeof kit.createApp !== 'function') throw new Error('kit did not load'); \
  await import('./dist/bff/server/app.js'); \
  await access('./dist/web/index.html'); \
  console.log('image self-check passed');"

# Design section 7: UID/GID 65532, read-only root filesystem compatible. The
# process writes nothing outside /tmp, which the chart mounts as an emptyDir.
USER 65532:65532

EXPOSE 8080

# No HEALTHCHECK: Kubernetes probes own health (design section 7).
ENTRYPOINT ["node", "dist/bff/server/main.js"]
