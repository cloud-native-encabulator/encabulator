//! Money as integer cents.
//!
//! The design requires prices "rounded to cents". Carrying cents in an `i64` and only widening
//! to `f64` at the JSON boundary means no accumulated binary-floating-point drift in the
//! arithmetic itself, which matters because the processor writes these totals to the ledger.

use std::fmt;
use std::ops::{Add, AddAssign, Neg, Sub};

use serde::de::{self, Deserializer};
use serde::{Deserialize, Serialize, Serializer};

/// An amount of money, in cents. Signed, because discounts are negative surcharges.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct Money(i64);

impl Money {
    pub const ZERO: Self = Self(0);

    #[must_use]
    pub const fn from_cents(cents: i64) -> Self {
        Self(cents)
    }

    /// Builds an amount from whole dollars. Used for the catalog's base prices, which the design
    /// states to the cent.
    #[must_use]
    pub const fn from_dollars_cents(dollars: i64, cents: i64) -> Self {
        Self(dollars * 100 + cents)
    }

    #[must_use]
    pub const fn cents(self) -> i64 {
        self.0
    }

    #[must_use]
    pub fn as_f64(self) -> f64 {
        self.0 as f64 / 100.0
    }

    /// Multiplies by a non-negative count. Saturates rather than panicking; an order is capped at
    /// ten units, so saturation is unreachable in practice but cheap insurance.
    #[must_use]
    pub fn times(self, count: u32) -> Self {
        Self(self.0.saturating_mul(i64::from(count)))
    }

    /// Applies a rate expressed in basis points, rounding half away from zero so that a 5%
    /// discount on $19.99 is $1.00 rather than $0.99.
    #[must_use]
    pub fn basis_points(self, bps: i64) -> Self {
        let numerator = self.0.saturating_mul(bps);
        let half = if numerator.is_negative() {
            -5_000
        } else {
            5_000
        };
        Self(numerator.saturating_add(half) / 10_000)
    }
}

impl Add for Money {
    type Output = Self;
    fn add(self, rhs: Self) -> Self {
        Self(self.0.saturating_add(rhs.0))
    }
}

impl AddAssign for Money {
    fn add_assign(&mut self, rhs: Self) {
        *self = *self + rhs;
    }
}

impl Sub for Money {
    type Output = Self;
    fn sub(self, rhs: Self) -> Self {
        Self(self.0.saturating_sub(rhs.0))
    }
}

impl Neg for Money {
    type Output = Self;
    fn neg(self) -> Self {
        Self(-self.0)
    }
}

impl fmt::Display for Money {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let sign = if self.0 < 0 { "-" } else { "" };
        let abs = self.0.unsigned_abs();
        write!(f, "{sign}{}.{:02}", abs / 100, abs % 100)
    }
}

impl Serialize for Money {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        serializer.serialize_f64(self.as_f64())
    }
}

impl<'de> Deserialize<'de> for Money {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        let dollars = f64::deserialize(deserializer)?;
        if !dollars.is_finite() {
            return Err(de::Error::custom("money must be finite"));
        }
        Ok(Self((dollars * 100.0).round() as i64))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn dollars_and_cents_round_trip() {
        let m = Money::from_dollars_cents(1944, 0);
        assert_eq!(m.cents(), 194_400);
        assert_eq!(m.to_string(), "1944.00");
        assert!((m.as_f64() - 1944.0).abs() < f64::EPSILON);
    }

    #[test]
    fn display_pads_cents_and_keeps_sign() {
        assert_eq!(Money::from_cents(5).to_string(), "0.05");
        assert_eq!(Money::from_cents(-1_234).to_string(), "-12.34");
    }

    #[test]
    fn basis_points_round_half_away_from_zero() {
        // 5% of 19.99 == 0.9995 -> 1.00
        assert_eq!(
            Money::from_cents(1_999).basis_points(500),
            Money::from_cents(100)
        );
        // 15% of 19.99 == 2.9985 -> 3.00
        assert_eq!(
            Money::from_cents(1_999).basis_points(1_500),
            Money::from_cents(300)
        );
        // negative amounts round away from zero too
        assert_eq!(
            Money::from_cents(-1_999).basis_points(500),
            Money::from_cents(-100)
        );
        // exact halves go up in magnitude: 5% of 0.10 == 0.005 -> 0.01
        assert_eq!(
            Money::from_cents(10).basis_points(500),
            Money::from_cents(1)
        );
    }

    #[test]
    fn serializes_as_a_json_number() {
        let json = serde_json::to_string(&Money::from_dollars_cents(2_749, 0)).unwrap();
        assert_eq!(json, "2749.0");
        let back: Money = serde_json::from_str("2749.00").unwrap();
        assert_eq!(back, Money::from_dollars_cents(2_749, 0));
    }

    #[test]
    fn arithmetic_is_exact_in_cents() {
        let mut total = Money::ZERO;
        for _ in 0..10 {
            total += Money::from_cents(1);
        }
        assert_eq!(total, Money::from_cents(10));
        assert_eq!(-Money::from_cents(7), Money::from_cents(-7));
        assert_eq!(
            Money::from_cents(10) - Money::from_cents(3),
            Money::from_cents(7)
        );
        assert_eq!(Money::from_cents(311).times(3), Money::from_cents(933));
    }
}
