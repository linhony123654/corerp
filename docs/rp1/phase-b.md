# RP-1B — Player Action / Move / Wait

Status: PASS locally on 2026-09-23 after final regression. This is not the full RP-1 playable loop.

## Implemented

- Migration 021 declares event-backed, directed place links for the bounded M2/RP fixture. `rp-travel-prepare` adds eight cafe↔home/work links to the existing five places. The links govern reachability only; `agent_positions` remains location authority.
- Player move validates session control, fresh observation cursor, origin, active destination and declared route. One committed `RPPlayerMoved` batch includes an existing Agent movement/schedule lineage, position projection, co-location knowledge, world clock lineage, Branch Head, audit and Outbox. Invalid shortcuts and stale state are rejected; retry does not create a second movement.
- Migration 022 stores a narrow, durable RP wait intent. The existing M2 scheduler executes due work within the supplied budget; `budget_exhausted` is partial, not completion. Once no due work remains, an `RPWaitCompleted` batch atomically advances the existing world clock, day, Branch Head, audit, Outbox and intent status. Same-key retries resume after restart or return the committed event; changed requests conflict. While a wait is pending, another RP wait/move on that branch and closing its session are refused.
- Authenticated HTTP routes `/api/v1/rp/actions/move` and `/api/v1/rp/actions/wait` bind the Principal at the gateway and return existing envelope/error semantics. Neither the browser nor session metadata becomes a second time or location authority.

## Verification

| Check | Result |
| --- | --- |
| Full backend regression | PASS — `cd backend && /usr/local/go/bin/go test ./... -count=1` |
| Static analysis | PASS — `cd backend && /usr/local/go/bin/go vet ./...` |
| Related race | PASS — `go test -race ./internal/storage ./internal/transport/httpapi ./cmd/corerp-m2 -run 'TestRP\|TestM2CLIRP\|TestEmbeddedMigrationsMatchContracts\|TestM2WagePolicyMigrationUpgradesExistingSplitLineage' -count=1` |
| Migration parity / upgrades | PASS — embedded 021/022 SQL byte-equals docs; disposable 019 and older M2 database upgrades preserve world state |
| Move/recovery | PASS — cafe→home changes observation and knowledge; unreachable shortcut, retry mismatch and precommit failure do not add unintended movement; reopen and projection comparison pass |
| Wait/recovery | PASS — no-due time advance emits one event; budget-1 due work is partial and resumes after database reopen; precommit failure leaves one recoverable intent; completed retry is stable; projection comparison passes |
| HTTP scope | PASS — non-player token cannot use session actions; player token moves/waits and observes committed world state |
| M2 evidence preservation | PASS — `npm run verify:m2-evidence` still finds 26 executable tests |
| Working tree formatting | PASS — `git diff --check` |

No frontend source or production world was changed in this stage. The existing Story frontend is still a fixture, not a Play surface. This RP-1B path is intentionally bounded to the local M2 instance/branch and one player; it does not provide a concurrent multiplayer scheduler lock or production identity system. A separate privileged runner advancing world time beyond a pending target can cause the wait to conflict rather than falsely completing it; operational reconciliation belongs to the later orchestrator stage.

## Recovery point and next stage

- Previous RP-1A checkpoint: `6978888`.
- RP-1B checkpoint: the Git commit containing this report; find via `git log -- docs/rp1/phase-b.md`.
- Next: RP-1C accepted speech, audibility evidence and listener Knowledge in one authority commit, then NPC decisions and continuous turn orchestration. No full RP-1 DoD is claimed here.
