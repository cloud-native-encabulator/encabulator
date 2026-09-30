/**
 * Carrier selection and tracking numbers (design 8.5). Pure functions, so the interesting
 * parts are testable without Valkey.
 */

export const CARRIERS = [
  'Trunnion Freight',
  'Waneshaft Express',
  'Grammeter Parcel',
  'Tremie Pipe & Sons',
] as const;

export type Carrier = (typeof CARRIERS)[number];

/** Simulated transit time bounds, in seconds. */
export const MIN_ETA_SECONDS = 10;
export const MAX_ETA_SECONDS = 60;

export type Random = () => number;

export function chooseCarrier(random: Random = Math.random): Carrier {
  const index = Math.min(CARRIERS.length - 1, Math.floor(random() * CARRIERS.length));
  // Non-null: the index is clamped into range above.
  return CARRIERS[index] as Carrier;
}

/** `NT` followed by exactly ten digits. */
export function trackingNumber(random: Random = Math.random): string {
  let digits = '';
  for (let i = 0; i < 10; i++) {
    digits += Math.floor(random() * 10).toString();
  }
  return `NT${digits}`;
}

/** Transit time for a shipment: 10 to 60 seconds inclusive. */
export function etaSeconds(random: Random = Math.random): number {
  const span = MAX_ETA_SECONDS - MIN_ETA_SECONDS;
  return MIN_ETA_SECONDS + Math.floor(random() * (span + 1));
}

export function isCarrier(value: unknown): value is Carrier {
  return typeof value === 'string' && (CARRIERS as readonly string[]).includes(value);
}
