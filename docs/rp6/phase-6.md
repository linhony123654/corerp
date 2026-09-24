# RP-6 — playable narrative product

Status: RP6 local acceptance PASS, authorized checkpoint pending. Personal views and narrative controls/streaming/context/recovery are implemented and verified, including independently configured constrained natural-language style planning. [Capability audit](capability-audit.md) retains bounded-style limitations. [Actual104-turn/128h browser journey](long-play.md), sourced memory/random behavior, midpoint restart and [full applicable stage verification](verification.md) all pass. All test processes terminal. Baseline source47c5d15, documentation364976f; schema026 unchanged. Full living-world goal continues through RP7/RP8/final integration. Follow-up sections below are chronological; current status and verification supersede earlier pending lists.

## Acceptance and evidence

Final verification results and any still-running checks are maintained in [verification.md](verification.md); complete source-to-requirement mapping is [capability-audit.md](capability-audit.md). The stage is not complete until that gate passes and the authorized checkpoint has a clean worktree.

| Original requirement | Observable acceptance | Required evidence |
|---|---|---|
| 6A immersive Play | Scene, text, input, time/place/presence and only necessary state dominate; no ledger/cohort/epochs/scores/debug/migration/projection internals by default | Real rendered desktop/mobile inspection and browser assertions |
| 6B in-world information | Wallet, work schedule, address book, phone messages and map opened on demand; only own/known sourced data, not permanent numeric HUD | Actual authenticated reads, empty/error states, keyboard entry/close/focus and privacy checks |
| 6C narrative | Presets/custom style/POV/density/dialogue ratio/context budget, actual streaming, safe retry and regenerate UI | Owner-backed persistence, stream failure/reconnect, pinned facts and world-head invariance on regeneration, browser recovery |
| 6D long play | At least100 turns spanning multiple world days with work/consumption/relationships/memory/opportunities, restart, usable UI throughout | One actual service+browser journey with factual checkpoints and retained artifacts; backend-only runs are insufficient |

## Reuse and boundaries

- Vue3/Vite/TypeScript, existing local typography and paper/ink Play identity; no framework replacement.
- Default main.ts mounts PlayWorkspace; /demo alone mounts historical Story/Inspector. Keep engineering tools opt-in.
- Existing durable pending command bookmark stores exact request before sending; credentials remain memory-only. Preserve retry/restart semantics.
- Existing Event/clock/movement/Career/economy/knowledge/history owners remain authoritative. Panels are filtered read views, not new state stores.
- Existing style profiles, scoped revisions and narrative/render are presentation owners. Regeneration cannot replay a turn, roll back facts or call the decision provider again.
- Deterministic renderer currently does not interpret arbitrary prose instructions; controls must disclose actual capability. No fabricated live-model support.

## Local delivery and recovery

Changed surfaces are player-only on-demand views, scoped read endpoints, narrative preferences/read streaming and independent bounded style planning; the existing world/Event/ledger authority remains intact. No dependencies, production deployment, schema migration or new world-state owner were introduced in RP6. Current schema remains `corerp-rp2-styles-026-2026-09-23`.

The pre-stage recovery point is `364976f17b481ef19dbd6c350b8825103ad2ea6f` (RP5 source `47c5d15403b8fdb1e4c8dd7dad4289caff9c215b`). A source checkpoint does not roll back committed world facts. Before trying another code version against important local data, preserve a consistent SQLite backup (including correct WAL handling); do not assume that unchanged schema proves every newer style JSON field is backward-compatible with an older binary. No database rollback or destructive Git operation was performed.

Known limits: development-only authentication/TLS setup, no live model quality validation, bounded style plans rather than arbitrary prose, transaction-only phone notices, topology-only map, most recent50history entries, and page-memory narrative variants. These are explicit product/operator boundaries, not hidden test substitutions. RP7/RP8/final integration remain required after this stage.

## Delivery order and unresolved design work

1. Map exact observation and style revision read contracts; resolve safe settings save/recovery and personal read privacy before UI writes.
2. Inspect current visual references and rendered Play; establish visual execution contract preserving long-session reading and compact mobile controls.
3. Implement a real owner-backed personal/settings slice with targeted service/browser checks, then complete all required panels and narrative controls/streaming.
4. Exercise100+turn multi-day browser play, restart/recovery, accessibility/error/loading/empty cases and actual world effects.
5. Required full-stage verification, phase audit, authorized checkpoint and clean tree, then RP7. No publication requested.

No RP6 completion claim; visual contract, transport design and long-run scenario remain to be resolved before broad implementation.

### Wallet read contract

`POST /api/v1/rp/wallet/read` accepts the existing session read request and authenticated bearer identity. It resolves the session's controlled entity, checks the live control grant and active spatial binding, then reads its actual asset account and currency in one transaction. No arbitrary entity/account selector, NPC decision context, private goals, relationship scores or account identifiers are exposed. It rejects inactive sessions and closed accounts. Reading does not create Events, advance the clock or update the session cursor.

Response fields: `balance_minor` (exact signed decimal string, not a JavaScript floating-point amount), `currency_id`, `currency_symbol`, `currency_scale`, `world_time`, `observation_cursor`. Clients must format using the returned scale rather than assuming two decimal places; this snapshot is not a cached authority for future commands. Schema and existing account ownership are unchanged. Work/contacts/messages remain separate pending read contracts; this endpoint does not claim those panels are implemented.

Verified initial increment: real SQLite balance read, actual gift changes balance by the committed amount, close/reopen preserves snapshot, foreign principal and revoked controller are rejected, empty binding is invalid, and reads preserve world head. HTTP tests cover authentication, principal spoofing, foreign session, the restricted field set and string amount transport. Focused normal tests PASS storage0.214s/HTTP0.200s; whole HTTP regression PASS11.655s; wallet+style race PASS storage9.594s/HTTP4.806s and both packages vet PASS. Browser/UI integration is NOT VERIFIED and remains the next dependent delivery work.

### First prerequisite slice: safe session style revision read

Verified gap: style/set requires expected_revision, but style/read previously exposed only resolved profile/source IDs. A returning client could not safely choose its session revision without guessing or relying on stale local state. Implemented optional read-only session_revision metadata, populated only by authenticated style/read from the existing revision tables in the same transaction (explicit0 when unconfigured). Profile/patch hashes, pinned turn styles, schema and world head are unchanged. Tests cover unconfigured0, world/session scope separation, actual save/read1, stale conflict, save/read2, reopen, foreign-principal rejection and unchanged world facts; HTTP verifies revision1 and absence from pinned narrative output. Focused normal style tests PASS storage0.891s/HTTP0.257s; race/vet tracked in progress. This is a prerequisite, not the completed settings UI or full representative product slice.

### First personal-object UI: wallet

PlayWallet opens from the scene action row and reads the real wallet endpoint on every opening. Native modal sheet is centered on desktop and bottom-aligned on phones; its own loading/error/retry state never enters the pending world-action bookmark. Explicit keyboard boundary cycling supplements native modal behavior; Escape and close restore opener focus. Late results from a removed component are ignored. Exact currency formatting supports server scales0–18 and signed integer strings; no floating-point amount conversion or assumed cents.

Visual direction and reference limitations are in [visual-contract.md](visual-contract.md). Initial real browser run caught Shift+Tab focus leaving the modal; corrected and rerun PASS. Rendered review caught a global light heading color; corrected and added contrast-color regression assertion. Initial successful real-service artifacts `/tmp/corerp-rp1-e2e-UlkYFc` include mobile/desktop wallet and reading states, but precede the title correction. Final corrected build/browser evidence is tracked in progress.

Reproduce: `npm run build` then `node scripts/verify-rp1-play.mjs --wallet`. This reuses the real temporary SQLite/server/browser/restart journey and adds wallet assertions before and after restart: database balance equality, restricted exact transport, focus/Escape, local error/read retry, close while loading/reopen, unchanged Events/world clock/bookmark and decimal formatting boundary cases. Fault injection and format examples are labeled fixtures; actual balance assertion uses SQLite, not mocked data. This is the wallet slice only; settings/work/contacts/messages/map/streaming and100+turn acceptance remain open.

### Session narrative preference UI

PlayStyle now provides日常/对话/细叙 presets, POV, verbosity, tense, dialogue preference, description density and a custom prose preference. Saves use authenticated current session scope and ReadRPStyle.session_revision. Exact request (scope, expected revision, patch, original idempotency key; no credential) is stored at `corerp.style.pending.<session>` before dispatch. Retain it until both write and authoritative read succeed. A lost response or read failure retries the same request; the server's existing idempotency owner prevents duplicate revisions. Reload shows a recovery action; new world commands wait while a style save remains uncertain. This storage is recovery intent, not authoritative style state.

A definite stale-revision response offers explicit discard-and-read-latest; no automatic overwrite or new-key retry. Read failures cannot save invented default settings. Successful save is a session override of the shown form; existing scene precedence remains and is disclosed. Existing paragraphs are not re-rendered by saving. The deterministic narrative renderer cannot interpret free prose; the UI distinguishes saved preference from supported execution, independently of NPC decision provider mode. Custom interpretation, context budget, streaming and regenerate are still open requirements, not silently waived.

Real browser initial PASS50432: preset use, custom preference, actual first-person next turn, exact saved-intent recovery after service restart/browser reload, one revision despite lost response, another authenticated client's revision race/conflict refusal and explicit reload. Existing world/history invariance and wallet/action recovery assertions pass. Scripts/rp6-style-checks.mjs adds `--style-ui`; final expanded read-failure/keyboard/44px variant verification tracked in progress. No backend authority/schema changes in this increment.

### Read-only narrative regeneration

Observation history now returns `can_regenerate` from the query's source: settled dialogue turns true, movement/wait/social/initiative Events false. No identifier heuristics or unsupported buttons. PlayRegenerate provides a collapsed per-dialogue disclosure, current-style generation, pinned-request read retry and restore-original. It uses the existing authenticated narrative/render owner; no new decision call, mutation, persistent transcript replacement or rollback. The displayed variant lives only in page memory, is pruned outside the50-entry history window, and clears on reload. Current style is captured for a generation attempt; retry retains its exact turn/patch. Failure preserves current text. The UI explicitly says deterministic output may be unchanged.

Focused normal storage/HTTP PASS2.051s/0.725s; updated eligibility assertion plus style/wait/session race PASS16.392s/6.909s, vet PASS. Build and combined wallet/settings/regenerate real browser PASS29541 with deterministic service; final readability/control-visibility revision plus local HTTP model fixture PASS69355 (18total decision calls), artifacts /tmp/corerp-rp1-e2e-e2Dp5N. Checked actual mobile screenshot with regenerated lines and both controls above sticky composer. Separate explicit model-call-count invariant and complete HTTP regression tracked in progress. These are not live-model or streaming evidence.

Reproduce `npm run build` and `node scripts/verify-rp1-play.mjs --wallet --style-ui --regenerate` (add `--fake-model` for the local HTTP decision fixture). Browser checks source eligibility, lost render response, exact retry payload, actual third-person variant, unchanged Events/decisions/utterances/persisted narrative, restore-original, reload and credentials. Context-budget, free-prose execution, streaming, remaining personal views and100+turn real play remain required and open.

### Streaming follow-up

Regeneration now uses real per-fact streaming with an explicit completion boundary, not the earlier single-JSON read. See [narrative-stream.md](narrative-stream.md) for authorization, wire protocol, failure semantics and evidence. Existing nonstreaming API remains available. Decoder and unfinished preview reject truncated streams without replacing the current complete text. Normal/race/vet and integrated browser checks passed, details in progress. This implements the regenerated-view streaming path; primary-turn integration, context budget and custom narrative execution still require work before RP6 completion.

Primary-turn follow-up now implemented: submitted dialogue uses the same attributed stream after world settlement. Durable pending.narrative_turn_id distinguishes an uncertain command from an already-committed turn whose presentation is unfinished; restart retries only the read in the latter case. No style override is sent, preserving original pinned style. Build/typecheck and combined existing wallet/settings/regeneration browser journey pass. Dedicated --turn-stream local HTTP fixture verifies actual truncation, process/browser recovery, no command re-send, unchanged Events/decisions/utterances/model calls, and actual visible partial reading before done; artifacts /tmp/corerp-rp1-e2e-uWb0qP. Context budget, custom narrative execution, other in-world views and100+turn acceptance remain open.

### Context budget and saved-original fallback

Actual presentation context limits are now enforced via optional context_budget_bytes in existing scoped profiles/patches. [narrative-budget.md](narrative-budget.md) defines measured UTF-8 JSON units, legacy encoding, complete-input rejection before emission and why canonical settlement is independent. Settings UI exposes KiB values and explicitly distinguishes them from model tokens/decision context. A committed turn can always be read through its saved original after a presentation limit/error; reconnect exposes that recovery path. No speech truncation, new decision or rollback.

Focused normal core/storage/HTTP PASS0.007s/1.124s/0.268s; final frontend build/typecheck and actual --context-budget HTTP fixture PASS30972, /tmp/corerp-rp1-e2e-jfZbED (18decision calls).1800-character real speech settles under4KiB presentation limit, presentation refuses it, restart recovers original without command/model repeat,64KiB current-style regeneration includes full speech. Additional race/vet and combined regression recorded in progress. Free-prose execution, work/contacts/messages/map,100+turn actual UI and full-stage gate remain open.

### Known contacts

Session-scoped contacts/read and PlayContacts now provide the actual own-knowledge address book without remote location/internal relationship leakage. Exclusive-ID paging, same-transaction authorization/clock, meaningful empty/error/loading, refresh/load-more and modal keyboard behavior are implemented. [contacts.md](contacts.md) documents exact evidence/privacy boundaries. Real conversations and movement establish known contacts; initial0→post-play/restart3 browser reads match SQLite, and reads add no knowledge or Events. Work/messages/map and custom prose execution remain open alongside100+turn/full-stage verification.

### Own work and agenda

Authenticated work/read and PlayWork are now implemented and increment-verified. [work.md](work.md) documents accepted terms, exact wage strings, known metadata versus unregistered fields, actual upcoming schedule, bounded list and evidenced delay semantics. Private employer evaluations remain excluded; no employment or payment authority added. Real temporary economic browser fixture verifies pre-effective empty →24h UI waits →current contract and process/browser restart equality; original combined browser journey passes all current RP6 increments. Actual Career acceptance/start,20-item agenda, sourced delay/projection damage, privacy and read invariance have focused normal/race evidence. Build/typecheck, affected helper race, vet and full HTTP regression pass. Work is removed from the open implementation list; messages/map/custom narrative execution and100+turn/full-stage acceptance remain open. No stage checkpoint or deployment.

### Local map

PlayMap provides an on-demand current-place/adjacent-route diagram using existing observation and movement authorities, without fabricated geography or distant characters. Additive `reachable_places.can_move_now` reflects the same physical traversal check as MoveRP; current local works are shown independently. Departure pins the map's origin/destination/cursor into the existing durable command flow. [map.md](map.md) records the contract and actual blocked→expiry→travel→lost-response/restart evidence. Build, scoped normal/race/vet, full HTTP and all-current-increment real-service browser pass; desktop/mobile screenshots inspected. No map state authority/schema/dependencies added. Remaining RP6 implementation is messages/custom narrative execution, followed by100+turn/full-stage acceptance; full goal stays active.

### Phone transaction notices

Phone now presents existing own Career invitation/response/offer/agreement/decision notices through authenticated messages/read, not a new SMS/chat authority. Historical status and no-send limits are explicit; internal evaluations/position assessments/ambient hearing are excluded by narrow source selection and public DTO. [messages.md](messages.md) defines50-item descending cursor paging and source/privacy boundaries. Actual invitation→own response browser plus process/browser restart,51real-invitation pagination, accepted employment/leave approval privacy, build/scoped normal/race/vet/fullHTTP and all-current-increment browser pass; populated mobile/desktop screenshots inspected. Remaining product implementation is actual custom narrative execution, then100+turn multi-day UI and full-stage verification. No RP6 checkpoint or full-goal completion.

### Independent narrative provider prerequisite

The actual service read/stream path now supports an independent operator-injected streaming narrative provider while canonical settlement retains the original deterministic record. [narrative-provider.md](narrative-provider.md) documents authorized immutable input, transaction release, budget-before-call and failure/replay boundaries. Focused normal/race/vet plus full HTTP/server regressions pass. This is not custom prose completion: external executor/output validation/configuration/capability UI and end-to-end custom-instruction behavior remain open, followed by100+turn and full-stage gates.

### Constrained natural-language style execution

Independent HTTP style planning/configuration and actual primary/regenerate UI execution now follow the provider prerequisite. [custom-style.md](custom-style.md) documents a closed presentation plan (no model-generated fact-bearing prose), style-only external context, validated literal rendering, protected settings, timeout/retry/refusal/redaction and explicit unsupported requirements. Real HTTP-fixture browser changes actual person/tense/layout while preserving canonical text/events; regeneration and restart do not repeat decisions; extra model prose is rejected. Focused normal/race/vet, full core/narrative/HTTP/server, build and prior-increment browser pass. No live-model quality claim. This supports natural-language presentation controls, not arbitrary literary expansion; retain that limit in full6C audit.100+turn actual multi-day UI and full-stage gates remain open.
