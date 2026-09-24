# RP6 independent narrative execution boundary

Status: replaceable execution prerequisite implemented and verified. **Custom prose execution remains incomplete.** No external narrative adapter/configuration or claim of live model quality is introduced by this increment.

Follow-up: the subsequent [constrained natural-language style executor](custom-style.md) now provides independent HTTP configuration and tested product execution. The limitation on arbitrary literary prose remains explicit; this document describes the earlier provider prerequisite, not current adapter availability.

Previously `RPNarrativeProvider` existed in core, but the real storage read/stream path directly instantiated the deterministic renderer. `RPService` only injected a decision provider. A model narrator could not be substituted in the actual product path.

`RPStreamingNarrativeProvider` now adds the existing per-fact stream contract to `RPNarrativeProvider`. `NewRPServiceWithNarrative` binds an independently supplied immutable presentation provider; normal `NewRPService` and direct Store reads preserve deterministic defaults. Operator construction owns this choice: style requests cannot supply endpoints, credentials or a provider implementation.

JSON and NDJSON service reads share the same storage pipeline:

1. Authenticate caller/session and live control, locate own settled turn.
2. Release the transaction; load pinned style and optional validated read override plus immutable accepted facts.
3. Validate the actual serialized presentation-context budget.
4. Invoke the selected narrative provider with that input, without holding a database connection.

Canonical settlement remains independent and saves its original deterministic narration. A later rendering failure cannot uncommit a turn, remove original text or require another character decision. Retrying a committed command still uses original durable replay; a separate presentation read may invoke the narrator again, as expected for regeneration.

The interface is an internal trusted implementation seam, not an untrusted output validator. A subsequent external adapter must constrain/validate output and preserve original speech/action attribution. Merely including original event IDs or quotes in arbitrary generated prose does not prove semantic equivalence; appended unobserved thoughts/actions would still violate the goal. That contract, actual custom instructions, explicit independent runtime configuration, truthful UI capability metadata and real/fake-provider acceptance are still required. Do not downgrade the full goal to injection alone or a keyword-only custom-style approximation.

## Evidence

- Real settled-turn storage fixture injects a distinct first-person presentation into a pinned second-person turn. Verifies source facts/style, JSON/stream equivalence, provider-time single-connection access (transaction released), foreign-session and over-budget requests do not call provider, failure preserves original observation, command replay does not call presentation, unchanged world head and rejection of nil provider.
- Initial focused narrative/style normal tests: PASS storage1.255s/HTTP0.247s.
- Final expanded selection with race: PASS storage11.237s/HTTP3.046s; storage/HTTP vet PASS.
- Complete HTTP package and server command regressions: PASS12.381s/0.168s.
- Diff whitespace PASS. No frontend/runtime configuration changes, deployment or stage checkpoint.
