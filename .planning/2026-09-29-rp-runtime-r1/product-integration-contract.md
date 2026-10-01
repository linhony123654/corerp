# Product vertical slice — shared implementation contract, 2026-10-01

Base: f236c6998feba739e76bd7b206d757ee31978040. This is a product fidelity increment, not final acceptance. Frozen binaries: /tmp/corerp-product-base-f236c69-20261001. Real Step short baseline passed at /tmp/corerp-r1-short-HFCtdX/short-samples.json (four decisions, one v1 render; human experience unapproved).

## Ownership

- Narration worker: core/rp_style.go and new core composition files/tests; internal/narrative composition/prose/provider tests. Own shared types below. Do not change context-selection, storage or UI.
- Storage worker: narrative input/turn/save/replay/history/session/render storage files/tests, store.go and additive migration 078. Do not change private/relevant dialogue readers, core or narrative provider.
- Character worker: rp_private_decision.go, rp_relevant_dialogue.go and their storage tests only. Parent owns core/rp_decision.go and rp_context_selection.go changes.
- Content/UI worker: authored spec/readme, src Studio preset/client streaming contracts/tests only. No Play redesign or backend changes.
- Parent: context selection, integration runners/real captures/docs/release, cross-module integration.

## Shared APIs (core owns presentation; no core -> narrative import)

Narration worker will add:

- `const RPFactCompositionVersionV2 = "corerp.fact-composition.v2"`.
- `RPNarrativeFact.CompanionEventID string` JSON `companion_event_id,omitempty`: set only on a witnessed expression whose existing committed event causation is the speech parent. Same batch/time alone never proves concurrency or physical before/after.
- `RPNarrativeInput.SourceHead int64` JSON `source_head,omitempty`: committed head of the short read snapshot.
- `RPNarrativeView.Artifact *RPNarrativeArtifact` JSON `-`: internal immutable presentation receipt, not sent to a player or model.
- `RPCompositionPlan`: versioned strict finite JSON grammar; every fact covered exactly once in source order. Worker defines actual shape; compatible compilation API is fixed below.
- `RPNarrativeArtifact {Input RPNarrativeInput; Plan RPCompositionPlan; InputSHA256 string}` with stable JSON tags. Hash via `core.HashJSON(Input)`. Input is only frozen authorized public presentation; no proposal/private data.
- `BuildDefaultRPComposition(in RPNarrativeInput) (RPCompositionPlan, error)`.
- `RenderRPComposition(ctx context.Context, in RPNarrativeInput, plan RPCompositionPlan, emit func(RPNarrativeChunk) error) (RPNarrativeView, error)` produces version, exact ordered IDs/groups, lines, internal Artifact; zero arbitrary model prose.
- `DecodeRPCompositionPlan(raw []byte, in RPNarrativeInput) (RPCompositionPlan, error)` and `RPCompositionSchema(in RPNarrativeInput) map[string]any` plus exported instruction / allowable public choices as needed for the provider. Reject unknown/duplicate/null keys and incompatible variants.

The normal deterministic provider uses the shared natural default compiler. Optional ChatProseProvider selects the same grammar. Keep explicit v1 reader/validator constants and legacy read behavior; do not relabel v1 or old saved prose as v2. Standard rendering must not fail merely because a read-time presentation budget is small. Optional provider input budgets still apply.

Natural realization must alter sentence/paragraph rhythm, not merely rename four wrappers. Exact accepted speech and refusal remain; gesture synonyms have identical meaning/target. No inferred gender, emotion, speed, causation, concurrence or consent. Compound speech + expression consumes only adjacent proven companions in actual source order. Scene orientation is optional and sourced once; no repeated ISO/place headers every fact. Public cues may choose eligible diction/rhythm, not introduce current feelings. Unsupported free instructions get a truthful concise warning, not a recurring v1 capability wall.

## Persistence and primary Play

Storage uses new additive 078 artifact JSON columns on canonical turns and variants, alongside independent ordered fact IDs/groups from 077. `markRPTurnNarrativeReady` saves the full view/receipt/source-head atomically. Default ordinary reads return saved canonical v2 even when historical mode is base; no new provider call. An explicitly authored world `FullProse=true` retains its opt-in first model refinement with a configured full_prose provider, once, over the validated frozen canonical public input; save its complete official v2 receipt and cache subsequent reads/reconnects. This preserves the original32 real-Narrator assertions and the user's explicit model choice rather than deleting them to get tests green. The product preset/frozen product Golden has FullProse absent/false, so uses natural primary without this extra model roundtrip. Explicit StyleOverride remains separate variant regeneration. Reload re-expands frozen plan, compares exact lines/groups/ordered independent IDs, validates public authority and expression causal linkage; identity changes cannot reinterpret old saved output. Retain private/visibility/owner/ledger/recovery invariants, no long DB transaction over provider/network/emit.

## Scoped character continuity

Parent adds `RPDecisionExchange.PeerContext bool` JSON `peer_context,omitempty`. Character worker selects latest two complete authorized actor/current-peer exchanges nonlexically; predicate before bounded candidate limit, total candidates <=512, exchange count<=4, runes<=6000. Prefer recent repair then peer retention then lexical. Waiting has no query text but can recall real exchanges. Parent gives peer groups priority1 in core selector; recent repair0. Actor-private latest current-peer record selected before global3 limit, union/dedup max4 in source order, same immutable approval/hash/head checks. It is dated historic evidence, not a continuing goal engine. Never transplant another peer's relationship stance as current.

## Evidence and completion

Authored full world source 31b6ce345006c380e34cf1d4e8384283e45c5aaa8f34e51b9d06dfce8dc44cf7 is explicit product content under delegated authority. All ten NPCs preflight READY. Preserve original four frozen files; compare f236 binaries vs new binaries on this same authored source, inputs and Step configuration. Original missing-data Golden remains historical missing-data evidence.

Require meaningful focused checks, primary fresh/replay/restart/source-corruption tests, comparable real transcript and qualitative inspection, original32+postchecks, full relevant integration and working preview. User experience approval is not invented by a machine score or another agent. Do not mark finished on templates, schemas, publication or green unit tests alone.
