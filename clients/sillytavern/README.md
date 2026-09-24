# CoreRP SillyTavern adapter

Installable thin browser extension, verified against pinned real SillyTavern1.19.0 for binding, context, dialogue, wait/move/gift, streaming, retirement, lost-response/restart recovery and chat-switch isolation. Actual Play/SillyTavern/MCP same-world acceptance passed. Single-character chats and explicit reconnect only; not every social variant or power-loss interleaving is browser-tested. See [integration evidence](../../docs/rp7/compatibility.md) and [stage checks](../../docs/rp7/verification.md).

## Install and use locally

Copy this directory to your trusted host's `data/<user-handle>/extensions/corerp-runtime` and reload the host. The extension uses its `manifest.json`, `index.js`, `client.js` and `style.css`; no server plugin or frontend build is needed. Configure CoreRP with `-browser-origins` equal to the host browser's exact origin (for example `http://127.0.0.1:8000`), preserving existing bearer authentication. HTTPS is required except for local HTTP runtime origins.

Open a single-character chat, then the host Extensions settings drawer. Enter the runtime origin and a player-scoped token. Supply an existing session ID, or expand the new-binding section and provide existing CoreRP instance/branch/controlled-entity IDs. The presentation card does not create a runtime NPC. Tokens clear from the input after connecting, are never saved and must be entered again after reload/chat switch.

The panel shows real recent world narration and injects bounded personally known context. Use “提交到世界” to speak. Native host generation is explicitly aborted while the chat is bound; independently generated prose is not accepted as world truth. Waiting/movement/social options submit typed JSON to existing runtime routes, without allowing principal/session/cursor/key overrides. Actual UI tests cover a one-minute wait, legal return journey,1minor gift and a budget-exhausted next-morning wait completed with the original request; not every social action is covered yet. Obtain parameters from authoritative observe/action contracts, never invent destinations or advances. Settled history is shared by the same controlled character across clients; regeneration remains session-owned.

“原样重试” reuses the persisted exact request and idempotency key after ambiguous failure. Metadata is read back from the host before any command is sent because the host's save API can catch errors without rejecting. Do not alter pending metadata or delete a pending command to make a retry succeed. “停用未接受请求” asks the server to permanently fence an unaccepted original key; only a confirmed `retired` response permits clearing pending metadata. Accepted/in-progress work remains pending for exact recovery, never world rollback. Lost retirement replies and failed acknowledgement saves preserve the key for another retirement attempt. After reload, enter a token and use retirement directly if you do not want to retry a failed open. See [request contract](../../docs/rp7/requests.md). “解除本地绑定” is available only with no pending request and does not roll back the world or close the server session. Group chats are not supported.

Chat changes clear the prompt and memory token and cancel the old subscription. Reopen the same chat and reconnect to resume its binding. No token-streamed prose is promised: streaming is the CoreRP fact/checkpoint feed followed by authoritative refresh. A stopped stream requires explicit reconnect; automatic backoff/recovery is not yet provided.

Actual tests now include graceful restart of both host and runtime after a lost accepted response, and delivery of an old context response after switching to a different chat. See [lifecycle evidence and limits](../../docs/rp7/lifecycle.md). An unfinished budget-limited wait remains pending; use the original retry rather than submitting a new wait.

`client.js` has no database, model, memory authority, credential persistence or automatic command retry. The adapter supplies a token in memory, calls explicit existing RP operations, and processes `rp_event`/`rp_checkpoint` frames. It retains a pending command and idempotency key before submitting, persists a stream cursor only after processing its complete checkpoint, and aborts an old subscription on chat/binding changes. Losing a reply is not permission to resend under a new key.

The transport permits HTTPS origins and local HTTP origins only, omits cookies, refuses redirects, keeps bearer tokens out of URLs/serialized state, and does not reflect remote error bodies into prompts. SSE parsing handles split UTF-8/CRLF/multiline data, rejects malformed/oversized/incomplete frames, and releases the stream on cancellation. It does not simulate browser CORS; actual browser testing remains necessary.

From the CoreRP project root:

```sh
node --test clients/sillytavern/client.test.js
```

These five tests use controlled transport responses. They prove parsing and transport behavior, not actual host or runtime compatibility.

The real fixture integration is `node scripts/verify-rp7-sillytavern.mjs` from the project root, after preparing the pinned host at the documented temporary location. It builds a real temporary CoreRP SQLite runtime, installs the extension in the disposable host, exercises actual host APIs/browser UI and simulates lost responses/storage-save failure without replacing either runtime. It uses loopback4187/4188 and closes its servers/browser. The script currently has a fixture-specific host path; it is not a general-purpose installer.

## Host fixture

`host-fixture.json` pins official npm package1.19.0, published git head and archive SHA512 integrity. The separately inspected release branch head differs; do not label tests against the npm artifact as tests against that branch head. The archive was retrieved with `npm pack sillytavern@1.19.0 --ignore-scripts` in an isolated temporary directory, checked against the pinned integrity and extracted there. No upstream source is vendored into CoreRP.

The published archive has no root lockfile. Host-only `npm install --omit=dev --ignore-scripts --no-audit --no-fund` resolves dependencies and generates a local lockfile; retain its hash with future actual-host evidence. Never run this install in the CoreRP project or change its dependency graph. Node22.23.2 satisfies the inspected >=20 host engine.

Use disposable host data, loopback-only listening, a distinct port, and disable browser auto-launch, extension auto-update, model/tokenizer auto-downloads and server plugins. Do not disable CSRF or use existing chats/accounts. No test implementation may replace the real `SillyTavern` global with a mock and count that as7B/7D compatibility.
