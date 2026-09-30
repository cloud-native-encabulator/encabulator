//! Prometheus metrics (design section 7, "Metrics").
//!
//! A dedicated registry rather than the crate's global default, so that two quoters in one test
//! process do not fight over metric names.

use std::sync::Arc;

use prometheus::{
    Encoder, HistogramOpts, HistogramVec, IntCounterVec, Opts, Registry, TextEncoder,
};

/// The exact buckets the design mandates for HTTP server latency.
pub const HTTP_DURATION_BUCKETS: &[f64] = &[
    0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
];

/// Buckets for `encabulator_quote_compute_seconds`. The design does not specify them; these
/// bracket the 5-15 ms pentametric harmonic closely enough to see it drift, and extend far enough
/// to show congestion queueing.
pub const QUOTE_COMPUTE_BUCKETS: &[f64] = &[
    0.001, 0.0025, 0.005, 0.0075, 0.01, 0.015, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0,
];

#[derive(Clone)]
pub struct Metrics {
    registry: Registry,
    pub http_server_duration: HistogramVec,
    pub http_client_duration: HistogramVec,
    pub quotes_total: IntCounterVec,
    pub quote_compute_seconds: prometheus::Histogram,
    pub malaise_applied_total: IntCounterVec,
    pub quote_cache_errors_total: IntCounterVec,
}

impl Metrics {
    /// Builds and registers every metric. Panics only on a programming error (a duplicate metric
    /// name or an invalid label set), which would fail on the first test run.
    pub fn new() -> Arc<Self> {
        let registry = Registry::new();

        let http_server_duration = HistogramVec::new(
            HistogramOpts::new(
                "http_server_request_duration_seconds",
                "Duration of inbound HTTP requests, by route template.",
            )
            .buckets(HTTP_DURATION_BUCKETS.to_vec()),
            &[
                "http_request_method",
                "http_route",
                "http_response_status_code",
            ],
        )
        .expect("http server histogram");

        let http_client_duration = HistogramVec::new(
            HistogramOpts::new(
                "http_client_request_duration_seconds",
                "Duration of outbound HTTP requests, by target service.",
            )
            .buckets(HTTP_DURATION_BUCKETS.to_vec()),
            &[
                "http_request_method",
                "server_address",
                "http_response_status_code",
            ],
        )
        .expect("http client histogram");

        let quotes_total = IntCounterVec::new(
            Opts::new(
                "encabulator_quotes_total",
                "Quotes served, by model and whether they came from the Valkey cache.",
            ),
            &["model", "cached"],
        )
        .expect("quotes counter");

        let quote_compute_seconds = prometheus::Histogram::with_opts(
            HistogramOpts::new(
                "encabulator_quote_compute_seconds",
                "Time spent pricing a quote, including the pentametric harmonic.",
            )
            .buckets(QUOTE_COMPUTE_BUCKETS.to_vec()),
        )
        .expect("quote compute histogram");

        let malaise_applied_total = IntCounterVec::new(
            Opts::new(
                "encabulator_malaise_applied_total",
                "Injected faults applied, by kind.",
            ),
            &["kind"],
        )
        .expect("malaise counter");

        let quote_cache_errors_total = IntCounterVec::new(
            Opts::new(
                "encabulator_quote_cache_errors_total",
                "Valkey cache operations that failed. Valkey is a soft dependency, so these are \
                 degradations rather than request failures.",
            ),
            &["operation"],
        )
        .expect("cache error counter");

        for collector in [
            Box::new(http_server_duration.clone()) as Box<dyn prometheus::core::Collector>,
            Box::new(http_client_duration.clone()),
            Box::new(quotes_total.clone()),
            Box::new(quote_compute_seconds.clone()),
            Box::new(malaise_applied_total.clone()),
            Box::new(quote_cache_errors_total.clone()),
        ] {
            registry.register(collector).expect("register collector");
        }

        // The runtime's default process metrics. The prometheus crate only implements the process
        // collector on Linux, which is where this service actually runs.
        #[cfg(target_os = "linux")]
        registry
            .register(Box::new(
                prometheus::process_collector::ProcessCollector::for_self(),
            ))
            .expect("register process collector");

        // Pre-create the label combinations that would otherwise be absent until the first quote,
        // so the console's panels read "0" rather than "no data".
        for model in crate::model::Model::ALL {
            for cached in ["true", "false"] {
                quotes_total.with_label_values(&[model.id(), cached]);
            }
        }

        Arc::new(Self {
            registry,
            http_server_duration,
            http_client_duration,
            quotes_total,
            quote_compute_seconds,
            malaise_applied_total,
            quote_cache_errors_total,
        })
    }

    pub fn observe_server_request(&self, method: &str, route: &str, status: u16, seconds: f64) {
        self.http_server_duration
            .with_label_values(&[method, route, &status.to_string()])
            .observe(seconds);
    }

    /// Records an outbound call. `status` is `None` when the request never produced a response
    /// (connection refused, timeout), which is reported as `status_code=""` rather than invented.
    pub fn observe_client_request(
        &self,
        method: &str,
        server_address: &str,
        status: Option<u16>,
        seconds: f64,
    ) {
        let status = status.map(|s| s.to_string()).unwrap_or_default();
        self.http_client_duration
            .with_label_values(&[method, server_address, &status])
            .observe(seconds);
    }

    pub fn record_quote(&self, model: crate::model::Model, cached: bool) {
        self.quotes_total
            .with_label_values(&[model.id(), if cached { "true" } else { "false" }])
            .inc();
    }

    pub fn record_malaise(&self, kind: &str) {
        self.malaise_applied_total.with_label_values(&[kind]).inc();
    }

    pub fn record_cache_error(&self, operation: &str) {
        self.quote_cache_errors_total
            .with_label_values(&[operation])
            .inc();
    }

    /// Renders the registry in Prometheus text format.
    pub fn encode(&self) -> String {
        let encoder = TextEncoder::new();
        let mut buffer = Vec::with_capacity(8 * 1024);
        if encoder
            .encode(&self.registry.gather(), &mut buffer)
            .is_err()
        {
            return String::new();
        }
        String::from_utf8(buffer).unwrap_or_default()
    }

    /// The content type Prometheus expects for the text exposition format.
    pub fn content_type() -> &'static str {
        prometheus::TEXT_FORMAT
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn server_histogram_uses_the_exact_buckets_from_the_design() {
        let metrics = Metrics::new();
        metrics.observe_server_request("POST", "/v1/quotes", 200, 0.02);
        let text = metrics.encode();
        for bucket in HTTP_DURATION_BUCKETS {
            let needle = format!("le=\"{bucket}\"");
            let alt = format!("le=\"{bucket:.3}\"");
            assert!(
                text.contains(&needle) || text.contains(&alt),
                "bucket {bucket} missing from:\n{text}"
            );
        }
        assert!(text.contains("http_server_request_duration_seconds_bucket"));
        assert!(text.contains("http_route=\"/v1/quotes\""));
        assert!(text.contains("http_request_method=\"POST\""));
        assert!(text.contains("http_response_status_code=\"200\""));
    }

    #[test]
    fn exposes_every_required_metric_family() {
        let metrics = Metrics::new();
        metrics.observe_server_request("POST", "/v1/quotes", 200, 0.02);
        metrics.observe_client_request("GET", "differential-girdlespring", Some(200), 0.004);
        metrics.record_quote(crate::model::Model::Turbo, true);
        metrics.quote_compute_seconds.observe(0.007);
        metrics.record_malaise("latency");
        metrics.record_cache_error("get");
        let text = metrics.encode();
        for family in [
            "http_server_request_duration_seconds",
            "http_client_request_duration_seconds",
            "encabulator_quotes_total",
            "encabulator_quote_compute_seconds",
            "encabulator_malaise_applied_total",
            "encabulator_quote_cache_errors_total",
        ] {
            assert!(text.contains(family), "{family} missing");
        }
    }

    #[test]
    fn quote_series_exist_before_the_first_quote() {
        let text = Metrics::new().encode();
        for model in crate::model::Model::ALL {
            assert!(text.contains(&format!("model=\"{}\"", model.id())));
        }
    }

    #[test]
    fn client_metric_tolerates_a_missing_status() {
        let metrics = Metrics::new();
        metrics.observe_client_request("GET", "malaise-injection-engine", None, 3.0);
        assert!(metrics.encode().contains("http_response_status_code=\"\""));
    }
}
