# Nonverbal action interrupted by controller assignment

Status: unresolved recovery risk; static audit only. The reproduction below has **not been executed**. No model calls, database mutations, or runtime code changes were made for this audit. Existing passing checks do not establish recovery for this interleaving.

The integration coordinator reported runtime freeze `85d43576`, schema `080`, final source `2070e375442`, and a real 22-case Golden run in progress when this document was requested. Those identifiers and the ongoing run are coordinator context, not independently verified audit results. This document does not change the frozen runtime or claim the next recovery slice is complete.

## Concrete static reproduction

1. Enroll an external controller for an NPC before starting the action; leave assignment inactive. The NPC must have no active session or interaction that would independently prohibit assignment.
2. Start a player-owned nonverbal turn whose frozen eligible witnesses include that NPC, with the NPC activated for an internal reaction. A directed action provides the smallest case.
3. While its decision provider is running, an authorized operator calls `AssignRPExternalControllerLocal` with the current expected branch head and expected controller generation. With exactly one pending committed turn and no pending wait, the listener exception permits assignment.
4. Assignment commits `RPExternalControllerAssigned`, advancing the branch head. Internal decision commit then rejects the changed NPC owner. `finishRPTurn` can detect the new owner, record an external-controller skip, and advance the NPC stage.
5. Before narrative construction, `validateRPActionTurnContinuation` rejects the assignment Event as unrelated to this action's authorized NPC decision batches. It remains in immutable history, so retry cannot remove the fence.

The expected consequence from this code path is a permanently pending turn. This is a static conclusion, not a captured runtime outcome. In a turn with multiple activated NPCs, an assignment can also fence reactions of other still-undecided NPCs before the NPC stage completes.

## Current invariants and conflicting guards

- [`AssignRPExternalControllerLocal`](../../../backend/internal/storage/rp_controller_authority.go) deliberately supports assignment during one committed heard turn. [`private_fact_command.go`](../../../backend/internal/storage/private_fact_command.go) checks that `player_event_id` exists and the assigned NPC belongs to `listener_ids_json`; this exception does not exclude a nonverbal trigger. Assignment still requires its normal operator, enrollment, generation, background-progression, session, interaction, shared-round, and expected-head checks.
- [`readRPActionTurnSource` and `validateRPActionTurnContinuation`](../../../backend/internal/storage/rp_decision_action_trigger.go) require the real player-owned single-Event action batch. Only complete, committed, same-parent, same-session NPC decision batches from activated listeners may follow it. The assignment Event is not such a batch. Frozen witnesses and target disclosure remain authoritative.
- [`finishRPTurn`](../../../backend/internal/storage/rp_turn.go) respects changed ownership through `skipControlledRPListener`, but applies the action continuation fence before narrative rendering. Narrative staging applies the fence again and requires the actual branch head to equal the artifact's frozen `SourceHead`.
- [`RunRPNonverbalTurn`](../../../backend/internal/storage/rp_nonverbal_turn.go) retries the original durable request. `markRPNonverbalTurnPlayerCommitted` checks continuation only while the run is `open`; later retries bypass that initial check but still encounter reaction or narrative continuation guards. Saved-winner recovery helps only when another caller has actually settled the same run.
- [`RetireRPRequest`](../../../backend/internal/storage/rp_request.go) returns `in_progress` for an existing unfinished nonverbal turn without altering accepted work. It does not settle this interruption.
- [`StopRPInteraction`](../../../backend/internal/storage/rp_interaction.go) rejects stopping a plan with an accepted child until that child is recovered. It preserves the committed player action, but cannot itself finish its fenced child.
- [`CloseRPSession`](../../../backend/internal/storage/rp_session.go) rejects a session with any unsettled turn. The pending turn also continues to block commands subject to the ordinary pending-turn guard.

These guards each preserve valid authority or immutable effects. Their composition lacks a terminal outcome for an action that legitimately loses its ability to continue.

## Proposed explicit partial historical settlement

Implement interruption as a separate recovery outcome, with durable evidence and an atomic terminal transition. Keep decision continuation strict.

1. Under a transaction, identify and pin the first unrelated Event and the historical boundary immediately before it. Verify the original action and every included complete NPC decision batch using the existing source, session, parent, activation, command, attempt, proposal-hash, scope, index, and batch-range checks. Reject projection corruption rather than treating it as ordinary interruption.
2. Preserve all committed player and NPC observables. Build the finite narrative from the action and already committed authorized effects at that boundary, using historical public identities, packages, perception evidence, actual words, recorded delivery, and frozen witness disclosure. Read no private proposal or emotion into the artifact. Do not create new world Events, speech, time advancement, consent, or decisions to complete the turn.
3. Record explicit interrupted accounting for selected listeners with no committed decision. Keep existing external-controller skips only where a genuine owner change authorizes them. Do not relabel a selected listener as historically `not_activated`, or invent a silence/wait decision to satisfy counts. Preserve previously committed decisions and existing activation provenance.
4. Persist an interruption outcome and fence Event identifier alongside the exact finite artifact. Stage and settle the run/session atomically with current ownership authorization, a compare-and-swap against the observed current head, and validation of the pinned historical boundary. This requires a narrow historical-settlement path: normal staging's `actual head == artifact.SourceHead` condition cannot be reused unchanged.
5. Return that saved terminal outcome on the original request key without another provider call. An interaction may then preserve the accepted child outcome and stop or pause its remaining steps according to its existing plan semantics. Do not silently continue additional planned actions because partial settlement succeeded.

`readRPNarrativeInputAtHead` currently reads same-parent NPC decision and expression rows before assigning its requested `SourceHead`. Merely lowering `SourceHead` is insufficient: the recovery builder must explicitly filter or reject batches after the pinned boundary. Legitimate action decision commits are already fenced after an unrelated Event, but recovery must also verify this rather than assume it. If an already committed authorized observable cannot be represented safely at the chosen boundary, retain it and report the inconsistency instead of dropping it.

## Rejected shortcuts and next validation

Do not deny the otherwise authorized controller handoff just to avoid this state; that would remove the owner's supported behavior and would not repair an already interrupted run. Do not allow unrelated Events through decision continuation, retry a stale proposal under the new owner, fabricate silence, erase the accepted action, or mark an incomplete run settled without its public artifact and explicit interruption outcome.

The next implementation slice should execute the interleaving with a locally controlled provider barrier, then verify directed and bounded multi-listener cases, effects committed before interruption, restart and original-key replay, interaction stop, request retirement, and session closure. Assert no post-fence internal decision, duplicate player action, lost committed effect, invented listener outcome, private disclosure, or extra model call during recovery. The runtime result, test evidence, and any remaining limitations must be recorded separately; this audit is not implementation or release evidence.
