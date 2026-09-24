# RP7D — actual three-client shared-world compatibility

PASS for the defined integration scenarios; full-stage verification is tracked
separately in [verification.md](verification.md).

## Real clients and one authority

`verify-rp7-sillytavern.mjs` now composes the installed pinned SillyTavern1.19.0
extension, an actual Vue Play browser page, and an official SDK client spawning
the actual MCP stdio adapter. All reach one built Go HTTP Runtime/temporary SQLite
world. Vite4189 proxies that same Runtime4188; the owned test server permits only
the two exact browser origins4187/4189. No production proxy/CORS configuration is
changed, no fake host global or HTTP-only substitute stands in for a client.

Three independently opened session IDs bind the existing `entity_m2_rp_lin` in
`inst_m2_t09/br_main`. Discovery supplies the MCP binding. Opening the sessions
must not alter the full materialized entity ID/source-cohort/population list.

## Cross-client actions and observations

1. Speak through actual Play UI; installed extension automatically refreshes through
   its event feed, MCP observe sees the same accepted speech.
2. Speak through actual SillyTavern UI; Play refresh and MCP observe show it.
3. Speak through actual MCP tool call; both actual UIs show it.
4. MCP moves along an observed available route to Ada Home and explicitly waits
   one minute. All three observations agree on controlled identity, presence,
   place, world time, event head and ordered history IDs/narration. Regeneration
   permissions remain source-session-owned and are intentionally not equated.
5. MCP permitted context and the extension's actually injected context agree on
   world/branch/observer/time/facts. The Play scene/header/history are inspected in
   the real rendered page. Play uses explicit refresh, not an unimplemented live
   event subscription.

Close MCP and reload both pages, releasing streams and memory credentials; stop
and restart Runtime with the same DB/cursor secret and a new PID. The authoritative
head/time/event/entity snapshot must not change. Re-enter tokens, resume the same
three session IDs, replay the original MCP dialogue unchanged, and compare again.
The same people/history remain; no duplicate action or independent NPC copy appears.

## Adverse host boundaries

The same harness retains real lost-response/graceful dual-host-runtime restart,
metadata-save failure, retirement, budget-wait and delayed-read chat-switch tests.
An additional scenario withholds a real already-committed dialogue response while
switching to a new actual host chat. Client cancellation and scope guards leave the
new chat unbound with no old prompt/token/scene and normal native generation.
Returning to the original card finds its exact persisted pending request; explicit
reconnect/retry resolves it without changing the post-commit authority snapshot.
This is not a claim of every possible save/write scheduling interleaving or power
loss; it proves the representative accepted-write switching boundary.

## Evidence

- Initial93008 FAIL at Play entry: fixture origin allowlist lacked4189. Corrected
  exact test-origin configuration and asserted actual HTTP open success. No
  product authorization relaxed; no failed run counted as passing.
-18640 PASS `/tmp/corerp-rp7-extension-rNHUfk`: three clients,7distinct player
  speeches, head33, world time2026-09-23T08:01:00Z, AdaHome; restart/retry invariants.
- Final49071 PASS `/tmp/corerp-rp7-extension-5zKKA5`: adds accepted-write chat switch;
  exactly8distinct player speeches, head35, same time/place; all prior assertions
  retained. All browsers/MCP/Vite/host/Runtime processes terminal, ports4187–4189
  empty. Play screenshot from18640 inspected; no browser page errors and390px
  extension submit-width bounds passed. No nested Codex/AI CLI was launched.

The existing `actions.head=21` output is an earlier action-phase checkpoint, not the
final head. Multiple session IDs are deliberate; multiple world characters are not.
Fixtures use deterministic providers, not evidence of live-model literary quality.
