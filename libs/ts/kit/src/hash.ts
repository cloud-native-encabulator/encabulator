/**
 * FNV-1a 32-bit, over UTF-8 bytes so that the Go, Python, Rust and TypeScript kits all
 * bucket a given subject identically (design section 6, flag rollout evaluation).
 */

const FNV_OFFSET_BASIS = 0x811c9dc5;
const FNV_PRIME = 0x01000193;

const encoder = new TextEncoder();

export function fnv1a32(input: string): number {
  const bytes = encoder.encode(input);
  let hash = FNV_OFFSET_BASIS;
  for (const byte of bytes) {
    hash ^= byte;
    hash = Math.imul(hash, FNV_PRIME) >>> 0;
  }
  return hash >>> 0;
}

/**
 * Rollout bucket for a subject: `fnv1a32(key + ":" + subject) % 100`.
 * A subject is in the rollout when its bucket is below `rolloutPercent`.
 */
export function rolloutBucket(key: string, subject: string): number {
  return fnv1a32(`${key}:${subject}`) % 100;
}

export function inRollout(key: string, subject: string, rolloutPercent: number): boolean {
  if (rolloutPercent <= 0) return false;
  if (rolloutPercent >= 100) return true;
  return rolloutBucket(key, subject) < rolloutPercent;
}
