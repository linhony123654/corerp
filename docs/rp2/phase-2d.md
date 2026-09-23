# RP-2D — Scoped style and decision/narrative separation

Status: PASS locally, 2026-09-23. Baseline `66211c0`; additive migration026. RP-2E and later remain pending.

## Reuse / implementation

Reused durable turn-open and recovery stages, immutable accepted speech/NPC Events and original Play history. Extracted typed committed narrative facts; core narrative input/provider interface is distinct from decision input. The default literal renderer preserves existing default output. Versioned profiles, property overlays, builtin declarative presentation references and explicit unsupported/conflict warnings are new presentation behavior, not world authority.

Application configuration revisions are immutable and permission-scoped. World styles require a scoped creator with existing write authority; session and location-bound scene styles require player/session control. Updates use expected revision and idempotency. Each turn atomically pins effective style before its first world effect. New settings during a crash affect only future turns. Read-only narrative variants use the same Event IDs and do not call DecisionProvider or alter stored history.

## Acceptance evidence

- Default/world/session/scene/turn property precedence and explicit zero/list-clear semantics tested; world POV is not accidentally overridden by an implicit legacy default.
- Core renderer changes POV, tense/framing, detail and quote layout while preserving all action identities and accepted text. Prose cannot make an NPC perform an action. Optional forbidden framing is suppressed; an unavoidable literal-fact conflict is reported rather than falsifying speech.
- Actual RP turn interrupts after committed NPC effects. Settings change, DB reopens, original pinned style survives and no decision call repeats. Decision input is inspected to exclude style/prose fields.
- Same settled turn renders in multiple styles with identical source Event IDs and unchanged Events/NPC decisions/utterances/Knowledge/balance totals.
- Scope/permission tests cover player rejection for world changes, creator rejection in another world, scene isolation after travel, exact retry after leaving a scene, stale revisions, rollback and immutable pins.
- HTTP tests exercise authenticated configuration, actual first-person turn and read-only same-event variant; another principal cannot read private turn history.
- Migration tests preserve authoritative speech/NPC rows from older RP schemas and preserve both settled and interrupted025 turns through026; immutable literal defaults are backfilled. Original downgrade fixtures were updated to remove the newly added presentation tables in their **temporary test databases** and use an explicit025 version constant.
- Actual browser style scenario PASS (`/tmp/corerp-rp1-e2e-ux2jcf`): set first person, lose committed response, change settings to second person, restart server/browser, recover first-person history, then use second person for the next turn. Read-only third-person variant leaves world counts unchanged.

## Verification status

- Focused core/storage/HTTP and migration tests PASS (latest core0.003s/storage4.027s/HTTP0.503s).
- Current-code typecheck/build and browser style (`/tmp/corerp-rp1-e2e-0HlhlS`), provider fixture (`/tmp/corerp-rp1-e2e-sNBXkM`,8calls) and emergent recovery (`/tmp/corerp-rp1-e2e-fLN3U3`) PASS. M0 52 checks, M1 evidence, M2 26 test references and diff check PASS.
- Final full Go PASS (storage55.812s, HTTP4.989s); vet PASS. Relevant race PASS (core1.022s, decision1.457s, storage98.491s, HTTP22.251s, server3.039s, CLI3.747s). Corrected older M2 upgrade fixture focused/race PASS (0.375s/4.396s), followed by full rerun. All migration/reopen/replay assertions retained.
- First full run failed because the older M2 wage-policy upgrade fixture left026 tables while deleting its version marker. Updated only that temporary fixture's downgrade boundary, then verified actual forward upgrade and full suite. No production database downgrade or repair was performed.
- No live narrative-model claim: deterministic literal provider supports bounded presentation; free-form instructions are retained only in separate narrative input with an explicit warning. Real decision adapter remains independently configured; live model REQUIRED_IF_AVAILABLE / NOT VERIFIED without credentials.

## Limitations / next

This stage is a minimal StyleProfile/runtime boundary, not full RP-6 narrative productization. LLM prose interpretation, richer narrative packs, streaming and style/regenerate controls remain in RP-6. Inner thoughts are not invented without committed evidence. Existing documented Extension Registry contract is not rewritten; current builtin pack references execute no plugin code. Scene scope is session+place, explicitly documented.

Recovery point before D: `66211c0` / schema025. Migration026 is forward-only and additive; retain a pre-upgrade DB copy to use a pre-D binary. D checkpoint follows full verification. Next is RP-2E long-run integration, including the recorded economy/RP bootstrap composition issue.
