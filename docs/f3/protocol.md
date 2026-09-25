# F3 shared-time and typed-action transport contract

This additive contract does not change the frozen `corerp.client.v1` RP7
minimum. All routes are authenticated `POST /api/v1/rp/...` JSON requests.
The HTTP server binds an optional body `principal_id` to the bearer principal;
a mismatched value is rejected. The round ID is a lookup key, **not** a
credential. Storage rechecks role, session, Entity, world/branch and control
generation on every participant operation.

| Route | Caller | Request fields | Effect |
| --- | --- | --- | --- |
| `rounds/open` | local operator principal | `binding` (`instance_id`, `branch_id`, `expected_head`, `idempotency_key`), `human_session_id`, `external_session_ids` | Application-only round at one observed baseline; all assigned external residents must be included. |
| `rounds/read` | exact participant | `session_id`, `round_id` | Own round status/receipt; no world Event. |
| `rounds/wait` | exact participant | `session_id`, `round_id`, `horizon_world_time`, `idempotency_key` | One private absolute horizon, strictly after baseline and at most 24 hours later; no time advance. Exact retries return the same submission. |
| `rounds/speech` | exact participant | `session_id`, `round_id`, `text`, optional `speech_act`/`delivery_channel`/`introduce_self`, `idempotency_key` | One private speech proposal at the observed baseline; no Event until all participants submit. An exact retry reads the same proposal. |
| `rounds/move` | exact participant | `session_id`, `round_id`, `from_place_id`, `to_place_id`, `idempotency_key` | One private **immediate** move from the resident's observed origin along a currently open route. Timed journeys require a separate decision path; no Event before the Human barrier. |
| `rounds/advance` | exact participant | `session_id`, `round_id`, `budget` (1–10000) | After every participant including Human submits: settle a Human-authorized wait at the nearest scheduler/horizon boundary, or choose one typed action at unchanged world time. Speech drains its internal-NPC turn; move delegates to `MoveRP`. The scheduler budget bounds this invocation and may change on retry without changing the accepted round/child identity. |

The participant receipt contains `round_id`, `status`, baseline/current world
time, required/submitted counts, own proposal disposition, and on settlement a sequence number. It does
not contain another participant's principal, Entity, private horizon, the
computed minimum target, or a raw evidence Event ID. `open`/`advancing` occupy
the branch's sole active-round slot; `stale` releases it after an intervening
world Event. An active participant session cannot be closed mid-round. The
legacy direct `actions/wait` remains blocked while an external controller is
assigned. During an active round, direct typed speech, wait, move, social,
journey, map survey and interaction/turn proposals are blocked; exact already
committed receipts remain recoverable. A round does not turn each participant's
wait into a separate Event. On an action boundary, nonselected proposals get
`deferred_no_effect`; their owners must reobserve and submit in a new round,
not silently reuse the old cursor. An unrelated world Event after a wait
selection but before its Human wait intent is accepted marks the round stale
and frees its slot; once the intent exists, its scheduler work resumes on the
same child key. Round opening rejects pre-retired or already accepted derived
child keys, and retirement cannot claim reserved keys in an active round.
Selection rotates after the prior selected Entity across speech **and immediate
move**, rather than following
network arrival order. Other action types (social/timed journey) are not yet
available as round proposals.
If Human waits again after a speech action at the same world time, the next
round prioritizes the earliest submitted wait or due scheduler boundary;
repeat model action proposals are deferred. Human action proposals remain
actions. This rule prevents a faster external controller from indefinitely
starving time while Human authorizes waiting. An already-due scheduler item
also wins when Human submitted a wait. The chosen speech's NPC replies use
the existing `RunRPTurn` provider and are fully settled before the round
closes; the round receipt still exposes only the speaker Event sequence and
own disposition, not NPC IDs or their private decision input.

The MCP adapter exposes `corerp_round_read`, `corerp_round_wait`,
`corerp_round_speech`, `corerp_round_move` and `corerp_round_advance`
with strict schemas. It intentionally has no operator
enrollment/assignment/open tool and no direct AgentKnowledge tool. The local
operator supplies each participant's round ID through the host's trusted
trigger. Do not poll rapidly or give another resident's session ID to a model.

The local `backend/cmd/corerp-controller` command accepts one JSON request on
stdin with `-db EXISTING_PATH -action enroll|assign|release|replace`. `enroll` uses
`RPExternalControllerEnrollmentRequest`; `assign` uses
`RPExternalControllerAssignmentRequest`; `release` uses
`RPExternalControllerReleaseRequest`; `replace` uses
`RPExternalControllerReplacementRequest`. All require an exact operator
`binding` with current `expected_head` and a durable key, and all commit
sourced Events via the same storage owner as tests. The command does not
create service principals, issue bearer credentials or add MCP tools. Service
principal provisioning remains a separate trusted operator prerequisite;
the integration fixture creates disposable principals in its temporary DB.
Never pass bearer tokens in CLI arguments or request JSON.

Verified locally: authenticated three-resident HTTP settlement and process
reopen; actual MCP stdio→Go HTTP→one SQLite world with two distinct service
tokens plus Human, one ten-minute clock advance and exact MCP reconnect replay.
The A MCP process disconnects after its accepted horizon; B can settle once
Human submits, and A's new process resumes the same session/result without a
second wait Event. This is accepted-work recovery, not a lease or automatic
controller substitution policy.
After that round, scripted MCP clients for A and B each submit two distinct
successive speeches, all four accepted as their own sourced world Events. The
same two service residents then move to one real place: B's own events include
A's co-located speech; after B leaves, a second A speech is absent from B's
event feed. This checks actual spatial hearing, not shared private context.
Migration039 keeps a sourced `released` generation rather than deleting the
authority row. Explicit operator release changes active generation 1 to
released generation 2 and closes that generation's service sessions. The
same enrolled controller may be reassigned at generation 3. Expected
generation must match exactly; old sessions cannot revive when the same
principal returns. Release refuses pending RP work, open/paused interactions
and active shared rounds, while accepted Scheduler journeys continue.
After an explicit release, a local operator may instead replace that Entity's
enrollment with a different active service principal/controller instance at
the exact released generation. Replacement does not grant control or advance
generation; a separate assignment advances it. The two-slot bound, duplicate
principal/instance exclusion, old exact-key receipts and old-generation fence
remain intact. This is a sourced manual rotation, not disconnect failover.
Settled shared-round receipts remain readable by the exact original
participant after release. Their `current_world_time` is frozen at the
round's own `RPWaitCompleted` Event, so a released controller cannot use an
old receipt to watch later clock advances. Disconnect alone never triggers this operator
action or an automatic model substitution.
The actual MCP/HTTP fixture also invokes the local CLI release for both
service residents: discovery and fresh observation/action are fenced, exact
old speech still replays, and an old settled-round receipt remains fixed after
a later Human wait. This is transport evidence for explicit release, not live
model decision quality.
Not yet verified: round-owned social/timed-journey actions and a full
disconnected-controller policy (both outside the implemented shared-round
scope). A configured live HTTPS gateway has produced two consecutive model
decisions for each of two separately authenticated residents, with exact
world-Event correlation; independent upstream request/billing logs remain
unreviewed. F3 gate remains open for that provenance audit.

Migration040 upgrades existing 038/039 rounds: it renames the stored
completion Event column, preserves earlier wait submissions/receipts, and
stores action proposals in a private application table. Accepted selected
speech uses the existing typed Event owner and exact child key, then an RP
turn adopts that same speech command to settle internal-NPC responses. A
crash after speech but before turn/round settlement is recovered without a
second speaker Event or duplicate NPC effect.
HTTP and real MCP fixtures cover two conflicting service proposals, Human
barrier, one selected Event, own-only dispositions and two-round rotation.
Migration041 adds a typed private `move` proposal and a selected-kind marker
without rewriting the applied 040 schema. It retains earlier wait/speech
receipts on upgrade. A selected move uses the existing `RPPlayerMoved` Event
owner with the pinned origin, destination, cursor and derived child key;
changing the child payload is rejected. If a response disappears after the
movement Event, reopening settles the original round without repeating the
movement or advancing time. A second Human wait at the same world time wins
over repeated movement attempts. Transport fixtures exercise a move versus
speech conflict with independent service credentials; no live LLM selected it.

Exact accepted-result recovery is separated from fresh-action authority for
session open, move, speech, social, journey start/cancel, completed wait,
settled turn, terminal interaction and request-retirement receipts, exact
map-survey commands, and session style/interaction-mode revision keys. After a quiescent assignment,
the original principal can read only the durable result of its own exact
session/key/request hash; it cannot submit a new action through that stale
session. Another body under the same key conflicts, and another principal
cannot read the old session. Pending work still needs the current generation.
An open/paused interaction prevents session closure and target assignment
until it settles or is explicitly stopped, including if the session row was
already closed. This is receipt recovery, not permission for the old
controller to continue an unfinished proposal after takeover. Settled
shared-round receipts are now covered; presentation-only idempotent revisions
retain exact historical receipts without granting fresh control. Current
generation with a revoked player grant still fails authorization. Any
additional receipt API must be audited before the F3 gate.
