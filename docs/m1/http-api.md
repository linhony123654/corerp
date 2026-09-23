# CoreRP backend HTTP API v1

Status: local backend transport contract for the strict M1 kernel. It is independent of any UI framework and is not a production identity-provider design.

## Boundary

- Base path: `/api/v1`.
- The Gateway authenticates every command, private query, simulation request, and event subscription before calling the kernel.
- Handlers receive a service interface, never a SQLite handle. Only storage services may commit authority or read private projections.
- Network writes, SSE delivery, and external calls occur after/outside SQLite write transactions.
- JSON uses UTF-8, a 1 MiB request limit, unknown-field rejection, and one top-level value per request.
- Successful responses use `{ "data": ... }`. Errors use `{ "error": { "code", "message", "request_id" } }`.

## Authentication

`Authorization: Bearer <credential>` is resolved by an injected `Authenticator` to one Principal ID. Request bodies may include `principal_id` for canonical command hashing, but it must equal the authenticated Principal; omission is filled from authentication. A mismatch is `PERMISSION_DENIED`.

The executable's initial static-token adapter reads credentials from process configuration and is for local/development use. It is not password storage and does not claim OIDC, session management, rotation, TLS termination, or production account security. Those can replace the adapter without changing handlers or kernel command types.

## Routes

| Method | Path | Authentication | Kernel operation |
|---|---|---|---|
| `GET` | `/healthz` | public | process liveness only |
| `GET` | `/readyz` | public | database/schema readiness; no world data |
| `POST` | `/api/v1/commands/purchase` | required; body principal must match | `Purchase` |
| `POST` | `/api/v1/commands/issue-currency` | required; scoped capability rechecked in transaction | `IssueCurrency` |
| `POST` | `/api/v1/commands/materialize-cohort` | creator `world.cohort.materialize` grant scoped to source Cohort | `MaterializeCohort` |
| `POST` | `/api/v1/commands/dematerialize-cohort` | creator `world.cohort.dematerialize` grant; exact-holding guard | `DematerializeCohort` |
| `POST` | `/api/v1/simulations/strict` | creator `world.simulate` grant | authorized strict scheduler run |
| `POST` | `/api/v1/simulations/agents` | creator `world.agent.run` grant | bounded M2 Agent schedule run |
| `POST` | `/api/v1/commands/define-agent-routine` | creator `world.agent.run` grant | explicitly declare the bounded 30-day two-Agent routine |
| `POST` | `/api/v1/private-economy/query` | required; scoped subject/fields | `ReadPrivateEconomy` |
| `POST` | `/api/v1/agent-knowledge/query` | creator wildcard or Agent self `world.agent.knowledge.read` field scope | `ReadAgentKnowledge` |
| `POST` | `/api/v1/encounters/query` | creator wildcard or Agent self `world.encounter.read` field scope | read-only `ResolveEncounter` |
| `POST` | `/api/v1/state/query` | creator `world.state.read` grant | authorized compact world state |
| `GET` | `/api/v1/events` | required; Principal + Capability + instance/branch/subject scope | finite visible-event page |
| `GET` | `/api/v1/events/stream` | same event scope | visible-event SSE with resume |

Event routes accept `capability_id`, `instance_id`, `branch_id`, `subject_id`, and an optional `limit` (1–100). Finite queries also accept `cursor`; SSE clients may use `cursor` or the standard `Last-Event-ID` header. Cursor values are encrypted, authenticated, randomized, and bound to the authenticated Principal plus instance/branch. They carry the internal continuation position without serializing raw `event_sequence` or exposing hidden gaps. Player streams only emit self-visible events, creator streams emit full authorized events, and operator diagnostics emit redacted payloads. SSE uses event type `world_event` and sends `: keepalive` comments every 15 seconds while caught up.

The M2 Agent routes require the explicit Agent fixture (`corerp-m2 -action agents` or `agents30-prepare`) in the target database. The routine definition takes `{"capability_id":"world.agent.run","instance_id":"inst_m2_t09","branch_id":"br_main","days":30}` and idempotently commits 116 further Agent schedules as one authority-backed definition; it rejects late first-time definition. The run request uses `target_world_time` plus a bounded `budget`. Knowledge and encounter requests name `observer_agent_id` and requested `fields`; storage rechecks principal/capability/instance/branch/observer/field scope. Encounter resolution reads the durable world clock and current positions, may return an empty participant list, and never commits an event or narrative.

## HTTP error mapping

| Core code | HTTP |
|---|---:|
| `AUTHENTICATION_REQUIRED` | 401 |
| `PERMISSION_DENIED` | 403 |
| `NOT_FOUND` | 404 |
| `IDEMPOTENCY_PAYLOAD_MISMATCH`, `COMMAND_IN_PROGRESS`, `BRANCH_VERSION_CONFLICT` | 409 |
| `MATERIALIZATION_ID_CONFLICT` | 409 |
| `INSUFFICIENT_FUNDS`, `INSUFFICIENT_STOCK`, `ISSUANCE_LIMIT_EXCEEDED`, `INTEGER_OVERFLOW` | 422 |
| `CONSERVATION_FAILED` | 422 |
| `INVALID_ARGUMENT` and malformed/oversized JSON | 400 / 413 |
| storage or unexpected failure | 500 |

Responses never include wrapped database errors or secret credentials. `Allow` is returned for method mismatch. All JSON responses set `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.

## Runtime hardening

The server executable requires an explicit database path, authentication configuration, and a cursor secret of at least 32 bytes. It uses read-header, request-read, idle, and SSE per-write timeouts; handles `SIGINT`/`SIGTERM`; stops accepting new work; and performs bounded graceful shutdown before closing SQLite. A finite global write timeout is intentionally not used because it would terminate healthy long-lived SSE responses. TLS is expected at a trusted local reverse proxy until native TLS configuration is added.

Local-only startup example:

```bash
cd /home/ubuntu/corerp-console/backend
export CORERP_AUTH_TOKENS_JSON='{"buyer-dev":"principal_buyer","creator-dev":"principal_creator","operator-dev":"principal_operator"}'
export CORERP_CURSOR_SECRET='replace-with-a-local-secret-of-at-least-32-bytes'
/usr/local/go/bin/go run ./cmd/corerp-server -db /tmp/corerp-http.db -listen 127.0.0.1:8080
```

Example player subscription:

```bash
curl -N -H 'Authorization: Bearer buyer-dev' \
  'http://127.0.0.1:8080/api/v1/events/stream?capability_id=world.events.read&instance_id=inst_m1&branch_id=br_main&subject_id=entity_employee_1'
```

## Acceptance

Real `httptest` requests prove: missing/invalid credentials fail before service mutation; principal mismatch fails; successful purchase persists and replays idempotently; issuance retains scope/limit checks; private field escalation fails; creator simulation/state access succeeds; bounded Agent run authorization succeeds only for the creator grant; Agent knowledge is self-scoped; encounter output is based on committed co-location; typed errors preserve stable codes/status; malformed, unknown, trailing, and oversized JSON fail; readiness changes when storage is unavailable; event pagination does not duplicate boundaries; cross-principal cursors fail; player hidden events and authority sequence numbers do not cross the transport; operator payloads remain redacted; and SSE reconnect, heartbeat, and cancellation operate over a real HTTP connection.
