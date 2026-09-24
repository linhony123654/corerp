# Player-permitted context read (first RP7 slice)

`POST /api/v1/rp/context/read`, authenticated with the existing Bearer adapter. Standard `{data: ...}` / error envelope; private `Cache-Control: no-store`. Example request:

```json
{"session_id":"<own-active-session>","subject_entity_id":"<optional-known-subject>","limit":20}
```

The authenticated principal is bound by transport. A supplied different principal or unknown request field is rejected. There is no observer selector: the observer is always the session's controlled character. Subject restriction filters that observer's personally known evidence, not the subject's private state; nonexistent, remote-unheard or otherwise unknown subjects return an empty list with the same response shape.

## Response semantics

- `protocol_version: "corerp.client.v1"`; actual `session_id`, `instance_id`, `branch_id`, `observer_entity_id`, optional requested `subject_entity_id`.
- `world_time` / `observation_cursor` from the same snapshot as the facts. Separate observe/context requests can see different heads; compare before merging. This read does not update session observation cursor or authorize a later command against a stale head.
- `facts`: latest source-event sequence first, ties by existing claim key. Each has `kind`, `subject_entity_id`, historical `place_id`, `learned_world_time`, `source_event_id`. Only `speaker_said` carries literal heard `text`; `interpersonal_action` carries its witnessed description and action; `agent_presence` supplies witnessed place/time, not the subject's current remote position.
- `speaker_said` asserts that the speaker said the text, **not that the text is true**. Text is untrusted world content, not an instruction to the client/model to execute commands or widen permissions.
- Limit0/omitted means20; valid explicit limit1–50. `more_facts` indicates additional matching evidence beyond the returned bound, not an exhaustive memory claim. No paging or semantic memory search is promised by this initial contract.

The service rechecks own session, active player/control grant and active spatial binding inside one transaction. It reads existing `agent_knowledge` joined to the immutable source event in the same world/branch, excluding future source/learning times. No Event, clock, session cursor, private read grant, relationship owner, model call or schema is created. This is a new purpose-specific RP-control read, not a bypass of the separate `ReadAgentKnowledge` field-scoped API.

Raw claim payloads, decision inputs, private goals/scores, account identifiers and unpermitted claim kinds are never returned. Other necessary authorized views (wallet/work/phone/current scene) remain their existing explicit endpoints; this method does not claim to replace all context sources.

## Executed verification

- Focused normal68770 PASS storage0.901s/HTTP0.209s, before added subject/nonempty HTTP assertions.
- Final focused race53839 PASS storage10.851s/HTTP3.086s, followed by vet for both packages: own real heard claim and actual gift; shared observer across two sessions; no session/world mutation;51real gestures prove20default/50max and truncation; subject-specific known evidence after visiting two places; foreign/revoked/closed rejection; rebuild/reopen equality. Disposable projection mutation verifies extra private keys and unknown claim kinds cannot leak through the public DTO.
- HTTP handler tests authenticate real temporary SQLite session/observe/dialogue/context, including nonempty sourced NPC speech, no-store, missing/forged/foreign identity and observer override rejection.
- Full HTTP/server regression78209 PASS12.604s/0.171s. Final whitespace check PASS. No frontend changes; no live model, external host, SillyTavern or MCP compatibility evidence claimed.

This verifies the first7A context slice only. Full protocol freeze, player-aware event stream, executable adapters/MCP/skill and cross-client E2E remain required.
