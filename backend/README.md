# CoreRP M1 strict-world kernel

This module is the executable M1 vertical slice of the M0 contract: one demo instance, one branch, one open Rule Epoch, one enterprise, three employees, one landlord, and one store backed by a real SQLite file.

The kernel provides authoritative genesis, deterministic world time and stable scheduler order, a strict 90-day wage/rent/household-purchase/restock run, atomic purchase and controlled-issuance commands, balanced obligation accounting, durable Outbox dispatch, replay/snapshots/projection repair, and scoped private-economic reads. Scheduler work is budgeted and restartable; each item commits independently, and a final clock checkpoint advances the requested day only after earlier due phases. A separate HTTP process now exposes authenticated commands/queries plus scope-filtered finite events and resumable SSE without giving transport handlers direct database authority.

The additive M2 demo also provides a bounded two-Agent path: T09-backed stable identities, deterministic schedules/movement, scoped observations and knowledge, replay/projection recovery, and an opt-in 30-day spatial driver. Its separate Cohort economy settles finite wages/rent, purchases, consumption and supplier stock transfers. Migrations 016–019 extend one-person wage participation with a versioned cumulative partial-payment rule, linked arrears cure, T09 transfer/return of only unpaid worker-slot claims, immutable origin and actual-recipient receipts, and a named-creditor bankruptcy-opening snapshot. Creator-scoped storage reads expose current and opening wage-claim status. When split claims exist, the old day-31 estate payment is recorded as deferred rather than routed to the wrong creditor; the pre-existing unsplit/post-term T09 and estate fixture remains unchanged. These are fixture-local accounting rules, not general liquidation or creditor priority. Complete employment-contract splitting, post-proceeding split-claim distribution/discharge, production identity/TLS, autonomous decisions, full LOD/economy/status, Rule Epoch hot activation, FX, frontend integration and deployment remain outside this slice. The built-in static Bearer-token adapter is local/development configuration only.

## Run

The workspace Go installation is currently available at `/usr/local/go/bin/go`:

```bash
cd /home/ubuntu/corerp-console/backend
/usr/local/go/bin/go test ./...
/usr/local/go/bin/go run ./cmd/corerp-m1 -db /tmp/corerp-m1.db -action purchase
/usr/local/go/bin/go run ./cmd/corerp-m1 -db /tmp/corerp-m1-90.db -action simulate90
/usr/local/go/bin/go run ./cmd/corerp-m1 -db /tmp/corerp-m1.db -action inspect
```

Run the local HTTP/SSE process:

```bash
export CORERP_AUTH_TOKENS_JSON='{"buyer-dev":"principal_buyer","creator-dev":"principal_creator","operator-dev":"principal_operator"}'
export CORERP_CURSOR_SECRET='replace-with-a-local-secret-of-at-least-32-bytes'
/usr/local/go/bin/go run ./cmd/corerp-server -db /tmp/corerp-http.db -listen 127.0.0.1:8080
```

The full route, envelope, authentication, cursor, and SSE contract is in [`../docs/m1/http-api.md`](../docs/m1/http-api.md).

Initialize and exercise the isolated M2 T09 Cohort fixture without changing the M1 demo instance:

```bash
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2.db -action bootstrap
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2.db -action roundtrip
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2-agents.db -action agents
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2-agents30.db -action agents30-prepare
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2-agents30.db -action agents30-drive -interval 1s -batch 4
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-rp1.db -action rp-prepare
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-rp1.db -action rp-travel-prepare
```

The schema, conservation boundary, evidence, and explicit broader-M2 deferrals are in [`../docs/m2/`](../docs/m2/).
The opt-in `rp-prepare` action reuses the M2 instance, T09 identities and Agent positions to add a real player, a third NPC, and a player-scoped control grant. `rp-travel-prepare` adds event-backed routes without a second location table. Start the HTTP server against that same database and map a local development token to `principal_m2_rp_player`; the RP open/read/resume/close/observe/move/wait/speak and durable turn run/resume endpoints are documented in [`../docs/rp1/`](../docs/rp1/README.md). NPC decision/commit remains an internal backend API owned by the turn orchestrator, not an unscoped HTTP endpoint. No server startup implicitly prepares the fixture. The default frontend now connects these APIs through player-only Play; see [local Play setup and recovery](../docs/rp1/play.md). Observation includes current legal destinations and session-scoped committed history, never a raw event stream.

## Readiness and next backend scope

RP-2A adds an operator-configured real Chat Completions DecisionProvider with strict local validation, bounded retries and safe silence fallback. Default remains deterministic. RP-2B adds sourced Life Context and interpersonal actions; RP-2C adds [minimal background initialization](../docs/rp2/background.md) for real materialized individuals, with existing identity/employment evidence and stable re-encounters. RP-2D adds [scoped narrative styles](../docs/rp2/style.md), immutable per-turn style pins and same-event read-only variants (schema026). See [configuration and authority boundaries](../docs/rp2/README.md); live model verification is not claimed without configured credentials. Current incremental product work is tracked in [RP6](../docs/rp6/phase-6.md). Its independent optional [natural-language style planner](../docs/rp6/custom-style.md) uses separate `CORERP_NARRATIVE_*` configuration and cannot rewrite world facts; arbitrary literary expansion is not supported.

A future web, desktop, or mobile client can integrate without choosing a UI stack first: commands and queries use stable JSON envelopes, identity is injected at the HTTP boundary, and live visible events resume through standard SSE `Last-Event-ID`. The client must not connect to SQLite or infer authority from UI state.

This is local-development backend readiness, not production readiness. Production operation still needs a real identity provider/session lifecycle, TLS and explicit origin policy, managed secret rotation, rate/abuse limits, persistent metrics/tracing, deployment/backup procedures, and a versioned generated client contract if desired.

T09 provides idempotent Cohort materialization/dematerialization with five-dimension conservation. The Agent slice adds scoped schedules, evidence-backed observations/knowledge and read-only encounters. A third, opt-in slice can declare and drive their deterministic spatial routines for 30 world days, checkpointing every move; the ordinary HTTP server never auto-starts it. This does not complete M2: broader action/economic/LOD/status logic and the 100-resident/10-store scale gate remain. Runtime Rule Epoch activation remains M3; Inspector product UI remains M4.

Running the same purchase command again returns the original committed result with `replayed: true` and does not append another event.

`simulate90` processes the strict world through day 90 with a work budget of 1000 and prints the durable run result plus a compact projection state. Re-running it against the same database processes no economic item again; only already-completed stable scheduler IDs exist.

Release evidence:

```bash
cd /home/ubuntu/corerp-console
npm run verify:m0
npm run verify:m1-evidence
npm run verify:m2-evidence
cd backend
/usr/local/go/bin/go vet ./...
/usr/local/go/bin/go test -race ./...
```

Migration 001 remains byte-identical to [`../docs/m0/schema.sql`](../docs/m0/schema.sql). Every additive M1/M2 migration has an exact documentation mirror, and the Go suite enforces all byte-parity contracts.
