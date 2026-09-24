# Attributed narrative streaming

## Implemented boundary

`POST /api/v1/rp/narrative/stream` accepts the same authenticated session/settled-turn/style-override request as `narrative/render`. Authorization, own-turn lookup, pinned style resolution and immutable fact gathering finish before emission. Database connections are released before network writes. The deterministic renderer emits each attributed line immediately after rendering it; no timer, typewriter effect, post-hoc character slicing, decision rerun or world mutation. Existing synchronous Render/ReadRPNarrative remain compatible wrappers over the same logic.

This is real per-fact streaming for the existing deterministic narrative provider. It is not model-token streaming, free-prose interpretation, or evidence that the primary turns/run path streams before settlement. The NPC decision provider is a different boundary. Those broader product/capability requirements remain subject to the RP6 gate.

## Primary turn integration

Play now uses the stream for newly submitted dialogue too. `turns/run` first settles world decisions through its existing idempotent command owner; only a response with settled status and a turn ID advances the browser bookmark to `pending.narrative_turn_id`. The original request/key remain preserved. This transition is saved before starting `narrative/stream`; the stream omits a style override so the settled turn's pinned style owns its original narrative. No speculative/uncommitted actions are narrated.

A command-response loss still replays the original command key. Once the settled-turn ID is known, a stream failure/reload retries **only the read**, never turns/run. The main reading area marks arriving lines “行动已提交 / 尚未读完”; world actions remain disabled. Clear the bookmark and draft only after a validated full stream and refreshed authoritative history. If the history refresh itself fails, the settled-turn marker remains and the retry is still read-only. Component unmount cancels the in-flight stream. A pending presentation is not a rolled-back command.

This deliberately begins after decisions settle; it does not stream speculative model reasoning or change the world transaction boundary. The existing synchronous response remains compatible with older clients. No new server mutation route or schema is introduced by this integration.

## Wire protocol

Success uses `application/x-ndjson; charset=utf-8`, `Cache-Control: no-store`, `X-Accel-Buffering: no`. Each JSON object is newline-terminated and flushed. A fact can contain literal newlines inside its JSON string; those are not frame boundaries.

```json
{"type":"line","chunk":{"index":0,"event_id":"attributed-event","line":"accepted fact rendered in the chosen style"}}
{"type":"done","count":1,"warnings":[]}
```

Indexes start0 and increase by1. `done.count` must equal received line count. An error before the first emission uses the normal API error envelope/status (including authorization/not-found). A later error uses a generic `type:error` frame without internal details; no `done` follows. A disconnect may provide neither error nor done. Writes have15s deadlines; context cancellation or callback failure stops further rendering. No Last-Event-ID/replay cursor is needed: clients retry the same immutable turn/style request from the beginning, replacing the preview rather than appending.

## Client semantics

The decoder incrementally handles UTF-8 bytes and NDJSON boundaries; validates content type, sequential index, nonempty attribution, terminal count/warnings and no frames after done. It rejects a missing done, an unterminated final frame and malformed data. Safety limits are2MiB/1024lines per response, **not** a model context budget. Fetch has45s timeout plus component-unmount cancellation.

PlayRegenerate shows received lines as explicitly unfinished preview. Current complete text remains until the stream validates and ends. Failure clears preview and preserves original/current complete variant; retry pins the same request. Only full completion emits the new page-local variant. Reload/restore-original behavior remains unchanged. No token or stream state is persisted.

## Evidence

- Core test proves synchronous/stream equivalence, multiline accepted speech, ordered event attribution, cancellation, callback failure and first emission before a later invalid fact fails (not buffered whole-result delivery).
- HTTP style test proves foreign-session rejection before flush, real line+done protocol, matching synchronous content/event IDs, private headers and actual flushing.
- Parser browser fixture delivers each UTF-8 byte separately and holds done until preview is observed. It rejects truncation, bad indexes/counts, explicit errors, trailing frames and incomplete terminal framing. This is labeled transport-fixture evidence, not production timing.
- Real-service regenerate test retains an actual first frame but drops done, checks original text remains, then retries actual stream and verifies no changed world/decision/transcript and no additional decision-model calls. A separate controlled timing fixture uses real server frames to inspect unfinished preview without adding production delays.
- Full wallet/settings/regenerate local HTTP decision fixture passes; no live LLM claim. Primary-turn fixture additionally proves truncated stream→process restart/browser reload→read-only recovery with unchanged facts/model-call count and no repeated command request. A controlled timing fixture over actual frames demonstrates main reading preview before done. Required100+turn browser acceptance and broader narrative capability remain open.
