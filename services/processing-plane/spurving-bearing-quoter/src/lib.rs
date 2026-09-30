//! # spurving-bearing-quoter
//!
//! > "The two main spurving bearings were in a direct line with the pentametric fan."
//!
//! The pricing engine of the Cloud Native Encabulator. It aligns the spurving bearings with the
//! pentametric fan, economically: given a model, a quantity, and whether the customer wants a drawn
//! reciprocation dingle arm, it returns a quote, and it computes the pentametric harmonic on the
//! way there whether anyone needs the number or not. That harmonic is the point. It costs real CPU,
//! which makes this the service the HPA has opinions about.
//!
//! Everything in design section 7 is implemented here directly rather than through a shared kit:
//! JSON logs with trace, span, and request ids; OpenTelemetry with W3C propagation; the three ops
//! endpoints; the metric families and their exact buckets; the one error shape; the malaise
//! middleware; and the girdlespring flag client.

pub mod app;
pub mod cache;
pub mod config;
pub mod error;
pub mod flags;
pub mod http;
pub mod malaise;
pub mod metrics;
pub mod model;
pub mod money;
pub mod outbound;
pub mod pricing;
pub mod serve;
pub mod telemetry;

use std::net::SocketAddr;

use tokio::net::TcpListener;
use tokio_util::sync::CancellationToken;

use crate::app::AppState;
use crate::config::Config;

/// Boots the service and runs it until SIGTERM.
///
/// Startup order matters: the listener is bound before the background pollers start, so that a pod
/// answering `/readyz` is a pod that can also answer `/v1/quotes`, and the pollers are cancelled
/// before the tracer is flushed, so that nothing new is recorded after the last export.
pub async fn run() -> Result<(), Box<dyn std::error::Error>> {
    let config = Config::from_env();
    let telemetry = telemetry::Telemetry::init(&config);

    let port = config.port;
    let burn_scale = config.pentametric_burn_scale;
    let workers = config.pentametric_workers;
    let state = AppState::new(config)?;

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    let listener = TcpListener::bind(addr).await?;
    tracing::info!(
        version = config::SERVICE_VERSION,
        %addr,
        pentametric_workers = workers,
        pentametric_burn_scale = burn_scale,
        tracing = state.config.otlp_endpoint.is_some(),
        malaise = state.malaise.enabled(),
        flags = state.flags.enabled(),
        quote_cache = state.store.enabled(),
        "the spurving bearing quoter is listening"
    );

    let background = CancellationToken::new();
    let flag_poller = tokio::spawn(
        state
            .flags
            .clone()
            .run(state.outbound.clone(), background.clone()),
    );
    let malaise_poller = tokio::spawn(
        state
            .malaise
            .clone()
            .run(state.outbound.clone(), background.clone()),
    );
    let valkey_connector = tokio::spawn(state.store.clone().run_connector(background.clone()));

    let shutdown = CancellationToken::new();
    let signal_state = state.clone();
    let signal_shutdown = shutdown.clone();
    let shutdown_delay = state.config.shutdown_delay;
    tokio::spawn(async move {
        let signal = serve::shutdown_signal().await;
        // Fail readiness first, then stop accepting, then drain (design section 7).
        signal_state.begin_draining();
        tracing::info!(
            signal,
            delay_seconds = shutdown_delay.as_secs(),
            "shutting down: readiness is now failing"
        );
        // Keep serving while the Service endpoints catch up.
        if !shutdown_delay.is_zero() {
            tokio::time::sleep(shutdown_delay).await;
        }
        signal_shutdown.cancel();
    });

    serve::serve(listener, http::router(state), shutdown).await;

    background.cancel();
    let _ = tokio::join!(flag_poller, malaise_poller, valkey_connector);
    telemetry.shutdown();
    tracing::info!("the spurving bearings have come to rest");
    Ok(())
}
