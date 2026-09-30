# nofer-trunnion-dispatch

> "It is not possible to enumerate the multitudinous applications of this remarkable device.
> Suffice it to say that the operation of nofer trunnions is now a matter of routine."
> (*Time*, 1946)

The dispatch office. When the side-fumbling detector passes a batch of units, dispatch picks
a carrier, prints a tracking number, and puts the shipment on a nofer trunnion. The
simulated transit time lives in a Valkey sorted set rather than in a `setTimeout`, so a pod
restart in the middle of a demo loses nothing: whichever replica comes back claims the
delivery when it falls due.

- Plane: `processing` · Language: TypeScript (Node 24, Fastify 5) · Owns Valkey keys `shipment:*`
- Design: [section 8.5](../../../docs/design.md#85-nofer-trunnion-dispatch)

## What it does

**Consumes `encabulator.inspection.passed`** on the durable pull consumer
`nofer-trunnion-dispatch`, four messages at a time. Booking is idempotent per order, so a
JetStream redelivery does not produce a second tracking number.

**Publishes `encabulator.shipment.dispatched`** with `{orderId, carrier, trackingNumber,
etaSeconds}`, then **`encabulator.shipment.delivered`** with `{orderId, carrier,
trackingNumber}` once the transit time is up.

**Delivers exactly once.** A one-second loop reads the due shipments out of a sorted set and
claims each with an atomic `ZREM`. Only the replica whose `ZREM` removes the member owns the
delivery, so scaling dispatch to three pods does not triple the delivery events. If
publishing the event fails, the shipment is re-armed five seconds later rather than being
quietly marked delivered.

**Carriers.** `Trunnion Freight`, `Waneshaft Express`, `Grammeter Parcel`, and
`Tremie Pipe & Sons`. Tracking numbers are `NT` plus ten digits. Transit time is 10 to 60
seconds.

## API

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/shipments/{orderId}` | The shipment, or `404 shipment_not_found` |
| GET | `/v1/shipments?limit=50` | Recent shipments, newest first (max 500) |
| GET | `/healthz`, `/readyz`, `/metrics` | Liveness, readiness (Valkey + NATS), Prometheus |

```json
{
  "id": "shp_018f...",
  "orderId": "ord_018f...",
  "carrier": "Grammeter Parcel",
  "trackingNumber": "NT4815162342",
  "etaSeconds": 34,
  "attempt": 1,
  "serials": ["ENC-turbo-m4x9k2-a1b2"],
  "dispatchedAt": "2026-09-30T12:00:00.000Z",
  "dueAt": "2026-09-30T12:00:34.000Z",
  "status": "in_transit"
}
```

`status` is `in_transit` or `delivered`; a delivered shipment also carries `deliveredAt`.

## Valkey keyspace

| Key | Type | Purpose |
|---|---|---|
| `shipment:order:<orderId>` | string (JSON) | The shipment record, written once and never mutated |
| `shipment:delivered:<orderId>` | string | Delivery timestamp; the key's presence *is* the status |
| `shipment:due` | sorted set | Order ids scored by due time in epoch millis; the delivery queue |
| `shipment:recent` | list | Recently dispatched order ids, newest first, capped at 500 |

Keeping the record immutable and the delivery in a separate key means no Lua script ever
calls `cjson.encode`, so nothing can quietly reshape the stored JSON, and claiming a
delivery is a single atomic step.

## Events

| Direction | Subject | Data |
|---|---|---|
| in | `encabulator.inspection.passed` | `orderId, attempt, serials[]` |
| out | `encabulator.shipment.dispatched` | `orderId, carrier, trackingNumber, etaSeconds` |
| out | `encabulator.shipment.delivered` | `orderId, carrier, trackingNumber` |

Durable consumer `nofer-trunnion-dispatch`, explicit ack, `AckWait` 30 s, `MaxDeliver` 5.
An injected `error` malaise naks the message instead of responding.

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `LOG_LEVEL` | `info` | |
| `VALKEY_URL` | **required** | `redis://default:<pw>@valkey:6379/0` |
| `NATS_URL` | `nats://nats:4222` | |
| `OTEL_SERVICE_NAME` | `nofer-trunnion-dispatch` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing |
| `MALAISE_ENGINE_URL` | unset | Unset disables fault injection |
| `SERVICE_VERSION` | `1.0.0` | Trace resource attribute |

Dispatch reads no feature flags.

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `encabulator_shipments_total` | counter | `carrier`, `status` (`dispatched`, `delivered`) |
| `encabulator_shipments_in_transit` | gauge | |
| `encabulator_messages_consumed_total` | counter | `subject`, `result` |
| `encabulator_messages_published_total` | counter | `subject`, `result` |
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |
| `encabulator_malaise_applied_total` | counter | `kind` |

Plus the Node process and runtime defaults.

## Running locally

```bash
source ../../../dev-env.sh
docker run -d --name dispatch-valkey -p 6379:6379 valkey/valkey:9
docker run -d --name dispatch-nats   -p 4222:4222 nats:2.12 -js

make build
VALKEY_URL=redis://127.0.0.1:6379/0 NATS_URL=nats://127.0.0.1:4222 LOG_LEVEL=debug make run

curl -s 'localhost:8080/v1/shipments?limit=5' | jq
```

To watch a shipment go out, publish an `inspection.passed` event:

```bash
nats pub encabulator.inspection.passed \
  '{"id":"evt_local","type":"encabulator.inspection.passed","time":"2026-09-30T12:00:00.000Z","source":"manual","data":{"orderId":"ord_local","attempt":1,"serials":["ENC-turbo-local-0001"]}}'
```

## Tests

```bash
make test-unit   # carrier selection, tracking numbers, transit window
make test        # adds the Valkey + NATS suite
```

The integration suite covers the parts that only real infrastructure can prove: idempotent
booking, delivery scheduling, two replicas claiming the same queue without double-delivering,
survival across a restart, and re-arming a delivery whose event failed to publish. It skips
cleanly when `DOCKER_HOST` is unset.

## Container

```bash
make image   # build context is the repository root
```

Multi-stage: the kit is built first, then the service, then production dependencies only.
Runtime is `gcr.io/distroless/nodejs24-debian13` as UID 65532, read-only-root-filesystem
compatible.
