# RP-2C background initialization

Representative slice, employment integration and final regression verified locally. Schema025 is unchanged. [Phase report](phase-2c.md).

## Authority and contract

1. Use the existing authorized `POST /api/v1/commands/materialize-cohort` to allocate one real individual. Population, balances, claims and inventory remain under the existing conservation rules.
2. Call `POST /api/v1/rp/background/materialize` with the same world's authorized creator credential. This checks the caller's active `world.cohort.materialize` grant on the source Cohort; ordinary player control is insufficient.
3. Existing Observe/turn/wait APIs see the initialized individual. No separate character card, player session or clone is created for that NPC.

Background request fields: `principal_id` (bound to authentication), `instance_id`, `branch_id`, `entity_id`, `expected_head`, `idempotency_key`, `age_min`, `age_max`, `residence_place_id`, `initial_place_id`, and `schedule` containing `{world_time, place_id, activity_code}` entries. A `work` entry additionally requires `employment_contract_id`; other activities must omit it.

The age range is a newly authorized minimal fact **at initialization world time**, not a secretly discovered date of birth. Name/identity/materialization come from existing facts. Residence references an active existing home; sharing a place does not invent a family relationship, tenancy contract or ownership. Existing employment is read through Life Context, never guessed from the Cohort name. Temperament reuses the versioned materialization seed.

Routine bounds: 1–16 chronological movements, within the next seven world days; every place must exist in the same branch, and each transition requires an existing route. Home entries must use the declared residence. No-op movement and conflicting existing profiles/backgrounds are rejected. Supported activities: `home`, `present`, `lunch`, `work`. Work requires a work-kind place and the individual's actual contract effective at that appointment time. The Event defines the person's initial work appointment/site, not ownership of the building or a new job. Actual wage participation supplies organization, wage and source Event. `initial_employment` is historical provenance; current employment and organizational relationship are derived dynamically in Life Context.

The existing scheduler only executes the bounded M2 instance/branch, and this endpoint explicitly retains that restriction. Generated routine items use the same world-day calendar as RP Wait; future appointments do not reset the day counter.

The immutable `RPBackgroundMaterialized` Event and existing profile/position/movement/scheduler rows commit atomically. Exact request/key retry returns the same background with `replayed=true`. A failed second command leaves a valid conserved materialized entity; retry initialization, do not rematerialize its population. Redefinition with another key is rejected. Pending turns/waits and stale head are rejected.

Own background is available only in the permitted decision Life Context. Public scene Observation does not disclose age/residence or other private background. Background initialization emits no unfiltered Outbox payload. Existing utterance/observation rules govern what other characters actually learn.

## Evidence so far

- Focused core/storage/HTTP PASS, including creator/player boundary, conflicting retry/redefinition, precommit rollback, fresh individual entering RP, schedule departure/re-encounter, and exact Life Context equality after reopen/rebuild.
- `npm run build` PASS (typecheck and production build).
- `npm run verify:rp2-emergent` PASS: real HTTP Cohort allocation and background command → browser sees Nora → conversation → scheduled departure/return → conversation → process/browser restart → identical background retry. Artifacts `/tmp/corerp-rp1-e2e-AldgkH`.
- Browser creator credential exists only in the test runner/server environment, never browser settings/storage. The test world is temporary, not production.
- Actual employment/organization/calendar test PASS: a conserved named-worker split supplies the contract, wrong contract is rejected, work schedule moves the character on day2, actual employment and employee relation retain source lineage, and replay matches. This supplemental test supplies only route topology directly as a fixture; wages/population/employment use the real command path. The separate browser case uses real route preparation and all runtime commands.
- Full C Go/vet/relevant race, current-code browser rerun, build/typecheck and configured verifiers PASS; see the phase report. RP-2D–E and later milestones remain pending.
