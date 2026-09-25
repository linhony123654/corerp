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
`corerp_request_retire`, `corerp_events` (bounded pages), plus the F2 spatial
tools `corerp_journey_start`, `corerp_journey_cancel`, `corerp_map_survey` and
`corerp_map_read`, and the F3 participant tools `corerp_round_read`,
`corerp_round_wait`, `corerp_round_speech`, `corerp_round_move`,
`corerp_round_advance`. Timed travel enters an actual
segment; cancellation leaves the traveller there. Map notes are dated beliefs,
not current route permission. Shared rounds must first be opened by a local
operator through the authenticated HTTP route. This adapter cannot enroll or
assign/release a controller or open a round; each resident receives its round ID from
the operator trigger. A round wait is an absolute horizon proposal, not an
immediate clock advance. Only after Human and all assigned external residents
submit may a participant request the nearest boundary. Receipts expose no
other participant's horizon or raw wait evidence Event ID. Direct
`corerp_wait` is refused while an external resident owns control.
`corerp_round_speech` and `corerp_round_move` are private proposals, not
immediate Events. Move requires the resident's observed origin and an open
immediate route, not a timed journey. Every participant, including Human, must
first submit a wait or action. At an action boundary, one selected typed action
is accepted at the current world time; other proposals return
`deferred_no_effect` and require a fresh observation/new round. Direct typed
RP actions cannot bypass an active round. A selected speech's internal-NPC
response settles before the round closes. If Human chooses to wait again at
the same world time, the next round gives the wait/scheduler boundary priority
over repeated external actions; Human can still choose an action instead.
`corerp_round_advance` may be retried with a different per-call scheduler budget
when due work remains; the round and accepted child key stay the same. This does
not change the exact-budget retry contract of direct `corerp_wait`.

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
(legacy dialogue plus move/wait mixed plans), each accepted exactly once, and
the same-world F2 map/journey/cancel path. F2's local gate and scoped limits are
recorded in [its phase report](../../docs/f2/phase-report.md).
The F3 integration now also exercises two separate service-credential MCP
stdio clients and one Human client against the same Go Runtime and SQLite
world. A local operator command enrolls/assigns the two residents (and can
explicitly release a generation; the MCP model cannot); the
operator opens one round through authenticated HTTP. Both service clients
submit ten-minute horizons, cannot advance without Human, and then observe
one authoritative ten-minute advance. A's MCP process disconnects after its
accepted submission; B settles after Human responds, then A reconnects and replays the
same settlement without another wait Event. Afterward each scripted resident
commits two successive dialogue Events through its own MCP credential. This
fixture also moves them into the same place, verifies B hears A there, and
verifies B stops receiving A's face-to-face speech after leaving. This
fixture then explicitly releases both controllers through the local CLI,
verifies their MCP discovery/observation/fresh-action fences, exact old speech
recovery, and a settled-round receipt frozen while Human time advances. This
proves two external MCP controllers, shared waiting and a scripted move-versus-speech
window with the deterministic provider, **not** two configured live model
decision providers. See
the [F3 transport contract](../../docs/f3/protocol.md).

## Opt-in external-resident acceptance

`node live-residents.mjs` is a separate, opt-in F3 acceptance driver. It requires
`CORERP_LIVE_RUN=1`, `CORERP_LIVE_ENDPOINT` (a chat-completions endpoint), and
`CORERP_LIVE_MODEL_A` / `CORERP_LIVE_MODEL_B`. A remote endpoint must use HTTPS
and requires `CORERP_LIVE_API_KEY`; loopback HTTP is allowed for a fixture but
does not establish live-provider provenance. Provide the key through a trusted
environment/secret manager, not in a command, checked-in file, MCP tool
argument, or chat. The driver makes four bounded provider requests, two for
each separately authenticated resident. The default is strict
`response_format: json_schema` with a 300-token completion cap. For a provider
that supports JSON Mode but not strict schema output, set
`CORERP_LIVE_RESPONSE_FORMAT=json_object`; the adapter still validates exact
answer fields, speech act and bounded text before submitting any MCP action.
`CORERP_LIVE_MAX_COMPLETION_TOKENS` may be set from 300 to 4096 when a
reasoning model otherwise returns `finish_reason=length`; incomplete responses
always fail closed. An incompatible provider fails closed. Running it can incur
provider charges.

From `clients/mcp`, after securely setting those project-specific environment
variables, run:

```sh
node live-residents.mjs
```

The driver builds isolated Go binaries, prepares a disposable SQLite world,
creates temporary Human/operator/A/B credentials, and uses real MCP stdio
sessions over a loopback authenticated Runtime. It scripts only the setup move
that places the residents together. Each of the four speech choices then comes
from the configured external endpoint; Human-gated shared rounds submit those
choices through the typed MCP action and verify two accepted speech Events per
resident plus observed co-located hearing. Each accepted receipt is also
checked against its authoritative Event sequence, actor and exact model text;
an Event count alone is not treated as proof. The backend's **internal NPC**
decision/narrative providers remain deterministic for this test; that does not
replace the external residents' configured models. The model receives a
whitelisted own-scene summary plus at most three recent speech texts from that
resident's own scoped `corerp_events` stream. The next model decision can thus
react to a co-located utterance it actually heard; raw Context, evidence IDs,
tokens and another resident's private state are not forwarded. Heard text is
bounded/redacted as untrusted world data, not an instruction. The response is
schema-checked and size-bounded.

The JSON result deliberately reports `provider_provenance_review_required`.
Each decision includes bounded gateway response ID, reported model and optional
request ID; these fields are kept out of resident MCP requests and world Events.
Review the provider endpoint, model identifiers and independent provider
request/billing logs before counting it as the goal's fully audited live
evidence. A local
fake endpoint or `npm test` remains **fixture evidence only**. The standalone
run retains its exact `disposable_world` path under a fresh temporary directory
for audit; treat that database as sensitive local test data and remove it only
after review. The automated fake-provider test cleans up its own directory.
The driver is a bounded speech-decision acceptance case, not a persistent
autonomous loop, production deployment, or automatic credential provisioning.

Official [TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk)
server/client2.1.0 and zod4.6.5 are pinned. Its
[stdio factory](https://ts.sdk.modelcontextprotocol.io/v2/serving/stdio.html) handles
protocol negotiation. Tests exercise both default legacy initialization and an
explicit2026-07-28 connection. This proves MCP interoperability, not that a nested
Codex model played the world; no such nested agent was started. Actual combined
[Play/SillyTavern/MCP7D](../../docs/rp7/compatibility.md) and
[local stage gates](../../docs/rp7/verification.md) passed.
