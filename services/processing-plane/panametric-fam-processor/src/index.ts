/**
 * panametric-fam-processor entrypoint.
 *
 * Telemetry starts before anything else is imported so the OpenTelemetry instrumentations
 * get to patch `http` and `undici` first.
 */

import { startTelemetry } from '@encabulator/kit/telemetry';

const telemetry = startTelemetry({
  serviceName: process.env.OTEL_SERVICE_NAME ?? 'panametric-fam-processor',
});

const { main } = await import('./main.js');
await main(telemetry);
