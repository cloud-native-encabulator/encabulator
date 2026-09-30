"""Password hashing (Argon2id) and RS256 token issuance, per design section 8.9.

Argon2id parameters follow the OWASP recommendation for a memory-constrained pod: 19 MiB,
2 iterations, 1 lane. The signing key comes from ``JWT_PRIVATE_KEY`` (PEM). No password, key, or
hash is ever logged.
"""

from __future__ import annotations

import base64
import hashlib
import logging
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from typing import Any

import jwt
from argon2 import PasswordHasher, Type
from argon2.exceptions import InvalidHashError, VerificationError, VerifyMismatchError
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.hazmat.primitives.asymmetric.rsa import RSAPrivateKey, RSAPublicKey

from .config import AUDIENCE, ISSUER

log = logging.getLogger(__name__)

ALGORITHM = "RS256"

_hasher = PasswordHasher(
    time_cost=2,
    memory_cost=19_456,  # KiB
    parallelism=1,
    hash_len=32,
    salt_len=16,
    type=Type.ID,  # Argon2id
)


def hash_password(password: str) -> str:
    return _hasher.hash(password)


def verify_password(password_hash: str, password: str) -> bool:
    try:
        return _hasher.verify(password_hash, password)
    except VerifyMismatchError, VerificationError, InvalidHashError:
        return False


def needs_rehash(password_hash: str) -> bool:
    try:
        return _hasher.check_needs_rehash(password_hash)
    except InvalidHashError:  # pragma: no cover - defensive
        return True


class TokenError(Exception):
    """A presented token is missing, malformed, expired, or not ours."""


@dataclass(frozen=True, slots=True)
class TokenClaims:
    subject: str
    roles: tuple[str, ...]
    expires_at: datetime


class SigningKey:
    """The RSA key pair the detractor signs with, plus its JWKS representation."""

    def __init__(self, private_key: RSAPrivateKey, ttl_seconds: int = 3600) -> None:
        self._private = private_key
        self._public: RSAPublicKey = private_key.public_key()
        self.ttl_seconds = ttl_seconds
        self.kid = _thumbprint(self._public)
        self._private_pem = private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        ).decode()

    @classmethod
    def from_pem(cls, pem: str, ttl_seconds: int = 3600) -> SigningKey:
        key = serialization.load_pem_private_key(pem.encode(), password=None)
        if not isinstance(key, rsa.RSAPrivateKey):
            raise ValueError("JWT_PRIVATE_KEY must be an RSA private key (RS256)")
        if key.key_size < 2048:
            raise ValueError("JWT_PRIVATE_KEY must be at least 2048 bits")
        return cls(key, ttl_seconds)

    @classmethod
    def generate(cls, ttl_seconds: int = 3600) -> SigningKey:
        """Mint an ephemeral key. Only for local runs and tests: it dies with the process."""
        return cls(rsa.generate_private_key(public_exponent=65537, key_size=2048), ttl_seconds)

    def issue(self, subject: str, roles: list[str], *, now: datetime | None = None) -> str:
        issued = now or datetime.now(UTC)
        claims = {
            "sub": subject,
            "roles": sorted(roles),
            "iss": ISSUER,
            "aud": AUDIENCE,
            "iat": int(issued.timestamp()),
            "exp": int((issued + timedelta(seconds=self.ttl_seconds)).timestamp()),
        }
        return jwt.encode(claims, self._private_pem, algorithm=ALGORITHM, headers={"kid": self.kid})

    def verify(self, token: str) -> TokenClaims:
        try:
            claims = jwt.decode(
                token,
                self._public,
                algorithms=[ALGORITHM],
                audience=AUDIENCE,
                issuer=ISSUER,
                options={"require": ["exp", "sub", "iss", "aud"]},
            )
        except jwt.PyJWTError as exc:
            raise TokenError(str(exc)) from exc
        roles = claims.get("roles") or []
        if not isinstance(roles, list):
            raise TokenError("roles claim must be an array")
        return TokenClaims(
            subject=str(claims["sub"]),
            roles=tuple(str(role) for role in roles),
            expires_at=datetime.fromtimestamp(int(claims["exp"]), UTC),
        )

    def jwks(self) -> dict[str, Any]:
        numbers = self._public.public_numbers()
        return {
            "keys": [
                {
                    "kty": "RSA",
                    "use": "sig",
                    "alg": ALGORITHM,
                    "kid": self.kid,
                    "n": _b64u(numbers.n),
                    "e": _b64u(numbers.e),
                }
            ]
        }


def _b64u(value: int) -> str:
    raw = value.to_bytes((value.bit_length() + 7) // 8 or 1, "big")
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def _thumbprint(public_key: RSAPublicKey) -> str:
    """A stable kid: the first 16 bytes of SHA-256 over the DER SubjectPublicKeyInfo."""
    der = public_key.public_bytes(
        encoding=serialization.Encoding.DER,
        format=serialization.PublicFormat.SubjectPublicKeyInfo,
    )
    digest = hashlib.sha256(der).digest()[:16]
    return base64.urlsafe_b64encode(digest).rstrip(b"=").decode()


def load_signing_key(pem: str | None, ttl_seconds: int) -> SigningKey:
    if pem:
        return SigningKey.from_pem(pem, ttl_seconds)
    log.warning(
        "JWT_PRIVATE_KEY is unset: generating an ephemeral signing key. "
        "Tokens will not survive a restart and replicas will disagree. Set the secret."
    )
    return SigningKey.generate(ttl_seconds)
