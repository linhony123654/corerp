# F2 design — shared spatial authority

Status: implemented and locally validated; see `phase-report.md` for the F2
gate, exact checks and scoped limits. F1 source checkpoint:
`dba744d515ca9dcc3a274fea9cfbf8866fe15edf`.

## Source-backed boundary

`agent_places.place_id` is already a stable opaque key; `display_name` is not a
key. `rp_place_links` supplies directed adjacency only. `agent_positions` has
exactly one place per actor, and `agent_movements` plus Events own changes.
`MoveRP` moves one adjacent edge instantly at the current world time and
creates co-location Knowledge. RP5 roadworks/`EarliestRPTransitArrival` delay
scheduled appointments but explicitly assign **no edge duration**. The
current session observation, encounter query and speech listeners all equate
same `place_id` with perception; that is insufficient for F2. Studio genesis
creates places and links from a declared skeleton, without lazy slots.

## Contract and invariants

1. The authoritative location key is `(instance_id, branch_id, location_id)`;
   the existing `agent_places.place_id` remains the concrete location ID where
   an actor can stand. Human-readable path and display name are mutable
   attributes, never identity. A location has at most one stable parent and an
   immutable logical slot key under that parent; containment is acyclic.
   Legacy places are roots. A *skeleton* location may own slots without
   materializing all children. A slot candidate may include bounded descriptive
   attributes, but no people, ownership, inventory, money, housing or cohort.
2. Topological reachability (`rp_place_links`) is not physical occupancy or
   perception. Transport edges add duration, direction, source Event and
   obstruction policy to a declared topology; existing durationless links
   retain their old behavior until explicitly marked timed. Closed roadworks
   cannot be bypassed by a hidden intermediate segment. No per-metre or
   per-node Event stream is required.
3. At world time `t`, each active actor occupies **one** concrete location.
   Every accepted relocation closes one half-open occupancy interval
   `[entered_at, exited_at)` and opens the next; same-time transitions use
   Event sequence to order boundaries. `agent_positions` is the current
   projection, and movement Events plus interval rows must agree. A journey
   has an actual source → segment → destination sequence, never simultaneous
   presence at endpoints. A segment is an ordinary interactable location:
   other actors can enter it, and an encounter requires overlapping intervals
   there. A route choice is pinned by an accepted journey Event, not redrawn
   on retries. World-time progress still belongs to the existing scheduler.
4. `same location`, `visible`, `audible`, `identified` and `known` are separate
   predicates. Visibility and audibility are evaluated from current location,
   environment, obstruction, range and channel; identity is granted only by
   explicit acquaintance/evidence. The actor-facing map is a dated belief,
   not an omniscient topology dump. Recorded observations are immutable and
   can become stale; the current world projection does not silently rewrite
   them. Never infer hearing from narrative prose or co-location alone.
5. Materialization key is `(instance, branch, parent_location_id, slot_key)`.
   Read slot → prepare bounded candidate outside write transaction → recheck
   inside immediate transaction → commit one `LocationMaterialized` Event and
   row (or return the already accepted row). A unique constraint is the final
   arbiter. No provider runs while the write lock is held. The accepted
   candidate, generator version and source Event are immutable; restart,
   replay and a new generator version return the same object. Failing before
   commit leaves no visible location, and retry is safe.

## Integration decisions

- Migration 032 is additive. Keep schema007 `agent_places`, movements and
  positions as the concrete spatial projection; add metadata/slot, transport,
  occupancy and journey tables with scope/uniqueness constraints. Do not
  rename historical IDs or rewrite old movement Events. Backfill legacy root
  metadata deterministically, and derive initial open occupancy from each
  current position's recorded movement lineage. Audit the backfill against
  Replay/Compare rather than inventing a new resident or cohort.
- Materialization is a creator-authorized command. World residents read the
  accepted location; they do not gain creator authority. A no-op exact retry
  returns the same ID; a conflicting request key is rejected. Display rename
  is a separate sourced command so historical names remain attributable.
- First vertical slice: a declared skeleton parent with one lazy child slot;
  three clients obtain the same stable location ID after concurrent
  materialization, restart and rename. This proves the new identity/source
  boundary before travel depends on it. Second slice: one timed edge with a
  real midpoint segment, two
  controlled clients and one resident. Begin journey puts the traveler in the
  segment in a committed Event; an existing scoped clock advance can later
  resolve arrival. During the interval, another actor can enter and interact.
  Exact retries, restart and replay must preserve the same interval and
  destination. `MoveRP` must not allow instant bypass of a timed edge.
- The scheduled arrival must resolve against the *pinned* journey and RP5
  roadworks, not a fresh random route. If obstructed, record a sourced delay
  and keep occupancy in the segment; reroute must commit a new accepted route
  decision. Cancellation stops automatic arrival but leaves the actor in an
  actually reachable segment; a subsequent explicit return/alternate move
  accounts for its time. A later unrelated appointment cannot create a
  second simultaneous position; chronology/supersession remains enforced.
- Perception must be integrated at each writer/reader boundary: session
  `PresentEntities`, `ResolveEncounter`, speech/listener Knowledge, movement
  and scheduled-movement co-location Knowledge, NPC decisions, community
  observations and event stream. Existing `co_location` evidence from before
  migration remains historical, not proof of future hearing. New evidence
  carries the actual channel and source. Identity shown to a client requires
  lawful identity evidence, not a raw profile join.
- APIs remain additive. The existing direct `MoveRP` and F1 interaction
  parser continue to work for durationless edges; a new journey command/read
  exposes timed travel state. The Play client may present a segment and
  travel status only after the backend vertical slice is verified. No UI may
  manufacture an encounter.

## Gate and test ledger

## Play spatial execution contract

Keep the existing paper-and-ink reading hierarchy. The current scene heading
remains the source of place; an active journey adds one quiet status line with
destination and expected arrival, never a second map authority or a floating
dashboard card. The nearby-map list distinguishes immediate movement, timed
departure (duration shown), and obstruction. A journey begins from the same
saved-intent action pipeline as Move; reloading observes the server's active
journey and may cancel only through a sourced command. Map memory is labelled
as a dated note and must not visually imply current road state. Mobile retains
44px targets, clear disabled reasons, focus visibility and no horizontal
overflow. Source IDs/unknown people are not rendered as names.

- One accepted materialization across concurrent callers/three clients;
  same ID after lost reply, crash/retry, restart, Replay/Compare/Rebuild;
  branch and instance isolation; rename retains ID.
- One actor/one current place and reconstructable half-open occupancy for
  start, segment, arrival, cancel, delay and reroute; no endpoint dual presence.
  True overlapping segment intervals yield encounter and lawful speech;
  non-overlap yields neither. RP5 works and later schedule do not teleport or
  duplicate arrival. Exact command retry and resumed clock advance do not
  duplicate Events or intervals.
- Closed door/wall, distance, channel and unfamiliar identity are independent
  negative tests; a known map can lag a committed closure/rename. No raw
  omniscient entity list leaks through HTTP/MCP/Play. Existing F1 and RP5
  regression remains green; schema031→032 upgrade and replay audit pass.

Implementation now includes migrations 032–035, stable lazy slots, derived
occupancy intervals, real timed segments, sourced roadworks delay at departure
and arrival, explicit cancellation, visual/audio zones, identity provenance and
dated map-memory Events. HTTP, Play and MCP have additive journey/map surfaces;
`docs/f2/protocol.md` describes the preview contract. Focused storage, HTTP,
browser and MCP tests have passed. Active-journey schedule deferral and explicit
cancel→return→alternate rerouting have focused recovery tests. Queue state and
appointment pointers are compared to Events and repaired where source data
suffices. Two independent Studio worlds and a foreign-branch denial are tested;
the product currently has no same-instance branch-fork operation, so a positive
fork fixture is unavailable. The full Go regression and final gate review are
still pending. No F2 checkpoint or stage-complete claim exists yet.
