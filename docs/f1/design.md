# F1 design — one world, unified interaction and variable-length reading

Status: design contract for the current stage; implementation and verification
are in progress. Source baseline: [F0](../f0/phase-report.md), schema030.

## Play visual execution contract

Keep the existing ink-on-warm-paper life-journal identity. The scene title and
chronological reading column remain the visual protagonists; interaction
controls are quiet marginalia at the composer, not a dashboard or a grid of
cards. Reuse the current serif/body/mono roles, rust accent, timestamp and
seal motifs, and accessible focus treatment. Long passages retain a bounded
reading measure, generous line height and wrapping. The mode control states
whether text is only spoken or may be parsed as a scene action; density lives
with narrative preferences and never suggests extra world time. On mobile,
controls wrap into a compact line above the composer without covering the
story. Recovery, clarification and pause messages remain readable beside the
original input; restrained arrival motion respects reduced-motion settings.

The opt-in unified composer path is additive. The existing speech button and
typed map/wait actions keep their established protocol and recovery semantics.
An explicit scene/AUTO input uses the new persisted interaction endpoint,
surfaces its actual status and ordered outcomes, and offers exact retry or
authorized stop only where the server permits it. No UI text may imply that a
paused plan rolled back already committed effects.
The exact additive routes and bounded grammar are recorded in
[protocol.md](protocol.md).

## Observable acceptance

- One authenticated input can mean speech, a typed action, or an ordered
  move/wait-then-speech plan. `AUTO`, `DIALOGUE` and `SCENE` are explicit
  interaction choices; `MIXED` is an internal resolved plan kind.
- Explicit per-request mode wins over a session default; absent both uses
  `AUTO`. Ambiguous “继续” asks for clarification and causes no world effects.
- The accepted request and resolved plan bind to the original session/key.
  Lost responses, process reopen and exact retries neither reparse to another
  action nor duplicate any child Event. A changed body under the same key fails.
- Each child effect goes through existing authorized Speech/Turn, Move or Wait
  owner. Only committed Effects and lawful Observation enter narrative. A
  choice, stale state, limited budget or permission failure stops remaining
  steps visibly; it never guesses past the boundary.
- Narrative density `concise` / `standard` / `long` is presentation-only and
  independent of interaction mode or elapsed world time. A rich fixture can
  display 1000+ Chinese characters without fabricated facts or minimum-length
  padding. Streaming/regenerate/recovery stay scoped to original session,
  world and branch. If a live provider is configured, run one real long sample;
  if not, record `NOT VERIFIED` without claiming literary quality.

## Reuse and implementation boundary

`rp_turn_runs` migration025 requires a player speech for every settled row, so
an action-only interaction cannot be stored there. Add a small **application
orchestration** record in a forward migration, not a second world ledger or
another RP engine. Store immutable original input, request hash, resolved
plan, ordered step state and deterministic child keys. Persist each child
request (including its cursor) before calling its existing owner. If its
commit succeeds but recording progress fails, replay the exact child call to
retrieve the existing result. No provider or RNG runs on history replay.

The interpreter is bounded and deterministic for the first supported Chinese
syntax: bare dialogue; known adjacent destination move; explicit wait interval;
and clear action followed by quoted speech. It resolves named targets only
against the caller's own current Observation and rejects ambiguous or unsupported
actions instead of inventing world facts. This grammar is a supported-scope
contract, not a claim to understand arbitrary prose. No LLM is required to
decide world mutations. Additive endpoint/client wiring leaves the frozen
`corerp.client.v1` operations unchanged.

For each step, obtain a fresh authorized observation before creating the next
child request. A pending exact wait retains its original target/budget/key.
Concurrent world changes trigger revalidation/pause; no plan is silently
rewritten. Session defaults are application preferences, not Event authority.
Existing request retirement and recovery semantics must be reconciled before
exposing the endpoint as complete.

Add an optional density field to the existing `RPStyleProfile`/patch/pinned
turn style, preserving missing legacy fields and old saved narratives. Long
rendering remains a read over validated committed facts. Sparse facts can
yield a shorter result; do not manufacture events, unobserved thoughts, time
passage or unaccepted dialogue to hit a character target. Rich prose may be
supplied by an independent provider only through the same attributed input
and strict output checks; the current model style planner is not itself a
full-prose provider.

## Representative slice and verification order

1. Add a persisted, actual-world `move → speech` interaction on one session,
   using source-owned Move/Turn and showing both Events in order.
2. Inject a failure after movement, reopen the same SQLite world, retry the
   original key, prove no duplicate movement/speech and unchanged world
   projections after rebuild. Verify mismatched retry and foreign control deny.
3. Extend the same orchestration to speech-only, action-only, wait and explicit
   mode/default/AUTO cases; add pauses and ambiguity. Test short/long/mixed
   ordering and interruption at every step boundary.
4. Extend style/render/read/stream and Play's existing pending-request UX;
   preserve SillyTavern/MCP compatibility and expose additive interaction
   capability where appropriate. Run targeted, race, migration/reopen/replay,
   frontend/browser and full-stage regressions before F1 checkpoint.

No F2 timed travel, F3 controller leases or new social/health/household owner
is smuggled into F1. Movement stays immediate until F2.
