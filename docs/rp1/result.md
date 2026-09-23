# Goal Result

COMPLETE — RP-1 implemented and verified locally. Not deployed. Stop at RP-1.

## Executive Summary

CoreRP now has a minimal playable persistent RP loop over the existing world: one real player, three materialized NPCs, five real places, scoped observation, speech/hearing/Knowledge, event-backed moves, scheduler waits, replaceable NPC decisions, resumable turns and a mobile-first Play frontend. The current provider is deterministic, not an external LLM. Existing world/economy authority was reused.

## Phase Results

| Phase | Implemented / evidence | Verification | Checkpoint |
|---|---|---|---|
| A | Durable session, entity binding, presence, scoped observation; [report](phase-a.md) | Auth/privacy, upgrade/replay, process restart, full Go/vet/race PASS | `6978888` |
| B | Real route-backed move and scheduler wait; [report](phase-b.md) | Reachability, retry, partial wait/reopen, full Go/vet/race PASS | `8c8ed78` |
| C | Atomic accepted speech, audibility, Knowledge and Outbox; [report](phase-c.md) | False claims, offsite exclusion, crash/replay/idempotency, full Go/vet/race PASS | `809f9cc` |
| D | Filtered decision input, replaceable provider, validated real effects; [report](phase-d.md) | Refusal/movement/no-op, privacy, invalid proposals, failures, full Go/vet/race PASS | `b183bf6` |
| E | Durable turn state machine and committed narrative; [report](phase-e.md) | 20 turns, six crash stages, replay/reopen without redoing effects, full Go/vet/race PASS | `98de398` |
| F | Mobile-first real Play, legal routes/history, resume; [report](phase-f.md) | Full Go/vet/race, typecheck/build, real Chromium/HTTP restart PASS | Commit containing this report; `git log -- docs/rp1/result.md` |

Pre-RP Git baseline: `d24b083`. Source snapshot: `/home/ubuntu/corerp-rp1-baseline.RC6ULh/source-before-rp1.tar.gz`, SHA256 `a54e813b32b98de4fb7269c439a560105582c4b9531fe993f9811074e99c7c90`.

## Architecture Reuse

World/Branch/Rule Epoch; Cohort→materialized Entity and own asset accounts; existing Agent profiles, places, positions, movement/schedules and clock; Command/Validation/Event batches; Observation/Knowledge and replay/projection/snapshot; audit and transactional Outbox. The NPC's own economic state and next work schedule are filtered decision input. No RP character card, second location/clock authority, global database prompt or narrative-to-world backdoor.

## New Contracts

- 020: application-only RPSession and scoped player control binding.
- 021: world topology links for legal RP travel.
- 022: durable wait retry intent, existing scheduler completion boundary.
- 023: immutable accepted speech indexed to Event/Turn and actual hearer Knowledge.
- 024: immutable NPC decision/effect identity for one NPC per parent Turn.
- 025: durable turn workflow/request identity and committed narrative view.
- Filtered `RPDecisionProvider`, explicit proposal validation/commit, player-authenticated RP HTTP surface, scoped Observation legal destinations and recent history. F adds no schema migration.

## End-to-End Evidence

The real browser opens Lin's session at M2 Cafe, sees Cai, speaks and receives a refusal. SQLite confirms actual hearing records. It moves to Ada Home, where Cai cannot participate; Ada refuses because of her next work schedule. Repeated waits cross actual work/lunch tasks, then moving to the cafe reveals Ada, Bo and Cai. A subsequent turn commits on the real backend, but its response is deliberately lost. The browser and server are stopped and reopened against the same DB/bookmark. Same-key retry restores the original session/history without adding Event, hearing or speech rows, and a new turn then succeeds. No credential is persisted in localStorage; screenshots and layout assertions cover mobile/desktop.

Recovery evidence before and after retry: 32 Events, 30 observation records, 8 accepted utterances. Sample artifacts are documented in phase-f.md; `npm run verify:rp1-play` reproduces the scenario in a fresh temporary database. Backend 20-turn and six-stage fault tests separately prove recovery at each commit boundary, Knowledge/projection equality and snapshot/full replay equivalence.

## Verification

Executed against the final backend source:

```text
cd backend
/usr/local/go/bin/go test ./... -count=1
/usr/local/go/bin/go vet ./...
/usr/local/go/bin/go test -race ./internal/storage ./internal/transport/httpapi ./cmd/corerp-m2 -run 'TestRP|TestM2CLIRP|TestEmbeddedMigrationsMatchContracts|TestM2WagePolicyMigrationUpgradesExistingSplitLineage' -count=1
```

All PASS, including upgrade/reopen, targeted RP, privacy, crash and replay tests. Full storage 56.532s; race storage 63.040s.

Frontend/local evidence:

```text
npm run build
npm run verify:rp1-play
npm run verify:m0
npm run verify:m1-evidence
npm run verify:m2-evidence
git diff --check
```

PASS. Build includes `vue-tsc --noEmit`; no separate lint/unit-test script was configured. M0 52 checks; existing M1 evidence; M2 26 executable evidence references. No external LLM or production release was verified.

## Deferred

External LLM adapter/E2E `REQUIRED_IF_AVAILABLE`; new relationship/episodic-memory authority, Needs/Goal depth, StyleProfile, history pagination and session-bookmark portability, production identity/TLS/deployment, multiplayer/concurrent driving, advanced acoustic/privacy rules, audience-routing publisher enforcement, and all explicit goal exclusions (Jev, SillyTavern, MCP, Studio, expanded economy/career/law/culture). Existing participant-scoped Outbox is recorded but Play does not expose an unfiltered stream.

## Needs User Decision

None blocks RP-1. No new irreversible product decision was assumed.

## Next Recommended Milestone

RP-2: deepen NPC Needs/Goal/Relationship and daily life, or separately configure a real provider. Proposed only; not implemented. [Run and review Play](play.md).
