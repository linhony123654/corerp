# RP-1A — Session / Presence / Observation

Status: PASS locally on 2026-09-23. This is not the full RP-1 playable loop.

## Implemented

- Migration 020 adds durable, idempotent RP application sessions with world/branch/controlled Entity binding, POV, observation/turn cursors and lifecycle. It does not copy character, location, economy, knowledge or world-time authority.
- `world.rp.control` is a player-only, entity-scoped grant. Open/read/resume/close/observe recheck identity; HTTP binds Principal from the Bearer authenticator. Invalid scope and cross-principal session reads fail closed.
- Observation derives current location, world time and same-place entities from the existing M2 projections inside one short SQLite transaction. Only names/IDs, place and observation cursor are exposed; no hidden assets, goals, knowledge, memory or debug trace.
- Opt-in `rp-prepare` extends the existing M2/T09 branch with Lin as a player and Cai as a third named NPC. Ada, Bo and the five established places remain. Lin/Cai have event-backed initial positions in `agent_movements`/`agent_positions`, not a parallel RP scene table.
- HTTP exposes POST open/read/resume/close/observe under `/api/v1/rp/`. Closing a session invalidates it; a new idempotency key creates a fresh session without resetting the world.

## Verification

| Check | Result |
| --- | --- |
| Full backend regression | PASS — `cd backend && /usr/local/go/bin/go test ./... -count=1` (final storage 45.076s; other packages passed) |
| Static analysis | PASS — `cd backend && /usr/local/go/bin/go vet ./...` |
| Related race | PASS — `go test -race ./internal/storage ./internal/transport/httpapi ./cmd/corerp-m2 -run 'TestRP|TestRPSchema|TestM2CLIRP|TestM2WagePolicyMigrationUpgradesExistingSplitLineage' -count=1` |
| Migration parity / 019→020 | PASS — embedded SQL equals `schema-020-sessions.sql`; disposable existing M2 data survives upgrade and can enter RP |
| Session/authorization/observation | PASS — targeted storage/HTTP/CLI tests cover retries, mismatches, invalid references, same/different-place presence, privacy, revoke, close and reopen |
| Replay / snapshot | PASS — RP fixture positions compare equal to existing event replay; snapshot continuation yields the same state hash |
| M2 evidence index | PASS — `npm run verify:m2-evidence` reports 26 executable test references |
| Actual-process smoke | PASS — `rp-prepare` produced head 7; HTTP process returned Lin at `place_m2_cafe` with Cai present and cursor 7; after process restart, `resume` returned the same session ID and cursor |

The `go run` wrapper returned exit 1 on interactive Ctrl-C during the process smoke while the application logged `CoreRP HTTP API stopped`; no request or persistence failure was observed. Baseline frontend `npm run build` passed before RP-1A; frontend source was not changed in this stage. No production database or external account was used.

## Recovery point and deferred

- Pre-RP Git baseline: `d24b083`; source archive `/home/ubuntu/corerp-rp1-baseline.RC6ULh/source-before-rp1.tar.gz`, SHA256 `a54e813b32b98de4fb7269c439a560105582c4b9531fe993f9811074e99c7c90`.
- RP-1A checkpoint: the Git commit containing this report; find via `git log -- docs/rp1/phase-a.md`. No destructive rollback performed.
- Deferred to RP-1B–F: player move/wait, speech/knowledge, NPC decisions, turn orchestration, narrative, real Play frontend and full restart E2E. Static local token mapping is not production identity/TLS.
