# RP-2C — Emergent character and minimal stable background

Status: PASS locally, 2026-09-23. Baseline `d694788`, schema025 unchanged. RP-2D and later remain pending.

## Recon and implementation

Existing Cohort materialization already conserves population, cash, inventory and claims, including optional named wage participation. It does not initialize an RP-capable Agent. Existing demo bootstraps depend on fixed identities/heads. Reused the conserved command, existing Agent/profile/position/scheduler tables and their immutable evidence, own Life Context and actual employment contracts. No character-card authority or new migration.

`MaterializeRPBackground` is an authenticated, source-Cohort-grant-checked second command after conserved materialization. It checks existing identity/places/routes and existing profile/background before defining missing minimal age range, residence and routine. Names and temperament retain identity/materialization lineage. Work entries require the person's effective contract and a real work place; the new Event defines that person's appointment, not a new employer/job or building ownership. Organization membership comes from actual employment; absent family evidence remains absent. Current Needs/Goals remain derived from life state, not saved personality bars.

The command atomically commits one immutable Event, normal command/attempt/batch/audit, and existing Agent/position/movement/scheduler definitions. Exact retry returns the original facts; conflicting redefinition fails. A crash between conserved materialization and background initialization leaves a valid named entity and can safely resume the second command. Neither an NPC card nor fresh population allocation is needed on re-encounter.

## Acceptance evidence

- Real Cohort → Nora → background → RP: storage and actual HTTP/browser use real materialization and initialization, then existing speech/decision/commit.
- Routine → departure → re-encounter: scheduler moves the same NPC home and back; interaction memories remain sourced observations.
- Stable identity/background: exact retry returns identical facts after process/browser restart; storage test additionally rebuilds projections and compares full Life Context.
- Evidence precedence/security: existing profile redefinition, conflicting key, invalid home and unauthorized player requests fail; precommit failure leaves no partial profile or head change. Background stays out of public scene Observation and unfiltered Outbox.
- Actual employment: named worker inherits the genuine wage split, creates a sourced work appointment, moves on world day2, retains actual organizational relationship and replay consistency. Invalid contract cannot invent employment. Only route topology is injected in this supplemental fixture; core employment/economics run through real commands.
- Calendar fix: generated schedule Day now follows the existing bounded-world calendar instead of always0; regression verifies actual day2 after execution.

## Verification

- Focused core/storage/HTTP PASS (0.002s/1.784s/0.373s), including employment/calendar integration.
- Current-code typecheck/build and actual browser emergent chain PASS (`/tmp/corerp-rp1-e2e-tJgWaI`). Provider fixture browser regression PASS (`/tmp/corerp-rp1-e2e-BFImKW`, 8 HTTP model-fixture calls, unchanged recovery counts `32:30:8`).
- Full Go PASS (storage66.943s, HTTP6.724s); vet PASS. Relevant race PASS (core1.014s, decision1.449s, storage89.184s, HTTP18.445s, associated server configuration1.011s/CLI RP3.618s). Full package run includes migration/reopen/replay tests; schema unchanged. M0 52 checks, M1 evidence and M2 26 test references PASS. `git diff --check` PASS.
- Live LLM: REQUIRED_IF_AVAILABLE / NOT VERIFIED; no configured credentials. No external accounts or deployment.

## Limits and next stage

Initialization is deliberately minimal; age range refers to the initialization time, no unseen lifetime biography or invented family. Existing scheduler scope remains explicitly M2-only, not a claim of arbitrary-world support. Broader world creation belongs to later workflow integration. Jobs still use existing employment/wage participation; recruitment and lifecycle remain RP-3. Initial work evidence is historical; it does not replace current contracts.

Before RP-2E, existing fixed-head RP bootstrap and the economy setup's 18-person prerequisite must be composed explicitly; this is recorded in the plan, not bypassed with fake balances. Next after this checkpoint is RP-2D style separation. Recovery point is `d694788`; C checkpoint hash will be reported after all checks pass. No migration rollback is needed.
