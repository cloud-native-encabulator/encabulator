/** Valkey connection, via ioredis (Valkey speaks the same protocol). */

import { requireEnv } from '@encabulator/kit';
import { Redis } from 'ioredis';

export interface ValkeyOptions {
  url?: string;
  connectionName: string;
}

export function createValkey(opts: ValkeyOptions): Redis {
  const url = opts.url ?? requireEnv('VALKEY_URL');
  return new Redis(url, {
    connectionName: opts.connectionName,
    // The offline queue holds commands while the socket is still coming up, which matters
    // during startup. A genuinely unreachable Valkey still fails after two retries, so
    // `/readyz` goes red rather than hanging.
    enableOfflineQueue: true,
    maxRetriesPerRequest: 2,
    connectTimeout: 3_000,
    retryStrategy: (attempts) => Math.min(attempts * 200, 2_000),
  });
}

/** Readiness check: Valkey is a hard dependency for the depot (design 8.7). */
export function valkeyReadiness(redis: Redis): { name: string; check: () => Promise<void> } {
  return {
    name: 'valkey',
    check: async () => {
      const pong = await redis.ping();
      if (pong !== 'PONG') throw new Error(`unexpected PING reply ${pong}`);
    },
  };
}
