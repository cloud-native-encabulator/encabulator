//! Logging and tracing setup (design section 7, "Logging" and "Tracing").
//!
//! Tracing is a no-op when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset: no exporter is built, no
//! OpenTelemetry layer is installed, and `trace_id`/`span_id` simply do not appear in the logs.
//! Spans are still created by the middleware, so the code path is identical either way.

pub mod json_log;

use std::time::Duration;

use opentelemetry::KeyValue;
use opentelemetry::global;
use opentelemetry::trace::TracerProvider as _;
use opentelemetry_otlp::WithExportConfig;
use opentelemetry_sdk::Resource;
use opentelemetry_sdk::propagation::TraceContextPropagator;
use opentelemetry_sdk::trace::SdkTracerProvider;
use opentelemetry_semantic_conventions::resource::{SERVICE_NAMESPACE, SERVICE_VERSION};
use tracing_subscriber::layer::SubscriberExt;
use tracing_subscriber::util::SubscriberInitExt;
use tracing_subscriber::{EnvFilter, Layer};

use crate::config::Config;

/// Owns the tracer provider so that shutdown can flush it.
pub struct Telemetry {
    provider: Option<SdkTracerProvider>,
}

impl Telemetry {
    /// Installs the global subscriber and, when an OTLP endpoint is configured, the exporter.
    ///
    /// Call once. A second call is a no-op for the subscriber (the `tracing` global can only be
    /// set once) and is reported on stderr rather than panicking.
    pub fn init(config: &Config) -> Self {
        // W3C trace context in and out, over HTTP and NATS headers alike.
        global::set_text_map_propagator(TraceContextPropagator::new());

        let filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| {
            EnvFilter::new(format!(
                "{level},hyper=warn,hyper_util=warn,h2=warn,reqwest=warn,tower=warn,redis=warn,\
                 opentelemetry=warn,opentelemetry_sdk=warn",
                level = config.log_level
            ))
        });

        let provider = config
            .otlp_endpoint
            .as_deref()
            .and_then(|endpoint| build_provider(config, endpoint));

        let otel_layer = provider.as_ref().map(|provider| {
            let tracer = provider.tracer(crate::config::SERVICE_NAME);
            tracing_opentelemetry::layer()
                .with_tracer(tracer)
                .with_error_records_to_exceptions(true)
                .boxed()
        });

        let registry = tracing_subscriber::registry()
            .with(filter)
            .with(json_log::JsonLogLayer::new(config.service_name.clone()))
            .with(otel_layer);

        if registry.try_init().is_err() {
            eprintln!("spurving-bearing-quoter: tracing subscriber already installed");
        }

        Self { provider }
    }

    /// Flushes and shuts down the exporter. Bounded, so a wedged collector cannot hold the pod in
    /// `Terminating`.
    pub fn shutdown(self) {
        if let Some(provider) = self.provider
            && let Err(err) = provider.shutdown_with_timeout(Duration::from_secs(5))
        {
            eprintln!("spurving-bearing-quoter: tracer shutdown failed: {err}");
        }
    }
}

fn build_provider(config: &Config, endpoint: &str) -> Option<SdkTracerProvider> {
    // The exporter resolves the endpoint from OTEL_EXPORTER_OTLP_ENDPOINT itself and appends
    // `/v1/traces`, which is exactly the contract in appendix A. Passing it explicitly here would
    // skip that path suffix.
    let exporter = match opentelemetry_otlp::SpanExporter::builder()
        .with_http()
        .with_timeout(crate::config::OUTBOUND_TIMEOUT)
        .build()
    {
        Ok(exporter) => exporter,
        Err(err) => {
            eprintln!(
                "spurving-bearing-quoter: OTLP exporter for {endpoint} could not be built ({err}); \
                 continuing without tracing"
            );
            return None;
        }
    };

    let resource = Resource::builder()
        .with_service_name(config.service_name.clone())
        .with_attributes([
            KeyValue::new(SERVICE_NAMESPACE, crate::config::SERVICE_NAMESPACE),
            KeyValue::new(SERVICE_VERSION, crate::config::SERVICE_VERSION),
            KeyValue::new("encabulator.io/plane", "processing"),
        ])
        .build();

    Some(
        SdkTracerProvider::builder()
            .with_batch_exporter(exporter)
            .with_resource(resource)
            .build(),
    )
}

/// Extracts a W3C trace context from inbound HTTP headers.
pub fn extract_context(headers: &http::HeaderMap) -> opentelemetry::Context {
    global::get_text_map_propagator(|propagator| {
        propagator.extract(&opentelemetry_http::HeaderExtractor(headers))
    })
}

/// Injects the current W3C trace context into outbound HTTP headers. Unused by the quoter's own
/// polling loops, which the design requires to stay untraced, but kept here so any future outbound
/// call gets propagation for free.
pub fn inject_context(context: &opentelemetry::Context, headers: &mut http::HeaderMap) {
    global::get_text_map_propagator(|propagator| {
        propagator.inject_context(context, &mut opentelemetry_http::HeaderInjector(headers));
    });
}

#[cfg(test)]
mod tests {
    use super::*;
    use http::HeaderMap;

    #[test]
    fn extracting_an_absent_trace_context_yields_an_invalid_span() {
        use opentelemetry::trace::TraceContextExt;
        global::set_text_map_propagator(TraceContextPropagator::new());
        let context = extract_context(&HeaderMap::new());
        assert!(!context.span().span_context().is_valid());
    }

    #[test]
    fn extracts_a_w3c_traceparent() {
        use opentelemetry::trace::TraceContextExt;
        global::set_text_map_propagator(TraceContextPropagator::new());
        let mut headers = HeaderMap::new();
        headers.insert(
            "traceparent",
            "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
                .parse()
                .unwrap(),
        );
        let context = extract_context(&headers);
        let span_context = context.span().span_context().clone();
        assert!(span_context.is_valid());
        assert_eq!(
            span_context.trace_id().to_string(),
            "4bf92f3577b34da6a3ce929d0e0e4736"
        );
        assert_eq!(span_context.span_id().to_string(), "00f067aa0ba902b7");
    }

    #[test]
    fn round_trips_a_context_through_headers() {
        use opentelemetry::trace::TraceContextExt;
        global::set_text_map_propagator(TraceContextPropagator::new());
        let mut inbound = HeaderMap::new();
        inbound.insert(
            "traceparent",
            "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
                .parse()
                .unwrap(),
        );
        let context = extract_context(&inbound);
        let mut outbound = HeaderMap::new();
        inject_context(&context, &mut outbound);
        let again = extract_context(&outbound);
        assert_eq!(
            again.span().span_context().trace_id(),
            context.span().span_context().trace_id()
        );
    }
}
