# RP-2D — Versioned narrative style

Style changes presentation only. It is never added to `RPDecisionInput`, accepted speech, economic commands or Knowledge. The narrative provider receives a separate `RPNarrativeInput` containing typed committed facts and a resolved style. Current provider is a deterministic literal renderer, **not an LLM narrator**. Free-form instructions are retained in narrative input but return an explicit unsupported-instruction warning with this provider.

## Profile and precedence

`corerp.style.v1` includes:

- `pov`: `first_person`, `second_person`, `third_person`.
- `tense`: `present`, `past`.
- `verbosity`: `terse`, `normal`, `detailed`.
- `dialogue_ratio`, `description_density`: integers0–100, presentation preferences rather than permission to omit facts. Literal rendering uses dialogue preference for quote layout; available place/time evidence bounds descriptive detail.
- `inner_monologue_policy`: `none`, `observed_only`. No committed thought evidence means no inner monologue; omniscient invention is rejected.
- `prose_instructions`: up to2000 characters, narrative-only. Unsupported free-form interpretation is reported, not silently claimed.
- `forbidden_patterns`: up to16 literal strings, each1–100 characters. Optional framing is suppressed if possible; conflicts with exact accepted quotes or necessary action/attribution are reported while preserving facts.
- `narrative_pack_ref`: `builtin/plain@1` or `builtin/dialogue@1`. These are declarative literal presets, not executable plugins or a new Extension Registry. Unknown pack references are rejected.
- `version`: structural contract version, included in resolved/pinned profiles.

Property precedence is Default → World → Session → Scene → Turn override. The legacy session POV supplies the unconfigured default; explicit new layers override it. A scene is currently the controlled character's place **within that session**. Leaving it stops that scene style from applying; returning reuses its current revision. Scene settings do not affect other sessions or places.

An omitted/null scalar inherits; explicit zero/empty overrides. `forbidden_patterns:null` inherits, `[]` clears. A new binding revision replaces the entire layer definition, not a merge with its previous revision. Each turn pins the resolved profile and source revisions atomically with its durable open intent, before player speech/decision effects. Later setting changes cannot change that turn during recovery.

## HTTP API

All endpoints use existing Bearer authentication and strict JSON decoding. `principal_id` may be omitted for binding to the authenticated principal.

`POST /api/v1/rp/style/set`

```json
{
  "instance_id": "inst_m2_t09",
  "branch_id": "br_main",
  "scope": "session",
  "session_id": "<your session>",
  "expected_revision": 0,
  "idempotency_key": "style-first-person-v1",
  "patch": {"pov": "first_person", "narrative_pack_ref": "builtin/dialogue@1"}
}
```

Scopes: `world` omits session/place; `session` requires session and omits place; `scene` requires both session and current `place_id`. World scope requires an active creator principal with existing scoped write authority (`world.cohort.materialize` or `world.simulate`) in that instance/branch. Player session control is insufficient. Session/scene changes require ownership and current control of the session character; new scene changes require actual presence at that place. Exact retries remain valid after moving away, subject to retained session authority.

Return: `revision_id`, monotonic `revision`, `replayed`. Start with expected_revision0 for an absent binding, then use the last revision for the next update. A stale revision fails; a reused key with different content fails. Changes append immutable configuration history and update its binding, without advancing the world head or writing world Events.

`POST /api/v1/rp/style/read` accepts `{session_id}` and returns the currently resolved `profile` plus revision `sources`. It is scoped to the caller's own session.

`POST /api/v1/rp/turns/run` accepts optional `narrative_style` containing the same patch fields as a turn-only override. The durable orchestration request pins it; the underlying speech command explicitly excludes it. The direct `/actions/speak` endpoint rejects narrative settings rather than silently applying them as world input. Turn responses include `narrative_style` and `narrative_warnings` alongside the committed narrative and Event IDs.

`POST /api/v1/rp/narrative/render` accepts `{session_id,turn_run_id,style_override?}` for a **settled own turn**. It returns resolved `style` and `{lines,event_ids,warnings}` under `view`. This is a read-only variant, not world rollback, a decision-model retry, or replacement of stored history. Dedicated style/regenerate UI belongs to RP-6; current Play already displays the pinned lines for real turns.

## Persistence and recovery

Schema026 adds application tables `rp_style_revisions`, `rp_style_bindings`, `rp_turn_styles`. Revision history and pinned turn styles are immutable. Migration backfills legacy turns with the old fixed second-person literal style, including interrupted turns, without touching accepted utterances, NPC effects or stored completed narrative. Prior schema025 files can be reopened and migrated forward; use a verified pre-upgrade DB backup if running an older binary is required. No downgrade operation is supplied.

The literal renderer preserves accepted quotes byte-for-byte, attributes statements to speakers and retains all committed action identities. It can add only supplied place/time facts, not hidden emotion, wealth, knowledge or causes. Source Event IDs stay identical across styles. [Phase report](phase-2d.md) records executed checks and limitations.
