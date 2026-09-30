# unilateral-phase-detractor

> "It supplies inverse reactive current for use in unilateral phase detractors."
> (J. H. Quick, *The Turbo-Encabulator in Industry*, 1944)

Identity for the Encabulator Works. The detractor supplies **inverse reactive credentials**: it
owns the user store, checks passwords, issues RS256 access tokens, publishes the JWKS that
`dingle-arm-gateway` verifies against, and keeps an audit trail of every login and every user
change. Design section 8.9.

- Plane: support (`encabulator.io/plane=support`)
- Language: Python 3.14, FastAPI, SQLAlchemy 2 async + asyncpg, PyJWT + cryptography
- Owns: PostgreSQL database `detractor`
- Readiness depends on: Postgres only

## API

All bodies are JSON. Errors use the house shape `{"error": {"code", "message"}}`.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/v1/auth/login` | public | Exchange credentials for an access token |
| POST | `/v1/auth/register` | public, rate limited per IP | Self-service `customer` signup |
| GET | `/v1/users/me` | any authenticated caller | The calling user |
| GET | `/v1/users?limit=500` | **admin** | Every user |
| POST | `/v1/users` | **admin** | Create a user with explicit roles |
| PATCH | `/v1/users/{username}` | **admin** | Change password, display name, roles, or disabled |
| DELETE | `/v1/users/{username}` | **admin** | Remove a user |
| GET | `/v1/audit?limit=100` | **admin** | Login successes and failures, user changes, newest first |
| GET | `/.well-known/jwks.json` | public | The public signing key set |
| GET | `/healthz`, `/readyz`, `/metrics` | public | Platform endpoints (see the kit) |

### Tokens

```
POST /v1/auth/login   {"username": "ada", "password": "…"}
200  {"accessToken": "eyJ…", "tokenType": "Bearer", "expiresIn": 3600,
      "user": {"username": "ada", "displayName": "Ada", "roles": ["customer"],
               "disabled": false, "createdAt": "…", "updatedAt": "…"}}
```

RS256, with `sub`, `roles` (always an array), `iss=unilateral-phase-detractor`,
`aud=encabulator`, `iat`, `exp` (`JWT_TTL_SECONDS`, default 1 hour), and a `kid` header. The `kid`
is the first 16 bytes of SHA-256 over the DER SubjectPublicKeyInfo, so every replica holding the
same secret publishes the same `kid` and the gateway's JWKS cache stays warm across restarts.

### Users and roles

Usernames are 3 to 64 characters of `[a-z0-9._-]`, normalised to lower case (so `Ada` and `ada`
are one account). Passwords are 8 to 1024 characters. Roles are `customer`, `viewer`, `operator`,
`admin`; unknown roles are rejected with `invalid_roles`. Registration always produces exactly
`["customer"]` · the request model forbids extra fields, so there is no path to a self-granted role.

The works always keeps at least one enabled administrator: demoting, disabling, or deleting the
last admin returns `409 last_admin`, and an admin cannot delete themselves (`409 cannot_delete_self`).

### How a caller is identified

1. `X-Encabulator-Subject` · set by the gateway (from a verified JWT) or by the console BFF (from
   its session). Internal services trust this header, per design section 7.
2. Otherwise an `Authorization: Bearer …` token, verified locally against our own public key.

**Roles always come from the user row, never from the header or the token.** The detractor owns the
store, so it can be authoritative. Two consequences worth knowing:

- A caller that forges `X-Encabulator-Roles: admin` gets exactly the roles it really has.
- Role changes and disablement take effect immediately, without waiting for a token to expire.

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | API, health, and metrics on one port |
| `LOG_LEVEL` | `info` | |
| `OTEL_SERVICE_NAME` | `unilateral-phase-detractor` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | unset ⇒ tracing is a no-op |
| `MALAISE_ENGINE_URL` | unset | unset ⇒ no malaise middleware |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | the detractor reads no flags today; polled for consistency |
| `DATABASE_URL` | `postgresql://detractor@postgres:5432/detractor` | `postgres://` and `?sslmode=` are translated for asyncpg |
| `JWT_PRIVATE_KEY` | unset | **secret.** RSA private key, PEM, ≥ 2048 bits |
| `JWT_TTL_SECONDS` | `3600` | token lifetime, also reported as `expiresIn` |
| `BOOTSTRAP_ADMIN_PASSWORD` | unset | **secret.** Creates/re-keys `admin` |
| `BOOTSTRAP_OPERATOR_PASSWORD` | unset | **secret.** Creates/re-keys `operator` |
| `REGISTER_RATE_PER_MINUTE` | `10` | per client IP, per pod |
| `SHUTDOWN_READINESS_GRACE_SECONDS` | `3` | see the kit's graceful shutdown |

No default passwords exist in code. An unset `BOOTSTRAP_*_PASSWORD` means that user is simply not
created, and the service logs a warning. On every start, a set password either creates the user or
re-keys it if the secret has been rotated. An unset `JWT_PRIVATE_KEY` generates an **ephemeral**
key with a loud warning: fine for `make run`, useless in a Deployment with more than one replica.

## Schema and migrations

Two tables (`users`, `audit_log`) plus a `schema_migrations` ledger, created by embedded versioned
SQL applied at startup (`src/detractor/migrations.py`). The runner takes a Postgres advisory lock,
so several replicas starting at once cannot race. The SQL is deliberately portable · `TEXT` for
roles (a sorted, comma-separated list) and for audit detail (JSON) · which lets the whole
authorization matrix run on SQLite without a container while production runs Postgres 18.

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `encabulator_logins_total` | counter | `result` (`success`, `failure`) |
| `encabulator_registrations_total` | counter | `result` (`created`, `rejected`, `rate_limited`) |
| `encabulator_tokens_issued_total` | counter | |
| `encabulator_users` | gauge | |
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |
| `http_client_request_duration_seconds` | histogram | `http_request_method`, `server_address`, `http_response_status_code` |
| `encabulator_malaise_applied_total` | counter | `kind` |

Plus the Python process, platform, and GC collectors.

## Security notes

- Passwords are hashed with **Argon2id** (`argon2-cffi`): 19 MiB, 2 iterations, 1 lane · the OWASP
  recommendation for a memory-constrained pod. Hashes are re-derived transparently on login when
  the parameters change.
- An unknown username spends comparable time to a wrong password, and both return the identical
  `401 invalid_credentials` body, so the endpoint does not enumerate users.
- Passwords, hashes, and tokens are never logged and never appear in the audit detail.
- A disabled account cannot log in and its outstanding tokens stop working immediately (`403
  account_disabled`).

## Running locally

```bash
make build                  # uv sync
make test                   # unit tests; Postgres tests skip without a container runtime
source ../../../dev-env.sh && make test    # adds the Postgres 18 container tests
make lint
make run                    # needs a Postgres on localhost:5432

# A throwaway Postgres for `make run`:
finch run -d --name enc-pg -p 5432:5432 \
  -e POSTGRES_USER=detractor -e POSTGRES_PASSWORD=detractor -e POSTGRES_DB=detractor postgres:18
```

Build the image from the repository root (the Dockerfile copies the kit in):

```bash
make image                  # or: docker build -f Dockerfile -t …:1.0.0 ../../..
```

Multi-stage, `uv`-based, no dev dependencies and no pytest in the runtime layer, runs as UID/GID
65532, and writes nothing outside `/tmp` so a read-only root filesystem is fine.
