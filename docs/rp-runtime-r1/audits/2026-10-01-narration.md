# Public narration audit — 2026-10-01

The public projection has explicit observation and identity boundaries, but full prose is still a free-text rewrite checked by bounded heuristics: exact utterance bytes are protected more strongly than speaker attribution, event coverage, or factual meaning. Improve composition structurally before expanding prose freedom.

## Scope and evidence

Audited repository HEAD `739a3ad94f34dd1d7a0a4cf1ec450ea02514415e` (requested base `739a3ad`). Read workspace/project AGENTS.md. Static source and existing test inspection only: no suite rerun, provider calls, installs, production action, code/plan edits, or Git history changes. Existing unrelated dirty planning files and screenshots were preserved. This document is the sole audit output. Examples below are code-derived counterexamples, not executed reproductions or observed live failures. Existing tests cited below were read, not newly verified.

## Top three accuracy gaps and minimal repairs

These are demonstrated missing or inconsistent enforcement in source, not demonstrated live failures. This audit did not inspect the running process/provider configuration or reproduce production output, so it cannot attribute two days of runtime problems to deployment or quantify incidence.

1. **Speaker accuracy is only partially implemented (N1).** Definition: facts carry ActorID and exact Text (`core/rp_style.go:146-151`). Builder: accepted/heard utterances retain their speaker (`storage/rp_turn_view.go:33-36,55-68`). Actual provider view: actor display name plus speech token (`narrative/prose.go:249-251`), but the returned token expands to a bare quote (`423-424`). Validation: verifies text membership, not adjacent speaker (`547-587`). Commit/recovery: all input EventIDs are returned and accepted as render provenance (`199-204`; `storage/rp_narrative_render.go:33-38`), even if framing swaps the speakers. **Minimal repair:** server-own the speaker framing attached to each speech reference and require speech-event multiplicity; no private context or new world lifecycle is required. Legacy prose should keep its existing, explicitly weaker guarantees.
2. **Canonical speech conflicts with presentation filters (N4).** Definition/builder preserve accepted speech. Provider rule requires verbatim token expansion (`prose.go:72`), yet validation runs audit-language and forbidden-pattern checks over the expanded utterance too (`535-536,557-560`). Accepted speech containing those terms cannot satisfy both constraints. The retry therefore cannot repair the conflict while preserving canon; deterministic fallback intentionally preserves it with a warning (`core/rp_style.go:420-425`). **Minimal repair:** scope presentation-only filters to framing outside protected speech and use preserve-and-warn for style/canon conflicts. This is an implementation inconsistency, not missing author data or deployment.
3. **Fact accuracy is not as strong as the returned evidence suggests (N2/N3).** Definition/builder supply typed observed actions and targets, but the actual provider receives a free-text action label (`prose.go:249-251`). Validation covers finite phrase forms and mandates speech only (`agency.go:16-33,51-103`; `prose.go:547-555`; `fact_guard.go:91-124`). Commit verifies source-array equality rather than action coverage or semantic attribution (`rp_narrative_render.go:33-38`). Thus omitting an action or adding an unrecognized NPC mental/action phrase can remain consistent with declared source IDs. **Minimal repair:** state the existing guarantee honestly as bounded phrase rejection; require explicit per-fact coverage/action references for supported beats, starting with speech/expressions/object interactions already implemented. Do not add persistent goals or expand kernel authority to fix renderer enforcement.

### Causes that this audit can and cannot establish

- **Implementation bugs/partial contracts:** N1 speaker/multiplicity binding, N4 canon/filter conflict, N2 bounded agency enforcement, N3 missing action-coverage policy/check. These are localized renderer/validator contracts; source evidence does not establish that they caused a particular user-visible incident.
- **No-op/partial style feature:** authored public presentations are not consumed by the deterministic renderer (`core/rp_style.go:265-437`), while the full-prose request explicitly carries them (`prose.go:254-280`). The style-planner path applies a profile then calls that same deterministic renderer (`core/rp_narrative_plan.go:57-68`). Character-specific cues therefore have no effect in those paths; this is a capability limitation, not evidence that the cue was missing. Minimal repair is an explicit capability warning or a few approved cue-aware framing options, not unrestricted persona inference.
- **Missing author data:** legacy/non-Studio worlds genuinely have no public declaration (`rp_narrative_presentation.go:24-26`); unknown NPC identities intentionally suppress cues (`rp_turn_view.go:398-404`). Activity labels fall back to raw codes when no authored label exists (`prose.go:111-118`). Verify these inputs before diagnosing the provider. Missing labels/cues should not be invented from model prior.
- **Runtime/provider deployment:** `storage/rp_style.go:404-411` explicitly diagnoses FullProse requested with a non-prose provider. The public Store streaming entry defaults to deterministic (`317-318`), while provider-aware callers can select another path. No running configuration was audited; source availability is not proof of deployment, and an emitted mismatch receipt would be stronger evidence than assuming the model was used.
- **Genuinely missing lifecycle/vocabulary:** gaze, elapsed duration and additional gestures do not have a narrator-licensed fact path in the inspected vocabulary. Existing wait must not become elapsed time. These are separate domain additions only if product requirements demand them; renderer fixes must not fake them.
- **Recovery:** deterministic fallback, immutable world facts and preservation of the previously selected variant are implemented in source (`prose.go:179-197`; `rp_narrative_render.go:30-31`). Their live execution was not reverified. A fallback is evidence of a renderer rejection/unavailability, not evidence that the kernel failed to commit an action.

The structural recommendation below is an optional way to enforce existing renderer contracts incrementally. It is not a conclusion that additional architecture, persistent goals or broader kernel work is required. Fix and measure the local accuracy gaps first.

## Authority separation that must remain

| Layer | Current source and boundary | Permitted use |
| --- | --- | --- |
| Authored public expression style | `storage/rp_narrative_presentation.go:19-24,45-57`: only StudioWorldPrepared declaration, not persona_text. `storage/rp_turn_view.go:394-410`: attach only to publicly identified NPCs actually represented in facts. `core/rp_style.go:193-205`: cue requires a matching actor fact and source ID. | Choice of composition/phrasing around facts; never new behavior or private thoughts. |
| Private intent/persona/memory | No private decision fields exist in `core/rp_style.go:146-184` narrator DTO. `storage/rp_turn_view.go:128-142` obtains expressions from separate witnessed events. `storage/rp_private_decision_test.go:175-186` checks private memory absence in public consumers. | Decision input only; do not recover it from model priors, reverse relationships, speech semantics, or public style. |
| Utterance claims | `storage/rp_turn_view.go:33-36,62-66,251-255` obtains accepted speech and heard speech. `narrative/agency.go:35-38` and `fact_guard.go:31-32` explicitly separate quoted claims from narration. | Render exactly as attributed speech. “I paid” or “you agreed” is not proof of payment/consent. |
| Observable committed facts | Visual effects use historical perception (`storage/rp_turn_view.go:188-207,328-340`); expressions use frozen observations and targets (`128-160`); unknown actors/targets are anonymized (`367-404`). | Narrate only the committed action, actor, target, state and place licensed by its source. Domain owner and ledger remain authoritative. |

Public style source IDs are recorded separately from the exact fact-by-fact view attribution (`storage/rp_narrative_render.go:33-58`), although storage currently flattens them into one source array. A style declaration must never be counted as an observable scene beat.

## Findings

### N1 — High: speech identity and multiplicity are not structurally bound

**Trigger.** Two accepted utterances, then a draft associates each token with the other actor. For player text “甲” and NPC text “乙”, a draft shaped as `NPC说：[[corerp-speech:0]]。你说：[[corerp-speech:1]]。` expands both exact strings but reverses who said them. Both framing verbs can survive the current guards after quotes are removed. Another path is legacy direct prose containing the same accepted utterance once when multiple events carry identical text.

**Evidence.** `narrative/prose.go:419-459` expands a token into text plus quotes only, not server-owned speaker framing. `prose.go:547-555` creates a set keyed by text and checks substring presence; `583-587` checks quote membership. It checks neither speaker nor count/order of identical utterances. `410-418` accepts valid legacy prose without enforcing one-token-per-event. `agency.go:91-99` allows a bare controlled actor speech verb once quoted spans are removed. `storage/rp_narrative_render.go:33-38` verifies declared source IDs, not output semantic attribution.

**Impact.** Presentation can assign promises, consent claims, accusations or information to the wrong speaker without changing any ledger. An array listing all correct source IDs does not repair that public misrepresentation.

**Evidence versus inference.** Missing binding/count checks are verified in code. Example acceptance is inferred by walking those checks; no live/provider reproduction was performed.

**Smallest design.** Introduce a server-rendered `speech(event_ref)` node including speaker/POV and exact text. Model selects grouping and safe framing templates, never supplies the adjacent speaker label. Validate every accepted speech event exactly once, including repeated text; maintain committed order unless an explicit presentation reorder policy is approved.

**Compatibility/privacy/recovery.** Keep persisted legacy prose readable; do not retroactively relabel it. New renderer version distinguishes protected structure from legacy free text. References must be opaque per-input IDs, not private actor IDs. Any structural violation returns the deterministic rendering and existing sanitized fallback receipt; do not rerun decisions.

### N2 — High: lexical agency guards leave semantic gaps and reject some sourced actions

**Trigger.** NPC narration such as `她轻轻抱住你。` with no embrace event, or private mental language such as `她心里盘算着下一步。`, falls outside NPC clause-prefix/action matches. Controlled narration prefixed by an unrecognized lead-in or exempted modality similarly evades a closed prefix list. Conversely a genuinely committed object interaction rendered as `你把灯打开了。` is rejected by the unconditional 把/将 rule.

**Evidence.** `narrative/agency.go:16-33,51-103` uses finite prefixes/actions, at most two modifiers, and broad hypothetical exemptions. NPC check `narrative/fact_guard.go:91-124` does not strip the player modifier list and uses the same finite action prefixes. Object interaction is a real supported fact in `core/rp_style.go:361-366`, but player 把/将 clauses are rejected at `agency.go:101-102`. The prompt forbids all private mental activity (`prose.go:71,75`), while runtime checks do not prove that universal constraint. The guard itself accurately calls this a bounded safety net (`fact_guard.go:21-23`).

**Impact.** Unsafe narrative claims can pass; safe stylistic variants can trigger repair/fallback. Repeated regex additions increase coverage but cannot make unrestricted prose a proof of authorization or private-state absence.

**Evidence versus inference.** The lexical implementation and unsupported forms are verified; example pass/fail outcomes are source-derived, not executed. No assertion is made that a live model produced these forms.

**Smallest design.** Server-owned action nodes keyed to a specific fact with actor, target, verb family and object/state bindings. Offer multiple audited sentence templates per typed action. Model controls grouping, rhythm and template choice within that vocabulary. Keep lexical guards as defense in depth for any residual literal connective text.

**Compatibility/privacy/recovery.** Do not add actions merely to satisfy a desired prose sentence. New action vocabulary needs a typed command, owner validation, committed event and observation projection first; financial changes still need ledger authority. Reject unknown node kinds rather than importing canon from model prior. Existing fallback must retain every fact.

### N3 — Medium: source attribution does not enforce non-speech coverage or sentence-level provenance

**Trigger.** A draft retains all quotes but drops an activity, departure, expression or object-state fact; alternatively it repeats one witnessed gesture multiple times in different paragraphs.

**Evidence.** `prose.go:547-555` mandates speech presence only. Expression checks find whether a matching expression exists, not how many times it is depicted (`fact_guard.go:34-56`); no mandatory coverage inventory is consumed by validation. `prose.go:199-204` returns all input fact IDs regardless of rendered coverage. Each chunk receives the full unique source set (`162-174,211-224`). `rp_narrative_render.go:33-38` compares IDs only.

**Impact.** Complete-looking receipts can support incomplete or repeated narration. Full-set provenance is useful for synthetic paragraphs but cannot identify which sentence is justified by which event.

**Evidence versus inference.** These omissions are verified statically. Whether omission of a particular non-speech fact is unacceptable is a product-policy decision; the current code does not declare a coverage policy as clearly as it declares exact speech.

**Smallest design.** A closed render plan references each selected fact explicitly; require all turn facts once or represent intentional omission in a typed, auditable coverage policy. Derive paragraph EventIDs from referenced nodes. Persist both the whole bounded input evidence set and the actual node coverage, with separate authored-style provenance.

**Compatibility/privacy/recovery.** Keep existing transport fields valid; add coverage/schema version without pretending old renders contain precise sentence provenance. Never convert a claim quoted in speech into an action node. Validation failure leaves selected revision intact (`rp_narrative_render.go:30-31`).

### N4 — Medium: prose style/audit filters can reject exact, otherwise valid speech

**Trigger.** Accepted dialogue includes “根据事实” or an English word matching `facts?`, or a world/user forbidden pattern. It is inserted verbatim, then rejected as prose meta-language/forbidden content.

**Evidence.** `prose.go:84,535-536` applies the meta-language regex to the entire expanded text, before quote-aware processing. `557-560` likewise checks forbidden patterns over accepted quotes. Deterministic rendering deliberately preserves literal facts while warning on a conflicting forbidden pattern (`core/rp_style.go:412-425`). Existing test `narrative/prose_test.go:229` covers rejection of audit language, but the implementation does not distinguish quoted speech from invented narration at that stage.

**Impact.** Canon-preserving drafts unnecessarily repair and fall back; model feedback can request removal of text that the exact speech contract requires. World facts remain intact through fallback, so this is availability/expressiveness friction, not permission to alter dialogue.

**Evidence versus inference.** Whole-text scope and incompatible policies are verified. Actual provider retry frequency is unknown.

**Smallest design.** Parse protected speech nodes first. Apply presentation-only bans/meta-language checks to model-authored framing. Explicitly define forbidden-pattern conflicts with accepted canon as preserve-and-warn, consistent with the deterministic path.

**Compatibility/privacy/recovery.** Distinguish style bans from any separately required content policy; do not silently weaken a true external policy. Warn with sanitized category, never log speech or private provider drafts. Existing text should not be rewritten during read migration.

### N5 — Medium: authored style disables sparse evidence limits even though it supplies no scene fact

**Trigger.** The same two short speech facts become nonsparse solely because an identified NPC has a public presentation cue. A third speech/wait fact also disables the sparse cap.

**Evidence.** `prose.go:269-273` forces concise output for sparse evidence; `538-544` caps it at `64 + 2 * speechRunes`. `601-612` returns nonsparse for any PublicPresentations or more than two facts. Public cue rules at `prose.go:74` explicitly prohibit turning style into action or psychology. Nonsparse output can use the general 6000-rune ceiling (`32-33,525-527`). Existing source test `prose_test.go:88` exercises sourced style; `127` exercises the two-utterance cap.

**Impact.** A declaration intended only for expression rhythm broadens output freedom abruptly, while sparse long/standard controls collapse to concise. This conflates available presentation guidance with available scene evidence and increases reliance on N2's heuristics.

**Evidence versus inference.** Threshold behavior is verified. Increased invention risk and perceived style discontinuity are inferred; no quality benchmark or failure-rate claim is available.

**Smallest design.** Budget framing by observable action/speech richness, separately from authored style. Public cues can select approved template vocabulary and rhythm without supplying extra beats. Make long output mean composition across available facts rather than a minimum length. Communicate capability limits when controls cannot create additional supported content.

**Compatibility/privacy/recovery.** Preserve user style values in receipts; record resolved effective density separately instead of silently implying that long was delivered. Never fetch private persona to enrich a short scene. Maintain exact speech and deterministic recovery even when expansion is unsupported.

### N6 — Medium: display-name-only model payload loses identity distinctions

**Trigger.** Multiple unknown characters are all projected as “陌生人”, or two known people share a display name; the model composes speech/actions using names alone. Movement by an anonymous character or unambiguous NPC pronoun is conservatively rejected even when a move exists.

**Evidence.** Public projection masks identities (`storage/rp_turn_view.go:382-404`) using anonymous IDs, but `narrative/prose.go:86-102,249-256` sends actor/target display names without reference identity. NPC movement validator refuses pronouns and “陌生人” (`fact_guard.go:303-316`). Expression subject resolution allows a single NPC pronoun but clears ambiguity (`fact_guard.go:128-157` and following resolution logic). Tests `prose_test.go:480` cover conservative expression subject handling.

**Impact.** Safe identity masking remains intact, but the renderer loses distinctions already safely available in the public DTO, producing ambiguity or avoidable fallback. A richer model prior must not be used to guess which anonymous actor is which.

**Evidence versus inference.** Payload loss and conservative movement behavior are verified; output confusion is a possible effect, not an observed production issue.

**Smallest design.** Per-input opaque actor/fact reference keys with server-owned public labels, preserved through structural nodes. Resolve pronouns only from explicit unambiguous references; otherwise repeat a public label or use distinct privacy-safe handles consistent with existing observation policy.

**Compatibility/privacy/recovery.** Do not transmit real IDs behind anonymous projections or give anonymous handles cross-world linkability. Keep frozen identity policy and legacy “陌生人” output readable. An ambiguous structural reference must fail closed and recover deterministically.

## Expressive limits that are intentional, and where to improve

Current deterministic output emits one literal line per fact (`core/rp_style.go:273-431`). Long mode adds per-fact place/time framing (`383-401`), which can be repetitive even with correct facts. The style planner only varies a closed profile (`core/rp_narrative_plan.go:5-26,51-73`); it cannot select paragraph grouping, speaker transition templates or fact-level emphasis. Full prose permits grouping but pays for it with the gaps above.

The observable gesture vocabulary is smile, nod, shake_head, turn_away, frown and beckon (`prose.go:109-110`; `core/rp_style.go:307`). Gaze and elapsed-time narration currently lack a typed licensed path and are rejected (`fact_guard.go:25-29,65-69`). Speech mentioning a gesture is still merely speech. Wait is a stance, not a clock advance (`observable_boundary_test.go:141-153`). This scarcity should be explained honestly; it should not be “fixed” by narrator invention.

The smallest useful structural renderer extension is a bounded, versioned presentation plan: paragraph groups containing `speech(event_ref)`, `action(event_ref, template_ref)` and narrowly allowed connective/template choices. The server expands all actor labels, quotes, actions, targets, object state and sourced place/time. Public authored style chooses among compatible templates; it never supplies evidence. Validate reference scope, exact speech multiplicity, coverage, target and action compatibility before emitting any paragraph. This delivers observable differences between concise/standard/long through grouping and rhythm while preserving canon over model prior.

Do not add a private-intent channel to the public narrator. If additional expressiveness requires genuine observable beats, extend the appropriate typed owner/command/event/observation path separately, including witness and privacy rules. Do not infer affect, intimacy, agreement, payment, name, relationship, gaze or duration from utterance claims.

## Compatibility and recovery acceptance for a future change

1. Same settled facts and authored sources across renderer variants; rendering never writes world events, moves branch head, replays decisions or affects ledgers. Existing versioned render/selection authorization remains (`rp_narrative_render.go:26-28,77-99,108-149`).
2. Preserve literal nested quotes, tokens and punctuation inside speech. Current one-pass reference expansion is valuable (`prose.go:406-459`); retain equivalent protection inside structural nodes.
3. Read legacy saved prose without silently upgrading its semantic assurances. Persist renderer/schema version and distinct fact/style sources for new artifacts.
4. Validate complete output before stream emission, preserving current full-prose behavior (`prose.go:175-199`). Keep sanitized failure receipts, selected-version preservation and canonical restore. A persistence failure after successful emission remains a material read-path failure (`storage/rp_style.go:421-425`), not a committed replacement.
5. Focused future checks should cover swapped speaker references, repeated identical speech, literal token data, duplicate/dropped action references, quoted audit words, private mental framing, same-name/anonymous actors, wrong expression recipient, and sourced object templates. These are proposed checks; none were added or run in this audit.

Finite result: six scoped findings and a structural composition recommendation. No code changes or release claim.

## Authorized accuracy implementation increment (2026-10-01)

A later parent instruction authorized implementation, superseding the audit-only scope for this increment. Ownership stayed within `backend/internal/narrative` plus this assigned report; parent owns the added core view fields and storage integration. No existing core/storage/transport/type file was edited by this worker.

**Local reproduction — PASS.** Added `narrative/composition_test.go:17` and ran it against the unchanged legacy validator before integration. The validator accepted a speaker-swapped draft with both exact utterances, and accepted one quote for two distinct events with identical speech. The test deliberately retains the historical weakness as evidence; fresh-plan tests reject those drafts. This is now locally reproduced, not just the earlier static inference. It is still not a live incident attribution.

**Implemented contract.** `narrative/composition.go:14-67` exports `CompositionVersion = "corerp.fact-composition.v1"` and supplies a finite JSON schema in the actual provider input. Root keys are `version` and `groups`; each group has `layout` (`inline`/`lines`) and `atoms`; each atom has `fact_ref` and `template` (`plain`, or speech-only `dialogue`). References are per-input `f0...fN`, not actor/event IDs. Local parsing rejects duplicate/null/unknown fields, extra framing, unknown or incompatible templates, missing/repeated/out-of-order facts and trailing JSON (`composition.go:71-159`). All supplied facts must occur exactly once, in original order, even when distinct events have identical text.

Server expansion reuses the existing deterministic provider for action/speech atoms (`composition.go:166-215`); the optional dialogue template server-owns its speaker/POV label and exact quote. Model output cannot provide or change text, actors, action, target, object or private intent. Supported observed gestures, object state, movement and activity remain the same typed vocabulary. Nested quotes, literal old token strings, embedded newlines and spaces are not reparsed as instructions or split into new paragraphs. Style conflicts preserve accepted speech and retain deterministic warnings; no arbitrary model-written framing exists, so audit/meta language cannot enter except as existing canonical source data. Public authored cues remain optional input influencing only allowed layout/template choices.

`narrative/prose.go:160-217` preserves read-budget validation, rejects missing/duplicate source IDs before HTTP, then emits fully validated composition groups. View `EventIDs` retain exact fact order; `CompositionVersion` and `FactGroups` are populated with complete line-indexed fact coverage, and chunks carry only their actual group's EventIDs. Public style sources are not fact-group members. No renderer output changes world facts or ledgers. Provider metadata Kind/NarrativeMode remain `full_prose` for existing selection. Timeout, attempt count, response bytes, output rune cap, cancellation and deterministic fallback stay in the existing provider path.

**Fresh versus saved legacy.** Fresh raw prose or quote-only token responses can no longer produce a successful versioned render. They may receive one bounded repair request and then fall back to deterministic output. Legacy heuristics are used only to preserve sanitized diagnostic categories for rejected old-format replies, never to accept them (`composition.go:124-136`). Saved legacy reads are owned by storage and remain outside this parser, with no version claim. New storage persistence and legacy reload compatibility are parent-owned and NOT VERIFIED by this worker.

**Verification — PASS (local fixtures only).** Executed from `backend` using `/usr/local/go/bin/go`, `TMPDIR=/dev/shm`, `GOTMPDIR=/dev/shm`, `GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930`, `GOPROXY=off`:

```text
go test ./internal/narrative -run '^TestLegacyProseAttributionGapReproduction$' -count=1
go test ./internal/narrative -run '^(TestComposition|TestLegacyProseAttributionGapReproduction|TestValidateProse|TestProseExpressionRecipient|TestProseMovement|TestProseNPCWait|TestProseSpeechReferences|TestProseQuotes|TestProseValidationCategory|TestChatProseProviderRejects|TestSparseSpeechAndSilence)' -count=1
```

The final focused command returned `ok corerp.local/backend/internal/narrative 1.132s`. It covers new schema/coverage, attribution, source rejection before HTTP, same-text events, canon/filter conflicts, protected whitespace/tokens, supported observable vocabulary and targets, budget/cancellation, local mock repair/fallback and existing validator/safety cases. `git diff --check -- backend/internal/narrative` passed. Tests using HTTP run only local httptest servers; no real provider call, quota use, install, deployment, commit or full suite occurred.

**Known incompatibility — FAIL, retained.** A separate targeted command including unchanged `TestChatProseProviderRendersSourcedParagraphs` and `TestChatProseProviderInsertsCommittedSpeechByReference` failed: those positive fixtures demand successful new renders from legacy raw prose/quote-only tokens, which the new contract explicitly prohibits. Assertions/fixtures were not weakened or deleted. Other old positive/repair fixtures expecting raw prose were not all rerun and can have the same incompatibility. Parent must migrate successful-provider fixture wire responses to versioned plans while retaining their original speech/action/privacy assertions, or explicitly retire the old fresh-provider protocol tests. There is no claim that the existing entire narrative suite passes. Existing negative safety tests initially exposed changed failure categories; rejected-legacy diagnostic preservation repaired that mismatch, and the final focused safety command passed.

**Remaining integration risks/limits.** Storage must classify the three fixed composition errors as validation failures (old sanitizer would otherwise call them unavailable); this was reported to parent. Parent-owned persistence/reload must validate version and group coverage and leave saved legacy version empty. This worker does not claim those checks passed. ContextBudgetBytes still bounds the existing public input DTO; the finite schema/prompt add wire overhead, as instructions did previously, and no provider token budget guarantee is inferred. Two templates improve accuracy and layout, not proven literary quality. Character-specific voice, broader safe composition vocabulary, real-model schema adherence, original full32 and human Golden remain unverified. This increment does not complete R1 or authorize provider/deployment checks.

Finite handoff: `prose.go` updated; `composition.go` and `composition_test.go` added; reproduction and scoped contract/safety checks passed; legacy successful-provider fixture incompatibility and parent-owned storage checks remain explicit.

## Final narrative fixture migration and package validation

Parent subsequently authorized migration of incompatible fresh-provider success fixtures and a full check of this small package. This supersedes the earlier known-FAIL fixture handoff; it does not erase its historical evidence.

Migrated fresh success/repair fixture replies in `narrative/prose_test.go`, `narrative/observable_boundary_test.go` and `narrative/reasoning_test.go` to versioned fact-ref JSON. Retained exact speech, source order, public-style attribution and independent reasoning/credential assertions. Streaming assertions now verify actual line-level groups rather than incorrectly attributing every input event to every line. Strengthened successful repair assertions to require the observed actor/recipient, retained silence, complete fact count and no wrong-recipient action. The activity-label fixture now checks that the authored label actually renders successfully, not merely that it reaches the request. Dropped-speech repair now drops a required speech fact reference in the first draft and verifies it is restored after explicit rejected-draft feedback.

All standalone legacy prose/token/quotation/agency/scene guards and existing negative safety cases remain. `TestCompositionProviderRejectsFreshLegacyWithoutEmission` explicitly covers both fresh raw prose and quote-only tokens: neither is accepted; only deterministic fallback lines are emitted, canonical speech is retained, and fallback leaves composition metadata empty. Legacy persisted prose is still not reinterpreted or relabeled as versioned composition by this worker.

**Final checks — PASS.** From `backend`, with the same /dev/shm TMPDIR/GOTMPDIR/GOCACHE and `GOPROXY=off`, executed `/usr/local/go/bin/go test ./internal/narrative -count=1` → `ok corerp.local/backend/internal/narrative 1.589s`; `/usr/local/go/bin/go vet ./internal/narrative` → exit 0; `git diff --check -- backend/internal/narrative` → exit 0. All package tests now pass; no known failing narrative fixture remains. Only local httptest fixtures contacted HTTP. No real provider/quota call, deployment, full32 or commit occurred.

Changed files owned by this worker: `backend/internal/narrative/prose.go`, new `composition.go`, new `composition_test.go`, `prose_test.go`, `observable_boundary_test.go`, `reasoning_test.go`, and this audit. Existing core/storage/transport/types changes remain other workers' ownership.

Remaining limits are unchanged: plain/dialogue templates provide accurate composition and bounded layout, not demonstrated literary improvement; private persona never becomes narrator context; fresh old-protocol replies must repair or fall back, while saved legacy remains readable without stronger-guarantee claims. Parent-owned storage/version persistence, integration, independent review, real-model adherence, original full32 and human Golden are outside this package verification and remain necessary R1 evidence.

## Independent-review repairs: refusal, framing and capability disclosure

Parent authorized fixes for integration audit P1/P2. These supersede the earlier two-template/forced-compact implementation description.

- **Refusal meaning preserved:** dialogue labels now server-own an explicit “拒绝了” marker for `refuse`, and “回应” for `respond`. Exact accepted words remain untouched. `TestCompositionRefusalKeepsActionWithNonObviousSpeech` uses “茶还温着。” as a refusal and checks all four allowed templates retain both the action marker and exact quote, not just the source ID.
- **Requested deterministic framing restored:** `plain` now inherits the requested source-safe style instead of forcibly resetting density, description, verbosity and narrative pack. Only the instruction string is removed before deterministic expansion, because that renderer does not interpret arbitrary prose. New `compact` explicitly chooses concise framing; `contextual` selects standard or preserves requested long framing; POV, tense, verbosity, pack, actor/action/target and exact speech remain server-owned. `dialogue` retains its compact label form and action marker. The actual provider's schema and per-fact allowed choices now expose all applicable templates.
- **Distinct source-rich density verified:** `TestCompositionPlainRetainsRequestedRichSourceFraming` uses accepted speech plus a targeted witnessed gesture with sourced place/timestamps. For concise/standard/long, the same plain plan must match the existing deterministic renderer exactly, retain speech/target/coverage, and produce distinct output. It checks compact omits optional place context while retaining tense, and contextual retains requested sourced place/time. This proves bounded presentation behavior, not literary quality or arbitrary style execution.
- **Capability reduction is explicit:** every successful composition view now carries a fixed warning naming `corerp.fact-composition.v1`, supported framing/grouping choices and unsupported free rewrite, character wording and psychology. The actual provider payload includes the same capability limit. Public style/custom instructions can influence only supported choices. `TestCompositionReportsLexicalAndPublicStyleCapability` checks an explicit unsupported lexical/private-thought request and public character cue receive the warning without rewriting speech or promoting the style declaration into fact coverage. The dropped-speech repair test now requires precisely this capability warning and no fallback, rather than requiring zero warnings. Kind/NarrativeMode remain `full_prose` solely for existing selection compatibility; version/capability disclosure states the actual contract.

**Verification — PASS:** final `/usr/local/go/bin/go test ./internal/narrative -count=1` returned `ok corerp.local/backend/internal/narrative 1.607s`, using the specified /dev/shm TMPDIR/GOTMPDIR/GOCACHE and offline module resolution. `/usr/local/go/bin/go vet ./internal/narrative` exited 0. Diff check passed. Legacy guards, raw-prose/token rejection and all migrated positive fixtures still pass. Tests made no real provider calls. No deployment/full32/commit occurred.

Parent integration should retain or reconstruct the fixed capability disclosure when reading versioned saved artifacts; legacy rows must not be silently stamped with the stronger version. Storage owns that reload behavior. The bounded vocabulary still cannot execute arbitrary custom lexical instructions, distinctive character voice or novel private/observable facts. Restored sourced framing and four templates address the identified local regressions; they do not establish unrestricted full-prose experience acceptance or R1 completion.
