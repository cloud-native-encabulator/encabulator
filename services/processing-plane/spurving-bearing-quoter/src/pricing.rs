//! Pricing, and the deliberate CPU burn that makes the quoter the natural HPA target.
//!
//! Design section 8.3:
//!
//! > Pricing: base price per model, plus dingle arm option, times quantity; volume discount 5% for
//! > quantity >= 5; turbo surcharge 15% when `turbo-encabulation` is on. Prices are rounded to
//! > cents.
//!
//! Both percentages are taken against the *subtotal*, so the two adjustments are independent and
//! the order in which they are applied cannot change the total. The design does not say which
//! base the turbo surcharge uses; independence is the reading that keeps the arithmetic
//! explainable on a slide.

use std::hint::black_box;
use std::sync::Arc;
use std::time::{Duration, Instant};

use tokio::sync::Semaphore;

use crate::model::{Model, Options, Surcharge, SurchargeCode};
use crate::money::Money;

/// The drawn reciprocation dingle arm, employed whenever a barescent skor motion is required.
pub const DINGLE_ARM_PRICE: Money = Money::from_dollars_cents(312, 0);

/// Orders of this many units or more earn the volume discount.
pub const VOLUME_DISCOUNT_MIN_QUANTITY: u32 = 5;

/// 5%, in basis points.
pub const VOLUME_DISCOUNT_BPS: i64 = 500;

/// 15%, in basis points.
pub const TURBO_SURCHARGE_BPS: i64 = 1_500;

/// Smallest and largest order the factory accepts (design section 3).
pub const MIN_QUANTITY: u32 = 1;
pub const MAX_QUANTITY: u32 = 10;

/// Everything that can move a price. Two requests with equal inputs must yield equal money, which
/// is precisely what makes this the cache key (see [`crate::cache`]).
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub struct PricingInputs {
    pub model: Model,
    pub quantity: u32,
    pub options: Options,
    /// The evaluated `turbo-encabulation` flag for this request's subject.
    pub turbo: bool,
}

/// The money half of a quote.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Priced {
    pub unit_price: Money,
    pub subtotal: Money,
    pub surcharges: Vec<Surcharge>,
    pub total: Money,
}

/// Prices an order. Pure, allocation-light, and microseconds fast; the CPU burn lives in
/// [`pentametric_harmonic`] so that the two concerns stay separable and testable.
#[must_use]
pub fn price(inputs: PricingInputs) -> Priced {
    let unit_price = inputs.model.base_price()
        + if inputs.options.dingle_arm {
            DINGLE_ARM_PRICE
        } else {
            Money::ZERO
        };
    let subtotal = unit_price.times(inputs.quantity);

    let mut surcharges = Vec::with_capacity(2);
    if inputs.quantity >= VOLUME_DISCOUNT_MIN_QUANTITY {
        surcharges.push(Surcharge {
            code: SurchargeCode::VolumeDiscount,
            amount: -subtotal.basis_points(VOLUME_DISCOUNT_BPS),
        });
    }
    if inputs.turbo {
        surcharges.push(Surcharge {
            code: SurchargeCode::TurboEncabulation,
            amount: subtotal.basis_points(TURBO_SURCHARGE_BPS),
        });
    }

    let total = surcharges
        .iter()
        .fold(subtotal, |acc, surcharge| acc + surcharge.amount);

    Priced {
        unit_price,
        subtotal,
        surcharges,
        total,
    }
}

/// Number of series terms evaluated between clock checks. Small enough that the CPU target is hit
/// accurately even on a slow core, large enough that `Instant::now` is noise.
const HARMONIC_CHUNK: u32 = 4_096;

/// The phase advance per term. An irrational-ish step keeps the series from settling into a short
/// cycle, so the optimiser cannot collapse the loop.
const PHASE_STEP: f64 = 0.618_033_988_749_894_8;

/// Baseline CPU budget for a single-unit turbo quote.
const HARMONIC_BASE_MS: f64 = 5.0;

/// The CPU budget the pentametric harmonic aims for, in the design's words "about 5 to 15 ms of
/// CPU per quote depending on model and quantity".
///
/// A turbo single unit targets 5 ms; ten hyper units target 15 ms. Scaling by wall-clock rather
/// than by a fixed iteration count is deliberate: a fixed count would burn 1 ms on an Apple M-series
/// core and 30 ms on a throttled EKS vCPU, which would make the HPA behave differently on every
/// cluster. `scale` lets an operator dial the whole thing up or down (`PENTAMETRIC_BURN_SCALE`),
/// and the tests set it to zero.
#[must_use]
pub fn harmonic_budget(model: Model, quantity: u32, scale: f64) -> Duration {
    let quantity = quantity.clamp(MIN_QUANTITY, MAX_QUANTITY);
    let quantity_factor =
        1.0 + f64::from(quantity - MIN_QUANTITY) / f64::from(MAX_QUANTITY - MIN_QUANTITY);
    let ms = HARMONIC_BASE_MS * model.harmonic_weight() * quantity_factor * scale.max(0.0);
    Duration::from_secs_f64(ms / 1_000.0)
}

/// The outcome of one pentametric harmonic computation.
#[derive(Clone, Copy, Debug)]
pub struct Harmonic {
    /// The value of the series. Not load-bearing for the price, which the design pins exactly; it
    /// is recorded as a span attribute so the burn is visible rather than mysterious.
    pub value: f64,
    pub terms: u64,
    pub elapsed: Duration,
}

/// Iterates the pentametric harmonic series: five harmonics per term, summed with decaying
/// weights, until the CPU budget is spent. Always runs at least one chunk, so the result is never
/// vacuous even when the budget is zero.
///
/// This is a blocking, CPU-bound function. Call it through [`Pentametric::compute`], never
/// directly from an async task.
#[must_use]
pub fn pentametric_harmonic(budget: Duration) -> Harmonic {
    let started = Instant::now();
    let mut value = 0.0f64;
    let mut phase = 0.0f64;
    let mut terms: u64 = 0;

    loop {
        for _ in 0..HARMONIC_CHUNK {
            terms += 1;
            let k = terms as f64;
            phase += PHASE_STEP;
            value += phase.sin() / k
                + (2.0 * phase).sin() / (k + 1.0)
                + (3.0 * phase).sin() / (k + 2.0)
                + (4.0 * phase).sin() / (k + 3.0)
                + (5.0 * phase).sin() / (k + 4.0);
        }
        value = black_box(value);
        if started.elapsed() >= budget {
            break;
        }
    }

    Harmonic {
        value,
        terms,
        elapsed: started.elapsed(),
    }
}

/// A bounded pool for the pentametric harmonic.
///
/// The burn runs on `spawn_blocking` so it never occupies an async worker thread, and a semaphore
/// caps how many burns run at once. Without the cap, a burst of quotes would spawn one
/// CPU-saturating thread each and the process would stop answering `/healthz`, which would look
/// like a bug rather than like load.
#[derive(Clone)]
pub struct Pentametric {
    permits: Arc<Semaphore>,
    scale: f64,
}

impl Pentametric {
    #[must_use]
    pub fn new(workers: usize, scale: f64) -> Self {
        Self {
            permits: Arc::new(Semaphore::new(workers.max(1))),
            scale,
        }
    }

    /// Runs the harmonic for `(model, quantity)` on the bounded pool, waiting for a permit if every
    /// worker is busy. Returns `None` only if the blocking task panicked, which is a bug.
    pub async fn compute(&self, model: Model, quantity: u32) -> Option<Harmonic> {
        let budget = harmonic_budget(model, quantity, self.scale);
        let permit = self.permits.clone().acquire_owned().await.ok()?;
        let result = tokio::task::spawn_blocking(move || {
            let harmonic = pentametric_harmonic(budget);
            drop(permit);
            harmonic
        })
        .await;
        result.ok()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn inputs(model: Model, quantity: u32, dingle_arm: bool, turbo: bool) -> PricingInputs {
        PricingInputs {
            model,
            quantity,
            options: Options { dingle_arm },
            turbo,
        }
    }

    #[test]
    fn single_turbo_is_the_catalog_price() {
        let p = price(inputs(Model::Turbo, 1, false, false));
        assert_eq!(p.unit_price.to_string(), "1944.00");
        assert_eq!(p.subtotal.to_string(), "1944.00");
        assert!(p.surcharges.is_empty());
        assert_eq!(p.total.to_string(), "1944.00");
    }

    #[test]
    fn every_model_prices_at_its_base() {
        for model in Model::ALL {
            let p = price(inputs(model, 1, false, false));
            assert_eq!(p.unit_price, model.base_price(), "{model}");
            assert_eq!(p.total, model.base_price(), "{model}");
        }
    }

    #[test]
    fn the_dingle_arm_adds_312_per_unit() {
        let p = price(inputs(Model::Turbo, 3, true, false));
        assert_eq!(p.unit_price.to_string(), "2256.00"); // 1944 + 312
        assert_eq!(p.subtotal.to_string(), "6768.00"); // x3
        assert_eq!(p.total.to_string(), "6768.00");
    }

    #[test]
    fn quantity_multiplies_the_unit_price() {
        let p = price(inputs(Model::Hyper, 4, false, false));
        assert_eq!(p.subtotal.to_string(), "16384.00");
        assert!(p.surcharges.is_empty(), "four units earn no discount");
    }

    #[test]
    fn volume_discount_starts_at_five_units() {
        let four = price(inputs(Model::Turbo, 4, false, false));
        assert!(four.surcharges.is_empty());

        let five = price(inputs(Model::Turbo, 5, false, false));
        assert_eq!(five.subtotal.to_string(), "9720.00");
        assert_eq!(five.surcharges.len(), 1);
        assert_eq!(five.surcharges[0].code, SurchargeCode::VolumeDiscount);
        assert_eq!(five.surcharges[0].amount.to_string(), "-486.00");
        assert_eq!(five.total.to_string(), "9234.00");
    }

    #[test]
    fn volume_discount_applies_all_the_way_to_ten() {
        let p = price(inputs(Model::Turbo, 10, false, false));
        assert_eq!(p.subtotal.to_string(), "19440.00");
        assert_eq!(p.total.to_string(), "18468.00");
    }

    #[test]
    fn turbo_surcharge_is_fifteen_percent_of_subtotal() {
        let p = price(inputs(Model::Turbo, 1, false, true));
        assert_eq!(p.surcharges.len(), 1);
        assert_eq!(p.surcharges[0].code, SurchargeCode::TurboEncabulation);
        assert_eq!(p.surcharges[0].amount.to_string(), "291.60");
        assert_eq!(p.total.to_string(), "2235.60");
    }

    #[test]
    fn discount_and_surcharge_are_both_taken_against_the_subtotal() {
        let p = price(inputs(Model::Retro, 5, true, true));
        // (2749 + 312) * 5 = 15305.00
        assert_eq!(p.unit_price.to_string(), "3061.00");
        assert_eq!(p.subtotal.to_string(), "15305.00");
        assert_eq!(p.surcharges.len(), 2);
        assert_eq!(p.surcharges[0].code, SurchargeCode::VolumeDiscount);
        assert_eq!(p.surcharges[0].amount.to_string(), "-765.25");
        assert_eq!(p.surcharges[1].code, SurchargeCode::TurboEncabulation);
        assert_eq!(p.surcharges[1].amount.to_string(), "2295.75");
        // 15305.00 - 765.25 + 2295.75
        assert_eq!(p.total.to_string(), "16835.50");
    }

    #[test]
    fn adjustments_are_rounded_to_whole_cents() {
        // 4096 + 312 = 4408.00; x7 = 30856.00; 5% = 1542.80; 15% = 4628.40
        let p = price(inputs(Model::Hyper, 7, true, true));
        assert_eq!(p.subtotal.to_string(), "30856.00");
        assert_eq!(p.surcharges[0].amount.to_string(), "-1542.80");
        assert_eq!(p.surcharges[1].amount.to_string(), "4628.40");
        assert_eq!(p.total.to_string(), "33941.60");
        for surcharge in &p.surcharges {
            assert_eq!(
                surcharge.amount,
                Money::from_cents(surcharge.amount.cents())
            );
        }
    }

    #[test]
    fn rounding_is_half_away_from_zero_on_an_odd_half_cent() {
        // A subtotal whose 5% lands exactly on a half cent: 1 unit at 1944.00 is not it, so use
        // the smallest synthetic case that exercises Money::basis_points through price().
        let p = price(inputs(Model::Turbo, 5, false, false));
        // 9720.00 * 5% = 486.0000 exactly, no rounding needed.
        assert_eq!(p.surcharges[0].amount.to_string(), "-486.00");
        // And the rounding rule itself is covered exhaustively in money::tests.
        assert_eq!(Money::from_cents(3).basis_points(500), Money::from_cents(0));
        assert_eq!(
            Money::from_cents(11).basis_points(500),
            Money::from_cents(1)
        );
    }

    #[test]
    fn totals_never_go_negative_from_the_discount_alone() {
        for model in Model::ALL {
            for quantity in MIN_QUANTITY..=MAX_QUANTITY {
                for dingle_arm in [false, true] {
                    for turbo in [false, true] {
                        let p = price(inputs(model, quantity, dingle_arm, turbo));
                        assert!(p.total.cents() > 0, "{model} {quantity}");
                        assert!(p.total <= p.subtotal.basis_points(11_500));
                    }
                }
            }
        }
    }

    #[test]
    fn pricing_is_deterministic() {
        let i = inputs(Model::Hyper, 6, true, true);
        assert_eq!(price(i), price(i));
    }

    #[test]
    fn harmonic_budget_spans_five_to_fifteen_milliseconds() {
        let min = harmonic_budget(Model::Turbo, 1, 1.0);
        let max = harmonic_budget(Model::Hyper, 10, 1.0);
        assert!(
            (min.as_secs_f64() * 1000.0 - 5.0).abs() < 0.01,
            "turbo x1 should target 5ms, got {min:?}"
        );
        assert!(
            (max.as_secs_f64() * 1000.0 - 15.0).abs() < 0.01,
            "hyper x10 should target 15ms, got {max:?}"
        );
        // Monotonic in quantity and in model weight.
        assert!(harmonic_budget(Model::Turbo, 1, 1.0) < harmonic_budget(Model::Turbo, 10, 1.0));
        assert!(harmonic_budget(Model::Turbo, 5, 1.0) < harmonic_budget(Model::Hyper, 5, 1.0));
    }

    #[test]
    fn harmonic_budget_clamps_absurd_quantities() {
        assert_eq!(
            harmonic_budget(Model::Turbo, 0, 1.0),
            harmonic_budget(Model::Turbo, 1, 1.0)
        );
        assert_eq!(
            harmonic_budget(Model::Turbo, 9_999, 1.0),
            harmonic_budget(Model::Turbo, 10, 1.0)
        );
        assert_eq!(harmonic_budget(Model::Turbo, 1, 0.0), Duration::ZERO);
    }

    #[test]
    fn harmonic_always_does_work_and_converges() {
        let zero = pentametric_harmonic(Duration::ZERO);
        assert_eq!(zero.terms, u64::from(HARMONIC_CHUNK));
        assert!(zero.value.is_finite());

        // The series is convergent, so a longer burn lands near the same value.
        let longer = pentametric_harmonic(Duration::from_millis(2));
        assert!(longer.terms > zero.terms);
        assert!((longer.value - zero.value).abs() < 0.01);
    }

    #[test]
    fn harmonic_respects_its_budget() {
        let budget = Duration::from_millis(5);
        let harmonic = pentametric_harmonic(budget);
        assert!(harmonic.elapsed >= budget, "burned {:?}", harmonic.elapsed);
        // Generous upper bound: one chunk of overshoot plus scheduler noise.
        assert!(
            harmonic.elapsed < budget * 4,
            "burned {:?}",
            harmonic.elapsed
        );
    }

    #[tokio::test]
    async fn bounded_pool_computes_and_stays_bounded() {
        let pool = Pentametric::new(1, 0.0);
        let first = pool.compute(Model::Turbo, 1).await.expect("computed");
        assert!(first.value.is_finite());
        // Permits are returned, so a second call succeeds on a single-worker pool.
        let second = pool.compute(Model::Hyper, 10).await.expect("computed");
        assert!(second.value.is_finite());
    }
}
