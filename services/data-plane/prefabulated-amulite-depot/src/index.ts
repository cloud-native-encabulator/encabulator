/**
 * prefabulated-amulite-depot entrypoint.
 *
 * Telemetry starts before anything else is imported so the OpenTelemetry
 * instrumentations get to patch `http`, `undici` and `ioredis` first.
 */

import { startTelemetry } from '@encabulator/kit/telemetry';

const telemetry = startTelemetry({
  serviceName: process.env.OTEL_SERVICE_NAME ?? 'prefabulated-amulite-depot',
});

const { main } = await import('./main.js');
await main(telemetry);
