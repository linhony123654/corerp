# CoreRP final architecture goal v3

## Objective
Execute `/tmp/f3aba99d-86cf-4b76-8ad6-c2e6390139e7.md` in strict F0→F13 order: preserve one authoritative, recoverable world while adding the specified interaction, spatial, resident, life-system, pack, QA, documentation and final long-run acceptance capabilities. No production release is implied.

## Route and stage gate
Full-project route. Only the current stage may be in implementation. Each stage: read-only source recon and reuse/gap map → representative vertical slice → targeted checks → full stage and relevant regressions → phase report → authorized checkpoint → clean recoverable tree → next stage. Do not claim absent live-provider evidence as passed. Preserve immutable Event authority, permissions, idempotency and replay. Resolve architecture-changing uncertainty before broad build.

## Current phase
F1 — unified interaction and sourced long-form display: local REQUIRED gates PASSED; the checkpoint is the commit containing `docs/f1/phase-report.md`. F0 passed at `f6b3df87ea9f41529998be14b1ef1224d37bfb10`. F1 backend, additive HTTP/MCP, Play, bounded narrative density, recovery, race, browser isolation and full Go regression are verified. Live provider is unavailable and remains `LIVE_VALIDATION_PENDING`. Verify the F1 checkpoint clean tree before entering F2.

## Phases
- [x] F0 — old goal ended; branch/HEAD/clean state, migrations, versions, clients, docs, Inspector/Studio, prior E2E/race/replay; 13-row reuse matrix; source-backed gaps; report/checkpoint `f6b3df8`; clean tree verified.
- [x] F1 — unified interaction modes and sourced long-form display. Representative backend slice, migration031, authenticated routes, Play/MCP adapters, recovery/long-fixture/two-world browser checks, focused race/vet and terminal full Go regression pass. Live quality pending; containing checkpoint and clean-tree audit finalize the stage.
- [ ] F2 — shared spatial topology, travel encounters, perception and lazy locations.
- [ ] F3 — MCP model residents, exclusive control and shared world time.
- [ ] F4 — household foundation.
- [ ] F5 — health contract.
- [ ] F6 — education, skills and qualification.
- [ ] F7 — information and communication network.
- [ ] F8 — organization agency.
- [ ] F9 — two real packs and author lifecycle.
- [ ] F10 — world QA and observer.
- [ ] F11 — static HTML manual and maintainer handoff.
- [ ] F12 — integrated long-run, clients and full engineering evidence.
- [ ] F13 — architecture freeze, final report and stop.

## Decisions and guardrails
- Baseline is `/home/ubuntu/corerp-console`, not the `/home/ubuntu` multi-project root.
- Prior `.planning/2026-09-23-living-world/` is historical evidence; this plan is the new goal's ledger.
- Checkpoint commits are authorized by the user-provided goal, after each stage gate; no push/deployment authorization.
- An unavailable live provider remains `LIVE_VALIDATION_PENDING`, never a fabricated COMPLETE.

## Errors encountered
- F0 source lookup guessed `backend/internal/storage/migrations.go`, which does not exist; actual schema constants live in `store.go` and migration SQL under `migrations/`. Corrected lookup scope.
- `vexor` semantic search could not reach its API (`OpenAI API request failed: Connection error`); stopped retrying and switched to scoped `rg` over known source modules. No repository code affected.
- F0 lookup guessed `backend/internal/storage/rp_narrative.go`, absent; actual narrative owner is split across `rp_narrative_*` files and `core/rp_style.go`. Corrected file inventory. Bare `go` is not on current PATH, but `/usr/local/go/bin/go` exists; use explicit binary for later stage checks.
- One F0 inspection wrapper had a JavaScript output-label typo (`cmd` instead of `cmds[i]`); the three read-only Git commands had executed, but their output was lost. Corrected the wrapper, reran inspection, and confirmed cached diff check PASS with exactly four F0 files staged.
- F1 looked for `rp_request_retire.go`/`core/rp_request*.go`, absent. Actual `rp_request.go` owns retirement and migration027 constrains operations. Located the correct owner without repeating the failed path.
- F1 HTTP compilation failed first because the `Service` interface had not been extended; adding its two interaction methods then exposed the test fixture's `*Store` implementation requirement. Added local deterministic Store wrappers while keeping product RPService's configured provider. Focused HTTP test then passed; no authorization bypass or weakened interface check.
- F1 long-fixture first run failed before interaction execution: 55 repetitions made only 990 Chinese characters, below its own 1000+ test precondition. Increased the synthetic fixture to 60 repetitions (within the existing 2000-character speech limit); focused test passed. This is a fixture correction, not a relaxed production check.
- F1 money/time invariance check initially passed a string snapshot to an `int64`-only test helper and failed compilation. Replaced it with a typed snapshot comparison; focused interaction tests rerun.
- F1 new wait→speech recovery test exposed a real orchestration fence bug: the completed typed wait's `event_sequence` preceded its own triggered warm/initiative Events, so the next step was falsely paused. Separated immutable child Event sequence from settled continuation cursor, accepting only contiguous warm/initiative Events tied to that wait trigger and fencing any unrelated Event. Focused interaction tests PASS; stale pre-fix full-suite run was explicitly cancelled, and current-code full suite/race were restarted.
- F1 review found a paused plan released the migration031 `one_open` index slot before explicit stop, potentially allowing another plan beside it. Changed the index to a single `open`/`paused` active slot and return a scoped in-progress error; added a regression assertion. Focused tests PASS. The already-running pre-change full suite was cancelled and restarted on the reviewed schema/code; do not count interrupted runs as passing.
- Terminal current-source `go test ./... -count=1 -timeout=40m` PASS (storage 1276.058s, HTTP 16.341s, remaining packages passed). Superseded earlier full runs were cancelled after subsequent source edits and are not counted. Later test-only assertions passed separately under focused checks/race; no production source changed afterward.
