# F0 — old-goal handoff and source baseline

Status: F0 baseline audit PASS (local only). Old RP2→RP8→Final goal is complete,
not merely RP8 complete. This report records the entry state before any F1
implementation; it does not claim that the new F1–F13 goal is complete.

## Entry and recovery point

- Repository `/home/ubuntu/corerp-console`, branch `main`, entry HEAD
  `3a87f286c52fff8cf097b53dd1aea44039032791`; worktree was clean on
  entry. Final source is `130a959c3006d37d81ab4c0fb29a92b5a1136690`;
  `3a87f28` records completion metadata only. RP8 source was `86ca6f9`.
- Current schema is 030 (`StudioReadySchemaVersion` in
  `backend/internal/storage/store.go`). Embedded migrations 001–030 are
  forward-applied by `Store.migrate`; no downgrade is supplied. Git history
  and a matching SQLite backup are both needed to restore an older binary;
  never open a schema030 world blindly with it.
- Prior [RP8 report](../rp8/phase-report.md) explicitly says RP8 was complete
  while Final was still outstanding. The [old Final report](../final/phase-report.md)
  and [audit](../final/requirements-audit.md) explicitly close that Final
  requirement. The old plan's `Current phase` is `complete`; no old Final
  runner, Go test or Runtime process remains. A Vite preview process launched
  on Sep 22 predates the old goal and was left untouched.
- Existing temporary evidence was read, not regenerated: `result.json` says
  `LONG_CLIENT_PASS`, 308 settled Play+MCP turns, 32.477083 world days, four
  service/browser/MCP restarts; `audit.log` says same-original-DB projection
  audit/rebuild PASS23.373s and unchanged all-Event digest
  `sha256:06c02158306b8bb072c8a6cc6502bc87c2bb1f30a489460723b3d66897423949`.
  Direct parsing of the retained full Go test JSONL found 406 top-level runs,
  406 passes, zero failures/skips and zero package failures. Prior relevant
  race, vet, frontend build and RP7 three-client evidence are documented in
  [Final](../final/phase-report.md) and [RP7](../rp7/verification.md); F0 did
  not rerun their expensive suites. Temporary evidence may later expire.
- This local environment exposes no `CORERP_*` provider variable names. It has
  Node v22.23.2, npm10.9.8, Go1.23.4 at `/usr/local/go/bin/go`; bare `go` is
  not on the present shell PATH. Live model language quality was **not**
  verified by the previous goal and is not silently promoted to PASS here.

## Reuse matrix

`VERIFIED_REUSE` means the existing owner is suitable as a foundation, not
that the new stage's additional behavior is already delivered.

| Capability | Verdict | Source-backed boundary / F-stage consequence |
| --- | --- | --- |
| RPSession / Turn / recovery | VERIFIED_REUSE | `rp_session.go`, `rp_turn.go` keep scope/cursor/original keyed requests and resumable settlement. F1 must pin any parsed multi-step plan to the same retry identity. |
| Speech / listener Knowledge | VERIFIED_REUSE | `rp_speech.go` atomically records accepted utterance and co-located observation/Knowledge; F1/F2/F7 must extend lawful perception, never infer hearing from narration. |
| DecisionProvider / NarrativeProvider | VERIFIED_REUSE | `rp_service.go`, `decision/`, `narrative/`, `core/rp_narrative_plan.go` separate model decisions from bounded presentation. Current planner produces closed style parameters, not unrestricted long prose. |
| StyleProfile / streaming / regenerate | THIN_ADAPTATION | `core/rp_style.go`, `rp_style.go`, `rp_narrative_*`, Play stream/regenerate exist, with pinned styles. No explicit concise/standard/long density or verified 1000+ character prose path yet (F1). |
| Spatial / route / obstruction / travel delay | THIN_ADAPTATION | `rp_routes.go`, `rp_move.go`, `rp_transit_*`, `studio_spatial.go` provide links, sourced roadworks and appointment delay. `core/rp_transit.go` explicitly states edges have no travel duration; player movement is immediate. No midway encounter or lazy location materialization (F2). |
| Scheduler / Wait | VERIFIED_REUSE | `scheduler.go`, `rp_wait.go` advance one scoped clock and pin retry intent/budget; F1–F3 reuse them, including lawful pause boundaries. |
| Economy / Career | VERIFIED_REUSE | Existing journal/inventory/economic actors and `career_*` implement finite wages, obligations, hiring and work. F4–F8 must attach to these owners, not copy money or roles. |
| Culture / Law / Institution | VERIFIED_REUSE | `rp_culture*`, `rp_institution*`, `rp_law*` implement sourced stance, transmission, effective rules, roles and enforcement; new information/organization decisions must preserve heard-vs-unheard and authority checks. |
| Relationship / Memory / Belief | THIN_ADAPTATION | `core/rp_life.go`, `rp_life.go`, `rp_social.go` derive sourced needs, relationships and memories; cultural stance exists. No general belief-claim lifecycle was identified; F4/F7 must distinguish belief from world truth. |
| Client Protocol / SillyTavern / MCP | THIN_ADAPTATION | RP7 protocol plus Play, SillyTavern0.1.0 (minimum host1.19.0) and MCP0.1.0 (`@modelcontextprotocol/server`2.1.0) were exercised together. `clients/mcp/server.js` is an authenticated stateless bridge; it does not provide persistent model-resident controller arbitration (F3) or new F1 intent methods. |
| Studio / Inspector | THIN_ADAPTATION | `studio_*`, `src/components/Studio*`, `src/main.ts` provide optional creator/authorized historical evidence views. `studio_explanation.go` reports recorded evidence, not omniscient motives; F10 QA/Observer is not yet supplied. |
| Extension Registry / Manifest / capabilities | THIN_ADAPTATION | `core/studio_package.go`, `studio_packages.go`, `studio_activation.go` validate/pin immutable declarative System daily-budget and Narrative style bundles with manifests/dependencies. No executable sandbox, generalized pack lifecycle or required Retail/Journal examples (F9). |
| Backup / replay / migration / race | THIN_ADAPTATION | `replay.go` Compare/Rebuild, embedded migrations 001–030, existing race tests and original-world audit are verified. A consistent pre-audit SQLite backup was made, but no general in-product backup/restore workflow was identified; F12 must re-evidence recovery on the new schema. |

## Real gaps and next stage

- F1: independent interaction mode, narrative density and world-advance controls;
  durable parsed intent/mixed ordering, pause semantics and long-form output.
  Current Play has speech textarea plus separate move/wait/social controls and
  short deterministic narration, not a unified text-intent engine.
- F2/F3: timed travel, encounter/perception and lazy place materialization are
  absent; MCP currently invokes HTTP tools but cannot claim an NPC with an
  exclusive, durable controller lease. Shared world clock already exists.
- F4–F8: legacy `household_budgets` and cohort food/rent are aggregate economic
  constructs, not person-level household membership/care. Career qualification
  assessment is private employer judgment, not an independent skill credential.
  Career/invitation read receipts exist, not the requested general delivery
  channel. Health and organization-agent loops need their own source-backed
  contracts. These are stage targets, not F0 failures.
- F9–F11: bounded declarative package installation and Studio/Inspector are
  foundations, not completed author lifecycle, World QA/Observer, or the static
  public manual.
- No source or data was changed by this audit. Begin F1 only after this report,
  F0 checkpoint and clean-tree verification. No production release, push,
  account use, or live-provider claim is authorized by F0.
