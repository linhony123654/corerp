# CoreRP RP-1 playable vertical slice

## Goal
Implement and verify all six stages of `/tmp/f1edf80b-3702-4376-9e97-85d6670a9109.md` without replacing existing world/economy authority; stop at RP-1 DoD.

## Route
Full-project DEFINE → DESIGN → BUILD → POLISH → local SHIP. Crosses SQLite authority, HTTP, NPC decisions, turn recovery, and UI. No production publication.

## Current Phase
Baseline / DEFINE / DESIGN. No RP-1 implementation has been validated yet.

## Phases

### Baseline and architecture
- [ ] Read v0.5, RP-0, M2 contracts, migrations and affected tests
- [ ] Record Git state, source archive SHA256 and baseline checks
- [ ] Map existing authority to RP contract, design first vertical slice
- **Status:** in_progress

### RP-1A — Session / Presence / Observation
- [ ] Durable idempotent real World/Branch/Entity binding and resume
- [ ] Derived location/presence and privacy-scoped observation
- [ ] Minimal authenticated HTTP gateway, restart and negative tests
- [ ] Full Go tests/vet/related race, report and checkpoint
- **Status:** pending

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
- Existing Git repo is empty (`main`, no HEAD); phase commits are explicitly authorized by the linked RP-1 goal. Protect the pre-RP source with an archive and SHA256 first.
- Keep existing `.planning/2026-09-22-corerp-m0-rfc` intact as historical state; this plan is independent.
- Architecture choices remain provisional until source/RP-0 baseline audit.

## Errors
- First historical-plan read was truncated by output limits; use targeted sections for needed facts.
- `vexor` semantic search failed with `OpenAI API request failed: Connection error.`; use scoped local `rg`/file inspection instead, without repeated retries.
- `git rev-parse HEAD` failed because this repository has no commits; recorded as expected baseline state, not a blocker.

## Gate state
- DEFINE: in_progress. DESIGN/BUILD/POLISH/SHIP: not yet passed.
