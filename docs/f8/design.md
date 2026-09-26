# F8 design — organization agency, scheduled review and life-system consequences

Status: **local acceptance PASS**. Baseline is the clean F7 checkpoint
`3d400c31ced0e77830019e3526c6d65a3cb02042` in the independent
`f8-organization` worktree. The containing scoped commit is the F8 checkpoint;
no push or deployment is included.

Build audit (2026-09-26): manually invoked reviews now enforce the declared
interval. Real wage expenditure supplies the freeze fixture; unfreezing follows
an explicit reserve-policy revision. Cash is checked against posted journals.
Policy and review reads use immutable Events, and Compare/Rebuild reconstruct
their derived tables while checking deterministic outcomes and source metadata.
Manager-only reads and F7 review-notice validation are implemented. Final-source
affected regression, focused race and scoped static checks pass. Migration 056 adds explicit manager opt-in
`automatic_review`: a policy/review sources its next bounded scheduler item,
execution rechecks policy and authority, and private terminal Events support
queue Compare/Rebuild. System-authored decisions retain the policy Event and
authorizing manager for F7 publication. Application and offer validation now
compares original Event-backed job terms with the current posting, allowing only
capacity/status changes; inactive recruitment still blocks both offer creation
and acceptance, and credentials/current capacity are independently checked.
Focused expansion/freeze tests at both hiring boundaries pass. The representative
automatic-expansion fixture fills the original slot, rejects a second offer,
then expands and admits the existing second application. Household forecast
income changes from 360 to 720 and rent gap from 140 to zero. Both actual
employees discover/access the sourced notice and record their interpretation;
an outsider cannot discover or access it. Restart and cross-system projection
comparison pass. Business evidence now derives wage payable account membership
from original employment/economy Events and balances from posted journals;
deleted or forged obligation rows cannot hide liabilities. Active workforce
includes sourced cohort participation and independent contracts only after
their start, subtracting activated exits rather than notices. Historical review
audits recompute cash, payables, workforce and pre-review posting terms at the
preceding Event sequence. Final-source full regression PASS (storage1326.854s,
HTTP30.900s), current identity/recovery/cross-system race PASS46.564s and MCP
PASS4/4. See `phase-report.md` for scope, limits and checkpoint handoff.

This is the unreleased F8 evidence contract: `active_employees` counts actually
started independent jobs plus the supported M2 cohort workforce, not onboarding
contracts alone. Posted wage-liability journals are the accounting authority;
the organization read does not repair obligation or contract projections.
New review IDs retain the full256-bit source digest. The legacy short-prefix
collision is covered by a deterministic regression; existing source record IDs
are retained rather than rewritten during replay or projection repair.

## Purpose and observable acceptance

Organizations in CoreRP (such as businesses, cooperatives, schools, hospitals,
and institutions) must not remain static database rows or passive registries.
F8 makes organizations low-frequency, capability-bounded, scheduled world
Actors that evaluate internal evidence, execute authorized decision procedures,
and produce consequential changes across Career, Household, and Information
systems.

An Organization Agent is explicitly **not** an always-online LLM:
1. **Evidence-based review**: Uses rule-based detection and scheduled review
   intervals (e.g. business days at designated review hours) rather than continuous
   polling.
2. **Deterministic policy**: Evaluates objective business metrics (cash balance,
   wage liabilities, active headcount and position capacity)
   against bounded thresholds to propose actions.
3. **Authorized decision point**: Operational changes require an authorized
   Manager Principal or valid organizational procedure. An AI suggestion does
   not equal a corporate decision.
4. **Budgeted frequency**: Review events have bounded execution budgets and
   scheduled cooldowns, avoiding unbounded decision thrashing.
5. **Privacy boundary**: Organizations inspect only aggregate organizational
   metrics and employee attendance; they never read private employee health
   diagnoses, private messages, or confidential household budgets.

### Minimal agency loop

```text
organization state / evidence
  → authorized decision point
  → proposal (freeze recruitment, unfreeze recruitment, expand capacity)
  → procedure / authority / rule validation
  → immutable Career Event carrying review evidence and enacted decision
  → downstream consequences:
      • Career: posting status, application availability, capacity
      • Household: member wage changes, financial pressure forecast
      • Information Network: F7 organization notice published to employees
```

### Representative slice

We test a representative retail organization slice using the existing
`actor_m2_coop_employer` ("街区合作社"):
1. **Financial & Workforce Evidence**:
   - The cooperative's cash account, wage liabilities, active contracts, and
     posting capacities are evaluated.
2. **Review & Decision**:
   - With financial headroom and capacity below the manager's declared target,
     the organization expands an existing posting (`expand_capacity`) or
     unfreezes it (`unfreeze_recruitment`). Headcount is explanatory evidence,
     not a staffing-demand model.
   - Under cash deficit / economic pressure, the organization enacts a recruitment
     freeze (`freeze_recruitment`), blocking new applications, offer creation
     and offer acceptance. Existing employment/payroll is not cancelled.
   - Schedule adjustment and creating a new posting are not implemented in F8;
     the original specification requires one of these operational consequences,
     and this slice implements freeze/unfreeze/capacity on an existing posting.
3. **Consequential Cross-System Effects**:
   - **Career**: Applications to a frozen posting fail with `BRANCH_VERSION_CONFLICT`;
     applications to an opened posting succeed through hiring.
   - **Household**: When an unemployed or low-income member secures employment
     from the newly opened vacancy, expected wage income increases and the
     household pressure forecast improves in `RPLifeContext`. This does not
     transfer rent contributions or guarantee future household payment.
   - **Information Network**: The manager publishes a redacted F7 organization
     notice for the decision; active employees discover, access, and take stances
     (`believe`, `doubt`), while outsiders cannot access internal operational notices.
4. **Recovery & Integrity**:
   - Restart, Event replay, CompareProjections, and idempotent deduplication
     verify that organizational decisions survive crashes without state divergence.

## Verified reuse and gaps

- **Career system (`backend/internal/storage/career_*.go`)**:
  - `CareerOrganizationDefinition`: defines org ID, manager principal, workplace.
  - `CareerPostingDefinition`: defines capacity, wage, qualifications, and credentials.
  - `authorizeCareerManager`: enforces management capabilities.
  - Postings are Event-backed definitions without a `career_postings`
    table. F8 snapshots changed posting status/capacity in an organization-review
    Career Event; both authorized manual and opt-in scheduled reviews use the
    same deterministic procedure.
- **Economic system (`backend/internal/storage/m2_economy.go`)**:
  - `m2_economic_actors`, cash accounts, wage liabilities, and balance queries.
  - `organizationBusinessSnapshot` derives source-defined wage payable accounts
    and workforce. Posted journals supply liabilities; cash is journal-checked.
    Financial headroom is cash minus accrued, unsettled wage liabilities.
- **Household system (`backend/internal/storage/rp_household*.go`)**:
  - Calculates household rent obligations, member income contributions, and
    financial pressure forecast (`RPLifeContext.HouseholdPressure`).
  - *Integration*: A newly opened position that hires a household member directly
    modifies that member's wage income and lowers household pressure.
- **Information network (`backend/internal/storage/rp_information_organization*.go`)**:
  - F7 established `PublishRPOrganizationNotice`, `AccessRPOrganizationNotice`,
    and `RecordRPInformationStance`.
  - *Integration*: Organization decision events serve as real business sources for
    workforce announcements.
- **Scheduler (`backend/internal/storage/agent.go`, `scheduler_items`)**:
  - Bounded scheduler items trigger deterministic life events at designated world times.
  - The `organization_review` phase executes bounded periodic reviews, with
    source-generation/manager rechecks and recoverable next-item scheduling.

## Architecture and increment order

1. **Schema & Migration 055**:
   - Add `organization_agency_policies`: defines review schedule, financial thresholds,
     manager principal, and target positions.
   - Add `organization_reviews`: records historical reviews, evidence snapshots,
     and enacted decisions.
   - Add an optional status to the Event-backed Career posting definition;
     a review Event carries a changed posting snapshot (`active`/`frozen`).
   - Migration 056 adds `automatic_review`, default false for existing policies;
     only an explicit manager revision opts into periodic execution.
2. **Core Domain & Types (`backend/internal/core/organization_agency.go`)**:
   - `OrganizationAgencyPolicy`, `OrganizationEvidence`, `OrganizationDecision`,
     `OrganizationReviewRequest`, `OrganizationReviewResult`.
3. **Storage & Execution (`backend/internal/storage/organization_agency*.go`)**:
   - `DefineOrganizationAgencyPolicy`: sets policy under manager authorization.
   - `ConductOrganizationReview`: gathers evidence, computes a deterministic
     proposal, validates authority, and writes one immutable Career review
     Event containing the enacted posting snapshot; no second Event is implied.
   - Scheduler integration: registers `organization_review` scheduler items for periodic execution.
4. **Cross-System Verification Fixture**:
   - Full integration test exercising evidence change → review → posting freeze / vacancy creation
     → Career application gate → Household pressure change → F7 organization announcement & stance.
5. **HTTP & API Boundary**:
   - Authenticated endpoints for policy definition and manual review execution under bearer principal.
6. **Regression, Phase Report & Checkpoint**:
   - Run focused storage/HTTP tests, race detection, fast packages, static vet/gofmt,
     and full storage regression suite.
