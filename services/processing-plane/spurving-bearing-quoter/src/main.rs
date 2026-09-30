//! Entry point. All of the work is in the library so the integration tests can drive the same
//! router the binary serves.

#[tokio::main]
async fn main() {
    if let Err(err) = spurving_bearing_quoter::run().await {
        // The subscriber may not be installed yet when startup fails, so say it on stderr too.
        tracing::error!(error = %err, "the spurving bearing quoter failed to start");
        eprintln!("spurving-bearing-quoter: {err}");
        std::process::exit(1);
    }
}
