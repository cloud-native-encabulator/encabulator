/**
 * The Lua scripts that make the parts cage honest.
 *
 * Every script declares every key it touches in `KEYS`, and the reserve script is the one
 * the design calls for: "one Lua script: check all, then decrement all". Nothing in here
 * re-encodes the stored reservation JSON; `cjson.decode` is used only to read the current
 * status, and the replacement blob always comes from the caller.
 */

/**
 * Reserve every SKU in the bill of materials, or nothing at all.
 *
 * KEYS[1]                  order index key (idempotency)
 * KEYS[2]                  pending sorted set
 * KEYS[3]                  reservation key
 * KEYS[4 + 2*(i-1)]        stock key for line i
 * KEYS[5 + 2*(i-1)]        reserved key for line i
 *
 * ARGV[1]                  reservation id
 * ARGV[2]                  reservation JSON
 * ARGV[3]                  expiry, epoch millis (the sorted-set score)
 * ARGV[4]                  key TTL in seconds
 * ARGV[5]                  number of BOM lines
 * ARGV[6 + 2*(i-1)]        sku for line i
 * ARGV[7 + 2*(i-1)]        quantity for line i
 *
 * Returns {"existing", id} | {"short", sku, available, needed, ...} | {"created"}
 */
export const RESERVE_SCRIPT = `
local existing = redis.call('GET', KEYS[1])
if existing then
  return {'existing', existing}
end

local n = tonumber(ARGV[5])
local short = {'short'}
for i = 1, n do
  local stockKey = KEYS[4 + (i - 1) * 2]
  local sku = ARGV[6 + (i - 1) * 2]
  local needed = tonumber(ARGV[7 + (i - 1) * 2])
  local available = tonumber(redis.call('GET', stockKey) or '0')
  if available < needed then
    table.insert(short, sku)
    table.insert(short, tostring(available))
    table.insert(short, tostring(needed))
  end
end
if #short > 1 then
  return short
end

for i = 1, n do
  local quantity = tonumber(ARGV[7 + (i - 1) * 2])
  redis.call('DECRBY', KEYS[4 + (i - 1) * 2], quantity)
  redis.call('INCRBY', KEYS[5 + (i - 1) * 2], quantity)
end

local ttl = tonumber(ARGV[4])
redis.call('SET', KEYS[3], ARGV[2], 'EX', ttl)
redis.call('SET', KEYS[1], ARGV[1], 'EX', ttl)
redis.call('ZADD', KEYS[2], tonumber(ARGV[3]), ARGV[1])
return {'created'}
`;

/**
 * Commit a pending reservation: the parts are now inside an encabulator, so the reserved
 * count drops and the stock stays where it is.
 *
 * KEYS[1]                  reservation key
 * KEYS[2]                  pending sorted set
 * KEYS[3]                  order index key (kept on commit, dropped on release)
 * KEYS[4 + 2*(i-1)]        stock key for line i (untouched here, declared for symmetry)
 * KEYS[5 + 2*(i-1)]        reserved key for line i
 *
 * ARGV[1]                  the new reservation JSON, status already "committed"
 * ARGV[2]                  reservation id
 * ARGV[3]                  number of BOM lines
 * ARGV[4 + 2*(i-1)]        sku for line i (unused, kept parallel to the reserve script)
 * ARGV[5 + 2*(i-1)]        quantity for line i
 *
 * Returns {"missing"} | {"conflict", status} | {"ok"}
 */
export const COMMIT_SCRIPT = `
local raw = redis.call('GET', KEYS[1])
if not raw then
  return {'missing'}
end
local current = cjson.decode(raw)
if current.status ~= 'pending' then
  return {'conflict', current.status}
end

local n = tonumber(ARGV[3])
for i = 1, n do
  local quantity = tonumber(ARGV[5 + (i - 1) * 2])
  redis.call('DECRBY', KEYS[5 + (i - 1) * 2], quantity)
end

redis.call('SET', KEYS[1], ARGV[1], 'KEEPTTL')
redis.call('ZREM', KEYS[2], ARGV[2])
return {'ok'}
`;

/**
 * Release a pending reservation: the parts go back on the shelf.
 *
 * Key and argument layout matches the commit script, and the order index is dropped so the
 * same order can be reserved again after a compensation.
 *
 * Returns {"missing"} | {"conflict", status} | {"ok"}
 */
export const RELEASE_SCRIPT = `
local raw = redis.call('GET', KEYS[1])
if not raw then
  return {'missing'}
end
local current = cjson.decode(raw)
if current.status ~= 'pending' then
  return {'conflict', current.status}
end

local n = tonumber(ARGV[3])
for i = 1, n do
  local quantity = tonumber(ARGV[5 + (i - 1) * 2])
  redis.call('INCRBY', KEYS[4 + (i - 1) * 2], quantity)
  redis.call('DECRBY', KEYS[5 + (i - 1) * 2], quantity)
end

redis.call('SET', KEYS[1], ARGV[1], 'KEEPTTL')
redis.call('ZREM', KEYS[2], ARGV[2])
redis.call('DEL', KEYS[3])
return {'ok'}
`;

/**
 * The amulite foundry: top each SKU up towards the cap. A SKU that is already over the cap
 * (the marzlevane bin starts at 6,000) is left alone rather than trimmed.
 *
 * KEYS[i]    stock key for SKU i
 * ARGV[1]    cap
 * ARGV[1+i]  units to add to SKU i
 *
 * Returns the resulting stock level for each SKU, in order.
 */
export const FOUNDRY_SCRIPT = `
local cap = tonumber(ARGV[1])
local out = {}
for i = 1, #KEYS do
  local add = tonumber(ARGV[1 + i])
  local have = tonumber(redis.call('GET', KEYS[i]) or '0')
  local target = have
  if add > 0 and have < cap then
    target = math.min(cap, have + add)
    redis.call('SET', KEYS[i], target)
  end
  table.insert(out, tostring(target))
end
return out
`;
