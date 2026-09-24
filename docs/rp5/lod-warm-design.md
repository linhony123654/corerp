# RP-5 WARM execution boundary

Status: first WARM runtime slice implemented with opt-in policy and actual movement/recovery evidence. Remaining acceptance items below are not all verified; no completed four-tier LOD or RP-5 claim.

## Current implementation

Immutable opportunity policy `warm_enabled:true` enables private completed-Wait selection of at most4 off-scene named actors, using player-owned important relationships (trust/tension/obligation with source) or active appointments reaching the player's current scene within one hour. Event-derived actor/world six-hour cadence and oldest-first ordering are independent of sessions. Selection reasons stay private and do not become NPC knowledge.

RPService drains private `RPWarmDecision` commands before the pinned HOT roster. Stable session/actor/trigger request identity survives a changed optimistic head on retry; authorization and original roster are rechecked. Stale time/unrelated intervening actions reject execution. Own context supplies appointment, known law and legal routes. The existing path planner supplies sourced intermediate steps, so home→cafe→work preparation moves only to the first reachable step. The existing movement owner rechecks current physical route. The lawful-action guard is defensive: current enacted laws support only `speak`, not movement prohibitions, and WARM never speaks. No implemented or tested movement-law constraint is claimed. Quiet decisions consume cadence, and neither branch calls a model or grants early attendance/pay.

Actual tests cover imminent-appointment selection, multi-hop preparation, rollback, off-scene privacy, restart/rebuild/idempotent movement, no WARM provider calls, later normal scheduled work/HOT encounter and unchanged COLD work. Relationship/closure/cross-session and same-Wait HOT checks also pass. Additional real-world tests reject triggers after same-time unrelated movement or a clock advance, including after projection rebuild. Accepted quiet decisions suppress selection through 5:59:59; selection and a new accepted decision resume at exactly six hours. Full large-world/cohort verification and broader behavior remain open.

## Reuse and constraints

Actual over-capacity HOT acceptance (`TestRPHotActualOverCapacityFairnessAndRestart`) now uses15 additional conserved materializations/backgrounds:19 named people plus1 remaining aggregate person,18 NPCs at the player's cafe. Each of two Waits calls the provider exactly16 times; all18 NPCs are served across the two rounds, including a database reopen/rebuild between rounds. Exact current and old Wait retries retain the original roster and add no provider calls; current retry also leaves the branch head unchanged. Projections compare clean. This complements the existing pure1000-candidate selector test and actual four-tier week; it does not claim thousand-person world throughput. The original goal prohibits continual1000+ NPC LLM calls, not requires a thousand-person synthetic world. Terminal normal/race results are in progress.

Seven-day actual tier integration (`TestRPLODTiersAcrossWeekAndRestart`) now exercises the existing twenty-person economy: 16 aggregate people and four named people, 21 completed waits plus exact retries, seven real off-scene preparations, seven normal work arrivals and daily lunch encounters. Observed 28 HOT provider calls and seven WARM decisions; provider inputs require own life context and actual player co-location. Every wait enforces the 16/4 bounds; retries add neither Events nor provider calls. COLD Bo still reaches work; aggregate wage obligations and consumption advance daily without materializing extra people. Database reopen/rebuild on day four and daily projection comparisons pass. Normal run PASS2.733s. This is a deterministic local provider fixture, not live LLM or thousand-NPC performance evidence; larger-scale cap/fairness and full RP-5 long-run divergence acceptance remain open.

- HOT remains the current scene's bounded, fair, committed Wait roster and full own-context provider path.
- COLD remains existing named-character schedules/economics. Cohorts remain aggregate simulation; neither receives a model loop.
- `readRPOwnDecisionContext` can build an off-scene actor's own filtered context without a player speech/turn. Do not pass player position or use co-location claims as current global truth.
- `commitRPNPCMovement` already rechecks route/obstruction, writes existing movement/schedule/position facts and actual destination co-location knowledge. Generic replay reads those movement facts; do not create a second position owner.
- `executePrivateFactCommand` supports private immutable Event + domain rows atomically, but rejects pending waits. WARM effects must therefore drain after completed Wait, not inside an unfinished wait or via nested transactions.

## First decision rule

`RPWarmDecisionInput` includes only actor/time/current place/activity, own next sourced schedule and legally reachable places. `ProposeRPWarmDecision` may propose leaving for the actor's own upcoming work within one hour; otherwise wait. It never invents conversation, attendance, wages, a contract, or knowledge of the player's whereabouts. Already-working/at-workplace, due/past appointments and unavailable routes stay with existing owners. A delayed appointment must retain original time and delay source.

The initial rule supported only direct destinations; the runtime increment now also passes an immediately traversable sourced path from the existing planner and the core validates its endpoints/source count before selecting one next step. It does not teleport along the whole path. Scheduled work remains the only owner of starting the work activity.

## Runtime acceptance contract (retain for full audit)

1. During Wait completion, select at most4 off-scene named actors from sourced important relationships or accepted imminent appointments affecting the current scene. Use only player-owned relationship evidence/real schedule sources for simulation priority. Selection is not NPC knowledge.
2. Enforce actor/world six-hour cadence from accepted WARM Events, across sessions; retain COLD schedules even while actors are not selected. Persist candidate IDs and source reasons privately in the completed Wait. Never reveal off-scene positions/assessments through public Wait output.
3. Drain WARM decisions before HOT through a distinct private command. Authorize the requesting player's session and exact committed trigger roster, reject unrelated intervening world commands and stale clocks, and revalidate each actor's own schedule/routes. Never accept a caller-chosen arbitrary NPC or destination.
4. Bind retry identity to session/NPC/trigger; the domain request hash must remain stable after earlier actors commit. A freshly observed head can be passed as the transaction's optimistic binding separately from that stable request. Do not hash a fresh head into every retry.
5. Commit a private `RPWarmDecisionRecorded` Event and any existing movement facts atomically; retain reason/selection source/own schedule source and effective delay source. Waiting is an accepted cadence-consuming decision, not fabricated speech. No provider is called.
6. Extend HOT trigger validation only for permitted WARM effects from the **same trigger**, not arbitrary WARM commands. Newly arrived actors are not silently appended to an already committed HOT roster; subsequent observation/Wait sees them normally.
7. Recovery replays accepted effects without duplicate movement, knowledge or model work; superseded triggers must not execute into another world moment. Check rollback, cross-session cadence, foreign actor/source rejection, current route obstruction, multi-hop boundary, no premature work/pay, restart/rebuild and actual arrival/next-scene encounter.

Acceptance remains actual end-to-end off-scene decisions/effects plus explicit HOT/WARM/COLD/COHORT evidence, not merely this design or the pure tests.
