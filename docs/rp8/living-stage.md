# Living-stage roleplay extensions

Status: local implementation and focused verification complete on 2026-09-27; production enablement is not implied.

This increment keeps the F13 authority split intact while adding the parts of a mature tavern-style experience that previously existed only as presentation: bounded multi-NPC response selection, typed scene objects, authorized turn replay, explicit execution modes, optional unattended progression, and safer SillyTavern lifecycle/import behavior.

## Execution modes

The active Studio system package may declare `rp_execution_mode` and `max_active_responders`:

| Mode | Activation behavior |
|---|---|
| omitted / `legacy` | Existing worlds keep all internally controlled legal listeners. No responder limit may be declared. |
| `deterministic` | Exactly one stable legal listener is selected. The responder limit must be `1`; player text is not interpreted as a direct-address instruction. |
| `orchestrated` | Familiar, explicitly addressed listeners are selected up to the declared limit; when nobody is addressed, one stable fallback is selected. |
| `multi_agent` | Familiar addressed listeners come first, then stable legal listeners fill the declared 1–8 responder budget. |

Every heard listener is durably accounted for as activated, externally controlled, or not activated with a public reason code. Hearing remains world evidence; provider output remains a proposal; only validated actions become Events. Active Human or external-controller ownership always wins, including a handoff immediately before commit.

## Typed scene objects

Studio world specs may declare bounded `door`, `container`, and `light` objects. Their stable object key, place and initial state are committed during world creation. Runtime actions are a closed vocabulary:

- door/container: `open`, `close`;
- light: `switch_on`, `switch_off`.

Object state changes are Events with rebuildable projections. Observation and narrative receive only objects the observer can physically perceive. The browser never sends arbitrary executable object data and a model cannot create a missing object or unsupported state transition.

## Authorized observatory

`POST /api/v1/rp/observatory/read` is bound to the authenticated principal's own active session. It exposes only observer-appropriate historical stages, anonymous actor/trace references, public activation dispositions, committed actions, sanitized narrative fallback codes, projection health, and aggregate background-progression health.

It does not expose prompts, chain-of-thought, private goals, proposal JSON, credentials, raw canonical identity unknown to the observer, raw Event IDs, lease owners, run IDs or fencing generations. Names are resolved using familiarity as of the trace's historical world time. The Play dialog includes loading/error/empty states, retry, pagination, keyboard focus containment and mobile layout.

## Opt-in background progression

Worlds remain stationary unless the active system package explicitly contains:

```json
{
  "background_progression": {
    "enabled": true,
    "step_minutes": 15,
    "scheduler_budget": 32
  }
}
```

`step_minutes` is bounded to 1–1440 and `scheduler_budget` to 1–10000. A disabled declaration cannot retain budgets. Each run:

1. acquires a per-world lease and fencing generation;
2. stops when a player session, external controller, unsettled turn/wait, or shared round owns the world;
3. runs only the existing scheduler/Agent Runtime up to the package budget;
4. resumes the same target after budget exhaustion rather than extending it;
5. commits any remaining clock advance as a standard `WorldTimeAdvanced` Event;
6. records a redacted operational status and settles due activities.

Session opening and controller assignment refuse a still-running, unexpired background run, closing the race where control could otherwise be acquired halfway through autonomous effects. A crashed worker can be replaced only after lease expiry. Completed runs shorten their lease to a one-second handoff fence, so another concurrently queued worker cannot double-advance while the next scheduled scan is not unnecessarily delayed.

The server worker is disabled by default. Set `CORERP_BACKGROUND_INTERVAL` to a duration from `30s` through `1h` to enable scans. This is an operational switch only; it cannot enable a world whose package did not opt in. Do not set it in production until model/scheduler cost, backup, monitoring and world-owner consent have been reviewed.

Rollback without deleting history:

1. unset `CORERP_BACKGROUND_INTERVAL` and restart the service;
2. leave migration 062 and its audit rows in place;
3. create a newly authorized world/package revision with progression disabled if the product supports package migration; current frozen Studio worlds are not hot-upgraded;
4. never delete `WorldTimeAdvanced` or scheduler Events to simulate rollback.

## SillyTavern lifecycle, groups and import

The 1.19.0 adapter still stores no credential. Within one loaded page, an interrupted event stream resumes from the last saved checkpoint with 1/2/4/8/16-second backoff and stops after five failed retries. Page reload and chat switch require the token again.

Both single and group chats bind exactly one CoreRP player session. Group member cards are saved as a presentation-only mapping and never create, clone, assign or control runtime NPCs. Membership drift is rejected instead of silently changing authority.

The card/world-info tool downloads `corerp.studio-import-draft.v1` locally. It performs no runtime API call. The file labels card and lore text as references, sets `creates_runtime_entities=false`, `activates_packages=false`, and `imported_text_is_canonical_truth=false`, and requires later review through an authorized Studio creation/activation workflow.

## Verification entry points

```sh
cd backend
/usr/local/go/bin/go test ./internal/core ./internal/storage ./internal/transport/httpapi ./cmd/corerp-server -count=1

cd ..
npm run build
node --test clients/sillytavern/client.test.js clients/sillytavern/draft.test.js
node scripts/verify-rp8-play-worlds.mjs
```

The exact-current real-host flow remains `scripts/verify-rp7-sillytavern.mjs` against the integrity-pinned SillyTavern 1.19.0 fixture. Production deployment, package activation and Git history changes are separate explicit actions.
