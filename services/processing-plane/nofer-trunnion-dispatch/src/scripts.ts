/**
 * Lua for the dispatch office. Two things must be atomic: booking a shipment exactly once
 * per order, and claiming a due shipment so that replicas never double-deliver.
 *
 * The shipment record itself is immutable once written; delivery is recorded as a separate
 * key. That keeps every script free of `cjson.encode`, so nothing round-trips the stored
 * JSON and no field can be quietly reshaped.
 */

/**
 * Book a shipment. The `GET` guard makes the whole thing idempotent, which matters because
 * `inspection.passed` can be redelivered.
 *
 * KEYS[1]  shipment key
 * KEYS[2]  due sorted set
 * KEYS[3]  recent list
 *
 * ARGV[1]  shipment JSON
 * ARGV[2]  order id
 * ARGV[3]  due time, epoch millis
 * ARGV[4]  recent list cap
 *
 * Returns {"booked"} | {"existing", json}
 */
export const DISPATCH_SCRIPT = `
local existing = redis.call('GET', KEYS[1])
if existing then
  return {'existing', existing}
end
redis.call('SET', KEYS[1], ARGV[1])
redis.call('ZADD', KEYS[2], tonumber(ARGV[3]), ARGV[2])
redis.call('LPUSH', KEYS[3], ARGV[2])
redis.call('LTRIM', KEYS[3], 0, tonumber(ARGV[4]) - 1)
return {'booked'}
`;

/**
 * Claim a due shipment and record the delivery, in one step. The `ZREM` is the claim: only
 * the replica whose `ZREM` removes the member owns the delivery, so replicas never publish
 * `shipment.delivered` twice for the same order.
 *
 * KEYS[1]  due sorted set
 * KEYS[2]  shipment key
 * KEYS[3]  delivered marker key
 *
 * ARGV[1]  order id
 * ARGV[2]  delivered-at timestamp
 *
 * Returns {"lost"} | {"missing"} | {"ok", json}
 */
export const CLAIM_DELIVERY_SCRIPT = `
if redis.call('ZREM', KEYS[1], ARGV[1]) ~= 1 then
  return {'lost'}
end
local raw = redis.call('GET', KEYS[2])
if not raw then
  redis.call('DEL', KEYS[3])
  return {'missing'}
end
redis.call('SET', KEYS[3], ARGV[2])
return {'ok', raw}
`;

/**
 * Put a claimed shipment back in the queue. Used when publishing `shipment.delivered`
 * failed, so the delivery is retried rather than silently lost.
 *
 * KEYS[1]  due sorted set
 * KEYS[2]  delivered marker key
 *
 * ARGV[1]  order id
 * ARGV[2]  retry time, epoch millis
 */
export const REARM_SCRIPT = `
redis.call('DEL', KEYS[2])
redis.call('ZADD', KEYS[1], tonumber(ARGV[2]), ARGV[1])
return {'rearmed'}
`;
