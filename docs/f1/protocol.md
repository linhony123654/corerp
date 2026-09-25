# F1 additive interaction protocol

The frozen `corerp.client.v1` speech, move, wait, observe, stream and request
retirement routes remain unchanged. These POST routes are additive under
`/api/v1/rp/`; all use the existing player bearer authentication and scoped
session control. Credentials and `principal_id` are never client payloads.

| Route | Input | Result / boundary |
| --- | --- | --- |
| `interactions/run` | `session_id`, `expected_cursor`, `idempotency_key`, `text`; optional `interaction_mode` (`AUTO`, `DIALOGUE`, `SCENE`) and `narrative_density` (`concise`, `standard`, `long`) | Persisted plan, status, ordered child outcomes; exact retry only. |
| `interactions/resume` | `session_id`, original `idempotency_key` | Reads the accepted original input/plan and continues exact child requests. |
| `interactions/stop` | `session_id`, original `idempotency_key` | Ends orchestration, never rolls back committed Events; refuses an accepted but unreconciled child. |
| `interactions/default/read` | `session_id` | Session interaction mode and immutable revision (initially `AUTO`, revision 0). |
| `interactions/default/set` | `session_id`, `interaction_mode`, `expected_revision`, `idempotency_key` | Compare-and-set immutable session preference. |
| `requests/retire` | `operation: "interaction"`, `session_id`, `idempotency_key` | Fences only an unaccepted key, or reports accepted-plan state. |

`DIALOGUE` always treats text as speech. `AUTO` and `SCENE` currently recognize
bare speech, `去<当前唯一可达地点>`, `前往<当前唯一可达地点>`, `等1/2/4小时`
(Chinese numerals also accepted), and either action followed by
`，随后说「<原话>」` (also `然后说` / `说`). This is a deliberately bounded grammar,
not arbitrary natural-language intent understanding. `继续` / `继续吧`, unknown
destinations and malformed action syntax return `clarification` with no Event.
An unsupported action should be rephrased as one of the typed actions; it must
not be assumed to have happened merely because it appears in speech text.

An interaction can be `open`, `budget_exhausted` (same pinned wait continues on
exact retry), `paused`, `clarification`, `stopped`, or `settled` in its response.
The first status is durable; `budget_exhausted` is a response projection of an
open plan with a pending typed wait. After uncertainty, retry the **same full
run request**, or call `resume` with the original key; never regenerate a new
text/key. A `paused` plan does not execute remaining steps; the client may call
`stop` and then freshly observe/re-plan. A paused plan retains the session's
single-active-interaction slot until stopped. Already committed move/wait/speech
Events remain. A speech outcome includes `turn_run_id`; the existing narrative
read/stream/regenerate endpoints own its presentation. Stream retry must not
rerun any world action. Ordered `outcomes` include Event IDs/sequences; the
application plan itself is not another world Event or Knowledge source.
Each outcome's `event_sequence` identifies its child Event, while
`settled_sequence` marks the last contiguous Event accepted as part of that
child (for wait, this can include wait-triggered warm/initiative Events). Any
later unrelated branch Event pauses the remaining plan.

Mode precedence is explicit request mode, then the latest session preference,
then `AUTO`; the resolved mode is pinned with the accepted plan. Density is
presentation-only, independent of mode and elapsed time. The deterministic
renderer can show a 1000+ character sourced fixture and leaves sparse scenes
short; this is not a promise of creative model prose or a minimum output length.
No live narrative-provider sample has been verified for F1.
