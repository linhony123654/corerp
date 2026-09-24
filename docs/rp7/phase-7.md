# RP7 — shared-world client integration

Status: RP7 local DEFINE/DESIGN/BUILD/POLISH/SHIP verification PASS. Baseline879dbc0
(RP6 source55faee5), current schema027 `corerp-rp7-request-retirement-027-2026-09-24`.
Source checkpoint: `c39af6b141b6898eb643799bf380c215c77a1957`; post-commit
`git status --porcelain` was empty. No publication or live-model claim.

Delivered: [frozen client protocol/discovery](protocol.md), [authorized context](context.md),
[events](events.md), [atomic request retirement](requests.md), actual thin
[SillyTavern](sillytavern.md), [MCP + RP skill](mcp.md), and
[three-client compatibility](compatibility.md). [Final gate evidence](verification.md)
records terminal results, prior failed attempts and coverage limits.

## Original acceptance

| Requirement | Intended evidence |
|---|---|
|7A stable minimum client protocol: open/resume, observe, command, context, event stream, permitted character context|Versioned documented request/response contracts and actual authenticated transport tests; clients use HTTP, never database writes.|
|7B thin SillyTavern adapter: binding, context injection, command bridge, streaming, minimal settings|Actual extension integration against supported host APIs, real service commands/recovery; host retains no economy/memory/event/character authority.|
|7C MCP + RP skill: world/session, observe, command, dialogue, wait, permitted context; call order/RP/rule boundaries|Executable MCP protocol/tools with permission/retry tests; runtime-oriented skill instructions, not a replacement simulator.|
|7D Play/SillyTavern/MCP share one instance and characters|Cross-client service/browser end-to-end writes/reads/restart; same event head/clock/identity, no independent NPC copies.|

## Final requirement audit

| Requirement | Result and actual evidence |
|---|---|
|7A|PASS: authenticated open/read/resume, observe, typed commands/turns/wait, bounded permitted context and JSON/SSE event protocol. Discovery and session control revalidation tested; no client database access. Cursor isolation, late-history checkpoints and all-five-owner atomic retirement/replay have HTTP/storage/recovery evidence.|
|7B|PASS: actual pinned SillyTavern1.19.0 installed extension, binding/context/commands/SSE/settings, host save readback, accepted-response loss, dual process restart, retirement, budget wait and accepted-write chat-switch isolation. Tokens remain memory-only; presentation card is not a runtime character.|
|7C|PASS: executable official-SDK MCP stdio server with12 closed tools; actual protocol client and SQLite runtime tests, permissions/cancellation/bounds/restart/exact retry. RP skill structurally validated and checked against real tool fields. No nested Codex/AI CLI or live-model evaluation.|
|7D|PASS: actual Play UI, actual SillyTavern UI and actual MCP stdio client,3 distinct sessions controlling one Lin in one instance. Final49071:8 distinct speeches, head35, same time/place/history/known facts and entity lineage, Runtime/client restart, exact old MCP retry with no duplicate world effects.|

Deferred, not silently claimed: group chats, automatic reconnect/backoff, exhaustive
private-memory/economic event views, arbitrary administrative commands, every social
variant in a browser, all power-loss/interleaving cases and live LLM quality. MCP
hosts must durably retain original calls; this adapter is not a pending-intent DB.
No full-storage race claim: relevant filtered race plus full normal suite passed.

Recovery: use this RP7 source checkpoint with schema027 and the same configured
cursor secret; resume original keys/pending intents. Schema027 is additive and
tested from026; do not downgrade or delete a live database to emulate Git rollback.
Next is RP8 read-only recon after checkpoint and clean-tree verification. The full
goal remains active through RP8 and Final Integration (300 turns /30 world days).

## Historical recon and increment design (superseded by final audit)

SillyTavern official-interface research and thin-client design are in [sillytavern.md](sillytavern.md). Direct browser access required an explicit origin boundary: optional exact-origin server flag now preserves bearer/capability checks and keeps CORS disabled by default. This is a verified transport prerequisite, not yet an installed extension or completed7B.

Existing HTTP authentication binds principal identity; RP sessions bind existing individual and active control grant. Typed actions/turn orchestration already own optimistic cursors, idempotency and settlement. Existing world-event SSE is permission-filtered with signed principal/world cursors; narrative NDJSON is a separate fact-attributed presentation stream. Personal read endpoints and observe already expose bounded player-visible state. Do not add parallel world/session/character databases.

Further source inspection identified an important reuse limit: `events.go` implements M1 economic visibility (`economic_entities` employee/household account and obligation IDs), while the RP fixture grants only `world.rp.control`. That SSE owner does not yet define RP heard/seen/own-action visibility. Reuse framing/deadline/cancellation/cursor mechanisms, but design and test a genuinely session-authorized RP event projection before claiming7A event-stream coverage; do not hand clients a creator token or broadly expose raw Events.

`ReadAgentKnowledge` is a different capability/field-scoped metadata reader, not an already suitable prompt context endpoint. Internal `RPDecisionInput`/Life Context may contain private NPC goals and authority details and must never be serialized wholesale. No existing SillyTavern adapter, MCP server or client protocol package was found in the inspected project file inventory.

## First vertical slice: authorized known context

Implement authenticated `POST /api/v1/rp/context/read`: own active session/live control/binding checks in one read snapshot, no arbitrary observer override. Optional `subject_entity_id` restricts **what the controlled observer knows about a subject**, never reads the subject's private mind. Unknown/unheard/out-of-scope subjects yield an empty list without revealing whether that subject exists.

Return protocol marker `corerp.client.v1`, actual session/instance/branch/observer, snapshot world time/head and at most50latest sourced knowledge facts (`limit` default20,1–50). Only personally observed presence, literally heard speech and witnessed interpersonal actions are admitted. Narrow fields retain type, subject, historical place/time and source-event ID; speech remains an attributed claim, not verified content truth. Raw knowledge/decision payloads, account data, private Career evaluations and hidden remote locations are excluded. `more_facts` reports truncation; this first slice does not promise exhaustive memory retrieval. Observe and context are separate snapshots; clients must compare world/head before combining them.

Acceptance: real hearing/social/movement produces exact visible evidence; outsider/forged principal/revoked control/closed session denied; unknown subject and private claim types absent; actual multi-session reads of one controlled observer agree apart from session binding; reopen/rebuild preserve source order/content; reads leave events/clock/session cursor untouched; bounded result and `more_facts` tested. No schema, new grant owner, model call or client-specific memory authority.

The next event slice now adds own active-session JSON/SSE reads, historical observed evidence plus narrow own speech/move/wait, version-isolated session/observer-bound encrypted cursors, safe checkpoints and per-poll reauthorization. Real SQLite recovery/repeated encounters and HTTP streaming/close evidence are in events.md. It deliberately does not expose raw private Events or claim every personal transaction subtype is already represented.

These slices do not finish7A. Next: freeze command/envelope/error/recovery/discovery/context/stream contracts and adapter refresh rules after representative tests; independently inspect current official SillyTavern/MCP interfaces before adapter design; add actual adapter and MCP/skill slices, then same-world compatibility E2E and full-stage gates. Never substitute documentation or mocked host APIs for final compatibility evidence.
