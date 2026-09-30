//! The wire contract of `POST /v1/quotes` and `GET /v1/quotes/{id}` (design section 8.3), plus
//! the encabulator model catalog from design section 3.

use std::fmt;

use chrono::{DateTime, SecondsFormat, Utc};
use serde::{Deserialize, Serialize};

use crate::money::Money;

/// The three encabulator models in the catalog (design section 3, "Models").
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Model {
    Turbo,
    Retro,
    Hyper,
}

impl Model {
    pub const ALL: [Self; 3] = [Self::Turbo, Self::Retro, Self::Hyper];

    /// Parses a model id. Returns `None` for anything not in the catalog, which the handler turns
    /// into `400 unknown_model`.
    #[must_use]
    pub fn from_id(id: &str) -> Option<Self> {
        match id {
            "turbo" => Some(Self::Turbo),
            "retro" => Some(Self::Retro),
            "hyper" => Some(Self::Hyper),
            _ => None,
        }
    }

    #[must_use]
    pub const fn id(self) -> &'static str {
        match self {
            Self::Turbo => "turbo",
            Self::Retro => "retro",
            Self::Hyper => "hyper",
        }
    }

    #[must_use]
    pub const fn display_name(self) -> &'static str {
        match self {
            Self::Turbo => "Turbo Encabulator (1944)",
            Self::Retro => "Retro Encabulator (Rockwell)",
            Self::Hyper => "HyperEncabulator (SANS ICS)",
        }
    }

    /// Base price per unit, before options, discounts, and surcharges.
    #[must_use]
    pub const fn base_price(self) -> Money {
        match self {
            Self::Turbo => Money::from_dollars_cents(1_944, 0),
            Self::Retro => Money::from_dollars_cents(2_749, 0),
            Self::Hyper => Money::from_dollars_cents(4_096, 0),
        }
    }

    /// How much pentametric harmonic work this model demands, relative to a turbo. The design
    /// says the CPU cost varies "depending on model and quantity"; this is the model half.
    #[must_use]
    pub const fn harmonic_weight(self) -> f64 {
        match self {
            Self::Turbo => 1.0,
            Self::Retro => 1.2,
            Self::Hyper => 1.5,
        }
    }
}

impl fmt::Display for Model {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.id())
    }
}

/// Order options. Only the drawn reciprocation dingle arm exists in v1 (design section 3).
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Options {
    #[serde(default)]
    pub dingle_arm: bool,
}

/// `POST /v1/quotes` request body.
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct QuoteRequest {
    /// Left as a raw string so an unrecognised model produces `unknown_model` rather than a
    /// generic deserialization failure.
    pub model: String,
    pub quantity: u32,
    #[serde(default)]
    pub options: Options,
}

/// The closed set of adjustment codes a quote can carry.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum SurchargeCode {
    /// 5% off for orders of five units or more (design section 8.3).
    VolumeDiscount,
    /// 15% on top when the `turbo-encabulation` flag is on (design section 6).
    TurboEncabulation,
}

/// A single line adjusting the subtotal. Discounts are negative amounts.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct Surcharge {
    pub code: SurchargeCode,
    pub amount: Money,
}

/// The currency every quote is denominated in. v1 has exactly one.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum Currency {
    #[default]
    #[serde(rename = "USD")]
    Usd,
}

/// `POST /v1/quotes` and `GET /v1/quotes/{id}` response body.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Quote {
    pub id: String,
    pub model: Model,
    pub quantity: u32,
    pub options: Options,
    pub unit_price: Money,
    pub subtotal: Money,
    pub surcharges: Vec<Surcharge>,
    pub total: Money,
    pub currency: Currency,
    pub computed_at: String,
    /// True when this body was served out of Valkey rather than computed for this request.
    pub cached: bool,
}

/// Formats a timestamp the way every Encabulator service does: RFC 3339, UTC, milliseconds.
#[must_use]
pub fn rfc3339_millis(at: DateTime<Utc>) -> String {
    at.to_rfc3339_opts(SecondsFormat::Millis, true)
}

/// Mints an id with the `qte_` prefix mandated by design section 8.
#[must_use]
pub fn new_quote_id() -> String {
    format!("qte_{}", uuid::Uuid::now_v7())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn model_ids_round_trip() {
        for model in Model::ALL {
            assert_eq!(Model::from_id(model.id()), Some(model));
        }
        assert_eq!(Model::from_id("Turbo"), None);
        assert_eq!(Model::from_id("pentametric"), None);
        assert_eq!(Model::from_id(""), None);
    }

    #[test]
    fn base_prices_match_the_catalog() {
        assert_eq!(Model::Turbo.base_price().to_string(), "1944.00");
        assert_eq!(Model::Retro.base_price().to_string(), "2749.00");
        assert_eq!(Model::Hyper.base_price().to_string(), "4096.00");
    }

    #[test]
    fn model_serializes_as_its_lowercase_id() {
        assert_eq!(serde_json::to_string(&Model::Hyper).unwrap(), "\"hyper\"");
    }

    #[test]
    fn options_default_to_no_dingle_arm() {
        let req: QuoteRequest = serde_json::from_str(r#"{"model":"turbo","quantity":1}"#).unwrap();
        assert!(!req.options.dingle_arm);
    }

    #[test]
    fn quote_ids_are_prefixed() {
        assert!(new_quote_id().starts_with("qte_"));
    }

    #[test]
    fn timestamps_carry_milliseconds_and_a_z() {
        let at = DateTime::parse_from_rfc3339("2026-09-30T12:00:00.123456Z")
            .unwrap()
            .with_timezone(&Utc);
        assert_eq!(rfc3339_millis(at), "2026-09-30T12:00:00.123Z");
    }
}

#[cfg(test)]
mod wire_tests {
    use super::*;

    #[test]
    fn currency_is_usd_on_the_wire() {
        assert_eq!(serde_json::to_string(&Currency::Usd).unwrap(), "\"USD\"");
    }

    #[test]
    fn surcharge_codes_are_snake_case_on_the_wire() {
        assert_eq!(
            serde_json::to_string(&SurchargeCode::VolumeDiscount).unwrap(),
            "\"volume_discount\""
        );
        assert_eq!(
            serde_json::to_string(&SurchargeCode::TurboEncabulation).unwrap(),
            "\"turbo_encabulation\""
        );
    }
}
