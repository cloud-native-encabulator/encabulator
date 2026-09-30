//! Outbound HTTP (design section 7, "Timeouts" and "Outbound retries").
//!
//! > Outbound client default timeout 3s. Outbound retries: idempotent requests (GET, PUT, DELETE)
//! > retry twice with exponential backoff (100ms base, full jitter).
//!
//! The quoter's only outbound HTTP is the flag poll and the malaise poll, both of which the design
//! requires to stay untraced, so no trace context is injected here and no span is created. Latency
//! is still measured into `http_client_request_duration_seconds`.

use std::sync::Arc;
use std::time::{Duration, Instant};

use http::StatusCode;

use crate::config::OUTBOUND_TIMEOUT;
use crate::metrics::Metrics;

/// Base for the exponential backoff.
const BACKOFF_BASE: Duration = Duration::from_millis(100);

/// One extra attempt, then a second: three total for an idempotent request.
pub const IDEMPOTENT_RETRIES: u32 = 2;

/// A completed response, body already read.
pub struct Fetched {
    pub status: StatusCode,
    pub etag: Option<String>,
    pub body: Vec<u8>,
}

impl Fetched {
    #[must_use]
    pub fn is_not_modified(&self) -> bool {
        self.status == StatusCode::NOT_MODIFIED
    }
}

#[derive(Clone)]
pub struct Outbound {
    client: reqwest::Client,
    metrics: Arc<Metrics>,
}

impl Outbound {
    /// Builds the shared client. Fails only if the platform cannot provide a TLS or resolver
    /// backend, which would be a broken image rather than a runtime condition.
    pub fn new(metrics: Arc<Metrics>) -> Result<Self, reqwest::Error> {
        let client = reqwest::Client::builder()
            .timeout(OUTBOUND_TIMEOUT)
            .user_agent(format!(
                "{}/{}",
                crate::config::SERVICE_NAME,
                crate::config::SERVICE_VERSION
            ))
            .build()?;
        Ok(Self { client, metrics })
    }

    /// A single GET, measured but not retried. Used by the malaise poller, which runs every two
    /// seconds and so has no use for retries.
    pub async fn get_once(
        &self,
        url: &str,
        server_address: &str,
        if_none_match: Option<&str>,
    ) -> Result<Fetched, reqwest::Error> {
        self.attempt(url, server_address, if_none_match).await
    }

    /// A GET with the design's retry policy: two retries, exponential backoff from a 100 ms base,
    /// full jitter.
    pub async fn get(
        &self,
        url: &str,
        server_address: &str,
        if_none_match: Option<&str>,
    ) -> Result<Fetched, reqwest::Error> {
        let mut last_error = None;
        for attempt in 0..=IDEMPOTENT_RETRIES {
            if attempt > 0 {
                tokio::time::sleep(full_jitter_backoff(attempt)).await;
            }
            match self.attempt(url, server_address, if_none_match).await {
                Ok(fetched) => return Ok(fetched),
                Err(err) => last_error = Some(err),
            }
        }
        Err(last_error.expect("at least one attempt was made"))
    }

    async fn attempt(
        &self,
        url: &str,
        server_address: &str,
        if_none_match: Option<&str>,
    ) -> Result<Fetched, reqwest::Error> {
        let mut request = self.client.get(url);
        if let Some(etag) = if_none_match {
            request = request.header(http::header::IF_NONE_MATCH, etag);
        }

        let started = Instant::now();
        let result = request.send().await;
        let status = result.as_ref().ok().map(|r| r.status().as_u16());
        self.metrics.observe_client_request(
            "GET",
            server_address,
            status,
            started.elapsed().as_secs_f64(),
        );

        let response = result?;
        let status = response.status();
        let etag = response
            .headers()
            .get(http::header::ETAG)
            .and_then(|v| v.to_str().ok())
            .map(str::to_owned);
        let body = response.bytes().await?.to_vec();
        Ok(Fetched { status, etag, body })
    }
}

/// Exponential backoff with full jitter: a uniform draw from `[0, base * 2^(attempt-1)]`.
fn full_jitter_backoff(attempt: u32) -> Duration {
    let ceiling = BACKOFF_BASE.saturating_mul(1 << attempt.min(10));
    let millis = ceiling.as_millis().max(1) as u64;
    Duration::from_millis(fastrand::u64(0..=millis))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn backoff_ceiling_doubles_and_stays_jittered() {
        for _ in 0..200 {
            assert!(full_jitter_backoff(1) <= Duration::from_millis(200));
            assert!(full_jitter_backoff(2) <= Duration::from_millis(400));
            assert!(full_jitter_backoff(3) <= Duration::from_millis(800));
        }
        // Full jitter means some draws are genuinely small.
        let any_small = (0..200).any(|_| full_jitter_backoff(3) < Duration::from_millis(400));
        assert!(
            any_small,
            "full jitter should sometimes return a short delay"
        );
    }

    #[test]
    fn backoff_never_overflows() {
        assert!(full_jitter_backoff(64) <= Duration::from_millis(100 * 1024));
    }

    #[tokio::test]
    async fn a_refused_connection_records_a_client_metric_without_a_status() {
        let metrics = Metrics::new();
        let outbound = Outbound::new(metrics.clone()).expect("client");
        // Port 1 on loopback refuses immediately on every platform we build for.
        let result = outbound
            .get_once(
                "http://127.0.0.1:1/v1/flags",
                "differential-girdlespring",
                None,
            )
            .await;
        assert!(result.is_err());
        let text = metrics.encode();
        assert!(text.contains("server_address=\"differential-girdlespring\""));
        assert!(text.contains("http_response_status_code=\"\""));
    }
}
