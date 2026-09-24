# RP7 MCP and RP skill increment

Status: actual stdio implementation, recovery and [three-client7D](compatibility.md)
verified; [RP7 local gates](verification.md) PASS. See [setup/tools/security](../../clients/mcp/README.md) and
[RP skill](../../.agents/skills/corerp-rp/SKILL.md).

## Authority and implementation

`clients/mcp` is an isolated Node package with official SDK server/client2.1.0 and
zod4.6.5 locked. Only its own package-lock/node_modules changed. Official Go SDK
main requiredGo1.25 while this Runtime usesGo1.23.4; using a thin Node client avoids
changing the backend toolchain. SDK sources were verified from published packages
and official documentation, not remembered v1 APIs.

All12tools call fixed CoreRP HTTP routes with environment-supplied bearer auth.
No direct SQL, world engine, NPC model, principal override, arbitrary URL, hidden
retry loop or parallel character store. Runtime idempotency and request retirement
own acceptance. The host must retain original arguments; the adapter does not
pretend its process memory is durable intent storage. Closed schemas and bounded,
sanitized HTTP transport limit credential/error leakage; untrusted returned prose
still requires the skill's instruction/data separation.

Discovery `/bindings/list` is read-only, player-control filtered, limited/keyset
paged, and rechecked at open. It exposes no global world inventory or private NPC
state. Frozen minimum protocol and retained limitations: [protocol.md](protocol.md).

## Evidence

- Discovery61030 race PASS storage4.524s/HTTP2.757s; fullHTTP/server98373 PASS
  12.508s/0.162s; scoped storage/HTTP vet PASS. Tests cover permissions, actual
  discovery→open→observe, paging, revocation/disabled principals, aggregate rejection,
  rebuild/reopen and no writes. Initial duplicate grant fixture was corrected to
  expect schema rejection, not weaken the real unique constraint.
- Actual MCP84262 PASS `/tmp/corerp-rp7-mcp-1k9PeW`: official client launches real
  adapter subprocess and reaches real built Go/SQLite runtime, not a fake tool map.
  Discovery, all12tool declarations, dialogue/replay, MCP-process restart and turn
  resume, permission/schema/mismatch failures, move/social, partial wait→exact retry,
  retired open and foreign principal denial exercised.
- Expanded11019 PASS `/tmp/corerp-rp7-mcp-BwQuXN`: explicit2026-07-28 client as well
  as default legacy initialization; same permitted context. Real HTTP fixture proxy
  consumes a committed dialogue response then drops it; MCP returns uncertainty,
  retirement reports completed, exact retry replays, precisely two distinct player
  speech Events. Transport unit test covers origin/credential restrictions, missing
  transport certainty, response size bound and non-reflection of secret error text.
- Final current-source43505 PASS `/tmp/corerp-rp7-mcp-f8fZBv` after correcting the
  observe tool description to not promise pending-request inventory; same full
  MCP integration and transport tests,2tests/0failures. Skill validator and final
  whitespace checks PASS; no unfinished scaffold markers found.
- Skill quick_validate PASS; manually checked all tool names/fields against actual
  catalog/Runtime. Corrected resume to original key (not runID), and context
  coherence to actual field ownership; observe has no pending-request inventory.
  Skill-creator kept this instruction-only, scoped to playing, without simulator
  or unrelated-development guidance. No nested coding agent/live-model evaluation.

All listed runs are terminal and fixture processes cleaned up. Stdio supports
legacy and modern negotiations through SDK; no claim of MCP Streamable HTTP,
MCP subscriptions, all personal action APIs or arbitrary narrative-style editing.
Follow-up completed: actual same-instance Play/SillyTavern/MCP integration,
accepted-write chat-switch boundary and full7A–D local gates passed. See
[compatibility](compatibility.md) and [phase handoff](phase-7.md). RP8/Final remain required.

## Official references consulted

- [SDK repository](https://github.com/modelcontextprotocol/typescript-sdk),
  [stdio](https://ts.sdk.modelcontextprotocol.io/v2/serving/stdio.html),
  [client](https://ts.sdk.modelcontextprotocol.io/v2/get-started/first-client.html),
  [tools](https://ts.sdk.modelcontextprotocol.io/v2/servers/tools.html).
- [Codex MCP configuration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli),
  [repo skill discovery](https://learn.chatgpt.com/docs/build-skills).
  Local Codex launcher was inspected read-only before docs fallback; no Codex/AI
  CLI session was launched or user configuration inspected/modified.
