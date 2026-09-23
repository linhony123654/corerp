# RP-1 findings

## Scope
Source goal: `/tmp/f1edf80b-3702-4376-9e97-85d6670a9109.md`. Contracts C1–C6 and acceptance RP-1A–F are authoritative for this task; current source and tests decide what is Existing versus Proposed.

## Initial evidence
- Project root: `/home/ubuntu/corerp-console`.
- Git repository exists on `main` with no commits (initialized in preceding user request). Existing history analysis is unavailable.
- Separate backend Go module and Vue/TypeScript frontend; previous M2 planning is complete only for its scoped wage-claim closure, not RP.
- No project-local `AGENTS.md` found.
- `vexor` provider connection failed; scoped `rg` is the discovery fallback. A newly initialized repository has branch `main` but no resolvable `HEAD`.
- v0.5 draft is `/tmp/872a0bfc-d74c-4cfc-86c2-676d7a88e08c.md`; RP-0 report is `/tmp/corerp-rp0.9EFoQi/rp0-report.md` (not in project Git).
- Pre-business-change source archive: `/home/ubuntu/corerp-rp1-baseline.RC6ULh/source-before-rp1.tar.gz`; SHA256 `a54e813b32b98de4fb7269c439a560105582c4b9531fe993f9811074e99c7c90`, 212 archive entries, excluding `.git`, dependencies, build outputs, runtime DB, logs and env files.

## Open questions to resolve from source
- Location and movement authority, scheduler API, observation/knowledge projection, principal authorization, and HTTP service boundary.
- RP-0 report and v0.5 location.
- Minimal safe migration and branch/world/entity binding semantics.

## Baseline architecture discoveries
- RP-0 confirms no existing RPSession/RPTurn/Speech/DecisionProvider or live Play wiring. This goal now approves C1–C6, but other v0.5 proposals are not automatically existing behavior.
- `BootstrapM2AgentDemo` materializes Ada and Bo from a Cohort, then seeds five `agent_places`, profiles, positions, four schedules and scoped grants in one existing world/branch. The minimal RP slice still needs player and third NPC; reuse T09 lineage.
- `ResolveEncounter` reads `agent_positions` and excludes a different-location participant. `agent_profiles` references `materialized_entities`; a player spatial profile can be separated from AI decision eligibility without a second location table.
- Migration 007 `agent_movements` currently accepts only `initialize`/`scheduled` and scheduled rows require a declared schedule; RP direct movement must preserve this authority/replay contract, not silently update only `agent_positions`.
- HTTP uses injectable authentication and storage-backed service interface. M2 player/agent identity differs from M1 buyer; creator grants must never backstop Play.
- v0.5 RP-T01–T26 are Proposed test matrix. The current goal's explicit RP-1A–F and DoD govern execution; no real LLM infrastructure is required when unavailable, but deterministic provider E2E must be honest about that limitation.

## RP-1A implementation decisions
- Migration 020 adds only app-session metadata; `world.rp.control` grant is the player/entity authority, and `agent_positions` remains the sole location projection. A session read never stores or returns a copied character sheet, location or world time.
- The opt-in RP demo extends the same M2 branch from head 4 to 7: Cai and Lin are independently materialized from the Cohort, then one setup event gives them existing cafe positions, player/agent principals and a player-only control grant. Existing Ada/Bo and five places are retained.
- Observation is serialized in a short `BEGIN IMMEDIATE` transaction with cursor advance so position/time/version match; HTTP body identity is bound to the authenticated Principal. Only identity, place and co-located names/IDs are returned.
- The existing replay logic reads `agent_movements` generically; the RP setup's two initialization movements compare equal to projections and snapshot continuation. A later RP move writer must preserve this lineage rather than writing only the position projection.
