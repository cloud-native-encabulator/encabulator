//! The Valkey quote store (design section 8.3).
//!
//! Two key families, both under the `quote:*` prefix the service catalog assigns to the quoter:
//!
//! | Key | TTL | Purpose |
//! |---|---|---|
//! | `quote:{id}` | 1 hour | backs `GET /v1/quotes/{id}` |
//! | `quote:input:{model}:{quantity}:{dingleArm}:{turbo}` | 60 seconds | the `quote-cache` flag's cache |
//!
//! The input key is written in full rather than hashed. A demo is better served by
//! `KEYS quote:input:*` reading like a sentence than by sixteen bytes of SHA-256, and the tuple is
//! small and closed: three models times ten quantities times two options times two flag states.
//!
//! **Valkey is a soft dependency.** Every operation here returns a `Result` that callers log and
//! discard; readiness never depends on it, and a dead Valkey costs a quote nothing but the cache
//! hit. The connection is established lazily so the pod starts even when Valkey does not.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, RwLock};
use std::time::Duration;

use redis::aio::{ConnectionManager, ConnectionManagerConfig};
use tokio_util::sync::CancellationToken;
use tracing::Instrument;

use crate::config::{QUOTE_CACHE_TTL, QUOTE_TTL, VALKEY_TIMEOUT};
use crate::metrics::Metrics;
use crate::model::Quote;
use crate::pricing::PricingInputs;

/// How often to retry the initial connection while Valkey is down.
const RECONNECT_INTERVAL: Duration = Duration::from_secs(5);

/// The cache's view of itself, reported by `/readyz` for operators (never used to gate readiness).
#[derive(Clone, Copy, Debug, PartialEq, Eq, serde::Serialize)]
#[serde(rename_all = "snake_case")]
pub enum CacheState {
    /// `VALKEY_URL` is unset. Quotes are computed every time and `GET /v1/quotes/{id}` 404s.
    Disabled,
    /// Configured but not yet connected.
    Connecting,
    Connected,
}

/// Builds the input cache key for a set of pricing inputs.
///
/// Every input that can move the price is in the key, and nothing else is: two requests that must
/// produce the same money produce the same key, and two that must not, cannot collide.
#[must_use]
pub fn input_key(inputs: &PricingInputs) -> String {
    format!(
        "quote:input:{model}:{quantity}:{dingle_arm}:{turbo}",
        model = inputs.model.id(),
        quantity = inputs.quantity,
        dingle_arm = inputs.options.dingle_arm,
        turbo = inputs.turbo,
    )
}

/// Builds the key a quote is stored under for retrieval by id.
#[must_use]
pub fn quote_key(id: &str) -> String {
    format!("quote:{id}")
}

pub struct QuoteStore {
    client: Option<redis::Client>,
    connection: RwLock<Option<ConnectionManager>>,
    connected: AtomicBool,
    server_address: String,
    server_port: u16,
    db_namespace: String,
    metrics: Arc<Metrics>,
}

impl QuoteStore {
    /// Parses `VALKEY_URL` without touching the network. A `None` url yields a disabled store; an
    /// unparseable url is reported and also yields a disabled store, because a typo in a
    /// ConfigMap should degrade the cache rather than crash-loop the pod.
    #[must_use]
    pub fn new(url: Option<&str>, metrics: Arc<Metrics>) -> Arc<Self> {
        let client = url.and_then(|url| match redis::Client::open(url) {
            Ok(client) => Some(client),
            Err(err) => {
                tracing::error!(error = %err, "VALKEY_URL could not be parsed; the quote cache is disabled");
                None
            }
        });

        let (server_address, server_port, db_namespace) = match client.as_ref() {
            Some(client) => {
                let info = client.get_connection_info();
                let (host, port) = match info.addr() {
                    redis::ConnectionAddr::Tcp(host, port) => (host.clone(), *port),
                    redis::ConnectionAddr::TcpTls { host, port, .. } => (host.clone(), *port),
                    redis::ConnectionAddr::Unix(path) => (path.to_string_lossy().into_owned(), 0),
                    other => (format!("{other:?}"), 0),
                };
                (host, port, info.redis_settings().db().to_string())
            }
            None => (String::new(), 0, String::new()),
        };

        Arc::new(Self {
            client,
            connection: RwLock::new(None),
            connected: AtomicBool::new(false),
            server_address,
            server_port,
            db_namespace,
            metrics,
        })
    }

    #[must_use]
    pub fn enabled(&self) -> bool {
        self.client.is_some()
    }

    #[must_use]
    pub fn state(&self) -> CacheState {
        if !self.enabled() {
            CacheState::Disabled
        } else if self.connected.load(Ordering::Relaxed) {
            CacheState::Connected
        } else {
            CacheState::Connecting
        }
    }

    fn connection(&self) -> Option<ConnectionManager> {
        match self.connection.read() {
            Ok(guard) => guard.clone(),
            Err(poisoned) => poisoned.into_inner().clone(),
        }
    }

    /// Connects, if not already connected. Returns whether a usable connection exists afterwards.
    pub async fn ensure_connected(&self) -> bool {
        if self.connection().is_some() {
            return true;
        }
        let Some(client) = self.client.clone() else {
            return false;
        };

        let config = ConnectionManagerConfig::new()
            .set_connection_timeout(Some(VALKEY_TIMEOUT))
            .set_response_timeout(Some(VALKEY_TIMEOUT))
            .set_number_of_retries(1)
            .set_max_delay(Duration::from_secs(2));

        match ConnectionManager::new_with_config(client, config).await {
            Ok(manager) => {
                if let Ok(mut guard) = self.connection.write() {
                    *guard = Some(manager);
                }
                self.connected.store(true, Ordering::Relaxed);
                tracing::info!(
                    server.address = %self.server_address,
                    server.port = self.server_port,
                    "connected to Valkey; quote caching is live"
                );
                true
            }
            Err(err) => {
                tracing::warn!(
                    error = %err,
                    server.address = %self.server_address,
                    "Valkey is unreachable; serving quotes without the cache"
                );
                false
            }
        }
    }

    /// Keeps trying to connect until it succeeds or the process shuts down. Valkey is soft, so
    /// this never blocks startup.
    pub async fn run_connector(self: Arc<Self>, cancel: CancellationToken) {
        if !self.enabled() {
            tracing::info!("VALKEY_URL is unset; the quote cache is disabled");
            return;
        }
        loop {
            if self.ensure_connected().await {
                return;
            }
            tokio::select! {
                biased;
                () = cancel.cancelled() => return,
                () = tokio::time::sleep(RECONNECT_INTERVAL) => {}
            }
        }
    }

    /// Reads a quote by id, for `GET /v1/quotes/{id}`.
    ///
    /// `Ok(None)` is a miss; `Err` means Valkey could not answer, which the handler turns into a
    /// 503 rather than a misleading 404.
    pub async fn get_by_id(&self, id: &str) -> Result<Option<Quote>, StoreError> {
        let key = quote_key(id);
        self.get_json(&key, "quote").await
    }

    /// Reads a cached quote for a set of pricing inputs, honouring the `quote-cache` flag's 60s
    /// window. A failure is reported as a miss: the cache is an optimisation.
    pub async fn get_cached(&self, inputs: &PricingInputs) -> Option<Quote> {
        let key = input_key(inputs);
        match self.get_json(&key, "quote:input").await {
            Ok(quote) => quote,
            Err(_) => {
                self.metrics.record_cache_error("get");
                None
            }
        }
    }

    async fn get_json(&self, key: &str, collection: &str) -> Result<Option<Quote>, StoreError> {
        let Some(mut connection) = self.connection() else {
            return Err(StoreError::Unavailable);
        };
        let raw: Option<String> = redis::cmd("GET")
            .arg(key)
            .query_async::<Option<String>>(&mut connection)
            .instrument(self.span("GET", collection))
            .await
            .map_err(StoreError::Redis)?;

        match raw {
            None => Ok(None),
            Some(json) => match serde_json::from_str::<Quote>(&json) {
                Ok(quote) => Ok(Some(quote)),
                Err(err) => {
                    // A record written by an older build. Treat as a miss rather than a failure.
                    tracing::warn!(error = %err, key, "discarding an unreadable cached quote");
                    Ok(None)
                }
            },
        }
    }

    /// Stores a quote under `quote:{id}` for an hour and, when `cache_key` is supplied, under the
    /// input key for sixty seconds. Both writes go in one pipeline, so a cache fill costs one
    /// round trip.
    pub async fn store(&self, quote: &Quote, cache_key: Option<&str>) {
        let Some(mut connection) = self.connection() else {
            return;
        };
        let json = match serde_json::to_string(quote) {
            Ok(json) => json,
            Err(err) => {
                tracing::error!(error = %err, "a quote could not be serialized for the cache");
                return;
            }
        };

        let mut pipeline = redis::pipe();
        pipeline
            .cmd("SET")
            .arg(quote_key(&quote.id))
            .arg(&json)
            .arg("EX")
            .arg(QUOTE_TTL.as_secs())
            .ignore();
        if let Some(cache_key) = cache_key {
            pipeline
                .cmd("SET")
                .arg(cache_key)
                .arg(&json)
                .arg("EX")
                .arg(QUOTE_CACHE_TTL.as_secs())
                .ignore();
        }

        let result = pipeline
            .query_async::<()>(&mut connection)
            .instrument(self.span("SET", "quote"))
            .await;
        if let Err(err) = result {
            self.metrics.record_cache_error("set");
            tracing::warn!(error = %err, "a quote could not be written to Valkey");
        }
    }

    /// A client span for one Valkey operation, following the OpenTelemetry database conventions so
    /// the console's topology draws a cache edge (design section 8.15, "Topology model").
    fn span(&self, operation: &'static str, collection: &str) -> tracing::Span {
        tracing::info_span!(
            "valkey",
            otel.name = %format!("{operation} {collection}"),
            otel.kind = "client",
            db.system = "redis",
            db.system.name = "valkey",
            db.namespace = %self.db_namespace,
            db.operation.name = operation,
            db.collection.name = %collection,
            server.address = %self.server_address,
            server.port = self.server_port,
            peer.service = "valkey",
        )
    }
}

#[derive(Debug)]
pub enum StoreError {
    /// No connection to Valkey (unset, unparseable, or not yet established).
    Unavailable,
    Redis(redis::RedisError),
}

impl std::fmt::Display for StoreError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Unavailable => f.write_str("Valkey is unavailable"),
            Self::Redis(err) => write!(f, "Valkey error: {err}"),
        }
    }
}

impl std::error::Error for StoreError {}

/// Rebuilds the pricing inputs a stored quote was computed from. Used by the integration test to
/// check that a round trip preserves the tuple.
#[must_use]
pub fn inputs_of(quote: &Quote, turbo: bool) -> PricingInputs {
    PricingInputs {
        model: quote.model,
        quantity: quote.quantity,
        options: quote.options,
        turbo,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::model::{Model, Options};

    fn inputs(model: Model, quantity: u32, dingle_arm: bool, turbo: bool) -> PricingInputs {
        PricingInputs {
            model,
            quantity,
            options: Options { dingle_arm },
            turbo,
        }
    }

    #[test]
    fn input_keys_live_under_the_quote_prefix() {
        let key = input_key(&inputs(Model::Turbo, 2, false, false));
        assert!(key.starts_with("quote:"), "{key}");
        assert_eq!(key, "quote:input:turbo:2:false:false");
    }

    #[test]
    fn quote_keys_live_under_the_quote_prefix() {
        assert_eq!(quote_key("qte_abc"), "quote:qte_abc");
    }

    #[test]
    fn identical_inputs_produce_identical_keys() {
        let a = inputs(Model::Hyper, 5, true, true);
        let b = inputs(Model::Hyper, 5, true, true);
        assert_eq!(input_key(&a), input_key(&b));
    }

    #[test]
    fn every_price_moving_input_changes_the_key() {
        let base = inputs(Model::Turbo, 2, false, false);
        let base_key = input_key(&base);
        let variants = [
            inputs(Model::Retro, 2, false, false),
            inputs(Model::Hyper, 2, false, false),
            inputs(Model::Turbo, 3, false, false),
            inputs(Model::Turbo, 2, true, false),
            inputs(Model::Turbo, 2, false, true),
        ];
        for variant in variants {
            assert_ne!(base_key, input_key(&variant), "{variant:?} collided");
        }
    }

    #[test]
    fn the_whole_input_space_is_collision_free() {
        let mut keys = std::collections::HashSet::new();
        for model in Model::ALL {
            for quantity in 1..=10 {
                for dingle_arm in [false, true] {
                    for turbo in [false, true] {
                        let key = input_key(&inputs(model, quantity, dingle_arm, turbo));
                        assert!(keys.insert(key.clone()), "{key} produced twice");
                    }
                }
            }
        }
        assert_eq!(keys.len(), 3 * 10 * 2 * 2);
    }

    #[test]
    fn an_unset_url_disables_the_store() {
        let store = QuoteStore::new(None, Metrics::new());
        assert!(!store.enabled());
        assert_eq!(store.state(), CacheState::Disabled);
    }

    #[test]
    fn an_unparseable_url_disables_the_store_rather_than_panicking() {
        let store = QuoteStore::new(Some("not-a-url"), Metrics::new());
        assert!(!store.enabled());
        assert_eq!(store.state(), CacheState::Disabled);
    }

    #[test]
    fn a_valid_url_yields_connection_metadata_for_the_client_spans() {
        let store = QuoteStore::new(Some("redis://default:secret@valkey:6379/3"), Metrics::new());
        assert!(store.enabled());
        assert_eq!(store.state(), CacheState::Connecting);
        assert_eq!(store.server_address, "valkey");
        assert_eq!(store.server_port, 6379);
        assert_eq!(store.db_namespace, "3");
    }

    #[tokio::test]
    async fn reads_fail_closed_and_writes_are_silent_without_a_connection() {
        let metrics = Metrics::new();
        let store = QuoteStore::new(Some("redis://127.0.0.1:1/0"), metrics.clone());
        assert!(matches!(
            store.get_by_id("qte_x").await,
            Err(StoreError::Unavailable)
        ));
        assert!(
            store
                .get_cached(&inputs(Model::Turbo, 1, false, false))
                .await
                .is_none()
        );

        // A store() with no connection must not panic and must not count an error: there was
        // nothing to write to.
        let quote = crate::model::Quote {
            id: "qte_x".to_owned(),
            model: Model::Turbo,
            quantity: 1,
            options: Options::default(),
            unit_price: crate::money::Money::from_cents(1),
            subtotal: crate::money::Money::from_cents(1),
            surcharges: Vec::new(),
            total: crate::money::Money::from_cents(1),
            currency: crate::model::Currency::Usd,
            computed_at: "2026-09-30T12:00:00.000Z".to_owned(),
            cached: false,
        };
        store
            .store(&quote, Some("quote:input:turbo:1:false:false"))
            .await;
        assert!(
            !metrics
                .encode()
                .contains("encabulator_quote_cache_errors_total{operation=\"set\"} 1")
        );
    }

    #[tokio::test]
    async fn a_disabled_connector_returns_immediately() {
        let store = QuoteStore::new(None, Metrics::new());
        tokio::time::timeout(
            Duration::from_secs(1),
            store.run_connector(CancellationToken::new()),
        )
        .await
        .expect("connector returned");
    }

    #[test]
    fn inputs_of_recovers_the_pricing_tuple() {
        let quote = crate::model::Quote {
            id: "qte_x".to_owned(),
            model: Model::Hyper,
            quantity: 4,
            options: Options { dingle_arm: true },
            unit_price: crate::money::Money::from_cents(1),
            subtotal: crate::money::Money::from_cents(1),
            surcharges: Vec::new(),
            total: crate::money::Money::from_cents(1),
            currency: crate::model::Currency::Usd,
            computed_at: "2026-09-30T12:00:00.000Z".to_owned(),
            cached: false,
        };
        let recovered = inputs_of(&quote, true);
        assert_eq!(recovered, inputs(Model::Hyper, 4, true, true));
    }
}

/// Captures span names and fields so a test can assert on the OpenTelemetry attributes the quoter
/// emits, without standing up a collector.
#[cfg(test)]
mod span_capture {
    use std::collections::BTreeMap;
    use std::sync::{Arc, Mutex};

    use tracing::field::{Field, Visit};
    use tracing_subscriber::layer::{Context, Layer};
    use tracing_subscriber::registry::LookupSpan;

    /// One captured span: its name and the fields recorded at creation.
    pub type CapturedSpan = (String, BTreeMap<String, String>);

    #[derive(Clone, Default)]
    pub struct Captured(pub Arc<Mutex<Vec<CapturedSpan>>>);

    impl Captured {
        pub fn named(&self, name: &str) -> Option<BTreeMap<String, String>> {
            self.0
                .lock()
                .unwrap()
                .iter()
                .find(|(span_name, _)| span_name == name)
                .map(|(_, fields)| fields.clone())
        }
    }

    pub struct CaptureLayer(pub Captured);

    impl<S: tracing::Subscriber + for<'a> LookupSpan<'a>> Layer<S> for CaptureLayer {
        fn on_new_span(
            &self,
            attrs: &tracing::span::Attributes<'_>,
            _id: &tracing::span::Id,
            _ctx: Context<'_, S>,
        ) {
            let mut visitor = Collector(BTreeMap::new());
            attrs.record(&mut visitor);
            self.0
                .0
                .lock()
                .unwrap()
                .push((attrs.metadata().name().to_owned(), visitor.0));
        }
    }

    struct Collector(BTreeMap<String, String>);

    impl Visit for Collector {
        fn record_str(&mut self, field: &Field, value: &str) {
            self.0.insert(field.name().to_owned(), value.to_owned());
        }
        fn record_u64(&mut self, field: &Field, value: u64) {
            self.0.insert(field.name().to_owned(), value.to_string());
        }
        fn record_i64(&mut self, field: &Field, value: i64) {
            self.0.insert(field.name().to_owned(), value.to_string());
        }
        fn record_debug(&mut self, field: &Field, value: &dyn std::fmt::Debug) {
            self.0.insert(field.name().to_owned(), format!("{value:?}"));
        }
    }
}

#[cfg(test)]
mod span_tests {
    use super::*;
    use tracing_subscriber::layer::SubscriberExt;

    #[test]
    fn valkey_spans_carry_the_database_semantic_conventions() {
        let captured = span_capture::Captured::default();
        let subscriber =
            tracing_subscriber::registry().with(span_capture::CaptureLayer(captured.clone()));

        let store = QuoteStore::new(Some("redis://default:secret@valkey:6379/2"), Metrics::new());
        tracing::subscriber::with_default(subscriber, || {
            let _span = store.span("GET", "quote");
        });

        let fields = captured.named("valkey").expect("a valkey span was created");
        // The console's topology draws cache edges from client spans with db.system set.
        assert_eq!(fields.get("db.system").map(String::as_str), Some("redis"));
        assert_eq!(
            fields.get("db.system.name").map(String::as_str),
            Some("valkey")
        );
        assert_eq!(fields.get("otel.kind").map(String::as_str), Some("client"));
        assert_eq!(
            fields.get("otel.name").map(String::as_str),
            Some("GET quote")
        );
        assert_eq!(fields.get("db.namespace").map(String::as_str), Some("2"));
        assert_eq!(
            fields.get("db.operation.name").map(String::as_str),
            Some("GET")
        );
        assert_eq!(
            fields.get("db.collection.name").map(String::as_str),
            Some("quote")
        );
        assert_eq!(
            fields.get("server.address").map(String::as_str),
            Some("valkey")
        );
        assert_eq!(fields.get("server.port").map(String::as_str), Some("6379"));
        assert_eq!(
            fields.get("peer.service").map(String::as_str),
            Some("valkey")
        );
        // The password must never reach a span attribute.
        assert!(!fields.values().any(|v| v.contains("secret")));
    }

    #[test]
    fn a_set_span_names_its_operation() {
        let captured = span_capture::Captured::default();
        let subscriber =
            tracing_subscriber::registry().with(span_capture::CaptureLayer(captured.clone()));
        let store = QuoteStore::new(Some("redis://valkey:6379/0"), Metrics::new());
        tracing::subscriber::with_default(subscriber, || {
            let _span = store.span("SET", "quote");
        });
        let fields = captured.named("valkey").unwrap();
        assert_eq!(
            fields.get("otel.name").map(String::as_str),
            Some("SET quote")
        );
    }
}
