# F4 phase report — household as a shared life constraint

Status: **F4 local acceptance PASS — scoped checkpoint in the containing commit**. F3 checkpoint:
`2a8cb08bbe01eb7b22c1d546bab0a5bec6770ef3`. This report covers the
local F4 implementation, not a deployment or external-model live run.

## Delivered boundary

- Migrations 042–045 source a stable branch-scoped household economic identity,
  residence, two adult membership intervals, a dependent-to-active-adult
  support relation, and a rent-only household account. Existing Persons,
  personal accounts and controller grants remain separate. The support
  relation conveys neither legal custody nor wallet authority.
- A local operator declares one shared rent agreement and explicit two-adult
  shares. A current member may fund only their own share from their own
  account, within the current period and available balance. The private Event,
  balanced journal and contribution projection commit atomically. RP AgentLife
  accrues each period, attempts rent settlement and records unpaid/past-due
  state without an ad hoc balance update.
- A derived budget forecast combines the current shared rent fund, outstanding
  rent and active members' wage terms. The disposable two-income fixture
  sources one member's employment exit and proves the other member's derived
  goal **and actual deterministic proposal** change. Exact combined wage and
  coverage-gap amounts remain local-only; serialized external decision input
  contains only a bounded pressure level, aggregate rent/fund/debt and the
  observing member's own share. The private agreement Event ID stays in the
  storage provenance check and is not appended to the model's economic source
  list or the derived goal's exported evidence list.
- A current adult can leave by reasoned, private Event; their membership
  interval ends without deleting the Person, earlier contributions, rent
  agreement or accepted receipts. The last adult cannot abandon an active
  household; an adult with an active dependent cannot self-exit. An active
  dependent can read their bounded own-role view but gains no rent share or
  account authority.
- Household, member intervals/support, rent agreement, contribution,
  scheduler, obligation, settlement and posted-journal projections are
  compared against sources. Rebuild repairs mutable projections but refuses to
  fabricate or erase immutable journal evidence.

## Verification ledger

| Gate | Evidence | Status |
| --- | --- | --- |
| Person/authority separation | Household foundation and dependent tests: existing Persons unchanged, duplicate memberships and nonoperator commands denied, active controller/current-member reads scoped | PASS focused |
| Real shared obligation | Agreement, member contribution and scheduler fixtures: two separate shares, source Event/journal, partial settlement, arrears and recurrence | PASS focused |
| Income shock → decision | Two-income fixture: sourced job exit reduces coverage, changes `stabilize_household_income` goal and deterministic proposal; reopen retains it | PASS focused |
| Member end/history | Adult self-exit fixture: reason, ended interval, old receipt and shares retained, fresh private read/funding denied, last-adult and dependent-support guards | PASS focused |
| Privacy | Nonmember household read/pressure denied; member JSON omits other Person/Event/account IDs; exact combined wage/gap absent from external decision JSON | PASS focused; no new household HTTP/MCP route |
| Restart/replay | Disposable-world reopen and deliberate projection corruption followed by Compare→Rebuild→Compare across household/rent/dependent/scheduler paths | PASS focused |
| Full RP storage regression | `go test ./internal/storage -run '^TestRP' -count=1 -timeout=20m` PASS282.748s before the final household-source privacy fix; final all-package run includes these tests | PASS via final all-package run |
| Race, migration and cross-package | Final-source focused household race PASS39.122s; embedded/M0/M2 upgrade and core/decision/HTTP/all CLI included in final all-package run | PASS |
| Static | Final-source scoped vet, `git diff --check`, empty `gofmt -l` | PASS |
| All-package Go regression | `go test ./... -count=1 -timeout=40m` on final source: storage PASS1359.050s, HTTP PASS20.323s, all other packages PASS | PASS |

## Scoped limitations and next gate

The local operator records the initial lease and dependent relation; this is
not a claim of member consent or legal guardianship. A member exit preserves
the existing contractual rent share rather than automatically amending it.
There is no member-authorized lease amendment, dependent departure, support
transfer, household closure, arrears refinancing or refund command. In
particular, a supporter's exit remains blocked until a future explicit
transition exists. The present stage contract tests one adult exit/history,
not a complete family-law or lease-lifecycle system. External household
models receive only the bounded decision context; there is no new household
MCP/HTTP command. A provider live run is not claimed.

The F4 minimum contract and representative slice pass locally with the above
limits explicitly retained. The final-source all-package regression and
relevant race/static gates pass. The containing scoped commit is the F4
checkpoint; verify a clean recoverable worktree before entering F5. No
deployment is implied.
