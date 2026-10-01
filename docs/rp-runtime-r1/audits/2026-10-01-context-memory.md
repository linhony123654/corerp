# Context and memory audit — 2026-10-01

Base inspected: `739a3ad94f34dd1d7a0a4cf1ec450ea02514415e`. Static audit only; no provider calls, installations, application changes, commits, deployment, or test-suite execution. Existing dirty planning files and screenshots were left alone. Project and workspace AGENTS.md were read. All line references below refer to this base. Findings describe code evidence; predicted model behavior is explicitly inference.

## Conclusion

The decision boundary already distinguishes authored canon, accepted/heard words, private owner sketches, and current visible activity. The main continuity weakness is several independent retrieval windows whose losses are invisible to final selection metadata. The strongest concrete presentation defect is field-name-based identity masking that rewrites excerpts while preserving full utterances. Ordinary knowledge/life projections have weaker source verification than delivered information. These findings do **not** establish a normal-path cross-actor private-state leak or justify replacing the typed decision owner/ledger.

## Existing protections to retain

- `backend/internal/storage/rp_decision.go:27–93`: validates active session/control and committed trigger; requires personal hearing evidence and current shared scene; constructs the snapshot inside `beginImmediate`.
- `rp_decision.go:161–171`: applies `rpCanPerceive` before offering visible entities. `backend/internal/storage/rp_scene_activity_context.go:35–75`: filters invisible actors before its limit, verifies committed activity sources, excludes others' terminal outcomes, and replaces their start time with snapshot time. Current perception therefore does not imply knowledge of an unseen start/end.
- `backend/internal/storage/rp_relevant_dialogue.go:32–44`: retrieves own speech or personally heard speech, at the requested branch/head. Retrieval is lexical, not a model-created summary.
- `backend/internal/storage/rp_private_decision.go:15–49`: scopes private sketches to this actor, committed commands/attempts, complete batches at the head, and matching proposal hashes; excludes this turn. `backend/internal/core/rp_decision.go:90–99` explicitly says the sketch is not proof of goal completion.
- `backend/internal/core/rp_context_selection.go:246–412`: verifies complete speech attribution, rejects conflicting canonical words, excludes private sketches and truncated excerpts from complete-quote resolution. `415–512` keeps references resolvable within the selected packet.
- `backend/internal/storage/rp_information_memory.go:58–124`: verifies recipient, branch, send/delivery ordering, channel, content, observation, knowledge and expected replay observation before exposing delivered information. Sender identity is separately checked (`126–145`).
- `backend/internal/storage/rp_decision.go:464–474,510–513`: checks internal owner before invoking provider, then validates grounding against the actual presentation packet and authoritative packet. `backend/internal/decision/chat.go:176–180` already specifies canon above model prior, speech as claims, private sketches as historical, and omitted history as uncertainty.

## Accuracy triage after user clarification

This is a source diagnosis of the existing implementation, not a recommendation to add architecture because recent kernel work was insufficient. No observed production turn, deployed build identity, database fixture, or live-provider transcript was supplied to this audit. The following top three are concrete source-backed failure mechanisms; their responsibility for the user's particular inaccurate turns remains unproven.

| Priority | Existing contract that fails or is partial | Chain to visible behavior | Minimal repair |
| --- | --- | --- | --- |
| 1 | Verbatim heard memory is changed by presentation masking (F3). | Accepted speech/excerpt → builder preserves source words → actual provider view rewrites `excerpt`/arbitrary strings → quote checker either accepts a sole changed excerpt or rejects a conflicting counterpart → decision can be inaccurate or unavailable before public commit. | Preserve excerpt bodies verbatim and replace only exact typed entity-reference values. This is a local masking fix, not new memory architecture. |
| 2 | Relevant exchange recovery skips missing members whenever one member is recent (F2). | Session/parent-turn defines group → latest-16 builder keeps only tail → retriever excludes entire group → provider never receives missing qualification/question → legality validation cannot reconstruct it → accepted response can contradict the earlier exchange. | Skip retrieval only when the authorized group is already represented; retrieve missing heard/own members within existing bounds, or explicitly mark a partial group. |
| 3 | Retrieval rank does not survive final byte selection (part of F5). | Lexical scorer ranks best older exchange first → retriever sorts results into chronology → selector treats newest array index as preference → scarce actual-view budget drops best match → provider answers from weaker evidence. | Carry the existing retrieval priority into selection while preserving chronological output. No new goal lifecycle or vector database is required. |

F1 explains additional bounded-history loss, but the current scope explicitly promises bounded candidates rather than all history: this is a partial continuity contract, not proof that limits themselves are bugs. F4 and F7 are conditional integrity/concurrency risks without reproduction. F6 is a limitation of sketch retention; it is **not** a demonstrated missing product lifecycle or a reason to start persistent-goal work.

### Guarantees that validation does not enforce

`backend/internal/core/rp_proposal.go:18–32` validates private-text lengths and that cited basis IDs are offered and unique. `65–99` checks action fields and legal activities/destinations. It does not check whether the natural-language response follows the cited facts, whether a source entails an assertion, whether a truncated passage is being overinterpreted, or whether the NPC wrongly denies having heard an omitted line. An empty basis list is allowed. Thus the name `ValidateRPDecisionProposalEvidence` establishes source membership, not semantic truth. `backend/internal/storage/rp_decision_commit.go:51–63,80–103` rebuilds/hash-checks input and rechecks owner/head; those checks protect authority and staleness, but do not strengthen semantic entailment.

The adapter's canon/uncertainty/private-boundary instructions (`decision/chat.go:176–180`) are model instructions. Treating those instructions as a proven accuracy guarantee would be unsupported. Minimal near-term repair is to expose the missing/partial evidence accurately and test actual provider packets; add narrow deterministic checks only for concrete known forbidden behaviors. Do not claim an arbitrary-text validator can universally guarantee truthful dialogue.

### Distinguishing data, deployment, lifecycle and implementation

- **Missing author data:** `rp_decision.go:121–135` exposes missing persona/address and unknown relationship. `decision/chat.go:191–193` refuses invocation on incomplete readiness; `rp_decision.go:489–500` returns silence with `rp_context_not_ready` when no attempt occurred. This can explain an apparent no-op without a broken model connection. No inspected live world establishes that required author data is missing. `UNKNOWN` stranger relationship is intentionally usable, not a readiness failure.
- **Runtime/deployment:** this report inspected base `739a3ad`, not the executing service binary or frontend bundle. It cannot establish that recent fixes are deployed or that a selected provider is configured. Match deployed revision and the failing turn's receipt/head/actual-view hash before attributing inaccurate behavior to kernel design.
- **Existing lifecycle:** typed meeting commitments already exist: `rp_life.go:292–317` records `promise_meeting`, removes on `keep_meeting`, and offers remaining commitments; `core/rp_decision.go:264–281` includes deterministic handling. General private sketches are different from these commitments. This audit does not prove a missing persistent-goal lifecycle.
- **Implementation:** F2, F3 and rank loss in F5 have concrete source triggers independent of missing author fields or deployment. Repair and reproduce these within the existing pipeline first.
- **Public rendering/recovery:** `rp_context_selection_test.go:148–155` asserts private sketches/selection metadata do not appear in public session output, and `rp_decision_commit.go:48–49,83–84` checks committed recovery before writing again. These are useful intended boundaries, not executed test results here. No public-rendering defect or replay failure was demonstrated by this audit; an inaccurate **committed utterance** can remain a real utterance without its underlying claim becoming world truth.

## Detailed findings

### F1 — Retrieval losses are not represented in the omission contract (verified; continuity risk)

**Paths:** `rp_decision.go:174–184,217–241,254–260`; `rp_relevant_dialogue.go:15–17,43–44,137–150`; `rp_private_decision.go:29`; `rp_information_memory.go:26`; `backend/internal/storage/rp_life.go:137–140,324–326`; `rp_context_selection.go:175–183`.

**Trigger:** an offer passes the last 40 interlocutor utterances or last 512 authorized utterances; a relevant response consumes more than the 6000-rune retrieval allowance; a remembered message passes the last eight deliveries; or an intent passes the last three applied private sketches. Paraphrase with no shared lexical terms also yields no relevant exchange (`rp_relevant_dialogue.go:98–128`).

**Evidence:** final omission counters compare the already bounded input arrays to selected arrays. They cannot count records dropped by SQL limits, lexical ranking, exchange limits or excerpt truncation. A packet with zero reported omissions can still lack a decisive historical reply. `Scope=bounded_authorized_candidates` is truthful but does not expose which bounds were hit.

**Impact (inference):** the provider can forget an offer, qualification, refusal, pending intent, or delivered message and contradict earlier behavior. The prompt discourages denial of absent history, but cannot reconstruct missing evidence.

**Smallest design:** add per-source coverage metadata: retrieval window/head, limit reached, ranking policy, group/text completeness and selection exclusions. A limit reached is a conservative signal, not proof that additional rows exist. Add bounded actor-authorized lookup for a referenced exchange or unresolved intent; absence must remain `unknown/not_in_packet`, never `never_happened`.

**Risks:** full-history counts can be expensive; count only authorized scope or use conservative saturation. Coverage metadata stays NPC-private. Do not turn provider-generated summaries into canon or make infinite history mandatory for recovery.

### F2 — A recent member suppresses retrieval of the rest of its exchange (verified; continuity risk)

**Paths:** `rp_relevant_dialogue.go:79–93,113–116`; `rp_decision.go:184`; `backend/internal/core/rp_decision.go:83–88`.

**Trigger:** an old player question and NPC qualification share a session/parent turn, but only the last member remains among the latest 16 utterances. A crowded scene can split the group at the window boundary. A 512-row boundary can likewise cut the earliest members of a retrieved group.

**Evidence:** `g.recent=true` when **any** group member is recent; the whole group is then skipped, regardless of whether all members are in `RecentDialogue`. SQL limits individual utterances before grouping. `RPDecisionExchange` has no explicit group completeness flag.

**Impact (inference):** a qualification or original question disappears while its answer remains; an offered exchange can look complete although its start was outside the candidate window.

**Smallest design:** exclude a group only when its authorized members are fully represented; retrieve missing authorized members by session/parent-turn at the same head. Mark `authorized_members_complete` separately from total exchange completeness: unheard members must never be fetched into the actor's packet.

**Risks:** exchanges must stay bounded and preserve event order; on oversized groups retain explicit partial coverage rather than silently clipping or widening hearing permissions.

### F3 — Masking rewrites excerpts and arbitrary strings but preserves full speech (verified; provenance defect)

**Paths:** `backend/internal/storage/rp_decision_provider_view.go:50–51,70–91,94–115`; `rp_decision.go:238–241`; `rp_context_selection.go:353–356,497–509`.

**Trigger:** personally heard words contain an unfamiliar actor's canonical ID. The body is carried as `excerpt`, especially a truncated excerpt whose complete utterance is unavailable; other provider fields contain the same ID as a substring.

**Evidence:** masking preserves only string keys `text`, `player_speech_text`, and `persona`. It applies `strings.ReplaceAll` to `excerpt` and every other string key, including provenance IDs and private phrases. A truncated excerpt with no complete counterpart bypasses quote-prefix comparison and remains changed. A complete excerpt standing alone becomes a changed complete quote. References or normalization can hide the discrepancy where another body is retained; they do not make the transformation safe generally.

**Impact:** the provider receives words different from the accepted speech. Conditional prefix conflicts can instead fail packet construction when an original full body and rewritten partial excerpt coexist. Substring replacement also risks altering unrelated identifiers rather than only typed entity references. This is not evidence that the claimed ID should grant canonical identity: heard claims should remain verbatim without establishing familiarity.

**Smallest design:** mask typed entity-reference fields using exact equality; transform display names only from an explicit familiarity policy. Keep all witnessed speech/excerpts immutable as data. Keep provenance IDs opaque, and preserve private text as actor-private claims. Re-normalize references and budget after mapping.

**Risks:** legacy consumers may have depended on rewritten strings; introduce a presentation-policy version. Verbatim claimed names/IDs are allowed heard content, not canonical recognition. Do not strip speech or reveal unoffered entity metadata to explain a claim.

### F4 — Ordinary knowledge and life memory lack the delivery reader's source closure (verified asymmetry; conditional integrity risk)

**Paths:** `rp_decision.go:254–289`; `backend/internal/storage/rp_life.go:108–129,137–161`; contrast `rp_information_memory.go:58–124`.

**Trigger:** stale, corrupt or incorrectly rebuilt ordinary observation/knowledge projection; a speech claim whose accepted utterance is outside other retrieval windows.

**Evidence:** ordinary knowledge joins an event and an observation but reads the body from `k.claim_payload`; it does not check all observation owner/subject/payload fields against the event. Life relationship/salient-memory queries are actor-keyed projection reads without an explicit event branch/head join or full source comparison. Quote conflict checks help when an accepted utterance counterpart is present, but do not establish source truth when that counterpart is absent.

**Impact (conditional inference):** an incorrect projection can supply fabricated or wrongly attributed knowledge and provenance to the provider. This audit did not reproduce a corrupt database or show an authorized application write creating such a row. Actor IDs may be globally unique by schema, so absence of a branch clause alone is not proof of cross-branch leakage.

**Smallest design:** reuse source-closure verification for ordinary claims: actor, observation ID, subject, branch/head, channel, event type and canonical payload must agree. Build relationship provenance from event sequence, not lexicographic `MIN/MAX(source_event_id)` (`rp_life.go:108,125–127`). Preserve replayable event authority and fail closed on disagreement.

**Risks:** source validation can add read cost; batch/cache at a snapshot rather than add broad replay per decision. Recovery should rebuild projections from immutable events, never repair evidence by asking the model. This does not warrant adding a parallel memory authority.

### F5 — Two-stage selection can lose candidates before presentation and drops relevance rank (verified; conditional budget risk)

**Paths:** `rp_decision.go:93`; `rp_decision_provider_view.go:60,91`; `rp_context_selection.go:75–82,107–115`; `rp_relevant_dialogue.go:130–155`; `backend/internal/storage/rp_context_selection_test.go:161–194`.

**Trigger:** a near-budget packet whose aliases change encoded byte size; multiple relevant exchanges compete for remaining space.

**Evidence:** selection occurs once on authoritative IDs and again on the already selected masked packet. Dropped first-stage candidates cannot be recovered if the actual presentation creates spare space. Relevant retrieval ranks by score, but then returns exchanges in chronology; byte selection uses `-i`, favoring newest chronology rather than highest retrieval score. The existing test explicitly covers alias growth causing a previously retained candidate to disappear, and validates grounding against the resulting actual view.

**Impact (inference):** a less relevant newer exchange can displace the older answer the retrieval scorer considered strongest; masking can waste available budget. Actual-view validation correctly prevents grounding in dropped material.

**Smallest design:** retain bounded authorized candidates server-side, carry relevance rank as selection metadata, map typed identities, then finalize one provider packet. Emit selected facts in event order regardless of rank. Bind proposal grounding to that exact packet, and still recheck authoritative facts/head before commit.

**Risks:** candidate and packet hashes must remain distinct; never validate against candidates not shown to the provider. Persist policy versions for reproducible retry and retain compatibility with existing hashes rather than silently changing their meaning.

### F6 — Private continuity is a short sketch history, not durable intention state (verified design limit)

**Paths:** `rp_private_decision.go:23–29,49`; `backend/internal/core/rp_decision.go:90–99`; `rp_context_selection.go:78–79`.

**Trigger:** more than three subsequent applied decisions, a budget that drops lower-priority sketches, or a current parent turn with prior own decisions deliberately excluded.

**Evidence:** only three earlier sketches are candidates; there is no pending/resolved intent lifecycle in this reader. The source proves application of the decision, not truth of a thought or completion of a task.

**Impact (inference):** a recurring objective can be forgotten after unrelated turns. Conversely, promoting a sketch into a completed promise would invent world effects.

**Smallest repair:** do not infer a new goal feature from this retention limit. First identify a failing existing behavior and check typed commitments/goals already offered in `Life`. If the failure is an omitted earlier sketch, improve existing bounded retrieval/coverage; keep sketches subjective. Any new general intention lifecycle would require a separately established product requirement and is outside this audit's repair recommendation.

**Risks:** a new mutable memory authority would conflict with ledger/replay semantics. Goal completion requires a typed owner's committed evidence. Never expose the record to other actors, public observation or narration merely because it has an event source.

### F7 — Provider identity reads are not one snapshot with their head check (verified structure; concurrency inference)

**Paths:** `rp_decision_provider_view.go:18–39,63–68`; `rp_decision.go:31–35,471–486`.

**Trigger:** a branch change between provider-view head read and familiarity/alias queries.

**Evidence:** builder uses an immediate transaction; provider view uses a standalone connection, checks head once, then reads familiarity and aliases. No read transaction or final head check appears in that function.

**Impact (inference):** provider packet can combine old decision facts with newer familiarity. Commit revalidation limits world effects, but cannot retract content already sent to an external provider. No concurrent execution was reproduced in this audit.

**Smallest design:** perform head, familiarity and presentation reads in one bounded read snapshot; abort stale views before invocation. Keep the commit-time head/owner checks.

**Risks:** avoid holding a database transaction across a provider call. Snapshot authorization and current owner authorization remain distinct checks; retries must rebuild when the head changes.

## One coherent context contract

Use the existing `RPDecisionInput` boundary with one versioned server-side context pipeline, rather than independent authoritative memory stores:

1. **Snapshot and authority:** identify `(instance, branch, head, world_time, actor, trigger)`; authorize control/typed decision owner. Source all canon and action legality from committed owners/ledger. Canon outranks model prior; `MISSING` and `UNKNOWN` remain explicit.
2. **Evidence classes:** each item carries a typed basis: `authored_canon`, `own_committed_action`, `personally_heard_speech`, `current_perception`, `delivered_claim`, or `own_private_sketch`. Include source event/sequence, actor/subject, learned versus happened time, observation basis where applicable, completeness, and reliability/relay policy for messages. Private sketches are subjective historical state; speech and messages prove receipt/wording, not truth.
3. **Authorization before retrieval/ranking:** construct bounded candidates only from the actor's evidence. Historical hearing persists even after a speaker leaves; current sight cannot authorize unseen starts, terminal outcomes, private thoughts, finances, or message audiences. Close projection evidence against immutable sources before it becomes provider-visible.
4. **Continuity retrieval:** preserve current trigger and a local exchange; fill authorized missing exchange members; retrieve relevant older exchanges and existing typed commitments/owner-state separately. Report coverage/window saturation, lexical policy, group completeness, excerpt truncation and budget exclusions. Missing context implies uncertainty. This does not authorize a new general persistent-goal subsystem.
5. **Presentation then final selection:** transform only typed entity references at that snapshot; preserve observed text verbatim. Carry retrieval score/order separately, select within the actual encoded budget, then output chronology. Store one complete speech body per `(event, speaker)` and only packet-local resolvable references. Do not reference private text as speech.
6. **Version/provenance:** distinguish context schema, retrieval/selection policy, presentation policy, authoritative-input hash and actual-provider-view hash. Current constants are `corerp.rp-context.v1` (`core/rp_decision.go:28`), `corerp.context-selection.v1` (`core/rp_context_selection.go:11`), and the adapter's separate decision envelope (`decision/chat.go`, `decisionContext`). A semantic change must be distinguishable on retry/replay. Proposal basis handles bind only to sources actually offered.
7. **Commit and recovery:** provider proposes only. Validate actual-view grounding, authoritative source/head, typed owner and legal action; commit through the existing command/attempt/event/ledger chain. Rebuild corrupt projections from ledger; re-create a packet at a new head on stale retry; preserve request-key/idempotency semantics. Restricted packet/selection diagnostics must never enter public narration or player observations.

## Compatibility, privacy and focused verification proposal

Adopt source metadata and coverage fields additively where feasible; deliberately version identity mapping and selection semantics. Preserve authored relationships/readiness, existing adapters, proposal schema, command authority and replay hashes. Do not retrofit summaries into historical canon. The contract is a design proposal, not an implementation or a new public API.

Before an implementation is accepted, use narrow fixtures for: split recent exchange; exchange crossing the retrieval boundary; 41st heard offer and paraphrase; full/truncated excerpts containing an unfamiliar ID; projection body/observer/source corruption; same-head deterministic restart; alias budget growth/shrink; preservation of existing typed commitments; concurrent familiarity change; and public-output absence of private sketches/coverage. A fixture should assert exact provenance and unknown/partial semantics, not require a model to infer a fact missing from its packet.

Validation performed here: read-only code and existing test inspection, base/status verification, and review of this report. Existing relevant tests include `rp_decision_dialogue_test.go`, `rp_context_selection_test.go`, `rp_scene_activity_context_test.go`, and `rp_context_readiness_gate_test.go`; their presence is evidence of intended coverage, **not a claim they were executed or passed**. No normal-path private leak, live-provider continuity result, or concurrency reproduction is claimed.

## Authorized implementation increment — provider view

Scope changed by explicit delegation after the static audit: implement only `backend/internal/storage/rp_decision_provider_view.go`, dedicated `rp_decision_provider_view_test.go`, and this evidence appendix. No shared core, plan, author-data, commit, deployment or provider edits/calls were performed by this increment. The parent supplied the new `core.RPDecisionPresentation` type; this worker only consumes it. The original findings above remain a historical base audit, not claims that every finding was repaired.

**Reproduction (FAIL before fix):** the dedicated integration fixture offered an unfamiliar ID inside personally heard words and provenance. On original provider-view code, `TestRPDecisionProviderViewPreservesClaimsAndProvenance` failed with a changed `Excerpt` containing `person_ecf06a6dc5a0960d12f5c99b` and a changed `EventID=excerpt_person_ecf06a6dc5a0960d12f5c99b`. This establishes the F3 text/provenance corruption locally, not merely by static inference. Setup initially needed `/usr/local/go/bin/go` because `go` was absent from shell PATH; an initial test-helper return-arity compilation issue was corrected before the behavioral reproduction.

**Implemented:** explicit entity-reference schema keys use exact alias lookup, including nested speaker/subject/actor/store/friend/counterparty references. All other string values, including heard full/partial text, private subjective phrases, goal conflict phrases, event/decision/message/contract/place IDs and evidence lists, are excluded from projection. Unknown visible people still receive the existing generic display name. Authoritative input is not mutated. The existing selector still normalizes complete speech references and enforces the final actual-view byte budget; this fix does not promise every candidate survives that budget.

Head, internal decision owner, unfamiliar-person lookup and alias-secret reads now share the existing `beginImmediate` transaction. All exits defer rollback; successful paths explicitly release the snapshot before packet finalization and before returning to any provider invocation. It uses the existing short SQLite writer reservation, so concurrent writers can wait during these local reads; no network call occurs under that reservation. Current owner is checked again inside this snapshot; commit-time authorization remains unchanged. A writer after stale and successful view calls is covered by the fixture. Concurrent interleaving was not separately stress-tested; snapshot consistency follows the SQLite transaction used by these reads.

Actual provider views carry `Presentation={policy_version: corerp.rp-identity-view.v2, source_head_sequence: input.HeadSequence, identity_mode: observer_relative}` before final budget selection, including the no-mask path. Canonical inputs retain nil presentation metadata and existing hash semantics. This explicitly versions changed presentation semantics without changing authored facts.

**Focused verification (PASS):** from `backend`, using `TMPDIR=/dev/shm GOTMPDIR=/dev/shm GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930`:

```sh
/usr/local/go/bin/go test ./internal/storage -run '^(TestRPDecisionProviderViewPreservesClaimsAndProvenance|TestRPDecisionProviderProjectionMapsOnlyExactEntityReferences|TestRPContextSelectionProviderMaskRechecksBudgetAndBasis|TestRPContextSelection.*Restart.*)$' -count=1
```

Result: `ok corerp.local/backend/internal/storage 5.601s`. Covers exact immutable full/truncated/current speech and historical resolution, private phrases/provenance, exact typed masking versus substrings, authoritative-input nonmutation, presentation metadata, stale head rejection, externally controlled actor rejection, no-mask identity handling, transaction release, existing alias-budget/actual-view basis checks, and existing restart/pressure integration. `gofmt` was applied to the two owned Go files; `git diff --check` passed. No full suite or live provider was run.

**Remaining limits:** F2/retrieval ranking and other audited modules were outside this worker's implementation scope. New entity-reference schema fields must explicitly join the projection allowlist; unknown string fields are preserved rather than silently rewritten. Canonical IDs inside genuinely heard speech remain received claims and do not establish familiarity. Source membership still does not prove arbitrary dialogue truth. Deployment and user-reported live-turn accuracy remain NOT VERIFIED.

### Independent-review follow-up — reachable schema inventory

Added `TestRPDecisionProviderProjectionCoversReachablePersonSchema` in the dedicated test file. It recursively visits actual reachable `core.RPDecisionInput` Go types (including pointers/slices/maps), derives person-reference semantics from Go field suffixes such as `EntityID`, `ActorID`, `AgentID` and `FriendID`, and checks the actual JSON projection. Explicit non-person suffixes exclude provenance, place/world scope, contracts, institutions and other records. New unclassified ID semantics fail with the type/field path. This is independent of the implementation's JSON-key allowlist and detects new semantic references, renamed tags and accidental mapping of unrelated IDs.

PASS: the focused inventory command with the same `/dev/shm` environment, `go test ./internal/storage -run '^TestRPDecisionProviderProjectionCoversReachablePersonSchema$' -count=1 -v`, reported **48 reachable types, 20 person-reference fields, 94 unrelated ID fields**, `ok ... 0.005s`. Formatting and `git diff --check` passed.

The subsequent combined run of inventory, exact text/provenance/snapshot, exact reference mapping and existing budget/basis tests could **not** execute: Go linking failed with `mapping output file failed: no space left on device` in `/dev/shm`. Mark that combined post-review run NOT VERIFIED; the earlier full focused increment run above remains the last executed behavioral PASS. No existing tests were weakened, no cache/other worker files were deleted, and no production or provider action was performed. Next integration action: rerun that focused combined command after the parent resolves shared `/dev/shm` capacity.

**Resolved integration rerun (PASS):** parent instructed using `TMPDIR=/tmp GOTMPDIR=/tmp` while retaining `GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930`. The combined focused command `go test ./internal/storage -run '^(TestRPDecisionProviderViewPreservesClaimsAndProvenance|TestRPDecisionProviderProjectionMapsOnlyExactEntityReferences|TestRPDecisionProviderProjectionCoversReachablePersonSchema|TestRPContextSelectionProviderMaskRechecksBudgetAndBasis)$' -count=1` completed successfully: `ok corerp.local/backend/internal/storage 2.127s`. This supersedes the capacity-blocked post-review run. Inventory and all owned behavior checks are complete; no pending implementation changes. Worker files are frozen for parent contract-test integration.
