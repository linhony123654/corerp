# RP-2A — Real DecisionProvider

Status: PASS locally after final regression, 2026-09-23. RP-2B–E and later milestones remain pending.

## Recon / reuse / representative slice

Baseline `e1aa4c7`, schema025, clean main. Reused `RPDecisionInput`, `RPDecisionProposal`, filtered storage context, `DecideRP`, proposal validation, `CommitRPDecision`, durable `RunRPTurn` and player-only HTTP auth. No LLM adapter existed. The representative slice was filtered input → local HTTP Chat Completions request/schema → validated refusal, followed by existing world commit and recovery.

## Implemented

- `internal/decision`: actual configurable HTTP adapter, versioned context, strict schema and independent local validation, bounded total timeout/attempts/retry, redirect/size limits and sanitized errors. No storage dependency or ability to write world state. Deterministic remains the default.
- `internal/core/rp_proposal.go`: shared domain proposal validation, called both by adapter and existing authoritative path; unknown actions cannot bypass validation through an accidentally permissive action list.
- `storage.RPService`: immutable provider selection for run/resume; inherited Store handles other APIs. Startup validates explicit CoreRP configuration; no credentials in browser.
- Player Observation includes only configured mode; UI reports actual mode rather than always claiming deterministic. No UI redesign or later RP-6 implementation.
- Browser verifier supports an explicit local HTTP model fixture and strips inherited live configuration. No new migration, world authority or dependency.

## Verification

- Adapter focused tests PASS: request structure/privacy separation, schema failures, illegal/unreachable/mixed proposals, length, refusal/truncation/tools, HTTP retry/nonretry, timeout, Retry-After total bound, redirects/oversize and error redaction.
- Configuration tests PASS: default/explicit mode, local model, incomplete/malformed settings, URL constraints and budget bounds.
- Real HTTP adapter + storage tests PASS: filtered own context, accepted utterance/actual hearing, crash immediately after NPC effect, DB reopen with no repeated provider call, projection replay, 503/timeout/schema/illegal-response audited silence with no invented effects or secret echoes.
- `npm run build` PASS (typecheck and Vite production build).
- `npm run verify:rp2-provider` PASS: actual backend/browser with local model fixture, 8 calls across the scenario, no extra calls during committed recovery; world recovery counts remain `32:30:8`. Artifacts `/tmp/corerp-rp1-e2e-fsaaVS`.
- `npm run verify:rp1-play` PASS: unchanged deterministic path, 0 model calls, same recovery counts. Artifacts `/tmp/corerp-rp1-e2e-kg9UAl`.
- Final full Go PASS (storage58.441s, HTTP4.921s, decision0.401s); vet PASS; related race PASS (storage68.848s, HTTP12.039s, decision1.441s, server3.057s, CLI5.039s). All existing migration/reopen/replay checks are included; schema unchanged. M0 52 checks, M1 evidence and M2 26 test references PASS. `git diff --check` PASS.
- Live LLM: REQUIRED_IF_AVAILABLE, unavailable configuration; NOT VERIFIED. Local HTTP fixture is not a live-model claim.

## Issues resolved / deferred / recovery

Initial timeout test had an httptest handler that waited indefinitely during cleanup after client timeout; stack inspection proved the client already returned. Added an explicit handler release channel. An outbound-context test incorrectly required `next_schedule` for Cai, who has no future schedule; corrected to require existing own assets/current activity, preserving absence rather than inventing schedule. These were test-fixture assumptions, not loosened world invariants.

Deferred to following RP-2 stages: temperament/values/needs/goals, multidimensional relationships, salient memories, emergent background and narrative style. No RP-3+ code. Restore point: RP-1 `e1aa4c7`; RP-2A checkpoint is the commit containing this report after its final gate. Schema remains025; no migration rollback needed.
