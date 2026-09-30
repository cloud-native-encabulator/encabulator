from __future__ import annotations

from datetime import UTC, datetime, timedelta

import jwt
import pytest

from detractor.config import AUDIENCE, ISSUER
from detractor.security import (
    SigningKey,
    TokenError,
    hash_password,
    load_signing_key,
    verify_password,
)


def test_argon2id_hashing_round_trip() -> None:
    digest = hash_password("inverse reactive current")
    assert digest.startswith("$argon2id$")
    assert verify_password(digest, "inverse reactive current") is True
    assert verify_password(digest, "inverse reactive currant") is False


def test_hashes_are_salted() -> None:
    assert hash_password("same") != hash_password("same")


def test_verify_tolerates_a_corrupt_hash() -> None:
    assert verify_password("not-a-hash", "anything") is False


@pytest.fixture(scope="module")
def key() -> SigningKey:
    return SigningKey.generate(ttl_seconds=3600)


def test_issue_and_verify(key: SigningKey) -> None:
    token = key.issue("ada", ["customer", "viewer"])
    claims = key.verify(token)
    assert claims.subject == "ada"
    assert set(claims.roles) == {"customer", "viewer"}
    assert claims.expires_at > datetime.now(UTC)


def test_token_header_carries_the_kid(key: SigningKey) -> None:
    header = jwt.get_unverified_header(key.issue("ada", ["customer"]))
    assert header["alg"] == "RS256"
    assert header["kid"] == key.kid


def test_claims_match_the_design(key: SigningKey) -> None:
    claims = jwt.decode(
        key.issue("ada", ["customer"]),
        options={"verify_signature": False},
        audience=AUDIENCE,
    )
    assert claims["iss"] == ISSUER == "unilateral-phase-detractor"
    assert claims["aud"] == AUDIENCE == "encabulator"
    assert claims["roles"] == ["customer"]
    assert claims["exp"] - claims["iat"] == 3600


def test_expired_token_is_rejected(key: SigningKey) -> None:
    stale = key.issue("ada", ["customer"], now=datetime.now(UTC) - timedelta(hours=2))
    with pytest.raises(TokenError):
        key.verify(stale)


def test_token_from_another_key_is_rejected(key: SigningKey) -> None:
    impostor = SigningKey.generate()
    with pytest.raises(TokenError):
        key.verify(impostor.issue("ada", ["admin"]))


def test_wrong_audience_is_rejected(key: SigningKey) -> None:
    forged = jwt.encode(
        {
            "sub": "ada",
            "roles": ["admin"],
            "iss": ISSUER,
            "aud": "some-other-system",
            "exp": int((datetime.now(UTC) + timedelta(hours=1)).timestamp()),
        },
        key._private_pem,
        algorithm="RS256",
    )
    with pytest.raises(TokenError):
        key.verify(forged)


def test_unsigned_token_is_rejected(key: SigningKey) -> None:
    none_token = jwt.encode({"sub": "ada", "roles": ["admin"]}, key=None, algorithm="none")
    with pytest.raises(TokenError):
        key.verify(none_token)


def test_jwks_shape(key: SigningKey) -> None:
    jwks = key.jwks()
    assert len(jwks["keys"]) == 1
    entry = jwks["keys"][0]
    assert entry["kty"] == "RSA"
    assert entry["use"] == "sig"
    assert entry["alg"] == "RS256"
    assert entry["kid"] == key.kid
    assert "=" not in entry["n"]  # base64url, unpadded
    # The published key really verifies our tokens.
    signing_key = jwt.PyJWKSet.from_dict(jwks).keys[0].key
    claims = jwt.decode(
        key.issue("ada", ["customer"]), signing_key, algorithms=["RS256"], audience=AUDIENCE
    )
    assert claims["sub"] == "ada"


def test_kid_is_stable_for_the_same_key(key: SigningKey) -> None:
    reloaded = SigningKey.from_pem(key._private_pem)
    assert reloaded.kid == key.kid


def test_non_rsa_key_is_rejected() -> None:
    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric import ed25519

    pem = (
        ed25519.Ed25519PrivateKey.generate()
        .private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        )
        .decode()
    )
    with pytest.raises(ValueError, match="RSA private key"):
        SigningKey.from_pem(pem)


def test_missing_key_falls_back_to_an_ephemeral_one(caplog) -> None:
    generated = load_signing_key(None, 3600)
    assert generated.kid
    assert "ephemeral" in caplog.text
