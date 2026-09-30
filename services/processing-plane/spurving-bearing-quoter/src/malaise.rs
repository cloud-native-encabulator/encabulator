//! The malaise (fault injection) middleware (design section 7, "Malaise").
//!
//! > Poll `GET ${MALAISE_ENGINE_URL}/v1/directives?service=${OTEL_SERVICE_NAME}` every 2s,
//! > untraced. If `MALAISE_ENGINE_URL` is unset, the middleware is disabled. If the engine is
//! > unreachable, keep applying the last directives until each one's `expiresAt`.
//!
//! Four kinds, applied to every inbound request except health and metrics:
//!
//! - `latency`: for `percent`% of requests, sleep `ms ± jitterMs` before the handler.
//! - `error`: for `percent`% of requests, respond with `status` and `malaise_injected`.
//! - `congestion`: at most `maxConcurrent` in the handler at once; others wait up to
//!   `queueTimeoutMs`, then get 503 `congested`.
//! - `cpu-burn`: a background worker burns roughly `millicores` of CPU until the directive expires.
//!
//! Order of application is congestion, then latency, then error: a congested quoter still queues,
//! a slow quoter still fails slowly, and a request rejected by `error` has already paid the
//! latency cost, which is what a real failing dependency looks like.

use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex, RwLock};
use std::time::{Duration, Instant};

use axum::extract::{Request, State};
use axum::middleware::Next;
use axum::response::{IntoResponse, Response};
use chrono::{DateTime, Utc};
use http::StatusCode;
use serde::{Deserialize, Serialize};
use tokio::sync::{OwnedSemaphorePermit, Semaphore};
use tokio_util::sync::CancellationToken;

use crate::config::MALAISE_POLL_INTERVAL;
use crate::error::ApiError;
use crate::metrics::Metrics;
use crate::outbound::Outbound;

/// The four app-level fault kinds. Anything the engine adds later deserializes to `Unknown` and is
/// ignored rather than crashing the poller.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub enum Kind {
    #[serde(rename = "latency")]
    Latency,
    #[serde(rename = "error")]
    Error,
    #[serde(rename = "congestion")]
    Congestion,
    #[serde(rename = "cpu-burn")]
    CpuBurn,
    #[serde(other)]
    Unknown,
}

impl Kind {
    /// The `kind` label value used in `encabulator_malaise_applied_total`.
    #[must_use]
    pub const fn label(self) -> &'static str {
        match self {
            Self::Latency => "latency",
            Self::Error => "error",
            Self::Congestion => "congestion",
            Self::CpuBurn => "cpu-burn",
            Self::Unknown => "unknown",
        }
    }
}

/// Every parameter any kind can carry. The engine sends only the ones its kind needs.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Params {
    #[serde(default)]
    pub ms: Option<u64>,
    #[serde(default)]
    pub jitter_ms: Option<u64>,
    #[serde(default)]
    pub percent: Option<u32>,
    #[serde(default)]
    pub status: Option<u16>,
    #[serde(default)]
    pub max_concurrent: Option<usize>,
    #[serde(default)]
    pub queue_timeout_ms: Option<u64>,
    #[serde(default)]
    pub millicores: Option<u32>,
}

impl Params {
    /// Percentage of requests a directive applies to. Absent means all of them.
    #[must_use]
    pub fn percent(&self) -> u32 {
        self.percent.unwrap_or(100).min(100)
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Directive {
    pub id: String,
    pub kind: Kind,
    #[serde(default)]
    pub params: Params,
    pub expires_at: DateTime<Utc>,
}

impl Directive {
    #[must_use]
    pub fn is_active_at(&self, now: DateTime<Utc>) -> bool {
        self.expires_at > now
    }
}

#[derive(Debug, Deserialize)]
struct DirectivesResponse {
    #[serde(default)]
    directives: Vec<Directive>,
}

/// What the middleware decided to do with one request. Separating the decision from the effect is
/// what makes the middleware testable without a clock or a server.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Plan {
    /// Sleep this long before the handler.
    pub latency: Option<Duration>,
    /// Short-circuit with this status instead of calling the handler.
    pub error: Option<StatusCode>,
    /// Admit at most this many requests concurrently, queueing for at most this long.
    pub congestion: Option<(usize, Duration)>,
}

impl Plan {
    #[must_use]
    pub fn is_empty(&self) -> bool {
        *self == Self::default()
    }
}

/// Default queue timeout when a congestion directive omits `queueTimeoutMs`.
const DEFAULT_QUEUE_TIMEOUT: Duration = Duration::from_millis(2_000);

/// Builds the plan for one request.
///
/// `roll` returns a value in `0..100`; it is a parameter so tests can be deterministic. A directive
/// with `percent` p applies when `roll() < p`, so `percent: 0` never applies and `percent: 100`
/// always does.
pub fn plan(directives: &[Directive], now: DateTime<Utc>, roll: &mut dyn FnMut() -> u32) -> Plan {
    let mut plan = Plan::default();
    for directive in directives.iter().filter(|d| d.is_active_at(now)) {
        match directive.kind {
            Kind::Latency => {
                if roll() < directive.params.percent() {
                    let base = directive.params.ms.unwrap_or(0);
                    let jitter = directive.params.jitter_ms.unwrap_or(0);
                    // ms +/- jitterMs, clamped at zero.
                    let offset = if jitter == 0 {
                        0
                    } else {
                        roll_jitter(jitter, roll)
                    };
                    let millis = base.saturating_add_signed(offset);
                    let latency = Duration::from_millis(millis);
                    plan.latency =
                        Some(plan.latency.map_or(latency, |existing| existing + latency));
                }
            }
            Kind::Error => {
                if roll() < directive.params.percent() {
                    let status = directive
                        .params
                        .status
                        .and_then(|s| StatusCode::from_u16(s).ok())
                        .unwrap_or(StatusCode::SERVICE_UNAVAILABLE);
                    plan.error = Some(status);
                }
            }
            Kind::Congestion => {
                let max_concurrent = directive.params.max_concurrent.unwrap_or(1).max(1);
                let queue_timeout = directive
                    .params
                    .queue_timeout_ms
                    .map_or(DEFAULT_QUEUE_TIMEOUT, Duration::from_millis);
                // The tightest congestion directive wins.
                let tighter = plan
                    .congestion
                    .is_none_or(|(existing, _)| max_concurrent < existing);
                if tighter {
                    plan.congestion = Some((max_concurrent, queue_timeout));
                }
            }
            // A background worker handles cpu-burn; it is not per-request.
            Kind::CpuBurn | Kind::Unknown => {}
        }
    }
    plan
}

/// Draws a symmetric jitter offset in `[-jitter, +jitter]` from a 0..100 roll.
fn roll_jitter(jitter: u64, roll: &mut dyn FnMut() -> u32) -> i64 {
    let draw = i64::from(roll().min(99));
    // Map 0..=99 onto -jitter..=+jitter.
    let span = 2 * jitter as i64;
    (draw * span) / 99 - jitter as i64
}

/// A congestion gate: a semaphore sized to the directive that created it.
struct Gate {
    max_concurrent: usize,
    semaphore: Arc<Semaphore>,
}

/// One running CPU burner.
struct Burner {
    millicores: u32,
    stop: Arc<AtomicBool>,
}

impl Drop for Burner {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Relaxed);
    }
}

pub struct Malaise {
    url: Option<String>,
    service: String,
    directives: RwLock<Arc<Vec<Directive>>>,
    gate: Mutex<Option<Gate>>,
    burners: Mutex<HashMap<String, Burner>>,
    metrics: Arc<Metrics>,
}

impl Malaise {
    /// Creates the middleware state. With `url` set to `None` (`MALAISE_ENGINE_URL` unset) the
    /// middleware is inert: [`Malaise::enabled`] is false and no request is ever touched.
    #[must_use]
    pub fn new(url: Option<String>, service: String, metrics: Arc<Metrics>) -> Arc<Self> {
        Arc::new(Self {
            url,
            service,
            directives: RwLock::new(Arc::new(Vec::new())),
            gate: Mutex::new(None),
            burners: Mutex::new(HashMap::new()),
            metrics,
        })
    }

    #[must_use]
    pub fn enabled(&self) -> bool {
        self.url.is_some()
    }

    #[must_use]
    pub fn directives(&self) -> Arc<Vec<Directive>> {
        match self.directives.read() {
            Ok(guard) => guard.clone(),
            Err(poisoned) => poisoned.into_inner().clone(),
        }
    }

    /// Installs a directive set and reconciles the CPU burners. Public so tests can inject
    /// directives without an engine.
    pub fn install(&self, directives: Vec<Directive>) {
        let now = Utc::now();
        let active: Vec<Directive> = directives
            .into_iter()
            .filter(|d| d.is_active_at(now) && d.kind != Kind::Unknown)
            .collect();
        self.reconcile_burners(&active);
        if let Ok(mut guard) = self.directives.write() {
            *guard = Arc::new(active);
        }
    }

    /// Drops any directive that has passed its `expiresAt`. Called when the engine is unreachable,
    /// so a fault still ends on schedule even if nobody can be asked about it.
    pub fn expire(&self) {
        let now = Utc::now();
        let current = self.directives();
        if current.iter().all(|d| d.is_active_at(now)) {
            return;
        }
        let remaining: Vec<Directive> = current
            .iter()
            .filter(|d| d.is_active_at(now))
            .cloned()
            .collect();
        self.reconcile_burners(&remaining);
        if let Ok(mut guard) = self.directives.write() {
            *guard = Arc::new(remaining);
        }
    }

    /// Starts burners for new `cpu-burn` directives and stops burners whose directive is gone or
    /// whose `millicores` changed.
    fn reconcile_burners(&self, active: &[Directive]) {
        let wanted: HashMap<&str, u32> = active
            .iter()
            .filter(|d| d.kind == Kind::CpuBurn)
            .map(|d| (d.id.as_str(), d.params.millicores.unwrap_or(0)))
            .filter(|(_, millicores)| *millicores > 0)
            .collect();

        let Ok(mut burners) = self.burners.lock() else {
            return;
        };

        // Dropping a Burner sets its stop flag.
        burners.retain(|id, burner| {
            wanted
                .get(id.as_str())
                .is_some_and(|millicores| *millicores == burner.millicores)
        });

        for (id, millicores) in wanted {
            if burners.contains_key(id) {
                continue;
            }
            let stop = Arc::new(AtomicBool::new(false));
            spawn_burn(millicores, stop.clone());
            self.metrics.record_malaise(Kind::CpuBurn.label());
            tracing::warn!(
                malaise_id = id,
                millicores,
                "burning CPU on behalf of the malaise injection engine"
            );
            burners.insert(id.to_owned(), Burner { millicores, stop });
        }
    }

    /// Returns the semaphore for a congestion limit, creating or resizing it as needed.
    fn gate(&self, max_concurrent: usize) -> Option<Arc<Semaphore>> {
        let mut guard = self.gate.lock().ok()?;
        match guard.as_ref() {
            Some(gate) if gate.max_concurrent == max_concurrent => Some(gate.semaphore.clone()),
            _ => {
                let semaphore = Arc::new(Semaphore::new(max_concurrent));
                *guard = Some(Gate {
                    max_concurrent,
                    semaphore: semaphore.clone(),
                });
                Some(semaphore)
            }
        }
    }

    /// Builds the plan for one request using the process RNG.
    fn plan_now(&self) -> Plan {
        let directives = self.directives();
        if directives.is_empty() {
            return Plan::default();
        }
        let mut roll = || fastrand::u32(0..100);
        plan(&directives, Utc::now(), &mut roll)
    }

    /// Acquires a congestion permit, or returns the 503 the design specifies.
    async fn admit(
        &self,
        max_concurrent: usize,
        queue_timeout: Duration,
    ) -> Result<Option<OwnedSemaphorePermit>, ApiError> {
        let Some(semaphore) = self.gate(max_concurrent) else {
            return Ok(None);
        };
        self.metrics.record_malaise(Kind::Congestion.label());
        match tokio::time::timeout(queue_timeout, semaphore.acquire_owned()).await {
            Ok(Ok(permit)) => Ok(Some(permit)),
            // The semaphore is never closed, so this arm is unreachable in practice.
            Ok(Err(_)) => Ok(None),
            Err(_) => Err(ApiError::congested()),
        }
    }

    /// Polls the engine every two seconds until cancelled. Untraced, as the design requires.
    pub async fn run(self: Arc<Self>, outbound: Outbound, cancel: CancellationToken) {
        let Some(base) = self.url.clone() else {
            tracing::info!("MALAISE_ENGINE_URL is unset; the malaise middleware is disabled");
            return;
        };
        let url = format!("{base}/v1/directives?service={}", urlencode(&self.service));
        let mut ticker = tokio::time::interval(MALAISE_POLL_INTERVAL);
        ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);

        loop {
            tokio::select! {
                biased;
                () = cancel.cancelled() => {
                    // Stop every burner on the way out.
                    self.install(Vec::new());
                    return;
                }
                _ = ticker.tick() => self.poll_once(&outbound, &url).await,
            }
        }
    }

    async fn poll_once(&self, outbound: &Outbound, url: &str) {
        match outbound
            .get_once(url, "malaise-injection-engine", None)
            .await
        {
            Ok(fetched) if fetched.status.is_success() => {
                match serde_json::from_slice::<DirectivesResponse>(&fetched.body) {
                    Ok(parsed) => {
                        let count = parsed.directives.len();
                        self.install(parsed.directives);
                        if count > 0 {
                            tracing::debug!(directives = count, "malaise directives refreshed");
                        }
                    }
                    Err(err) => {
                        tracing::warn!(error = %err, "malaise engine returned unparseable directives");
                        self.expire();
                    }
                }
            }
            Ok(fetched) => {
                tracing::warn!(
                    status = fetched.status.as_u16(),
                    "malaise directive poll failed"
                );
                self.expire();
            }
            Err(_) => {
                // Keep applying the last directives until each one's expiresAt.
                self.expire();
            }
        }
    }
}

/// Percent-encodes the few characters a service name could plausibly contain. Service names in the
/// catalog are `[a-z-]+`, so this is belt and braces.
fn urlencode(input: &str) -> String {
    let mut out = String::with_capacity(input.len());
    for byte in input.bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(byte as char);
            }
            other => out.push_str(&format!("%{other:02X}")),
        }
    }
    out
}

/// Spawns dedicated OS threads that busy-spin on a duty cycle, approximating `millicores` of CPU.
///
/// Dedicated threads rather than `spawn_blocking`: a burner runs until its directive expires, and
/// parking a tokio blocking worker for minutes at a time would starve the pentametric harmonic.
fn spawn_burn(millicores: u32, stop: Arc<AtomicBool>) {
    let full_cores = millicores / 1_000;
    let remainder = millicores % 1_000;
    let mut duties: Vec<f64> = vec![1.0; full_cores as usize];
    if remainder > 0 {
        duties.push(f64::from(remainder) / 1_000.0);
    }

    for (index, duty) in duties.into_iter().enumerate() {
        let stop = stop.clone();
        let _ = std::thread::Builder::new()
            .name(format!("malaise-cpu-burn-{index}"))
            .spawn(move || burn(&stop, duty));
    }
}

/// One burner thread: spin for `duty` of each slice, sleep the rest.
fn burn(stop: &AtomicBool, duty: f64) {
    const SLICE: Duration = Duration::from_millis(50);
    let busy = SLICE.mul_f64(duty.clamp(0.0, 1.0));
    let idle = SLICE.saturating_sub(busy);
    let mut spinner = 0.0f64;
    let mut n = 0u64;

    while !stop.load(Ordering::Relaxed) {
        let started = Instant::now();
        while started.elapsed() < busy {
            for _ in 0..1_024 {
                n = n.wrapping_add(1);
                spinner += (n as f64).sqrt();
            }
            spinner = std::hint::black_box(spinner);
            if stop.load(Ordering::Relaxed) {
                return;
            }
        }
        if !idle.is_zero() {
            std::thread::sleep(idle);
        }
    }
}

/// The axum middleware. Layered only onto the API router, so `/healthz`, `/readyz`, and `/metrics`
/// are never affected (design section 7: malaise applies to inbound requests "excluding health and
/// metrics").
pub async fn middleware(
    State(malaise): State<Arc<Malaise>>,
    request: Request,
    next: Next,
) -> Response {
    let plan = malaise.plan_now();
    if plan.is_empty() {
        return next.run(request).await;
    }

    let _permit = match plan.congestion {
        Some((max_concurrent, queue_timeout)) => {
            match malaise.admit(max_concurrent, queue_timeout).await {
                Ok(permit) => permit,
                Err(err) => return err.into_response(),
            }
        }
        None => None,
    };

    if let Some(latency) = plan.latency {
        malaise.metrics.record_malaise(Kind::Latency.label());
        tokio::time::sleep(latency).await;
    }

    if let Some(status) = plan.error {
        malaise.metrics.record_malaise(Kind::Error.label());
        return ApiError::malaise_injected(status).into_response();
    }

    next.run(request).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeDelta;

    fn directive(kind: Kind, params: Params, seconds_from_now: i64) -> Directive {
        Directive {
            id: format!("mal_{}", kind.label()),
            kind,
            params,
            expires_at: Utc::now() + TimeDelta::seconds(seconds_from_now),
        }
    }

    /// A roll sequence that returns each value in turn, then repeats the last.
    fn rolls(values: Vec<u32>) -> impl FnMut() -> u32 {
        let mut index = 0usize;
        move || {
            let value = values[index.min(values.len() - 1)];
            index += 1;
            value
        }
    }

    #[test]
    fn parses_the_design_s_directive_payload() {
        let body = serde_json::json!({
            "directives": [
                {"id": "mal_1", "kind": "latency",
                 "params": {"ms": 400, "jitterMs": 100, "percent": 100},
                 "expiresAt": "2026-09-30T12:02:00.000Z"},
                {"id": "mal_2", "kind": "error", "params": {"status": 503, "percent": 25},
                 "expiresAt": "2026-09-30T12:02:00.000Z"},
                {"id": "mal_3", "kind": "congestion",
                 "params": {"maxConcurrent": 2, "queueTimeoutMs": 2000},
                 "expiresAt": "2026-09-30T12:02:00.000Z"},
                {"id": "mal_4", "kind": "cpu-burn", "params": {"millicores": 500},
                 "expiresAt": "2026-09-30T12:02:00.000Z"}
            ]
        });
        let parsed: DirectivesResponse = serde_json::from_value(body).unwrap();
        assert_eq!(parsed.directives.len(), 4);
        assert_eq!(parsed.directives[0].kind, Kind::Latency);
        assert_eq!(parsed.directives[0].params.ms, Some(400));
        assert_eq!(parsed.directives[0].params.jitter_ms, Some(100));
        assert_eq!(parsed.directives[1].params.status, Some(503));
        assert_eq!(parsed.directives[2].params.max_concurrent, Some(2));
        assert_eq!(parsed.directives[3].kind, Kind::CpuBurn);
        assert_eq!(parsed.directives[3].params.millicores, Some(500));
    }

    #[test]
    fn an_unrecognised_kind_deserializes_to_unknown() {
        let parsed: DirectivesResponse = serde_json::from_value(serde_json::json!({
            "directives": [{"id": "mal_x", "kind": "gravitational-inversion", "params": {},
                            "expiresAt": "2026-09-30T12:02:00.000Z"}]
        }))
        .unwrap();
        assert_eq!(parsed.directives[0].kind, Kind::Unknown);
    }

    #[test]
    fn no_directives_means_an_empty_plan() {
        let mut roll = rolls(vec![0]);
        assert!(plan(&[], Utc::now(), &mut roll).is_empty());
    }

    #[test]
    fn expired_directives_are_ignored() {
        let directives = vec![directive(
            Kind::Error,
            Params {
                status: Some(503),
                ..Params::default()
            },
            -1,
        )];
        let mut roll = rolls(vec![0]);
        assert!(plan(&directives, Utc::now(), &mut roll).is_empty());
    }

    #[test]
    fn latency_sleeps_within_the_jitter_band() {
        let directives = vec![directive(
            Kind::Latency,
            Params {
                ms: Some(400),
                jitter_ms: Some(100),
                percent: Some(100),
                ..Params::default()
            },
            60,
        )];
        // First roll gates on percent, second draws the jitter.
        for jitter_roll in [0u32, 49, 99] {
            let mut roll = rolls(vec![0, jitter_roll]);
            let latency = plan(&directives, Utc::now(), &mut roll).latency.unwrap();
            assert!(
                latency >= Duration::from_millis(300) && latency <= Duration::from_millis(500),
                "{latency:?} outside 400 +/- 100ms"
            );
        }
    }

    #[test]
    fn latency_without_jitter_is_exact() {
        let directives = vec![directive(
            Kind::Latency,
            Params {
                ms: Some(250),
                percent: Some(100),
                ..Params::default()
            },
            60,
        )];
        let mut roll = rolls(vec![0]);
        assert_eq!(
            plan(&directives, Utc::now(), &mut roll).latency,
            Some(Duration::from_millis(250))
        );
    }

    #[test]
    fn percent_gates_the_directive() {
        let directives = vec![directive(
            Kind::Error,
            Params {
                status: Some(503),
                percent: Some(25),
                ..Params::default()
            },
            60,
        )];
        // A roll below 25 applies.
        let mut inside = rolls(vec![24]);
        assert_eq!(
            plan(&directives, Utc::now(), &mut inside).error,
            Some(StatusCode::SERVICE_UNAVAILABLE)
        );
        // A roll at or above 25 does not.
        let mut outside = rolls(vec![25]);
        assert!(plan(&directives, Utc::now(), &mut outside).error.is_none());
    }

    #[test]
    fn percent_zero_never_applies_and_percent_absent_always_does() {
        let never = vec![directive(
            Kind::Error,
            Params {
                percent: Some(0),
                ..Params::default()
            },
            60,
        )];
        let mut roll = rolls(vec![0]);
        assert!(plan(&never, Utc::now(), &mut roll).error.is_none());

        let always = vec![directive(Kind::Error, Params::default(), 60)];
        let mut roll = rolls(vec![99]);
        assert_eq!(
            plan(&always, Utc::now(), &mut roll).error,
            Some(StatusCode::SERVICE_UNAVAILABLE)
        );
    }

    #[test]
    fn an_invalid_error_status_falls_back_to_503() {
        let directives = vec![directive(
            Kind::Error,
            Params {
                status: Some(9_999),
                ..Params::default()
            },
            60,
        )];
        let mut roll = rolls(vec![0]);
        assert_eq!(
            plan(&directives, Utc::now(), &mut roll).error,
            Some(StatusCode::SERVICE_UNAVAILABLE)
        );
    }

    #[test]
    fn congestion_is_not_percent_gated_and_the_tightest_wins() {
        let directives = vec![
            directive(
                Kind::Congestion,
                Params {
                    max_concurrent: Some(8),
                    queue_timeout_ms: Some(1_000),
                    ..Params::default()
                },
                60,
            ),
            directive(
                Kind::Congestion,
                Params {
                    max_concurrent: Some(2),
                    queue_timeout_ms: Some(2_000),
                    ..Params::default()
                },
                60,
            ),
        ];
        let mut roll = rolls(vec![99]);
        assert_eq!(
            plan(&directives, Utc::now(), &mut roll).congestion,
            Some((2, Duration::from_millis(2_000)))
        );
    }

    #[test]
    fn congestion_defaults_are_sane() {
        let directives = vec![directive(Kind::Congestion, Params::default(), 60)];
        let mut roll = rolls(vec![0]);
        assert_eq!(
            plan(&directives, Utc::now(), &mut roll).congestion,
            Some((1, DEFAULT_QUEUE_TIMEOUT))
        );
    }

    #[test]
    fn cpu_burn_never_shows_up_in_a_request_plan() {
        let directives = vec![directive(
            Kind::CpuBurn,
            Params {
                millicores: Some(500),
                ..Params::default()
            },
            60,
        )];
        let mut roll = rolls(vec![0]);
        assert!(plan(&directives, Utc::now(), &mut roll).is_empty());
    }

    #[test]
    fn several_kinds_combine_into_one_plan() {
        let directives = vec![
            directive(
                Kind::Latency,
                Params {
                    ms: Some(100),
                    ..Params::default()
                },
                60,
            ),
            directive(
                Kind::Error,
                Params {
                    status: Some(500),
                    ..Params::default()
                },
                60,
            ),
            directive(
                Kind::Congestion,
                Params {
                    max_concurrent: Some(4),
                    ..Params::default()
                },
                60,
            ),
        ];
        let mut roll = rolls(vec![0]);
        let plan = plan(&directives, Utc::now(), &mut roll);
        assert_eq!(plan.latency, Some(Duration::from_millis(100)));
        assert_eq!(plan.error, Some(StatusCode::INTERNAL_SERVER_ERROR));
        assert_eq!(plan.congestion, Some((4, DEFAULT_QUEUE_TIMEOUT)));
    }

    #[test]
    fn roll_jitter_spans_the_full_symmetric_band() {
        let mut low = rolls(vec![0]);
        let mut mid = rolls(vec![49]);
        let mut high = rolls(vec![99]);
        assert_eq!(roll_jitter(100, &mut low), -100);
        assert_eq!(roll_jitter(100, &mut high), 100);
        let middle = roll_jitter(100, &mut mid);
        assert!((-5..=5).contains(&middle), "{middle}");
    }

    #[test]
    fn urlencode_leaves_service_names_alone() {
        assert_eq!(
            urlencode("spurving-bearing-quoter"),
            "spurving-bearing-quoter"
        );
        assert_eq!(urlencode("a b&c"), "a%20b%26c");
    }

    #[test]
    fn a_disabled_middleware_installs_nothing() {
        let malaise = Malaise::new(None, "quoter".to_owned(), Metrics::new());
        assert!(!malaise.enabled());
        assert!(malaise.directives().is_empty());
        assert!(malaise.plan_now().is_empty());
    }

    #[test]
    fn install_drops_expired_and_unknown_directives() {
        let malaise = Malaise::new(
            Some("http://engine:8080".to_owned()),
            "quoter".to_owned(),
            Metrics::new(),
        );
        malaise.install(vec![
            directive(Kind::Error, Params::default(), 60),
            directive(Kind::Latency, Params::default(), -5),
            directive(Kind::Unknown, Params::default(), 60),
        ]);
        let installed = malaise.directives();
        assert_eq!(installed.len(), 1);
        assert_eq!(installed[0].kind, Kind::Error);
    }

    #[test]
    fn expire_removes_directives_past_their_expiry() {
        let malaise = Malaise::new(
            Some("http://engine:8080".to_owned()),
            "quoter".to_owned(),
            Metrics::new(),
        );
        // install() filters expired directives, so write one in directly to simulate the clock
        // moving past expiresAt between polls.
        {
            let mut guard = malaise.directives.write().unwrap();
            *guard = Arc::new(vec![directive(Kind::Error, Params::default(), -1)]);
        }
        assert_eq!(malaise.directives().len(), 1);
        malaise.expire();
        assert!(malaise.directives().is_empty());
    }

    #[test]
    fn the_gate_is_reused_for_one_limit_and_replaced_for_another() {
        let malaise = Malaise::new(None, "quoter".to_owned(), Metrics::new());
        let first = malaise.gate(2).unwrap();
        let again = malaise.gate(2).unwrap();
        assert!(Arc::ptr_eq(&first, &again));
        let resized = malaise.gate(4).unwrap();
        assert!(!Arc::ptr_eq(&first, &resized));
        assert_eq!(resized.available_permits(), 4);
    }

    #[tokio::test]
    async fn congestion_queues_then_rejects_with_congested() {
        let malaise = Malaise::new(None, "quoter".to_owned(), Metrics::new());
        let held = malaise
            .admit(1, Duration::from_millis(50))
            .await
            .expect("first request admitted");
        assert!(held.is_some());
        let err = malaise
            .admit(1, Duration::from_millis(50))
            .await
            .expect_err("second request should time out in the queue");
        assert_eq!(err.status, StatusCode::SERVICE_UNAVAILABLE);
        assert_eq!(err.code, "congested");
        drop(held);
        // Once the permit is released the next request is admitted again.
        assert!(
            malaise
                .admit(1, Duration::from_millis(50))
                .await
                .unwrap()
                .is_some()
        );
    }

    #[test]
    fn burners_start_stop_and_resize() {
        let malaise = Malaise::new(None, "quoter".to_owned(), Metrics::new());
        let burn = |millicores: u32| Directive {
            id: "mal_burn".to_owned(),
            kind: Kind::CpuBurn,
            params: Params {
                millicores: Some(millicores),
                ..Params::default()
            },
            expires_at: Utc::now() + TimeDelta::seconds(60),
        };

        malaise.install(vec![burn(200)]);
        let stop = {
            let burners = malaise.burners.lock().unwrap();
            assert_eq!(burners.len(), 1);
            burners["mal_burn"].stop.clone()
        };
        assert!(!stop.load(Ordering::Relaxed));

        // Changing millicores replaces the burner.
        malaise.install(vec![burn(700)]);
        assert!(stop.load(Ordering::Relaxed), "old burner was told to stop");
        let resized = {
            let burners = malaise.burners.lock().unwrap();
            assert_eq!(burners.len(), 1);
            assert_eq!(burners["mal_burn"].millicores, 700);
            burners["mal_burn"].stop.clone()
        };

        // An empty directive set stops everything.
        malaise.install(Vec::new());
        assert!(resized.load(Ordering::Relaxed));
        assert!(malaise.burners.lock().unwrap().is_empty());
        assert!(malaise.metrics.encode().contains("kind=\"cpu-burn\""));
    }

    #[test]
    fn a_zero_millicore_burn_starts_no_thread() {
        let malaise = Malaise::new(None, "quoter".to_owned(), Metrics::new());
        malaise.install(vec![Directive {
            id: "mal_zero".to_owned(),
            kind: Kind::CpuBurn,
            params: Params {
                millicores: Some(0),
                ..Params::default()
            },
            expires_at: Utc::now() + TimeDelta::seconds(60),
        }]);
        assert!(malaise.burners.lock().unwrap().is_empty());
    }

    #[tokio::test]
    async fn an_unreachable_engine_keeps_unexpired_directives() {
        let metrics = Metrics::new();
        let outbound = Outbound::new(metrics).unwrap();
        let malaise = Malaise::new(
            Some("http://127.0.0.1:1".to_owned()),
            "quoter".to_owned(),
            Metrics::new(),
        );
        malaise.install(vec![directive(Kind::Error, Params::default(), 60)]);
        malaise
            .poll_once(&outbound, "http://127.0.0.1:1/v1/directives?service=quoter")
            .await;
        assert_eq!(
            malaise.directives().len(),
            1,
            "last directives still applied"
        );
    }

    #[tokio::test]
    async fn a_disabled_poller_returns_immediately() {
        let malaise = Malaise::new(None, "quoter".to_owned(), Metrics::new());
        let outbound = Outbound::new(Metrics::new()).unwrap();
        tokio::time::timeout(
            Duration::from_secs(1),
            malaise.run(outbound, CancellationToken::new()),
        )
        .await
        .expect("poller returned");
    }
}
