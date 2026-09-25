# F4 design — household as a shared life constraint

Status: **BUILD — household foundation, rent, member history and dependency slices implemented; F4 acceptance OPEN**. Baseline is
the clean F3 checkpoint `2a8cb08bbe01eb7b22c1d546bab0a5bec6770ef3` on
`f4-household` in `/home/ubuntu/corerp-goal`. The separately approved Play UI
changes remain in the original `/home/ubuntu/corerp-console` worktree.

## Source-backed reuse and gap

- `economic_entities` already permits `entity_kind='household'`, but has no
  sourced membership, roles, residence or decision authority. It is an
  economic identity, not another Person. `materialized_entities` and
  `agent_profiles` remain the individual resident owners.
- Migration 003's `household_budgets` is a fixed-period food/rent-limit table.
  Demo bootstrap writes one row per employee entity, and the scheduler looks
  up a single `household_entity_id` for food purchases. The name does not prove
  shared membership or a derived multi-person budget.
- Existing `rent_contracts`/`rent_obligations` and scheduler accounting have
  one tenant Entity and one tenant payment account. Wage contracts/payroll and
  personal accounts remain separate. Shared rent must retain an explicit
  per-person funding/obligation source rather than silently merging wallets or
  replacing old rent semantics.
- `RPLifeContext` currently reads an NPC's own assets, liabilities, rent due,
  employment and unemployment; `DeriveRPLifeGoals` already turns cash pressure
  into `stabilize_income`/`find_work`. Household pressure can enter this
  existing decision boundary as a derived, source-backed member view, without
  disclosing other members' private account IDs or balances to an external
  model. The unrelated chosen-family cultural declaration is not membership.

## Acceptance contract and first vertical slice

1. Create one stable, world/branch-scoped household identity and enroll two
   existing adult Entities with sourced start time, role and authority; retain
   both persons and their accounts. A member's exit ends an interval, never
   removes the historical source. Residence is an association to an existing
   place, not a new spatial authority.
2. Establish one shared rent obligation and explicit contribution shares.
   Accrual, attempted payment, partial payment and debt must trace to one
   Event/source and existing personal posting accounts. No payment may be
   inferred from co-residence alone. Each authorized decision has an exact
   principal/member scope, payload/key recovery and stale-generation fence.
3. In a disposable world, run two adults with separate wage sources and one
   common rent: one loses employment, reducing expected coverage. A derived
   household budget-pressure view changes an affected member's consumption,
   housing or work goal via the existing decision input. This must be an
   observed behavior change, not a hardcoded narrative assertion.
4. A nonmember cannot read private household assets/contributions. An assigned
   external model receives only the authorized member's bounded derived view,
   never another member's raw account/Entity/evidence IDs through perception.
5. Exact retry, process reopen and Compare→Rebuild→Compare preserve identity,
   membership history, obligation/payment sources and budget pressure.

## Selected first-slice architecture

Use one new world/branch-scoped, sourced household membership/residence
projection, linked to a new `economic_entities(kind='household')` identity and
a household-owned **rent-only** payment account. Keep each person's existing
asset/income accounts. A source-backed contribution agreement records each
member's share; authorized, explicit member payments post from the member's
account into the rent account. The M1 rent contract and two-sided obligation
ledger provide a reusable accounting shape for **one** landlord claim, but
its scheduler path cannot be reused unchanged in the M2/RP world. The
household account does not give another member authority over personal cash,
and an unpaid share remains traceable to the agreement and payment Events.

This is an additive adapter, not a reinterpretation of the demo's per-person
`household_budgets` rows or old rent contracts. New membership, agreement,
contribution and account-opening Events need projection compare/rebuild coverage;
the existing ledger replay continues to own balances/postings. A member-facing
pressure view should derive aggregate rent coverage and that member's own
responsibility from sourced obligations, contributions and employment status,
then feed the existing `RPLifeContext` goal derivation. External models receive
only that bounded view, not other members' balances, account IDs or raw event
IDs. Migration 042 follows the existing Event-time account creation pattern;
its household-specific projection check covers an unopened zero-balance account,
while generic ledger replay takes over after the first posted contribution.
The design does not authorize ad hoc balance updates or a second ledger.

## Implemented slices and remaining boundary

Migration 042 now creates scoped household/membership projections. A local
operator can found one household from two existing active adults and a scoped
residence. One sourced Event opens a separate zero-balance rent account and
records each adult interval; it grants no control over either personal
account. A current member's RP session can read a bounded own-role/aggregate
view, while a nonmember cannot. Compare/Rebuild checks the new household,
members, economic identity and account opening, including the otherwise
unobserved zero balance before any posting. The focused test covers rollback,
exact retry/mismatch, duplicate membership, non-operator denial, privacy,
reopen and deliberate projection corruption/repair. This is **not** shared
rent payment or budget-pressure acceptance.

Migration 043 now records one local-operator-defined, branch-scoped rent
agreement with exact two-adult shares, a dedicated landlord economic identity
and cash account, an existing-format `rent_contracts` row and four rent
accrual accounts. The landlord is created for this agreement, not borrowed
from M1's separate demo world. This operator declaration does not itself move
personal cash or prove member consent. Migration 044 records each current
member's explicit, period-scoped contribution. The member's own active
asset account is debited and the household rent-only account credited through
one balanced, posted journal in the same transaction as the private source
Event and projection; the command enforces current controller generation,
active membership, observed head, share cap and sufficient personal cash.
The member view exposes only the aggregate rent/fund and that member's share,
paid and remaining amounts, not the other member's raw identity/account.
Compare/Rebuild now audits agreement, contract/ledger linkages, opened accounts,
shares, contribution rows and journal shape; generic ledger replay owns
balances. Focused tests cover rollback, exact retry, nonoperator/nonmember,
privacy, reopen and deliberate projection corruption/repair.

Recurring rent now uses RP-scoped AgentLife scheduler items. The agreement
sources both its start world time and start world day; every period queues
accrual, payment and past-due checks at derived timestamps, not M1's fixed
epoch. Accrual sources the next period's queue in the same transaction.
Ledger-backed private Events, balanced journals and guarded balances record
the obligation and attempted payment; insufficient funds leave an explicit
partial or failed settlement and later past-due state. Compare/Rebuild derives
queues, obligations and settlements from agreement and rent Events. It also
checks the exact posted journal and four posting amounts/accounts for every
money-moving rent Event; unexpected or missing journal evidence requires
manual audit rather than synthesizing immutable accounting history. The
disposable AgentLife test covers rollback, partial payment, reopen, recurrence,
source repair and a late-night boundary. Funding currently caps each member's
contribution at the current period's agreed share; old arrears cannot be
refinanced through that command. A bounded `RPLifeContext.household_pressure`
now forecasts the next rent period from active members' sourced wage terms,
the rent-only household fund and outstanding rent obligations. It reports
the household rent/fund/outstanding and observer's own share, never another
member's Entity/account/evidence IDs; exact combined wage and deficit amounts
remain local-only because a two-person member could subtract their own income
to infer the other's. The private rent-agreement Event ID is checked inside
the source query but is not appended to serialized life/goal evidence.
External decisions receive a `covered`/`at_risk` pressure level. The view
grants no payment authority. A disposable
two-income fixture uses Ada's independent employment and Cai's M2 wage split:
before Cai's sourced exit, coverage is sufficient; after activation, the
forecast gap is positive and Ada gains `stabilize_household_income`; the
deterministic provider then changes its actual proposal to a rent-prioritizing
refusal, whereas the pre-shock proposal has no rent rationale. The
nonmember has no pressure view, and the result survives reopen. This is a
forecast of current active wage terms, not a promise of future payment or a
full historical income statement. A current adult can now submit a private,
reasoned `RPHouseholdMemberLeft` Event with a current control generation and
observed head. It ends that membership interval only: the Person, earlier
contributions, rent share and active lease remain sourced, and the former
member loses household private read/fresh-funding access. Exact-key receipts
remain recoverable. Compare/Rebuild preserves both interval endpoints; the
last adult must use a future explicit household/lease closure workflow rather
than leaving an active lease ownerless. Migration 045 and a local-operator
`RPHouseholdDependentEnrolled` Event now connect an existing active Person to
one active adult supporter's membership. This is a sourced care/dependency
relation, not legal custody, a copied Person, an account grant or a rent share.
The dependent has a bounded own-role/member-count view without another
member's canonical identity, and an adult cannot self-exit while they have
an active dependent. Compare/Rebuild audits and repairs the supporter link.
There is no dependent departure or support-transfer command yet, so a
supporter exit may be blocked until a future explicit transition exists.
Member-authorized lease/share changes, external-controller filtering and full
F4 acceptance/regression remain required.

The contribution Event is a private household financial source, not a public
gift or transfer of general account authority. Accepted-key replay returns
only the original session principal's receipt after a lost response. Current
funding is limited to the current rent period and up to each agreed share;
arrears refinancing, refunds and member-consent amendments are not implied.
