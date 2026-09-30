//! A `tracing` layer that emits the exact JSON log line the design asks for.
//!
//! Design section 7, "Logging":
//!
//! > JSON lines on stdout. Required fields: `time` (RFC 3339 with ms), `level`, `msg`, `service`.
//! > Include `trace_id` and `span_id` when a span is active, and `request_id` for HTTP requests.
//!
//! `tracing_subscriber`'s own JSON formatter emits `timestamp`/`message`/`target`, which is close
//! but not the contract, and it knows nothing about OpenTelemetry ids. This layer writes the line
//! itself: fixed fields first in a stable order, then span and event fields.

use std::collections::BTreeMap;
use std::fmt;
use std::io::Write;
use std::sync::Arc;

use chrono::{SecondsFormat, Utc};
use opentelemetry::trace::TraceContextExt;
use serde_json::Value;
use tracing::field::{Field, Visit};
use tracing::{Event, Subscriber};
use tracing_opentelemetry::OpenTelemetrySpanExt;
use tracing_subscriber::layer::{Context, Layer};
use tracing_subscriber::registry::LookupSpan;

/// Field name prefixes that are instrumentation plumbing rather than information: `otel.*` steers
/// the OpenTelemetry layer, `log.*` comes from the `log` compatibility shim.
const HIDDEN_PREFIXES: [&str; 2] = ["otel.", "log."];

/// Fields emitted before everything else, in this order, and never repeated later.
const LEADING_FIELDS: [&str; 3] = ["trace_id", "span_id", "request_id"];

/// Per-span field storage, kept in the registry's span extensions.
#[derive(Default, Debug)]
struct SpanFields(BTreeMap<String, Value>);

pub struct JsonLogLayer {
    service: Arc<str>,
}

impl JsonLogLayer {
    pub fn new(service: impl Into<Arc<str>>) -> Self {
        Self {
            service: service.into(),
        }
    }
}

impl<S> Layer<S> for JsonLogLayer
where
    S: Subscriber + for<'a> LookupSpan<'a>,
{
    fn on_new_span(
        &self,
        attrs: &tracing::span::Attributes<'_>,
        id: &tracing::span::Id,
        ctx: Context<'_, S>,
    ) {
        let Some(span) = ctx.span(id) else { return };
        let mut visitor = FieldVisitor::default();
        attrs.record(&mut visitor);
        span.extensions_mut().insert(SpanFields(visitor.fields));
    }

    fn on_record(
        &self,
        id: &tracing::span::Id,
        values: &tracing::span::Record<'_>,
        ctx: Context<'_, S>,
    ) {
        let Some(span) = ctx.span(id) else { return };
        let mut visitor = FieldVisitor::default();
        values.record(&mut visitor);
        let mut extensions = span.extensions_mut();
        if let Some(existing) = extensions.get_mut::<SpanFields>() {
            existing.0.extend(visitor.fields);
        } else {
            extensions.insert(SpanFields(visitor.fields));
        }
    }

    fn on_event(&self, event: &Event<'_>, ctx: Context<'_, S>) {
        // Resolve the OpenTelemetry ids first. `OpenTelemetrySpanExt::context` locks the span's
        // extensions internally, so no extension borrow may be held across this call.
        let (trace_id, span_id) = current_trace_ids();

        let mut fields: BTreeMap<String, Value> = BTreeMap::new();
        if let Some(scope) = ctx.event_scope(event) {
            for span in scope.from_root() {
                if let Some(stored) = span.extensions().get::<SpanFields>() {
                    for (key, value) in &stored.0 {
                        fields.insert(key.clone(), value.clone());
                    }
                }
            }
        }

        let mut visitor = FieldVisitor::default();
        event.record(&mut visitor);
        fields.extend(visitor.fields);

        // Some libraries (the OpenTelemetry SDK's internal logs among them) name an event instead of
        // giving it a message. Promote that name rather than emitting an empty `msg`.
        let message = visitor
            .message
            .unwrap_or_else(|| match fields.remove("name") {
                Some(Value::String(name)) => name,
                Some(other) => other.to_string(),
                None => event.metadata().name().to_owned(),
            });

        let mut line = String::with_capacity(256);
        line.push('{');
        push_field(
            &mut line,
            "time",
            &Value::String(Utc::now().to_rfc3339_opts(SecondsFormat::Millis, true)),
        );
        push_field(
            &mut line,
            "level",
            &Value::String(event.metadata().level().as_str().to_ascii_lowercase()),
        );
        push_field(&mut line, "msg", &Value::String(message));
        push_field(
            &mut line,
            "service",
            &Value::String(self.service.to_string()),
        );
        if let Some(trace_id) = trace_id {
            push_field(&mut line, "trace_id", &Value::String(trace_id));
        }
        if let Some(span_id) = span_id {
            push_field(&mut line, "span_id", &Value::String(span_id));
        }
        for key in LEADING_FIELDS {
            if let Some(value) = fields.remove(key) {
                push_field(&mut line, key, &value);
            }
        }
        for (key, value) in &fields {
            push_field(&mut line, key, value);
        }
        // Trim the trailing comma left by the last field.
        if line.ends_with(',') {
            line.pop();
        }
        line.push_str("}\n");

        // One write, so concurrent tasks cannot interleave halves of a line.
        let _ = std::io::stdout().write_all(line.as_bytes());
    }
}

/// Reads the active OpenTelemetry span context, if any. Returns `(None, None)` when no
/// OpenTelemetry layer is installed, which is the case whenever `OTEL_EXPORTER_OTLP_ENDPOINT` is
/// unset, so log output stays quiet in tests and local runs.
fn current_trace_ids() -> (Option<String>, Option<String>) {
    let context = tracing::Span::current().context();
    let span = context.span();
    let span_context = span.span_context();
    if span_context.is_valid() {
        (
            Some(span_context.trace_id().to_string()),
            Some(span_context.span_id().to_string()),
        )
    } else {
        (None, None)
    }
}

fn push_field(out: &mut String, key: &str, value: &Value) {
    out.push_str(&serde_json::to_string(key).unwrap_or_else(|_| "\"?\"".to_owned()));
    out.push(':');
    out.push_str(&serde_json::to_string(value).unwrap_or_else(|_| "null".to_owned()));
    out.push(',');
}

#[derive(Default)]
struct FieldVisitor {
    fields: BTreeMap<String, Value>,
    message: Option<String>,
}

impl FieldVisitor {
    fn insert(&mut self, field: &Field, value: Value) {
        let name = field.name();
        if name == "message" {
            self.message = match value {
                Value::String(s) => Some(s),
                other => Some(other.to_string()),
            };
            return;
        }
        if HIDDEN_PREFIXES.iter().any(|p| name.starts_with(p)) {
            return;
        }
        self.fields.insert(name.to_owned(), value);
    }
}

impl Visit for FieldVisitor {
    fn record_str(&mut self, field: &Field, value: &str) {
        self.insert(field, Value::String(value.to_owned()));
    }

    fn record_bool(&mut self, field: &Field, value: bool) {
        self.insert(field, Value::Bool(value));
    }

    fn record_i64(&mut self, field: &Field, value: i64) {
        self.insert(field, Value::Number(value.into()));
    }

    fn record_u64(&mut self, field: &Field, value: u64) {
        self.insert(field, Value::Number(value.into()));
    }

    fn record_i128(&mut self, field: &Field, value: i128) {
        self.insert(field, Value::String(value.to_string()));
    }

    fn record_u128(&mut self, field: &Field, value: u128) {
        self.insert(field, Value::String(value.to_string()));
    }

    fn record_f64(&mut self, field: &Field, value: f64) {
        match serde_json::Number::from_f64(value) {
            Some(number) => self.insert(field, Value::Number(number)),
            None => self.insert(field, Value::String(value.to_string())),
        }
    }

    fn record_error(&mut self, field: &Field, value: &(dyn std::error::Error + 'static)) {
        self.insert(field, Value::String(value.to_string()));
    }

    fn record_debug(&mut self, field: &Field, value: &dyn fmt::Debug) {
        self.insert(field, Value::String(format!("{value:?}")));
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn hides_instrumentation_fields_and_keeps_the_rest() {
        assert!(HIDDEN_PREFIXES.iter().any(|p| "otel.kind".starts_with(p)));
        assert!(HIDDEN_PREFIXES.iter().any(|p| "log.target".starts_with(p)));
        assert!(!HIDDEN_PREFIXES.iter().any(|p| "http.route".starts_with(p)));
    }

    #[test]
    fn push_field_escapes_values() {
        let mut out = String::new();
        push_field(
            &mut out,
            "msg",
            &Value::String("he said \"malaise\"\n".to_owned()),
        );
        assert_eq!(out, r#""msg":"he said \"malaise\"\n","#);
    }

    #[test]
    fn trace_ids_are_absent_without_an_opentelemetry_layer() {
        let (trace_id, span_id) = current_trace_ids();
        assert!(trace_id.is_none());
        assert!(span_id.is_none());
    }
}
