/**
 * The dispatch office's Valkey keyspace. Dispatch owns `{shipment}:*` (design section 2).
 *
 * `{shipment}` is a hash tag: every key hashes to the same slot, so the multi-key Lua scripts
 * and MGET also work on a cluster-mode server such as ElastiCache Serverless, which refuses a
 * command whose keys span slots (CROSSSLOT).
 */

export const PREFIX = '{shipment}:';

/** The immutable shipment record. */
export const shipmentKey = (orderId: string) => `${PREFIX}order:${orderId}`;
/** Presence means delivered; the value is the delivery timestamp. */
export const deliveredKey = (orderId: string) => `${PREFIX}delivered:${orderId}`;
/** Sorted set of shipments awaiting delivery, scored by due time in epoch millis. */
export const DUE_KEY = `${PREFIX}due`;
/** Capped list of recently dispatched order ids, newest first. */
export const RECENT_KEY = `${PREFIX}recent`;
export const RECENT_LIMIT = 500;
