# Commit, recovery, and private actor ownership audit

Date: 2026-10-01. Base inspected: `739a3ad94f34dd1d7a0a4cf1ec450ea02514415e`.

## Conclusion

No ordinary commit/restart/rebuild failure is demonstrated by this source-only audit. The existing applied-decision boundary supports bounded private recollection; it does not guarantee semantic accuracy or persistent goals. The sparse lifecycle design below is a conditional option, not a recommendation to add architecture before reproducing the user's inaccurate behavior. If persistent goals are actually an acceptance requirement, storage must own their restricted typed transitions; provider receipts and prose cannot supply authority.

This is a source audit, not a test execution report. No suites, installs, deployments, commits, or intentional provider requests were performed. Existing tests were read as evidence of intended coverage, not reported as passing. Root/project AGENTS.md were read. Vexor filename discovery stalled and was terminated; targeted local source searches supplied the findings. Existing planning/screenshot changes were preserved. Only this report is an authorized deliverable.

## Top three source-backed accuracy limitations and minimal repairs

1. **Grounding validation checks accessible reference IDs, not semantic truth.** `backend/internal/core/rp_proposal.go:19–31` permits empty private basis arrays, bounds prose, and checks any supplied ID against accessible evidence. Speech checks (`65–69`) constrain shape/size and action fields; they do not prove factual consistency with persona, relationships or prior events. Definition → builder (`rp_decision.go:98–135`) supplies authored facts/readiness; actual provider view (`rp_decision_provider_view.go:16–91`) masks identities and reselects context; validation runs on both views (`rp_decision.go:510–513`); commit repeats structural/evidence validation (`rp_decision_commit.go:62`); accepted speech becomes a witnessed/public fact of *what was said*. Thus “approved” does not mean “canonically accurate.” This is a verified partial semantic contract, not a demonstrated bad response. **Minimal repair:** reproduce one incorrect claim with its actual provider packet and enforce only the specific typed constraint violated (e.g. authored identity/address or outcome authority); otherwise improve the packet/prompt or record claim uncertainty. Do not claim an accessible citation proves entailment or impose broad truth classification on all dialogue.
2. **Private continuity is deliberately bounded and can shrink again at provider selection.** The reader selects three sketches (`rp_private_decision.go:29`); `backend/internal/core/rp_context_selection.go:45,78,141–143,179` selects within a byte budget and reports omissions. `rp_decision_provider_view.go:60,91` re-applies selection after identity masking. A builder containing a sketch does not prove the actual provider saw it. This is a verified limited recall contract, not a broken persistent-goal implementation: no persistent-goal lifecycle is currently defined. **Minimal repair:** inspect the exact provider view and omission counts for the failing example; prioritize the needed sourced context within the existing selector if that is the demonstrated cause. Only introduce explicit active-intent state if long-lived intent is an agreed requirement. No automatic migration from prose is warranted.
3. **Provider success precedes world application.** See F3: a successful receipt is not an effect receipt. If observed inaccuracies are descriptions of an action that never happened, check committed decision/Event/batch and rendering sources rather than assuming provider success means action completion. **Minimal repair:** bind any user-visible “done” statement to committed effect/outcome evidence. In this audit, current commit paths already retain the necessary atomic source records; no public mislabeling was reproduced. Crash retries may legitimately call the provider again before one final applied effect.

**Classification:** missing authored persona/address/relationship data must be distinguished from dropped provider context and semantic hallucination (`rp_decision.go:121–135` records readiness). This audit did not inspect a deployed runtime, its database or a failing live packet, and establishes no deployment mismatch. Event-only restoration limitations (F2), future generation/version fencing and a missing persistent-goal lifecycle are separate design concerns, not explanations proved for the reported two-day accuracy regression. No top-three demonstrated runtime bugs are asserted.

## Verified current boundary

- `backend/internal/storage/rp_decision_commit.go:37–119`: validate request/result binding, compare fresh full input hash/head, authorize player control, recheck internal NPC ownership inside the immediate transaction, require active turn/stage, reject pending waits and moved actors.
- `rp_decision_commit.go:219–319`: command, attempt, batch, Event, immutable decision, hearing/activity/movement/own-action projections, expression, branch/clock/turn state, audit, outbox and committed statuses are one transaction. The rollback hook is before commit. The ready attempt/30-second lease is created and committed within this same transaction; it is not a durable pre-provider work claim.
- `backend/internal/storage/rp_initiative_commit.go:215–249,347–458`: initiative rechecks context hash, owner, legal proposal, cadence and chronology under the final transaction. Event count/last sequence and epoch validation include the optional expression (`301–315`). Cadence is actor/world scoped (`88–116`), so a new session does not reset it.
- `backend/internal/storage/rp_private_decision.go:14–58`: private history requires the same instance/branch/NPC, committed command and attempt, matching attempt/proposal hash, source actor, and the full batch below the input head. It excludes the current parent turn, verifies canonical proposal hash, then returns chronological history. Audit-only and rolled-back candidates cannot enter through this reader.
- `rp_initiative_commit.go:265–269,422–435` removes `Private` from observable Event proposals and publishes only visible effects. Spoken decisions use effect payloads rather than full proposal JSON (`rp_decision_commit.go:137–166,227–228,302`).
- `backend/internal/core/rp_decision.go:90–99,188–211` explicitly treats the source Event as proof of application, not proof that a private thought is true or a goal is complete. Private content is model metadata. `backend/internal/core/rp_proposal.go:21–26` bounds private text and basis references. Sourced evidence does not convert speculation into canon.

## Findings

### F1 — History truncation cannot implement persistent commitments (high for the proposed extension)

**Evidence:** `rp_private_decision.go:29` selects only the latest three applied private proposals. `core/rp_decision.go:207–211` has intent/emotion/relationship prose and basis IDs, with no stable item ID, state, predecessor version, due time, supersession or outcome. `backend/internal/storage/rp_private_decision_test.go:207–234` explicitly expects a sliding three-item history.

**Trigger/impact:** An actor adopts a long-lived intent, then produces three newer private sketches. The earlier intent drops from context even if unresolved. Conversely, an obsolete intent among the latest three still appears without a terminal transition. Treating this packet as active state creates accidental forgetting or repeated action. This behavior follows directly from the query; resulting model behavior is inference, not a demonstrated runtime failure.

**Smallest design:** Retain the existing bounded sketches for recollection. Add typed private items and append-only transitions with stable actor/item IDs, version, kind, status, source decision, branch sequence, basis, optional target/due world time and terminal reason. Build a separate bounded *active-state* packet; selection must not terminate omitted items. Completion needs owner-validated evidence, not speech or model self-report. Emotion and relationship stance remain subjective actor assessments and cannot overwrite authored relationships/persona.

**Compatibility/privacy/recovery:** Do not reinterpret old prose as live commitments or backfill completion by inference. Legacy rows remain sketches. Active/terminal transitions require a private recovery source; their text and targets must not enter narrator, observatory, public Event, outbox or other actors' packets.

### F2 — Rebuild preserves private authority but cannot recreate it from public replay (high for recovery design)

**Evidence:** Private input reads `rp_npc_decisions.proposal_json` (`rp_private_decision.go:18,28,41–49`). Public initiative Events explicitly strip private data (`rp_initiative_commit.go:265–269`); spoken Event insertion and decision insertion are distinct (`rp_decision_commit.go:227–228`). `backend/internal/storage/replay.go:54–63` ReplayState contains balances, positions and knowledge, not private decision authority. `RebuildProjections` (`341` onward) repairs projections without recreating private proposal history. Immutable decision triggers are defined at `backend/internal/storage/migrations/024_rp_npc_decisions.sql:20–24`.

**Trigger/impact:** Rebuilding projections in the intact database preserves history. Restoring only world Events/public snapshots or losing the private authority rows cannot recover the private sketches, and therefore cannot recover a future lifecycle based on them. The first is supported by source/test intent; the latter is a recovery inference from deliberately missing information, not a claim that ordinary rebuild currently deletes rows.

**Smallest design:** Name the private ledger as retained authority included in backups/export policy, with its own schema/hash version and source links to committed decisions/batches. Rebuild only a disposable current-state projection from that ledger. Fail closed on missing/corrupt authority; never repopulate from audits or a new model call. Snapshot scope must explicitly include a restricted private checkpoint or declare it public-only.

**Compatibility/privacy/recovery:** Preserve public replay hashes and historical Event formats. Do not embed private state in public ReplayState merely for convenience. Rebuild/restart tests must cover private state equality, projection corruption repair, missing-ledger refusal and public leakage separately.

### F3 — Provider success is not approval or application (medium now; high if used as a lifecycle source)

**Evidence:** `rp_decision.go:481–486,526–534` starts/finishes provider receipts and audits validated output before `CommitRPDecision` is called. `rp_turn.go:162–180` makes decision and effect commit distinct steps. `rp_provider_calls.go:67–69` explicitly leaves an interrupted call's actual HTTP attempt uncertain. `rp_initiative_commit.go:205–212` likewise records provider success before final commit. The final authority is a fresh transactional revalidation, not a separate human approval token.

**Trigger/impact:** Crash after provider success/audit but before effect commit, cancellation, stale input or ownership change. Retry can call the provider again and obtain a different proposal. The success receipt does not prove that either candidate was applied. This sequencing is verified; repeated cost or different output is an inferred possibility. No claim of an HTTP authentication bypass is made: exported storage result/status values are a trusted in-process boundary, not independently authenticated approval credentials.

**Smallest design:** Private-state mutation must happen exclusively in the final commit transaction. If durable candidate reuse is needed, persist a separate candidate with typed `proposed/validated/applied/rejected/abandoned` status, input hash, owner generation and proposal hash; revalidate before application. Never let `result='success'`, `Status='validated'`, or proposal audits seed active state. Mark abandoned receipts diagnostically without inventing an attempted count.

**Compatibility/privacy/recovery:** Keep provider diagnostic history distinct from actor knowledge. This system can guarantee one applied effect, not exactly one external provider execution across crashes. Retrying before commit must not double-apply private transitions.

### F4 — Ownership is actor-scoped today, but persistent state needs generation policy (medium design risk)

**Evidence:** `rp_controller_authority.go:111–138` fences currently external/Human-controlled actors. Authority assignment has a generation (`93–103`). Private history keys on NPC/world/branch and joins its original session only for interlocutor attribution (`rp_private_decision.go:18–29`); it does not scope memory to controller generation. Both final commit paths recheck live internal ownership.

**Trigger/impact:** Internal actor becomes externally/Human controlled, then returns to internal control. Existing private history remains actor memory across that handoff. This is verified query behavior; whether it is undesirable depends on product policy. A future pending private transition authored before handoff would need a stale-generation fence even when the actor is internal again. Current head/hash validation mitigates intervening event-backed authority changes, so this audit does not assert a proven stale-proposal exploit.

**Smallest design:** Separate state *owner* `(instance, branch, actor)` from current decision *executor* and `controller_generation`. Preserve historical memory as actor continuity by explicit policy; suspend autonomous live intentions while noninternal control is active. Require generation plus expected private-state version at commit, and explicit resume/rebase/cancel on return. External controller/human access to private history must be a distinct scoped permission, not implied by controlling movement/speech.

**Compatibility/privacy/recovery:** A session is an interaction container, not the private-state owner. Do not erase history on session closure or reset it when opening another session. Replay authority handoffs before resuming private transitions; generation changes must not silently transfer restricted provider access.

### F5 — Retried receipts and future private transitions need consistent integrity checks (medium hardening)

**Evidence:** `rp_decision_commit.go:338–358` retry lookup matches stored input/proposal hashes but only joins the decision and Event. Initiative retry (`rp_initiative_commit.go:59–85`) requires a committed command and checks event request identity; it does not join the committed attempt or verify the retained private proposal hash. The private-memory reader has stronger checks (`rp_private_decision.go:20–28,45–47`). Normal transaction atomicity and immutable rows protect ordinary operation.

**Trigger/impact:** Restore/import inconsistency or manual corruption affects command/attempt status, batch scope, or decision linkage. A receipt can report replayed while a future private-state reader refuses or omits corresponding state. This is an inferred corruption/recovery hazard, not a demonstrated ordinary retry defect.

**Smallest design:** Share an applied-decision integrity check for lifecycle application/recovery: command/attempt committed, full batch within head, matching actor/scope/source and canonical proposal hash. Verify transition uniqueness by `(decision_id, transition_index)` and private item version. Return divergence for incomplete authority rather than treating it as a fresh candidate.

**Compatibility/privacy/recovery:** Keep existing request keys, decision/event IDs and proposal hashes stable. Exact spoken retry is intentionally hash-sensitive (`352–353`); initiation replay intentionally accepts the same request without a provider. Changing those contracts would break recovery. Do not hash a redacted public proposal as if it were the private approved proposal.

### F6 — Private/observable effect decisions share a batch, but private-only evolution needs an explicit contract (medium design risk)

**Evidence:** Both commit paths always create a base Event, including `RPNPCDecisionRecorded` for silence/wait (`rp_decision_commit.go:136–138`; `rp_initiative_commit.go:264–269`). Silence is therefore an applied decision marker rather than proof of a public action. Initiative cadence counts base Events including quiet decisions (`rp_initiative_commit.go:102`); optional expressions share the same batch and terminal sequence.

**Trigger/impact:** An extension updates only private intent/emotion or schedules a private lifecycle tick. Reusing ordinary initiative commit blindly can consume actor action budget and branch sequence, emit inappropriate public receipts, or count the wrong completion sequence when an expression exists. Current cadence behavior is verified; extension consequences are inference.

**Smallest design:** Define whether private-only transitions require a world decision marker or a private ledger sequence. If attached to an existing approved decision, use the same atomic commit and full batch boundary; do not manufacture visible action/narration. If independent, authorize a typed private-state command with actor ownership, world-time/source basis, its own idempotency key and rate policy. A private deadline is not a scheduler/world action until the actual action owner approves it.

**Compatibility/privacy/recovery:** Preserve existing silence cadence and expression batch semantics unless explicitly changing product behavior. Public consumers may know request/effect status without seeing private content or implying that a hidden plan was fulfilled.

## Minimal ownership and transition contract

1. Canonical world/authored character data outranks model prior. Restricted actor assessments are separately labeled subjective; sourced references prove accessibility/application, not factual truth.
2. Storage owns authorization, transition legality, sequence/version fencing and effect commit. The provider proposes; the narrator only renders observed effects. Sessions, receipts and audit prose own no actor lifecycle.
3. Store immutable restricted transitions and a rebuildable private current-state projection. Identity is actor/world/branch, with executor generation recorded separately. Stable typed item kinds/statuses prevent prose from becoming executable authority.
4. Candidate transitions may become applied only with fresh owner/input/private-version checks. Apply effect batch, private transitions/projection and statuses atomically. Terminal success needs an owner-validated world effect or a typed private-only outcome; speech about success is insufficient.
5. Rollback leaves neither effect nor private mutation. Retry returns the previously applied transition set. Restart resumes only durable nonterminal work after ownership revalidation. Rebuild consumes retained private authority deterministically and never invokes a provider.
6. Preserve legacy sketch read behavior; introduce lifecycle state additively and version its packet. Never infer legacy commitments or transfer hidden state into public replay/observations. Keep authored relationship claims distinct from private stance.

## Concrete critique of sparse transitions in existing decision rows

The smallest implementation can use `rp_npc_decisions` itself as the restricted ledger rather than introducing another authoritative log. Add an optional versioned typed intent transition field to the retained proposal, leaving the existing private sketch fields unchanged. **Do not put this field into observable Events:** initiative currently copies the proposal and clears only `Private` (`rp_initiative_commit.go:267–269`), so a new sibling field would leak unless public payload construction is changed to an explicit allowlist or the transition is nested inside the stripped private envelope. The nested private envelope is the smaller compatible choice, provided its validation/wire contract is versioned.

For a minimal single-active-intent contract, use `keep`, `start`, `resolve`, `abandon`. `keep` is an explicit no-op, not an implicit rewrite from new prose. `start` requires no active intent and lets the owner assign a stable ID from the committing decision ID plus a transition slot; the model cannot invent the ID of a historical item. `resolve`/`abandon` must name the current intent ID and expected state version. Starting another intent requires explicit abandonment first, or a bounded ordered pair of transitions in one decision if replacement is essential. Do not silently supersede. Keep terminal reasons typed and optional prose restricted. For goal fulfillment, resolve must cite actual authorized outcome evidence checked by the owner; otherwise label resolution as an actor's subjective closure, never world success.

Reduce all eligible transition-bearing rows in ascending Event sequence, with the private reader's scope/actor/committed attempt/proposal hash checks and `batch.last_sequence <= head`. Do not stream only the last three sketches or last N transitions: an old start can remain live across arbitrarily many keeps and unrelated decisions. Rows without the new contract, including historical legacy rows, are **no update**, never inferred start/resolve. An absent transition in new-format output must have an explicit compatibility policy; treating it as no update is safer than clearing active state. Rejected/audited/uncommitted candidates have no reducer effect.

State ownership remains `(instance, branch, actor)`, even when interaction partners change. Record the interlocutor/target on start from the validated source decision, not the current session during reduction. A stance or commitment to A does not automatically transfer to B. The input packet can expose the actor's own active intent and its original target while requiring separately authorized evidence before acting on it; an absent target is not an abandonment condition. No cross-actor read is introduced.

For a small first version, an indexed full sparse-history scan on input build is simpler than a checkpoint and avoids a second truth source. Measure its cost before introducing optimization. A cached private projection may store active item, reducer version, latest processed *complete batch* boundary and state hash; validate it against retained source rows/head and invalidate/rebuild on mismatch. A cache alone cannot recover missing authority. Any checkpoint must be restricted, bind reducer/schema version and source prefix/high-water mark, preserve terminal/identity constraints needed for later transitions, and fall back to a deterministic full reduction. A public replay snapshot is unsuitable.

Fresh decision input must include the reduced intent ID/state version so the existing full input hash fences changes; the final owner should additionally re-reduce/check the expected private version within the commit transaction before applying a transition. Whole-batch boundaries matter when an expression is the second Event: do not make private state visible at the base Event sequence while the batch is incomplete. A receipt may continue to return the existing base effect sequence; the private cache cursor is the batch terminal sequence. Normal restart simply rebuilds or verifies this state from committed rows with no provider call. Controller generation fencing/suspension remains as described in F4.

This approach is preferable to a new general agent framework and, initially, to a separate ledger table. Its costs are coupling the private wire/schema version to proposal hashes and scanning cumulative sparse history. Existing hashes must remain calculated over historical shapes; adding default fields during decoding/hash verification can invalidate legacy hashes, so preserve omitted fields and test old rows. If multiple independent items, non-decision private transitions, or long-term archival become required, move to the dedicated typed ledger described above; those are scope upgrades, not prerequisites for one active intent.

## Finite verification follow-up (not executed)

Existing relevant coverage was inspected: `rp_private_decision_test.go:14–78` checks private initiative stripping, rebuild/reopen/retry and later actor recall; `80–205` covers audit-only/rollback exclusion, scope, public consumers and context-hash stability; `207–234` covers bounded chronological memory. `rp_turn_test.go:181` starts durable-stage recovery coverage. These are source evidence only.

Before shipping a lifecycle extension, add narrow tests for atomic rollback with optional expression, concurrent expected-private-version conflict, duplicate retry, crash between provider receipt and commit, private projection repair, missing-authority refusal, three-sketch eviction with an unresolved typed item retained, controller handoff/resume, legacy no-state compatibility, and private-state exclusion from narrator/observatory/outbox/public replay. No broad suite rerun is recommended merely for this audit report.

## Authorized composition receipt implementation — 2026-10-01

Following the audit, the parent explicitly assigned narrative composition persistence/reload, then expanded it to canonical receipts and sanitized composition-failure classification. This is independent of the conditional intent design above; no goal/private ledger implementation was added.

**Implemented:** `backend/internal/storage/migrations/077_rp_narrative_composition.sql` adds default-empty version/default-`[]` group metadata to immutable variant rows and canonical turn receipts. `store.go` registers 077 as latest schema. `rp_narrative_render.go` validates that versioned groups match lines and flatten to the exact ordered committed fact IDs before save; presentation/style provenance stays in the separate source list. It imports the narrator's actual exported version, currently `corerp.fact-composition.v1`, and preserves provider kind `full_prose`. Legacy unversioned prose remains readable without inferring/backfilling a composition guarantee.

**Reload:** `rp_style.go` saves/reloads canonical metadata with prose and preserves actual per-group EventIDs on saved streams. `rp_session.go` exposes selected/canonical metadata on history; `rp_narrative_render.go` exposes it on selection/canonical restore; `rp_turn.go` returns it on settled replay and gathers prose/metadata in the same row snapshot. These are additive optional storage response fields; no core, narrative provider, context, dialogue, goal, ledger or planning files were edited by this subtask. Canonical prose/effects stay independent of variant selection; world Events and branch head are unchanged.

**Failures:** invalid composition plan, fact coverage mismatch and invalid template reasons map to `prose_validation_failure` in the existing sanitizer; response text/private IDs remain excluded. Invalid saved groups fail before insert/selection. Existing fallback receipt handling is preserved.

**Focused evidence:** the initial check could not find Go on PATH; installed `/usr/local/go/bin/go` and `gofmt` were then used. The initial new fixture failed because a raw fact builder has no style; the fixture was corrected to load the actual pinned turn style, without weakening production validation. Subsequent checks used `TMPDIR=/dev/shm GOTMPDIR=/dev/shm GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930` from `backend`:

- `/usr/local/go/bin/go test ./internal/storage -run 'TestRPNarrative(Composition|Render|UnsafeDraft)' -count=1` — PASS, 2.704s.
- `/usr/local/go/bin/go test ./internal/storage -run 'TestRPNarrative(Composition|Render|UnsafeDraft|Provider|Unsafe|Saved|Official)|TestRPProviderReceiptRejectsUntrustedFallback' -count=1` — PASS, 3.353s after making turn replay prose/metadata a single row snapshot.
- `/usr/local/go/bin/go test ./internal/storage -run 'TestRPOfficialNarrativePersistsAcrossSelectedVariantAndRestart|TestRPNarrativeBudgetCannotBlockSettlementOrDiscardFacts|TestRPTurnRecoversEveryDurableStageWithoutDuplicatingEffects' -count=1` — PASS, 5.051s.
- `git diff --check` on changed tracked storage files — PASS.

The new `rp_narrative_composition_test.go` covers composition rows, exact group/provenance separation, multiple atoms per paragraph, restart/selection/history/canonical read and grouped streaming, canonical restore, turn replay, unchanged canonical content/head under variant selection, invalid/missing/reordered/duplicated/empty/unknown-version groups, selection preservation on rejection, and populated pre-077 legacy upgrade. The existing render migration fixture now removes the new canonical columns/version before its earlier-schema simulation. No live providers, full suites, installs, commits or deployment were used. This evidence verifies storage receipt semantics, not live provider accuracy or UI interpretation of the optional metadata.

### P2 independent-source review repair

Independent review found a real reload defect: the initial group decoder flattened groups and validated that array against itself. Save validation was authoritative, but out-of-band corrupted groups could pass Observe/Select/turn replay. The fix adds separate fact-only `fact_event_ids_json` and `narrative_fact_event_ids_json` arrays to the same draft migration 077, persisted independently with variant/canonical receipts. Every reload now compares exact ordered, unique group coverage against that independent array. Canonical saved reads additionally compare with the already reconstructed authorized input. Authored presentation/style sources are excluded from these fact-only arrays. Legacy defaults remain `[]`, with empty version and no backfill/stronger inferred guarantee. Prose and receipt metadata are still loaded in a single row snapshot.

`TestRPNarrativeCompositionCorruptionRejectedAfterRestart` exercises unknown/duplicate/reordered group IDs for both variant and canonical receipts, changes only groups, retains the independent source sequence, reopens the database, and requires `ProjectionDiverged` on Observe/Select plus canonical read/turn replay. All six cases and the surrounding composition/render/legacy/canonical-restart checks passed using the bounded test binary command below.

Resource evidence: normal compilation in `/dev/shm` failed writing a package archive (`no space left on device`); a serialized build without debug information then failed mapping the linker output for the same resource reason. No assertions had executed in those attempts. Compilation succeeded with the prescribed shared cache and temporary paths when only the stripped test binary output was placed in `/tmp`:

`TMPDIR=/dev/shm GOTMPDIR=/dev/shm GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930 /usr/local/go/bin/go test -c -p=1 -gcflags='corerp.local/backend/internal/storage=-dwarf=false' -ldflags='-s -w' -o /tmp/corerp-r1-composition-p2-storage.test ./internal/storage`

The parent authorized `/tmp` test temporary paths; the unchanged assertions passed with:

`TMPDIR=/tmp GOTMPDIR=/tmp GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930 /tmp/corerp-r1-composition-p2-storage.test -test.run='TestRPNarrative(Composition|Render|UnsafeDraft)|TestRPOfficialNarrativePersistsAcrossSelectedVariantAndRestart' -test.count=1`

Draft compatibility: a local database that already recorded the earlier unpublished 077 cannot rerun the revised SQL automatically; the parent was told to recreate/migrate that draft fixture before integration. Populated pre-077 upgrade and legacy receipt readability are covered. No shared artifacts or caches were deleted.

Saved capability disclosure was restored after the narrator owner exported `narrative.CompositionCapabilityWarning`. Versioned saved `RPNarrativeView` reads now return the same supported/unsupported presentation warning as fresh composition. The reload test checks its exact presence; the legacy test checks that unversioned saved prose receives neither version nor capability guarantee. No narrator-owned source was edited by this subtask.

Final bounded regression — PASS, 15.763s:

`TMPDIR=/tmp GOTMPDIR=/tmp GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930 /usr/local/go/bin/go test -p=1 -gcflags='corerp.local/backend/internal/storage=-dwarf=false' -ldflags='-s -w' ./internal/storage -run 'TestRPNarrative(Composition|Render|UnsafeDraft)|TestRPOfficialNarrativePersistsAcrossSelectedVariantAndRestart|TestRPProviderReceiptRejectsUntrustedFallback|TestRPTurnRecoversEveryDurableStageWithoutDuplicatingEffects' -count=1`

The flags reduce debug/build artifacts and serialize compilation; they do not remove tests or assertions. Final `git diff --check` and `gofmt -l` on changed receipt/test paths were clean. The P2 repair and capability disclosure are complete and frozen for parent review. Earlier resource failures remain recorded above; no full suite or live provider was used.

### P3 replay warning and saved chunk alignment

Final review requested two bounded receipt corrections. `loadRPTurnResult` now appends the exported composition capability warning **after** any deterministic warning reconstruction, preserving it on both settled reads and replay. The composition receipt test pins a prose instruction so the deterministic warning branch actually runs, and asserts that both warnings remain in order. Legacy replay explicitly asserts no composition capability guarantee.

Saved versioned stream chunks now match fresh composition: the singular `EventID` is empty and actual per-line `EventIDs` remain populated. Legacy singular attribution is unchanged. The saved-stream assertion now checks this exact distinction.

Final affected checks — PASS, 12.970s:

`TMPDIR=/tmp GOTMPDIR=/tmp GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930 /usr/local/go/bin/go test -p=1 -gcflags='corerp.local/backend/internal/storage=-dwarf=false' -ldflags='-s -w' ./internal/storage -run 'TestRPNarrativeComposition|TestRPTurnRecoversEveryDurableStageWithoutDuplicatingEffects' -count=1`

`git diff --check` and `gofmt -l` on the three affected paths were clean. No other behavior/schema/provider scope changed. Storage receipt work is frozen again for parent integration.


## Authorized v2 public artifact implementation — 2026-10-01

The product integration delegation added 078 canonical/variant artifact JSON columns and switched primary settlement to the shared natural v2 renderer. The complete receipt and snapshot boundary are saved atomically with the session observation cursor. All saved reload paths—ordinary read/stream, turn replay, canonical/variant selection and history—validate authority and exact finite re-expansion. Versioned history now carries the independent fact-only `event_ids`. v1 and legacy prose retain their original version/attribution and warning behavior.

The validator binds the artifact to its owning turn/player source, verifies the complete required source projection and chronological coverage, and checks frozen witness/hearing evidence and exact companion causation. Typed place/activity/object/target/action fields, historical package labels and public cue provenance cannot be authorized merely by recomputing a hash and prose. Historical familiarity and place rename resolution preserve old output. Names currently originate in committed materialization; no unsupported entity-name update is claimed. Unknown artifact fields and malformed/duplicate/unknown finite-plan fields are rejected.

Default v2 reads cache the canonical receipt. Explicit `FullProse=true` permits one configured full-prose refinement over the validated frozen input, preserving its input hash and source head. No capable provider returns a recorded truthful fallback. Concurrent first refinements can produce duplicate provider attempts, but only the committed winner is emitted/returned; the losing plan creates no selected variant. This is presentation concurrency, not duplicated world effects.

Meaningful new cases cover fresh natural primary speech/expression grouping, private-input exclusion, cache/restart/replay/chunk IDs, full-prose one-time refinement after a later head and restart, before-commit rollback, populated-077-to-078 v1 migration, historical introduction and normal spatial rename, and concurrent two-plan winner behavior. Twelve coherent canonical forgeries recompute the input hash, finite plan, prose/groups and fact IDs while leaving world evidence intact: place, activity extras, label map, object extras, target extras, cue, accepted words, required-source omission, expression code, expression target, removed companion, and movement direction. Read/history/select/replay all reject them. A coherent damaged v2 variant is also rejected without altering canonical reads.

A 46.125s focused regression found only an outdated populated-052 fixture that retained the 078 marker after dropping render tables. The fixture now removes the corresponding canonical artifact column and marker so additive migrations can recreate both artifact columns. The subsequent artifact + populated-052 migration checks passed in 18.922s; after complete-batch checks and the normal spatial rename case, the same affected checks passed in 20.035s. Earlier targeted runs caught and corrected a guessed identity scope column, a settled-column typo, an extra-target normalization gap, a stale session cursor, and current-familiarity reconstruction. These failed checkpoints are not counted as passes.

Final frozen-source focused regression — **PASS, 47.347s**:

`TMPDIR=/tmp GOTMPDIR=/tmp GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930 /usr/local/go/bin/go test -p=1 -gcflags='corerp.local/backend/internal/storage=-dwarf=false' -ldflags='-s -w' ./internal/storage -run '^TestRPNarrative|^TestRPOfficialNarrative|^TestRPTurnRecoversEveryDurableStage|^TestRPTurnRunsTwenty' -count=1`

This includes the existing narrative source/perception/window/budget/provider/render checks, six durable recovery stages and twenty deterministic turns with time, movement, restart and replay, alongside all new artifact checks. `gofmt -l` returned no affected files; `git diff --check` was clean. No full suite, live provider, commit, push, deployment or production data was used by this storage worker. Source and tests are frozen for parent integration. Source coverage remains the existing bounded turn/window policy, not a claim of comprehensive world history or semantic entertainment quality.


### Parent full-suite follow-up: long interaction fixture

The parent's separately running full backend suite reported `TestRPInteractionLongNarrativeFixtureStreamRecoveryAndWorldInvariance` at its obsolete singular stream-`EventID` assertion. Read-only diagnosis established that ordinary reopened reads still returned the long canonical lines/IDs with empty RenderID; an explicit concise override persisted as the selected history variant. There was no demonstrated production persistence defect.

Only this test was updated. It now checks every v2 chunk's exact `EventIDs` against its FactGroup, empty singular EventID, index and line; unchanged canonical version/groups/ordered IDs and long prose after restart; persisted selected concise history metadata; byte-equivalent canonical DB prose/version/groups/fact IDs/artifact/head before and after the variant/restart; and its existing money/clock/head/knowledge/replay invariants. No production source changed.

Focused corrected case passed in **1.149s** using the same local Go flags/cache with `-run '^TestRPInteractionLongNarrativeFixtureStreamRecoveryAndWorldInvariance$' -count=1`. `gofmt` and scoped `git diff --check` were clean. This focused correction does not retroactively label the parent's earlier compiled full-suite run as passed.


### Parent full-suite follow-up: configured decision provider fixtures

The parent's earlier compiled full suite stopped two configured-provider fixtures on obsolete presentation assertions: a reply plus proven beckon now occupies two v2 paragraphs with three ordered source atoms, and the closed silence realization says “没有作答” instead of the legacy “保持沉默”. No production code changed.

`rp_provider_test.go` now asserts exact accepted player/NPC quotations once and the paired reply/expression group against the committed expression's actual parent ID. Existing outbound filtered-context/privacy checks, actual HTTP count, private-data exclusion, provider success receipts, stop/restart/replay, same-observer history, projection comparison and rebuild remain. For unavailable/illegal/schema/timeout cases, the tests retain their distinct provider-failure categories, one attempted call, silence fallback receipts, audit sanitation and ledger/event counts. They additionally inspect the persisted canonical artifact for `Action=silence` with no invented text/expression/companion, exact independent IDs/groups, and replay without an extra HTTP call or ledger effect.

The two configured-provider tests (including all four failure modes) plus the corrected long interaction case passed in **6.795s**, using the local resource flags and `-run '^TestRPLLMProviderUsesFilteredContextCommitsAndRecoversWithoutModel$|^TestRPLLMFailureAndIllegalOutputSettleOnlyAuditedSilence$|^TestRPInteractionLongNarrativeFixtureStreamRecoveryAndWorldInvariance$' -count=1`. Scoped formatting and diff checks were clean. HTTP endpoints were local `httptest` fixtures, not live model providers. The parent's earlier compiled full-suite failures remain historical failures until its own subsequent integration validation.


### Final parent full-suite fixture follow-up

The parent's earlier compiled full backend run completed with storage **FAIL, 1745.973s**, without timeout. Its additional failures were two historical shared-round fixtures that manually created pre-046/pre-048 schemas but invoked current turn orchestration without its independent application receipt columns; a 024 fixture that dropped the turn owner while retaining later presentation schema markers; and a safe-silence assertion using legacy wording. That full run remains a failed checkpoint.

Test-only corrections preserve the actual domain checks. The shared health/information fixtures install only independent application presentation migrations 073/077/078 before creating rounds with current orchestration; their shared target tables remain in their historical shapes until the existing staged migrations. Populated accepted/pending rows, receipts and foreign-key checks are unchanged. These are staged shared-domain migration fixtures, not full-schema historical snapshots. The 024 fixture removes dependent presentation tables and corresponding markers when removing their owner so real forward migration recreates every column; existing committed speech/NPC source rows remain. The false-claim test now pins the exact quoted “我有一百万。” as accepted speech, v2 source groups and canonical silence fact with no invented utterance/expression, while retaining no purchase/currency issuance and non-authoritative diagnostic checks. This validates speech provenance, not the truth of its assertion.

The first requested union failed in 22.828s only on another obsolete provider fixture lexical assertion: the closed renderer may realize beckon as either “招手” or “招了招手”. The test now requires the action exactly once and additionally pins recovered history lines, paired groups and all three independent IDs. Production rendering was unchanged.

Final focused union — **PASS, 22.802s**:

`TMPDIR=/tmp GOTMPDIR=/tmp GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930 /usr/local/go/bin/go test -p=1 -gcflags='corerp.local/backend/internal/storage=-dwarf=false' -ldflags='-s -w' ./internal/storage -run '^TestRPSharedHealthMigrationPreservesF3RoundRowsAndForeignKeys$|^TestRPSharedInformationMigrationPreservesPopulatedRounds$|^TestRPTurnProviderFailureSettlesSilenceWithoutPromotingFalseClaim$|^TestRPTurnSchemaUpgradeFrom024PreservesCommittedSpeechAndNPC$|^TestRPLLMProviderUsesFilteredContextCommitsAndRecoversWithoutModel$|^TestRPLLMFailureAndIllegalOutputSettleOnlyAuditedSilence$|^TestRPInteractionLongNarrativeFixtureStreamRecoveryAndWorldInvariance$|^TestRPRelevantDialogue' -count=1`

The union includes the four corrected cases, two configured-provider cases with all failure modes, the long stream case, and all character relevant-dialogue tests. Scoped formatting/diff checks were clean. This follow-up changed tests and this evidence only, not production source, preview data or runtime behavior; it used no real model, original32 rerun or full-suite rerun. The initial full FAIL is not converted into a full-suite PASS by this focused result.

## Accuracy followup delivery

The latest 079 application opening boundary and server-derived source-support ranges preserve the existing world owners and private/public inputs. New targeted recovery/privacy/artifact tests and fullHTTP passed. Clone078→079 and live activation preserve all198oldEvents/heads/13provider receipts/canonical hash;24oldsession openings remain0. One real defaultStep smoke in the existing R1 world retains4oldpublic turns, excludes3oldexpressions from current narration, commits only2speech+1currentSmile and survives restart with no extra effects/calls. The actual speech still assumes standing and a seat, so technical delivery is distinguished from semantic experience acceptance. See source-support-preview-release-2026-10-01.json; its initial zero-model-call harness failures are retained. Current new production has not rerun the full original32.079-aware reading is required after new-window artifacts; automatic downgrade/DB restore is not an established recovery route.
