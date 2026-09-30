//! Shared application state, assembled once at startup and cloned into every handler.

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};

use crate::cache::QuoteStore;
use crate::config::Config;
use crate::flags::Flags;
use crate::malaise::Malaise;
use crate::metrics::Metrics;
use crate::outbound::Outbound;
use crate::pricing::Pentametric;

#[derive(Clone)]
pub struct AppState {
    pub config: Arc<Config>,
    pub metrics: Arc<Metrics>,
    pub flags: Arc<Flags>,
    pub malaise: Arc<Malaise>,
    pub store: Arc<QuoteStore>,
    pub pentametric: Pentametric,
    pub outbound: Outbound,
    draining: Arc<AtomicBool>,
}

impl AppState {
    /// Builds every collaborator from config. Performs no I/O: the flag poller, the malaise poller,
    /// and the Valkey connector are all started separately by [`crate::run`].
    pub fn new(config: Config) -> Result<Self, reqwest::Error> {
        let metrics = Metrics::new();
        let outbound = Outbound::new(metrics.clone())?;
        let flags = Flags::new(config.girdlespring_url.clone());
        let malaise = Malaise::new(
            config.malaise_engine_url.clone(),
            config.service_name.clone(),
            metrics.clone(),
        );
        let store = QuoteStore::new(config.valkey_url.as_deref(), metrics.clone());
        let pentametric =
            Pentametric::new(config.pentametric_workers, config.pentametric_burn_scale);

        Ok(Self {
            config: Arc::new(config),
            metrics,
            flags,
            malaise,
            store,
            pentametric,
            outbound,
            draining: Arc::new(AtomicBool::new(false)),
        })
    }

    /// State suitable for unit tests: no cache, no flags, no malaise, no CPU burn.
    pub fn for_tests() -> Self {
        Self::new(Config::for_tests()).expect("test state")
    }

    /// True once SIGTERM has been received. `/readyz` fails from this moment on, so Kubernetes
    /// stops sending new work while in-flight requests drain (design section 7, "Graceful
    /// shutdown").
    pub fn draining(&self) -> bool {
        self.draining.load(Ordering::Relaxed)
    }

    pub fn begin_draining(&self) {
        self.draining.store(true, Ordering::Relaxed);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_state_has_no_external_dependencies() {
        let state = AppState::for_tests();
        assert!(!state.store.enabled());
        assert!(!state.flags.enabled());
        assert!(!state.malaise.enabled());
        assert!(!state.draining());
    }

    #[test]
    fn draining_is_one_way() {
        let state = AppState::for_tests();
        state.begin_draining();
        assert!(state.draining());
        state.begin_draining();
        assert!(state.draining());
    }
}
