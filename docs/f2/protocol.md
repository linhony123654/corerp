# F2 spatial API (additive, local preview)

The existing `corerp.client.v1` session, observation cursor, authentication,
idempotency and event-continuation rules remain in force. These endpoints are
`POST /api/v1/rp/...`, accept JSON, and return the normal `{ "data": ... }`
envelope or a typed error. The authenticated principal is bound by the server;
never take it from model text. A new action uses a fresh observation cursor;
an uncertain reply is retried with the **same** body/key/cursor.

| Route | Authority | Request | Sourced effect/read |
| --- | --- | --- | --- |
| `locations/materialize` | scoped world creator | `binding`, `parent_location_id`, `slot_key`, bounded `candidate` | one immutable child slot/Event; no residents/assets |
| `locations/rename` | scoped world creator | `binding`, `location_id`, `new_name` | display name changes; stable ID remains |
| `edges/define` | scoped world creator | `binding`, origin/destination/segment IDs, `duration_minutes` | timed overlay on an actual directed route |
| `perception/links/define` | scoped world creator | `binding`, place, zones, barrier, ranges | visual/audio barrier source |
| `perception/zones/place` | scoped world creator | `binding`, actor, place, zone | zone choice tied to actual place entry |
| `journeys/start` | session controller | same body as `actions/move` | enters actual segment now; schedules arrival |
| `journeys/cancel` | session controller | session, `journey_id`, cursor, key | stops automatic arrival; stays in segment |
| `map/survey` | session controller | session, cursor, key | private dated local map Event |
| `map/read` | session controller | session | latest saved note per visited place, not live authority |

`observe` adds optional `active_journey` (`journey_id`, origin, destination,
segment, scheduled arrival) and `reachable_places[].can_start_journey` /
`travel_minutes`. `can_move_now` is false on a timed edge: callers must use
`journeys/start`. `active_journey` remains after reload, and a cancelled
journey no longer appears there. Waiting advances the world scheduler toward
arrival; it does not make a traveller present at both endpoints.

`present_entities` is filtered by actual visibility. Unknown identities use
`display_name: "陌生人"`, `identified: false`, and a stable observer/world-scoped
`person_…` handle rather than an authoritative entity ID. The handle is
derived with a database-persistent private presentation key, so public IDs
alone cannot reproduce or enumerate it; the key is not a world fact or API
field. A copied/reopened database keeps handles, while an Event-only rebuild
may assign new presentation handles without changing world authority. It is
accepted as an interpersonal target and context subject; once a lawful
introduction is heard, the same scene uses the actual entity ID and name.
Guessing a raw ID for an unfamiliar target/subject returns not found, and a
targeted gesture requires actual visual perception (not merely co-location).
Context/event evidence IDs for unknown people are opaque `evidence_…`
references. Turn narration labels an unintroduced respondent “陌生人”. Sight
and hearing alone never grant a name. Accepted speech may declare `delivery_channel` (`voice`,
`whisper`, `shout`) and `introduce_self: true`; only actual hearers acquire
identity from the self-introduction. Speech action responses do not expose the
internal listener list: hearing does not reveal who heard it. Player event continuation returns safe
own-journey summaries, not raw scheduler/occupancy or co-located entity lists.

The saved map is deliberately a belief: an old note can still show a route or
name after roadworks/rename. Check `observe` and submit a real movement command
to obtain current permission. `map/read` does not survey or advance time.

Current F2 preview limitations: a cancelled trip returns to its origin only
through a later explicit move; the F1 natural-language interaction parser
does not yet orchestrate a timed journey. Creator construction endpoints and
new RP session endpoints are exposed over HTTP; MCP exposes resident journey
and map tools, not creator authority. The older capability-protected
`world.agent.knowledge.read` endpoint still supplies authoritative subject and
source IDs to the career-referral workflow; it is not a player RP view and
must not be exposed as one. Current MCP tools call only RP session/context
routes and do not expose this AgentKnowledge endpoint. The HTTP route is still
callable by a principal holding its specific capability. Before F3 connects
an external model/controller to an Agent principal, enforce
principal-and-purpose filtering at the HTTP/service boundary so that a model
without identity knowledge cannot request raw subject/source IDs merely
because a physical observation exists or a referral grant is present. This
is a hard F3 entry condition; F2 does not redesign career-referral
credentials. The legacy demo also has person-named place labels/IDs (for
example `Ada Home`), which may provide contextual clues even
when the occupant remains formally unintroduced. These are compatibility
limits, not evidence that sight grants familiarity. The local F2 gate and its
remaining limits are recorded in `phase-report.md`.
