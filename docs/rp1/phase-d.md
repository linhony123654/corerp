# RP-1D — NPC DecisionProvider / Scoped Decisions

Status: PASS locally on 2026-09-23 after final regression. The continuous turn loop and Play UI are still pending.

## Implemented

- A replaceable `RPDecisionProvider` receives a deliberately filtered, immutable `RPDecisionInput` only after the NPC is proven to have heard the committed player speech and remains in the same scene. Input contains same-place visible identities, that NPC's own Goal, activity, next schedule, own asset balance/currency, own allowlisted Knowledge, the heard player utterance, and currently legal actions/routes. It excludes account identifiers, other people's finances, raw world/Creator state and audit data. No separate Relationship or episodic-memory authority exists in this M2 slice; observed Knowledge is the available memory evidence.
- The local deterministic provider can respond, refuse due to a borrowing request, low own funds or a work schedule, and returns identical proposals for identical inputs. The interface is replaceable by an LLM adapter when one exists; this repository has none. A provider error/cancellation yields an auditable safe-silence fallback; an illegal proposal is rejected and audited. Neither proposal path creates a world Event.
- `CommitRPDecision` independently rechecks input hash, Branch Head, scene positions, turn, pending wait and route, then creates one authoritative Event for each NPC/parent Turn pair. Replies/refusals become accepted utterances with actual listener evidence and Knowledge in the same transaction; leave uses the existing Agent movement/schedule/position chain; silence/wait are recorded no-effect choices. Migration 024 stores immutable NPC decision lineage. Session turn stage becomes `npc_effects_committed`, and the Outbox is scoped to participants. Retry returns the original Event; different effects conflict.

## Verification

| Check | Result |
| --- | --- |
| Full backend regression | PASS — `cd backend && /usr/local/go/bin/go test ./... -count=1` |
| Static analysis | PASS — `cd backend && /usr/local/go/bin/go vet ./...` |
| Related race | PASS — targeted RP/migration/HTTP/CLI `go test -race` |
| Migration parity / upgrade | PASS — embedded 024 SQL equals docs mirror; disposable 023→024 and older 019→024 upgrades retain player turn/world state |
| Input privacy / legality | PASS — Cai sees only heard speech and own filtered state; offsite Ada cannot read it; player cannot pose as NPC; invalid destination is rejected; legal leave is only a candidate until commit |
| Deterministic behavior / fallback | PASS — same input yields same proposal; own balance and work schedule affect policy; timeout/cancellation gives audited silence, not an Event |
| Actual NPC effects | PASS — Ada's refusal is committed as speech that Lin hears, without transferring money; Cai's leave changes authoritative position; silence/wait leave location unchanged; three same-place NPCs can commit child decisions sequentially |
| Recovery / replay | PASS — precommit injection rolls back NPC utterance/decision/knowledge, retry commits once; reopen retry returns original Event; changed effect conflicts; stale-world proposal cannot commit; projections replay across speech/movement |
| M2 evidence preservation | PASS — `npm run verify:m2-evidence` still finds 26 executable tests |

No actual external LLM was available in this repository, so real-provider E2E is `REQUIRED_IF_AVAILABLE`, not claimed passed. The deterministic and fixed test providers are executable substitutes for architecture/authority tests. `DecideRP` and `CommitRPDecision` are internal backend APIs; the upcoming RP-1E orchestrator will own their sequencing and external exposure. An NPC `wait` choice records a no-effect decision; world-time advancement remains the existing scheduler-backed player wait path.

## Recovery point and next stage

- RP-1C checkpoint: `809f9cc`.
- RP-1D checkpoint: the Git commit containing this report; find via `git log -- docs/rp1/phase-d.md`.
- Next: RP-1E durable, idempotent turn orchestration/recovery and a narrative view across player/NPC events. No full RP-1 DoD is claimed here.
