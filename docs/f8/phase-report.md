# F8 local phase report — organization agency

Status: **local acceptance PASS**. Baseline is F7 checkpoint
`3d400c31ced0e77830019e3526c6d65a3cb02042`. The containing scoped commit is the
F8 checkpoint; verify its clean worktree before entering F9. No push or release.

## Implemented and demonstrated

- Migrations055–056 add recoverable policy/review projections and explicit
  manager opt-in periodic reviews. Manual and scheduled reviews share one
  deterministic procedure with bounded intervals, current authority checks,
  source-generation checks, atomic next-item scheduling and rollback recovery.
- Original Career/economy Events determine wage payable accounts and effective
  workforce. Posted journals determine cash and unpaid wages; mutable missing
  or forged obligation rows cannot authorize a false operational decision.
  Cohort employees, future starts and activated departures are accounted for.
- Financial headroom can freeze/unfreeze recruitment or expand an existing
  posting to the manager's declared target. Operational revisions preserve
  submitted applications/offers without accepting altered salary, qualifications,
  credentials, capabilities or other substantive job terms. Frozen recruitment
  denies applications, offer creation and acceptance, not existing payroll.
- The representative full posting rejects a second qualified candidate's offer;
  a scheduled review expands capacity and the original application completes
  hiring. Household forecast income360→720 removes rent gap140. Both actual
  employees explicitly access the sourced notice and retain their stance;
  an outsider cannot discover/access it and learns nothing. Restart and
  cross-system CompareProjections pass.
- F7 manager notice publication accepts authorized system-review lineage;
  model-facing source discovery/resolution uses session-bound opaque handles,
  not raw business evidence or identity IDs. Organization evidence does not
  query household budgets, health or private messages.
- Compare/Rebuild reconstruct policy, reviews and scheduler state. Historical
  review auditing independently recomputes cash, liabilities, workforce and
  prior posting terms at the preceding Event sequence; self-consistent forged
  evidence or altered review-outcome salary is rejected.
- Final audit caught a truncated review ID: slicing the first12 characters of
  `sha256:<digest>` retained only20 digest bits. New reviews now use the full
  256-bit digest; existing source IDs are not rewritten. A deterministic
  collision fixture exercises two distinct inputs sharing the old prefix.

## Verification

### Original F8 requirement mapping

| Requirement | Current evidence |
| --- | --- |
| Low-frequency, bounded organization actor | `TestOrganizationAgencyScheduledReviewsRecoverAndRespectPolicy`: bounded scheduler run, recurring reviews, suspension and sourced queue recovery |
| Authorized role/procedure, not model advice | `TestOrganizationAgencyPolicyDefinitionAndReviewAuthorization`, HTTP workflow, and scheduled rollback/authority-loss fixture; deterministic policy is the only decision procedure |
| Reliable business evidence changes operations | Real wage expenditure freeze/unfreeze fixture, source-journal corruption fixture and workforce start/exit fixture |
| Career, Household and Information consequences | `TestOrganizationAgencyVacancyChangesHouseholdAndInformation`: full posting, scheduled expansion, second hire, forecast improvement and employee-only explicit notice access |
| No unauthorized private-information reads | Business snapshot reads scoped employment/economy Events and journals, not household/health/chat tables; publication-source fixture proves opaque session handles and no cash/raw Event leakage |
| Recoverable authoritative world | Source projection/rebuild, historical tamper, rollback, retry and restart fixtures; final all-package regression PASS |
| Checkpoint before F9 | Containing scoped commit; post-commit clean-tree/parent audit required before F9 |

The management HTTP routes bind the authenticated principal before calling
storage authorization. Scheduled review execution independently rechecks the
current manager grant. Review Events are private and do not grant knowledge;
F7 explicit access is required. This is a source-and-fixture audit, not a claim
of live external-model execution or exhaustive authorization of future APIs.

### Executed checks

- Affected regression before the final ID correction: `go test -buildvcs=false -p=1 ./internal/core
  ./internal/storage ./internal/transport/httpapi -run
  '^(TestOrganizationAgency|TestCareer|TestRPInformation|TestRPShared)'
  -count=1 -timeout=10m` PASS (core0.003s/storage106.983s/HTTP16.165s).
- Effective-workforce lifecycle fixture PASS0.911s:18 before start,19 after
  start,19 after notices,18 after individual exit,17 after cohort departure.
- Scoped core/storage/HTTP vet, changed-source gofmt and diff whitespace PASS.
- `GOFLAGS=-p=1 npm test` in `clients/mcp`: PASS4/4 (14.802s), including real
  stdio→authenticated Runtime and mock-provider two-resident wiring. Initial
  run failed before integration because this independent worktree lacked the
  MCP client package. Locked offline `npm ci --offline --ignore-scripts
  --no-audit --no-fund` installed14 cached packages; package/lock files remain
  unchanged. No remote model call or live-provider verification is claimed.
- Accounting/audit changed-path race before the ID correction: `go test -race -buildvcs=false -p=1
  ./internal/storage -run
  '^TestOrganizationAgency(BusinessEvidence|HistoricalEvidence|Workforce|Vacancy)'
  -count=1 -timeout=5m` PASS126.499s, covering accounting, historical audit,
  effective workforce and cross-system restart recovery.
- ID correction: all `TestOrganizationAgency` storage/HTTP tests PASS
  (13.793s/0.832s), including the actual old-prefix collision fixture. Targeted
  final-source race for review identity, projection recovery and the complete
  vacancy/household/notice path PASS46.564s.
- Final-source MCP re-run after the ID correction:
  `TMPDIR=/dev/shm GOFLAGS=-p=1 npm test` PASS4/4 (8.158s). Fixture transport
  and mock provider only; no remote provider or live-model claim.
- Pre-ID-correction all-package Go regression PASS (storage1579.080s,
  HTTP51.146s; command/core/decision/narrative packages also passed). This is
  explicitly the earlier source baseline, not final-source completion.
- Final-source all-package Go regression PASS:
  `TMPDIR=/dev/shm go test -buildvcs=false -p=1 ./... -count=1 -timeout=40m`.
  Storage1326.854s, HTTP30.900s; all command/core/decision/narrative packages
  passed (the M1 command has no test files). Session26960 returned exit0.
  No tests were omitted; the alternate temporary directory avoided root-disk
  pressure without deleting unrelated files or caches. Only documentation
  changed after this final-source suite started.

During development the first historical audit held a query cursor while asking
the one-connection DB for another connection. Diagnostic SIGQUIT stacks and
focused tests identified this; buffering audits and closing the cursor before
secondary queries fixed it. Subsequent regression above includes the fix.

## Limits and handoff

- This slice expands/freezes existing postings; it does not implement automatic
  schedule adjustment, new-posting generation or inferred sales/workload demand.
  The original F8 requirement accepts one of these operational effects.
- Cohort source interpretation uses the existing supported M2 economy; no new
  omniscient model or autonomous external provider is introduced.
- Household evidence is a forecast, not payment or permission to spend money.
- Source-based business reads do not repair legacy obligation/contract tables.
- Original F8 acceptance has evidence; after confirming the containing commit
  has exact F7 ancestry and a clean recoverable tree, proceed to F9's two real
  Pack author lifecycles. The full F0–F13 goal is not complete.
