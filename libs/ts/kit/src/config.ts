/**
 * Environment-variable configuration helpers.
 *
 * Design section 7 ("Configuration"): environment variables only. Everything is read
 * lazily through these helpers so tests can mutate `process.env` freely.
 */

export function envRaw(name: string, env: NodeJS.ProcessEnv = process.env): string | undefined {
  const value = env[name];
  if (value === undefined) return undefined;
  const trimmed = value.trim();
  return trimmed === '' ? undefined : trimmed;
}

export function envString(name: string, fallback: string, env = process.env): string {
  return envRaw(name, env) ?? fallback;
}

export function envInt(name: string, fallback: number, env = process.env): number {
  const raw = envRaw(name, env);
  if (raw === undefined) return fallback;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

export function envFloat(name: string, fallback: number, env = process.env): number {
  const raw = envRaw(name, env);
  if (raw === undefined) return fallback;
  const parsed = Number.parseFloat(raw);
  return Number.isFinite(parsed) ? parsed : fallback;
}

export function envBool(name: string, fallback: boolean, env = process.env): boolean {
  const raw = envRaw(name, env)?.toLowerCase();
  if (raw === undefined) return fallback;
  if (['1', 'true', 'yes', 'on'].includes(raw)) return true;
  if (['0', 'false', 'no', 'off'].includes(raw)) return false;
  return fallback;
}

export function requireEnv(name: string, env = process.env): string {
  const raw = envRaw(name, env);
  if (raw === undefined) {
    throw new Error(`required environment variable ${name} is not set`);
  }
  return raw;
}

/** `OTEL_SERVICE_NAME`, falling back to the name baked into the service code. */
export function serviceName(fallback: string, env = process.env): string {
  return envString('OTEL_SERVICE_NAME', fallback, env);
}

/** `PORT`, default 8080 (design Appendix A). */
export function servicePort(env = process.env): number {
  return envInt('PORT', 8080, env);
}

export function logLevel(env = process.env): string {
  return envString('LOG_LEVEL', 'info', env);
}

export function serviceVersion(env = process.env): string {
  return envString('SERVICE_VERSION', '1.0.0', env);
}
