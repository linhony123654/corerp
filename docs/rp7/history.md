# RP7 — one observer's history across clients

## Corrected visibility contract

RP7's same-world clients must not present separate personal histories for one controlled character. The actual SillyTavern external-client test exposed the old behavior: RP observe selected settled turns and own actions only from the current session. The event subscription refreshed successfully but the other client's accepted speech was absent from the refreshed history.

`ObserveRPSession` now returns the latest50 settled entries for the authenticated session's **existing controlled entity, instance and branch**, irrespective of which client session initiated them. Dialogue uses settled turn views; own movement/wait/social actions remain narrow sourced descriptions; initiative history retains the existing observer/causal-session visibility constraints. No private raw Event/decision state is exposed. This changes the old session-local presentation contract deliberately to satisfy7D's single-world/single-character requirement; it does not merge different characters' histories.

Regeneration is still session-owned: only turns originating in the current session have `can_regenerate=true`, and the narrative endpoint independently checks its session/turn binding. The existing Play UI already honors that flag. Closed originating sessions do not erase historical world facts. No new table, migration, character, duplicated memory store or world write is introduced.

Storage coverage changes the former same-character/new-session empty-history expectation into exact shared IDs/text plus no cross-session regeneration. It retains foreign-principal rejection, adds a separately controlled Ada observer with no access to Lin's personal turn history, and calls the narrative endpoint to prove shared display does not broaden its permission.

## Late application settlement

World Events can finish before a turn's narrative application view settles. World sequence alone therefore cannot notify clients of every completed history view. Event pages/checkpoints now carry `history_revision`: the count of settled turn views for the same controlled observer/world/branch, bounded by snapshot head. It is an application-view invalidation marker, **not a new world Event, command cursor, timestamp advance or model invocation**.

Live SSE sends a checkpoint when this revision changes even if `next_sequence` does not. The encrypted continuation remains the scoped world sequence. Reconnect always sends an initial checkpoint and therefore refreshes the current view even at the same world head. Clients must process repeated-sequence checkpoints; they must not deduplicate them solely by decoded world sequence. The SillyTavern adapter already refreshes on complete checkpoints.

An actual interrupted turn test pauses after NPC world effects, reads the events while history is not yet settled, then resumes the original request. It proves unchanged world head/no new feed events, increased history revision, newly visible shared settled history, denied regeneration and rebuild-stable revision. A separate HTTP transport test controls only the revision marker over a real authorized temporary store to prove a second same-head checkpoint is delivered; it is not claimed as a second actual-turn integration test.

## Verification

- Shared-history initial normal95394 PASS2.423s. History/session/style/initiative/client race50535 PASS storage40.558s / HTTP8.885s, vet PASS, before the additive late-settlement marker.
- Final affected race57179 PASS storage11.997s / HTTP7.380s plus vet; real late-turn interruption/resume/rebuild and transport checkpoint included. FullHTTP/server69366 PASS16.106s /0.181s.
- Final installed-host browser68743 PASS `/tmp/corerp-rp7-extension-ak2N5w`: UI wait to02:04, two legal moves, real gift1minor with wallet/known-evidence checks, forbidden principal override rejected before persistence, second authenticated session's accepted dialogue appears without a refresh click, shared observer/world/time/head/facts equality and changed persisted checkpoint. Final head21, four player speeches including the external one. Existing reload/lost-response/save-failure checks still pass.

This is a real SillyTavern plus HTTP-client integration, not yet the required Play/SillyTavern/MCP three-client E2E. Process-restart and adversarial chat-switch checks, definitive rejected-command recovery and MCP/skill work remain open.
