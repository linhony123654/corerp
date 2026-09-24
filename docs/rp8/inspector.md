# Studio event evidence — first backend increment

`POST /api/v1/studio/events/read` uses normal bearer authentication and the existing
`{data: ...}` / error envelope, no-store headers and strict JSON decoding. Input:
`instance_id`, `branch_id`, `event_id`; optional principal must match authentication.
All four resolved IDs are nonblank and at most256 bytes. There is no caller-selected
capability, role, SQL, provider, observer or ruleset override.

## Explicit permission boundary

An active creator needs an active `world.inspector.read` grant; an active operator
needs `diagnostics.inspector.read`. The grant must match the instance/branch, have
subject equal to that branch or `*`, and include both `event` and `rule` fields.
Roles, world.events.read and player world.rp.control do not imply this permission.
Player principals are denied even if a malformed setup gives them an inspector
grant. Authorization occurs before checking whether an event exists, in the same
read transaction as the evidence query. Every call rechecks status/scope/fields.

This increment does **not** auto-grant access, migrate all creators, or expose a
grant-writing API. Tests provision explicit grants only in disposable databases.
The [local operator configuration tool](access.md) provides Event-backed grant/revoke
without database editing. [Actual Studio UI](studio-ui.md) now verifies the complete
read/setup/restart path; browser clients never provision their own grants.

## Evidence semantics

- Actual Event ID/type/sequence/time and scoped branch head.
- Actual committed batch ID/hash and bound Rule Epoch ID/hash/sequence interval.
  The interval is checked; an event outside its epoch/head fails as divergence.
  A ruleset digest identifies the effective recorded ruleset; it does not claim a
  specific condition caused an outcome. Raw audit inputs are not exposed.
  For activated Studio worlds, creators also receive `rule.packages` containing
  the exact validated lock and System/Narrative bundles from immutable installation
  Events. `rule.package_status` is `verified`, `preparation`, `not_recorded`, or
  `redacted`. The reader selects the inspected batch's historical epoch, never the
  branch's current epoch. It verifies activation identity/sequence/hash, manifest
  and content pins, dependency set, and that installation preceded activation.
  Corrupt or missing sources fail closed. These are installed base rules, not proof
  all rules caused this Event or that later narrative overlays were absent.
- Creator: actor, command ID, exact stored payload and an in-scope recorded causal
  Event reference when present. `not_recorded` means the causal column is absent,
  not “uncaused.” `outside_scope` does not reveal the foreign Event ID.
- Operator: no payload, actor, command, causal Event ID or package contents; redacted=true and causal
  evidence marked redacted. Diagnostic batch/rule/Event metadata remains visible.

No rule execution, NPC decision call, fabricated explanation, cursor update,
observation, audit append or world write occurs. SQLite read snapshot is rolled
back/released after reading; no transaction is held across HTTP/network work.
The read API adds no migration (current world schema030). Most current writers do not populate causation_event_id;
full8B must also inspect recorded command/audit/knowledge provenance rather than
pretend a sparse causal column explains all outcomes.

## Verification and remaining work

Initial `go test ./internal/storage ./internal/transport/httpapi -run TestStudioEvent
-count=1 -timeout=3m` PASS (0.183s/0.155s). Expanded55159 terminal PASS:
`go test -race ./internal/storage ./internal/transport/httpapi -run
'TestStudioEvent|TestVisibleEventsRespect' -count=1 -timeout=5m`
(storage6.190s/HTTP2.317s); `go vet ./internal/storage ./internal/transport/httpapi`;
full HTTP/server normal tests (12.602s/0.147s). No full backend/stage race claim.
Tests cover real persisted genesis and purchase metadata, explicit grant boundaries,
operator field omission, status/field/subject revocation, cross-world/branch,
missing/oversized input, no world mutation, projection rebuild and reopen; HTTP
tests exercise authentication, spoofed principal/unknown field, method and cache
headers against real SQLite, not a mock service.

Subsequent [recorded explanation](explanation.md), [scope/timeline discovery](discovery.md),
[creator UI](creator-ui.md) and historical package inspection are implemented.
Historical-rule normal48170 PASS storage5.855s/HTTP0.518s; race11622 PASS
storage56.802s/HTTP6.041s plus wholebackend vet. Real creator-to-Play-to-Inspector
browser9614 PASS `/tmp/corerp-rp8-play-worlds-N3zLal`, including exact package content,
activation-event keyboard navigation back to its preparation epoch, ops redaction
and direct player403. Full-stage gates and final report are recorded separately.
