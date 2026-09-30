# Glossary

Every piece of nonsense in this repository maps to a real technical concept. This table gives you
both, plus where the term comes from.

Sources: **Q44** is J. H. Quick, "The Turbo-Encabulator in Industry," *Students' Quarterly Journal*
15(58), 1944. **T46** is *Time* magazine's reader correspondence of April to June 1946. **GE62** is
the General Electric Instrument Department data sheet of 1962. **BH77** is the Bud Haggart film,
circa 1977. **RR** is Mike Kraft's Rockwell "Retro Encabulator." **SANS22** is the SANS ICS
HyperEncabulator, 2022. **New** means we made it up in the same spirit.

## Services

| Name | Source | What it really is |
|---|---|---|
| **encabulation control console** | New | Web UI plus backend-for-frontend: topology visualization and control surface |
| **cardinal grammeter** | Q44 ("differential girdlespring on the 'up' end of the grammeters") | Synthetic load generator |
| **malaise injection engine** | RR ("prevent internal precombobulation") | Chaos engineering: fault injection, both application-level and Kubernetes-level |
| **demo scenario orchestrator** | New | Scripted, narrated demo runner |
| **dingle arm gateway** | T46 (a reader asked "What is a 'dingle arm'?"; the editors answered "employed whenever a barescent skor motion is required") | API gateway: JWT authentication, rate limiting, routing |
| **panametric fam processor** | Q44 as "pentametric fan"; BH77/RR say "panametric fam" | Saga orchestrator for the order workflow |
| **spurving bearing quoter** | Q44 ("two main spurving bearings") | Pricing engine, CPU-bound on purpose |
| **logarithmic casing line** | Q44 ("malleable logarithmic casing") | Worker pool simulating manufacturing, emitting telemetry |
| **nofer trunnion dispatch** | T46 ("For Nofer Trunnions") | Shipping and delivery service |
| **hydrocoptic datastore** | Q44 ("six hydrocoptic marzlevanes") | PostgreSQL-backed order ledger and event timeline |
| **prefabulated amulite depot** | Q44 ("base-plate of prefabulated amulite") | Inventory service with atomic parts reservation |
| **sinusoidal recorder** | BH77 ("reduce sinusoidal repleneration") | Time-series telemetry store on InfluxDB |
| **unilateral phase detractor** | GE62 ("inverse reactive current for use in unilateral phase detractors") | Identity provider: users, passwords, JWT issuance, JWKS |
| **differential girdlespring** | Q44 ("differential girdlespring") | Feature flag service with percentage rollouts |
| **side fumbling detector** | Q44 ("side fumbling was effectively prevented") | Anomaly detection over manufacturing telemetry |

## Domain vocabulary

| Term | Source | What it really is |
|---|---|---|
| **encabulation** | Q44 | A unit of work: one customer order flowing through the system |
| **the Works** | New | The factory as a whole; the system under demonstration |
| **prefabulated amulite** | Q44 | A part SKU (the base plate) |
| **hydrocoptic marzlevane** | Q44 | A part SKU; six per unit, so the first thing to run out of stock |
| **ambifacient lunar waneshaft** | Q44 | A part SKU |
| **lotus-o-delta winding** | Q44 | A part SKU |
| **panendermic semi-boloid slots** | Q44 | Where the winding goes; a fabrication stage |
| **non-reversible tremie pipe** | Q44 | A part SKU; three per unit |
| **drawn reciprocation dingle arm** | BH77 | An optional extra that reduces the defect rate, because it "reduces sinusoidal repleneration" |
| **side fumbling** | Q44 | A manufacturing defect, detected statistically in unit telemetry |
| **sinusoidal repleneration** | BH77 | A telemetry metric: amplitude around 1.0 |
| **grammeter drift** | Q44 | A telemetry metric: cumulative drift in parts per million |
| **marzlevane alignment** | Q44 | A telemetry metric: degrees of misalignment |
| **malaise** | RR | An injected fault |
| **turbo encabulation** | Q44 | A feature flag that halves fabrication time and adds a surcharge |
| **magneto reluctance** | BH77 | Traffic pattern shaping (the sine wave) |
| **capacitive diractance** | BH77 | Traffic burstiness |
| **fluorescent skor motion** | T46 (as "barescent skor motion") | Request rate |
| **modial interaction** | BH77 | The observable effect of load on system behavior |
| **nofer trunnion** | T46 | A shipping carrier |
| **novertrunnion** | BH77 | Anything adjacent to the system that we do not model |
| **prelaminate** | Q44 | The first fabrication stage |
| **spurving** | Q44 | Bearing alignment; the third fabrication stage |
| **pentametric harmonic** | New, from Q44's "pentametric fan" | The deliberately expensive computation in the pricing engine |

## Technical terms, for the reverse lookup

| Real concept | Where it lives here |
|---|---|
| API gateway | dingle arm gateway |
| Saga orchestration with compensation | panametric fam processor |
| Optimistic reservation with rollback | prefabulated amulite depot |
| Event-driven workers | logarithmic casing line |
| Anomaly detection (rolling z-score) | side fumbling detector |
| Feature flags with percentage rollout | differential girdlespring |
| JWT/JWKS identity | unilateral phase detractor |
| Chaos engineering | malaise injection engine |
| Load generation | cardinal grammeter |
| Service graph from distributed tracing | console Factory Floor |
| Consumer-lag autoscaling (KEDA) | logarithmic casing line |
| CPU autoscaling (HPA) | spurving bearing quoter |
| Circuit breaker | panametric fam processor |
| Idempotent event consumption | hydrocoptic datastore |
| Durable delayed jobs | nofer trunnion dispatch |
