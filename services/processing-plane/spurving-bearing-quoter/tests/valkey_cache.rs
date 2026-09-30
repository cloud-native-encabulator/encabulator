//! Integration test for the Valkey quote cache, against a real `valkey/valkey:9` container.
//!
//! Design section 7, "Testing": integration tests run against real backing stores and "skip
//! cleanly when no container runtime is available". See `tests/support/mod.rs` for why the container
//! is driven by CLI rather than by Testcontainers-Rust.
//!
//! Run it with a container runtime available:
//!
//! ```sh
//! source dev-env.sh && cargo test --test valkey_cache
//! ```
//!
//! With `DOCKER_HOST` unset every test here prints a skip notice and passes.

#[macro_use]
mod support;

use std::time::Duration;

use axum::Router;
use axum::body::{Body, to_bytes};
use http::{Request, StatusCode};
use serde_json::{Value, json};
use spurving_bearing_quoter::app::AppState;
use spurving_bearing_quoter::config::Config;
use spurving_bearing_quoter::flags::{Flag, QUOTE_CACHE};
use spurving_bearing_quoter::http::router;
use support::Valkey;
use tower::ServiceExt;

async fn start_valkey() -> (Valkey, String) {
    let valkey = Valkey::start().await;
    let url = valkey.url();
    (valkey, url)
}

/// Builds a quoter wired to a real Valkey, with the connection already established.
async fn app_with_valkey(url: &str) -> (Router, AppState) {
    let mut config = Config::for_tests();
    config.valkey_url = Some(url.to_owned());
    let state = AppState::new(config).expect("state");

    // The connector normally runs in the background; drive it directly so the test is not racy.
    for _ in 0..50 {
        if state.store.ensure_connected().await {
            break;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    assert!(
        state.store.ensure_connected().await,
        "could not connect to Valkey at {url}"
    );
    (router(state.clone()), state)
}

async fn send(app: Router, request: Request<Body>) -> (StatusCode, Value) {
    let response = app.oneshot(request).await.expect("router responded");
    let status = response.status();
    let bytes = to_bytes(response.into_body(), 1024 * 1024).await.unwrap();
    let body = if bytes.is_empty() {
        Value::Null
    } else {
        serde_json::from_slice(&bytes).unwrap()
    };
    (status, body)
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

#[tokio::test]
async fn a_quote_can_be_fetched_back_by_id() {
    skip_without_runtime!("a_quote_can_be_fetched_back_by_id");
    let (_valkey, url) = start_valkey().await;
    let (app, _state) = app_with_valkey(&url).await;

    let (status, created) = send(
        app.clone(),
        post_quote(json!({"model": "hyper", "quantity": 3, "options": {"dingleArm": true}})),
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(created["cached"], false);
    let id = created["id"].as_str().unwrap().to_owned();

    let (status, fetched) = send(app, get(&format!("/v1/quotes/{id}"))).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(fetched["id"], created["id"]);
    assert_eq!(fetched["model"], "hyper");
    assert_eq!(fetched["quantity"], 3);
    assert_eq!(fetched["total"], created["total"]);
    assert_eq!(fetched["surcharges"], created["surcharges"]);
    assert_eq!(fetched["computedAt"], created["computedAt"]);
    // Anything served out of Valkey is marked cached.
    assert_eq!(fetched["cached"], true);
}

#[tokio::test]
async fn an_unknown_id_is_a_404_when_valkey_is_healthy() {
    skip_without_runtime!("an_unknown_id_is_a_404_when_valkey_is_healthy");
    let (_valkey, url) = start_valkey().await;
    let (app, _state) = app_with_valkey(&url).await;

    let (status, body) = send(app, get("/v1/quotes/qte_never-existed")).await;
    assert_eq!(status, StatusCode::NOT_FOUND);
    assert_eq!(body["error"]["code"], "quote_not_found");
}

#[tokio::test]
async fn identical_inputs_are_served_from_the_cache() {
    skip_without_runtime!("identical_inputs_are_served_from_the_cache");
    let (_valkey, url) = start_valkey().await;
    let (app, state) = app_with_valkey(&url).await;

    let body = json!({"model": "turbo", "quantity": 5, "options": {"dingleArm": true}});
    let (status, first) = send(app.clone(), post_quote(body.clone())).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(first["cached"], false);

    let (status, second) = send(app.clone(), post_quote(body.clone())).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(
        second["cached"], true,
        "second identical request should hit"
    );
    // A cache hit returns the *same* quote, not an equal one.
    assert_eq!(second["id"], first["id"]);
    assert_eq!(second["computedAt"], first["computedAt"]);
    assert_eq!(second["total"], first["total"]);

    // A different input must miss.
    let (status, other) = send(
        app,
        post_quote(json!({"model": "turbo", "quantity": 6, "options": {"dingleArm": true}})),
    )
    .await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(other["cached"], false);
    assert_ne!(other["id"], first["id"]);

    let text = state.metrics.encode();
    assert!(
        text.contains("encabulator_quotes_total{cached=\"true\",model=\"turbo\"} 1"),
        "{text}"
    );
    assert!(
        text.contains("encabulator_quotes_total{cached=\"false\",model=\"turbo\"} 2"),
        "{text}"
    );
}

#[tokio::test]
async fn the_quote_cache_flag_off_bypasses_the_cache_but_keeps_retrieval_by_id() {
    skip_without_runtime!("the_quote_cache_flag_off_bypasses_the_cache_but_keeps_retrieval_by_id");
    let (_valkey, url) = start_valkey().await;
    let (app, state) = app_with_valkey(&url).await;
    state.flags.install(vec![Flag {
        key: QUOTE_CACHE.to_owned(),
        r#type: Some("boolean".to_owned()),
        value: json!(false),
        rollout_percent: None,
        description: None,
        updated_at: None,
        updated_by: None,
    }]);

    let body = json!({"model": "retro", "quantity": 2});
    let (_, first) = send(app.clone(), post_quote(body.clone())).await;
    let (_, second) = send(app.clone(), post_quote(body)).await;
    assert_eq!(first["cached"], false);
    assert_eq!(second["cached"], false, "the cache must be bypassed");
    assert_ne!(first["id"], second["id"], "each request gets a fresh quote");

    // GET by id still works: quote:{id} is written regardless of the flag.
    let id = second["id"].as_str().unwrap();
    let (status, fetched) = send(app, get(&format!("/v1/quotes/{id}"))).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(fetched["id"], second["id"]);
}

#[tokio::test]
async fn the_keys_written_match_the_documented_layout_and_ttls() {
    skip_without_runtime!("the_keys_written_match_the_documented_layout_and_ttls");
    let (_valkey, url) = start_valkey().await;
    let (app, _state) = app_with_valkey(&url).await;

    let (_, quote) = send(
        app,
        post_quote(json!({"model": "turbo", "quantity": 1, "options": {"dingleArm": false}})),
    )
    .await;
    let id = quote["id"].as_str().unwrap();

    let client = redis::Client::open(url.as_str()).unwrap();
    let mut conn = client.get_multiplexed_async_connection().await.unwrap();

    // Every key the quoter owns lives under `quote:*` (design section 2, service catalog).
    let keys: Vec<String> = redis::cmd("KEYS")
        .arg("*")
        .query_async(&mut conn)
        .await
        .unwrap();
    assert!(!keys.is_empty());
    for key in &keys {
        assert!(key.starts_with("quote:"), "{key} is outside quote:*");
    }
    assert!(keys.contains(&format!("quote:{id}")));
    assert!(keys.contains(&"quote:input:turbo:1:false:false".to_owned()));

    // quote:{id} keeps an hour, the input cache keeps a minute.
    let quote_ttl: i64 = redis::cmd("TTL")
        .arg(format!("quote:{id}"))
        .query_async(&mut conn)
        .await
        .unwrap();
    assert!(
        (3_500..=3_600).contains(&quote_ttl),
        "quote TTL was {quote_ttl}"
    );
    let cache_ttl: i64 = redis::cmd("TTL")
        .arg("quote:input:turbo:1:false:false")
        .query_async(&mut conn)
        .await
        .unwrap();
    assert!((50..=60).contains(&cache_ttl), "cache TTL was {cache_ttl}");
}

#[tokio::test]
async fn losing_valkey_mid_flight_degrades_instead_of_failing() {
    skip_without_runtime!("losing_valkey_mid_flight_degrades_instead_of_failing");
    let (valkey, url) = start_valkey().await;
    let (app, state) = app_with_valkey(&url).await;

    let body = json!({"model": "turbo", "quantity": 2});
    let (status, _) = send(app.clone(), post_quote(body.clone())).await;
    assert_eq!(status, StatusCode::OK);

    // Valkey is a soft dependency: pricing must survive its disappearance.
    valkey.kill();

    let (status, quote) = send(app.clone(), post_quote(body)).await;
    assert_eq!(status, StatusCode::OK, "pricing must not depend on Valkey");
    assert_eq!(quote["cached"], false);
    assert_eq!(quote["total"], 3888.0);

    // Readiness stays green.
    let (status, readiness) = send(app, get("/readyz")).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(readiness["status"], "ok");
    assert!(
        state
            .metrics
            .encode()
            .contains("encabulator_quote_cache_errors_total")
    );
}
