//! Configuration. Environment variables only, per design section 7 ("Configuration").

use std::env;
use std::time::Duration;

/// The canonical service name. Used as the default for `OTEL_SERVICE_NAME`, as the `service`
/// field in every log line, and as the `service=` query parameter sent to the malaise engine.
pub const SERVICE_NAME: &str = "spurving-bearing-quoter";

/// The namespace every Encabulator service shares (`service.namespace` resource attribute).
pub const SERVICE_NAMESPACE: &str = "encabulator";

/// Build version, surfaced as the `service.version` resource attribute.
pub const SERVICE_VERSION: &str = env!("CARGO_PKG_VERSION");

/// Default outbound HTTP client timeout (design section 7, "Timeouts").
pub const OUTBOUND_TIMEOUT: Duration = Duration::from_secs(3);

/// Server read-header timeout (design section 7, "Timeouts").
pub const READ_HEADER_TIMEOUT: Duration = Duration::from_secs(5);

/// Server idle-connection timeout (design section 7, "Timeouts").
pub const IDLE_TIMEOUT: Duration = Duration::from_secs(60);

/// How long to drain in-flight requests after SIGTERM (design section 7, "Graceful shutdown").
pub const DRAIN_TIMEOUT: Duration = Duration::from_secs(20);

/// How long to keep serving after readiness starts failing, before the listener closes.
///
/// The design lists the shutdown steps as "fail readiness, stop accepting, drain" but names no
/// delay between the first two. Without one the ordering is notional: the listener closes in the
/// same millisecond, long before kube-proxy has removed the pod from its Service, so a rolling
/// update produces connection refusals. Five seconds of continued service is enough for endpoint
/// propagation, and `SHUTDOWN_DELAY_SECONDS=0` restores the literal reading.
///
/// Budget for the Helm chart: `terminationGracePeriodSeconds` must exceed
/// `SHUTDOWN_DELAY_SECONDS` + 20.
pub const DEFAULT_SHUTDOWN_DELAY: Duration = Duration::from_secs(5);

/// Malaise directive poll interval (design section 7, "Malaise").
pub const MALAISE_POLL_INTERVAL: Duration = Duration::from_secs(2);

/// Feature flag poll interval (design section 6).
pub const FLAG_POLL_INTERVAL: Duration = Duration::from_secs(5);

/// Valkey is a *soft* dependency, so cache operations get a much tighter budget than the
/// general outbound timeout: a sick cache must never become the quoter's latency.
pub const VALKEY_TIMEOUT: Duration = Duration::from_secs(1);

/// TTL for `quote:{id}` records, read back by `GET /v1/quotes/{id}` (design section 8.3).
pub const QUOTE_TTL: Duration = Duration::from_secs(3600);

/// TTL for the input-keyed cache governed by the `quote-cache` flag (design section 6).
pub const QUOTE_CACHE_TTL: Duration = Duration::from_secs(60);

#[derive(Debug, Clone)]
pub struct Config {
    pub port: u16,
    pub log_level: String,
    pub service_name: String,
    pub otlp_endpoint: Option<String>,
    pub malaise_engine_url: Option<String>,
    pub girdlespring_url: Option<String>,
    pub valkey_url: Option<String>,
    /// Multiplies the pentametric harmonic's CPU target. `1.0` yields the 5-15 ms of the design.
    pub pentametric_burn_scale: f64,
    /// Upper bound on concurrent pentametric harmonic computations, so the CPU burn cannot
    /// starve the async runtime.
    pub pentametric_workers: usize,
    /// How long to keep serving after readiness starts failing. See [`DEFAULT_SHUTDOWN_DELAY`].
    pub shutdown_delay: Duration,
}

impl Config {
    /// Reads configuration from the process environment, applying the defaults in appendix A.
    pub fn from_env() -> Self {
        let service_name =
            non_empty("OTEL_SERVICE_NAME").unwrap_or_else(|| SERVICE_NAME.to_owned());
        Self {
            port: non_empty("PORT")
                .and_then(|v| v.parse().ok())
                .unwrap_or(8080),
            log_level: non_empty("LOG_LEVEL").unwrap_or_else(|| "info".to_owned()),
            service_name,
            otlp_endpoint: non_empty("OTEL_EXPORTER_OTLP_ENDPOINT"),
            malaise_engine_url: non_empty("MALAISE_ENGINE_URL").map(trim_slash),
            girdlespring_url: non_empty("DIFFERENTIAL_GIRDLESPRING_URL").map(trim_slash),
            valkey_url: non_empty("VALKEY_URL"),
            pentametric_burn_scale: non_empty("PENTAMETRIC_BURN_SCALE")
                .and_then(|v| v.parse::<f64>().ok())
                .filter(|v| v.is_finite() && *v >= 0.0)
                .unwrap_or(1.0),
            pentametric_workers: non_empty("PENTAMETRIC_WORKERS")
                .and_then(|v| v.parse::<usize>().ok())
                .filter(|v| *v > 0)
                .unwrap_or_else(default_workers),
            shutdown_delay: non_empty("SHUTDOWN_DELAY_SECONDS")
                .and_then(|v| v.parse::<u64>().ok())
                .map(Duration::from_secs)
                .unwrap_or(DEFAULT_SHUTDOWN_DELAY),
        }
    }

    /// A configuration with every optional integration switched off. Used by the unit tests so
    /// they never touch the network, and by `Config::from_env` consumers that want a baseline.
    pub fn for_tests() -> Self {
        Self {
            port: 0,
            log_level: "info".to_owned(),
            service_name: SERVICE_NAME.to_owned(),
            otlp_endpoint: None,
            malaise_engine_url: None,
            girdlespring_url: None,
            valkey_url: None,
            pentametric_burn_scale: 0.0,
            pentametric_workers: 2,
            shutdown_delay: Duration::ZERO,
        }
    }
}

fn default_workers() -> usize {
    std::thread::available_parallelism()
        .map(std::num::NonZeroUsize::get)
        .unwrap_or(2)
}

fn non_empty(key: &str) -> Option<String> {
    env::var(key)
        .ok()
        .map(|v| v.trim().to_owned())
        .filter(|v| !v.is_empty())
}

fn trim_slash(mut url: String) -> String {
    while url.ends_with('/') {
        url.pop();
    }
    url
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn trims_trailing_slashes_from_service_urls() {
        assert_eq!(
            trim_slash("http://malaise:8080///".to_owned()),
            "http://malaise:8080"
        );
        assert_eq!(
            trim_slash("http://malaise:8080".to_owned()),
            "http://malaise:8080"
        );
    }

    #[test]
    fn test_config_disables_every_integration() {
        let cfg = Config::for_tests();
        assert!(cfg.otlp_endpoint.is_none());
        assert!(cfg.malaise_engine_url.is_none());
        assert!(cfg.girdlespring_url.is_none());
        assert!(cfg.valkey_url.is_none());
    }
}

#[cfg(test)]
mod shutdown_tests {
    use super::*;

    #[test]
    fn the_shutdown_budget_fits_the_default_kubernetes_grace_period() {
        // Kubernetes defaults terminationGracePeriodSeconds to 30.
        let total = DEFAULT_SHUTDOWN_DELAY + DRAIN_TIMEOUT;
        assert!(total < Duration::from_secs(30), "{total:?}");
    }

    #[test]
    fn tests_shut_down_immediately() {
        assert_eq!(Config::for_tests().shutdown_delay, Duration::ZERO);
    }
}
