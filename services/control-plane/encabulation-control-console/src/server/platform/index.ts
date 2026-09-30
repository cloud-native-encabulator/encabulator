/**
 * The seam onto the shared TypeScript kit (`@encabulator/kit`, `libs/ts/kit`).
 *
 * Everything cross-cutting comes from there: structured logging, the OpenTelemetry
 * bootstrap, the Fastify factory with `/healthz`, `/readyz`, `/metrics`, request
 * ids and the error shape, the two HTTP duration histograms, and the outbound
 * HTTP client with the house timeout and retry policy.
 *
 * One kit feature the console deliberately does not use: `MalaiseController`.
 * Design section 7 is explicit that the console, the grammeter, the malaise
 * engine and the orchestrator do not apply malaise to themselves, so `createApp`
 * is called without a controller. The console asks the engine to break other
 * services; breaking the instrument panel would only hide the demo.
 */

export {
  ALL_ROLES,
  HttpClient,
  HttpClientError,
  HttpError,
  ROLE_ADMIN,
  ROLE_CUSTOMER,
  ROLE_OPERATOR,
  ROLE_VIEWER,
  ROLES_HEADER,
  SUBJECT_HEADER,
  createApp,
  createLogger,
  errorBody,
  forbidden,
  hasAnyRole,
  identityHeaders,
  logLevel,
  serveApp,
  serviceName,
  servicePort,
  serviceVersion,
  unauthorized,
  unavailable,
} from '@encabulator/kit'

export type {
  ErrorBody,
  HttpResponse,
  Identity,
  Logger,
  ReadinessCheck,
  RunningService,
  Telemetry,
} from '@encabulator/kit'

export { consoleMetrics, type ConsoleMetrics } from './metrics.js'
