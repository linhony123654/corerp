# F2 phase report — shared spatial authority

Status: **local F2 gate passed; this report is included in the F2 checkpoint**.
F1 source checkpoint:
`dba744d515ca9dcc3a274fea9cfbf8866fe15edf`. This is a reviewed local
result, not a release.

## Delivered boundary

- Migrations 032–035 add scoped stable location nodes/slots, timed edges and
  journey state, sourced perception zones/barriers and identity familiarity.
  Existing movement Events and positions remain the authority for occupancy;
  half-open intervals are derived, not a second location engine. A lazy slot
  accepts one candidate per `(instance, branch, parent, slot)` after an
  out-of-transaction preparation and in-transaction recheck. Renames do not
  change the stable ID; materialization never copies residents or assets.
- A timed departure enters an interactable segment, schedules arrival, and
  excludes origin/destination presence until an accepted relocation. RP5
  roadworks delay departure or arrival on the pinned route, even when an old
  durationless alternative exists. Cancellation leaves the actor in the
  segment; explicit return/alternate movement supplies rerouting. A later
  appointment cannot teleport an actor out of an active journey: it records
  sourced `journey_occupancy` deferral until after the current ETA. Journey
  state, queue/pointer state and occupancy are audited against Events and
  repaired only when source evidence suffices.
- Visual, audio, channel, range and obstruction checks are independent of
  co-location and identity. Ordinary RP/encounter views of unfamiliar people
  use stable observer-scoped `person_` handles and `evidence_` references;
  these use a database-persistent private HMAC key rather than publicly
  computable hashes. The key is presentation state, not world/Event authority.
  Guessed raw targets are rejected and targeted gestures require actual sight.
  Identity changes only on declared acquaintance or an actually heard
  self-introduction. Speech responses do not expose their internal hearer
  list. Actor map notes are private dated beliefs and may remain stale after
  route/name changes.
- Authenticated HTTP, Play and MCP expose additive journey/map paths. Play
  restores one committed journey after lost response/restart, displays the
  actual segment and cancels via a sourced command; it does not invent a
  second map or movement authority. Existing durationless movement remains.

## Verification ledger

| Gate | Evidence | Status |
| --- | --- | --- |
| Lazy location scope/concurrency/restart/rename | Three DB clients, exact-key and rollback, two independent Studio worlds, foreign-parent/branch denial, Compare→Rebuild→Compare | PASS, focused storage |
| Real travel/occupancy/RP5 chronology | Segment overlap and non-overlap, roadworks with alternate old route, later appointment deferral, cancel→return→alternate, restart and projection queue corruption/repair | PASS, focused storage |
| Perception/identity/map | Door/wall/range/channel negative cases, unfamiliar actor alias and guessed-ID rejection, intro transition, context/events/narrative, stale map after roadworks/restart | PASS, focused storage/HTTP |
| Long economic/social regression | 14-day opportunity divergence and real gift accounting | PASS, 134.866s |
| Month-long world mechanics | Focused `TestFinalWorldMonthLongRunMechanics -timeout=15m` timed out without an assertion result; the unchanged case ran within the clean final-source full storage suite under the 50m deadline | PASS in full suite; focused 15m run INCONCLUSIVE |
| Full backend regression | Clean final-source `/usr/local/go/bin/go test ./... -count=1 -timeout=50m`: storage 1295.688s, HTTP 16.123s, all other packages passed. A prior full run failed only an obsolete raw-ID context assertion; that test was corrected to require guessed-ID rejection and lawful-alias empty history before this clean rerun. | PASS |
| Static checks and frontend | Scoped `go vet`, `git diff --check`, `npm run build` | PASS |
| Concurrent-path race checks | Final-source `go test -race` focused on private alias key, location one-slot/three clients, identity, perception, timed journey and HTTP anonymous social boundary | PASS; storage 17.978s, HTTP 3.713s |
| Actual Play browser | Final-source legacy, contacts, F1 interaction, route/roadworks map and F2 journey/restart/cancel | PASS; one concurrent-port attempt invalidated and rerun serially |
| Actual MCP stdio → shared HTTP world | Final-source `clients/mcp/npm test`: SDK integration with old tools, mixed actions, F2 map/journey, and origin/credential/error boundary | PASS 2/2 |

## Scoped limits and next stage

The old explicitly capability-authorized `world.agent.knowledge.read` path
retains stable Entity/Event IDs for career referral. It is not an ordinary
player/Agent perception view; current MCP tools do not expose it, but the
HTTP route remains callable by a principal holding its specific capability.
Per the user's boundary decision, F2 does not redesign referral credentials.
Before F3 connects an external model/Agent controller to any such read,
enforce principal-and-purpose filtering at the HTTP/service boundary so a
career-referral grant is not reused to release raw identity to the model.
Physical observation alone is insufficient. Treat this as a hard F3 entry
condition.

The legacy demo's person-named place labels/IDs (such as `Ada Home`) can offer
contextual clues; scene sight still does not grant formal familiarity. There
is no product same-instance branch-fork operation, so the positive branch
fork fixture is unavailable; scoped constraints, two independent worlds and
foreign-branch denial are verified. F1's natural-language parser does not yet
orchestrate timed journeys. These limits are explicit, not silently counted as
tests passed.

The local F2 gate passes with the scoped limitations above. No publication or
live external-model validation is claimed. Record the F2 source checkpoint
before F3 implementation; F3 must first enforce the AgentKnowledge
principal-and-purpose boundary described above.
