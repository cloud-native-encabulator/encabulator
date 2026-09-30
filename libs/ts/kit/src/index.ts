/**
 * `@encabulator/kit` — the shared TypeScript service kit for the Cloud Native
 * Encabulator. It implements design section 7 (cross-cutting conventions) for Node 24 on
 * Fastify 5, so each service only has to contain its own domain.
 *
 * Service entrypoints should import `@encabulator/kit/telemetry` first and only then
 * dynamic-import the rest; see that module's comment for why.
 */

export * from './app.js';
export * from './config.js';
export * from './errors.js';
export * from './flags.js';
export * from './hash.js';
export * from './http-client.js';
export * from './identity.js';
export * from './ids.js';
export * from './logger.js';
export * from './malaise.js';
export * from './metrics.js';
export * from './nats.js';
export * from './paths.js';
export * from './telemetry.js';
