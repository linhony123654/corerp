# F7 design — sourced information delivery and belief

Status: **direct-message, stance, rumor, Career organization and law-sourced public notice slices focused-tested; shared publication plus opaque source discovery added; F7 gate OPEN**. Baseline is the
clean F6 checkpoint `dc056888b9cdb26abce2f082fa012feb94972c14` in the
independent `f7-information` worktree. No F7 gate, checkpoint, push or release.

## Purpose and observable acceptance

Knowledge must travel through actual channels instead of appearing whenever
the underlying world fact becomes true. The minimum contract covers existing
face-to-face speech, private direct/phone-like messages, manager-authorized
organization announcements, a public notice/news-like publication, and a
recipient's interpersonal rumor relay. Each transmission must preserve source,
sender, channel, intended audience, actual recipients, claim text, world time,
visibility, forwarded-from lineage and stated reliability; recipient belief is
not automatically authoritative truth.

1. A private work decision is initially known only to authorized management
   and any employee directly notified by the Career transaction. A direct
   message or announcement changes other people's knowledge only after their
   actual delivery/access. Unaddressed coworkers and the public learn nothing
   merely from the decision Event. Check with a real Career-sourced work
   announcement and bounded own/other reads.
2. A known contact can send a private claim across locations. The send Event
   alone does not teach the recipient; a later world-time delivery Event does.
   Offline/reopened recipients can read the delivered claim without the read
   itself mutating the world. Wrong principal, unknown recipient, cross-branch,
   stale head, mismatched idempotency and active F3 round are denied.
3. A recipient may relay a heard claim as a rumor only from its own sourced
   learning chain. The relay retains the earlier sender/learning source, not a
   fabricated firsthand observation. A confidentiality policy blocks widening
   a private or organization-confidential audience without authorization.
   A false claim may still be delivered and relayed.
4. Two recipients can record different bounded stances (believe/doubt/reject)
   on the same claim, and a later correction changes current stance without
   deleting earlier receipt or belief history. A no-message third party stays
   unaware. No belief write changes the underlying Career/world truth.
5. A public notice is published to a real channel and learned only upon lawful
   access/delivery, never injected into every NPC from Debug Truth. Face-to-face
   speech remains compatible with the common information view. Check audience
   filtering, private leakage, later delivery, restart, Compare/Rebuild and
   authenticated client reads on a disposable world.

Not in F7: social-media feeds, an omniscient search index, arbitrary private
Career payload forwarding, external telephony, production publication, or
redesign of F3/F5/F6 authorities.

## Verified reuse and gaps

- `RPSpeechAccepted` and `insertRPSpeechHearings` commit co-located listeners,
  `observation_records` and `agent_knowledge` atomically. `ReadRPEvents` reads
  immutable observation evidence, and its own comment requires a *new learning
  Event* for delayed delivery. This is the template for attribution, not a
  remote message protocol.
- `ReadRPMessages` projects only own Career transaction receipts. It is
  explicitly not SMS or an ambient conversation ledger. The generic `outbox`
  publishes application notifications; it is not authoritative in-world
  delivery or belief.
- `SpeakCareerAnnouncement` currently speaks effective position facts to
  co-located listeners. `AnnounceRPLaw` similarly teaches only actual hearers;
  authoritative effective law is read separately from known law. Those paths
  demonstrate the truth/knowledge boundary but not an organization-wide
  broadcast.
- Migration 007 restricts `observation_records.channel` to `co_location` and
  requires place and subject references. `agent_knowledge` is a latest-state
  projection while observations preserve learning history. `ReadRPContext`,
  `ReadRPEvents`, contacts and `RPLifeContext` whitelist existing claim kinds,
  so new delivery claims require explicit safe reader changes and a compatible
  schema migration, not a raw Event dump.
- The world scheduler has branch/time-ordered items. `RunAgentLife` does not
  advance an idle world clock by itself; normal RP wait owns time advancement.
  A scheduled delivery phase must be deliberately allowed by the per-world
  dispatch guard, consume a sourced queue item, write a delivery Event, and
  survive replay. A read must not impersonate that delivery.
- Direct targeting must use current RP control and an actually known contact
  (`rp_identity_familiarity`/contacts); it must not let a model enumerate or
  guess canonical Entity IDs from physical perception. Internal source IDs
  may be pinned for validation but are not generic external identity grants.

## Proposed minimum architecture and increment order

Keep immutable transmission and delivery Events as authority; derive recipient
observations and current knowledge. A transmission envelope carries an opaque
message ID, origin/forwarded source, sender, channel, intended audience,
bounded content, confidentiality and claimed reliability. A delivery Event
records the actual recipient and time. An independently sourced stance Event
records a recipient's interpretation; neither stance nor the claim rewrites
truth. Do not create a belief automatically from message receipt.

First vertical slice: a known-contact direct message sent while recipients
are in different places, scheduled for later world time, then delivered through
normal RP time advance. Verify recipient-only knowledge/event read, sender
receipt, no pre-delivery knowledge, restart and Compare/Rebuild before adding
relays or broader audiences. Next add guarded forwarding and stance/correction;
then a manager-authorized Career work announcement and public notice with
audience selection. Bring in authenticated HTTP/MCP only through the existing
RP authority boundary, with per-recipient DTOs that omit source IDs/secret
audiences where not authorized.

## First-slice decisions and remaining stage risk

- Add migration 047 to widen `observation_records.channel` for in-world
  delivery; rebuild its `agent_knowledge` child in the same deferred-FK
  migration, preserve all existing rows and indexes, and verify a populated
  046 world upgrade. A separate delivery silo would fail the common Knowledge
  path; falsely labelling a remote message `co_location` is not acceptable.
- Send is a current RP-session command with current controller generation,
  observed branch head, known-contact identity, exact idempotency, pending
  action and F3 active-round checks. The first slice allows only a solo Human
  window; later typed shared-action ingress is required before external
  residents may send. No guessed stable Entity ID becomes a contact.
- Persist a private `RPInformationSent` Event and a scoped scheduled item at
  send. The scheduler verifies the immutable source and exact queue fields,
  writes a private `RPInformationDelivered` Event, then inserts recipient-only
  observation/knowledge in the same transaction. `ReadRPEvents` uses the
  *delivery* sequence, so a recipient offline during send still learns later.
  Scheduled delivery is passive world work and may run during a shared wait;
  the active shared round bars new sends, not due background work. The item
  carries the correct world-day derived from the RP epoch, including midnight.
- For the later business integration, `EndCareerEmployment` has a real
  manager-authorized future `layoff` decision; existing Career messages notify
  the affected employee but not unrelated coworkers. An organization/public
  F7 announcement can source this Event and expose only bounded public
  workforce-change terms, not the private employee notice, manager assessment
  or raw Career payload. Its exact lawful audience/policy remains a stage-B
  investigation before that later increment.

The first slice now has `POST /api/v1/rp/information/direct/send` with bearer
principal binding. Its response contains only the caller's message ID, delivery
due world time, event sequence and replay flag; the source Event ID, sender and
recipient canonical IDs, private text and delivery place are not echoed in the
receipt. The existing authenticated RP context/event readers expose a bounded
recipient claim after delivery, not at send time. Focused storage tests cover
populated 046→047 migration, cross-location delivery after restart, source-based
queue/observation repair (pending and delivered), wrong principal/contact,
idempotency, active F3 round denial and passive delivery during shared wait.
The HTTP slice checks auth, recipient selection, replay and receipt privacy.

Delivered direct messages also enter the recipient Agent's bounded decision
context under `life.information`, distinct from relationship trust, salient
speech memory and authoritative world facts. Each item carries claim text,
channel, stated reliability, delivery time, an observer-scoped sender handle
and a display name only if the recipient knows that identity. It never carries
canonical sender/recipient or raw source Event IDs. The reader checks the
recipient's Knowledge and Observation against the immutable send/delivery
Event chain before exposing content to a model. The old face-to-face readers
exclude information-channel rows so a corrupted claim cannot masquerade as
heard speech. Receipt remains `unverified`; no stance or automatic belief is
created.

This first-slice account is retained for provenance, not as the current gate
state. The later sections implement public publication, bounded client reads,
Career integration and different recipient stances. F7 still requires typed
shared-action ingress and a broader organization audience regression. Fresh
information actions remain solo-only while an external controller is active.

## Stance and correction increment

Use the already branch-unique message ID as the recipient's client reference;
do not require a raw send or delivery Event ID in the request. A controlled
recipient may record `believe`, `doubt` or `reject` only after a real delivery
has created its own sourced Knowledge. A private immutable stance Event pins
the send and delivery Events internally, the recipient, the message ID and the
previous stance Event. A later stance is an append-only correction: current
belief is the last valid source Event, while earlier stances remain in history.
The claim and the underlying Career/world truth are never rewritten by a
stance. Fresh stance writes obey the same current session/head/F3-window
guards as private send; exact retries do not create a second belief.

The stance slice is implemented: `RecordRPInformationStance` sources only a
delivered claim in the controlled recipient's Knowledge, appends a private
Event with the previous-stance pointer, and exposes the derived current stance
in both `life.information` and the recipient's authenticated RP context.
`POST /api/v1/rp/information/stance/record` binds bearer principal and returns
only message ID, stance, sequence, world time and replay status. Tests verify
pre-delivery/foreign/guessed-message denial, exact replay, believe→doubt,
restart, unchanged `unverified` reliability, an unaware third party, and
source-chain corruption detection by CompareProjections. The HTTP fixture
checks recipient context and receipt privacy with a disposable test-only
second control grant. Relay and multiple-recipient divergence follow; a
private claim cannot be forwarded merely because its recipient believes it.

## Explicit one-hop interpersonal rumor increment

`SendRPInformation` now accepts `allow_relay` (default false). Only the actual
recipient of a delivered direct message with this explicit sender permission
may call `RelayRPInformation` / `POST /api/v1/rp/information/rumor/relay` to
repeat the exact claim to one known contact. The new private `rumor` send Event
pins both the earlier send and delivery Event IDs, schedules a fresh delivery,
and cannot be relayed a second time. A false or unverified claim can travel
without gaining authority; believing it does not grant forwarding permission.
Recipients see `forwarded` and `may_relay`, never raw lineage IDs; the HTTP
receipt omits the claim, recipient and source pointers. Permission is not
retroactive and does not authorize an organization/public broadcast.

Storage and authenticated HTTP tests cover denied private and undelivered
sources, guessed IDs, exact replay, delayed recipient-only delivery, two
different stances, an unaware third party, second-hop rejection, restart,
CompareProjections and tampered lineage. The demo's only sourced known-contact
pair is Lin↔Cai, so the representative relay returns to Lin; a broader
multi-person scenario remains for the Career announcement integration.

## Career-sourced organization notice — implemented increment

An authorized manager may publish one organization-scoped announcement only
from an actual prior `EndCareerEmployment(kind=layoff)` Event in the same
world. The public claim is generated from the layoff kind and effective day;
the private employee notice, employee/contract/compensation identifiers and
assessment payload are never copied to the notice. Publishing records an
immutable announcement source and its intended audience policy (active
employees of that organization), but creates no recipient Knowledge.
An eligible employee must actually access the notice; that action records a
new private delivery/learning Event, observation and Knowledge. Access is
idempotent and denied for nonemployees or the wrong world; post-employment
access requires separate policy and is not implied by an old membership.
This is the same truth-versus-knowledge boundary as delayed direct delivery,
without fabricating an organization-wide NPC read at publication time.

`PublishRPOrganizationNotice` and the authenticated
`/api/v1/rp/information/organization/publish` route require the same manager
principal that issued the real Career layoff source, still authorized for that
organization, publishing as its own active Person. The immutable publication
has `visibility=organization` and `claimed_reliability=official_statement`;
its message is derived only from the effective day. It contains no employee,
contract, wage or private notice text. An active employee can discover only
the notice handle/time through `organization/list`, then deliberately call
`organization/access` to create the private recipient Event and Knowledge.
The access Event pins the effective employment contract and terms Event so
Compare/Rebuild can validate that audience as of receipt even after a later
lawful termination. Exact retries are reauthorized; former employees lose
fresh access, while previously learned information remains historical.
The original single-employee source/replay fixture is complemented by a real
two-employee Career fixture. A capacity-two posting produces separate Ada and
Bo contracts under Lin's management; a Bo layoff sources one redacted notice.
Listing teaches neither employee, Ada's actual access teaches only Ada, and
Bo's later access teaches Bo independently. Ada doubts and Bo believes the
same claim while nonemployee Cai cannot list/access it. After the sourced
layoff takes effect, Bo loses fresh access but retains the historical receipt;
Ada remains an eligible employee. Projection comparison stays clean before
and after the layoff. External shared-action ingress remains F7 work.

## Law-sourced public notice — implemented increment

The `public_notice` channel has a different audience from a confidential
organization announcement. Only the principal who issued a real
`RPInstitutionFactRecorded(kind=law_enactment)` may publish it, while still
holding that institution's legislative capability and acting as the recorded
legislator Person. The text is deterministically derived from the enacted law
ID, effective time and public law text (or repeal), never from a raw caller
payload, a proposal alone, Debug Truth or a private Career notice. The send
Event records a public intended audience but writes no observer Knowledge.

Any currently controlled Person in the world can list bounded handle/time
metadata through `public/list`. A separate `public/access` action creates its
own recipient `RPInformationDelivered` Event, observation and Knowledge. The
actual recipient can then see the claim in RP context/events and in
`life.information` with `official_statement` as claimed reliability, not an
automatic belief or a silent grant of authoritative `RPKnownLaw`. Recipient
stance remains an independent append-only correction chain. Publication,
list and access have authenticated HTTP routes under
`/api/v1/rp/information/public/`; receipts omit law source IDs, Person IDs and
claim text. Event→observation/knowledge Compare/Rebuild and restart validate
the source and exact recipients. A real-law fixture proves Lin doubts, Cai
believes, and Bo remains unaware; no publication/list read teaches them.

Fresh public access and publication now have separate F3 typed shared-action
ingress; legacy publication remains solo-only with an external controller.
The public board remains bounded to the latest 20 without pagination. The F7
stage gate remains open.

## F3 typed information ingress — direct-send slice implemented

F3 round proposals are application state; they are not an alternate authority
for `RPInformationSent`. Extend the closed shared-round action and settlement
checks in a populated FK-safe migration, then accept a bounded
`information_send` proposal by a current round participant. The proposal
records the exact child `SendRPInformation` request with the round baseline
head and derived `shared_action_<round>` key. Selection invokes that existing
command, whose own transaction verifies the exact selected request before it
may bypass the solo/external-controller guard. Event-before-receipt recovery
must replay that same child and settle the round; direct bypass, changed text,
recipient, relay consent or principal must fail. The first slice now uses
migration 048 to rebuild the populated round→participant→proposal FK chain
and add distinct `information_send` / `information` action/settlement kinds.
A real Human+external fixture verifies no premature Event, payload-change and
direct-bypass denial, selected-child interruption, Event-before-receipt
restart, one accepted send and later shared-wait delivery to the actual
recipient. Compare/Rebuild and authenticated HTTP acceptance pass. The MCP
tool is registered and its missing-round route is tested, but a successful
live model-originated send is not claimed. Relay and notice
access/publication are covered by later typed slices below.

The second typed slice, `information_stance`, is implemented: an already delivered recipient
may propose a bounded `RecordRPInformationStance` request in a current shared
round. Migration 049 preserves populated 048 rounds and permits this action kind, while
the existing `information` settlement remains distinct from speech/health.
The proposal pins the exact child request; selection calls the unchanged
stance Event owner, and only its exact selected child bypasses the solo action
window. Storage tests reject an unrelated participant, unread/guessed message,
changed stance or reason, direct call during an active round and external
controller before selection. The Event-before-receipt restart settles once,
with the recipient-only source chain still clean under CompareProjections.
An authenticated HTTP fixture proves shared send→actual later delivery→Human
stance proposal→external wait→one accepted stance Event, with bounded round
receipts. The MCP tool's schema/routing and missing-round denial pass; neither
a live model-originated stance nor a successful stance through MCP is claimed.
Organization/public publication and access are covered by later typed slices
below; the full stage gate remains separate.

The third typed action, `information_relay`, is implemented. Only a current
round participant who actually received a `direct_message` with explicit
`allow_relay` may propose repeating that exact claim to a known distinct
contact. The proposal is application state and pins the original
`RelayRPInformation` child; it cannot edit the claim, fabricate firsthand
observation, broaden organization or public audiences, or relay a rumor a
second time. Migration 050 rebuilds the populated round→participant→proposal
FK chain to add the action kind. Selection calls the existing relay Event owner
under an exact request/window check; Event-before-receipt restart settles one
private `RPInformationSent(channel=rumor)`. Only later scheduled delivery
teaches the actual recipient. Storage and authenticated HTTP fixtures verify
consent, unread/guessed source, Human gate, direct bypass, payload pinning,
recipient isolation, restart and clean CompareProjections. The MCP tool's
schema/routing and missing-round denial pass, but neither a successful relay
through MCP nor a live model-originated relay is claimed. Organization/public
publication and access are covered by later typed slices below; this paragraph
records the relay increment's original boundary.

The fourth typed action, `information_public_access`, is implemented. A
participant who sees a real published public notice may propose access at
the shared baseline; the proposal alone creates no Knowledge. After Human
submission and selection, only the pinned `AccessRPPublicNotice` child may
create that recipient's `RPInformationDelivered` Event. The existing Event
writer still verifies current controller, law-sourced notice, distinct reader
and absence of an earlier delivery. Migration 051 preserves populated F3
rounds while admitting the new action kind. Storage and authenticated HTTP
fixtures cover direct bypass, changing to a second valid notice after
selection, Event-before-receipt recovery, recipient isolation and clean
CompareProjections. MCP registration/routing and missing-round denial pass;
a successful MCP/model-originated public access is not claimed. Organization
access and scoped organization/public publication are covered by later typed
slices below.

The fifth typed action, `information_organization_access`, is implemented.
Only a currently eligible employee can propose reading a real organization
notice. Selection pins the existing `AccessRPOrganizationNotice` child, which
rechecks the actual current employment contract, its terms Event, source and
controller before committing the recipient's delivery Event. Proposal/list
alone creates no Knowledge. Migration 052 preserves populated F3 rounds and
admits this action. Storage and authenticated HTTP fixtures cover nonemployee
denial, Human gate, changed valid notice after selection, Event-before-receipt
recovery, private receipt, recipient isolation, and denial of fresh access
after sourced layoff while historical delivery remains. Recovery also handles
the stronger ordering where the delivery Event commits, a real scheduled
layoff takes effect, and the round receipt is still pending: it verifies the
exact committed command hash and delivery Event plus the historical source
chain, then settles without reopening fresh employee access. MCP schema/routing
and missing-round denial are covered, but not a successful MCP/model action.
Organization publication is the next typed slice below.

The sixth typed action, `information_public_publish`, admits only a current
round participant controlling the legislator Person named in an actual
enacted-law Event. The proposal derives the source Agent principal from that
Event; it cannot substitute the service/player controller for the legislator
in the publication Event or inherit the legislative grant. On selection the
original `PublishRPPublicNotice` command requires the exact pinned child,
current controller/session binding, active sourced role/grant and real law
source. Migration 053 transactionally widens the populated F3 round,
participant and action FK chain. Publication alone still writes no recipient
Knowledge; only later access does. Storage and authenticated HTTP tests cover
wrong actor, unenacted source, Human gate, changed selected child, bounded
receipt, Event-before-receipt restart and clean source/projection comparison.
The authenticated source-discovery route now returns a session-bound opaque
handle and bounded public law summary only to the currently controlling
legislator. The shared HTTP/MCP proposal accepts that handle only; unknown
raw `law_event_id` input is rejected. MCP schema/routing and missing-round
denial pass, but a successful publication through MCP or a live model is not
claimed. The original solo publication request's empty optional
controller fields retain its old canonical hash/idempotency behavior.

The seventh typed action, `information_organization_publish`, follows the
same separate source-principal/controller boundary for a real manager-authored
Career layoff. The proposal derives the source manager Person and Agent
principal from the scoped immutable Career Event. Only the currently
controlling shared-round participant can select a redacted announcement; the
original publisher still rechecks the active organization-management grant,
current controller/session, exact selected child and the real layoff source.
The `RPInformationSent` Event retains the source manager as actor and omits
employee identity, contract, wage and private notice. Migration 054 preserves
populated prior F3 rounds and widens the closed action checks. Storage and
authenticated HTTP fixtures cover wrong source/speaker, Human gate, changed
selected child, bounded private receipt, Event-before-receipt restart,
recipient non-learning at publication and clean CompareProjections. A later
eligible employee access, not this publication, creates Knowledge. The
authenticated source-discovery route exposes only a session-bound opaque
handle, redacted effective-day summary and world time to the current
controlling manager; it never returns the Career Event ID or private payload.
The shared HTTP/MCP proposal accepts that handle only and rejects raw
`career_event_id` input. MCP schema/routing and missing-round denial pass;
a successful publication through MCP or a live model is not yet claimed.
