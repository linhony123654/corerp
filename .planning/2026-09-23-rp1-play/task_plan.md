# CoreRP RP-1 playable vertical slice

## Goal
Implement and verify all six stages of `/tmp/f1edf80b-3702-4376-9e97-85d6670a9109.md` without replacing existing world/economy authority; stop at RP-1 DoD.

## Route
Full-project DEFINE → DESIGN → BUILD → POLISH → local SHIP. Crosses SQLite authority, HTTP, NPC decisions, turn recovery, and UI. No production publication.

## Current Phase
RP-1A local gate passed; checkpoint pending, then RP-1B.

## Phases

### Baseline and architecture
- [x] Read relevant v0.5, RP-0, M2 contracts, migrations and affected tests
- [x] Record Git state, source archive SHA256 and baseline checks
- [x] Map existing authority to RP contract and choose first vertical slice
- **Status:** complete

### RP-1A — Session / Presence / Observation
- [x] Durable idempotent real World/Branch/Entity binding and resume
- [x] Derived location/presence and privacy-scoped observation
- [x] Minimal authenticated HTTP gateway, restart and negative tests
- [x] Full Go tests/vet/related race and report
- [ ] Git checkpoint commit
- **Status:** in_progress — verification PASS; checkpoint pending

### RP-1B — Player Action / Move / Wait
- [ ] Validated event-backed move, projection, derived observation
- [ ] Existing scheduler-backed wait and retry/recovery semantics
- [ ] Full Go tests/vet/related race, report and checkpoint
- **Status:** pending

### RP-1C — Speech / Knowledge / Atomic Turn
- [ ] Accepted utterance, audibility evidence, listener knowledge in one authority commit with turn stage and Outbox
- [ ] Crash/retry/reopen/replay, privacy and false-claim tests
- [ ] Full Go tests/vet/related race, report and checkpoint
- **Status:** pending

### RP-1D — NPC DecisionProvider
- [ ] Replaceable deterministic provider and legally filtered NPC input
- [ ] Validated respond/refuse/silence/action proposals; failure no-op
- [ ] Full Go tests/vet/related race, report and checkpoint
- **Status:** pending

### RP-1E — Orchestrator / Recovery
- [ ] Durable idempotent turn state machine and narrative view
- [ ] 20 deterministic turns and crash/restart/replay scenarios
- [ ] Full Go tests/vet/related race, report and checkpoint
- **Status:** pending

### RP-1F — Minimal CoreRP Play
- [ ] Mobile-first player-only play UI and session resume
- [ ] Real backend E2E with speech, NPC, move, wait and process restart
- [ ] Frontend configured checks; final DoD audit, report and checkpoint
- **Status:** pending

## Non-goals
No Jev, SillyTavern, MCP, Studio, character-card authority, multiplayer, new macroeconomy, or production deployment.

## Decisions and risks
- Pre-RP Git baseline commit `d24b083` on `main`; source archive SHA256 `a54e813b32b98de4fb7269c439a560105582c4b9531fe993f9811074e99c7c90`. Phase commits are authorized by the linked RP-1 goal.
- Keep existing `.planning/2026-09-22-corerp-m0-rfc` intact as historical state; this plan is independent.
- RP-1A: a dedicated application-state `rp_sessions` table stores only binding/cursors/lifecycle; `capability_grants` owns `world.rp.control` authorization; `materialized_entities`/`agent_profiles`/`agent_positions` remain sole identity/location authority. Observation derives from position and world clock under one SQLite snapshot and returns only player-visible fields. A player profile can be spatial but must not be selected for NPC AI decisions.
- Representative first slice: Open session for a T09 materialized entity with a player control grant, Observe actual position, close/reopen DB and observe again. Broaden to invalid bindings, privacy, HTTP and 3-NPC fixture only after this path passes.

## Errors
- First historical-plan read was truncated by output limits; use targeted sections for needed facts.
- `vexor` semantic search failed with `OpenAI API request failed: Connection error.`; use scoped local `rg`/file inspection instead, without repeated retries.
- `git rev-parse HEAD` failed because this repository has no commits; recorded as expected baseline state, not a blocker.
- Initial `git diff --cached --check` flagged three pre-existing whitespace issues in migration 006 and `scripts/verify-m0.cjs`; preserved source, committed baseline with known formatting debt.
- First RP-1A documentation patch failed atomically due to a wrong root README heading; corrected with a targeted patch after rereading the file.

## Gate state
- DEFINE: PASS — six stages and DoD from goal are observable. DESIGN: PASS for RP-1A slice only; later phases require their own local design. BUILD/POLISH/SHIP: pending.
