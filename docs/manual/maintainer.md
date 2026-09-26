# CoreRP Maintainer & Architecture Manual

This manual provides the technical engineering reference for CoreRP maintainers, system operators, and contributors. It documents the core event-sourced architecture, state transitions, sub-system contracts, migrations, testing methodology, and recovery procedures.

---

## 1. Core Architecture: The Unidirectional State Pipeline

CoreRP is strictly event-sourced and guarantees deterministic replay:

```text
  Client Command (with Idempotency Key)
               │
               ▼
       Domain Transaction
    (Validates Business Rules)
               │
               ▼
       Immutable Event(s)
  (Appended to SQLite events Table)
         │           │
         │           ▼
         │   Asynchronous Projections
         │   (Balances, Positions, Caches)
         ▼
  Bounded Observations
  (Derived Sensory Context)
```

### Invariants

1. **Commands never mutate projections directly**: All state changes occur by appending immutable rows to `events`.
2. **Projections are disposable**: Any projection table (e.g. `account_balances`, `agent_positions`) can be dropped and reconstructed from `events` using `RebuildProjections`.
3. **Double-Entry Bookkeeping**: Economic transactions emit balanced `postings` inside `journal_entries`. The sum of all debits and credits across the universe is identically zero (`DoubleEntryBalanceZeroSum == 0`).

---

## 2. Interaction, Turns & Epistemic Knowledge

### Turn Lifecycle (`rp_turn_runs`, `rp_utterances`)

- A player turn submits an utterance (`UtterRP`) with speech and action text.
- Co-located NPC actors evaluate the utterance against their salient memories and goals, emitting child decisions in `rp_npc_decisions`.
- Co-located observers receive rows in `observation_records_f7` and `agent_knowledge_f7`.

### Knowledge Containment

Knowledge is never granted globally:
- `co_location`: Generated only when observer and subject occupy the same location at the observed world time.
- `direct_message`: Sourced from an explicit `RPInformationSent` event addressed to the recipient.
- `rumor`: Single-hop relay of an existing message with explicit relay authorization.
- `organization_announcement`: Sourced from managerial events (e.g., layoff notices) and accessible only to verified employees.
- `public_notice`: Official promulgation of enacted laws.

---

## 3. Scheduler, World Clock & Spatial Journeys

### World Clock & Phases (`world_clocks`)

World time is stored as RFC3339 strings (e.g., `2026-09-22T08:00:00Z`) with a monotonically advancing `current_day`. The scheduler advances time during `WaitRP` operations by draining due items from `scheduler_items`.

### Half-Open Occupancy Intervals

Rather than maintaining a mutable location column that can desynchronize, CoreRP defines occupancy as a half-open temporal interval via the `rp_occupancy_intervals` view:
- `entered_at` (inclusive): Timestamp of the movement event.
- `exited_at` (exclusive): Timestamp of the subsequent movement event.
- If no subsequent movement exists, the interval remains open.

### Directed Timed Edges (`rp_timed_edges`, `rp_journeys`)

When an agent departs along a timed edge:
1. `rp_journeys` records an active journey with `scheduled_arrival_at`.
2. A scheduler entry is enqueued for the arrival timestamp.
3. The agent occupies the transitional `segment_place_id` during travel.
4. When arrival triggers, the agent enters the destination place.

---

## 4. Subsystem Domain Contracts

### Households (`rp_households`, `rp_household_memberships`)
- Separates personal property from shared domestic obligations.
- Shared rent account funded by adult members.
- Dynamic rent pressure forecasting (`covered`, `strained`, `critical`).

### Health & Body (`rp_condition`)
- Sleep debt, fatigue, and minor conditions.
- Fatigue changes NPC decision refusal probabilities without creating complex Sim-like demand bars.
- Objective world health truth is strictly separated from observed symptoms and character self-claims.

### Education & Career (`education`, `career`)
- Traceable path: `Enrollment` -> `Training` -> `Credential` -> `Position Application` -> `Hiring` -> `Shift Schedule` -> `Promotion`.
- Organization agency policies (`organization_agency_policies`) run automated review cycles to freeze, unfreeze, or expand hiring based on cash reserves and liabilities.

---

## 5. Database Schema & Migrations

CoreRP uses modern, embedded SQLite with strict type affinity (`STRICT` tables).

- Migrations are versioned sequentially in `backend/internal/storage/migrations/` (`001_init.sql` through `056_organization_review_schedule.sql`).
- Embedded into the Go binary using `//go:embed migrations/*.sql`.
- Applied automatically on store initialization (`Open`).
- Never perform out-of-order schema mutations or raw SQL table drops outside the migration chain.

---

## 6. Backup, Recovery & Projection Verification

### Backup

Because SQLite uses WAL mode (`PRAGMA journal_mode=WAL`), you can perform online backups using the SQLite backup API or by taking a snapshot while no transaction is active:

```bash
# Safe live backup
sqlite3 /data/corerp.db ".backup /data/backups/corerp-$(date +%s).db"
```

### Projection Drift Detection (`CompareProjections`)

To verify that the current database projections have not drifted from the authoritative event log:

```go
diffs, err := store.CompareProjections(ctx, instanceID, branchID)
if err != nil {
    log.Fatalf("Compare failed: %v", err)
}
if len(diffs) > 0 {
    log.Printf("Detected %d projection divergences: %+v", len(diffs), diffs)
}
```

### Projection Rebuild (`RebuildProjections`)

If corruption or divergence is detected, projections can be rebuilt deterministically from historical events:

```go
err := store.RebuildProjections(ctx, instanceID, branchID)
```

---

## 7. World QA Diagnostics & Health Monitoring

CoreRP provides automated read-only simulation health checks across 14 dimensions:

```bash
curl -X POST http://localhost:8080/api/v1/studio/qa \
  -H "Authorization: Bearer principal_operator" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "inst_m2_t09",
    "branch_id": "br_main"
  }'
```

### The 14 Diagnostic Dimensions

1. **Population & Cohorts**: Materialized entities count, active profiles, demographic distribution.
2. **Employment & Vacancies**: Active contracts, open postings, unemployment ratio.
3. **Finances & Arrears**: Ledger balances, accrued wages, rent past-due minor.
4. **Household Pressure**: Covered vs strained vs critical household ratios.
5. **Housing Coverage**: Housed vs unhoused population ratio.
6. **Commute & Transit**: Active, delayed, arrived, and cancelled journeys.
7. **Relationship Density**: Familiarity network graph density, isolated entities (0 ties).
8. **Event Frequency**: Events per day, top event types, repetition alert detection.
9. **Agent Decisions**: Decisions by action, decisions by actor, external controller count.
10. **Knowledge Containment**: Observation channel breakdown, leakage anomaly indicators.
11. **Organization Decisions**: Agency policies, review frequency, freeze/expansion ratios.
12. **Failed Actions**: Rejected command count, default reviews, insolvency filings.
13. **Spatial Reachability**: Orphan location nodes, unreachable places with zero links.
14. **Economic Conservation**: Verification that double-entry balance sum is strictly zero.

Health statuses:
- `HEALTHY`: All invariants satisfied.
- `WARNING`: Anomaly threshold exceeded (e.g. high unemployment or unhoused ratio) without core invariant failure.
- `CRITICAL`: Invariant violated (e.g. non-zero balance sum, orphan spatial nodes, knowledge containment leak).

---

## 8. Test Execution Strategy & Race Splitting

CoreRP uses pure-Go SQLite (`modernc.org/sqlite`). Under Go's `-race` detector, pure-Go SQLite emulates C semantics in software and runs ~15–20x slower.

### Recommended CI / Local Test Workflow

1. **Fast Packages Suite (< 1 minute)**:
   ```bash
   /usr/local/go/bin/go test ./backend/internal/core/...
   /usr/local/go/bin/go test ./backend/internal/transport/httpapi/...
   /usr/local/go/bin/go test ./backend/internal/storage -run "TestWorldQA|TestWorldObserver"
   ```

2. **Race-Split Targeted Verification**:
   Run targeted tests with race detection rather than the entire 1,500-test storage suite in a single invocation:
   ```bash
   /usr/local/go/bin/go test -race ./backend/internal/storage -run "TestWorldQA|TestWorldObserver|TestF10"
   ```

3. **Static Checks**:
   ```bash
   /usr/local/go/bin/go vet ./backend/...
   git diff --check
   ```
