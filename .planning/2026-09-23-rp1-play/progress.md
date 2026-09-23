# RP-1 progress

## 2026-09-23 — baseline / discovery
- Read complete RP-1 goal and selected full-project workflow.
- Inspected existing project planning and Git state; new isolated RP-1 plan activated.
- No business source modification or baseline test yet.
- Semantic discovery attempted once; provider connection failed. Switched to scoped local search.
- Created and hashed pre-RP source archive at `/home/ubuntu/corerp-rp1-baseline.RC6ULh/source-before-rp1.tar.gz` (SHA256 `a54e813b32b98de4fb7269c439a560105582c4b9531fe993f9811074e99c7c90`).
- Read RP-0 audit, relevant v0.5 contracts and M2 docs/schema/tests; mapped existing Agent/Encounter identity and missing RP surfaces.
- Baseline uncached full Go tests, Go vet and frontend build passed. Related Agent/HTTP race is running.
- Baseline Agent/HTTP race passed; initial Git commit `d24b083` captures the pre-RP tree. `git diff --cached --check` reported only three pre-existing whitespace issues, left unchanged.
- RP-1A first slice implemented and targeted tests passed: session binding, idempotency, co-location, privacy, restart, HTTP auth, real T09 player/third NPC fixture, CLI preparation and 019→020 migration. Existing replay/projection and snapshot continuation also passed for the new fixture.
- A combined docs patch failed atomically because root README starts with a longer title than assumed; reread the exact anchor and applied documentation in separate patches. No partial doc changes from the failed patch.
- Full Go, vet and targeted race are running against the final Phase A code. `npm run verify:m2-evidence` passed (26 executable test references); no frontend source changed in Phase A.
- RP-1A final full uncached Go suite, vet and targeted race passed. A real CLI/HTTP process smoke on an isolated database opened and observed Lin/Cai, stopped the server, restarted it, and resumed the same session/cursor. Interactive `go run` wrapper exited 1 on Ctrl-C although application logged graceful stop; record this wrapper nuance, not as a request failure.
- Wrote `docs/rp1/phase-a.md`; stage checkpoint commit is next, then automatically begin RP-1B.
- Final pre-checkpoint review tightened session idempotency ordering and revalidated the bound entity on reads. Repeated full tests, vet and related race after that change: all PASS (storage 45.076s; race storage 10.681s). No phase gate was carried forward from stale evidence.

## Verification ledger
| Check | Result | Evidence |
|---|---|---|
| Project Git status | PASS | `main`, no commits; project-local `.git` exists |
| Baseline source snapshot | PASS | Archive read/list count 212; SHA256 recorded above |
| Baseline Go tests | PASS | `cd backend && /usr/local/go/bin/go test ./... -count=1`; all packages passed, storage 43.376s |
| Baseline Go vet | PASS | `cd backend && /usr/local/go/bin/go vet ./...`; no findings |
| Baseline Agent/HTTP race | PASS | `go test -race ./internal/storage ./internal/transport/httpapi -run 'TestM2Agent|TestM2Runner|TestAgentHTTP' -count=1`; storage/HTTP passed |
| Baseline frontend build/typecheck | PASS | `npm run build`; `vue-tsc --noEmit` and Vite build passed |
| Baseline frontend lint/test | N/A | No lint or test script in current `package.json` |
| RP-1A targeted storage/HTTP/CLI | PASS | `go test ./internal/storage ./internal/transport/httpapi ./cmd/corerp-m2 -run 'TestRP|TestM2CLIRP|TestEmbeddedMigrationsMatchContracts' -count=1` |
| RP-1A 019→020 upgrade and M2 wage upgrade regression | PASS | Focused `TestRPSchemaUpgradeFrom019PreservesExistingM2World` and `TestM2WagePolicyMigrationUpgradesExistingSplitLineage` |
| RP-1A replay/snapshot | PASS | `TestRPPlaySetupMaterializesPlayerAndThirdNPCInExistingWorld` compares projections and snapshot replay |
| RP-1A final full Go tests/vet/race | PASS | Final full storage 45.076s; vet clean; related race storage 10.681s, HTTP 3.288s, CLI 2.740s |
| RP-1A actual process restart | PASS | `rp-prepare` head 7; POST open/observe; server stop/restart; POST resume same session/cursor 7 |
