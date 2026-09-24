# Recorded explanation evidence

`POST /api/v1/studio/events/explain` reads one authorized Event's recorded NPC
candidate-validation diagnostics, committed decision relation and observer metadata. It requires the existing
role/branch/event/rule grant plus an explicit `explain` field on that same grant.
Local administrators opt in with `corerp-admin -explain`; ordinary event reads and
historical grants do not imply this permission. Players are denied.

Request: `instance_id`, `branch_id`, `event_id`, optional `observer_id`, `limit`
(default20, maximum50), `through_record_order`, `after_record_order`, and
`after_observer_id`. Authentication binds the principal server-side. A specific
observer lookup cannot also carry an observer cursor. Every page reauthorizes.

The first response captures an audit cutoff; preserve `through_record_order` and
use `next_record_order` for subsequent diagnostic pages. Audit records can append
without advancing the world Event head. Observer pages use `next_observer_id`;
they reflect recorded observations at each read, not a frozen knowledge snapshot.

Diagnostics are explicitly `non-authoritative` and marked
`npc_candidate_validation_only`: validated candidates are not proof that an action
committed. New rejection records contain bounded reason codes; historical records
without a reason return `not_recorded`. Provider failures record `provider_failure`,
never the original provider error. No raw proposal, input hash, private context,
claim payload or arbitrary audit family is returned.

Creators may inspect recorded observer IDs/counts/first observation time. Absence
means only `not_recorded`, never proof of subjective ignorance. Operators receive
only redacted diagnostic metadata, without NPC IDs/actions or observer lists;
explicit observer probes are denied. Reads do not call models or mutate the world.

`committed_evidence` separately reports `recorded_trigger_and_outcome`,
`not_recorded`, or `redacted`. For a committed NPC response Event, creators receive
`committed_decision`: decision ID, trigger Event ID/sequence, NPC ID, action and
outcome (`committed_speech`, `committed_movement`, `committed_silence`,
`committed_wait`). It joins immutable `rp_npc_decisions` and `rp_utterances` with
both Events in the exact authorized branch and the trigger earlier than the result.
No proposal text, private context or input hashes are exposed. Operators receive no
relation details. Candidate-only validation cannot produce this committed relation.

Committed initiative Events are also supported: `source_kind` distinguishes
`npc_response` from `npc_initiative_command`; the latter's `decision_id` is its
committed command ID. The exact same-branch earlier trigger is read from the immutable
initiative Event, with committed-command ownership checked. `status` retains the
existing outcome contract; new optional `reason_code` distinguishes provider failure,
invalid proposal validation and contact-opportunity suppression. Invalid proposals
still settle as silence under `provider_fallback`; their original content and provider
error bodies are not persisted. Validated Events omit the new reason field to retain
legacy encoding; old missing reasons are displayed as `not_recorded`, never inferred.
The existing cooldown return creates no Event; no historical cooldown record is
invented. This remains a known distinction from committed silence.
This is a recorded trigger/outcome, not proof of subjective motive or a complete
causal graph. Silence/wait explain the absence of speech/movement in that submitted
decision; they do not assert that all world state remained unchanged.

Studio displays this separately from non-authoritative diagnostics and offers a
trigger-Event button that invokes the normal reauthorizing event read. Existing
Events and hashes are never rewritten to backfill generic causation fields.

Verified storage/HTTP coverage includes real rejected candidates, append-between-page
cutoff behavior, actual hearing records and offsite absence, role isolation,
rebuild/reopen and permission revocation. CLI opt-in compatibility has an additional
regression test. Focused race5221 passed core/storage/HTTP/admin; related normal7219
passed decision/storage/HTTP, and whole-backend vet92910 passed. The server package
had no matching tests in7219. Studio now offers explicit evidence reads, observer
lookup and bounded pagination; real-browser verification is recorded in progress.md.
Five actual committed action families are tested before/after rebuild and reopen,
including candidate-vs-commit separation, ops redaction and no extra world writes.
The [seven-question acceptance reconciliation](inspector-acceptance.md) distinguishes
these supported recorded paths from unknowable historical absence; it does not
claim a generic causal engine for every event family. Historical package provenance
now complements the rule metadata. Full-stage verification remains separate.
