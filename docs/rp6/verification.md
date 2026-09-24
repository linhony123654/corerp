# RP6 final local verification

Verified source checkpoint: `55faee5e2c0355ddcd25ee5d1bbcb07ac72b4acc`, post-commit clean tree confirmed; baseline364976f (RP5 source47c5d15), schema026 unchanged. This record is updated only from terminal command results; a running check is not a pass.

| Required check | Command / evidence | Status |
|---|---|---|
| Full backend normal | `go test ./... -count=1 -timeout=30m`,26862; storage326.534s,HTTP15.060s,all other packages pass/no-test as declared | PASS |
| Full vet | `go vet ./...`, reached after successful full tests in26862, terminal exit0 | PASS |
| Migration/reopen/replay/recovery | Included in unfiltered normal suite above; existing source-compatible schema026, new personal views/provider/style tests include reopen/privacy/retry; actual browser midpoint/recovery evidence below | PASS |
| Core/narrative/server race | `go test -race ./internal/core ./internal/narrative ./cmd/corerp-server -count=1 -timeout=20m`,first command30674;1.464/1.167/2.321s | PASS |
| Relevant storage/HTTP race | `go test -race ./internal/storage ./internal/transport/httpapi -run 'TestRP(Wallet\|Work\|Contacts\|Messages\|Narrative\|Style\|Background\|Life\|Turn\|Session\|Move\|Wait)\|TestRPCurrentLocalTransit' -count=1 -timeout=60m`,30674;storage1750.004s/HTTP24.635s,terminal exit0 | PASS |
| Prior current-source map/shared-helper race | Increment map64692 storage8.150s/HTTP2.691s; own-employment affected4642 storage258.650s/HTTP4.321s; those implementations unchanged since their checks, plus current full normal suite above | PASS, retained scoped evidence |
| Full HTTP race | `go test -race ./internal/transport/httpapi -count=1 -timeout=20m`,42392;131.893s. Unfiltered transport regression beyond the selected RP prefix. Stream assertions are inside the selected style test, not separate omitted Narrative-named tests. | PASS |
| Frontend typecheck/build | `npm run build`,21433,vue-tsc then Vite2.62s | PASS |
| Configured verifiers | `npm run verify:m0`, `verify:m1-evidence`, `verify:m2-evidence`,21433; preserve their own bounded/deferred scope | PASS |
| Separate frontend lint/unit | No configured scripts in current package.json; no invented suite | N/A |
| Current all-increment browser | `verify-rp1-play.mjs --messages --map --work --contacts --wallet --style-ui --regenerate --turn-stream --context-budget --fake-model`,5307,33local decision calls, `/tmp/corerp-rp1-e2e-IMEzRX`; runtime unchanged since this final-source verification | PASS |
| Independent custom execution | `verify-rp1-play.mjs --custom-style --fake-model`,69531,5planner/24decision calls, `/tmp/corerp-rp1-e2e-dCa9QH`; closed output/recovery/canonical invariance | PASS, local HTTP fixture only |
| Work/message default harness regression | `verify-rp6-work-life.mjs --messages`,49291, `/tmp/corerp-rp6-work-Z3iZVx` | PASS |
| Actual104turn multi-day UI | Extended `verify-rp6-work-life.mjs --long-play`,78682, `/tmp/corerp-rp6-work-jxrRFf`; causal and visual details in [long-play.md](long-play.md) | PASS |
| Syntax/whitespace | Both long-play JS syntax checks; `git diff --check` | PASS |
| Live model language quality | No live provider configured/used; fixtures do not establish it | NOT VERIFIED, explicit availability limitation |

The storage race scope follows RP6 changed reads/presentation/shared employment context and associated background/life/session/turn/move/wait owners. Prior RP5 exhaustive race artifacts are historical, not a substitute for this current affected-scope check. Original stage requirement is relevant race, not a repeated unbounded fortnight race run.

Inventory confirmed33storage and10HTTP top-level tests match the selected filter, including60-turn life/reopen/replay,024/025upgrade, all new personal reads, style revision/budget/provider, durable turn and Wait failure recovery. The optional `TestRPCurrentLocalTransit` alternative matches no current top-level test; retained map race evidence is listed separately rather than silently treating that name as coverage. Current full HTTP race is unfiltered. Final host environment check prints booleans only: decision/narrative provider, endpoint, model and API-key fields all unset; no other credentials searched.

Pre-commit hygiene: modified/new Go files have no gofmt output; whitespace check passes; ignored local database/environment/build artifacts are absent from candidates. A bounded filename-only scan of new narrative/browser/UI/docs surfaces found no recognizable private-key or provider-key patterns; this is hygiene, not a general security certification.

Overall RP6 local verification gate: **PASS**. All required applicable checks have terminal successful evidence; no live test handles remain. Requirements audit and report are finalized with explicit capability/live-model limits. Authorized source checkpoint and clean-tree verification completed; no deployment. Next stage is RP7 recon.
