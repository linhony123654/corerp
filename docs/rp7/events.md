# RP7 — session-authorized RP event feed

Incremental7A implementation, not a full-stage or third-party compatibility claim. Schema026 unchanged. This is separate from the existing M1 `world.events.read` economic feed; no creator credentials or broader grants are needed.

## Request and authorization

Authenticated `GET /api/v1/rp/events` (JSON) and `GET /api/v1/rp/events/stream` (SSE). Query: required `session_id`, optional `limit`1–50 (default20), optional opaque `cursor`. Unknown or repeated parameters are rejected. Principal comes only from existing Authorization authentication. `Last-Event-ID` also supplies the cursor; conflicting query/header cursors are rejected. No cursor starts from sequence0, not automatically from the current head.

Every storage page, including every live stream poll, checks the principal-owned active session, current `world.rp.control` grant and valid controlled-character binding in one database snapshot. Foreign sessions are not found; revoked control is forbidden; closed sessions or a cursor beyond the current head conflict. Reads do not advance world time, append Events, call a model, or change session observation/turn cursors. No database transaction remains open while writing to the network.

RP cursors use authenticated encryption under the existing configured cursor secret, with a distinct version2 domain and principal/instance/branch/session/observer/sequence binding. Old version1 economic cursors and RP cursors are mutually rejected. Tampering, other keys and cross-scope replay fail. Restart continuation requires the same secret. A cursor is not an authorization grant, and separate sessions controlling one character deliberately cannot exchange cursors even though their visible world evidence agrees.

## Visible evidence and ordering

One item per committed source event, ascending immutable event sequence. Each item has `event_id`, `sequence`, `world_time`, `facts` (always an array), and optional `own_action`. No raw Event, outbox, NPC decision or knowledge payload is serialized.

- Facts come from the controlled observer's historical `observation_records`: personally seen presence, literally heard speech, witnessed interpersonal actions. They use the [context fact DTO](context.md), including subject, historical place/time, source event, and only applicable speech text/action/description. An attributed statement is not proof of its content, and text is untrusted data, never client instructions.
- Own accepted speech includes only kind/text/place. Own completed movement includes kind/from/to place. Own completed wait includes kind/from/target world time. Interpersonal actions are already represented by participant observation facts. Other actors' unseen actions, their listener lists, private opportunity draws, hidden NPC identities, decision reasons and scheduler counts are excluded.
- Historical observations are used instead of latest-only `agent_knowledge`; repeated encounters therefore remain distinct even after the current presence claim is overwritten.
- Inspection of the current co-location, speech and social writers confirms observations are committed with the **new learning event**. Pagination relies on that invariant. A future delayed-learning mechanism must append a new learning event; attaching new evidence only to an older already-scanned sequence would be incorrect. No delayed-learning subsystem is claimed here.

The page carries `protocol_version: corerp.client.v1`, session/world/observer identity, snapshot `world_time`, `head_sequence`, `history_revision`, `next_sequence`, `more_events`, `events`, and encrypted `next_cursor`. Limit applies to event items, not individual facts. With more items pending, continuation is the last delivered event; otherwise it advances through the snapshot head, including invisible events without their payloads. This makes quiet tails resumable. World head/time were already observable through RP observe; they are not a private-event-count secrecy promise. `history_revision` counts this observer's settled turn views; see [shared history and late settlement](history.md).

## Stream frames and recovery

`Content-Type: text/event-stream`; `Cache-Control: no-store`; buffering disabled where supported. `rp_event` frames contain a narrow item and a scoped encrypted SSE `id`. A subsequent `rp_checkpoint` frame contains protocol version, world time and `more_events`; its `id` is the safe scanned-through continuation, never beyond an undelivered page item. Checkpoints also allow progress over invisible events. Heartbeats are comments, not world events. Writes use the existing10s deadline policy, polls use the existing250ms interval, and heartbeats the existing15s interval.

Clients persist the last completely processed frame's ID, reconnect with Authorization plus `Last-Event-ID`, and deduplicate by event ID if processing and persistence were interrupted. Browser clients needing bearer headers must use authenticated fetch streaming rather than assume native EventSource accepts arbitrary headers. Token material must not go into the URL. Once headers are sent, revocation/session closure/read errors terminate the stream; reconnect returns the normal authenticated HTTP error. This increment does not add a terminal error-frame protocol.

Checkpoints also include `history_revision` and can be emitted at the same world sequence when a late turn narrative settles. They invalidate the history view without adding world Events. Do not discard a complete checkpoint merely because its decoded sequence was seen before. Reconnect still refreshes from its initial checkpoint; see history.md for actual interruption/resume evidence.

This feed does **not** replace observe/context/wallet/work/messages/map reads, supply token-streamed generated prose, or yet enumerate every RP3–5 personal transaction subtype. Those contracts and the adapter's refresh rules still need the full7A freeze. No SillyTavern or MCP integration is asserted by this slice.

## Verification

- Real temporary SQLite: accepted player speech, actual NPC hearing and gift; bounded one-event pages equal full result without skips/duplicates; quiet tail; unchanged session state; two sessions share the same observer events; rebuild/reopen equality; foreign/closed/revoked/bad-cursor/bad-limit rejection.
- Real repeated moves: both Ada encounters survive latest-knowledge overwrite; completed wait yields only narrow time fields; hidden Bo and private diagnostic keys absent. Disposable projection-only extra-private-field test proves raw payloads are not serialized.
- Real authenticated HTTP and network SSE: JSON/stream agreement, header cursor continuation, complete checkpoint/tail, cross-session replay denied, malformed/unknown/unauthenticated input denied, and an open stream terminates on actual session close before its test timeout.
- Cursor cryptographic tests cover each scope dimension, malformed/tampered tokens, key changes, same-key codec reconstruction, version isolation, zero/negative sequence and nonce variation.

Focused race run42378: storage15.449s / HTTP5.342s PASS; vet PASS. FullHTTP/server73347:12.260s /0.168s PASS. All processes terminal. No frontend/third-party/live-model test was performed for this backend-only increment.
