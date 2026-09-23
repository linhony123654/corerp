# M2 background economy: finite settlement, supply and arrears slice

Status: **30 scheduled wage/rent periods, purchases, consumption, finite restock and linked arrears/grace decisions implemented; the rest of the economy is not**. The existing
two-Agent 30-day spatial run alone has no economic settlement. This document covers the
isolated `inst_m2_t09/br_main` demo first; it does not redefine the M1 strict world
or claim the M2 100-person / 10-store / 30-day exit.

## Authority and ownership

- The existing Cohort represents 20 people at genesis and 18 after Ada and Bo are
  materialized. Its account and inventory belong to those remaining people, not
  to an employer, landlord, supplier or store. Open these as *separate*, branch-
  scoped counterparties by an idempotent setup Event Batch. The current demo setup
  is explicitly invoked via local CLI/storage; a caller-authenticated definition
  API is still open, and ordinary server startup never prepares this fixture.
- For the first local fixture, explicitly transfer finite startup capital and
  inventory from the existing Cohort to the new counterparties. Each initial
  transfer must have balanced postings or a stock movement and replayable
  projections in that setup batch; an empty account/location starts at zero.
  These transfers model a declared cooperative allocation, not an invisible
  endowment or newly issued currency/stock. Record transfer amounts, owners,
  quote, SKU, currency, contract and effective dates in the setup event.
- A Cohort employment contract pays a declared aggregate amount for **18
  background workers**, not for Ada and Bo. A housing contract covers only that
  Cohort. Their contract IDs, rate, unit/period, owner IDs, and due times are
  immutable for an accepted epoch; wage accrual recognizes payable/receivable
  once per `(contract_id, period)` before payment. The aggregate obligation
  records `population_count=18` and per-person rate, with checked integer
  multiplication. No individual wage, household wealth or observation is
  inferred from the aggregate beyond its explicit allocation rule.
- A one-person T09 during the active wage term must transfer exactly one
  currently Cohort-owned outstanding worker slot per open wage obligation,
  together with its receivable, and declare future participation at the next
  unaccrued wage task. A zero or mismatched transfer is rejected; the original
  18-person obligation, accrued origin slices and prior receipts are immutable.
  An entity may return only its verified wage cash and unpaid slots. T09 still
  cannot jump over a pending world-time task.

Additive [migration 008](schema-008-background-economy.sql) keeps economic ownership distinct from M1's
`economic_entities`: `m2_economic_actors` has an `(instance_id, branch_id)`
owner, `kind` (employer/store/landlord/supplier), account ID, optional stock
location ID and defining event; `m2_cohort_contracts` refers to the Cohort,
actor and currency, fixes the worker count and unit
rate, and has an effective range. `m2_economic_obligations` has a unique
`(contract_id, period_start, period_end, kind)`, due/paid integer amounts,
status, defining event and last event sequence. An obligation's payment facts
are separate from its accrual fact and cannot exceed due. Opening balances,
locations and any payable/receivable/expense/income accounts have their own
definition event. Enforce branch/currency/SKU joins in the write transaction;
foreign keys on individual IDs alone are insufficient for branch isolation.
Additive [migration 009](schema-009-finite-store.sql) defines a branch-checked
store offer with SKU, currency, fixed unit quote, daily budget, household-owned
destination and effective interval, plus one durable outcome per scheduled
attempt. Additive [migration 010](schema-010-consumption-supply.sql) records
the supplier's finite quote, linked daily consumption outcomes and paid-restock
outcomes. Additive [migration 011](schema-011-arrears.sql) declares contract-local
grace, links later settlement attempts to the original obligations and records
reasoned default reviews and one funded service order. A pre-011 demo definition
cannot be replayed as the new fixture. Additive [migration 012](schema-012-insolvency.sql)
declares a fixture-local employer insolvency policy, one day-30 review and an
event-linked open proceeding. A pre-012 demo definition likewise
cannot be replayed as the new fixture:
the setup call fails closed and requires a fresh local demo database. There
is no automatic retroactive stock allocation or task insertion. Additive
[migration 013](schema-013-bankruptcy-claims.sql) records each unpaid unsplit wage
obligation as an immutable, unranked Cohort claim at the same proceeding-opening
Event Batch. Its due/paid/outstanding amount, claimant, currency and opening
event are preserved; the canonical claim-set hash and count are part of the
opening event. A proceeding opened before migration 013 has no historical
claim snapshot and must not be treated as if it did: use a fresh local demo
database for claim-level refinement evidence, not a retroactive guess.
The demo's wage contract now declares a fixed day-30 07:12 end in its original
definition event. Existing local demo definitions without that term fail closed
on setup replay and require a fresh demo database. This is a declared contract
term, not an inferred bankruptcy discharge or a retroactive closure.
[Migration 014](schema-014-claim-allocations.sql) records immutable per-obligation
T09 claim assignments and matching return facts. An authorized one-person
materialization after the fixed term transfers exactly 10 from each of the 23
remaining wage claims (230 total) alongside 230 of Cohort receivable in its
one Event Batch; dematerialization restores both if no claim has since been
paid. The original wage obligations and bankruptcy snapshot remain unchanged.
The active rent contract covers the continuing *one household*, so the Cohort
must retain a positive population. [Migration 016](schema-016-wage-participation.sql)
records an immutable, event-linked one-worker participation right effective at
the next unaccrued wage task, plus claimant-specific obligation slices and
settlement receipts. The 18×10 employer obligation remains 180; after one
mid-term T09, the next accrual records 170 Cohort receivable/income and 10
named-entity receivable/income, and a funded due payment routes real employer
cash 170/10. T09 opens the named income account; it does not invent a personal
work history. Existing unpaid obligations cannot be silently reassigned, and
underfunded split payments and linked retries use the immutable
[`worker_round_robin_v1` policy](schema-017-wage-allocation-policy.sql): each
paid monetary unit walks the 18 worker slots, current Cohort slots first and
named entities in stable ID order. Payment receipts carry policy/hash/count;
zero liquidity records an overdue/deferred world fact, not an accounting error.
[Migration 018](schema-018-wage-claim-ownership.sql) records event-linked current
ownership transitions, verified participation returns and actual-recipient
slot receipts. A T09 after accrual or partial payment transfers only the
remaining unreceived slot value; later payout follows the current owner.
Dematerialization restores evidenced wages and outstanding receivables, and a
later materialization may claim them again without changing old origin slices.
[Migration 019](schema-019-bankruptcy-slot-claims.sql) freezes unpaid origin and
current creditor per slot at proceeding opening. The creator-scoped storage
`ReadM2WageClaimStatus` query returns current due/paid/outstanding and, after
opening, the immutable opening creditors. Split-claim estate distribution is
explicitly deferred until a future claimant-aware liquidation rule exists;
the old post-opening wage retry remains fail-closed. Old unsplit bankruptcy and
post-term T09/estate behavior remains unchanged.
[Migration 015](schema-015-estate-distribution.sql) adds one
fixture-local, predeclared distribution path: at day 31 07:00 the landlord
voluntarily contributes 18 of its posted cash to the employer estate, with no
recourse or new claim; at 07:01 the estate pays one unit per original worker
against the oldest unpaid wage obligation. The event and immutable receipts
route 18 to Cohort when no further worker was materialized, or 17 to Cohort
and 1 to a named claimant after a post-term one-person T09. The same transaction
reduces the original obligation, employer payable and each recipient receivable.
An unfunded contribution/distribution records a reasoned deferred fact without
creating cash. This is a declared demo choice, **not** a general creditor
priority, liquidation or discharge rule. After that payment, a new one-person
T09 can assign only the remaining Cohort receivable: 9 from the paid day-8
claim and 10 from each of the other 22 claims (229 total), whether the earlier
payout went entirely to Cohort or 1 went to a prior named claimant. Verified
recipient facts, active assignments and current Cohort population must reconcile
without rounding. This new assignment may return its 229 if its entity has
received no later claim payment; an entity already paid on its assignment
cannot dematerialize as if it still held the original receivable. No previously
paid cash or personal work history is inferred or transferred.

Implemented first slice: `economy-day1` initializes the separate two-Agent
fixture, then commits `M2BackgroundEconomyDefined` after allocating **1,200**
existing Cohort currency units to a separate cooperative employer. On the
next world day at 07:00 it accrues `18 × 10 = 180` currency units into the
employer expense/payable and Cohort receivable/income accounts, then at 07:01
pays 180 from existing employer cash into Cohort cash and clears the matched
payable/receivable. Each step has its own Event Batch, balanced posted journal,
Outbox, clock/Branch Head checkpoint, and typed obligation. Retries, rollback,
reopen, projection comparison and snapshot replay are tested. The initial
capital allocation is not an issuance. The same definition transfers **25** of
the Cohort's **89** existing SKU units to a separate store and **10** to a
separate supplier, leaving 54 in its original location. The store and supplier
cash accounts open at zero. The store's unit quote and daily food budget are
both 5; the supplier's unit quote is 3 with a 10-unit transport/order cap. A
declared sink and capability authorize consumption only from household-owned
stock. Population changes under these contracts require the guarded one-worker
T09 split; household rent and the store offer continue while the Cohort retains
a positive population.
For this fictional fixture, wage grace is 3 world days, rent grace is 2, and
late fees are zero; these are contract terms, not a global payment hierarchy.

The same explicit definition schedules 30 daily accrual/payment pairs **for
each of wage and rent**, one purchase at 07:04 and one conditional consumption
at 07:06 per day, plus a day-one 07:05 budget-boundary attempt. Day 26 and 27
also have declared 07:07 supplier-restock attempts. Contract-specific arrears
retries and a grace review follow those actions each world day; on day 15 the
landlord buys a declared 60-unit maintenance service from the employer using
existing rent cash before the wage retry. Opt-in
`economy30` also defines the two-Agent 30-day spatial routine and processes the
mixed queue. Six wages are initially paid in full; day 7 initially receives
120, then its remaining 60 is paid against **the same obligation** after the
day-15 service revenue. The 30 obligations show 5,400 due, 1,260 paid and
4,140 outstanding, without a second wage expense. A separate landlord starts with zero cash; the
one-household rent contract accrues 320 per day **after** wage payment. Of
9,600 rent due, 8,830 is actually paid; 27 periods are paid in full, day 28
receives 190, and days 29–30 remain unpaid. The remaining 770 is explicitly
recorded as Cohort payable/landlord receivable, with structured
`insufficient_tenant_liquidity` evidence. No landlord money is created by
accrual. The store sells 26 units for 130, transferring both cash and stock
through posted facts. The day-26 stock-out precedes a paid 10-unit/30-cash
supplier transfer; the next day's purchase succeeds. A second restock attempt
records supplier shortage without creating stock. The purchase path also records
3 insufficient-funds and 1 budget-limit rejections. Each successful purchase
is followed by a separate capability-backed `consume` fact; four rejected
purchases produce consumption skips, never implied stock disappearance. The
physical SKU total is 74 after 26 consumptions, while asset cash remains
10,000. Daily review marks each contract's elapsed grace exactly once; a
day-7 wage case is marked expired before its later cure, and day-28 rent's
grace expires on day 30. The scheduled review records that the old aggregate-
only refinement path is blocked while wage debts remain open; explicit T09
worker-slot transfer can still proceed with matching receivables. The review does **not** pretend liquidation
or eviction has executed. At day 30 07:12, after the grace review, the declared
fictional policy checks posted employer cash (0), unpaid wages (4,140), and
expired wage cases against a 360-unit threshold. It opens one durable
bankruptcy proceeding, linked to its review and Event Batch, without paying,
forgiving, ranking, or transferring any debt or assets. Its 23 outstanding
wage obligations become 23 immutable claims totalling 4,140; the cured day-7
obligation is excluded. At day 31, the separately scheduled donor transfer
and wage distribution provide a real partial payout against the oldest unpaid
claim, leaving 4,122 in employer wage payable; the original opening snapshot
remains 4,140. A no-action review is
recorded if the conditions fail. The open proceeding rejects subsequent
employer service execution; liquidation/discharge and post-opening split-claim
payment remain unimplemented. In all, 273 economic items interleave with 120 Agent movements
and reach branch head 399. Restart after an accrual,
unchanged population, bounded work, idempotent repeat and empty/snapshot
replay are tested. Remaining unpaid balances stay open debts, not forgiven
payments. This does not prove general supply/demand, bankruptcy liquidation,
complete contract splitting or the 100-person/10-store exit.
Corrupted cash or stock projections cannot be treated as genuine shortage:
the writers compare against posted accounting and movement facts before making
a payment or sale decision.

## Causal timeline and settlement

Each due item uses the shared M2 branch-wide queue, ordered by
`(world_time, phase_id, declared_priority, scheduler_item_id)`. Register
non-overlapping phase IDs for accrual, payment, rent, purchase, replenishment
and distress checks. Define all times before advancing the clock; setup rejects
due items **at or before** the committed clock (including a same-time earlier
phase). The runner independently rejects a backdated pending item. One
scheduler item commits at most
one Event Batch, its normalized journal/stock facts, checked balance/stock
projections, outcome/arrears reason, Branch Head CAS, clock and Outbox atomically.
The scheduled item's stable ID is its idempotency key. A crash/reopen resumes
from the first pending item, not from elapsed server time.
T09 materialization/dematerialization commands also reject world times before
committed branch history or its clock, and cannot jump over a pending due item;
this protects the later contract/claim split from retroactive population edits.

1. Accrue wage expense/payable exactly once for the period. Without a T09
   participation transfer, accrue Cohort receivable/income and pay the Cohort
   from employer *available cash*, with partial payment and explicit arrears
   when needed. With an active transfer, accrue separate Cohort/entity slices
   and pay each from employer available cash under the cumulative slot policy,
   including partial/zero payments and subsequent retries. Never issue cash to settle wages.
   Unsplit zero-payment due items still record an auditable reason.
2. Accrue rent once. Pay from Cohort available cash to landlord after wages;
   record partial/unpaid rent and any grace expiry as separate state/facts.
3. Evaluate the Cohort's declared food budget and seller's quote/cash/stock.
   If any constraint fails, record a typed rejection with evidence; otherwise
   move existing cash to the store and one existing SKU unit from store to a
   household-owned location. Actual consumption is a distinct, authorized
   `consume` stock movement to a declared sink; purchase alone does not make
   food disappear or put it back into the store's warehouse.
4. Replenish from a finite supplier-owned location under an explicit quote and
   store cash/transport capacity. Move cash store→supplier and stock
   supplier→store. No unlimited external pool or unrecorded stock creation.
5. A late payment references the original wage/rent obligation and a distinct
   attempt ID. After each declared grace deadline, record a single case
   transition and a reasoned review even when no new deadline expires. An
   active aggregate contract blocks unsafe refinement. The separate, declared
   day-30 employer policy may open a proceeding only after its cash, debt and
   elapsed-grace checks. Liquidation, eviction and broader
   distress policy remain open; these reviews do not silently execute them. Do not blindly multiply daily
   totals over an interval. This is a world-time rule, independent of process
   uptime and the UI.

## Gate for implementation

The accepted local slice now covers funded wage/rent, sourced store and
supplier stock, posted purchase and consumption, a finite paid restock,
linked arrears/grace decisions, budget/cash/stock rejections, rollback, reopen, projection repair,
empty/snapshot replay and idempotent scheduler retry. Its explicit extra
day-one purchase and day-27 restock are boundary fixtures, not general demand
or organization decision models. A fixture-local proceeding opening, claim
snapshot, post-term T09 assignment/return, active split partial payment,
arrears cure, T09 slot transfer/return and named bankruptcy-origin snapshot
are executable. Future split-claim liquidation/discharge remains deferred, with
World/System Pack legal policy rather
than one hardcoded global creditor order.

Separately test 100
people/10 stores/30 days, organization supply/demand and observer-scoped
status/perception; these are still open M2 gates. No frontend decision is
required for this backend contract.

Sources: revised blueprint §§22.3, 22.7, 22.8, 23, 25.2 and 26.2; schema
migrations 001, 006–019; current M1 strict scheduler and M2 Agent routine.
