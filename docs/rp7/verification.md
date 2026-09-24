# RP7 stage verification — PASS

Baseline879dbc0 (RP6 source55faee5), current schema027. All required local stage
checks are terminal PASS. Checkpoint identity is recorded in the phase report.

| Gate | Current command / evidence | Status |
|---|---|---|
| Full backend and vet |18487 `go test ./... -count=1 -timeout=30m && go vet ./...`; storage322.433s, HTTP21.773s, CLI8.321s, server0.282s, core0.208s, decision0.398s, narrative0.201s; combined exit0|PASS, terminal|
| Relevant race |22251 `go test -race ./internal/core ./internal/storage ./internal/transport/httpapi ./cmd/corerp-server -run 'TestRP(Client\|Discovery\|Request\|PlayObservation\|Session\|Move\|Wait\|Turn\|Social\|Schema\|Cursor)\|TestBrowserOrigin' -count=1 -timeout=30m`; storage128.347s/HTTP37.594s/core1.054s/server1.105s|PASS, terminal|
| Migration/reopen/replay |026→027 replay-hash check34467 and owner/migration race78947 PASS; final unfiltered suite18487 PASS|PASS|
| Frontend typecheck/build and existing evidence verifiers |44600 `npm run build` (vue-tsc+Vite4.94s), M0/M1/M2 verifiers,5browser transport tests|PASS, terminal|
| Actual Play/SillyTavern/MCP same world |Final49071 PASS `/tmp/corerp-rp7-extension-5zKKA5`:3independent sessions,8distinct speeches, sharedLin/time/place/history/facts, Runtime+client restart, accepted-write chat-switch isolation; [details](compatibility.md)|PASS|
| MCP protocol/recovery |43505 actual legacy and explicit2026-07-28 stdio→realRuntime,2tests|PASS, current runtime/adapter source unchanged|
| Skill artifact |quick_validate; actual tool-name/field manual review|PASS structural/content review; no nested-agent/live-model claim|
| Frontend generic lint/test script |No such script in current package.json; dedicated browser/client checks above|N/A, not configured|

Initial three-client93008 failed at Play entry: the fixture's exact-origin allowlist
only contained4187 while the added Play proxy page uses4189. Added that exact origin
to the fixture server launch only, and assert the real Play open response before
waiting for the scene. No wildcard or application permission change; failed run
not counted as a pass. Cleanup closed all fixture servers.

Earlier incremental evidence remains in context/events/history/requests/mcp and
lifecycle reports; it is not a substitute for the current full-stage commands.
No production release, push, global configuration change or live LLM evaluation.

Read-only final hygiene: gofmt reports no changed Go formatting; git diff whitespace
passes; bounded filename-only private-key/provider-token scan of new client/skill/
RP7 surfaces reports no matches. This is not a general security certification.
All fixture browser/host/Runtime/MCP/Vite processes and required verification
commands have finished. No remaining live verification handles.
