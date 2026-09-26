# F10 local phase report — World QA & Observer

Status: **local acceptance PASS**. Baseline is F9 checkpoint `8e9da2d8a4e8d35688a24cbdb5026a7a13d7ca54`. The containing scoped commit is the F10 checkpoint; verify its clean worktree before entering F11. No push or release.

## Implemented and demonstrated

- **World QA (Read-Only Simulation Health Diagnostics)**:
  - Strict read-only semantics: queries committed immutable events, balances, and projections; emits zero new events and writes zero tables. It never forms a second source of world truth.
  - Computes 14 mandatory diagnostic categories:
    1. **Population & Cohort Materialization**: Total materialized entities, active agents, population movement count, and cohort breakdown.
    2. **Employment & Vacancies**: Active contracts, ended contracts, total positions, unfilled postings, unemployed count, and unemployment ratio.
    3. **Financial Flows & Arrears**: Total accounts, balances by currency, wage obligations (accrued, paid, arrears), rent obligations (due, paid, past-due), arrears cases count, and bankruptcy count.
    4. **Household Rent Pressure**: Active households, total members, dependents count, covered households, strained households, and critical households.
    5. **Housing Coverage**: Total residences, housed population, unhoused population, and housing coverage ratio.
    6. **Commute & Transit Topology**: Total edges, active journeys, arrived journeys, cancelled journeys, and delayed journeys past scheduled arrival.
    7. **Relationship Network**: Total familiarity ties, entities with ties, isolated entities (0 ties), isolated ratio, and graph density.
    8. **Event Density & Repetition**: Total events, events by type, top event types sorted descending, events per day, and repetition anomaly alerts.
    9. **Agent Decision Distribution**: Total NPC decisions, decisions by action, decisions by actor, and active external controllers.
    10. **Knowledge Containment**: Total observations, total knowledge records, channels breakdown (co_location, direct_message, rumor, org_announcement, public_notice), and leakage indicators.
    11. **Organization Decisions**: Active agency policies, total reviews, and reviews by decision kind (freeze, unfreeze, expand, no_change, adjust_schedule).
    12. **Failed / Rejected Actions**: Rejected commands count, cancelled journeys, default reviews, and insolvency/bankruptcy reviews.
    13. **Spatial Reachability**: Total location nodes, root places, orphan nodes, and unreachable places with zero transit edges.
    14. **Economic Conservation**: Double-entry balance zero-sum check (`DoubleEntryBalanceZeroSum == 0`), balanced postings count, and currency issuance conservation.
  - **Configurable Anomaly Thresholds & Health Status**:
    - Evaluates configurable anomaly thresholds (`MaxIsolatedEntityRatio`, `MinHousingCoverageRatio`, `MaxPastDueRentRatio`, `MaxUnemploymentRatio`, `MaxStagnantVacancies`) with sensible defaults.
    - Generates typed `WorldQAAnomaly` with severity (`info`, `warning`, `critical`).
    - Computes overall world status (`HEALTHY`, `WARNING`, `CRITICAL`) without mutating world facts.

- **Observer Mode (Non-Controlling Player & Creator Perspective)**:
  - Read-only timeline and digest generation for observers who do not pilot a specific actor.
  - Structured views across 5 perspectives:
    - **Macro Perspective**: Significant world events (laws enacted, cohort transitions, household formations, organization reviews, bankruptcies, public notices) with categories and headlines.
    - **Entity Life Perspective**: Work, household, relationships count, and recent decisions for target individuals.
    - **Organization Perspective**: Headcount (active employees), cash balance, active postings, and recent agency reviews.
    - **Relationship Perspective**: Recent familiarity changes with observer/subject, origin kind, and world time.
    - **Digest Perspective**: Period-bounded summaries over specified world-time windows (events count, new entities, households formed, contracts formed, wages disbursed, macro highlights, key events).
  - **Direct Studio Inspector Linkage**:
    - Every summarized event and entity item provides a direct link to the Studio Inspector (`/studio/inspect?instance_id=...&branch_id=...&event_id=...`).
  - **Strict Authorization Scoping**:
    - `creator`: Full unredacted view across all perspectives (unredacted wages, full org balances, unredacted decision reason codes).
    - `operator`: Diagnostic access with aggregated health.
    - `observer` (resident/public player): Public facts only; wages redacted to 0, org cash balances masked/nil, sensitive internal decision reason codes scrubbed.

- **Representative Multi-Day Long Run Demonstration**:
  - `TestF10RepresentativeLongRun_WorldQAAndObserver` establishes a small multi-agent world:
    - Human player + multiple NPC residents (Ada, Bo, Cai).
    - Multiple distinct locations (home, store, workplace, cafe).
    - Active hiring: organization defined, position posted, Ada applies, interviews, evaluates, receives offer, and accepts.
    - Multi-day world time progression across day boundaries.
    - Spatial anti-clustering verified: agents distributed across multiple distinct places rather than congregating in a single room.
    - Economic conservation verified: double-entry bookkeeping strictly balances to zero sum (`DoubleEntryBalanceZeroSum == 0`).
    - Zero knowledge leakage verified (`LeakageIndicators == 0`).
    - Spatial integrity verified: zero orphan location nodes, full route reachability.
    - Full coherence across all Observer perspectives.

## Verification

### Requirement mapping

| Requirement | Implementation and Evidence |
| --- | --- |
| 14 Diagnostic Dimensions | `ReadWorldQA` in `backend/internal/storage/world_qa.go`; verified by `TestWorldQA_FourteenDimensionsAndStatus` |
| Threshold alerts without forced auto-fix | `WorldQAThresholds` and `WorldQAAnomaly`; verified by `TestWorldQA_ThresholdsAndAnomalyTriggers` |
| Observer Macro Perspective | `PerspectiveMacro` in `world_observer.go`; verified by `TestWorldObserver_MacroPerspective` |
| Observer Entity Life Summary | `PerspectiveEntity` in `world_observer.go`; verified by `TestWorldObserver_EntityPerspective_CreatorVsObserver` |
| Observer Organization Summary | `PerspectiveOrganization` in `world_observer.go`; verified by `TestWorldObserver_OrganizationPerspective` |
| Observer Relationship Changes | `PerspectiveRelationship` in `world_observer.go`; verified by `TestWorldObserver_RelationshipAndDigestPerspectives` |
| Observer Time-Window Digest | `PerspectiveDigest` in `world_observer.go`; verified by `TestWorldObserver_RelationshipAndDigestPerspectives` |
| Inspector Links | `makeInspectorLink` producing `/studio/inspect?...`; verified across all observer perspectives |
| Role-based Privacy Scoping | Verified by `TestWorldObserver_EntityPerspective_CreatorVsObserver` & `TestWorldObserver_OrganizationPerspective` |
| Anti-clustering validation | `TestF10RepresentativeLongRun_WorldQAAndObserver` step 4 |
| Lack of economic collapse / zero-sum | `TestF10RepresentativeLongRun_WorldQAAndObserver` step 5 |
| Zero knowledge leaks | `TestF10RepresentativeLongRun_WorldQAAndObserver` step 5 |

### Test suite results

- **F10 unit & integration tests**: PASS 9/9 tests (6.410s).
- **Targeted race detection**: PASS `internal/storage` 9/9 tests (158.021s).
- **Static checks**: `go vet ./...` PASS (0 errors); `git diff --check` PASS (clean).
