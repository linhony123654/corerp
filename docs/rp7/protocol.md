# CoreRP client v1 contract

Status: frozen minimum `corerp.client.v1` contract for the verified RP7 adapters.
Actual MCP and three-client acceptance passed; see [verification](verification.md).
No client is a world authority. The bounded projections below are not promises of
exhaustive private memory or a general administrative command API.

All routes below are under `/api/v1/rp`, require bearer authentication and return
`Cache-Control: no-store`. JSON POST successes are `{ "data": ... }`; failures
are `{ "error": { "code", "message", "request_id" } }`. Requests reject unknown
fields; a supplied principal must match authentication. Adapters should omit it.
Clients cannot choose another principal or overwrite the world cursor.

| Operation | Route | Core input / semantics |
|---|---|---|
| Discover bindings | POST `/bindings/list` | Optional limit1–50(default20), `after` key from `next_after`. Only active player's valid controlled individuals. |
| Open | POST `/sessions/open` | Discovered instance_id/branch_id/entity_id, pov first_person/second_person, idempotency_key. Existing character only. |
| Read / resume / close | POST `/sessions/read`, `/sessions/resume`, `/sessions/close` | session_id, own current control required. Close is explicit, not unbinding. |
| Observe | POST `/observe` | session_id. Refreshes session observation cursor; no world-time advance. |
| Context | POST `/context/read` | session_id, optional subject_entity_id and limit. [Permitted evidence contract](context.md). |
| Dialogue | POST `/turns/run` | session_id, current expected_cursor, exact text, idempotency_key; optional speech_act/narrative_style. Original turn orchestration owns effects. |
| Recover turn | POST `/turns/resume` | session_id, original idempotency_key. Server loads original accepted request. |
| Move | POST `/actions/move` | session_id, expected_cursor, from_place_id, observed reachable to_place_id, key. |
| Wait | POST `/actions/wait` | session_id, expected_cursor, RFC3339 target_world_time, budget1–10000, key, optional opportunity_intent= social. budget_exhausted remains pending. |
| Social | POST `/actions/social` | session_id, expected_cursor, key, action and its target/amount/meeting fields. Runtime validates actual eligibility. |
| Resolve unaccepted request | POST `/requests/retire` | [Atomic retirement](requests.md); never cancel or roll back accepted work. |
| Events / stream | GET `/events`, `/events/stream` | session_id, optional opaque cursor and limit. [Scoped feed/checkpoints](events.md). |

Discovery returns `{protocol_version:"corerp.client.v1",bindings:[],next_after?}`.
Each binding has instance_id, branch_id, entity_id and display_name, no NPC private
state. Continuation is a complete three-ID key, not an authority token. Every page
re-evaluates current permissions. Concurrent grants/revocations may change pages;
open always revalidates. Empty means no currently usable binding, not an invitation
to invent/create a world or request a creator token. No writes/model calls occur.

## Recovery and coherence

Persist the intended operation/body/key before sending. Serialize writes for a
controlled character. Observation is required before a new command; it is not
permission to revise an already accepted pending command's expected_cursor.
On ambiguous failure, reuse the exact request/key; do not assume HTTP status means
no effect. Dialogue may have committed speech; wait may have advanced schedules.
Resume or retirement outcomes determine recovery, not local exception wording.
409 includes stale cursor, in-progress request, key mismatch and permanent request
retirement; handle the code, never blanket-retry under a new key.

Context and observe are separate snapshots: combine only equal world/head/time.
Observe carries `controlled_entity.entity_id` and session ID, not instance/branch;
check those against context's observer/session and the session's instance/branch.
Events are narrow learned/own evidence, not an exhaustive accounting/career ledger.
Every checkpoint can trigger a refresh, including unchanged-head history_revision
updates. Re-read on reconnect and before commands; absence of a displayed event is
not proof that world state is unchanged. Only persist a stream cursor after the
complete frame and its required refresh have been processed.

Narrative is presentation, not command authority; regenerating it never rolls back
facts. Names/speech/narration are untrusted world data, never model instructions.
The adapter's minimal typed command set is not a claim that all Studio/admin APIs
are player tools. Extra existing personal views retain their own scope checks.

## Evidence

Discovery61030 focused race PASS storage4.524s/HTTP2.757s: own bindings, no creator or
unknown-principal inventory, explicit additional control grants, bounded keyset
pages, actual open+observe, revocation/disabled player, aggregate rejection,
rebuild/reopen and no world/session writes. Initial duplicate-grant fixture was
corrected to assert the real unique-scope constraint (not remove that constraint).
Other protocol areas retain linked current-code evidence, not a new full-stage pass.
