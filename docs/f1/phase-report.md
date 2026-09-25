# F1 phase report — unified interaction and sourced long-form display

Status: **locally COMPLETE; live narrative-quality validation pending**. F0
source checkpoint: `f6b3df87ea9f41529998be14b1ef1224d37bfb10`.
The F1 checkpoint is the commit containing this report. No release or
production change is claimed.

## Delivered boundary

- One authenticated SQLite world remains authoritative. Migration031 stores
  only application plans, child request identity/progress, session interaction
  preferences and request fences. Existing typed Move, Wait and Turn paths own
  all world Events, scheduler work, Knowledge and money changes.
- A bounded deterministic interpreter handles speech, observed-place movement,
  explicit wait, and ordered move/wait→speech. `AUTO`, `DIALOGUE`, `SCENE` and
  internally resolved `MIXED` do not form another RP engine. Explicit request
  mode beats persisted session default, which beats AUTO; accepted plans pin
  the result and all child keys. Ambiguous `继续` asks for clarification without
  changing the world.
- Interrupted mixed requests resume their exact persisted child requests.
  Changed payload/scope is denied; intervening world change pauses the remaining
  plan. Stop cannot undo committed effects or conceal an accepted unreconciled
  child. Unaccepted original/child keys can be fenced by their respective
  retirement owners. Wait-triggered warm/initiative Events are included in the
  wait's settled continuation cursor only when their trigger matches; unrelated
  branch Events still pause the next step. An `open` or `paused` plan holds one
  active slot per session until settled/stopped; response loss after any typed
  child is recovered from its pinned request.
- Narrative density is an optional presentation field layered onto the
  existing pinned style. Sparse scenes are not padded, and long attributed
  source-rich fixtures exceed 1000 Chinese characters. Reading, regeneration
  and streaming remain read-only over committed Events/authorized Observation.
- Additive HTTP and MCP routes are documented in [protocol.md](protocol.md).
  Play adds an opt-in unified composer path and density selector, preserving
  legacy direct speech/map/wait behavior and the existing life-journal layout.
  SillyTavern's frozen v1 adapter was not reinterpreted.

## Verification ledger

| Gate | Evidence | Status |
| --- | --- | --- |
| Parser / storage / migration030→031 / exact recovery / retirement / modes / long fixture | `go test ./internal/core`; focused `TestRPInteraction*` storage and HTTP, including reopened DB and Compare→Rebuild→Compare | PASS |
| Concurrent-path checks | Focused Go `-race` for storage and HTTP interaction tests; two simultaneous exact-key callers, repeated three times under race; scoped `go vet` | PASS |
| Existing backend full regression | `go test ./... -count=1 -timeout=40m`; storage 1276.058s, HTTP 16.341s, other packages PASS | PASS |
| Frontend typecheck + production build | `npm run build` after final Play layout | PASS |
| Actual Play browser | Legacy flow, `--interaction`, `--style-ui`, `--turn-stream` | PASS; synthetic lost replies, process/browser restart, no duplicates, 1000+ display/recovery |
| Two-world late-stream isolation | `verify-rp8-play-worlds.mjs --interaction-isolation` | PASS; pending blocks switch, navigation aborts late stream, original binding recovers, other world stays clean |
| Actual MCP stdio client → HTTP world | `clients/mcp/npm test` | PASS 2/2; old and additive tools, mixed order, exact resume |
| SillyTavern compatibility | `node --test clients/sillytavern/client.test.js` | PASS 5/5 transport; real host fixture not rerun for F1 |
| Live provider 1000+ prose | No configured live narrative provider/credential in this environment | LIVE_VALIDATION_PENDING, not a pass |

The terminal full Go run used the reviewed production source. Later test-only
assertions (speech-child loss, simultaneous exact callers and density variants)
passed separately under focused normal/race checks; they do not alter that source.

The 1000+ fixture demonstrates transport length, immutable quotation,
attribution and restart recovery. It does **not** prove a model can produce
literary long-form prose, nor that sparse facts should be inflated to a length
target. The action grammar is intentionally limited; arbitrary natural-language
commands are not authorized to mutate the world. No F2 timed journey or F3
resident lease is included in this stage.

F1 local REQUIRED gates are PASSED. Its live-provider condition is explicitly
pending, not silently counted as a pass. After the containing checkpoint and
clean-tree verification, enter F2; no F2 implementation is part of this report.
