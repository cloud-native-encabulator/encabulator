/**
 * The parts catalog and the bill of materials (design section 3).
 *
 * Pure functions only: no Valkey, no Fastify. This is the part of the depot that can be
 * reasoned about, and tested, without a container in sight.
 */

export const MODELS = ['turbo', 'retro', 'hyper'] as const;
export type Model = (typeof MODELS)[number];

export const MIN_QUANTITY = 1;
export const MAX_QUANTITY = 10;

export interface Part {
  sku: string;
  name: string;
  /** Units of this SKU consumed per encabulator. */
  perUnit: number;
  /** Only consumed when the order carries this option. */
  requiresOption?: 'dingleArm';
}

export const CATALOG: readonly Part[] = Object.freeze([
  { sku: 'AMU-BASE', name: 'Prefabulated amulite base-plate', perUnit: 1 },
  { sku: 'LOG-CASE', name: 'Malleable logarithmic casing', perUnit: 1 },
  { sku: 'SPV-BRG', name: 'Spurving bearing', perUnit: 2 },
  { sku: 'PNT-FAN', name: 'Pentametric fan', perUnit: 1 },
  { sku: 'HYD-MARZ', name: 'Hydrocoptic marzlevane', perUnit: 6 },
  { sku: 'LUN-WANE', name: 'Ambifacient lunar waneshaft', perUnit: 1 },
  { sku: 'LOT-WIND', name: 'Lotus-o-delta main winding', perUnit: 1 },
  { sku: 'TRM-PIPE', name: 'Non-reversible tremie pipe', perUnit: 3 },
  { sku: 'GRD-SPRG', name: 'Differential girdlespring', perUnit: 1 },
  {
    sku: 'DNG-ARM',
    name: 'Drawn reciprocation dingle arm',
    perUnit: 1,
    requiresOption: 'dingleArm',
  },
]);

export const SKUS: readonly string[] = CATALOG.map((p) => p.sku);

const BY_SKU = new Map(CATALOG.map((p) => [p.sku, p]));

export function findPart(sku: string): Part | undefined {
  return BY_SKU.get(sku);
}

/** Design 8.7: initial stock 1,000 per SKU, 6,000 marzlevanes. */
export const INITIAL_STOCK = 1_000;
/** Design 8.7: the foundry tops each SKU up to this ceiling. */
export const STOCK_CAP = 5_000;
/** The marzlevane is the one SKU the design singles out for a 6x multiplier. */
export const MARZLEVANE_SKU = 'HYD-MARZ';
export const MARZLEVANE_MULTIPLIER = 6;

export function stockMultiplier(sku: string): number {
  return sku === MARZLEVANE_SKU ? MARZLEVANE_MULTIPLIER : 1;
}

export function initialStock(sku: string): number {
  return INITIAL_STOCK * stockMultiplier(sku);
}

/** Units the foundry adds to `sku` for a configured `amulite-foundry-rate`. */
export function foundryIncrement(sku: string, rate: number): number {
  if (rate <= 0) return 0;
  return Math.floor(rate) * stockMultiplier(sku);
}

export interface OrderOptions {
  dingleArm?: boolean;
}

export interface BomItem {
  sku: string;
  quantity: number;
}

export class InvalidReservationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'InvalidReservationError';
  }
}

export function isModel(value: unknown): value is Model {
  return typeof value === 'string' && (MODELS as readonly string[]).includes(value);
}

/**
 * The bill of materials for `quantity` encabulators. The dingle arm is only drawn when
 * asked for; every other part is always fitted.
 */
export function billOfMaterials(
  model: string,
  quantity: number,
  options: OrderOptions = {},
): BomItem[] {
  if (!isModel(model)) {
    throw new InvalidReservationError(`unknown model "${model}"`);
  }
  if (!Number.isInteger(quantity) || quantity < MIN_QUANTITY || quantity > MAX_QUANTITY) {
    throw new InvalidReservationError(
      `quantity must be an integer between ${MIN_QUANTITY} and ${MAX_QUANTITY}`,
    );
  }
  const items: BomItem[] = [];
  for (const part of CATALOG) {
    if (part.requiresOption === 'dingleArm' && !options.dingleArm) continue;
    items.push({ sku: part.sku, quantity: part.perUnit * quantity });
  }
  return items;
}
