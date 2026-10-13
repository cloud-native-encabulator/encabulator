/**
 * The depot's Valkey keyspace. The depot owns `{depot}:*` and nothing else touches it
 * (design section 2, service catalog).
 *
 * `{depot}` is a hash tag: every key hashes to the same slot, so the multi-key Lua scripts,
 * the seeding transaction and MGET also work on a cluster-mode server such as ElastiCache
 * Serverless, which refuses a command whose keys span slots (CROSSSLOT).
 */

export const PREFIX = '{depot}:';

export const stockKey = (sku: string) => `${PREFIX}stock:${sku}`;
export const reservedKey = (sku: string) => `${PREFIX}reserved:${sku}`;
export const reservationKey = (id: string) => `${PREFIX}reservation:${id}`;
/** Idempotency index: order id to reservation id. */
export const orderIndexKey = (orderId: string) => `${PREFIX}order:${orderId}`;
/** Sorted set of pending reservations, scored by expiry, read by the sweeper. */
export const PENDING_KEY = `${PREFIX}pending`;

/**
 * The stock/reserved key pair for each BOM line, flattened in BOM order. The Lua scripts
 * take these in `KEYS` so the keys a script touches are always declared up front.
 */
export function stockKeyPairs(skus: readonly string[]): string[] {
  return skus.flatMap((sku) => [stockKey(sku), reservedKey(sku)]);
}
