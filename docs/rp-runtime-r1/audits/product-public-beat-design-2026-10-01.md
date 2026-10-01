# Public speech/expression coupling design

2026-10-01. Source inspected at HEAD `f236c69`, including the current v1 composition receipts. Design only: no production source changes, tests, provider calls or migration execution in this pass.

## Decision

Add an optional **server-derived public beat** linking an already heard utterance to an already witnessed expression from the same committed NPC decision batch. A v2 presentation unit may consume that pair and produce one natural sentence with a shared subject, exact accepted words and the witnessed gesture. The existing decision storage owner remains authoritative; the narrator chooses only approved presentation forms.

Atomic commit proves a shared decision, not simultaneous physical execution. Do not use the link to invent “while speaking,” “then,” “because,” agreement, mood, sincerity or intention. The coupling relation is an optional `CompanionEventID` on the public expression, not a new decision field. This change improves coupling and readability; it does not implement arbitrary prose, character voice, emotional inference or a new world owner.

## What exists and what is lost

| Source | Verified behavior | Implication |
| --- | --- | --- |
| `backend/internal/storage/rp_npc_expression.go:55–78` | An expression is its own `RPNonverbalAction`, batch index 1, with `causation_event_id` pointing to the parent effect. The owner freezes observer-specific claims. | There is a durable relation that can support public coupling after both effects pass observation checks. |
| `rp_decision_commit.go:143–157,279–285` | Speech/base effect and optional expression share an atomic batch, world time, epoch and final branch boundary. | Storage order is not a declaration of gesture duration or simultaneity. |
| `rp_initiative_commit.go:301–315,411,441–444` | Time-triggered initiatives use the same expression commit and complete-batch boundary. | The relation also works for initiative speech, if its facts are actually present in the authorized narrative window. |
| `rp_turn_view.go:55–68,128–160` | Current-turn speech needs hearing evidence; expressions need frozen witness claims. The expression query joins by batch but does not retain or check the explicit parent link in the returned fact. | The current loader loses coupling provenance. Do not reconstruct it from two nearby same-actor sentences. |
| `rp_turn_view.go:169,188–209,233–268,355–359` | Facts are perceptually filtered and ordered; the cross-turn window is capped at 64 rows and precedes the player's speech. | Build beats from the final included fact set. A clipped pair or a missing public member must remain standalone. |
| `rp_turn_view.go:365–412` | Actor/target identities are masked, and only authored public presentation cues are included. | Retain these boundaries. Private persona and private proposal JSON are unnecessary. |
| `backend/internal/core/rp_style.go:146–184` | Facts carry source/actor/action/text/expression/context; no coupling field exists. | Add a narrow optional public relation, keeping existing facts unchanged. |
| `backend/internal/narrative/composition.go:172–185,193–260` | v1 expands each fact separately, then concatenates atoms by layout. | v1 grouping is typographic, not a verified same-decision linguistic relation. |
| `rp_narrative_render.go:33–75` and migration `077_rp_narrative_composition.sql` | Receipts independently retain fact-only source IDs and validate exact ordered unique group coverage. | Keep this invariant for v2; a coupled sentence still consumes both source events exactly once. |

These are source-backed capabilities, not results of a new runtime experiment. In particular, no claim is made that the window currently delivers every possible initiative/overheard utterance. Its session-specific utterance join (`rp_turn_view.go:245,261`) remains a separate coverage issue to investigate if a reported example is missing speech.

## Loader contract

Refactor the existing internal builder to collect all its facts within one read snapshot. Resolve an explicit source boundary: settled turn `settled_sequence` for historical reads; the current complete branch head for in-progress deterministic settlement. Preserve chapter/window constraints. Do not hold this connection during provider calls or streaming.

Proposed storage interfaces, names illustrative but signatures concrete:

```go
func readRPNarrativeInputOnConn(
    ctx context.Context, conn *sql.Conn,
    sessionID, playerTurnID, playerEventID string,
) (core.RPNarrativeInput, error)

func attachRPNarrativeCompanions(
    ctx context.Context, conn *sql.Conn,
    instanceID, branchID, observerEntityID string,
    throughSequence int64, facts []core.RPNarrativeFact,
) error // annotate only validated public expression facts
```

Keep the external `Store.readRPNarrativeInput` call shape as a wrapper. Its callers keep the existing principal/session/control authorization; the beat helper is not a new read API or grant.

For a pair to qualify, the helper must establish all of these from immutable event/batch/attempt metadata and the observer's frozen claims:

1. Both source IDs exist in the final fact list, next to one another in existing source order: accepted speech (`speak/respond/refuse`) followed by `expression`. Do not move facts across another actor, another event or the player-turn/window boundary.
2. Expression `causation_event_id` equals that exact speech EventID; both events have the same batch, actor, instance, branch and world time. Require the expected base/expression indices and a committed NPC decision/initiative command and attempt. Matching world time or actor alone is insufficient.
3. The complete batch satisfies `last_sequence <= throughSequence`. Speech has the existing exact hearing claim for this observer; expression has the existing exact witnessed claim. The actor is the same before identity masking; the final public handles must also agree. A parent from an unseen member is never added to satisfy a pair.
4. Extract expression code and any visible target solely from the observer's claim. Never read `proposal_json`, `Private`, persona, provider output or an unseen target. The helper may query existing decision linkage metadata if needed, but not its private content.
5. Missing public member: emit no beat and render surviving facts normally. Inconsistent authoritative linkage/scope/claims: return `ProjectionDiverged`, not a stronger invented relation. Standalone player gestures are not NPC decision beats.

Use the same helper for current-turn and window facts. A 64-row window cutoff can exclude one member; it does not justify expanding the window, disclosing a hidden event or asserting the remaining expression was accompanied by speech. Budget failure remains explicit; never drop facts to fit a richer interface.

## Exact core additions

```go
// Add to RPNarrativeFact; set only on a validated public expression.
// CompanionEventID string `json:"companion_event_id,omitempty"`
// Meaning: parent speech from the same committed decision, not simultaneity.

type RPNarrativeCompositionUnit struct {
    Kind     string `json:"kind"`     // fact or speech_expression
    Ref      string `json:"ref"`      // fN; compound anchors its first (speech) fact
    Template string `json:"template"`
}
type RPNarrativeCompositionGroup struct {
    Layout string                       `json:"layout"`
    Units  []RPNarrativeCompositionUnit `json:"units"`
}
type RPNarrativeCompositionPlan struct {
    Version string                        `json:"version"`
    Groups  []RPNarrativeCompositionGroup `json:"groups"`
}

// RPNarrativeView.CompositionPlan *RPNarrativeCompositionPlan
//     `json:"composition_plan,omitempty"` — present only for newly saved v2.
```

Prefer the parent's smaller interface: an optional `CompanionEventID` on the expression fact. It points only to the publicly included parent speech EventID after the full loader check. No separate `PublicBeats` array or bN namespace is needed. The provider wire translates that public link to the parent's local fN reference; batch/command/decision IDs, hidden member counts and private sources stay out. Both facts remain individually represented. The model cannot supply or alter the companion relation.

`ValidateReadBudget` must account for the actual serialized enriched packet and check companion shape, existing adjacent members and public actor consistency. An absent `CompanionEventID` is valid, and existing v1 renderers can ignore it. The stronger database source check belongs to the loader, not to an interface struct populated by a model.

## v2 composition and natural output

Version: `corerp.fact-composition.v2`. Reuse `inline/lines` layouts. A fact unit retains the finite existing fact templates; a `speech_expression` unit initially permits one `joined` template, consuming its anchor speech and the immediately following expression with that companion link. No free text, labels, subject changes, custom connectors or new fields are allowed in the plan.

Example wire plan, given `f1` = an accepted reply and `f2` = its witnessed nod, and `f2.CompanionEventID == f1.EventID` established by the loader:

```json
{"version":"corerp.fact-composition.v2","groups":[
  {"layout":"lines","units":[{"kind":"fact","ref":"f0","template":"plain"}]},
  {"layout":"inline","units":[{"kind":"speech_expression","ref":"f1","template":"joined"}]}
]}
```

Server expansion can render: `阿岚回应：「我听着。」；点了点头。` This example illustrates a hypothetical already accepted quote and gesture, not fixture dialogue to copy into production. It shares the subject, preserves speech first and gesture second, and states no duration, simultaneity or private motivation. Punctuation/subject elision is the initial natural coupling mechanism. A refusal keeps its explicit refusal marker; exact speech, quotation nesting, visible gesture target and POV/tense rules remain server-controlled.

Do **not** render `她赞同地点头`, `她安慰道`, `一边点头一边说`, `说完才点头` or `因为担心而点头` from this relation. A nod proves a nod, not assent or worry. A shared timestamp and atomic transaction do not prove overlapping actions. If the finished product later requires simultaneous wording, the existing decision owner must first establish an explicit typed observable temporal relation; that is a separate world contract, not a narrator inference.

Expansion rules:

- Resolve each unit to one fact or the two ordered beat members; flatten to the independent full fact sequence, exactly once in original order. A beat does not exempt its members from coverage, uniqueness, budget or identity checks.
- Reject a beat with incompatible speech action/expression, foreign/unseen/nonadjacent sources, unknown ref, repeated member, reordered pair or template outside the declared finite set. Allow the model to choose ordinary separate fact units instead; coupling is optional presentation.
- Freeze full plan validation and server expansion before emitting the first paragraph. `FactGroups[i]` contains every source consumed by line i, including both members of a beat. Versioned streaming uses only `EventIDs`; singular `EventID` stays empty. No stream claims a completed receipt until save succeeds.
- If v2 generation fails, keep the prior selected receipt. A deterministic fallback may compose the same facts safely; persist its actual version and sanitized reason, never a fake v2 success label or remote draft.

## Receipts and compatible readers

Preserve the independent fact-ID arrays, lines, groups and existing canonical/variant boundaries from 077. Add v2 data in the next available additive migration (078 if unclaimed), rather than redefining already published 077: optional `composition_plan_json` and `composition_public_input_json` on variant and canonical receipts, default absent/null. The latter contains only the validated public input needed for stable expansion: ordered public facts with validated companion links, resolved style and authored public presentation/context. Exclude credentials, provider config, private actor state and raw internal linkage metadata. Bind it with a canonical input hash.

Persist both the actual v2 plan and public input snapshot atomically with prose/group/source metadata. Current identity familiarity and current authored presentation may change later (`rp_turn_view.go:394–412` uses current familiarity); therefore a historical v2 receipt must not be reinterpreted using newly unmasked identities or newly installed packages. The frozen public snapshot defines the historical presentation. New regeneration may use a newly authorized input and create another immutable variant; it does not rewrite history.

Proposed validation entry points:

```go
// Presentation grammar/template validation and deterministic expansion only.
func ValidateCompositionPlan(
    input core.RPNarrativeInput, plan core.RPNarrativeCompositionPlan,
) error
func RenderRPNarrativeComposition(
    ctx context.Context, input core.RPNarrativeInput,
    plan core.RPNarrativeCompositionPlan,
) (core.RPNarrativeView, error)

// Storage authorization + authoritative source association, no provider.
func validateRPNarrativeReceiptOnConn(
    ctx context.Context, conn *sql.Conn,
    instanceID, branchID, observerEntityID string, throughSequence int64,
    receipt rpNarrativeReceipt,
) (core.RPNarrativeView, error)
```

`rpNarrativeReceipt` is a private storage DTO containing version, saved lines/groups, independent fact IDs, optional v2 plan/public input/hash. Do not make it a new world record. Loader/read validation must distinguish artifact schema integrity from event authority: compare plan/groups to independently retained IDs, validate the frozen input hash, and verify claimed beat links and frozen observer evidence against authoritative source events. The snapshot alone cannot manufacture source authority. Where the current read already reconstructs authorized facts, compare source identities/order and link eligibility with that reconstruction, but do not replace historical names/style from the current projection.

For v2, re-expand with the pinned v2 renderer and require exact saved line/group agreement before returning or streaming. This protects against edited prose as well as corrupted groups. Keep the renderer version's rules stable once receipts are saved; a later grammar/template change requires another version. Coupling plans and frozen manifests are application artifacts, not an additional world owner.

Version dispatch must replace the current single-version equality (`rp_narrative_render.go:40`) and latest-version warning equality (`rp_style.go:469`, turn-result warning path):

| Stored version | Read behavior |
| --- | --- |
| Empty legacy | Return existing saved prose; retain legacy streaming behavior. No inferred beats, v1/v2 plan, backfill or stronger capability claim. |
| `corerp.fact-composition.v1` | Keep existing ordered unique group/independent-source validation and v1 capability disclosure. v2 fields absent. Do not retroactively couple or re-render saved lines. |
| `corerp.fact-composition.v2` | Require its typed plan, frozen public input/hash, independent sources and verified beats; validate/re-expand before returning. Use v2 capability disclosure. |
| Unknown | Fail closed with a bounded unsupported-version/divergence result. Do not silently downgrade a saved receipt. |

Keep explicit `CompositionVersionV1` and `CompositionVersionV2` constants, with a latest/default alias for new generation only. Export a version-aware capability helper so Read/Select/history/settled replay cannot advertise the latest capabilities for older saved receipts. Retain provider kind/mode `full_prose` for configuration compatibility; the receipt version states what it can actually do.

## Primary deterministic settlement integration

The first-turn path currently discards receipt metadata: `rp_turn.go:203–226` obtains a full `RPNarrativeView` from `renderRPTurnStyled`, then passes only `view.Lines` and a later `ObserveRPSession` cursor to `markRPTurnNarrativeReady`. That helper (`492–499`) writes only prose, status, sequence and time. Composition metadata added by later Read/regeneration cannot repair this primary-path omission. This is a concrete integration point, independent of whether a remote provider exists.

The loader's `CompanionEventID` must be populated for both current-turn and retained window expressions, after final perception filtering and identity masking. Keep the independently witnessed expression as a public fact; the companion lookup must require explicit `e.causation_event_id=speech.event_id`, matching actor/scope and committed lineage, then the shared complete-batch/observer check. Do not tighten the fact-selection join in a way that silently drops a visible standalone expression merely because no public speech companion qualifies. Never populate a companion from an absent speech fact or from equality of world time. An expression that follows silence/wait has no speech companion.

Avoid a core↔narrative import cycle. Place the **v2 pure server renderer and deterministic plan builder in core**, operating only on authorized public input and typed finite plans; the model-facing narrative package supplies a candidate plan to that same renderer. Keep existing v1 expansion available for historical compatibility. Move version-specific constants/capability lookup to a dependency-neutral core module, with narrative exports kept as aliases for compatibility. Exact shared interfaces:

```go
func BuildDeterministicRPNarrativePlan(
    input RPNarrativeInput,
) (RPNarrativeCompositionPlan, error)

func RenderRPNarrativeComposition(
    ctx context.Context, input RPNarrativeInput,
    plan RPNarrativeCompositionPlan,
) (RPNarrativeView, error)

func RPNarrativeCapabilityWarning(version string) (string, error)

// Internal storage result; public input and bound come from one snapshot.
type rpNarrativeBuild struct {
    Input           core.RPNarrativeInput
    View            core.RPNarrativeView
    ThroughSequence int64
}

func (s *Store) buildRPTurnNarrative(
    ctx context.Context, sessionID, playerTurnID, playerEventID string,
    style core.RPStyleProfile,
) (rpNarrativeBuild, error)

func (s *Store) markRPTurnNarrativeReady(
    ctx context.Context, runID string, build rpNarrativeBuild,
) error
```

`core.DeterministicRPNarrativeProvider` for the new path derives a complete v2 plan itself: verified adjacent pairs may use `speech_expression/joined`, everything else remains a fact unit, all in canonical source order. The core renderer returns version, exact ordered EventIDs, per-line FactGroups and the actual plan, with capability disclosure. Model generation/regeneration uses the identical renderer and constraints; the provider cannot create a different interpretation of a companion.

The primary storage write validates the view against `build.Input`, then atomically saves `narrative_json`, version/groups/independent fact IDs, v2 plan/frozen public input/hash, and the pinned sequence while transitioning `npc_effects_committed → narrative_ready`. Recheck run stage, actor/session authority and source boundary under the write transaction. For a newly advanced branch after the build snapshot, reject with a branch conflict and rebuild the presentation from a fresh snapshot; do not rerun already committed NPC decisions. Do not use a separately read later observation cursor as the snapshot's fact boundary. The subsequent existing settlement transaction changes application status only, and a restart at `narrative_ready` retains the full validated v2 receipt rather than regenerating it.

`renderRPTurnStyled` can remain as a view-only compatibility wrapper over the new build/render function; the primary caller must use the richer build object. Historical helper consumers returning only lines may remain lines-only, but they are not the receipt owner. No world Event/branch head is added by the presentation write.

Integration points after persistence:

- `loadRPTurnRun` and the existing retry lookup load all receipt metadata together with canonical prose. `loadRPTurnResult` validates v2 against the frozen input/source association and returns its groups/version; append version-specific capability disclosure **after** deterministic warning reconstruction. Do not rebuild a v2 plan simply to load it.
- `ObserveRPSession` and `SelectRPNarrative` validate the selected or canonical v2 receipt before returning it. Canonical restoration restores its original plan/group attribution. Variant saves remain independent artifacts and never overwrite the primary receipt by selection.
- Saved stream reads preserve actual per-line EventIDs and leave singular EventID empty for both v1 and v2. Source sequence remains speech followed by expression; the renderer must not place an expression before speech to imitate “笑着说,” or infer physical simultaneity from an equal world time.
- Current `streamRPNarrativeWithProvider` uses `savedAt.Valid && savedMode != "base"` as its saved-canonical shortcut (`rp_style.go:364`), while deterministic settlement stores a base receipt. Updating persistence alone would still route an ordinary Read through later generation. The finished-product default should return a valid primary v2 snapshot on an ordinary read even when mode is base/presented-at is absent. Explicit regeneration/provider presentation produces an immutable variant. If the product intentionally preserves the existing one-time official full_prose upgrade instead, make that a named explicit presentation operation with atomic v2 replacement and stable facts; do not make a generic refresh silently rewrite the just-settled canonical receipt. This policy change must be reflected in the Play caller alongside the storage gate.

This primary receipt requirement should be the first vertical slice: deterministic Play → persisted full v2 receipt → immediate observation → restart/replay → canonical read/restore, with no configured narrative provider. Only after that passes should a model choose v2 layouts. Regeneration-only success is insufficient product acceptance.

## Observable acceptance before implementation expands

Design acceptance is source-backed: both public source effects and their explicit parent link already exist; the missing element is retaining that relationship across the authorized loader/core boundary. Runtime acceptance is NOT VERIFIED in this design pass.

A minimal future vertical slice should demonstrate one accepted reply + witnessed nod becoming one v2 paragraph, with both exact source IDs, unchanged words/head/effects, stable restart and canonical/variant selection. Cover hearing-only/seeing-only/no-target visibility, silence-expression, initiative/window pair, pair clipped by the window, equal-time unrelated decisions, foreign scope/actor, incorrect parent link, incomplete batch, unknown/duplicated/reordered beat members, nested quotes/refusal, changed familiarity/package after save, corrupted plan/input/groups/prose, interrupted stream and legacy/v1 reload. The server must reject fabricated chronology/motivation even when the same-decision link is valid.

No execution or test result is claimed here. The smallest implementation reuses storage ownership, observable facts, frozen claims and existing receipt tables; it adds a public relation, a finite paired presentation unit and versioned artifact validation. It does not require a private-state ledger, autonomous agent framework or another world authority.


## Authorized implementation checkpoint — 2026-10-01

The later BUILD delegation implemented this design through the shared core APIs in the product integration contract. The actual handoff is `RPNarrativeView.Artifact`, rather than the candidate `rpNarrativeBuild` wrapper above. Storage retains the complete public input, finite plan and input hash in additive migration 078; 077 remains the independent ordered fact-ID/group receipt. No legacy or v1 row is backfilled or relabeled.

Primary Play captures one short snapshot, releases its connection before rendering, then saves prose/version/groups/fact IDs/artifact/settled sequence and advances the session observation cursor atomically at `narrative_ready`. A changed head rejects the presentation save without repeating NPC effects. Restart and replay validate the saved receipt. The source validator checks complete committed batches (actual event count/range, command and attempt status, scope and head), frozen hearing/nonverbal witness claims, and exact expression parent lineage. Equal batch/time alone is insufficient.

Reload reconstructs the owning turn's complete source projection at the frozen head, checks every typed fact field, historical package labels and published character cues, and re-expands the strict finite plan. Historical recognition is resolved at the saved head. Actor names come from scoped committed materialization; a future entity-rename owner would need its own historical event resolution. The existing spatial rename owner is supported: saved place names survive a normal `RenameRPLocation` and restart unchanged. Canonical heads equal the turn's settled boundary; variants may freeze a later complete boundary but retain this turn's required source set.

Default ordinary v2 reads return the saved natural receipt without a provider call, including base mode and a small optional regeneration budget. The parent's integration policy preserves explicit `FullProse=true`: a configured full-prose provider may refine the base receipt once, using its validated frozen input and unchanged input hash. It cannot substitute today's head, identity, cues or facts. Successful refinement replaces only canonical presentation; later ordinary/reconnect/restart reads cache it. An unavailable capable provider returns the natural receipt with truthful fallback metadata. Explicit style overrides remain immutable variants.

Concurrent first refinements may incur two provider attempts, but their emission is buffered. After the compare-and-save operation each reader reloads and emits the committed canonical winner, with no losing selected variant and no world/head change. Providers and stream callbacks never retain the DB snapshot.

Finite implementation evidence is recorded in `2026-10-01-commit-recovery.md`. This checkpoint does not claim a real-model comparison, full integration suite, release, or human experience acceptance; those remain parent-owned.
