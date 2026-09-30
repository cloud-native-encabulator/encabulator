//! A minimal container harness for the integration tests.
//!
//! ## Why not Testcontainers
//!
//! The design (section 7, "Testing") calls for Testcontainers. `testcontainers` 0.28 speaks to the
//! daemon through `bollard`, whose `ContainerCreateResponse` requires a `Warnings` field:
//!
//! ```text
//! Client(CreateContainer(JsonDataError { message: "missing field `Warnings`" }))
//! ```
//!
//! finch-daemon v0.23 answers `POST /containers/create` with `{"Id":"..."}` and no `Warnings`, so
//! every Testcontainers-Rust call fails before the container is usable. The Go, Node, and Python
//! Testcontainers clients tolerate the omission; the Rust one does not.
//!
//! So this module drives the container through the CLI instead: same image, same behaviour, same
//! clean skip when no runtime is configured, and no dependency on a client that cannot reach the
//! daemon this repository actually ships with.

use std::net::TcpListener;
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

/// The Valkey image the design pins (section 2, "Backing stores").
const VALKEY_IMAGE: &str = "valkey/valkey:9";

/// How long to wait for the image to pull and the server to answer PING.
const READY_TIMEOUT: Duration = Duration::from_secs(180);

/// True when a Docker-compatible API is configured. `dev-env.sh` sets this for Finch.
#[must_use]
pub fn container_runtime_available() -> bool {
    std::env::var("DOCKER_HOST").is_ok_and(|v| !v.trim().is_empty()) && container_cli().is_some()
}

/// Finds a container CLI. Finch first, since that is what this repository documents.
fn container_cli() -> Option<&'static str> {
    for cli in ["finch", "docker", "nerdctl"] {
        let found = Command::new(cli)
            .arg("--version")
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()
            .is_ok_and(|status| status.success());
        if found {
            return Some(cli);
        }
    }
    None
}

/// Skips the enclosing test when no container runtime is configured.
#[macro_export]
macro_rules! skip_without_runtime {
    ($name:literal) => {
        if !$crate::support::container_runtime_available() {
            eprintln!(
                "skipping {}: no container runtime (DOCKER_HOST unset)",
                $name
            );
            return;
        }
    };
}

/// A running Valkey container. Removed on drop, so a panicking test leaves nothing behind.
pub struct Valkey {
    cli: &'static str,
    name: String,
    port: u16,
}

impl Valkey {
    /// Starts `valkey/valkey:9` on an ephemeral loopback port and waits until it answers PING.
    pub async fn start() -> Self {
        let cli = container_cli().expect("a container CLI");
        let name = format!(
            "encabulator-quoter-valkey-{}",
            uuid::Uuid::now_v7().simple()
        );
        let port = free_port();

        let output = Command::new(cli)
            .args([
                "run",
                "-d",
                "--name",
                &name,
                "-p",
                &format!("127.0.0.1:{port}:6379"),
                VALKEY_IMAGE,
            ])
            .output()
            .expect("container run");
        assert!(
            output.status.success(),
            "{cli} run failed: {}",
            String::from_utf8_lossy(&output.stderr)
        );

        let valkey = Self { cli, name, port };
        valkey.wait_until_ready().await;
        valkey
    }

    #[must_use]
    pub fn url(&self) -> String {
        format!("redis://127.0.0.1:{}/0", self.port)
    }

    async fn wait_until_ready(&self) {
        let started = Instant::now();
        let client = redis::Client::open(self.url()).expect("valkey url");
        let mut last_error = None;
        while started.elapsed() < READY_TIMEOUT {
            match client.get_multiplexed_async_connection().await {
                Ok(mut conn) => {
                    let pong: Result<String, _> = redis::cmd("PING").query_async(&mut conn).await;
                    if pong.as_deref() == Ok("PONG") {
                        return;
                    }
                    last_error = pong.err().map(|e| e.to_string());
                }
                Err(err) => last_error = Some(err.to_string()),
            }
            tokio::time::sleep(Duration::from_millis(250)).await;
        }
        panic!(
            "valkey never became ready on port {}: {}",
            self.port,
            last_error.unwrap_or_else(|| "no error recorded".to_owned())
        );
    }

    /// Kills the container so a test can watch the quoter degrade.
    pub fn kill(&self) {
        let _ = Command::new(self.cli)
            .args(["kill", &self.name])
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status();
    }
}

impl Drop for Valkey {
    fn drop(&mut self) {
        let _ = Command::new(self.cli)
            .args(["rm", "-f", &self.name])
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status();
    }
}

/// Reserves a port by binding and immediately releasing it. Racy in principle, reliable in practice,
/// and it avoids parsing three CLIs' `port` output.
fn free_port() -> u16 {
    TcpListener::bind("127.0.0.1:0")
        .expect("bind an ephemeral port")
        .local_addr()
        .expect("local addr")
        .port()
}
