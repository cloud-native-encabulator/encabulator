/**
 * The lexicon.
 *
 * The labels on this panel are in the vocabulary of J. H. Quick's 1944 paper on
 * the turbo-encabulator, the 1977 Rockwell film, and the 1988 SANS ICS
 * hyper-encabulator. That is the joke. The tooltip is not a joke: every term
 * here is paired with what the console is actually measuring, so a visitor who
 * hovers learns the real system, and nobody is ever misled by a funny word.
 *
 * Keep both halves honest. If a label's `real` text stops being true because the
 * metric changed, the label is a bug.
 */

export interface Lexeme {
  /** The 1944 term, as printed on the plate. */
  term: string
  /** What the console is measuring, plainly. */
  real: string
  /** Where the number comes from, when that is worth saying. */
  source?: string
}

export const LEXICON: Record<string, Lexeme> = {
  skorMotion: {
    term: 'Fluorescent skor motion',
    real: 'Requests per second',
    source: 'rate(http_server_request_duration_seconds_count[1m]) from Prometheus',
  },
  barescentSkor: {
    term: 'Barescent skor motion',
    real: 'Traffic routed through the edge gateway',
  },
  repleneration: {
    term: 'Sinusoidal repleneration',
    real: 'Per-stage telemetry amplitude, nominally 1.0',
    source: 'sinusoidal-recorder /v1/summary, written by the casing line',
  },
  sideFumbling: {
    term: 'Side fumbling',
    real: 'A telemetry anomaly that marks a unit defective',
    source: 'side-fumbling-detector, rolling z-score over unit readings',
  },
  marzlevanes: {
    term: 'Hydrocoptic marzlevanes',
    real: 'A part: six per encabulator, SKU HYD-MARZ',
  },
  amulite: {
    term: 'Prefabulated amulite',
    real: 'The base-plate part, SKU AMU-BASE',
  },
  waneshaft: {
    term: 'Ambifacient lunar waneshaft',
    real: 'A part: one per encabulator, SKU LUN-WANE',
  },
  dingleArm: {
    term: 'Drawn reciprocation dingle arm',
    real: 'An order option that halves the defect rate, and the name of the edge gateway',
  },
  girdlespring: {
    term: 'Differential girdlespring',
    real: 'Feature flags, polled every five seconds',
  },
  grammeterDrift: {
    term: 'Grammeter drift',
    real: 'Cumulative drift in parts per million, one of three telemetry metrics',
  },
  magnetoReluctance: {
    term: 'Modial interaction of magneto-reluctance',
    real: 'The sine traffic pattern: rate × (1 + 0.8·sin(2πt/period))',
  },
  capacitiveDiractance: {
    term: 'Capacitive diractance',
    real: 'The burst traffic pattern: base rate 80% of the period, five times that for 20%',
  },
  panendermic: {
    term: 'Panendermic semi-boloid slots',
    real: 'Fabrication stage 5, where the lotus-o-delta winding goes in',
  },
  inverseReactive: {
    term: 'Inverse reactive credentials',
    real: 'RS256 JWTs issued by the identity service',
  },
  noferTrunnion: {
    term: 'Nofer trunnion',
    real: 'Shipping: carrier, tracking number, and simulated transit',
  },
  malaise: {
    term: 'Malaise',
    real: 'Injected faults: latency, errors, congestion, CPU burn, pod kills, partitions, scaling',
  },
  encabulation: {
    term: 'Encabulation',
    real: 'One order moving through pricing, reservation, fabrication, inspection and dispatch',
  },
  spurvingBearings: {
    term: 'Spurving bearings',
    real: 'A part: two per encabulator, and the name of the pricing service',
  },
  pentametricFan: {
    term: 'Pentametric fan',
    real: 'A part: one per encabulator, SKU PNT-FAN',
  },
  tremiePipe: {
    term: 'Non-reversible tremie pipe',
    real: 'A part: three per encabulator, and the metaphor for the event bus',
  },
  logarithmicCasing: {
    term: 'Malleable logarithmic casing',
    real: 'A part, and the name of the fabrication workers',
  },
  turboEncabulation: {
    term: 'Turbo encabulation',
    real: 'A flag that halves stage durations and adds a 15% pricing surcharge',
  },
  amuliteFoundry: {
    term: 'The amulite foundry',
    real: 'A flag-controlled restock loop: units of each SKU added every ten seconds',
  },
  prefamulated: {
    term: 'Prefamulated',
    real: "Quick's own coinage; the console uses it for anything pre-provisioned",
  },
}

/** Empty-state copy, in the register of the rest of the panel. */
export const EMPTY_STATES = {
  noEdges:
    'No encabulation in progress. The service graph has reported no calls in the last minute, so no lines are drawn.',
  noPods:
    'The Kubernetes API has told us nothing. Pod dots appear as soon as it does; the plates below are the static registry.',
  noPrometheus:
    'Prometheus is not answering, so every gauge on this plate reads "no data". Nothing here is estimated.',
  noOrders: 'The order book is empty. Place one from the Works, or start the grammeters.',
  noIncidents: 'No side fumbling detected. The detector is watching every unit’s readings.',
  noShipments: 'No nofer trunnions in transit.',
  noMalaises: 'The works are in good health. No malaise has been induced.',
  noScenarios:
    'The scenario library is empty or unreachable. Built-in scenarios ship in the orchestrator image.',
  noFlags: 'The differential girdlespring has reported no flags.',
  noUsers: 'No users returned. Administration requires the admin role.',
  noAudit: 'The audit log is empty.',
  noParts: 'The parts cage is unreadable. Check the depot.',
  guest:
    'You are signed in as a guest, which is to say a viewer: every gauge is live, every control is locked. Sign in as an operator to induce malaise, drive traffic, or flip a flag.',
} as const
