# RP-1E — Durable Turn Orchestration / Recovery / Narrative

Status: PASS locally on 2026-09-23 after final regression. The Play UI and real browser/process E2E remain RP-1F.

## Implemented

- Migration 025 adds `rp_turn_runs`, a durable application workflow for one active conversational turn per session. It records request intent/idempotency and the stages `open → player_committed → npc_deciding → npc_effects_committed → narrative_ready → settled`; it is not a second world, location, clock or Knowledge authority.
- `RunRPTurn` reserves the turn, submits the player utterance using the existing idempotent speech command, freezes its actual listener IDs, and commits each listener's NPC decision/effect through RP-1D. At every retry it checks committed speech/NPC facts and skips them. The Session stage and observation cursor advance before settlement. Unsettled turns block direct competing move/wait/speech and session close.
- A simple narrative renderer reads exact accepted utterances and committed NPC effects only. It neither calls a model nor writes world facts; a claim like “I have a million” stays quoted dialogue. The settled result caches only a narrative view and stable Event/cursor references. Same-key replay does not call the Provider; even a nil Provider can read a settled turn.
- Authenticated POST `/api/v1/rp/turns/run` uses the local deterministic provider; POST `/api/v1/rp/turns/resume` resumes from the stored request with session/key only. A real LLM adapter can be supplied through the replaceable internal `RunRPTurn` API when available.

## Verification

| Check | Result |
| --- | --- |
| Full backend regression | PASS — `cd backend && /usr/local/go/bin/go test ./... -count=1` |
| Static analysis | PASS — `cd backend && /usr/local/go/bin/go vet ./...` |
| Related race | PASS — targeted RP/migration/HTTP/CLI `go test -race` |
| Migration parity / upgrade | PASS — embedded 025 SQL equals docs mirror; disposable 024→025 and older upgrade tests preserve prior world/turn facts |
| Twenty-turn run | PASS — 20 deterministic player turns with NPC replies, database close/reopen after turn 10, scheduler-backed advance to noon, a real player move, derived presence, projection comparison, and full-vs-snapshot replay hash equality |
| Stage recovery | PASS — injected stop after turn intent, player Event, player-stage update, NPC Event, NPC-stage update and narrative-ready stage; each resumed from a reopened database with one player utterance/one NPC effect, consistent Knowledge, and no duplicate committed provider call |
| Provider failure / false claim | PASS — provider error becomes an audited committed silence choice, while false player speech remains attributed dialogue rather than currency/economic fact |
| HTTP scope / restart | PASS — non-player token cannot run another player's turn; player token runs/replays/resumes after reopening the server's database handle; observation cursor matches settled sequence |
| Outbox | PASS — world Events and listener Knowledge are committed before delivery; pending Outbox survives recovery and is independently retryable |
| M2 evidence preservation | PASS — `npm run verify:m2-evidence` still finds 26 executable tests |

The current orchestrator owns conversational speech turns. Move and scheduler-backed wait remain explicit, separately idempotent actions between turns; they are included in the 20-turn continuity test. There is no production identity, external LLM or autonomous background turn worker. Recovery uses the original session/key through the HTTP resume route; the final frontend will persist them. The narrative renderer is intentionally minimal, and optional StyleProfile was deferred because it is not needed for world correctness.

## Recovery point and next stage

- RP-1D checkpoint: `b183bf6`.
- RP-1E checkpoint: the Git commit containing this report; find via `git log -- docs/rp1/phase-e.md`.
- Next: RP-1F mobile-first CoreRP Play UI and real backend/browser/process E2E, then final RP-1 DoD audit. No full RP-1 completion is claimed here.
