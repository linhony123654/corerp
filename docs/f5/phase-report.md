# F5 phase report — sourced health, work and lawful knowledge

Status: **F5 local acceptance PASS — scoped checkpoint in the containing
commit**. This report covers local implementation only; no production release,
remote medical provider or clinical conclusion is claimed. F4 baseline checkpoint:
`7b9096f1ef2dd621ec42b4ec24081a1208d1f951`.

## Delivered boundary

- An actor's `RPSleepStarted` and `RPSleepEnded` Events source real rest intervals.
  Global waiting and being at home alone do not create sleep. Two consecutive
  short intervals derive moderate fatigue without a mutable need meter; a
  real committed evening refusal changes its reason and later long rest
  removes that fatigue refusal. Only bounded own fatigue/functional state is
  sent to the decision provider, not the sleep Event ID or exact duration.
- A current employee can attempt one `routine_check` during a real sourced
  shift. Its private Event records `completed` or `recheck_required` from
  current functional state; attendance and earned wages remain separate.
  Manager read is internally capability-authorized and returns only bounded
  task outcome, not private fatigue/condition evidence. No raw task/health
  truth read is exposed over HTTP or MCP.
- A local operator can source minor illness/injury onset, severity,
  active/recovering/resolved progression and optional independently sourced
  treatment reference. The subject perceives symptom and functional impact,
  not an automatic formal diagnosis. A minor illness can affect work or lead
  the employee to request leave; the manager sees only the submitted reason
  and review evidence. A coworker learns a symptom claim only after actually
  hearing voluntary co-located speech. Severe injury blocks the sample work
  task until recovery.
- Migration 046 extends F3 Human-gated shared rounds with private sleep
  start/end and work-task proposals. Only the selected, exact pinned child
  may cross the active decision window to its typed Event owner. Direct
  external health/work actions remain blocked. Round receipts contain only
  sequence/time and own disposition, never another resident's health truth.
  The migration transaction rebuilds the full round/participant/proposal FK
  chain, preserving populated F3 rows.
  Shared work proposal validation first checks that a supplied contract is
  scoped to the submitting employee, so external callers cannot distinguish
  a missing ID from another employee's contract through proposal errors.

## Verification ledger

| Gate | Evidence | Status |
| --- | --- | --- |
| Sleep→decision→recovery | Real source intervals, committed speech refusal, long-rest recovery, private provider input | PASS focused |
| Health→work/leave | Current-shift task outcome, unchanged attendance/pay, illness work versus employee-requested/manager-approved leave, injury guard | PASS focused |
| Lawful knowledge | Own symptom-only context, manager bounded reads, co-located speech claim, private Event absent from coworker feed | PASS focused |
| Shared authority/recovery | No Event before Human, direct bypass denial, exact child pin, restart after selection and after Event-before-receipt | PASS focused |
| Existing DB upgrade | Populated 045→046 settled speech and pending move preserved; reopen and `PRAGMA foreign_key_check` | PASS focused |
| Authenticated clients | `npm test` 4/4: MCP sleep start/end with two service residents/Human, work-tool schema/routing, opt-in live-driver wiring; HTTP work task through real employment shift | PASS focused; no separate MCP work success fixture |
| Race/static | Final-source focused shared storage+HTTP race (storage25.171s, HTTP12.207s), scoped vet, `git diff --check`, empty gofmt listing and Node syntax | PASS |
| Full Go regression | Final-source `go test ./... -count=1 -timeout=40m`: storage1358.342s, HTTP25.463s, all other packages PASS | PASS |

## Scope and remaining gate

The exemplar task is one bounded work sample, not a manager's evaluation or a
clinical performance prediction. A treatment reference is accepted only for
an existing matching treatment Event; F5 does not implement a hospital or
treatment workflow. There is no formal diagnosis command or implicit
diagnosis inferred from symptoms or leave text. The condition truth command
is local-operator-only and does not expose a direct external-model tool.

The MCP work tool uses the same generic authenticated route adapter as the
successfully exercised sleep tool. Its schema/routing are tested through MCP,
while the complete work effect is tested through authenticated HTTP and the
shared storage owner. A separate employed-resident MCP success fixture remains
a useful coverage extension, not a distinct unverified authority path. No
live external health model run, deployment or push is claimed. The containing
scoped commit is the local F5 checkpoint; verify clean-tree recoverability
before entering F6.
