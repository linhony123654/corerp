# Narrative context budget

## Meaning and compatibility

`context_budget_bytes` is an additive StyleProfile/StylePatch field scoped through the existing default→world→session→scene→turn override resolution. It bounds the UTF-8 JSON encoding of the complete `RPNarrativeInput` (controlled identity, style and all accepted facts) supplied to a presentation read. It is not a claimed tokenizer estimate, output-token cap, memory-selection strategy or NPC decision budget. The UI uses KiB (1024bytes), with default64KiB and explicit4/16/64/256KiB options; server accepts any integer4096–262144 or0 for default.

Zero is omitted from profile encoding; nil patch inherits, explicit pointer0 resets to default. Existing zero-valued profiles/patches retain their canonical encoding and existing pinned style hashes. No schema migration or alternate style owner is introduced.

## Why this boundary

Narrative preferences must not change world authority or erase accepted speech. Therefore budget enforcement happens in authenticated `StreamRPNarrative`/`ReadRPNarrative` after input gathering but before any provider emission. It rejects the whole presentation request when over budget; it never cuts speech, drops a person's action, or silently lowers detail. The error reports actual encoded bytes and configured maximum without exposing foreign state.

Canonical turn settlement still renders and saves the full accepted record independently of this presentation-read preference. Otherwise a small budget could leave a command whose world effects had committed permanently unable to finish. A low budget may thus reject a post-settlement stream, but cannot prevent or undo settlement.

The primary UI offers “读取已保存原叙述” for a committed turn with unfinished presentation. This performs only the existing authenticated observation read, clears pending presentation after success, and does not call turns/run or the decision provider. On reconnect, a stream failure loads observation so this exit remains available rather than trapping the user at the entry form. Raising live settings cannot change an older turn's pinned budget; after reading its saved original, use regenerate with the larger current budget for that turn.

## Verification

- Core checks legacy encoding omission, invalid limits, explicit-zero layering, Chinese UTF-8 bytes, exact byte boundary and no input mutation/truncation.
- Real storage test sets4KiB, submits1800 Chinese characters through the actual turn owner, proves settlement/full persisted speech, rejects stream before the first chunk, reads the original through observation, succeeds with explicit64KiB override, and safely replays the original command with no new head.
- `node scripts/verify-rp1-play.mjs --context-budget --fake-model` uses actual setting UI/service/SQLite and1800-character accepted speech; checks over-budget error, process/browser restart, saved-original fallback with zero additional command/model calls, larger-budget regeneration with complete speech, and unchanged Events/decisions/utterances. HTTP model is a local fixture, not live LLM evidence.

Free-prose instruction execution is still unsupported by the deterministic narrative provider and remains an open requirement. This budget does not disguise that capability gap.
