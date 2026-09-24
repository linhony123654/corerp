# CoreRP final architecture goal v3

## Objective
Execute `/tmp/f3aba99d-86cf-4b76-8ad6-c2e6390139e7.md` in strict F0→F13 order: preserve one authoritative, recoverable world while adding the specified interaction, spatial, resident, life-system, pack, QA, documentation and final long-run acceptance capabilities. No production release is implied.

## Route and stage gate
Full-project route. Only the current stage may be in implementation. Each stage: read-only source recon and reuse/gap map → representative vertical slice → targeted checks → full stage and relevant regressions → phase report → authorized checkpoint → clean recoverable tree → next stage. Do not claim absent live-provider evidence as passed. Preserve immutable Event authority, permissions, idempotency and replay. Resolve architecture-changing uncertainty before broad build.

## Current phase
F0 — old-goal handoff and actual baseline (`complete`, checkpoint pending). Source/report/artifact checks confirm old Final completion, clean entry baseline, schema030 and all 13 reuse categories. Do not start F1 implementation until F0 checkpoint and clean-tree verification.

## Phases
- [x] F0 — old goal ended; branch/HEAD/clean state, migrations, versions, clients, docs, Inspector/Studio, prior E2E/race/replay; 13-row reuse matrix; source-backed gaps; report written. Checkpoint and clean-tree verification are the final gate action.
- [ ] F1 — unified interaction modes and long-form narrative.
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
