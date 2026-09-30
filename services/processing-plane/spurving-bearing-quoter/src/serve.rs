//! The HTTP server: connection timeouts and graceful shutdown (design section 7).
//!
//! > Timeouts: server read-header 5s, idle 60s.
//! > Graceful shutdown on SIGTERM: fail readiness, stop accepting, drain for up to 20s, exit.
//!
//! `axum::serve` exposes neither timeout, so this is a hyper accept loop instead. hyper's
//! `header_read_timeout` covers the 5 s; the 60 s idle budget is enforced by [`IdleTimeout`], an IO
//! wrapper that fails a connection which has gone that long without delivering a byte.

use std::io;
use std::pin::Pin;
use std::task::{Context, Poll};
use std::time::Duration;

use axum::Router;
use hyper_util::rt::{TokioExecutor, TokioIo, TokioTimer};
use hyper_util::server::conn::auto;
use hyper_util::server::graceful::GracefulShutdown;
use hyper_util::service::TowerToHyperService;
use tokio::io::{AsyncRead, AsyncWrite, ReadBuf};
use tokio::net::TcpListener;
use tokio::time::{Instant, Sleep, sleep_until};
use tokio_util::sync::CancellationToken;

use crate::config::{DRAIN_TIMEOUT, IDLE_TIMEOUT, READ_HEADER_TIMEOUT};

/// Serves until `shutdown` is cancelled, then drains for up to [`DRAIN_TIMEOUT`].
pub async fn serve(listener: TcpListener, app: Router, shutdown: CancellationToken) {
    let mut builder = auto::Builder::new(TokioExecutor::new());
    builder
        .http1()
        // hyper panics on a timeout with no timer, so this pairing is not optional.
        .timer(TokioTimer::new())
        .header_read_timeout(Some(READ_HEADER_TIMEOUT))
        .keep_alive(true);
    builder.http2().timer(TokioTimer::new());
    let graceful = GracefulShutdown::new();
    let service = TowerToHyperService::new(app);

    loop {
        let accepted = tokio::select! {
            biased;
            () = shutdown.cancelled() => break,
            accepted = listener.accept() => accepted,
        };

        let (stream, peer) = match accepted {
            Ok(accepted) => accepted,
            Err(err) => {
                // Per-connection accept failures (a client that vanished between the SYN and the
                // accept, or a transient fd shortage) must not take the listener down.
                tracing::warn!(error = %err, "accept failed");
                tokio::time::sleep(Duration::from_millis(10)).await;
                continue;
            }
        };
        let _ = stream.set_nodelay(true);

        let io = TokioIo::new(IdleTimeout::new(stream, IDLE_TIMEOUT));
        let connection = builder.serve_connection_with_upgrades(io, service.clone());
        let watched = graceful.watch(connection.into_owned());
        tokio::spawn(async move {
            if let Err(err) = watched.await {
                tracing::debug!(error = %err, %peer, "connection closed with an error");
            }
        });
    }

    // Stop accepting, then drain.
    drop(listener);
    tracing::info!(
        drain_seconds = DRAIN_TIMEOUT.as_secs(),
        "draining in-flight requests"
    );
    tokio::select! {
        () = graceful.shutdown() => tracing::info!("all connections drained"),
        () = tokio::time::sleep(DRAIN_TIMEOUT) => {
            tracing::warn!("drain timeout elapsed; closing remaining connections");
        }
    }
}

/// Wraps a stream so that a connection which delivers no bytes for `idle` is failed.
///
/// hyper's `header_read_timeout` restarts for every request on a keep-alive connection, so relying
/// on it alone would mean either a 5 s keep-alive window or no idle bound at all. This wrapper
/// separates the two: hyper still enforces 5 s once a request has started arriving, and a silent
/// connection is reaped after `idle`.
pub struct IdleTimeout<S> {
    inner: S,
    idle: Duration,
    deadline: Pin<Box<Sleep>>,
}

impl<S> IdleTimeout<S> {
    pub fn new(inner: S, idle: Duration) -> Self {
        Self {
            inner,
            idle,
            deadline: Box::pin(sleep_until(Instant::now() + idle)),
        }
    }

    fn reset(&mut self) {
        let next = Instant::now() + self.idle;
        self.deadline.as_mut().reset(next);
    }
}

impl<S: AsyncRead + Unpin> AsyncRead for IdleTimeout<S> {
    fn poll_read(
        self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        let this = self.get_mut();
        let before = buf.filled().len();
        match Pin::new(&mut this.inner).poll_read(cx, buf) {
            Poll::Ready(Ok(())) => {
                if buf.filled().len() > before {
                    this.reset();
                }
                Poll::Ready(Ok(()))
            }
            Poll::Ready(Err(err)) => Poll::Ready(Err(err)),
            Poll::Pending => match this.deadline.as_mut().as_mut().poll(cx) {
                Poll::Ready(()) => Poll::Ready(Err(io::Error::new(
                    io::ErrorKind::TimedOut,
                    "connection idle for too long",
                ))),
                Poll::Pending => Poll::Pending,
            },
        }
    }
}

impl<S: AsyncWrite + Unpin> AsyncWrite for IdleTimeout<S> {
    fn poll_write(
        self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &[u8],
    ) -> Poll<io::Result<usize>> {
        Pin::new(&mut self.get_mut().inner).poll_write(cx, buf)
    }

    fn poll_flush(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        Pin::new(&mut self.get_mut().inner).poll_flush(cx)
    }

    fn poll_shutdown(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        Pin::new(&mut self.get_mut().inner).poll_shutdown(cx)
    }

    fn poll_write_vectored(
        self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        bufs: &[io::IoSlice<'_>],
    ) -> Poll<io::Result<usize>> {
        Pin::new(&mut self.get_mut().inner).poll_write_vectored(cx, bufs)
    }

    fn is_write_vectored(&self) -> bool {
        self.inner.is_write_vectored()
    }
}

/// Resolves when SIGTERM or SIGINT arrives. On non-Unix platforms only Ctrl-C is available; the
/// service ships on Linux, where both are wired up.
pub async fn shutdown_signal() -> &'static str {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{SignalKind, signal};
        let mut terminate = match signal(SignalKind::terminate()) {
            Ok(signal) => signal,
            Err(err) => {
                tracing::error!(error = %err, "could not install a SIGTERM handler");
                let _ = tokio::signal::ctrl_c().await;
                return "SIGINT";
            }
        };
        tokio::select! {
            _ = terminate.recv() => "SIGTERM",
            _ = tokio::signal::ctrl_c() => "SIGINT",
        }
    }
    #[cfg(not(unix))]
    {
        let _ = tokio::signal::ctrl_c().await;
        "SIGINT"
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    #[tokio::test]
    async fn an_idle_connection_is_reaped() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();

        let server = tokio::spawn(async move {
            let (stream, _) = listener.accept().await.unwrap();
            let mut wrapped = IdleTimeout::new(stream, Duration::from_millis(50));
            let mut buf = [0u8; 16];
            wrapped.read(&mut buf).await
        });

        // Connect and then say nothing at all.
        let _client = tokio::net::TcpStream::connect(addr).await.unwrap();
        let result = server.await.unwrap();
        let err = result.expect_err("idle read should have timed out");
        assert_eq!(err.kind(), io::ErrorKind::TimedOut);
    }

    #[tokio::test]
    async fn activity_pushes_the_idle_deadline_out() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();

        let server = tokio::spawn(async move {
            let (stream, _) = listener.accept().await.unwrap();
            let mut wrapped = IdleTimeout::new(stream, Duration::from_millis(120));
            let mut total = 0usize;
            let mut buf = [0u8; 64];
            loop {
                match wrapped.read(&mut buf).await {
                    Ok(0) => return Ok(total),
                    Ok(n) => total += n,
                    Err(err) => return Err(err),
                }
            }
        });

        let mut client = tokio::net::TcpStream::connect(addr).await.unwrap();
        // Five writes 60 ms apart span 300 ms, well past a 120 ms idle budget, yet none of the
        // gaps reaches it.
        for _ in 0..5 {
            client.write_all(b"spurving").await.unwrap();
            tokio::time::sleep(Duration::from_millis(60)).await;
        }
        drop(client);

        let total = server.await.unwrap().expect("no idle timeout");
        assert_eq!(total, 40);
    }

    #[tokio::test]
    async fn serve_stops_accepting_when_cancelled() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let shutdown = CancellationToken::new();
        let app = crate::http::router(crate::app::AppState::for_tests());
        let handle = tokio::spawn(serve(listener, app, shutdown.clone()));

        // The listener is live.
        tokio::net::TcpStream::connect(addr)
            .await
            .expect("accepted");

        shutdown.cancel();
        tokio::time::timeout(Duration::from_secs(5), handle)
            .await
            .expect("server exited")
            .expect("server task did not panic");
    }

    /// Issues real HTTP over a real socket. Exercising the whole `auto::Builder` configuration is
    /// the only way to catch mistakes like a timeout set without a timer, which hyper answers with
    /// a panic on the connection task rather than an error.
    #[tokio::test]
    async fn serve_answers_a_real_request_over_a_real_socket() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let shutdown = CancellationToken::new();
        let app = crate::http::router(crate::app::AppState::for_tests());
        let handle = tokio::spawn(serve(listener, app, shutdown.clone()));

        let mut stream = tokio::net::TcpStream::connect(addr).await.unwrap();
        stream
            .write_all(
                format!("GET /healthz HTTP/1.1\r\nHost: {addr}\r\nConnection: close\r\n\r\n")
                    .as_bytes(),
            )
            .await
            .unwrap();
        let mut response = String::new();
        tokio::io::AsyncReadExt::read_to_string(&mut stream, &mut response)
            .await
            .unwrap();

        assert!(response.starts_with("HTTP/1.1 200 OK"), "{response}");
        assert!(response.contains("x-request-id:"), "{response}");
        assert!(response.contains(r#"{"status":"ok"}"#), "{response}");

        shutdown.cancel();
        tokio::time::timeout(Duration::from_secs(5), handle)
            .await
            .expect("server exited")
            .expect("server task did not panic");
    }

    /// A request whose headers never arrive must be dropped after the read-header timeout rather
    /// than held open. The wire behaviour differs by hyper version (a 408 or a bare close), so the
    /// assertion is only that the connection does not stay open indefinitely.
    #[tokio::test]
    async fn a_request_with_no_headers_is_dropped() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let shutdown = CancellationToken::new();
        let app = crate::http::router(crate::app::AppState::for_tests());
        let handle = tokio::spawn(serve(listener, app, shutdown.clone()));

        let mut stream = tokio::net::TcpStream::connect(addr).await.unwrap();
        // A request line and then silence: hyper is waiting for the rest of the headers.
        stream
            .write_all(b"GET /healthz HTTP/1.1\r\n")
            .await
            .unwrap();
        let mut response = String::new();
        let read = tokio::time::timeout(
            READ_HEADER_TIMEOUT + Duration::from_secs(3),
            tokio::io::AsyncReadExt::read_to_string(&mut stream, &mut response),
        )
        .await;
        assert!(read.is_ok(), "the connection was never closed");

        shutdown.cancel();
        tokio::time::timeout(Duration::from_secs(5), handle)
            .await
            .expect("server exited")
            .expect("server task did not panic");
    }
}
