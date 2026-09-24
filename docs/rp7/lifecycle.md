# RP7 — actual host lifecycle and bounded-wait recovery

Status: the scenarios below PASS against the pinned real SillyTavern1.19.0 host and actual temporary CoreRP SQLite runtime. They do not complete7A/7B/7D or the full goal.

These lifecycle assertions also pass in the newer schema027 request-retirement
run32340, `/tmp/corerp-rp7-extension-cJ6RJP`. See [request recovery](requests.md)
for additional real invalid-open/rejected-wait/lost-fence/host-save evidence.

## Dual process restart after a lost accepted response

The integration harness commits the second player speech, deliberately loses its HTTP response and retains the exact pending request in host chat metadata. It closes the page to release SSE/discard memory credentials, sends TERM to both owned servers, awaits their terminal states, and starts new host/runtime PIDs against the same data/configuration and cursor secret. The authoritative world head, time, event count and materialized-entity count must remain exactly equal through startup.

A fresh page reopens the same actual presentation card. The pending request is read from persisted host data, not reconstructed from an in-memory copy; it must equal the original request including its idempotency key. Token input and injected context start empty. Explicit reconnect and original retry resolve the already accepted speech without increasing the player-speech count. Earlier ordinary reload, save-failure and typed-action checks remain in the same run.

This is a graceful process restart test, not a SIGKILL/power-loss or disk-corruption claim.

## Delayed old-chat context response

`rp7-chat-switch-checks.mjs` obtains a real context response for the connected chat and withholds delivery. While it is in flight, the real host creates/selects a different unbound presentation card. Only then is the old response released. The new chat retains no prior binding, injected prompt, scene text or memory token, and native generation is not blocked by the old binding. Re-selecting the original card preserves its session binding but still requires a token/reconnect to restore context. Authoritative world state is unchanged by all these host operations; no duplicate runtime character is materialized.

This tests a delayed read and subscription cancellation boundary. It does not yet prove every possible chat-switch timing during an accepted write or host-save failure.

## Wait budget exhaustion

After earlier short-wait/move/gift/external-client scenarios, the actual extension submits a wait to the existing fixture's next morning `2026-09-23T08:00:00Z` with budget1. The first actual HTTP response must be `budget_exhausted`, report one processed item and positive pending work. The extension retains its original request. Every subsequent UI retry is compared with that exact request; it may not change expected cursor, target, budget or key as the scheduler progresses.

In the recorded run one original retry completed the wait. SQL read-only checks verify two total completed wait Events (the previous short wait plus this one), zero pending wait intents and four player speech Events. This is not a claim of a new speech or a thirty-day acceptance run. The earlier action phase reports head21; later scheduled work advances beyond it, so21 must not be described as the final run head.

## Evidence and remaining work

-68413 PASS `/tmp/corerp-rp7-extension-hLoH1a`: actual dual process restart plus prior recovery/action/stream checks.
-89032 PASS `/tmp/corerp-rp7-extension-n33iAd`: adds delayed-response chat isolation and original-binding recovery.
-Final96783 PASS `/tmp/corerp-rp7-extension-iKXSeU`: all preceding scenarios plus explicitly asserted real budget-exhausted response and unchanged-request retry. Browser/runtime/host cleanup completed. Script syntax and diff-whitespace checks PASS. No production or real-account data used.

This increment changes the verification harness, not runtime code; previous relevant backend race/vet evidence remains applicable. The actual three-client Play/ST/MCP run, full protocol freeze, definitive rejection recovery and MCP/skill delivery remain required. In particular, do not clear an ambiguous request or infer non-commit from a failed HTTP response or an absent receipt snapshot.
