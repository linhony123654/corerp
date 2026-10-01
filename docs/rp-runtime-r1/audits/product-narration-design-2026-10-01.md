# Expressive public narration v2 — product design, 2026-10-01

Build a small source-entailing prose grammar that composes public events into natural dramatic beats, rather than printing one immutable sentence per event. The model selects discourse, rhythm and eligible lexical realizations; a server compiler owns people, targets, exact dialogue and what each clause asserts. Product acceptance requires readable, character-appropriate scenes, not just a valid plan and correct source IDs.

This is design only, following the user's delegated product direction. No code, provider call, installation, test rerun or deployment was performed for this report. Paths below refer to the current working tree after the v1 accuracy increment. Proposed interfaces and examples are specifications, not implemented APIs or observed outputs.

## What the source already supplies, and what it loses

| Verified source | What it proves | Current loss / v2 consequence |
| --- | --- | --- |
| `backend/internal/storage/rp_decision_commit.go:32-34,143-150,279` | Main observable decision and optional expression commit as separate events in one atomic decision batch. Expression occupies the next event sequence. | Batch membership is available for presentation association without adding a world owner. Event sequence is ledger order, not measured gesture start/end time. |
| `backend/internal/storage/rp_npc_expression.go:55-75` | Expression event has `batch_id`, `causation_event_id` pointing to the main decision event, the same actor, and frozen witness-specific observations. | The narrator can derive a public companion relation from authoritative provenance rather than proximity. Do not transmit raw decision/private metadata. |
| `rp_npc_expression.go:16-27,43-49` | Current NPC smile/nod/shake_head/frown/turn_away are undirected; beckon alone supplies the interlocutor as target. Witness target visibility is recorded separately. | “贾母向你笑了笑” is not licensed by an undirected smile. Same NPC/interlocutor does not fill an absent target. |
| `backend/internal/storage/rp_turn_view.go:55-68,128-160` | Speech requires a heard observation; expression requires its own witnessed observation. Expression query finds a sibling by batch. | Tighten fusion eligibility to include exact parent causation, actor, instance/branch and complete committed batch lineage. Current narrator DTO has no association field. |
| `rp_turn_view.go:169,233-268,355-359` | Current-turn facts sort by sequence; cross-turn window reverses sequence-descending results to oldest-first and precedes player speech. | Preserve sequence/origin and authorized companion links through both paths. A shared timestamp, adjacent array entries or common display name is insufficient. |
| `rp_turn_view.go:367-410`; `backend/internal/core/rp_style.go:146-184` | Actors/targets are observer-masked; public style is a separately sourced declaration. DTO preserves exact dialogue, action, expression, object state, place and time. | No public pronoun/gender, temporal-overlap or current prosodic fact is present. Do not infer “她”, “低声” or “笑着说” from names/style/persona. |
| `backend/internal/storage/rp_session.go:481-490` | Existing history suppression already matches parent batch, causation, actor, instance/branch and observing settled session. | Reuse this lineage discipline when deriving eligible narration beats; grouping is presentation, not new authority. |
| `backend/internal/narrative/composition.go`; `prose.go:205-213` | v1 has exact speech/actor/action atoms, four bounded framing choices, complete source coverage and line-specific source groups. | Keep these invariants; replace the sentence vocabulary and beat compiler, not the owner/ledger. |

The existing Jia Mu sample in `docs/rp-runtime-r1/rongqing-minimal-short-samples-2026-10-01.json:35-73` contains player greeting, accepted Jia Mu reply and a public smile. That reply includes “身子可好些了？” without a supplied illness premise; v2 must preserve it as a speaker's question, never endorse an illness or quietly fix upstream dialogue. The public declaration in `rongqing-minimal-test-fixture-2026-10-01.json:73-77` permits an affectionate, composed narrative register. Its private persona and wish to hear Bao Yu out are not narrator evidence.

## Product promise and acceptance scene

A player should read a short scene in which replies and witnessed expressions flow together, speakers remain unmistakable and the next player choice remains open. Different density and style settings should change how the same facts read: crisp exchange, connected scene, or spacious but economical narrative. They must not change the world or rewrite accepted words.

The first shipped slice must satisfy these simultaneous criteria:

1. On the greeting/companion-smile fixture, standard prose contains one natural Jia Mu reply beat, not separate repetitive “贾母回应…贾母笑了笑” audit rows. No raw ISO clock repeated before every utterance.
2. On a richer four-beat fixture, concise/standard/long differ in paragraph structure, sentence placement, contextual focus and permitted lexical register. Long expands available scene structure, not repeated explanations or extra facts.
3. Exact quotes occur once for each speech event, including identical words in two events, nested delimiters, literal token-looking strings and embedded newlines. Refusal remains explicit even if its words are ambiguous.
4. The grammar offers materially different readable realizations, including quotation-first, subject-first, shared-subject speech/gesture clauses, action-to-dialogue transitions and one-time sourced scene orientation. It cannot be implemented as four renamed wrappers.
5. Public author style affects eligible narrative diction and rhythm. It never changes accepted speech, certifies the character's current feeling or supplies a physical vocal/gesture modifier.
6. Human review of paired same-fact examples judges attribution obvious, prose natural and published style distinguishable without spotting added scene facts. A legal JSON response or compiler unit pass does not satisfy this criterion.

## Beat association and temporal semantics

Derive a **public companion** only after independently selecting each fact for this observer. The internal builder verifies: exact expression causation points to the main event; same committed command/attempt and complete batch; same actor, instance and branch; both sequences within the settled narrative snapshot; relevant hearing/witness observations; compatible main action. Record which public facts the relation joins, not private proposal fields or raw command IDs.

Companion membership means “these observable effects belong to one accepted decision”. It permits co-presentation in a paragraph or coordinated clause. It does not mean “the gesture happened while the words were spoken”, “the gesture answered the player”, or “the gesture caused the reply”. Distinguish these relations explicitly:

| Relation | Source requirement | Permitted prose | Not licensed |
| --- | --- | --- | --- |
| `companion` | Verified parent/batch/actor lineage; both observed | `「…」贾母应道，笑了笑。` Two facts coordinated without a temporal adverb. | `贾母笑着说…`, `一边点头一边…`, `因而…`, redirected gesture. |
| `ledger_order` | Independently sourced fact order | Arrange paragraphs in committed order; neutral juxtaposition. | Physical duration, pauses, “立刻”, “片刻后”, or proved simultaneity. |
| `physical_before` | Explicit existing/future observable temporal relation, not insertion sequence alone | “说罢…” or “随后…” only if relation specifically licenses that order. | Treating batch index 1 as an observed “after speaking”. |
| `overlap` | Explicit committed public temporal relation with witness scope | `贾母笑着应道：「…」` for an overlapping smile/speech pair. | Promoting equal WorldTime values to overlap. |

V2's first slice implements companion and ledger ordering, not invented overlap. If the product later requires “笑着说”, the existing decision owner must commit an explicit observable relation with documented witness semantics, or the authoring/runtime contract must already prove it. A renderer-only boolean is not such proof. This is a narrowly scoped observable relation, not a private-intent lifecycle or new world authority. Historical same-batch events retain unknown physical timing.

If one sibling is unheard/unseen, render the authorized fact alone. If either sibling is outside the window or across a chapter boundary, no fusion. Do not retrieve an invisible sibling to complete a prettier sentence. No model-specified beat membership is accepted unless it matches a server-supplied candidate.

## A compositional realization grammar

Model output selects **eligible beat candidates** and realization/rhythm IDs. A candidate is a singleton or an authorized companion group and contains canonical ordered fact refs. The compiler instantiates typed slots from those facts. It builds an AST before any streaming; no arbitrary connective text is accepted.

The grammar has separable production families rather than whole-paragraph templates:

- **Speech envelope:** subject-before-quote, quote-before-subject, compact label, postposed attribution. `speak` can use “说/道”; `respond` can use “回应/答道/应道”; `refuse` requires “拒绝/婉拒” only if the latter's politeness is explicitly licensed—default is the plain refusal marker. “问道” requires a typed question speech act, not a model's guess from punctuation. “解释/劝慰/打趣/许诺” need an applicable typed act or entailment-approved source tag and are unavailable initially.
- **Gesture realization:** smile → “微笑/露出笑容/笑了笑”; nod → “点头/点了点头”; shake_head → “摇头/摇了摇头”; frown → “皱眉/眉头皱起”; turn_away → “转过身/转身”; beckon → “招手/招了招手”. Validate each realization against the precise definition of its expression code: no laugh versus smile ambiguity, amplified force, duration, “又”, lowered head or implied emotion. A lexical pack must not quietly broaden a fact.
- **Shared-subject fusion:** combine compatible speech and expression under one server-resolved subject, with a neutral comma/semicolon or paragraph boundary. Quote-first postposed attribution followed by subject ellipsis can avoid repeated labels. No new participant, recipient, causal connective or physical timing modifier enters the clause.
- **Action realization:** arrived/departed, activity start/complete/interrupted and object state each have fact-preserving lexemes. Starting tea preparation is not bringing a cup or completing it. Source-safe phrase choices such as “开始/着手” require the same typed activity meaning; raw code is not permission to invent task details. Never add facial/hand/body motion to paraphrase an object interaction.
- **Discourse:** introduce sourced place once at a scene transition; omit redundant orientation, join same-scene beats, choose attribution position and sentence length, and split paragraphs at speaker/action changes. A timestamp can be omitted as presentation; converting it into “傍晚/翌日/片刻后” requires a validated world-calendar/elapsed-time mapping. No weather, lighting, furniture, acoustics or “空气凝住” without their own observed/canonical scene facts.

Each realization entry is data plus executable guards: `id`, `version`, `required_fact_kinds`, slot bindings, effect obligations, allowed temporal relation, subject/target requirements and surface AST. A text string from the model or author's public cue is never a new realization body. Initially ship a reviewed server pack; an eventual Studio-authored pack requires its own validation and immutable version/hash. Reuse existing narrative-pack selection and don't create an uncontrolled per-character prose engine.

The model chooses among contextual candidates under a budget and diversity objective. Penalize mechanically repeating the same attribution envelope across consecutive eligible speech beats; prefer a shared-subject fusion over two identical subject openings. These are presentation objectives only. Variety cannot license a variant that fails semantic guards. A deterministic default selector using the same grammar must produce readable fallback, not resume the current audit-style rows whenever the model is unavailable.

## Jia Mu literary examples and exact evidence requirements

These are authored acceptance examples, not claimed live outputs. The new acceptance fixture's exact speech is explicitly declared here so it cannot masquerade as existing player/world history:

- `f0`: player/Bao Yu speaks `老祖宗，我来看您了。`
- `f1`: known Jia Mu responds `宝玉来了。有什么话，慢慢说给老祖宗听。`
- `f2`: independently witnessed undirected Jia Mu smile, verified companion of `f1`; physical timing unknown.
- Optional rich fixture: `f3` known Xi Ren arrives at 荣庆堂; `f4` Xi Ren speaks `老太太，茶还温着。`; `f5` Jia Mu refuses with exact words `先放着。` and `f6` is a witnessed undirected shake_head companion. These are test-author commitments, not facts inferred from the novels or f4's claim about tea. No physical tea/cup is present in narrative evidence.

**Concise, same f0–f2:**

> 你说：「老祖宗，我来看您了。」
> 贾母答道：「宝玉来了。有什么话，慢慢说给老祖宗听。」笑了笑。

Coverage groups `[f0]`, `[f1,f2]`; a reply and its smile read as a beat, with no claim that the smile is directed toward the player or concurrent with the words.

**Standard, same f0–f2:**

> 「老祖宗，我来看您了。」你道。
>
> 「宝玉来了。有什么话，慢慢说给老祖宗听。」贾母应道，笑了笑。

The quotation-first envelope produces a different rhythm without “亲昵地”, invented pauses or private affection. The public author cue can favor the classical “道/应道” register and this measured layout, not certify a new emotional state.

**Long, rich f0–f6 with sourced scene orientation:**

> 在荣庆堂，你开口：「老祖宗，我来看您了。」
>
> 「宝玉来了。有什么话，慢慢说给老祖宗听。」贾母应道，露出笑容。
>
> 袭人来到荣庆堂，说道：「老太太，茶还温着。」
>
> 「先放着。」贾母拒绝道，摇了摇头。

This is intentionally not padded. It varies envelopes, fuses companions, leaves room between speakers and keeps refusal explicit. It does not put a cup in anyone's hand, let Bao Yu sit, turn tea into an object fact, or narrate Jia Mu's motives. The arrival-to-speech clause shares an immutable same-actor subject across adjacent singleton beats; it uses neutral coordination, not an overlap assertion. This cross-beat subject-elision production must be guarded by exact scoped identity equality and retain both source refs. It does not require a guessed gendered pronoun.

**First person, same rich evidence:** replace only controlled-player labels with “我”, e.g. `「老祖宗，我来看您了。」我道。` Accepted quote text is unchanged. Third person uses the already-public player name. Jia Mu remains the immutable actor of her reply and gestures; public-only same-name handles use stable scoped identity slots, never guessed pronouns.

**Overlap version, only with an additional explicit observed overlap relation:**

> 贾母笑着应道：「宝玉来了。有什么话，慢慢说给老祖宗听。」

This is a target expression of natural fusion for a future properly sourced overlap case. It must be rejected on today's companion-only fixture. The literary gate must not tempt implementers to reinterpret transaction atomicity as physical simultaneity.

**Existing live/sample quote retained:** with the accepted “身子可好些了？” speech, quotation-first and companion coordination are allowed; `病已好转`, `贾母想起你先前病着`, `她疼惜你` or `你走过去让她看` are not. The narrator cannot repair an upstream incorrect premise by changing the line, nor worsen it by stating the implied illness as a fact.

## Proposed DTO and schema boundaries

These are interface requirements, not code added in this design task.

1. **`core.RPNarrativeFact`:** retain all current fields. Add internal/public-safe `origin_sequence`, `origin_kind` (current player/current NPC/window), and an opaque scene ref if needed for identity-independent scene changes. Preserve full source EventID internally. Use observation-derived place/target; do not carry decision private text.
2. **`core.RPNarrativeInput`:** add versioned `beat_candidates` with candidate ref, canonical ordered fact refs and relation enum `singleton/companion`; optional typed temporal relation exists only if separately sourced. Add frozen `public_entities` entries keyed by scoped opaque ref, display label, controlled status and optional explicitly public pronoun/reference forms. Existing masked actor/target identity remains authoritative. Add resolved `realization_pack_ref/hash` and sourced public style choices; free author cue remains data, never grammar code.
3. **Builder snapshot:** gather facts, observed identity, candidate lineage and author presentation from a consistent settled/head-bounded read. Snapshot authorization/settled upper sequence and historical observer visibility must be explicit. Current DTO drops the metadata; do not make the model rediscover it by joining strings. At privacy boundaries a masked target ref remains masked throughout.
4. **Actual model input:** supply local `fN/aN/bN` refs, exact public quotes, public action/target labels, eligible realization IDs and relation restrictions. Supply only candidates formed from public facts. No raw command/batch/private memory IDs. Retain actual wire-budget accounting after adding candidate/lexical options; if exceeded, reduce eligible choice multiplicity, never facts or quotes. If minimal choices still cannot fit, use deterministic grammar fallback with a diagnosed receipt.
5. **Response:** `version = corerp.fact-composition.v2`, paragraphs containing selected `{beat_ref, realization_ref, rhythm}` nodes. `rhythm` is a finite audited enum, not a text/modifier slot. Example for singleton player plus neutral fused Jia Mu:

```json
{
  "version": "corerp.fact-composition.v2",
  "paragraphs": [
    {"beats": [{"beat_ref": "b0", "realization_ref": "speech.quote_then_subject.classical@1", "rhythm": "compact"}]},
    {"beats": [{"beat_ref": "b1", "realization_ref": "reply.quote_then_subject.gesture.coordinate@1", "rhythm": "measured"}]}
  ]
}
```

`b0` covers `[f0]`; `b1` covers `[f1,f2]`. Candidates must form an exact ordered partition of all supplied facts. Competing singleton and companion choices may coexist in the input, but the output cannot consume the same fact twice. Binding actors/targets/quote strings is exclusively compiler work. No prose-valued field or arbitrary actor/lexical modifier is accepted.

6. **Compiler semantic checks:** unknown/null/duplicate/extra fields fail; references/pack version must match input; each candidate's facts pass actor/target/action/relation guards; selected candidates cover each fact exactly once in canonical sequence. Each generated AST node must discharge its effect obligations (speech + refusal marker + expression, not IDs alone). Same name/different ref never shares a subject. No rhetorical verb asserts evidence not present. Claims inside quotes stay claims.
7. **Renderer/view:** retain `Lines`, full canonical `EventIDs`, `FactGroups`, `CompositionVersion`. Add optional per-clause node/source map for evidence inspection; one compiled fused beat may have two fact sources. For v2, FactGroups contain each paragraph's facts in canonical order, independent of safe attribution placement inside that paragraph. Semantic expansion finishes before first chunk. Stream whole compiled paragraphs so embedded quote newlines survive.

## Persistence and compatibility

Current `storage/rp_narrative_render.go:30-75` recognizes one version and checks complete ordered groups against a separate fact-only array. V2 must add a version-dispatched validator without loosening v1 or silently upgrading legacy. Current canonical/variant writes already persist version, independent fact IDs and groups (`rp_style.go:535-536`; `rp_narrative_render.go:158`); reuse these fields for v2.

Persist immutable v2 resolved plan JSON, realization pack version/hash, authorized public input/beat-relation digest and style/source provenance separately from generated lines. A stored plan is presentation evidence, not a world event. Do not persist NPC-private inputs. Keep fact-only IDs separate from authored style/pack/context IDs. Replay never calls a provider. Read existing generated lines exactly; validate versioned source coverage/pack identity against independent saved evidence and, for regeneration, the authorized reconstructed public input. Corrupt unknown/reordered/duplicate fact groups or mismatched pack/plan digest must fail with ProjectionDiverged rather than claim a strong contract.

Do not rerender saved v1/legacy on load. A historical empty version remains legacy, v1 retains its capability disclosure, and v2 has a narrower accurate warning only for requested unsupported effects/lexical goals. Style overrides produce a new selectable variant of the same facts; rejected regeneration preserves current selection. Parent-owned canonical/selection/history/turn/NDJSON interfaces must expose v2 metadata consistently. `src/lib/narrativeStream.ts:1,14-53` currently returns the limited view fields and does not retain composition metadata; a product implementation needs deliberate client types, history round-trip and capability-display behavior, not only a backend column.

Saving pack hashes and plans adds a small additive migration; it does not create a competing simulation state. Known v1 rows retain their independent fact-array/group validation. Recovery tests must open a fresh store, select/read both versions, preserve exact quote bytes and detect separately mutated linkage/plan/source fields.

## Style, lexical safety and honest limits

Published “亲切、从容，带长辈亲昵” is an authored expression cue, not proof of a present smile, vocal softness, attention, family relationship or emotion. V2 can use a less clipped rhythm, classical attribution lexemes and consistent character-specific envelope preferences. It cannot rewrite accepted speech into more affectionate vocabulary. Unpublished private persona never trains a scene realization at runtime.

Global instructions map to supported dimensions: compact/measured rhythm, narration register, attribution placement, scene-orientation frequency and paragraph density. An instruction such as “多写心理” or “让她更疼爱我” requires an explicit unsupported capability notice. Do not claim all free instructions are understood. Keep style demand resolution visible in the receipt, e.g. applied/unsupported instruction classes with no private text. Public cues can prefer diction only among already-reviewed fact-entailing choices.

A phrase registry is not automatically safe because it is finite. “缓缓点头/叹道/慈爱地/强忍笑意/再次/终于/仍然” each add speed, sound, feeling, restraint, repetition or continuity that today's facts do not establish. Each needs a source guard or removal. Equal world-time values are particularly dangerous because many events happen at a single frozen world-clock instant. A model or unit test cannot define away these semantic additions.

Naturalness also depends on upstream dialogue and availability of observable events. This design must improve supported scene presentation, but it cannot make a bland/inaccurate accepted line witty or truthful while preserving its bytes. Keep decision dialogue quality and narrator literary quality as separate product evaluations on the same authored world. If a target scenario genuinely lacks required props/lighting/activities, use its existing typed authored/observed scene channels; do not invent decorative facts to pass a prose review.

## Delivery gates for a playable result

**Design gate (this report):** required source relation, grammar, exact Jia Mu examples and compatibility contract are specified. Implementation and experience gates are NOT VERIFIED.

**First vertical slice:** preserve existing public observation/identity projection; add companion candidates from proven lineage; compile speech + undirected smile into quotation-first/shared-subject prose; implement a small reviewed registry with at least three distinct speech envelopes, two neutral companion envelopes and two realizations for each supported gesture. Add one-time sourced scene orientation and explicit refusal semantics. Complete it through saved read, regeneration, selected variant, streaming and Play display before expanding the registry.

**Contract verification:** swapped actor/target, identical-text events, omitted/duplicated fact, hidden sibling, different batch/actor with identical WorldTime, same-name actors, false overlap, quote token/nesting/whitespace, refusal without obvious refusal words, physical object claim inside speech, private cue injection, budget exhaustion and corrupt/restart receipts. Require hard semantic obligations for compiled effects in addition to JSON/coverage checks. Keep historical guard tests and frozen negative fixtures.

**Literary product verification:** on an explicitly authored Jia Mu world, compare v1 and v2 using identical public fact inputs. Review greeting with smile, nondirected nod with silence, targeted beckon, ambiguous refusal, an arrival interrupting conversation, and a multi-speaker exchange. Require the reviewer to judge varied natural prose, unmistakable speakers, appropriate published register and no invented player behavior or scene information. A reported count of eligible variants is not acceptance. Keep two held-out scene cases so authors cannot tune only the examples above.

**Real-model and run verification:** later authorized provider evaluation checks actual schema adherence, retry/latency and literary selection on those fixed facts; it does not permit changed world inputs between comparisons. Playable completion then requires the parent-owned end-to-end runtime, original required full32/recovery and human Golden, plus release-specific verification/authority. No real-model, R1 or production claim is made here.

Finite design handoff: proceed with the companion/neutral-fusion grammar slice using existing committed provenance. Ship a readable source-entailing fallback and a versioned choice engine together. Do not stop at DTO/schema validity or expand private-intent architecture to compensate for narration.

## Primary Play path: natural default with no second narrator request

The design must improve the first settled result, not only a later provider variant. Verified current path: `backend/internal/storage/rp_turn.go:212-226` calls `renderRPTurnStyled` and stores its lines before settling; `rp_turn_view.go:20-26` calls the core deterministic provider. `src/components/PlayWorkspace.vue:327-353` subsequently requests narrative streaming without an override and displays the returned variant. Thus moving good prose only into `ChatProseProvider` leaves the primary/default and saved-base experience mechanical and can require a second HTTP/model wait. This is a product-path defect in any v2 plan that ignores the first renderer, not evidence that every current session makes a real narrator call.

**Chosen architecture:** one public beat builder, one pure realization compiler, two plan selectors. The core natural default selector is local/deterministic; the existing narrative adapter can optionally select a richer plan using a model. Both feed exactly the same compiler, actor/target bindings, lexical registry, effect obligations, paragraph logic and provenance validator. There is no alternate prompt feeding free text into a different renderer.

Place the pure plan types, realization grammar/registry, default selector and compiler in core-owned files (or refactor types into a genuinely shared lower package). Since `internal/narrative` already imports core, core must not import narrative to reuse the compiler. A feasible initial interface is `core.CompileRPNarrativePlan(ctx, publicInput, plan, resolvedPack) -> RPNarrativeView`; `core.SelectNaturalRPNarrativePlan(publicInput, resolvedPack) -> plan`. These names are proposals. `ChatProseProvider` performs transport/closed selection only and invokes that compiler. The old literal deterministic provider remains available for audit/legacy compatibility while the primary Play call selects the new natural provider. Do not rename or alter historical saved rows to simulate continuity.

The natural selector performs useful discourse work without an LLM:

1. Choose the authorized companion candidate over competing standalone speech/gesture candidates when a neutral fused realization is available. If either member is absent or relation validation fails, preserve the authorized singleton; no guessed linking.
2. Prefer one orientation at a real sourced scene boundary; suppress repeated time/place prefixes within the same scene. Clock detail remains available in provenance/inspector and deliberately requested detailed context, not mandatory every sentence.
3. Resolve paragraph boundaries from supplied speaker/action changes. Collapse shared-subject labels only across exact same scoped actor refs; anonymous same-label people never collapse.
4. Choose readable source-entailing envelopes by action and published register; quote-first is useful after a subject-first opening, and reply/gesture coordination avoids the redundant second character label. Track only bounded within-turn realization choices to avoid immediate repetition. Selection is deterministic from source/plan version and style so replay/restart never randomize the saved reading.
5. Implement density through available structure: concise uses a short exchange and combined beat; standard uses connected paragraphs and varied attribution placement; long uses source-rich scene/actor transitions and more space, without stretching two utterances into imaginary atmosphere. No invented delay, physical overlap, gaze, prosody or affect is a diversity option.

**Normal Play policy:** return and display this natural primary result as soon as the turn settles; it is a complete readable artifact. Saving canonical v2 must take the complete view (not `view.Lines` alone) so its plan version, independent fact sequence and groups survive the primary path. Plain read/reconnect returns the saved natural artifact with no provider attempt. Make extra model selection an explicit optional style-regeneration/refinement action using the same facts/compiler. If the product retains an explicitly chosen automatic-refinement setting, the natural primary artifact must still be displayable and recoverable while that optional presentation runs; it must not hold the completed player action behind another unavailable model. Model profile/FullProse semantics and UI request policy need coordinated parent implementation so an explicit user's paid refinement choice is not silently ignored.

`markRPTurnNarrativeReady` currently receives lines at `rp_turn.go:226`; the v2 product increment must propagate full metadata in that base/canonical write and in the turn result. The later stream request, selected variant, history and saved-original action need a consistent version/capability contract. Do not rely on the later provider endpoint to attach a composition version to a primary artifact it did not compile.

### Default prose before code: readability audition

For the authored f0–f2 fixture from this report, a no-provider default can render:

> 「老祖宗，我来看您了。」你说。
>
> 「宝玉来了。有什么话，慢慢说给老祖宗听。」贾母应道，笑了笑。

This sounds like an exchange rather than a receipt. It binds the smile to Jia Mu, adds no recipient, avoids repeating her name, retains every word and lets the player choose what to do next. It is not claiming that she smiles during speech; the coordinated clauses have unknown finer physical timing. A public classical register chooses “应道”; neutral modern register uses “回答”. It cannot choose “温柔地/宠溺地/低声/含笑说” on these sources.

On the richer authored fixture, the same local selector can render:

> 在荣庆堂，你说：「老祖宗，我来看您了。」
>
> 「宝玉来了。有什么话，慢慢说给老祖宗听。」贾母应道，露出笑容。
>
> 袭人来到荣庆堂，说道：「老太太，茶还温着。」
>
> 「先放着。」贾母拒绝道，摇了摇头。

The improvement is joining related effects, varied attribution positions, scene orientation only once and a compact arrival-to-speech transition. It uses the same verified neutral relation policy as the optional model selector. It does not narrate illness, warm tea as an objective prop, taking a seat, looking at the player, bodily timing or private care. A smile realization must remain a smile; “笑出声来” is not another stylistic synonym.

**Assessment:** these examples are plausibly playable as clear short conversational prose and substantially better than repeated timestamp/location/action rows. That is a design judgment, not a passed user/human test. They remain economical and can sound formulaic over many sparse turns. Exact dialogue supplies most of the character voice, and an inaccurate accepted line stays inaccurate. A phrase registry becomes a product improvement only when the composition handles real multi-speaker scenes and sustained turns with natural attribution and no conspicuous repeated wrappers. Merely increasing the registry from four to forty items does not establish that outcome.

Before implementation acceptance, audition at least six consecutive turns from the fixed authored Jia Mu scenario using only the deterministic selector. Include two sparse turns, one silence+gesture, an arrival, refusal and anonymous bystander. Judge cumulative repetition, attribution clarity, fitting public register, sentence/paragraph rhythm and preserved action opportunities—not just one polished example. Compare model-selected and deterministic plans on identical public facts later; the model is useful only if it materially improves those prose judgments within acceptable latency. If both sound mechanical, revise the grammar/selection, not the owner rules or exact dialogue contract. Playable product completion remains the goal; this report does not substitute a schema pass for that experience.

### Implementation and reviewer corrections (2026-10-01)

Implemented the shared v2 compiler in `backend/internal/core/rp_composition.go`, with default plan construction, eligible beat/schema export, strict JSON decoding, exact ordered source coverage, whole-paragraph streaming and a deep-frozen input/plan/hash artifact. The default narrator and optional model selection use the same grammar. A companion speech/expression pair requires the explicit adjacent source link; identical timestamps do not authorize fusion. Fresh model output rejects v1; explicit legacy v1 helpers retain the literal renderer. Primary rendering preserves the complete committed input rather than applying the optional model read budget.

The reviewer corrections bind the actor explicitly again on the second line of subject-first speech/gesture composition, even when the quote names another person. Generic public cues such as 从容 or 长辈 do not imply an ancient setting. Explicit classical actor cues affect only that actor; a global classical plan requires an explicit style instruction, checked in the compiler and advertised schema. Neutral response diction uses 答. Forms follow action and requested density; equivalent lexical alternatives use a stable source key rather than alternating by row index.

Verification: serial focused core tests (`TestRPNatural`, `TestRPCompositionPlan`, `TestRPStyle`, `TestRPNarrative`, `TestDeterministicNarrator`, `TestRPContextSelection`) passed in 0.027s; the complete `internal/narrative` package passed in 1.597s; `go vet -p=1 ./internal/core ./internal/narrative` exited 0. Tests cover named-person quotes with explicit gesture attribution, modern and actor-scoped cues, unsupported model global register, stable selection after unrelated source insertion, exact source/quote/target/refusal preservation, malformed plans, frozen artifacts, cancellation and local HTTP selection/fallback. No real provider was called.

These checks establish compiler and provider boundaries, not literary acceptance. Real scene comparison, persistent artifact source authority, application integration and human reading remain separate evidence owned by the integration run. No commit, publication or deployment was performed.
