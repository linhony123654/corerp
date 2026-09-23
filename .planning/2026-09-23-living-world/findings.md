# Living-world findings

## RP-2B recon
- Three causal chains now exercised: actual gift spending creates cash-security need/goal and refusal; actual work schedule constrains invitation; observed repeated conflict changes later choice into committed movement, persisting across restart. A promised meeting only becomes fulfilled with matching actor, target, time, location and co-presence; retries never create a second fulfillment.
- Existing immutable Events, journals and observation_records suffice; schema remains025. Derived temperament seed is explicitly versioned, sourced to materialization, and is not a biography. Relationship obligation is signed (positive owes, negative owed), independent from trust/familiarity/affinity/tension/role.
- `materialized_entities` links own asset/receivable/liability accounts and immutable cohort_materializations. Existing NPC Ada/Bo/Cai owe75/90/60 minor respectively and have500/600/400 assets; these are actual initial allocations, not invented game bars.
- M1 employment/rent contracts are linked to economic_entities; M2 named wages use `m2_wage_participation_splits` and own obligations/receipts. A source Cohort's contract is not automatically every member's individual employment. Only actual participation can be exposed.
- `agent_profiles.goal_code` and position activity/next schedule exist. No relationship/temperament/memory tables found in migrations001–025. Accepted speech is immutable with statement/question/request only; do not rebuild this constrained table casually to add social effects.
- Existing `agent_knowledge` replays/rebuilds from `observation_records` for arbitrary claim types. Extend filtered allowlist for a carefully scoped interpersonal observation when implemented. A derived memory/relationship view can reuse this lineage without a second memory/history authority.

## RP-2A implementation evidence
- Official OpenAI structured-output guide and Chat Completions reference fetched: closed JSON object with all properties required and strict schema. Adapter supports this explicit dialect; no automatic downgrade to weaker JSON mode. Sources linked in docs/rp2/README.md. Remote JSON is locally validated regardless of claimed schema compliance.
- Only CORERP provider environment presence checked (not values): mode/endpoint/model/key all unset. Live E2E REQUIRED_IF_AVAILABLE; current tests use a real HTTP adapter with local fake model, not external accounts.
- Immutable RPService provider wrapper avoids mutating shared Store/provider configuration per request. No migration/new DB authority. Existing fallback converts adapter errors to audited silence while retaining accepted player speech.
- All network tests pass after fixing test handler cleanup and correcting the assumption that Cai must have next_schedule. Cai has current activity/assets but no next scheduled task; optional field absence is correct.
- Browser model-fixture E2E passes with 8 model calls; recovery after committed effects adds zero calls and zero Event/hearing/utterance rows. Deterministic E2E also passes with0 calls.

- Baseline main `e1aa4c7` clean; schema025. No nested AGENTS/CLAUDE files found. Vue3/TS/Vite with npm lock; Go1.23 module, SQLite sole backend dependency. No LLM SDK/adapter exists.
- `storage.BuildRPDecisionInput` already filters hearing evidence, own assets, next schedule, own Knowledge and visible scene/legal routes. `DecideRP` audits fallback without world mutation; `CommitRPDecision` checks legality/freshness and commits. `RunRPTurn` accepts injected provider but HTTP-facing `PlayRPTurn` hardcodes deterministic.
- Server config currently validates static auth and cursor secret, constructs Store and HTTP service. Provider config should fail closed on partial/invalid setup and default deterministic when unset.
- External semantic search was unavailable during RP-1; no repeat attempt needed. Scoped rg used over known symbols, as goal permits.
- Current audit retains only status/input hash/proposal, not provider error bodies. Retain that boundary to avoid logging remote echoes of sensitive request data or credentials.
