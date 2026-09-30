/**
 * Entry point.
 *
 * `@encabulator/kit/telemetry` is imported first and the rest of the console is
 * dynamic-imported afterwards. That ordering is load-bearing: ESM hoists static
 * imports, so the OpenTelemetry instrumentations only get to patch `http` and
 * `undici` if nothing has imported them yet. The kit's own comment says the same.
 */

import { startTelemetry } from '@encabulator/kit/telemetry'

const { run } = await import('./run.js')
await run(startTelemetry({ serviceName: 'encabulation-control-console' }))
