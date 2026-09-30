/**
 * Order validation and the shapes the processor exchanges with its neighbours.
 * Pure functions, so the rules can be tested without a single network call.
 */

export const MODELS = ['turbo', 'retro', 'hyper'] as const;
export type Model = (typeof MODELS)[number];

export const MIN_QUANTITY = 1;
export const MAX_QUANTITY = 10;
/** Design section 3: the third failed inspection scraps the order. */
export const MAX_ATTEMPTS = 3;

export interface OrderOptions {
  dingleArm: boolean;
}

export interface OrderRequest {
  model: Model;
  quantity: number;
  options: OrderOptions;
}

export class InvalidOrderError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'InvalidOrderError';
  }
}

export function isModel(value: unknown): value is Model {
  return typeof value === 'string' && (MODELS as readonly string[]).includes(value);
}

/**
 * Validate and normalise an inbound order body. The customer comes from
 * `X-Encabulator-Subject`, never from the body.
 */
export function parseOrderRequest(body: unknown): OrderRequest {
  if (typeof body !== 'object' || body === null) {
    throw new InvalidOrderError('body must be an object');
  }
  const raw = body as { model?: unknown; quantity?: unknown; options?: { dingleArm?: unknown } };

  if (!isModel(raw.model)) {
    throw new InvalidOrderError(`model must be one of ${MODELS.join(', ')}`);
  }
  if (
    typeof raw.quantity !== 'number' ||
    !Number.isInteger(raw.quantity) ||
    raw.quantity < MIN_QUANTITY ||
    raw.quantity > MAX_QUANTITY
  ) {
    throw new InvalidOrderError(
      `quantity must be an integer between ${MIN_QUANTITY} and ${MAX_QUANTITY}`,
    );
  }
  const dingleArm = raw.options?.dingleArm;
  if (dingleArm !== undefined && typeof dingleArm !== 'boolean') {
    throw new InvalidOrderError('options.dingleArm must be a boolean');
  }

  return {
    model: raw.model,
    quantity: raw.quantity,
    options: { dingleArm: dingleArm === true },
  };
}

/** The next fabrication attempt, or `undefined` when the order must be scrapped. */
export function nextAttempt(attempt: number): number | undefined {
  if (!Number.isInteger(attempt) || attempt < 1) return 1;
  return attempt >= MAX_ATTEMPTS ? undefined : attempt + 1;
}

export interface Finding {
  serial: string;
  stage: string;
  metric: string;
  score: number;
  reason: string;
}

/** A short, human-readable reason for `order.scrapped`. */
export function scrapReason(attempt: number, findings: Finding[] = []): string {
  const serials = new Set(findings.map((f) => f.serial));
  const worst = findings.reduce<Finding | undefined>(
    (acc, f) => (acc === undefined || f.score > acc.score ? f : acc),
    undefined,
  );
  const detail = worst
    ? `worst was ${worst.metric} during ${worst.stage} (z=${worst.score.toFixed(2)})`
    : 'no findings recorded';
  const units = serials.size === 1 ? '1 unit' : `${serials.size} units`;
  return `Side fumbling persisted through ${attempt} attempts in ${units}; ${detail}.`;
}

// ---------------------------------------------------------------------------
// Neighbour contracts
// ---------------------------------------------------------------------------

export interface Quote {
  id: string;
  model: string;
  quantity: number;
  unitPrice: number;
  subtotal: number;
  surcharges: Array<{ code: string; amount: number }>;
  total: number;
  currency: string;
  computedAt: string;
  cached: boolean;
}

export interface Reservation {
  id: string;
  orderId: string;
  items: Array<{ sku: string; quantity: number }>;
  expiresAt: string;
}

export interface LedgerOrder {
  id: string;
  customer: string;
  model: string;
  quantity: number;
  options: OrderOptions;
  status: string;
  stage?: string | null;
  attempt: number;
  quote: { id: string; total: number; currency: string };
  reservationId: string;
  serials?: string[];
  carrier?: string | null;
  trackingNumber?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface InspectionFailed {
  orderId: string;
  attempt: number;
  serials?: string[];
  findings?: Finding[];
}
