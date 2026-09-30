//! The differential girdlespring flag client (design section 6).
//!
//! > Clients poll `GET /v1/flags` every 5 seconds with `If-None-Match`, keep the last good set,
//! > and fall back to built-in defaults if the service is unreachable at startup.
//!
//! The quoter reads two flags:
//!
//! | Key | Default | Effect |
//! |---|---|---|
//! | `turbo-encabulation` | `false` | adds a 15% turbo surcharge |
//! | `quote-cache` | `true` | cache quotes in Valkey for 60s |
//!
//! Polling is deliberately untraced (design section 7, "Tracing").

use std::collections::HashMap;
use std::sync::{Arc, RwLock};

use serde::{Deserialize, Serialize};
use tokio_util::sync::CancellationToken;

use crate::config::FLAG_POLL_INTERVAL;
use crate::outbound::Outbound;

pub const TURBO_ENCABULATION: &str = "turbo-encabulation";
pub const QUOTE_CACHE: &str = "quote-cache";

/// The girdlespring's flag representation (design section 8.10).
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Flag {
    pub key: String,
    #[serde(default)]
    pub r#type: Option<String>,
    pub value: serde_json::Value,
    #[serde(default)]
    pub rollout_percent: Option<u32>,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(default)]
    pub updated_at: Option<String>,
    #[serde(default)]
    pub updated_by: Option<String>,
}

#[derive(Debug, Deserialize)]
struct FlagsResponse {
    #[serde(default)]
    flags: Vec<Flag>,
}

/// An immutable snapshot of the flag set. Cheap to clone (it is behind an `Arc`), so a request
/// evaluates every flag against one consistent view.
#[derive(Debug, Default)]
pub struct FlagSet {
    by_key: HashMap<String, Flag>,
}

impl FlagSet {
    fn from_flags(flags: Vec<Flag>) -> Self {
        Self {
            by_key: flags.into_iter().map(|f| (f.key.clone(), f)).collect(),
        }
    }

    /// Evaluates a boolean flag for a subject.
    ///
    /// Design section 6: "Boolean flags may also carry `rolloutPercent` (0 to 100). Evaluation for
    /// a subject is `fnv1a32(key + ":" + subject) % 100 < rolloutPercent`. When `rolloutPercent` is
    /// absent, the value applies to everyone."
    #[must_use]
    pub fn bool_flag(&self, key: &str, default: bool, subject: Option<&str>) -> bool {
        let Some(flag) = self.by_key.get(key) else {
            return default;
        };
        match flag.rollout_percent {
            Some(percent) => in_rollout(key, subject.unwrap_or(""), percent),
            None => flag.value.as_bool().unwrap_or(default),
        }
    }

    #[must_use]
    pub fn turbo_encabulation(&self, subject: Option<&str>) -> bool {
        self.bool_flag(TURBO_ENCABULATION, false, subject)
    }

    #[must_use]
    pub fn quote_cache(&self, subject: Option<&str>) -> bool {
        self.bool_flag(QUOTE_CACHE, true, subject)
    }

    #[must_use]
    pub fn len(&self) -> usize {
        self.by_key.len()
    }

    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.by_key.is_empty()
    }
}

/// Whether a subject falls inside a percentage rollout.
#[must_use]
pub fn in_rollout(key: &str, subject: &str, percent: u32) -> bool {
    if percent == 0 {
        return false;
    }
    if percent >= 100 {
        return true;
    }
    fnv1a32(&format!("{key}:{subject}")) % 100 < percent
}

/// 32-bit FNV-1a, as specified in design section 6. Written out rather than pulled from a crate so
/// that every language in the repository can be checked against the same five lines.
#[must_use]
pub fn fnv1a32(input: &str) -> u32 {
    let mut hash: u32 = 0x811c_9dc5;
    for byte in input.as_bytes() {
        hash ^= u32::from(*byte);
        hash = hash.wrapping_mul(0x0100_0193);
    }
    hash
}

/// Holds the current flag snapshot and the ETag that produced it.
pub struct Flags {
    snapshot: RwLock<Arc<FlagSet>>,
    etag: RwLock<Option<String>>,
    url: Option<String>,
    healthy: std::sync::atomic::AtomicBool,
}

impl Flags {
    /// Creates a client. With `url` set to `None` (the `DIFFERENTIAL_GIRDLESPRING_URL` variable
    /// unset) every evaluation returns the built-in default forever.
    #[must_use]
    pub fn new(url: Option<String>) -> Arc<Self> {
        Arc::new(Self {
            snapshot: RwLock::new(Arc::new(FlagSet::default())),
            etag: RwLock::new(None),
            url,
            healthy: std::sync::atomic::AtomicBool::new(false),
        })
    }

    /// The current snapshot. Never blocks for long: the lock is held only for the clone of an
    /// `Arc`.
    #[must_use]
    pub fn snapshot(&self) -> Arc<FlagSet> {
        match self.snapshot.read() {
            Ok(guard) => guard.clone(),
            // A poisoned lock means a panic inside the poller. Defaults are the correct fallback.
            Err(poisoned) => poisoned.into_inner().clone(),
        }
    }

    /// True once at least one poll has succeeded. Informational only: the girdlespring is a soft
    /// dependency and never gates readiness.
    #[must_use]
    pub fn healthy(&self) -> bool {
        self.healthy.load(std::sync::atomic::Ordering::Relaxed)
    }

    #[must_use]
    pub fn enabled(&self) -> bool {
        self.url.is_some()
    }

    /// Replaces the snapshot. Public so tests can drive flag-dependent behaviour without a server.
    pub fn install(&self, flags: Vec<Flag>) {
        let set = Arc::new(FlagSet::from_flags(flags));
        if let Ok(mut guard) = self.snapshot.write() {
            *guard = set;
        }
        self.healthy
            .store(true, std::sync::atomic::Ordering::Relaxed);
    }

    /// Polls `GET /v1/flags` every five seconds until cancelled. Keeps the last good set on any
    /// error and on `304 Not Modified`.
    pub async fn run(self: Arc<Self>, outbound: Outbound, cancel: CancellationToken) {
        let Some(base) = self.url.clone() else {
            tracing::info!("DIFFERENTIAL_GIRDLESPRING_URL is unset; using built-in flag defaults");
            return;
        };
        let url = format!("{base}/v1/flags");
        let mut ticker = tokio::time::interval(FLAG_POLL_INTERVAL);
        ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);

        loop {
            tokio::select! {
                biased;
                () = cancel.cancelled() => return,
                _ = ticker.tick() => self.poll_once(&outbound, &url).await,
            }
        }
    }

    async fn poll_once(&self, outbound: &Outbound, url: &str) {
        let etag = self.etag.read().ok().and_then(|g| g.clone());
        match outbound
            .get(url, "differential-girdlespring", etag.as_deref())
            .await
        {
            Ok(fetched) if fetched.is_not_modified() => {
                self.healthy
                    .store(true, std::sync::atomic::Ordering::Relaxed);
            }
            Ok(fetched) if fetched.status.is_success() => {
                match serde_json::from_slice::<FlagsResponse>(&fetched.body) {
                    Ok(parsed) => {
                        let count = parsed.flags.len();
                        self.install(parsed.flags);
                        if let Ok(mut guard) = self.etag.write() {
                            *guard = fetched.etag;
                        }
                        tracing::debug!(flags = count, "flag snapshot refreshed");
                    }
                    Err(err) => {
                        tracing::warn!(error = %err, "girdlespring returned an unparseable flag set");
                    }
                }
            }
            Ok(fetched) => {
                tracing::warn!(
                    status = fetched.status.as_u16(),
                    "girdlespring flag poll failed"
                );
            }
            Err(err) => {
                tracing::warn!(error = %err, "girdlespring unreachable; keeping the last known flags");
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn flag(key: &str, value: serde_json::Value, rollout: Option<u32>) -> Flag {
        Flag {
            key: key.to_owned(),
            r#type: Some("boolean".to_owned()),
            value,
            rollout_percent: rollout,
            description: None,
            updated_at: None,
            updated_by: None,
        }
    }

    #[test]
    fn missing_flags_fall_back_to_built_in_defaults() {
        let set = FlagSet::default();
        assert!(set.is_empty());
        assert!(!set.turbo_encabulation(None));
        assert!(set.quote_cache(None));
    }

    #[test]
    fn an_explicit_value_applies_to_everyone() {
        let set = FlagSet::from_flags(vec![
            flag(TURBO_ENCABULATION, json!(true), None),
            flag(QUOTE_CACHE, json!(false), None),
        ]);
        assert_eq!(set.len(), 2);
        for subject in ["alice", "bob", ""] {
            assert!(set.turbo_encabulation(Some(subject)));
            assert!(!set.quote_cache(Some(subject)));
        }
    }

    #[test]
    fn a_non_boolean_value_falls_back_to_the_default() {
        let set = FlagSet::from_flags(vec![flag(TURBO_ENCABULATION, json!("yes"), None)]);
        assert!(!set.turbo_encabulation(Some("alice")));
    }

    #[test]
    fn fnv1a32_matches_the_reference_vectors() {
        // The canonical FNV-1a 32-bit test vectors.
        assert_eq!(fnv1a32(""), 0x811c_9dc5);
        assert_eq!(fnv1a32("a"), 0xe40c_292c);
        assert_eq!(fnv1a32("foobar"), 0xbf9c_f968);
    }

    #[test]
    fn rollout_bounds_are_absolute() {
        assert!(!in_rollout(TURBO_ENCABULATION, "alice", 0));
        assert!(in_rollout(TURBO_ENCABULATION, "alice", 100));
        assert!(in_rollout(TURBO_ENCABULATION, "alice", 250));
    }

    #[test]
    fn rollout_is_stable_per_subject_and_spreads_across_subjects() {
        let set = FlagSet::from_flags(vec![flag(TURBO_ENCABULATION, json!(false), Some(50))]);
        // Stable: the same subject always lands on the same side.
        let first = set.turbo_encabulation(Some("customer-7"));
        for _ in 0..10 {
            assert_eq!(set.turbo_encabulation(Some("customer-7")), first);
        }
        // Spread: roughly half of a thousand subjects are inside a 50% rollout.
        let inside = (0..1_000)
            .filter(|i| set.turbo_encabulation(Some(&format!("customer-{i}"))))
            .count();
        assert!(
            (350..=650).contains(&inside),
            "{inside} of 1000 in a 50% rollout"
        );
    }

    #[test]
    fn rollout_overrides_the_stored_value() {
        // rolloutPercent = 100 with value false still evaluates true, per the design's wording.
        let set = FlagSet::from_flags(vec![flag(TURBO_ENCABULATION, json!(false), Some(100))]);
        assert!(set.turbo_encabulation(Some("alice")));
    }

    #[test]
    fn parses_the_girdlespring_response_shape() {
        let body = json!({
            "flags": [
                {"key": "turbo-encabulation", "type": "boolean", "value": true,
                 "rolloutPercent": 25, "description": "d", "updatedAt": "2026-09-30T12:00:00Z",
                 "updatedBy": "operator"},
                {"key": "amulite-foundry-rate", "type": "number", "value": 20}
            ]
        });
        let parsed: FlagsResponse = serde_json::from_value(body).unwrap();
        assert_eq!(parsed.flags.len(), 2);
        assert_eq!(parsed.flags[0].rollout_percent, Some(25));
        assert_eq!(parsed.flags[1].value, json!(20));
    }

    #[test]
    fn install_replaces_the_snapshot_and_marks_healthy() {
        let flags = Flags::new(Some("http://girdlespring:8080".to_owned()));
        assert!(!flags.healthy());
        assert!(flags.enabled());
        assert!(!flags.snapshot().turbo_encabulation(None));
        flags.install(vec![flag(TURBO_ENCABULATION, json!(true), None)]);
        assert!(flags.healthy());
        assert!(flags.snapshot().turbo_encabulation(None));
    }

    #[tokio::test]
    async fn a_disabled_client_returns_immediately() {
        let flags = Flags::new(None);
        assert!(!flags.enabled());
        let outbound = Outbound::new(crate::metrics::Metrics::new()).unwrap();
        // Would hang forever if the poller did not short-circuit on a missing URL.
        tokio::time::timeout(
            std::time::Duration::from_secs(1),
            flags.run(outbound, CancellationToken::new()),
        )
        .await
        .expect("poller returned");
    }

    #[tokio::test]
    async fn an_unreachable_girdlespring_keeps_the_last_good_flags() {
        let metrics = crate::metrics::Metrics::new();
        let outbound = Outbound::new(metrics).unwrap();
        let flags = Flags::new(Some("http://127.0.0.1:1".to_owned()));
        flags.install(vec![flag(TURBO_ENCABULATION, json!(true), None)]);
        flags
            .poll_once(&outbound, "http://127.0.0.1:1/v1/flags")
            .await;
        assert!(
            flags.snapshot().turbo_encabulation(None),
            "kept the last good set"
        );
    }
}
