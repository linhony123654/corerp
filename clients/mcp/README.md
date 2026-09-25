# CoreRP MCP adapter

Node>=20, standalone stdio MCP adapter. Runtime remains the authenticated HTTP
service; this process never opens SQLite, creates a second world, evaluates NPCs,
stores credentials on disk or automatically retries commands. Dependencies are
pinned in this directory's package-lock; backend and frontend dependencies are
unchanged.

## Local setup

From `clients/mcp`, run `npm ci --ignore-scripts --no-audit --no-fund`.
Start an existing CoreRP HTTP server with a player-scoped token. Supply
`CORERP_ORIGIN` (HTTPS, or loopback HTTP) and `CORERP_TOKEN` through the MCP host's
environment. Never put the token in tool arguments, command-line arguments or a
checked-in configuration. The host launches `node /absolute/path/clients/mcp/index.js`;
stdout is reserved for MCP JSON-RPC. No extra inbound network listener is created.

For Codex, add this reviewed example to the chosen user/project configuration;
replace the absolute path and supply the named environment values securely:

```toml
[mcp_servers.corerp]
command = "node"
args = ["/absolute/path/corerp-console/clients/mcp/index.js"]
env_vars = ["CORERP_ORIGIN", "CORERP_TOKEN"]
startup_timeout_sec = 15
tool_timeout_sec = 150
```

These stdio/configuration fields come from the
[official Codex MCP documentation](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).
No user/global configuration was edited during implementation. Do not disable host
approvals; observe/resume update application metadata, actions mutate world state,
and retirement permanently disables a request key.

The repository skill is [corerp-rp](../../.agents/skills/corerp-rp/SKILL.md), normally
discovered when working inside this repository. For another workspace, review and
copy that folder into the workspace's `.agents/skills/`. Repo-local discovery is
documented in [official Codex skills guidance](https://learn.chatgpt.com/docs/build-skills).
Invoke `$corerp-rp` to play; development tasks should not select it. The skill teaches
call order, permitted knowledge and original-key recovery; it does not simulate a
runtime or grant permission for actions beyond the user's intent.

## Tools and recovery

`corerp_worlds`, `corerp_session_open`, `corerp_session_read`,
`corerp_session_resume`, `corerp_observe`, `corerp_context`, `corerp_dialogue`,
`corerp_turn_resume`, `corerp_wait`, `corerp_command` (move/social),
`corerp_interaction`, `corerp_interaction_resume`, `corerp_interaction_stop`,
`corerp_interaction_default`, `corerp_interaction_default_set`,
`corerp_request_retire`, `corerp_events` (bounded pages).

Every tool has a closed input schema: no arbitrary URL, token, principal or raw
HTTP operation. Tool results contain `{data: ...}` or sanitized `{error: ...}` and
errors set `isError`. Runtime messages and fetch exceptions are not reflected.
HTTP requests have a120s deadline,4MiB response bound, no cookies or redirects.
Cancellation is forwarded; it does not imply effects were rolled back.

Record the exact request/key before a write. Retry it unchanged after uncertainty;
use turn resume for accepted dialogue, exact wait retry for `budget_exhausted`.
Retirement only clears an unaccepted key. See [client protocol](../../docs/rp7/protocol.md)
and [request recovery](../../docs/rp7/requests.md). MCP does not maintain an additional
pending-intent database: the invoking host must retain exact calls durably to promise
recovery after losing conversation state. Dialogue currently exposes plain speech
and speech act, not all narrative-style editing APIs; events use bounded polling,
not a new MCP streaming or subscription implementation.

The additive interaction tool supports a deliberately small Chinese action grammar:
speech, a currently observed adjacent destination (`去地点`), explicit `等1小时` /
`等2小时` / `等4小时`, and either action followed by `，随后说「原话」`.
`AUTO` and `SCENE` ask for clarification when an action is ambiguous; explicit
`DIALOGUE` always treats the text as speech. `budget_exhausted` retains the
original interaction request. `paused` preserves already committed effects and
requires an explicit stop/new choice; stop cannot hide an accepted child.

## Verification and provenance

`npm test` runs transport boundary tests and an actual SDK MCP client spawning this
adapter against a built Go server/temporary SQLite world. Requires project Go,
`sqlite3` and Node; no live model or nested coding agent is launched. Tests cover
discovery/open/observe, dialogue and process-restart recovery, permissions, mismatch,
move/social, budget wait, retirement, and a real accepted-but-lost HTTP reply through
a fixture proxy. The current fixture verifies four distinct player speeches
(legacy dialogue plus move/wait mixed plans), each accepted exactly once.

Official [TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk)
server/client2.1.0 and zod4.6.5 are pinned. Its
[stdio factory](https://ts.sdk.modelcontextprotocol.io/v2/serving/stdio.html) handles
protocol negotiation. Tests exercise both default legacy initialization and an
explicit2026-07-28 connection. This proves MCP interoperability, not that a nested
Codex model played the world; no such nested agent was started. Actual combined
[Play/SillyTavern/MCP7D](../../docs/rp7/compatibility.md) and
[local stage gates](../../docs/rp7/verification.md) passed.
