//! Handler tests driven through `tower::ServiceExt::oneshot`, so the real router, the real
//! middleware stack, and the real extractors are exercised without binding a socket.

use axum::Router;
use axum::body::{Body, to_bytes};
use http::{Request, Response, StatusCode};
use serde_json::{Value, json};
use spurving_bearing_quoter::app::AppState;
use spurving_bearing_quoter::flags::{Flag, QUOTE_CACHE, TURBO_ENCABULATION};
use spurving_bearing_quoter::http::router;
use tower::ServiceExt;

fn app() -> (Router, AppState) {
    let state = AppState::for_tests();
    (router(state.clone()), state)
}

async fn send(app: Router, request: Request<Body>) -> (StatusCode, http::HeaderMap, Value) {
    let response: Response<Body> = app.oneshot(request).await.expect("router responded");
    let status = response.status();
    let headers = response.headers().clone();
    let bytes = to_bytes(response.into_body(), 1024 * 1024).await.unwrap();
    let body = if bytes.is_empty() {
        Value::Null
    } else {
        serde_json::from_slice(&bytes)
            .unwrap_or(Value::String(String::from_utf8_lossy(&bytes).into_owned()))
    };
    (status, headers, body)
}

fn post_quote(body: Value) -> Request<Body> {
    Request::builder()
        .method("POST")
        .uri("/v1/quotes")
        .header("content-type", "application/json")
        .body(Body::from(body.to_string()))
        .unwrap()
}

fn get(uri: &str) -> Request<Body> {
    Request::builder().uri(uri).body(Body::empty()).unwrap()
}

fn boolean_flag(key: &str, value: bool) -> Flag {
    Flag {
        key: key.to_owned(),
        r#type: Some("boolean".to_owned()),
        value: json!(value),
        rollout_percent: None,
        description: None,
        updated_at: None,
        updated_by: None,
    }
}

#[tokio::test]
async fn healthz_is_ok() {
    let (app, _) = app();
    let (status, headers, body) = send(app, get("/healthz")).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(body, json!({"status": "ok"}));
    // Even the probe route echoes a request id.
    assert!(headers.contains_key("x-request-id"));
}

#[tokio::test]
async fn readyz_is_green_with_no_hard_dependencies() {
    let (app, _) = app();
    let (status, _, body) = send(app, get("/readyz")).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(body["status"], "ok");
    // Valkey is reported but never gates readiness.
    assert_eq!(body["quoteCache"], "disabled");
    assert_eq!(body["flags"], "disabled");
}

#[tokio::test]
async fn readyz_fails_while_draining() {
    let (app, state) = app();
    state.begin_draining();
    let (status, _, body) = send(app, get("/readyz")).await;
    assert_eq!(status, StatusCode::SERVICE_UNAVAILABLE);
    assert_eq!(body["status"], "draining");
}

#[tokio::test]
async fn metrics_is_prometheus_text() {
    let (app, _) = app();
    let response = app.oneshot(get("/metrics")).await.unwrap();
    assert_eq!(response.status(), StatusCode::OK);
    let content_type = response
        .headers()
        .get("content-type")
        .unwrap()
        .to_str()
        .unwrap()
        .to_owned();
    assert!(content_type.starts_with("text/plain"), "{content_type}");
    let bytes = to_bytes(response.into_body(), 1024 * 1024).await.unwrap();
    let text = String::from_utf8(bytes.to_vec()).unwrap();
    assert!(text.contains("encabulator_quotes_total"));
}

#[tokio::test]
async fn health_and_metrics_are_not_in_the_server_histogram() {
    let (app, state) = app();
    for uri in ["/healthz", "/readyz", "/metrics"] {
        let (status, _, _) = send(app.clone(), get(uri)).await;
        assert!(status.is_success() || status == StatusCode::SERVICE_UNAVAILABLE);
    }
    let text = state.metrics.encode();
    for uri in ["/healthz", "/readyz", "/metrics"] {
        assert!(
            !text.contains(&format!("http_route=\"{uri}\"")),
            "{uri} leaked into the server histogram"
        );
    }
}

#[tokio::test]
async fn a_quote_has_the_shape_the_design_specifies() {
    let (app, _) = app();
    let (status, headers, body) = send(
        app,
        post_quote(json!({"model": "turbo", "quantity": 1, "options": {"dingleArm": false}})),
    )
    .await;

    assert_eq!(status, StatusCode::OK);
    assert!(headers.contains_key("x-request-id"));
    for field in [
        "id",
        "model",
        "quantity",
        "options",
        "unitPrice",
        "subtotal",
        "surcharges",
        "total",
        "currency",
        "computedAt",
        "cached",
    ] {
        assert!(body.get(field).is_some(), "{field} missing from {body}");
    }
    assert!(body["id"].as_str().unwrap().starts_with("qte_"));
    assert_eq!(body["model"], "turbo");
    assert_eq!(body["quantity"], 1);
    assert_eq!(body["options"], json!({"dingleArm": false}));
    assert_eq!(body["unitPrice"], 1944.0);
    assert_eq!(body["subtotal"], 1944.0);
    assert_eq!(body["surcharges"], json!([]));
    assert_eq!(body["total"], 1944.0);
    assert_eq!(body["currency"], "USD");
    assert_eq!(body["cached"], false);
    assert!(
        body["computedAt"].as_str().unwrap().ends_with('Z'),
        "{}",
        body["computedAt"]
    );
}

#[tokio::test]
async fn options_may_be_omitted_entirely() {
    let (app, _) = app();
    let (status, _, body) = send(app, post_quote(json!({"model": "retro", "quantity": 2}))).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(body["total"], 5498.0);
    assert_eq!(body["options"], json!({"dingleArm": false}));
}

#[tokio::test]
async fn the_dingle_arm_and_the_volume_discount_show_up_as_lines() {
    let (app, _) = app();
    let (status, _, body) = send(
        app,
        post_quote(json!({"model": "turbo", "quantity": 5, "options": {"dingleArm": true}})),
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(body["unitPrice"], 2256.0);
    assert_eq!(body["subtotal"], 11280.0);
    assert_eq!(
        body["surcharges"],
        json!([{"code": "volume_discount", "amount": -564.0}])
    );
    assert_eq!(body["total"], 10716.0);
}

#[tokio::test]
async fn the_turbo_flag_adds_a_surcharge() {
    let (app, state) = app();
    state
        .flags
        .install(vec![boolean_flag(TURBO_ENCABULATION, true)]);
    let (status, _, body) = send(app, post_quote(json!({"model": "turbo", "quantity": 1}))).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(
        body["surcharges"],
        json!([{"code": "turbo_encabulation", "amount": 291.6}])
    );
    assert_eq!(body["total"], 2235.6);
}

#[tokio::test]
async fn an_unknown_model_is_a_400_with_the_documented_code() {
    let (app, _) = app();
    let (status, _, body) = send(
        app,
        post_quote(json!({"model": "pentametric", "quantity": 1})),
    )
    .await;
    assert_eq!(status, StatusCode::BAD_REQUEST);
    assert_eq!(body["error"]["code"], "unknown_model");
    assert!(body["error"]["message"].as_str().unwrap().contains("turbo"));
}

#[tokio::test]
async fn quantities_outside_one_to_ten_are_rejected() {
    for quantity in [0, 11, 1_000_000] {
        let (app, _) = app();
        let (status, _, body) = send(
            app,
            post_quote(json!({"model": "turbo", "quantity": quantity})),
        )
        .await;
        assert_eq!(status, StatusCode::BAD_REQUEST, "quantity {quantity}");
        assert_eq!(body["error"]["code"], "invalid_quantity");
    }
}

#[tokio::test]
async fn malformed_bodies_are_rejected_with_the_standard_envelope() {
    let cases: Vec<(&str, &str, StatusCode)> = vec![
        ("{not json", "application/json", StatusCode::BAD_REQUEST),
        (
            r#"{"model":"turbo"}"#,
            "application/json",
            StatusCode::BAD_REQUEST,
        ),
        (
            r#"{"model":"turbo","quantity":1}"#,
            "text/plain",
            StatusCode::UNSUPPORTED_MEDIA_TYPE,
        ),
    ];
    for (body, content_type, expected) in cases {
        let (app, _) = app();
        let request = Request::builder()
            .method("POST")
            .uri("/v1/quotes")
            .header("content-type", content_type)
            .body(Body::from(body.to_owned()))
            .unwrap();
        let (status, _, payload) = send(app, request).await;
        assert_eq!(status, expected, "{body} / {content_type}");
        assert!(
            payload["error"]["code"].is_string(),
            "no error envelope for {body}"
        );
    }
}

#[tokio::test]
async fn an_unmatched_route_is_a_404_with_the_standard_envelope() {
    let (app, _) = app();
    let (status, _, body) = send(app, get("/v1/nofer-trunnions")).await;
    assert_eq!(status, StatusCode::NOT_FOUND);
    assert_eq!(body["error"]["code"], "not_found");
}

#[tokio::test]
async fn the_wrong_method_is_a_405_with_the_standard_envelope() {
    let (app, _) = app();
    let (status, _, body) = send(app, get("/v1/quotes")).await;
    assert_eq!(status, StatusCode::METHOD_NOT_ALLOWED);
    assert_eq!(body["error"]["code"], "method_not_allowed");
}

#[tokio::test]
async fn getting_a_quote_by_id_without_valkey_is_a_404() {
    let (app, _) = app();
    let (status, _, body) = send(app, get("/v1/quotes/qte_does-not-exist")).await;
    assert_eq!(status, StatusCode::NOT_FOUND);
    assert_eq!(body["error"]["code"], "quote_not_found");
}

#[tokio::test]
async fn getting_a_quote_by_id_with_valkey_down_is_a_503() {
    // Configured but unreachable: answering 404 here would be a lie the processor could act on.
    let mut config = spurving_bearing_quoter::config::Config::for_tests();
    config.valkey_url = Some("redis://127.0.0.1:1/0".to_owned());
    let state = AppState::new(config).unwrap();
    let (status, _, body) = send(router(state), get("/v1/quotes/qte_anything")).await;
    assert_eq!(status, StatusCode::SERVICE_UNAVAILABLE);
    assert_eq!(body["error"]["code"], "dependency_unavailable");
}

#[tokio::test]
async fn a_client_request_id_is_echoed_back() {
    let (app, _) = app();
    let request = Request::builder()
        .method("POST")
        .uri("/v1/quotes")
        .header("content-type", "application/json")
        .header("x-request-id", "req-spurving-001")
        .body(Body::from(
            json!({"model":"turbo","quantity":1}).to_string(),
        ))
        .unwrap();
    let (status, headers, _) = send(app, request).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(headers["x-request-id"], "req-spurving-001");
}

#[tokio::test]
async fn a_generated_request_id_is_returned_when_the_client_sends_none() {
    let (app, _) = app();
    let (_, headers, _) = send(app, post_quote(json!({"model":"turbo","quantity":1}))).await;
    let id = headers["x-request-id"].to_str().unwrap();
    assert!(uuid::Uuid::parse_str(id).is_ok(), "{id}");
}

#[tokio::test]
async fn w3c_trace_context_is_accepted_without_an_exporter() {
    // With OTEL_EXPORTER_OTLP_ENDPOINT unset the span is a no-op, but the header must not break
    // the request.
    let (app, _) = app();
    let request = Request::builder()
        .method("POST")
        .uri("/v1/quotes")
        .header("content-type", "application/json")
        .header(
            "traceparent",
            "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
        )
        .body(Body::from(
            json!({"model":"hyper","quantity":1}).to_string(),
        ))
        .unwrap();
    let (status, _, body) = send(app, request).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(body["total"], 4096.0);
}

#[tokio::test]
async fn the_server_histogram_records_the_route_template_not_the_path() {
    let (app, state) = app();
    let (status, _, _) = send(app.clone(), get("/v1/quotes/qte_12345")).await;
    assert_eq!(status, StatusCode::NOT_FOUND);
    let text = state.metrics.encode();
    assert!(
        text.contains("http_route=\"/v1/quotes/{id}\""),
        "route template missing:\n{text}"
    );
    assert!(
        !text.contains("qte_12345"),
        "the raw path leaked into a label:\n{text}"
    );
}

#[tokio::test]
async fn the_server_histogram_bounds_unmatched_route_cardinality() {
    let (app, state) = app();
    for path in ["/wat", "/also-wat", "/v9/quotes"] {
        send(app.clone(), get(path)).await;
    }
    let text = state.metrics.encode();
    assert!(text.contains("http_route=\"unmatched\""));
    for path in ["/wat", "/also-wat", "/v9/quotes"] {
        assert!(!text.contains(&format!("http_route=\"{path}\"")));
    }
}

#[tokio::test]
async fn quotes_are_counted_by_model_and_cache_state() {
    let (app, state) = app();
    send(
        app.clone(),
        post_quote(json!({"model":"retro","quantity":1})),
    )
    .await;
    let text = state.metrics.encode();
    assert!(
        text.contains("encabulator_quotes_total{cached=\"false\",model=\"retro\"} 1"),
        "{text}"
    );
    assert!(text.contains("encabulator_quote_compute_seconds"));
}

#[tokio::test]
async fn the_quote_cache_flag_off_still_prices_normally() {
    let (app, state) = app();
    state.flags.install(vec![boolean_flag(QUOTE_CACHE, false)]);
    let (status, _, body) = send(app, post_quote(json!({"model":"turbo","quantity":1}))).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(body["cached"], false);
}

#[tokio::test]
async fn injected_error_malaise_short_circuits_the_handler() {
    use spurving_bearing_quoter::malaise::{Directive, Kind, Params};

    let (app, state) = app();
    state.malaise.install(vec![Directive {
        id: "mal_test_error".to_owned(),
        kind: Kind::Error,
        params: Params {
            status: Some(503),
            percent: Some(100),
            ..Params::default()
        },
        expires_at: chrono::Utc::now() + chrono::TimeDelta::seconds(60),
    }]);

    let (status, _, body) = send(app, post_quote(json!({"model":"turbo","quantity":1}))).await;
    assert_eq!(status, StatusCode::SERVICE_UNAVAILABLE);
    assert_eq!(body["error"]["code"], "malaise_injected");
    assert!(
        state
            .metrics
            .encode()
            .contains("encabulator_malaise_applied_total{kind=\"error\"} 1")
    );
    // The injected failure is measured like any other response.
    assert!(
        state
            .metrics
            .encode()
            .contains("http_response_status_code=\"503\"")
    );
}

#[tokio::test]
async fn injected_latency_malaise_delays_the_handler() {
    use spurving_bearing_quoter::malaise::{Directive, Kind, Params};

    let (app, state) = app();
    state.malaise.install(vec![Directive {
        id: "mal_test_latency".to_owned(),
        kind: Kind::Latency,
        params: Params {
            ms: Some(150),
            percent: Some(100),
            ..Params::default()
        },
        expires_at: chrono::Utc::now() + chrono::TimeDelta::seconds(60),
    }]);

    let started = std::time::Instant::now();
    let (status, _, _) = send(app, post_quote(json!({"model":"turbo","quantity":1}))).await;
    let elapsed = started.elapsed();
    assert_eq!(status, StatusCode::OK);
    assert!(
        elapsed >= std::time::Duration::from_millis(150),
        "only waited {elapsed:?}"
    );
    assert!(
        state
            .metrics
            .encode()
            .contains("encabulator_malaise_applied_total{kind=\"latency\"} 1")
    );
}

#[tokio::test]
async fn injected_congestion_malaise_rejects_the_excess() {
    use spurving_bearing_quoter::malaise::{Directive, Kind, Params};

    let (app, state) = app();
    state.malaise.install(vec![
        Directive {
            id: "mal_test_congestion".to_owned(),
            kind: Kind::Congestion,
            params: Params {
                max_concurrent: Some(1),
                queue_timeout_ms: Some(30),
                ..Params::default()
            },
            expires_at: chrono::Utc::now() + chrono::TimeDelta::seconds(60),
        },
        // Latency holds the single permit long enough for the second request to give up queueing.
        Directive {
            id: "mal_test_hold".to_owned(),
            kind: Kind::Latency,
            params: Params {
                ms: Some(300),
                percent: Some(100),
                ..Params::default()
            },
            expires_at: chrono::Utc::now() + chrono::TimeDelta::seconds(60),
        },
    ]);

    let first = tokio::spawn(send(
        app.clone(),
        post_quote(json!({"model":"turbo","quantity":1})),
    ));
    tokio::time::sleep(std::time::Duration::from_millis(40)).await;
    let (status, _, body) = send(app, post_quote(json!({"model":"turbo","quantity":1}))).await;

    assert_eq!(status, StatusCode::SERVICE_UNAVAILABLE);
    assert_eq!(body["error"]["code"], "congested");

    let (first_status, _, _) = first.await.unwrap();
    assert_eq!(
        first_status,
        StatusCode::OK,
        "the admitted request still succeeded"
    );
    assert!(
        state
            .metrics
            .encode()
            .contains("encabulator_malaise_applied_total{kind=\"congestion\"}")
    );
}

#[tokio::test]
async fn malaise_never_touches_health_or_metrics() {
    use spurving_bearing_quoter::malaise::{Directive, Kind, Params};

    let (app, state) = app();
    state.malaise.install(vec![Directive {
        id: "mal_test_total".to_owned(),
        kind: Kind::Error,
        params: Params {
            status: Some(503),
            percent: Some(100),
            ..Params::default()
        },
        expires_at: chrono::Utc::now() + chrono::TimeDelta::seconds(60),
    }]);

    for uri in ["/healthz", "/metrics"] {
        let (status, _, _) = send(app.clone(), get(uri)).await;
        assert_eq!(status, StatusCode::OK, "{uri} was affected by malaise");
    }
    let (status, _, _) = send(app, get("/readyz")).await;
    assert_eq!(status, StatusCode::OK);
}

#[tokio::test]
async fn quoting_stays_responsive_under_a_full_pentametric_burn() {
    // The burn runs on a bounded blocking pool, so /healthz answers while quotes are pricing.
    let mut config = spurving_bearing_quoter::config::Config::for_tests();
    config.pentametric_burn_scale = 1.0;
    config.pentametric_workers = 2;
    let state = AppState::new(config).unwrap();
    let app = router(state);

    let quotes = (0..8).map(|i| {
        let app = app.clone();
        let model = ["turbo", "retro", "hyper"][i % 3];
        tokio::spawn(
            async move { send(app, post_quote(json!({"model": model, "quantity": 10}))).await },
        )
    });
    let probes = (0..8).map(|_| {
        let app = app.clone();
        tokio::spawn(async move { send(app, get("/healthz")).await })
    });

    for handle in probes {
        let (status, _, _) = handle.await.unwrap();
        assert_eq!(status, StatusCode::OK);
    }
    for handle in quotes {
        let (status, _, body) = handle.await.unwrap();
        assert_eq!(status, StatusCode::OK);
        assert!(body["total"].as_f64().unwrap() > 0.0);
    }
}
