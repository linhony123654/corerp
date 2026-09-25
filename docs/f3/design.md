# F3 design — external residents, exclusive control and common time

Status: design established; security, sourced enrollment, first assignment,
accepted-hearer handoff, local monotone release/reassignment/replacement,
shared-time rounds and speech/immediate-move HTTP/MCP slices verified.
Social/timed-journey proposals remain outside shared rounds. Four live gateway
speech decisions now have matched world Events; independent provider provenance
review remains open. See `phase-report.md` for the bounded receipts.
**No F3 gate claim.**
F2 source checkpoint: `0daf9f238f66d99b63bf3aa8f73a7c2ac7d2db16`.

## Existing owners and gaps

- `rp_sessions` binds a player principal to one existing Entity, but permits
  multiple sessions and has no control generation. `authorizeRPControl` and
  discovery require `principal_type='player'` and an exact `world.rp.control`
  grant. It cannot safely represent an external model merely by minting a
  second player token.
- The HTTP authenticator maps bearer token to principal only. The MCP adapter
  has a closed RP-session tool list and no direct `AgentKnowledge` tool, but
  `/api/v1/agent-knowledge/query` remains an authenticated HTTP route. The
  old `agent` principals have capability-scoped raw Entity/Event ID reads for
  career referral. Tool omission is not a service authorization boundary.
- `WaitRP` persists retry intent then calls the scoped scheduler toward the
  caller's absolute target time. Two sequential requests can add elapsed time;
  existing waits must remain backward compatible, while F3 residency needs a
  separate shared-round boundary. Scheduler obligations and accepted journeys
  must continue independently of controller availability.
- Internal NPC decisions arise in turn, warm and initiative paths. Their
  accepted commits must be fenced against a controller change; suppression
  only at the MCP surface would still permit a double decision.

## F3 invariant and authority model

1. A controller is an authenticated principal plus a controller instance,
   session, scoped Entity and monotone generation. Principal and controlled
   Entity are distinct. External models use dedicated service principals and
   credentials, never an existing `agent` referral principal or a reused
   player credential. A scoped assignment has one active owner; concurrent
   assignment attempts resolve by expected generation in one transaction.
2. Assignment/release is a sourced world command/Event with a recoverable
   projection. A session pins the generation it opened under. Every new
   proposal and its final commit must compare that generation against the
   active authority; a late old-generation proposal fails without a new
   Event. Previously accepted world effects are not rolled back, and exact
   recovery can read the committed outcome. A disconnected external owner
   remains owner but issues no new decisions until its explicit resume; no
   implicit model substitution or internal-AI takeover.
3. The service boundary denies raw `AgentKnowledge` reads from any external
   controller principal regardless of incidental legacy grant. The HTTP
   handler must bind authenticated principal; the storage/service method must
   independently enforce observer scope and internal referral purpose. Model
   observations use F2 `observe`/`context` projections with anonymous
   references. Do not redesign F2 referral evidence or expose its raw IDs to
   MCP as a shortcut.
4. A shared-time round stores each active participant's proposed action or
   absolute wait horizon against the same world-time baseline. No wait moves
   the clock until the required participants have responded or an explicit
   authorized boundary resolves the round. Scheduler advances only to the
   nearest necessary world event, interaction boundary or minimum horizon,
   then reopens decisions. Faster clients cannot buy more simulated time by
   sending sequential waits. Wall-clock deadlines, controller connectivity
   and model-call budgets are separate from world time.

The first wait-only shared-round slice will pin one Human RP session plus all
currently assigned external sessions at one observed head/time. Operator setup
is application metadata, not a world Event. Participant submissions are
private absolute horizons; only counts/status are readable across sessions.
After every participant (including Human) submits, one persisted round target
is the earliest horizon or pending scheduler boundary. Exactly one Human-owned
`RPWaitCompleted` Event advances the clock through the existing retryable
scheduler path; no per-model wait Event is emitted. A later step can generalize
submissions from waits to action proposals. The old direct `WaitRP` remains
blocked while external residency exists. Round opening/advancement must reject
stale generation, changed baseline, missing Human authorization and foreign
session/actor tampering.
5. External model input is an ordinary bounded proposal built from its own
   Observation/Knowledge. Runtime validates action, cursor, authority and
   evidence before Event commit. Trigger-driven delivery and capped work
   replace high-frequency polling; A and B never share private context.

### Accepted-work handoff contract

Controller changes cannot use the generic private-fact helper unchanged:
that helper blocks whenever any RP turn/wait is pending. F3 needs a scoped
custom sourced command that can advance the controller generation while an
already accepted request is draining. A normal request under an old session
generation is refused. A recovery call for an *exact persisted, already
accepted* key may return its committed result or finish only its previously
accepted stages; it may not reinterpret input or accept a fresh proposal.
This distinction is made at the durable request owner, not by a client flag.

A player speech Event records everyone who actually heard it. The turn owner
must track each hearer's disposition: committed internal decision or skipped
because another controller owns that Entity. If control changes while an
internal provider is thinking, the final commit rechecks generation; a stale
proposal is rejected and that listener receives a skip disposition. Settling
counts decisions plus skips against the immutable accepted hearer set, so a
handoff neither produces two decisions nor leaves the turn stuck. Skip is
application orchestration, not an invented speech/world Event. Warm and
initiative triggers receive the same owner recheck before committing.

## Vertical slices and evidence

1. **Security precondition:** constrain raw AgentKnowledge at the service
   boundary to the specific internal-read capability and either creator review
   or an internal Agent's own scoped profile. External service/player
   principals are denied even if accidentally granted that capability. The
   controller registry later binds external credentials to dedicated service
   principals and must forbid reuse of an internal Agent credential. Prove
   internal career-referral reads still pass; guessed raw RP subject IDs
   remain denied. This gate precedes connecting any live model.
2. **Exclusive control:** first enroll up to two dedicated service principals
   against distinct existing Entities with a sourced, replayable local setup
   command. Enrollment alone grants no decision authority or MCP access.
   Then one Human and one external Entity in one world; sourced assignment,
   monotone generation, contention/late-request tests,
   and turn/warm/initiative suppression with accepted-effect recovery. Add
   the second model resident only after the first route is verified.
3. **Shared time:** two model residents plus Human wait from one baseline;
   assert ten-minute waits do not sum to twenty; scheduled pay/journey work
   executes once; one absent Human leaves a decision window open. Test
   disconnect/resume and absolute-time idempotency.
4. **Transport/live:** actual MCP stdio→HTTP world, two isolated credentials,
   real spatial meeting and leaving-scene hearing denial. With configured
   live Provider, each of two distinct model residents makes two successive
   real decisions; otherwise record `LIVE_VALIDATION_PENDING`, not pass.

F3 exit requires the full acceptance matrix in the goal, relevant regression,
Compare→Rebuild→Compare, phase report and a local checkpoint. No production
deployment is implied.

## Implementation increments

`ReadAgentKnowledge` now validates the internal-read capability and accepts
only a creator review or the exact Agent principal owning the requested
observer profile. An external `service` or `player` principal remains denied
even with an accidental exact legacy grant; an internal referral read still
returns stable IDs. This filter by itself does **not** establish a controller
registry or make old Agent credentials safe to hand to an external model.
The later enrollment/assignment increments require distinct service
principals and forbid reuse of an Agent referral credential. The later
higher-generation lifecycle and wait-only shared-time increment are below.

The enrollment increment now exists: a local operator registers one
existing `service` principal and one existing spatial Entity, with a unique
controller-instance key. Two registrations per world are allowed; a third,
duplicate Entity, duplicate principal or reused instance key is denied.
Registration commits a scoped Event and a rebuildable projection but **does
not itself** change `authorizeRPControl` or start an external model.
Focused tests prove exact retry, two-DB contention with one winner, restart,
projection corruption detection/repair, rejection of an old Agent credential,
and the fact that enrollment alone grants no RP session. A service credential
with an accidental legacy AgentKnowledge grant remains denied by the earlier
service-layer filter. Targeted race and vet checks pass.

Migration037 and `AssignRPExternalControllerLocal` add the first active control
slice. An operator may assign one enrolled service controller only at expected
generation zero, with no active session on the target Entity and no pending
wait. A pending turn is allowed only if it is the sole unsettled turn, already
has a committed speech Event, and its immutable hearer list contains that
target. Open/unheard turns remain fenced. The assignment is a world Event with a
Compare/Rebuild projection. New service sessions pin generation one and their
controller-instance key; the old player grant cannot open another session for
that Entity. Read-only decision-context builders remain available to internal
diagnostics, but provider invocation and final commit in turn/initiative/warm
paths reject internal autonomous actions for an externally owned Entity or an
Entity with an active Human controller session. Speech heard by either retains its hearing
Event but records an application-only skip disposition, so the turn settles
without an internal NPC effect. A Human session opened while an internal
provider is thinking wins at the final commit fence; the accepted speech
still settles with a skip. `WaitRP` refuses new unilateral waits in a branch
with an active external resident (for Human and service sessions alike); that
public gate remains in place after the local round slice was added.
Rebuild repairs damaged enrollment and authority rows in one transaction,
temporarily removing the derived authority row before replacing its referenced
enrollment and restoring it from the assignment Event.

This covers one accepted-work handoff path: a Human speech heard by a CoreRP
NPC may continue across target assignment, including provider-in-flight and
restart after committed speech. The old internal proposal is rejected and the
accepted turn settles once with a listener skip. This is **not yet** the full
handoff contract above: assignment still refuses a live target RP session,
pending waits and uncommitted turn intents. At this first increment, release
and generation increment were not yet implemented; migration039 below adds a
quiescent operator lifecycle. The provisional wait gate by itself was
not shared-time coordination; migration038 supplies a local wait-only round.
The existing generic private fact helper is safe only for this
constrained transition only through its explicit committed-listener exception;
other private-fact commands still block pending RP work. Later transport
verification connects two MCP service clients plus Human, but not two live
decision providers.

Migration038 adds application-only shared rounds for one Human and every
currently assigned service resident (one or two) at the same observed
head/time. Every participant submits one private absolute wait horizon; the
local operator opens the round, and any participant may request advancement
only after all submissions. The earliest horizon or due scheduler item becomes
one pinned target. The existing Human-owned wait intent and `RPWaitCompleted`
Event remain the sole time authority. Exact retries, process-reopen replay,
Human-withheld-window, one active round, and stale-baseline retirement are
covered by the focused three-resident test. Active-round sessions cannot be
closed. A later test round gives all three residents a horizon beyond a
fifteen-minute journey arrival; the scheduler settles that arrival once and
the shared wait stops at its due boundary, not the later horizons. The
participant receipt contains only count/status, world time and
settled sequence: it does not expose another horizon or a raw evidence Event
ID. `RPService` reuses the existing post-wait warm/initiative drain after
settlement, so replay can complete derived work. This is a **local wait-only
slice** at the storage layer, not an action-submission coordinator,
disconnect/resume policy, or full wage/obligation scheduler acceptance.
Transport verification is recorded below; F3 remains open.

## HTTP/MCP shared-round transport increment

Expose four additive authenticated POST operations: an operator-only round
open, and participant-bound round read, wait-horizon submit, and advance.
`httpapi` binds every body principal to the bearer identity before calling
storage; the storage owner rechecks exact session, entity, world and control
generation. The MCP bridge exposes only the three participant operations,
never operator enrollment/assignment/open or raw AgentKnowledge. A model must
receive its round ID from its operator/controller trigger; no unauthenticated
or high-frequency polling discovery is added. Strict request schemas exclude
an arbitrary endpoint, bearer override, canonical other-actor IDs and raw
wait evidence. A real HTTP fixture with two distinct service credentials
proves A cannot act as B or Human, a missing Human cannot advance, all three
agreed ten-minute waits commit only one world-time Event, and exact replay
works after service reopen. Actual MCP stdio-to-HTTP exercise now repeats the
successful shared round with two independent service connections, a Human
connection and an operator-opened round. A disconnects after submitting its
horizon; B settles the accepted work after Human submits, and A's new process
recovers the same settlement. Each scripted service client then commits two successive
own-character speeches through MCP. They meet in one real place; B receives
A's audible speech there, then stops receiving face-to-face speech after
leaving. `DiscoverRPBindings` lists exact active service ownership
and hides an externally assigned Entity from a displaced player. A local
stdin operator CLI calls the existing sourced enrollment/assignment commands;
it does not mint or store bearer tokens. The [transport contract](protocol.md)
records routes and recovery. At this historical transport increment,
coordination was still wait-only; the later speech-action window is recorded
below. Different-controller rotation and actual live-provider decisions
remain outside the proven slice.

## Accepted-command handoff increment

Typed committed commands now perform an exact original-principal/session/key
lookup before requiring the current generation. This covers move, speech,
social, journey start/cancel, completed wait, and settled turn; terminal
interaction and exact session-open receipts are likewise readable after a
quiescent assignment. The cancellation path uses a command-specific replay
authorization hook in the shared private-fact executor; other domains retain
their original pre-lookup authorization. Fresh or unfinished proposals still
require current ownership. Because an interaction may have a committed child
while its orchestration row remains open, closing the RP session or assigning
its Entity now refuses an open/paused interaction. This preserves a path for
the original owner to finish or explicitly stop it before transfer. The
broader handoff contract for live in-flight speaker takeover is not proved.

## Monotone controller lifecycle increment

Migration039 adds an `active`/`released` state to the sourced authority row.
First assignment records generation 1; operator-only release retains the row
as generation 2 with no active owner; exact expected-generation reassignment
of the same enrolled service advances to generation 3. New sessions pin the
current generation. Old service sessions close on release and their fresh
proposals remain fenced even if the same principal reacquires control; exact
committed receipts remain readable. The replay projection folds ordered
assignment/release Events, detects damaged status/generation and repairs them.
An actual 038→039 upgrade preserves an existing generation-one assignment.

Release refuses pending turn/wait work, open/paused interaction plans and
active shared rounds, but it does not cancel accepted scheduled journeys.
One disposable-world test follows a service-started timed journey through
release and a generation-two Human wait to exactly one arrival. Another
settles a shared round before release and then recovers the original service
participant's read, wait and advance receipts. Terminal round views pin their
time to the settled wait Event (or the stale baseline), not the live clock;
an old participant cannot observe later time changes through a receipt.
The local operator CLI exposes
`release`; MCP deliberately has no such tool. This is **explicit** release to
internal AI, not automatic failover on stdio disconnect. Rotation to another
principal/controller instance, action decision windows and live model quality
remain unproved.

## Next: shared action decision windows

The wait-only round cannot be generalized by writing an ordinary Move/Speech
Event as soon as one model submits: that changes the pinned head before Human
or the other resident has answered. The next increment will persist a private
proposal from each participant at one observed baseline. No proposal itself
is a world fact. Every participant, including Human, must respond before any
round-owned action or clock advance. Each participant may submit one action
or a wait horizon with an exact key; changing a committed submission conflicts.

At an action boundary, settle at most **one** proposed action through its
existing typed command owner. The choice must be deterministic and fair across
eligible participants rather than arrival-order driven; a persistent selected
session/action key makes crash/retry resume the same child. Other proposals
receive an explicit no-effect/deferred disposition, not a silent rebase to the
new head. Everyone reobserves and may submit again in the next round. This
avoids executing a second action against world changes its controller had
not observed. If all proposals are waits, use the existing nearest horizon /
scheduled-event rule and one Human-owned wait Event. A missing Human response
leaves the decision window open; a fast model cannot gain extra world time.

The first vertical slice should bind one service speech action plus Human
wait, prove no Event before Human submission, exactly one sourced speech after
selection, exact-key recovery after accepted speech/response loss, and no
second wait Event. Then add conflicting A/B actions, fair selection across
successive rounds, true spatial validation, disconnect, and HTTP/MCP clients.
Participant receipts must expose only own action outcome plus shared safe
status, never another model's proposal or raw AgentKnowledge identity.

The speech-action vertical slice is implemented by migration040 and
authenticated HTTP/MCP routes: private speech proposals are collected before
Human's response, one selected typed speech is committed at unchanged world
time, and other proposals receive own-only deferred dispositions. The old
wait path remains intact. The storage fixture interrupts after the selected
Event and recovers on reopen; HTTP exercises two conflicting service
proposals over two rounds, and real MCP stdio exercises one conflict.
Non-speech action proposals, further in-flight handoff and live-provider
decisions remain unproved; this is not an F3 exit claim.

## Selected speech's adopted NPC turn

The former round settled immediately after typed `SpeakRP`; that recorded
hearing but omitted `RunRPTurn` listener dispositions and NPC decisions.
The implementation keeps the speech Event as the
selected action's single source-backed acceptance point. After it commits,
create/resume an RP turn intent **adopting that exact speech command key**;
`RunRPTurn` replays the same `SpeakRP` receipt, then drains NPC decisions and
narrative. A selected round's turn may bypass the ordinary new-turn window
guard only when its exact pinned request and already committed speech Event
match the round/session/baseline. It may not start a different speech. The
round remains `advancing` until the adopted turn settles; a failure after
speech or during provider work resumes the accepted turn without duplicate
speech or NPC effects. `RPService` supplies its configured decision provider;
local Store tests use the deterministic provider. Public round receipts keep
only safe own disposition and the speaker Event sequence, not listener or
NPC IDs. A focused co-location fixture proves one Cai reply, skips Human
and the other externally controlled listener, recovers after a speech-stage
failure and database reopen, and passes CompareProjections. This is a
deterministic/local provider test, not live-provider quality evidence.

## Shared-time action/wait fairness rule

One action boundary may settle at a given world-time while Human chooses to
wait. If another round at that same world-time again has a Human wait and
external action proposal(s), the next boundary is a wait: use the earliest
submitted **wait** horizon or pending scheduler item, and explicitly defer
the action proposals. This prevents a faster external controller from
starving time and already accepted obligations by submitting speech every
round. A Human speech/action proposal is not silently converted into a wait;
it keeps the action branch and ordinary deterministic Entity rotation.
Missing Human still blocks all settlement. An already-due scheduler item at
the baseline also takes priority when Human authorized a wait. A forced wait
must ignore the empty horizon fields of action proposers and keep their
receipts `deferred_no_effect`, not `wait_completed`. This is a bounded policy
for Human-wait windows, not a wall-clock lease, model quota or autonomous
substitution. The authenticated HTTP fixture proves two same-time action
rounds with Human explicitly acting on the second, then a third round where
Human waits and repeated model speech is deferred to one wait Event. A
separate real-SQLite mixed-round fixture now proves the same deferral when a
sourced Human journey arrival is due before the submitted wait horizon: the
round stops at that arrival, emits one wait Event, and does not commit another
model speech. Live Provider evidence remains open.

## Shared immediate movement and accepted-key review

Migration041 extends the existing private action table with `action_kind`
and stores the selected kind separately; the legacy 040 `settlement_kind=speech`
marks the action branch for compatibility, while the typed kind determines
whether its completion Event is `RPSpeechAccepted` or `RPPlayerMoved`. The
ordinary immediate `MoveRP` validates origin, route, time, generation, active
journey and cursor at the selected commit. Submission also checks the current
observed origin and immediate route, so a known-invalid move cannot pin an
advancing round indefinitely. A move is private until the Human and all
external residents submit; exactly one typed action wins by the existing
Entity rotation. Other proposals are explicitly deferred. An interrupted
selected move resumes its exact original child after database reopen; neither
movement nor time is duplicated. An intervening Event before typed acceptance
stales the window rather than silently rebasing the proposal. Timed journeys
and social actions are not yet selectable round types.

Read-only accepted-key audit found that retirement, session-scoped style and
interaction-mode revisions still checked current authority before their
historical exact-key receipts. They now return only the original principal's
exact accepted result after a generation handoff; fresh keys still require
current control. A currently held generation with a revoked player grant
remains denied. The derived move key is reserved for the selected round before
`MoveRP` creates its command, so an early `RetireRPRequest(move)` cannot strand
that round. Tests cover wrong-principal and altered-payload denial, restart,
040→041 upgrade, one Human-gated movement, conflict with speech, Human wait
priority, transport and projection parity. Exact map-survey replay now joins
the accepted-key contract; a fresh map survey remains controller-fenced.
Replacement/assignment cannot change the Event head of another active
shared round, and replacement cannot interrupt an open interaction. No
live-provider proof is implied.

## Wait acceptance and derived-key recovery review

A separate contract review found that an advancing wait could be selected
before its Human wait intent existed; an unrelated Event in that gap left a
permanent active slot. Recovery now marks that unaccepted window stale when its
baseline changes. Once the exact intent exists, scheduler Events may advance
the head without staling accepted work. The operator cannot open a round whose
derived Human wait or participant action/turn key was already retired **or
accepted** by another command, and an active round reserves those keys even
before any private proposal is submitted. Submissions also check retirement
inside the same acceptance transaction.

The scheduler `budget` of `rounds/advance` is a per-call work limit, not an
immutable part of the shared Human wait's identity. Shared waits normalize only
that budget for new child hashes, while retaining the exact legacy hash when
resuming a previously accepted wait whose budget was included. Ordinary direct
`WaitRP` keeps its exact-budget idempotency contract. Focused tests interrupt
wait selection, change the head, reopen, recover the active slot, reject
pre-retired and preaccepted child keys, and retry current/legacy accepted
waits with a different budget. This repairs local recovery; it does not supply
two real model-provider decisions.
