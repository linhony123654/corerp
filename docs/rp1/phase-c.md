# RP-1C — Speech / Audibility / Knowledge / Turn Consistency

Status: PASS locally on 2026-09-23 after final regression. This is not the full RP-1 playable loop.

## Implemented

- POST `/api/v1/rp/actions/speak` accepts a player utterance through the same scoped session-control and fresh-observation checks as earlier RP actions. The server assigns stable Event, Turn and Utterance IDs from the session/idempotency key. Same request retries return the committed Event; same key with different text conflicts.
- `RPSpeechAccepted` Event and migration 023's immutable `rp_utterances` row record speaker, world/branch via Event, place, world time, exact text, speech act and turn identity. The table is an immutable accepted-utterance index, not a second objective truth or editable narrative transcript.
- Audibility in this bounded scene is actual same-place active Entity presence. For each listener, one existing `observation_records` evidence row and `agent_knowledge` projection row records `claim_type=speaker_said` with text and speaker attribution. Different-place entities receive neither row. No rule for whispers or complex sound propagation exists in the current M2 kernel.
- One SQLite authority transaction includes speech Event, immutable utterance, all hearing evidence/knowledge, Session `turn_cursor`/`speech_committed`, Branch Head, clock lineage and Outbox. Outbox `audience_scope` names only speaker and hearers; an external publisher must enforce that scope. Delivery is at least once, while the speech/knowledge commit is exactly once. No vector or memory index is on the authority path.

## Verification

| Check | Result |
| --- | --- |
| Full backend regression | PASS — `cd backend && /usr/local/go/bin/go test ./... -count=1` |
| Static analysis | PASS — `cd backend && /usr/local/go/bin/go vet ./...` |
| Related race | PASS — targeted RP/migration/HTTP/CLI `go test -race` |
| Migration parity / upgrade | PASS — 023 embedded SQL equals docs mirror; disposable 022→023 and 019→023 upgrades preserve RP/M2 world data |
| Audibility / false claim | PASS — initial cafe speech reaches Cai, not offsite Ada/Bo; after scheduler wait, two or more same-place NPCs hear. A false financial boast remains only `speaker_said`; it does not create money/economic Events. |
| Retry / crash boundary | PASS — same-key retry does not duplicate Event or Knowledge; changed text conflicts; precommit failure leaves no speech, knowledge or turn stage. A simulated crash after publisher delivery but before Outbox publish mark preserves world/knowledge; after reopen the same Outbox ID is retried and then marked sent. |
| Replay / immutability | PASS — reopen, projection comparison and rebuild recover listener Knowledge; SQLite rejects editing the accepted utterance. |
| HTTP principal scope | PASS — creator token cannot operate the player's RP session; player token can speak and observe new Branch Head. |
| M2 evidence preservation | PASS — `npm run verify:m2-evidence` still finds 26 executable tests |

No production database, external account or actual LLM/vector service was used. The current Outbox dispatcher passes `audience_scope` to an injected publisher but does not itself implement an external recipient router; production delivery must respect that scope. `speech_committed` is a minimal authoritative stage, not the later continuous turn orchestrator.

## Recovery point and next stage

- RP-1B checkpoint: `8c8ed78`.
- RP-1C checkpoint: the Git commit containing this report; find via `git log -- docs/rp1/phase-c.md`.
- Next: RP-1D replaceable NPC DecisionProvider with scoped inputs, validated proposals and safe refusal/failure behavior. No full RP-1 DoD is claimed here.
