# RP7 — SillyTavern integration design and browser prerequisite

Status: RP7B local acceptance PASS, including actual binding/context/typed actions/
streaming/settings and recovery. Final [three-client integration](compatibility.md)
and [stage checks](verification.md) passed. Earlier incremental statements below
are historical; retirement/discovery/protocol/accepted-write chat switch/7D have
since been completed. Group chats, automatic reconnect and exhaustive social/
power-loss interleavings remain outside the verified minimum.

## Official interface inspection

The official [UI extension guide](https://docs.sillytavern.app/for-contributors/writing-extensions/) specifies a browser JavaScript entry point and manifest. `SillyTavern.getContext()` exposes host services; extension settings and per-chat metadata have separate persistence APIs. Chat metadata references must be reread on chat changes. Prompt interceptors can intervene before generation. Prefer the supported context API over internal file imports. The [release context source](https://raw.githubusercontent.com/SillyTavern/SillyTavern/release/public/scripts/st-context.js) was also inspected. These moving URLs are research, not pinned host-version compatibility evidence; pin and run an actual host before claiming integration.

## Intended authority and client design

Use a thin browser extension talking to authenticated CoreRP HTTP endpoints. Persist only endpoint preferences and existing CoreRP session/world/character bindings in the appropriate host metadata. Tokens stay in memory and are never stored in host settings, chat metadata, character cards, URLs or generated prompts. A chat's character card is presentation, not authority to create or clone a CoreRP NPC. Switching chats must stop old subscriptions, clear the old injected context, and reject stale in-flight results.

Refresh observe and permitted context against one matching head/time before injection. Heard text is attributed untrusted data. Host-generated prose does not commit facts; dialogue/actions/waits must use CoreRP commands, expected cursors and persistent retry identities. Ambiguous failures retain the exact command for retry; never mint a new action after losing a response. Stream checkpoints drive refetch of authoritative views, not independent local state simulation. Other RP3–5 read views still need explicit refresh rules because the current feed does not enumerate every personal transaction subtype.

Next vertical slice: actual host loads extension → user binds an existing world character through CoreRP session open/resume → own observation/context appears → one explicit dialogue command commits → SSE updates/refetch → restart preserves binding and exact retry behavior. Then expand typed command bridge and cross-client compatibility with Play/MCP. Do not use a mock `SillyTavern` global as host compatibility evidence.

## Explicit browser origin configuration

CoreRP previously had no CORS grants. A browser UI extension served on another origin needs an intentional grant, unlike CLI/MCP clients. The server now accepts optional `-browser-origins` with comma-separated **exact origins**, for example `-browser-origins http://127.0.0.1:8000`. Use the address actually shown in the host browser; `localhost` and `127.0.0.1` are different origins. Do not add a path, trailing slash, spaces or wildcard. Existing token/cursor-secret configuration and `-db` remain required; this flag never supplies credentials.

Default empty configuration preserves the existing HTTP behavior and adds no CORS grants. With an allowlist configured:

- Only exact listed HTTP(S) origins on `/api/v1/` routes receive browser access. Other Origin-bearing requests are rejected before dispatch. CLI requests without Origin remain supported. If a same-origin proxy forwards Origin headers, list its public browser origin too.
- Preflight permits GET/POST and only Authorization, Content-Type, Last-Event-ID. It never invokes storage/business handlers. Actual requests still pass the unchanged bearer authentication and capability checks; allowed browsers can read normal authentication/error responses.
- No wildcard, credentialed cookies, origin suffix matching, automatic host/proxy trust, or private-network CORS grant is added. Cache variation follows Origin and preflight headers; responses are no-store. Browser policies concerning HTTPS/mixed content/local network remain applicable. This is not a production TLS/authentication hardening claim.
- The middleware passes the original response writer through, retaining SSE flushing and deadline capabilities. No reverse proxy, server-side URL fetcher, additional database or third-party server plugin is introduced.

Security implication: an allowed origin's scripts can use a token entered there. Only configure a trusted local host/origin; CORS is not a substitute for token scope. This work does not change any deployed service or real account.

## Evidence

### Actual installed-extension vertical slice

Added manifest/index/style to the transport directory. Native host Extensions panel supplies origin, memory-only password token, existing-session or existing-world/entity binding, coherent context refresh, explicit dialogue/typed-command controls, exact pending retry and local detach. Single-character chats only. Native generation interceptor explicitly aborts while bound; no second local NPC/economy/memory authority. Context is bounded own evidence marked as untrusted data. Scope checks and AbortController invalidate old chat requests; further adversarial chat-switch coverage remains open.

The pinned host's `saveMetadata`/`saveChat` catch errors without rejecting. The extension therefore reads its own persisted chat binding/intent back via the authenticated host chat API before sending a runtime command, including retries. If storing acknowledgement fails, it restores the exact pending key in memory. No token is placed in host metadata/settings/prompts.

Final real Chromium98889 PASS `/tmp/corerp-rp7-extension-aB4sYN`: actual installed manifest loaded after real host onboarding; authenticated cross-origin runtime open/read/context/dialogue; literal narrative appears in panel; native host generation intercepted; same presentation chat binding survives reload and re-selection, token/context clear until reconnect; actual accepted second speech response lost then pending survives reload and same-key retry leaves speech count at2; injected host save503 prevents third speech until verified save/retry, final count3. Five transport tests also PASS. All fixture processes stopped by harness cleanup. No live external LLM, fake host/global, duplicated character runtime or database writes by adapter.

Earlier evidence:62923 first bind/dialogue/reload PASS;36312 lost-response/retry and responsive-width PASS. Initial failures were real onboarding not yet completed, a test selector assuming a semantic Save button where host uses a div, native browser `fetch` losing its global receiver, and a test assuming reload automatically reopens the same card. Corrected against actual host source/behavior. Native fetch binding bug now has unit receiver assertion plus browser regression. The current harness explicitly re-selects the persisted card, not a fabricated replacement chat.

Visual contract for this scoped extension surface: host-native density/material/type, no new dashboard/cards/imagery/animation; world time/place and actual narration lead reading, explicit command is the main action, connection details are secondary/collapsible. Controls need keyboard labels, live error/status and busy/disabled handling; buttons wrap with >=40px height. Actual screenshot review caught host `.menu_button` fixed width causing vertical Chinese labels; explicit width/padding corrected.390×844 mobile screenshot from36312 and final desktop panel98889 inspected, submit remains in bounds and text/controls readable. Final desktop includes the expected host error toast from injected save failure; this is not claimed to be a polished release screenshot. Remaining UI/host lifecycle gates are open.

Follow-up68743 PASS `/tmp/corerp-rp7-extension-ak2N5w`: actual UI one-minute wait, two moves using observed legal routes, gift1minor with authoritative wallet/fact checks, principal override rejected before command persistence, external authenticated session dialogue updates the panel automatically with shared observer/world/head/facts. This test exposed session-local history despite functioning SSE; corrected history ownership and late-settlement invalidation are detailed in [history.md](history.md). Four total player speeches, final head21. All preceding recovery checks retained.

Follow-up96783 PASS `/tmp/corerp-rp7-extension-iKXSeU`: real graceful host+runtime process restart at the lost-response boundary, unchanged authority snapshot, fresh-page pending-key recovery; delayed old context after actual chat switch leaves new chat isolated; next-morning budget1 wait returns actual budget_exhausted and completes via unchanged request. See [lifecycle evidence](lifecycle.md). Earlier action/stream/recovery checks remain in that run.

Open: definitive command-rejection resolution (pending intents are conservatively retained), remaining social variants and write-stage chat-switch races, full7A discovery/command/refresh freeze, and Play/ST/MCP same-world E2E. Group chats and automatic reconnect are not implemented. Do not treat the external plain HTTP client as proof of actual MCP integration or graceful restart as power-loss testing.

Rejection-boundary inspection: `RunRPTurn` commits player speech before later NPC/narrative stages; `WaitRP` can process scheduler work before completing its intent. An error status is therefore not proof that nothing happened. Recovery must retain the exact request or obtain a server-verifiable outcome; a snapshot showing no receipt also cannot rule out a delayed in-flight request. Do not implement client-only discard/automatic new-key retry as a substitute for this missing protocol boundary.

### Pinned actual host preparation and smoke

The npm1.19.0 artifact is pinned in `clients/sillytavern/host-fixture.json`; SHA512 integrity was checked before use. Published git head is `7e8663cd9c184a550b37238218bdd32c6efc68e9`, different from inspected release head `06bde939fb1e9c4c8d8641d810f0a916b5bce127`. Git transport did not return and was terminated; GitHub archive requests timed out (the codeload download was partial, not used). Official npm download succeeded; no moving-source fallback was labeled equivalent to the pinned archive.

Isolated fixture: `/tmp/corerp-rp7-host-RCfN0V/package`, data/config adjacent under that temporary directory. Node22.23.2; host dependency installation32383 succeeded (656packages; upstream deprecation warnings retained, not an audit/security pass). The archive has no root lockfile; generated host-only package-lock SHA256 is `590c7841d0fbd5ced98db261b5b9f146e1cc5f0532ebd807b1769ff703440772`. No CoreRP dependency/lockfile changes. Runtime setup disables extension/model/tokenizer downloads and server plugins, keeps CSRF enabled, and binds only127.0.0.1:4187.

Actual Chromium smoke60790 PASS via `node scripts/verify-rp7-host.mjs`: real `/version` reports1.19.0; real context exposes setExtensionPrompt/saveMetadata/saveSettingsDebounced/getCurrentChatId/addOneMessage/saveChat, eventTypes CHAT_CHANGED/APP_READY and slash-command registration; imported actual extensions module exposes generation interception. No page errors were observed during this check; third-party browser requests were blocked. This is API availability evidence, **not** installed-extension behavior or7D E2E. The fixture server is stopped after inspection; restart with `node server.js --configPath /tmp/corerp-rp7-host-RCfN0V/config.yaml` from the package directory when continuing tests.

Browser transport `clients/sillytavern/client.js` and five Node tests PASS: exact existing route mapping, header-only credentials/cursor, cookie omission/redirect denial/no auto-retry, split Unicode/CRLF/multiline SSE, incomplete/oversized/malformed-frame refusal and subscription release on cancellation. Fetch fixtures alone are not host compatibility evidence; the subsequently installed extension's actual-host evidence is recorded above.

Focused race89789 PASS: HTTP2.133s, server2.174s. Tests cover exact-origin/preflight rejection, no business execution on preflight, absence of wildcard/cookie/private-network grants, CLI/default preservation and actual SQLite-backed API authentication/error handling. Added startup rejection check before database creation for invalid origin configuration. Final fullHTTP/server7525 PASS11.954s/0.162s; vet PASS; all handles terminal. Actual cross-origin browser/SillyTavern end-to-end remains required; HTTP tests alone do not prove browser compatibility.
