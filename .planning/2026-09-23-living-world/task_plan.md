# CoreRP living-world goal

## Objective and authority
Implement the full goal `/tmp/0d1b0723-9876-4319-912d-c780b0c66aea.md`: strictly serial RP-2 → RP-3 → RP-4 → RP-5 → RP-6 → RP-7 → RP-8 → Final Integration. Do not redefine completion around a subset. Stage commits are explicitly authorized. Current baseline `e1aa4c7`, clean main, schema 025. Prior RP-1 plan/report are historical.

## Route and invariants
Full-project DEFINE/DESIGN/BUILD/POLISH/local SHIP per stage. One large stage in implementation at a time. Each stage: read-only recon → reuse map → representative vertical slice → focused tests → full scope → required verification → report/commit → clean tree → next stage. No second character/economy/location/time/relationship/memory/history authority; validated Event chain owns facts. Models get only legally known context. No production or external publication authorization.

## Current phase
RP-2C local SHIP: scope and full gates passed, phase report complete; authorized checkpoint next, followed by RP-2D read-only recon. No RP-2D+ implementation.

## Milestones
- [x] RP-2A: real provider adapter, strict structured output/schema, timeout/retry/fallback, illegal rejection, configuration, targeted real HTTP/fake model tests, full gate and checkpoint.
- [x] RP-2B: audited temperament/values; derived Needs, source-backed Goals, multidimensional relationships; Life Context; economic/schedule/relationship-memory decision chains.
- [x] RP-2C: minimal materialization and evidence-consistent stable background; Cohort→NPC→RP→re-encounter.
- [ ] RP-2D: versioned scoped StyleProfile (default/world/session/scene/turn); decision/narrative separation and fact invariance.
- [ ] RP-2E: 50–100 turns, 3–5 NPC/places, work unit, materialized NPC, economic/work/relationship chains, initiative, quiet day, restart; 3 same-initial-world divergent runs with RNG/provider/state attribution. RP-2 gate.
- [ ] RP-3: recruitment and all employment lifecycle transitions; career↔RP chains; verify/report/checkpoint.
- [ ] RP-4: layered culture; institutions/law lifecycle, knowledge propagation, evaluation/rebellion/violation consequences; verify/report/checkpoint.
- [ ] RP-5: sourced probability/cooldown opportunities, non-directorial event pressure, HOT/WARM/COLD/COHORT LOD, long-run substantive divergence; verify/report/checkpoint.
- [ ] RP-6: immersive world-native information, narrative presets/custom/stream/regenerate without rollback, 100+ turn multiday Play/restart; verify/report/checkpoint.
- [ ] RP-7: stable protocol, thin SillyTavern adapter, MCP/Skill, same-world identity across clients; verify/report/checkpoint.
- [ ] RP-8: optional real Studio/Inspector/Creator workflow with player/creator/ops permission separation; preserve extension registry; verify/report/checkpoint.
- [ ] Final Integration: 5–10 NPC, Cohorts and all life/society systems, 300 turns / 30 days, multiple restarts/clients, emergent identity/economic/career/culture/law/relationship/quiet/rare-event chains; full DoD item audit and final report.

## Verification policy
Each completed stage: full uncached Go, vet, related race, applicable migrations/reopen/replay, frontend typecheck/build/configured verifiers, targeted E2E, browser recovery if Play changes. Live model REQUIRED_IF_AVAILABLE if no configured endpoint/key/model; implement real adapter regardless. Mock HTTP tests prove transport/contract and world boundary, not remote live quality.

## Decisions
- RP-2C slice: keep existing population/economic MaterializeCohort command; follow with an atomic BackgroundMaterialization command for a fresh one-person entity. Reuse names/materialization lineage, validate existing home/initial/routine places, store only missing age-range/residence/routine facts in an immutable Event, initialize existing Agent profile/position/scheduler, and supply own background to permitted decision context. No generated lifetime biography or inferred family. Exact retry is stable; conflicting redefinition fails. Existing M2 scheduler is instance-bound, so initial delivery explicitly rejects unsupported worlds rather than creating an inert queue; general scheduler scope is separate follow-up within the goal when needed.
- RP-2B recon: no existing temperament/relationship/episodic-memory authority. Use existing immutable materialization lineage for a versioned minimal temperament seed; existing own accounts/contract participation, schedule and observed Knowledge supply Life Context. Do not invent employment/rent for an NPC without a matching contract. Memory is a selected view of actual observations, not an extra truth store. Relationship evolution must be grounded in accepted interpersonal actions/observations; generic dialogue text must not fabricate fulfilled promises.
- RP-2B representative slice acceptance: same chat + changed own obligation/asset pressure yields sourced Need/Goal and a different choice; schedule constraints persist; personal-state fields remain absent from public Observation. Full B still requires multidimensional relations, sourced memories/commitments and actual-world integration tests, not just a derived DTO.
- Preserve prior baseline verification evidence; rerun affected checks after implementation. No business edits before recon/design.
- Provider credentials configured only by server operator, never player requests/browser; no search of unrelated credential stores or use of real accounts without clear target.
- Existing `RPDecisionInput`/`RPDecisionProposal` and `RunRPTurn(provider)` are reusable. Current HTTP product wrappers hardcode deterministic; supply an immutable service-level provider wrapper rather than mutable per-request global state.

## Gates
DEFINE PASS: requested phase order/scope/DoD captured, original goal remains full acceptance reference. RP-2A/B/C DESIGN/BUILD/POLISH/local SHIP PASS (live model REQUIRED_IF_AVAILABLE, unconfigured; local HTTP adapter verified). RP-2B checkpoint `d694788`; C checkpoint next. All later implementation gates pending.
