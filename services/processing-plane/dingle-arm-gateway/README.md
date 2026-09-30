# dingle-arm-gateway

> The drawn reciprocation dingle arm, employed whenever a barescent skor motion is required.

The only public entry point for customer traffic. It verifies tokens, applies a per-subject rate
limit, and proxies to exactly nine upstream operations; everything else in the Works is internal.
Plane: processing. Language: Go. Design reference: [section 8.1](../../../docs/design.md).

Routing uses Go 1.22 `net/http` patterns and no third-party router, and proxying uses
`net/http/httputil` so responses stream rather than buffer.

## Routes

Prefix `/api`. A path that is not in this table does not exist: the gateway has no catch-all.

| Method | Path | Auth | Upstream |
|---|---|---|---|
| POST | `/api/v1/auth/login` | public | detractor `POST /v1/auth/login` |
| POST | `/api/v1/auth/register` | public | detractor `POST /v1/auth/register` |
| GET | `/api/v1/users/me` | authenticated | detractor `GET /v1/users/me` |
| GET | `/api/v1/parts` | public | depot `GET /v1/parts` |
| POST | `/api/v1/quotes` | authenticated | quoter `POST /v1/quotes` |
| POST | `/api/v1/orders` | authenticated | processor `POST /v1/orders` |
| GET | `/api/v1/orders` | authenticated | datastore `GET /v1/orders` |
| GET | `/api/v1/orders/{id}` | authenticated | datastore `GET /v1/orders/{id}` |
| GET | `/api/v1/shipments/{orderId}` | authenticated | dispatch `GET /v1/shipments/{orderId}` |

Plus `GET /healthz`, `GET /readyz` and `GET /metrics`, which are not proxied.

**"Authenticated"** means a valid token carrying at least one role. The design writes this as
`customer+` on the customer routes and "any role" on `/users/me`; the gateway treats both the same
way, because the finer rules belong to the upstreams that own the data. The ledger, for instance,
narrows a customer's order list to its own orders whatever the query says.

## Authentication

- RS256 only, verified against `${UNILATERAL_PHASE_DETRACTOR_URL}/.well-known/jwks.json`.
- Keys are cached, refreshed every five minutes, and refreshed on demand when a token arrives with a
  `kid` the cache has not seen, so a detractor key rotation does not need a gateway restart. The
  on-demand refresh is throttled to once every ten seconds, so a flood of bogus tokens cannot turn
  into a flood of JWKS fetches.
- `exp` is required. `iss` must be `unilateral-phase-detractor` and `aud` must be `encabulator`.
- `alg: none` and HS256-signed-with-the-RSA-modulus are rejected, with tests to prove it.
- A failed JWKS fetch leaves the cached keys in place. If the JWKS has *never* been fetched,
  authenticated routes answer `503 service_unavailable` rather than pretending a token is bad.

On success the gateway **strips every inbound `X-Encabulator-*` header** and sets:

- `X-Encabulator-Subject`: the token's `sub`
- `X-Encabulator-Roles`: the token's `roles`, comma-separated

The bearer token itself is not forwarded: internal services trust the identity headers, so the token
never leaves the edge. Public routes forward no identity headers at all.

## Rate limiting

- Token bucket, rate from the `gateway-rate-limit-rps` flag (default 50), burst 2x the rate.
- Keyed on the JWT subject for authenticated routes, and on the first `X-Forwarded-For` hop (or the
  peer address) for public routes.
- Buckets live in a bounded LRU of 10,000 entries, so a flood of distinct keys evicts rather than
  grows.
- Over the limit: `429 rate_limited` with `Retry-After` in whole seconds, never less than one.
- The rate is read from the flag on **every** request, so moving it mid-demo takes effect at once;
  changing the rate resets each bucket to the new burst rather than letting a stale fill rate leak
  through. A rate of `0` closes the gate completely, which is the point: it is the demo's congestion
  lever.

## Errors

`{"error": {"code": "...", "message": "..."}}`:

| Status | Code | When |
|---|---|---|
| 401 | `unauthorized` | Missing or unacceptable token (with `WWW-Authenticate: Bearer`) |
| 403 | `forbidden` | Valid token carrying no roles |
| 429 | `rate_limited` | Over the token bucket (with `Retry-After`) |
| 502 | `upstream_unavailable` | The upstream refused or dropped the connection |
| 504 | `upstream_timeout` | The upstream did not send response headers in time |
| 503 | `service_unavailable` | Token verification is impossible: no signing keys cached |

Any other status, including 4xx and 5xx from an upstream, is passed through unchanged.

## Environment

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | API, health and metrics on one port |
| `LOG_LEVEL` | `info` | |
| `OTEL_SERVICE_NAME` | `dingle-arm-gateway` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing |
| `MALAISE_ENGINE_URL` | unset | Unset disables the malaise middleware |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | Unset means the built-in 50 rps default |
| `UNILATERAL_PHASE_DETRACTOR_URL` | **required** | Also the JWKS source |
| `PREFABULATED_AMULITE_DEPOT_URL` | **required** | |
| `SPURVING_BEARING_QUOTER_URL` | **required** | |
| `PANAMETRIC_FAM_PROCESSOR_URL` | **required** | |
| `HYDROCOPTIC_DATASTORE_URL` | **required** | |
| `NOFER_TRUNNION_DISPATCH_URL` | **required** | |
| `UPSTREAM_TIMEOUT` | `10s` | Wait for upstream response headers |
| `RATE_LIMIT_BUCKETS` | `10000` | LRU capacity |
| `SERVICE_VERSION` | `dev` | Reported as `service.version` |

All six upstream URLs are required at startup: a missing one is a startup failure, not a surprise at
the first request.

## Metrics

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `encabulator_gateway_requests_total` | counter | `route`, `status` | Every request, by route template |
| `encabulator_gateway_rate_limited_total` | counter | · | Requests the bucket rejected |
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` | Inbound, route template only |
| `http_client_request_duration_seconds` | histogram | `http_request_method`, `server_address`, `http_response_status_code` | Outbound; `server_address` is the upstream **service name**, not `host:port` |
| `encabulator_malaise_applied_total` | counter | `kind` | From the kit's malaise middleware |

Route labels are always templates: `/api/v1/orders/{id}`, never `/api/v1/orders/ord_01J...`. A test
asserts no raw path or upstream `host:port` reaches a label.

## Running locally

The gateway is a proxy, so it needs its upstreams. The quickest way to see it work is to point every
upstream at one stub:

```bash
export UNILATERAL_PHASE_DETRACTOR_URL=http://127.0.0.1:9001
export PREFABULATED_AMULITE_DEPOT_URL=http://127.0.0.1:9001
export SPURVING_BEARING_QUOTER_URL=http://127.0.0.1:9001
export PANAMETRIC_FAM_PROCESSOR_URL=http://127.0.0.1:9001
export HYDROCOPTIC_DATASTORE_URL=http://127.0.0.1:9001
export NOFER_TRUNNION_DISPATCH_URL=http://127.0.0.1:9001
make run

curl -s localhost:8080/api/v1/parts            # public: proxied
curl -s localhost:8080/api/v1/orders            # 401: needs a token
curl -s localhost:8080/api/v1/nonsense          # 404: not in the routing table
```

In the cluster, point each variable at its service and the detractor's JWKS becomes real.

```bash
make build             # bin/dingle-arm-gateway
make test              # unit tests: JWT, JWKS rotation, token bucket, proxying
make lint              # gofmt -l plus go vet
make image             # builds from the repository root
```

The gateway owns no backing store, so it has no Testcontainers tests; `make test-integration` is the
same as `make test` here. The test suite still exercises the real verifier and the real limiter
together against a detractor that genuinely signs tokens.

## Deployment notes

- Probes: `GET /healthz` for liveness, `GET /readyz` for readiness. **Readiness has no checks by
  design**: every dependency the gateway has is a downstream service, and design section 7 forbids
  probing those. A gateway that cannot reach the detractor still reports ready and answers `503` on
  authenticated routes, which is the honest signal.
- Stateless. Scale horizontally; rate limit buckets are per pod, so the effective cluster-wide limit
  is roughly `replicas x gateway-rate-limit-rps`. Worth remembering when tuning the flag for a demo.
- Needs no Kubernetes API access and no service account token.
- This is the pod the console's *Malaise* tab is most likely to congest, so give it headroom:
  100m CPU / 64Mi memory requests, 1000m / 256Mi limits.
- The only service that ever needs an Ingress alongside the console, if customer traffic should come
  from outside the cluster. In v1 the cardinal grammeter drives it from inside.
