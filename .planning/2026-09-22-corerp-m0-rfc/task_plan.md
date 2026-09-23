# Task Plan: CoreRP M0 RFC → M1 Kernel → Backend Transport → M2 T09 → Agent Life → Unattended Progression

## Goal
Complete the backend milestones in the revised blueprint independently of any eventual UI framework. Preserve the M0/M1/M2 T09/Agent achievements, then advance from bounded Agent life toward an unattended, checkpointed M2 world; do not mistake a small demo for full M2 or backend completion.

## Current Phase
Phase 36 — M2 LOD Economic Implementation

## Active scoped goal — wage-claim consistency closure (2026-09-23)
The attached user goal supersedes Phase 36's broader backlog for this run. Exit only after G1–G5 and I1–I8 are evidenced, current-baseline migration upgrade, recovery/replay, full Go tests, vet, relevant race, synchronized docs/schema and a deferred list; then stop without starting RP or another economy milestone. Route: full cross-module DEFINE → DESIGN → BUILD → POLISH → local SHIP because claim identity, money movement, T09 lineage and bankruptcy snapshots cross persistent authority boundaries. No external release is requested.

- [x] DEFINE: G1 partial wage pay; G2 original-obligation arrears and cumulative cure; G3 retained/queryable cohort/entity claim origins at bankruptcy without liquidation; G4 T09 before/after accrual, partial pay, arrears and cure, dematerialize/rematerialize, same-time and backdated guards; G5 idempotency, crash/reopen, concurrent transitions, event/snapshot recovery, race/vet/full tests. Scope excludes banking, law, RP, UI, scale and unrelated economy work.
- [x] DESIGN: contract-local cumulative worker-round-robin v1 (017), immutable accrual-origin slices/current-owner worker-slot transfers (018), and bankruptcy-opening creditor snapshots (019). The schema-016 upgrade test preserves existing split lineage.
- [x] BUILD 1: due full/partial/zero pay and per-claimant immutable receipts under one aggregate wage obligation; focused conservation, projection and retry checks pass.
- [x] BUILD 2: split wage arrears retries use cumulative allocation to cure or partially cure the original obligation; focused one-cure and two-positive-retry tests pass, including a test-only sourced funding event and idempotent rerun.
- [x] BUILD 3: T09 transfers one unpaid worker slot plus matching receivable per original obligation; verified dematerialization returns wage cash and outstanding claims; rematerialization and later partial/cure pay the current owner while origin slices remain immutable.
- [x] BUILD 4: bankruptcy opening freezes named/Cohort slot creditors in 019, scoped storage read exposes current and opening status, and the old estate route records a typed defer for split claims while unsplit behavior remains unchanged.
- [x] POLISH: G1–G5 matrix and I1–I8 code/test evidence, schema-016→019 upgrade, rollback/reopen/snapshot/concurrency, final full/race/vet checks, synchronized docs and explicit deferred list.
- [x] SHIP: reviewable local result and one final Goal Result report; no external release or next milestone implementation.

Current scoped gate: PASS. G1–G4 implementation and G5 validation are complete: final uncached full Go suite, vet, related race (including post-opening return), migration 016→019, rollback, concurrent T09, empty/snapshot replay and reopen creditor read all pass. `docs/m2/wage-claim-consistency.md` records I1–I8 evidence and deferred boundaries. Local SHIP only; no deployment or next milestone work. This is a fixture-local monetary tie-break, not a law or global creditor ranking.

## Route
Full-project route: preserve completed M0/M1/T09/Agent evidence, extend the M2 branch with explicit recurring world-time actions, bounded unattended progression and recovery, then add the missing LOD/economy/status/scale behaviors required by the original M2 exit. This plan's new phases are increments, not a redefinition of the backend goal.

## Acceptance Criteria
1. A project-local M0 RFC clearly separates frozen, conditional, pending, and deferred semantics and is traceable to the revised blueprint.
2. The RFC contains an executable SQLite DDL draft, transaction algorithm, idempotency/Outbox/recovery rules, canonical hash decision, Go interface sketch, errors, and T01–T12 vectors.
3. Two minimal world package examples exercise the same core schema without `world_id` conditionals.
4. Existing TypeScript demo contracts and fixtures no longer contradict the frozen authority, amount/quantity, Rule Epoch, sequencing, and permission semantics.
5. Project-native build and focused fixture/contract checks pass; documentation-only claims remain labeled as unimplemented where appropriate.
6. README links the RFC package and does not imply that the RFC or backend behavior has been implemented.
7. A project-local Go backend can initialize a real SQLite database from a versioned embedded migration and bootstrap one instance/branch/open Rule Epoch.
8. A purchase command atomically commits contiguous events, balanced postings, stock movement, projections, Branch Head and Outbox under optimistic concurrency.
9. Same-key/same-payload retries return the original result; same-key/different-payload and stale-head commands fail without additional facts or projection changes.
10. Injected pre-commit failure leaves no half-commit; committed state survives closing and reopening the database.
11. Focused Go tests, SQLite invariant checks, existing M0 verification and frontend build pass together.
12. A versioned HTTP API exposes health, commands, simulation, scoped queries, and event consumption without granting handlers direct authority outside storage services.
13. Transport authentication is injected and fail-closed; caller-supplied principal IDs cannot impersonate another principal.
14. JSON success/error envelopes, request limits, timeouts, cursor semantics, and graceful shutdown are stable and integration-tested independently of a frontend.
15. Authorized SSE/reconnect filtering completes the remaining backend portion of T10 without leaking hidden events or sequence gaps.
16. An additive migration defines Cohort aggregate state and materialization lineage without modifying frozen migrations 001–005.
17. Materialization uses one stable `materialization_id` and stable entity ID, atomically transfers population, assets, inventory, receivables, and liabilities from a Cohort to a named NPC, and preserves each conserved total.
18. Same-ID/same-payload retries return the committed result; same-ID/different-payload, insufficient Cohort resources, invalid lineage, and stale head fail without partial authority or projection writes.
19. Dematerialization atomically returns only the materialized allocation to its source Cohort, is idempotent, and cannot double-count or absorb unrelated post-materialization state.
20. Empty replay, snapshot continuation, reopen recovery, injected rollback, and authenticated HTTP commands reproduce the same M2 state; executable T09 evidence is recorded without claiming the rest of M2.
21. An additive migration defines branch-scoped Agent profiles, places, declared schedules, movement facts, current-position projections, observation evidence, and bounded knowledge projections without modifying migrations 001–006 or changing M1/T09 fixture behavior.
22. Two one-person named entities materialized through T09 receive stable profiles and schedules; a bounded M2 runner executes due movement items in `(world_time, phase_id, declared_priority, scheduler_item_id)` order and resumes identically after reopen.
23. Co-location is determined from committed positions at one world time; observations reference the movement event and produce knowledge only for agents actually present, with no cross-agent omniscient read or invented personal history.
24. An authorized encounter query returns current co-located agents and evidence-backed knowledge, returns an ordinary empty result when nobody is present, and performs no authoritative write or random narrative generation.
25. Empty replay, snapshot continuation, projection divergence/repair, injected rollback, authenticated HTTP, and a real-process CLI/server smoke reproduce the bounded Agent state while documentation keeps 30-day scale, LOD breadth, beliefs, subjective status, supply-demand, and LLM decisions explicitly deferred.
26. An explicit, authorized 30-day routine definition yields stable per-day Agent scheduler items without changing the original four-item fixture; retries and rollback cannot create duplicate schedules, facts, or authority.
27. The M2 runner advances the declared 30-day path in stable world-time order with bounded budget, restart/reopen checkpoints, evidence-backed encounters, and no duplicate movement or population/economic asset creation.
28. An opt-in unattended process advances bounded M2 world-time increments without choosing a frontend, keeps server time separate from world time, shuts down cleanly and recovers after restart; the 100-person/10-store economy and full LOD/status exit remain independently testable requirements.

## Phases

### Phase 1: Requirements & Discovery
- [x] Understand user intent from the revised blueprint and prior audit
- [x] Inspect project instructions, repository state, and top-level documentation conventions
- [x] Inspect manifests and core contract types
- [x] Inspect remaining demo data consumers and verification scripts
- [x] Map initial implementation/prototype mismatches to the M0 contract scope
- [x] Document initial constraints and discovery errors in findings.md
- **Status:** complete

### Phase 2: RFC Structure & Decisions
- [x] Define the smallest RFC document set that fits existing project conventions
- [x] Resolve architecture terms against current code and docs
- [x] Record frozen, conditional, pending, and deferred decisions
- **Status:** complete

### Phase 3: Author RFC Package
- [x] Write the M0 core contract RFC
- [x] Add SQLite DDL/schema constraints and Go interface sketches
- [x] Add error catalog, test vectors, and two minimal world package examples
- [x] Update nearby documentation/index references where appropriate
- **Status:** complete

### Phase 4: Verification
- [x] Check internal links, terminology, section structure, and stale conflicting statements
- [x] Validate SQL syntax and representative positive/negative constraints with local SQLite
- [x] Run project-native static, build, and browser checks
- [x] Record all checks as passed, failed, not verified, or not applicable
- **Status:** complete

### Phase 5: Delivery
- [x] Review changed files and preserve unrelated user changes
- [x] Mark planning phases complete and summarize remaining product choices
- [x] Prepare changed files and validation evidence for user handoff
- **Status:** complete

### Phase 6: M1 Define & Environment Proof
- [x] Inspect current filesystem state and Go/SQLite toolchain
- [x] Select the SQLite driver from verified environment evidence
- [x] Freeze the first vertical slice API, package boundaries, and acceptance mapping
- [x] Record full-project Gate A/B evidence
- **Status:** complete

### Phase 7: M1 Skeleton & Migration
- [x] Create a separate backend Go module and package structure
- [x] Embed/version the M0 SQLite migration without duplicating divergent schema semantics
- [x] Implement database open/configure/migrate/bootstrap
- [x] Verify startup and persisted bootstrap state
- **Status:** complete

### Phase 8: Atomic Purchase Vertical Slice
- [x] Implement command/request canonical hash and idempotent claim
- [x] Implement final transaction validation, sequence allocation, Event Batch/Event writes
- [x] Implement balanced journal, stock movement, projection updates, Branch Head CAS, and Outbox
- [x] Expose a minimal executable path and structured result/errors
- **Status:** complete

### Phase 9: M1 Failure & Recovery Verification
- [x] Test same/different payload retries and stale branch head
- [x] Test insufficient funds/stock and unbalanced/unauthorized writes
- [x] Inject a pre-commit failure and prove rollback
- [x] Close/reopen the database and verify persisted authoritative/projection state
- **Status:** complete

### Phase 10: Integration & Handoff
- [x] Run Go tests/race-relevant checks available in the environment
- [x] Re-run M0 static checks and frontend build
- [x] Update README/backend usage and M1 status without overstating the 90-day simulation
- [x] Record remaining M1 work and deliver a reproducible local handoff
- **Status:** complete

### Phase 11: M1 Completion Audit & Next-Increment Definition
- [x] Re-read the architecture source and enumerate the full M1 requirements beyond the completed purchase slice
- [x] Inspect the current backend and tests as authoritative state
- [x] Map every remaining M1 requirement to evidence, missing behavior, and dependency order
- [x] Freeze Gate A/B for the next real vertical slice and extend this plan with implementation phases
- **Status:** complete

### Phase 12: Recovery Foundation — Authoritative Genesis & Migration 002
- [x] Add a versioned M1 migration without changing the frozen M0 migration
- [x] Replace direct-only demo bootstrap state with a traceable genesis Event Batch, journal, and stock fact
- [x] Move purchase expectations/tests to the authoritative post-genesis branch head
- [x] Verify new database startup, repeat bootstrap, and explicit handling of incompatible legacy demo files
- **Status:** complete

### Phase 13: Replay, Snapshots & Projection Recovery (T07)
- [x] Rebuild account/inventory state from authoritative postings and stock movements
- [x] Create and validate snapshot payloads at a sequence high-water mark
- [x] Continue replay from the latest valid snapshot and prove equality with empty-ledger replay
- [x] Detect deliberate projection corruption and rebuild projections without modifying history
- **Status:** complete

### Phase 14: Durable Outbox Delivery Recovery (T06 remainder)
- [x] Implement ordered at-least-once Outbox dispatch and retry bookkeeping
- [x] Prove close/reopen delivery of committed unpublished messages
- [x] Inject a crash after consumer delivery but before publish marking
- [x] Prove consumer dedupe by `outbox_id` prevents duplicate effects while allowing redelivery
- **Status:** complete

### Phase 15: Strict World Schema, Clock & Stable Scheduler
- [x] Add M1 economic contracts, obligations, quote/budget, world clock, and scheduler migration
- [x] Bootstrap one enterprise, three employees, one landlord, and one store with explicit sources
- [x] Implement stable `(world_time, phase_id, declared_priority, scheduler_item_id)` execution
- [x] Prove insertion-order independence, bounded work, restart checkpoints, and no wall-clock tie-breaks
- **Status:** complete

### Phase 16: Wage, Rent, Purchase & Restock 90-Day Settlement (T01–T03)
- [x] Implement idempotent wage accrual, partial/full payment, and arrears on one obligation per period
- [x] Implement rent due/payment/past-due transitions and queryable grace/rejection evidence
- [x] Implement scheduled household purchases, finite-stock pricing, and traceable restock source
- [x] Run the strict world for 90 days and verify budgets, assets/debts, journals, inventory, and deterministic summary
- **Status:** complete

### Phase 17: Controlled Issuance & Minimum Read Authorization (T08/T10)
- [x] Model creator issuance source, capability/scope, limit, intervention record, and balanced accounting
- [x] Prove unauthorized issuance leaves no writes and authorized issuance is fully traceable
- [x] Add player/creator/diagnostic read boundaries for private balances and evidence
- [x] Prove issuance does not mutate market prices without a separately committed pricing rule/result
- **Status:** complete

### Phase 18: Full M1 Regression & Handoff
- [x] Map T01–T08 and T10 to executable evidence and update vector statuses without overstating scope
- [x] Run Go race/vet, SQLite integrity, M0 verification, frontend build, and affected browser regression
- [x] Update architecture/operations docs for recovery, simulation, limitations, and reproducible commands
- [x] Audit the complete M1 exit condition against current artifacts and record remaining risks
- **Status:** complete

### Phase 19: Backend Transport Definition & Contract
- [x] Re-audit the architecture and current service surface for HTTP/SSE requirements
- [x] Freeze versioned routes, authentication boundary, error envelope, limits, and non-goals
- [x] Define handler/service boundaries that preserve storage authority and UI independence
- [x] Record acceptance tests for commands, scoped reads, simulation, health, and shutdown
- **Status:** complete

### Phase 20: Authenticated HTTP Vertical Slice
- [x] Add an injectable fail-closed authenticator and bounded JSON middleware
- [x] Expose health/readiness, purchase, issuance, simulation, state, and private-economy routes
- [x] Map typed core errors to stable HTTP status/error envelopes
- [x] Add real `httptest` integration coverage for success, rejection, idempotency, and persistence
- **Status:** complete

## Backend Transport Gate A — Definition
- **Purpose:** make the completed kernel consumable by any future UI or non-Web client through one fail-closed, versioned HTTP boundary.
- **Primary path:** authenticate a request, validate bounded JSON, invoke a typed kernel service, persist/read SQLite state, and return a stable envelope.
- **Failure cases:** missing/invalid credential, body principal mismatch, unauthorized scope/field, malformed/unknown/trailing/oversized JSON, stale head, idempotency mismatch, unavailable storage, and shutdown cancellation.
- **Non-goals:** choosing a UI framework, production IdP/session management, TLS termination, generated SDKs, CORS policy, public deployment, hot Rule Epoch activation, or external model calls.
- **Gate A:** passed; every item has an observable HTTP/integration assertion in `docs/m1/http-api.md`.

## Backend Transport Gate B — Design
- **Stack:** Go `net/http` and existing SQLite module; no new runtime dependency.
- **Boundary:** `internal/transport/httpapi` depends on a narrow service interface. Storage remains the only authority owner; handlers never receive `*sql.DB`.
- **Identity:** injected Bearer authenticator returns the trusted Principal. Body identity is filled or matched, never trusted independently.
- **Contract:** `/api/v1`, data/error envelopes, 1 MiB strict JSON, typed status mapping, no-store/nosniff headers, and explicit method rejection.
- **Executable:** a separate server command owns flags/env, timeouts, signals, graceful shutdown, bootstrap, and database close. Existing CLI behavior remains available.
- **Increment order:** auth/envelopes → real purchase HTTP slice → issuance/private query → authorized simulation/state → process lifecycle → SSE/cursor in Phase 21.
- **Gate B:** passed; architecture-invalidating identity and transaction/network boundaries are resolved.

### Phase 21: Authorized Event Cursor & SSE (T10 remainder)
- [x] Define visible-event filtering by Principal + Capability + Scope
- [x] Add finite cursor query and SSE resume using authoritative event sequence
- [x] Prove hidden events and sequence gaps do not reveal payloads or existence
- [x] Verify disconnect/reconnect, heartbeat, cancellation, and bounded polling behavior
- **Status:** complete

### Phase 22: Backend Transport Hardening & Handoff
- [x] Add server configuration, timeouts, request/body limits, graceful shutdown, and operational health semantics
- [x] Run focused/race/integration regressions and CLI/server smoke tests
- [x] Publish API examples and clearly separate demo authentication from production identity integration
- [x] Audit backend readiness for an undecided future UI and record remaining backend product scope
- **Status:** complete

### Phase 23: M2 T09 Definition & Data Contract
- [x] Freeze Cohort, allocation, lineage, conservation, idempotency, and failure semantics
- [x] Map the design onto existing Event Batch, ledger, inventory, projection, replay, and authorization boundaries
- [x] Add migration 006 plus byte-identical documentation and define the minimal seeded Cohort fixture
- [x] Pass Gate A/B and migration/bootstrap compatibility checks
- **Status:** complete

## M2 T09 Gate A — Definition
- **Purpose:** prove one aggregate Cohort can become one stable named entity and later return without duplicating or losing population/economic state.
- **Primary path:** bootstrap an isolated M2 Cohort, submit an authorized materialization, verify conserved totals and durable lineage, then submit an authorized dematerialization and verify exact restoration.
- **Failure cases:** materialization ID payload conflict, command idempotency mismatch, stale branch head, insufficient population/asset/inventory/receivable/liability, unauthorized scope, injected pre-commit failure, changed named-entity holdings, and corrupted projection.
- **Non-goals:** Agent reasoning, schedules, encounters, approximate LOD settlement, subjective status, broader market simulation, M3 Rule Epoch activation, UI selection, or production deployment.
- **Gate A:** passed; acceptance criteria 16–20 are observable through migration, storage/replay, HTTP, and regression tests.

## M2 T09 Gate B — Design
- **Authority:** one Event Batch per materialize/dematerialize command; balanced Postings, Stock Movement, Population Movement, lineage, projections, Branch Head, audit, and Outbox commit together.
- **Data ownership:** migration 006 adds Cohort/materialized projections and lineage. Existing account/inventory facts remain the only economic authority; only population gets a new fact type.
- **Compatibility:** M2 uses an explicit isolated demo instance/branch. M1 migrations, genesis, hashes, routes, and exact evidence remain unchanged.
- **Identity/idempotency:** command key and `materialization_id` are independently checked against canonical request hashes; stable entity/resource IDs derive deterministically from the materialization.
- **Dematerialization safety:** reverse only when active named projections exactly equal the recorded allocation; otherwise fail with no writes.
- **Replay:** extend snapshot state with optional population balances, preserving canonical hashes of pre-M2 snapshots when the field is empty.
- **Increment order:** migration/fixture → materialize transaction → reverse/replay → authorized HTTP/evidence.
- **Gate B:** passed; no architecture-invalidating uncertainty remains before migration/bootstrap implementation.

### Phase 24: Atomic Cohort Materialization Vertical Slice
- [x] Add typed materialization command/result and canonical request hashing
- [x] Atomically transfer population and economic allocations into one stable named entity
- [x] Commit lineage, authority event, projections, Branch Head, audit, and Outbox together
- [x] Prove conservation, retry, mismatch, stale-head, insufficient-resource, and rollback behavior
- **Status:** complete

### Phase 25: Safe Dematerialization, Replay & Recovery
- [x] Define and implement eligibility rules that prevent absorbing unrelated named-entity state
- [x] Atomically return the original allocation and close lineage without double counting
- [x] Extend replay/snapshot/projection comparison and repair to Cohort/materialized state
- [x] Prove retry, reopen, crash recovery, and materialize→dematerialize conservation
- **Status:** complete

### Phase 26: Authorized T09 Transport, Evidence & Handoff
- [x] Add scoped materialize/dematerialize HTTP commands without exposing direct database authority
- [x] Map T09 to executable storage and transport evidence while keeping broader M2 deferred
- [x] Run focused, full, vet, race, migration parity, M0/M1 regression, and real-process smoke checks
- [x] Document API usage, remaining M2 scope, and local-versus-production limitations
- **Status:** complete

### Phase 27: M2 Agent Definition & Data Contract
- [x] Re-read the Agent/time/knowledge/encounter requirements and inspect current scheduler, replay, authorization, transport, and T09 boundaries
- [x] Freeze the bounded two-Agent acceptance path, failure cases, authority split, non-goals, and increment order
- [x] Design migration 007 and exact contract mirror without modifying migrations 001–006
- [x] Pass Gate A/B and migration/bootstrap compatibility checks
- **Status:** complete

## M2 Agent Gate A — Definition
- **Purpose:** prove named entities follow declared schedules, meet only through committed co-location, and gain evidence-backed limited knowledge without an LLM or a second world authority.
- **Primary path:** explicitly bootstrap the isolated M2 Cohort, materialize two stable entities, commit Agent/place/schedule setup, run bounded due movements, resolve their co-location, and query each Agent's scoped knowledge and the current encounter.
- **Failure cases:** missing/inactive materialized entity, duplicate/conflicting setup, unauthorized run/read, backward or malformed target time, exhausted budget, invalid schedule/location linkage, stale branch head, injected pre-commit failure, absent co-location, and corrupted position/knowledge projection.
- **Non-goals:** full M2 30-day/100-resident scale, autonomous background daemon, L0/L2 promotion policy, false beliefs, semantic/vector memory, subjective status, economy supply-demand expansion, relationship inference, LLM decisions/narration, UI work, or production deployment.
- **Gate A:** passed; acceptance criteria 21–25 are observable through migration, storage/replay, transport, CLI, and regression tests.

## M2 Agent Gate B — Design
- **Authority:** setup and every due movement commit through one Event Batch; an `agent_movements` fact owns location truth, while co-location observations are immutable evidence linked to that event and `agent_knowledge` is a rebuildable projection.
- **Scheduling:** reuse `scheduler_items` and the frozen stable tuple, but add an M2-specific branch/time runner instead of generalizing the M1 day scheduler. Each item commits independently for bounded restart checkpoints.
- **Data ownership:** migration 007 references active `materialized_entities`; adds profiles, places, one-shot declared schedule entries, position projections, movement facts, observation records, and knowledge projections. No historical table is rebuilt.
- **Encounter semantics:** a scoped query joins current position and evidence-backed knowledge at the durable world clock. It may return no participants and never creates an event or prose.
- **Authorization:** creator can run/read the isolated branch; Agent principals can read only their own knowledge/encounter subject. HTTP binds authenticated identity and storage enforces capability + instance + branch + subject + field scope.
- **Replay compatibility:** optional Agent position/knowledge slices preserve pre-007 snapshot hashes when empty; replay, compare, and repair cover both projections.
- **Increment order:** migration/parity → two-Agent authority-backed setup → stable movement slice → observation/knowledge/encounter → replay/recovery → HTTP/CLI/evidence.
- **Gate B:** passed; the existing M1 scheduler remains untouched and no architecture-changing uncertainty remains.

### Phase 28: Deterministic Agent Setup & Schedule Vertical Slice
- [x] Add migration 007, schema parity, and an explicit idempotent two-Agent demo setup built on T09 materialization
- [x] Commit profiles, places, schedules, initial positions, clock, grants, scheduler items, audit, Outbox, and Branch Head through authority
- [x] Execute due Agent movements with stable ordering, bounded work, checkpoints, and no wall-clock tie-breaks
- [x] Prove setup retry/conflict, ordering independence, budget exhaustion, reopen continuation, and rollback behavior
- **Status:** complete

### Phase 29: Limited Knowledge, Encounter, Replay & Recovery
- [x] Record co-location observations and update only present observers' knowledge in the movement transaction
- [x] Add scoped knowledge and read-only encounter queries, including empty/no-co-location and compatible-observer cases
- [x] Extend replay/snapshots/projection comparison and repair to Agent positions and knowledge
- [x] Prove no omniscient leakage, no query writes, snapshot equality, corruption detection/repair, and injected rollback
- **Status:** complete

### Phase 30: Agent Transport, Evidence & Handoff
- [x] Add authenticated Agent run/knowledge/encounter routes and a reproducible CLI action
- [x] Add executable bounded-Agent evidence while keeping the full M2 exit explicitly incomplete
- [x] Run focused, full, vet, race, migration parity, M0/M1/T09 regression, and real-process smoke checks
- [x] Update API/operations/scope documentation and record the next dependency-ordered M2 slice
- **Status:** complete

### Phase 31: Unattended M2 Definition & Integration Design
- [x] Inspect original M2/LOD/30-day acceptance and current M2 runner/server ownership
- [x] Freeze routine authority, idempotency, pacing, cancellation, checkpoint and failure semantics
- [x] Record observable acceptance and a dependency-ordered integration design without UI assumptions
- **Status:** complete

## Unattended M2 Gate A/B — Scope and design
- **A: observable path:** creator explicitly defines 29 additional days of two-Agent morning/noon schedules on the isolated M2 branch, then the existing bounded runner consumes 120 total movements over 30 world days. Reopen and resume must match uninterrupted replay; no extra population, currency, or stock fact may appear. Repeating definition/run is a no-op for authority. One day of movement is not full M2.
- **A: failures:** missing Agent setup, unauthorized principal, definition retry conflict, clock already past first newly scheduled event, injected transaction failure, partial budget, cancel/restart, and mismatched projection must never duplicate or skip facts.
- **B: authority:** one explicit `AgentRoutineDefined` Event Batch records a deterministic 29-day definition with declared item IDs/times; one transaction inserts future scheduler items and schedule entries linked to the definition event. Per-item execution retains the existing world-time stable tuple and one-event-per-move checkpoint. Old day-one setup and M1/T09 fixtures remain unchanged; no schema change is needed.
- **B: process:** after the routine vertical slice passes, an opt-in driver (never a silent server bootstrap) will call the authorized runner with bounded work and explicit pacing/target. Server wall time cannot become world-time authority. Cancellation closes without inventing a world event; committed items remain restartable.
- **B: remaining risk:** two persons moving for 30 days do not meet the 100-person/10-store economic, LOD or subjective-status requirements. These remain open acceptance gates and require their own future additive contracts.
- **Gate A/B:** passed against blueprint §§5, 12, 22.7, 23 and existing branch/schedule transaction design.

### Phase 32: Authoritative 30-Day Agent Routine
- [x] Define 29 further days of deterministic, event-backed schedules while keeping day-one fixture unchanged
- [x] Verify retries, rollback, stable ordering, bounded runs, reopen, replay and conservation
- [x] Expose a reproducible CLI path and document the bounded 30-day result without claiming full M2
- **Status:** complete

### Phase 33: Opt-In Unattended Execution
- [x] Add independent, bounded world-time driver and lifecycle wiring with explicit configuration
- [x] Prove restart, cancellation, budget, no double-run and separation of server/world clocks
- **Status:** complete

### Phase 34: Regression & Next M2 Dependencies
- [x] Run focused, race-relevant and affected full regression/evidence checks
- [x] Record remaining economy, LOD, subjective status, 100-person/10-store and non-M2 backend gaps
- **Status:** complete

### Phase 35: M2 Background Economic Settlement Definition
- [x] Map the blueprint's causal L0 household/store/wage/rent/stock requirements onto current M1/M2 ledger and scheduler ownership
- [x] Define a real background Cohort economic vertical slice with conserved population/asset/stock/debt, intermediate failure evidence, and bounded replay
- [x] Resolve schema and authority design without copying M1's fixture-specific scheduler or faking a 100-person simulation
- **Status:** complete — definition only; design at `docs/m2/background-economy.md`, no economic settlement implemented

### Phase 36: M2 LOD Economic Implementation
- [ ] Implement the accepted background economic settlement and individualization transition across its real event/account/stock facts
- [ ] Verify restart, bankruptcy/stock/arrears boundaries, rollback, replay, projection repair, and conservation
- **Status:** in_progress — migrations 008–016 and 30-day wage/rent/purchase/consumption/paid-restock plus linked arrears/grace, fixture-local bankruptcy opening, 23 immutable wage claims, post-term one-person claim assignment/return, one donor-funded claimant-aware day-31 partial payout, post-payment residual-claim assignment/return, and a funded one-worker future wage split have local evidence. T09 rejects backdated/skip-due transitions; split partial/late payment and bankruptcy origins, return of already-paid individual claims, generic/repeated payout, liquidation/discharge and full economic scale/status remain open

## Background economy Gate A/B — 2026-09-23

- **A passed (definition):** revised blueprint §§22.7/23 requires ordered, evidence-backed Cohort settlement and a separate 100-person/10-store/30-day exit. The first acceptable implementation slice must post one funded wage, one rent and one stock/budget-controlled purchase or explicit rejection with replay and crash evidence; the spatial routine is not an economy substitute.
- **B passed for the initial slice (design, not implementation):** `docs/m2/background-economy.md` specifies separate branch-scoped counterparties and contracts, event-sourced initial transfers, an additive migration, a branch-wide scheduler, accrual vs payment and finite stock/consumption, fail-closed backdating and transition constraints. This does not decide final world tuning or finish later scale/status gates.
- **Current evidence:** the earliest mixed-phase item is selected globally; wage, rent, purchase, consumption, finite supplier transfer, contract-local arrears/grace and the day-30 insolvency review commit through the shared writer; unsupported/backdated phases fail closed. The 30-day run reaches head 399 with 26 sales/consumptions, one funded 60-unit service and late wage cure, a reasoned blocked-refinement transition, and one open bankruptcy proceeding with 23 immutable Cohort wage claims totalling 4,140. T09 chronology rejects committed-history and pending-boundary backdating. A post-term T09 command atomically moves one population and 23×10 claim/receivable units to a named entity; reverse T09 returns them before payout. Day-31 donor→estate transfer posts 18, then 17/1 reaches Cohort/named claimant (or 18 Cohort-only), clearing the matching receivables and 18 of employer payable with hashed recipient evidence. A subsequent T09 transfers only 229 residual receivable (9 + 22×10), and returns it if its entity has not itself been paid. Mid-term employment split, paid-entity residual return, generic liquidation/discharge and full scale remain open.
- **Contract-safe refinement subgate A/B passed:** §27.1 requires named-entity debt/receivable allocation at the same authority boundary as population transfer, while §25.2 forbids implicit universal creditor order. The aggregate bankruptcy row currently lacks per-obligation claimant provenance. First increment: migration 013 adds an immutable, unranked, event-linked claim snapshot at proceeding opening; verify every claim and conserved total, rollback/reopen/idempotence, then use those claims as inputs to later atomic contract split. Keep T09's active-contract guard until split and payout routing exist; this increment is prerequisite evidence, not a claim that refinement is executable.
- **Post-term claim-allocation subgate A/B passed:** Declare the existing 30-day wage contract's fixed end at day 30 07:12 in its definition event; never infer termination from bankruptcy. After all due work is settled, T09 may atomically materialize one person and assign an exact conserved share of each open wage claim with 230 of the existing receivable. Keep the one-household rent contract only while Cohort population stays positive; reject materialization before wage expiry, non-divisible/unproven claims, and post-opening wage retry until payout routing exists. Dematerialization must atomically return assignments if no assigned debt was paid. Migration 014 stores immutable assignment and return lineage; tests must prove replay, rollback, idempotence and total claims/receivable/population conservation. Mid-term active employment split and post-assignment payout remain open.
- **Claimant-aware payout subgate A/B passed:** Predeclare a fixture-local 18-unit no-recourse landlord contribution to the bankrupt employer's estate at day 31 07:00 and one 07:01 wage-claim distribution of one unit per original worker to the oldest unpaid obligation. This uses only posted landlord cash and explicitly avoids a global creditor ranking; it creates no new landlord claim. Migration 015 will hold the policy and typed contribution/distribution/recipient facts. The shared writer must atomically post donor→estate cash and then estate→actual Cohort/entity claimants, reduce the employer payable/original obligation and each recipient receivable, and preserve original claim snapshots. Verify no materialization and one materialization, zero-funds no-action, restart/rollback, idempotence, replay and conservation. Generic liquidation, repeated partial payouts and mid-term employment splitting remain open.
- **Claimant-aware payout implementation gate:** passed for the fixture-local rule. Migration 015, both scheduled writers, recipient hash/lineage verification and docs are in place. Focused tests prove Cohort-only versus named routing, zero-donor-cash deferral from a posted test transfer, rollback at both items, reopen, no duplicate run, replay/snapshot/projection agreement, and forged-receipt rejection. Full backend tests, vet and targeted estate/T09 race passed. This gate does not cover generic priority/liquidation, repeated payout, or post-payment T09 residual assignment.
- **Post-payment residual-claim subgate A/B passed:** §27.1 requires materialization to move only owned, existing receivables, and the prior day-31 payment changes the Cohort's share of the oldest wage claim. Use verified paid-fact and typed recipient receipts to calculate `opening outstanding - active assignments - Cohort receipts`, divide only by current Cohort population without rounding, and require the command's receivable transfer to equal the complete residual share. A materialization after payment must receive 9 on day-8 plus 10 on each other open claim (229 total); no cash or past labor history is silently assigned. An assignment made after payout may return while its entity has received no later distribution; one paid to that entity must remain protected. Tests: no prior assignment and one prior assignment; conservation, wrong-share rejection, return, restart/replay/rollback and unchanged day-30 baseline. Active mid-term wage split remains a separate unresolved gate.
- **Post-payment residual-claim implementation gate:** passed for the existing fixture. Focused two-branch T09 tests, full backend tests, vet and targeted storage race (243.839 s) passed for the no-schema calculation. This does not satisfy the active mid-term employment-contract split or arbitrary estate-payment policy.
- **Active-wage split Gate A/B passed for a first vertical slice:** Permit T09 after a fully settled wage boundary and before the next accrual to atomically assign one future worker-participation right under the unchanged 18×10 contract, with a new income account and event-linked typed assignment. Next due accrual and payment must conserve 180 while routing 170 to Cohort and 10 to the named entity, with no duplicate personal history, sourced cash, checked projections, rollback/reopen/idempotence/replay and negative chronology/underfunding checks. A remaining aggregate wage obligation is not silently reassigned; until split-aware partial arrears and bankruptcy claim origins are implemented, those later cases fail closed. This is an explicit dependency increment, not a substitute for the full 30-day split requirement.
- **Active-wage split implementation gate:** passed for the funded vertical slice. Focused tests cover an offset-formatted T09 timestamp, injected materialization/payment rollback, reopen and command replay, 170/10 accrual and funded payment, projection comparison, rejection of a second split while wage debt is open, day-7 underfunded payment fail-closed, and forged receipt rejection. The existing 30-day baseline remains separate and unchanged. Full backend `go test ./... -count=1`, `go vet ./...`, and targeted split/migration `go test -race` passed after the final code edits. The next dependency is claimant-aware partial payment/arrears/insolvency so the split branch can continue past day 7; this gate does not mark full Phase 36 complete.

### Phase 37: M2 Perception and Scale Gate
- [ ] Add evidence-backed, observer-scoped status perceptions and a bounded behavioral feedback path without exposing hidden finances
- [ ] Exercise unattended 100-person/10-store/30-day economic and encounter scale, including recovery and authorization
- **Status:** pending

### Phase 38: Backend Milestone Audit
- [ ] Audit original M2 requirements, independent-world-pack compatibility, and remaining backend M3/M4/production integration requirements; retain incomplete items as open work
- **Status:** pending

## Decisions Made
| Decision | Rationale |
|----------|-----------|
| Work in `/home/ubuntu/corerp-console` | Local search identified it as the actual CoreRP project. |
| Treat the revised `/tmp/...md` as the architecture source | It contains the user-approved 2026-09-22 P0 revisions from the previous turn. |
| M0: produce documentation/contracts, not runtime implementation | The blueprint explicitly defines M0 as contract/schema/test-vector work and M1 as the first executable kernel. |
| M0: use the local-engineering route | RFC authoring plus contract/fixture alignment affected docs, types, data, and checks before the later M1 scope upgrade. |
| RFC package = index, core RFC, SQLite DDL, JSON Schema, machine-readable test vectors, and two world examples | This is the smallest set that directly satisfies the blueprint's named M0 outputs while keeping normative artifacts separate from the demo. |
| Canonical hash format = `sha256:<lowercase hex>` over RFC 8785 canonical JSON | Gives cross-language stable hashes and avoids map-order dependence; test vectors will pin actual bytes/hashes. |
| Event sequence advances only for authoritative events | Audit `record_order` remains a separate Inspector concern and cannot affect branch version or Rule Epoch ranges. |
| Demo epoch 0 is `[1,13)` and epoch 1 begins at event sequence 13 | The activation event is authoritative event 12 and belongs to epoch 0; the next committed event is sequence 13 under epoch 1. |
| Put the Go module under `backend/` | Keeps the existing Vue demo and package manager isolated from the M1 service while allowing the RFC/schema to remain project-shared. |
| First M1 slice is a purchase command across real SQLite persistence | It crosses command, validation, event ledger, accounting, stock, projections, optimistic concurrency, Outbox, and reopen recovery—the riskiest shared kernel path. |
| M1 package boundary = `cmd/corerp-m1` + `internal/core` + `internal/storage` | Keeps the executable thin, typed domain/hash rules independent, and all SQLite transaction ownership in one package. |
| M1 is a CLI-backed kernel slice, not an HTTP service | A local executable proves the architecture path without prematurely freezing an external transport API. |
| Keep the M0 DDL byte-identical as embedded migration `001_init.sql` | The checked RFC artifact stays the schema source; an automated parity test prevents silent migration drift. |
| Make T09 the first M2 slice | It is the smallest architecture-critical M2 path and proves atomic conservation/replay before Agent or scale breadth. |
| Keep Agent scheduling separate from the M1 executor while reusing `scheduler_items` | The M1 implementation is fixture-specific and fully evidenced; a branch/time-scoped M2 runner preserves that baseline while retaining the frozen stable ordering contract. |
| Derive encounters as authorized reads over committed position/observation state | The blueprint requires state-based encounters and permits uneventful results; a query must not become another fact-writing or random-story path. |
| Store observations as immutable evidence and knowledge as a replayable projection | This distinguishes world truth from who learned it, supports limited knowledge, and allows corruption detection/repair without granting omniscience. |

## M1 Gate A — Definition
- **Purpose:** prove one real purchase command can cross validation, authoritative facts, projections, optimistic concurrency, idempotency, Outbox, rollback, and reopen recovery in SQLite.
- **Primary user path:** initialize a local database, bootstrap one instance/branch/open Rule Epoch, submit a purchase, receive the committed Event Batch range, and inspect the durable result.
- **Non-goals:** HTTP/authentication transport, LLM behavior, scheduler simulation, 90-day world content, Rule Epoch hot activation, multi-process scaling, and production deployment.
- **Acceptance mapping:** criteria 7–10 cover bootstrap, commit, retries/conflicts, rollback, and reopen; criterion 11 is the release regression gate.

## M1 Gate B — Design
- **Executable:** `backend/cmd/corerp-m1` provides a minimal database-backed demo command and JSON result.
- **Domain boundary:** `backend/internal/core` owns typed purchase input/result, stable request/event hashes, and classified errors.
- **Persistence boundary:** `backend/internal/storage` exclusively owns SQLite open/configuration, migration/bootstrap, transaction orchestration, and readback helpers.
- **Migration:** `backend/internal/storage/migrations/001_init.sql` is embedded and guarded by a test against `docs/m0/schema.sql`.
- **Transaction boundary:** one pinned connection executes `BEGIN IMMEDIATE`; all command, attempt, batch/event, journal/postings, stock, projection, Branch Head, and Outbox writes commit or roll back together.
- **Failure seams:** typed validation/conflict errors plus a test-only before-commit hook make negative paths and rollback observable without weakening production gates.

## M1 Recovery Gate A — Definition
- **Purpose:** make the existing kernel recoverable from authoritative facts and complete both crash sides of T06 plus T07 before adding 90-day scheduler breadth.
- **Primary path:** initialize traceable genesis facts, commit a purchase, close/reopen, rebuild state from sequence 0 or a validated snapshot, compare/repair projections, and deliver durable Outbox messages.
- **Failure cases:** corrupted snapshot hash, corrupted projection, publisher failure, crash after consumer receipt but before Outbox marking, duplicate delivery, and legacy demo state with no authoritative genesis.
- **Non-goals:** HTTP/SSE transport, production message brokers, Rule Epoch hot activation, LLM/Agent behavior, and 90-day contract settlement in this increment.

## M1 Recovery Gate B — Design
- **Migration:** preserve byte-identical M0 migration 001; add documented migration 002 for durable snapshot payloads and explicit schema-version progression.
- **Genesis:** sequence 1 is a system `WorldInitialized` event with balanced opening-money postings and a typed inventory creation movement; projections become derivable rather than unexplained seeds.
- **Replay:** fold posted postings and stock movements in event-sequence order using checked integer arithmetic; snapshot payloads are canonical JSON with verified hashes.
- **Projection recovery:** compare replayed state to current projections, report exact differences, and repair only projection tables in a pinned write transaction while leaving events/facts immutable.
- **Outbox:** dispatch unpublished rows in event order, record attempts after failure/success, permit redelivery after the publish/mark crash window, and require consumer effects to dedupe by `outbox_id`.
- **Increment order:** migration/genesis → empty replay → snapshot continuation/projection repair → Outbox crash recovery → full regression.

## Errors Encountered
| Error | Resolution |
|-------|------------|
| Initial `vexor` call expanded unquoted glob exclusions | Retried with quoted exclusions. |
| `vexor` indexing failed with an OpenAI API connection error | Fell back to local `rg` discovery as allowed by the skill. |
| M2 semantic discovery again failed with the same provider connection error | Recorded the repeated external failure and switched directly to scoped `rg`/file inspection for T09. |
| Agent-slice semantic discovery failed with the same provider connection error | Recorded the failure and switched to scoped `rg`/direct inspection; no semantic output informed the design. |
| First M2 planning patch duplicated update blocks for `task_plan.md` | The patch was rejected atomically; combine all hunks for that file into one update block. |
| Broad `/tmp` search emitted permission warnings for system-private directories | Search still found the project; future searches stay scoped to the project and explicit source file. |
| Login shell reports missing `/home/ubuntu/.cargo/env` | Harmless environment warning; do not modify user shell configuration for this task. |
| Initial Markdown link checker treated the README's literal security example `[x](javascript:…)` as a local file link | Adjust the validation rule to ignore non-file URI schemes and rerun. |
| Python `jsonschema` and Node `ajv` are unavailable | Do not install dependencies; record formal meta-schema validation as NOT VERIFIED. JSON parsing, custom schema invariants, manifest shape/hash checks, and examples still pass. |
| `go` is not available in the current PATH | Use `/usr/local/go/bin/go` explicitly; Go 1.23.4 is installed and the sqlite driver is already cached. |
