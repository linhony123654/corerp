# CoreRP RP-1 playable vertical slice

## Goal
Implement and verify all six stages of `/tmp/f1edf80b-3702-4376-9e97-85d6670a9109.md` without replacing existing world/economy authority; stop at RP-1 DoD.

## Route
Full-project DEFINE → DESIGN → BUILD → POLISH → local SHIP. Crosses SQLite authority, HTTP, NPC decisions, turn recovery, and UI. No production publication.

## Current Phase
RP-1D DESIGN — replaceable NPC decision provider, legally filtered input and validated proposals. RP-1C passed and awaits checkpoint commit.

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
- [x] Git checkpoint commit `6978888`
- **Status:** complete

### RP-1B — Player Action / Move / Wait
- [x] Validated event-backed move, projection, derived observation
- [x] Existing scheduler-backed wait and retry/recovery semantics
- [x] Full Go tests/vet/related race and report
- [x] Git checkpoint commit `8c8ed78`
- **Status:** complete

### RP-1C — Speech / Knowledge / Atomic Turn
- [x] Accepted utterance, audibility evidence, listener knowledge in one authority commit with turn stage and Outbox
- [x] Crash/retry/reopen/replay, privacy and false-claim tests
- [x] Final full Go tests/vet/related race and report
- [ ] Git checkpoint commit
- **Status:** validated, checkpoint pending

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
- RP-1A checkpoint `6978888` is clean and passed final full Go/vet/related race, migration/replay and actual-process restart smoke. Phase report: `docs/rp1/phase-a.md`.
- RP-1B design: add versioned place links as world topology, not presence/location authority. A separate explicit setup event after RP-1A declares links, preserving existing head-7 fixture and migration compatibility. An immediate player move is one validated Event Batch using the existing `agent_movements`/`agent_positions` chain; because schema 007 accepts only `scheduled`, its same-transaction scheduler item/entry is created already completed, never run a second time. Guard session control, observed head, from-place and declared link; use existing idempotent command/event/Outbox pattern and co-location knowledge. A focused cafe→home move and forbidden home→work move are the representative slice.
- RP-1B wait: a narrow application `rp_wait_intents` table records the target/request hash before any scheduler work, not a second clock. On retry, the same intent resumes within a fixed work budget; different payload conflicts. The existing M2 runner settles due items, and only after `pending_due=0` does a separate short transaction commit a `RPWaitCompleted` Event, same world-clock checkpoint, Branch Head, intent completion and Outbox. Budget exhaustion returns partial status and current authoritative time; no false completion. This avoids a pending world command whose original expected head would become stale as scheduler events commit. Branch-level pending RP wait blocks another RP move/wait; closing its session is denied until completion. Privileged concurrent external scheduler advancement is outside the single-player slice and may create a reported conflict; never mark a false completion.
- RP-1C design: add immutable accepted utterance metadata linked to one Event and Turn ID, not an editable transcript authority. Same-place active profiles are the only first-slice hearers (no whisper/acoustic/privacy rule exists yet). For each listener insert an `observation_records` row using the existing `co_location` evidence channel, with `claim_type=speaker_said` and explicit speaker/utterance attribution; update `agent_knowledge` in the same transaction. This reuses the existing replay/rebuild path and never asserts the content as objective economic/character truth. Commit utterance, listener evidence/knowledge, session turn stage, Branch Head/clock lineage and Outbox together. Deny a new speech while a branch wait is pending.
- Representative first slice: Open session for a T09 materialized entity with a player control grant, Observe actual position, close/reopen DB and observe again. Broaden to invalid bindings, privacy, HTTP and 3-NPC fixture only after this path passes.

## Errors
- First historical-plan read was truncated by output limits; use targeted sections for needed facts.
- `vexor` semantic search failed with `OpenAI API request failed: Connection error.`; use scoped local `rg`/file inspection instead, without repeated retries.
- `git rev-parse HEAD` failed because this repository has no commits; recorded as expected baseline state, not a blocker.
- Initial `git diff --cached --check` flagged three pre-existing whitespace issues in migration 006 and `scripts/verify-m0.cjs`; preserved source, committed baseline with known formatting debt.
- First RP-1A documentation patch failed atomically due to a wrong root README heading; corrected with a targeted patch after rereading the file.

## Gate state
- DEFINE: PASS — six stages and DoD from goal are observable. RP-1A/B DESIGN/BUILD/POLISH PASS with checkpoints. RP-1C DESIGN/BUILD/POLISH PASS: final full Go, vet, related race, migration/replay/reopen/HTTP and docs; checkpoint pending. RP-1D–F pending.
