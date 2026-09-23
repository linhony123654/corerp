# RP-2B — Life Context and sourced relationships

Status: PASS locally, 2026-09-23. Schema025, no migration. RP-2C and later remain pending.

## Reuse and behavior

Reused own materialization/account lineage, actual employment/rent contracts, agent schedules/movements, immutable Events, observation_records/Knowledge, posted journals, authenticated RP sessions, recovery and participant Outbox. No second character, memory, relationship or economic authority was introduced.

Core provides versioned stable temperament/values and pure derived Needs/Goals/relationship rules. Storage builds an NPC-scoped Life Context and commits explicit interpersonal actions. HTTP exposes authenticated social actions; Play reads committed action descriptions through its existing history renderer. See [contracts](README.md#life-context-and-interpersonal-actions).

## Acceptance evidence

- Economy → need/goal → decision: Cai gives 350 of 400 minor units through a real conserved transfer; pressure is sourced to the posted Event, and the same chat now yields refusal. Rollback leaves funds unchanged; identical retry has one effect. Replay matches balances.
- Schedule → decision: sourced work commitment creates a conflicting goal; existing actual browser journey verifies work-driven refusal and scheduler movement.
- Relationship/memory → later choice: three observed insults create tension, survive SQLite reopen, and cause actual NPC movement rather than different prose. Browser verifies absence and remembered history after a second restart.
- Promise → later choice: actual meeting promise creates signed obligation and waiting behavior. Early fulfillment fails; actual world-time advancement permits fulfillment only with correct co-presence. Duplicate fulfillment fails and replay preserves relationship evidence.
- Privacy and identity: input includes only the NPC's own economics and observed experiences; a speaker claiming wealth does not alter assets or become factual knowledge of wealth. Public Observation excludes temperament, debt and relationship axes. No employment is invented from the source Cohort.

## Verification

- Focused core/storage/HTTP tests PASS (storage4.779s, HTTP0.451s).
- Typecheck and frontend production build PASS.
- Browser life scenario PASS: `/tmp/corerp-rp1-e2e-ixVaxb`, real backend/SQLite/browser, conflict-driven movement and two restarts.
- Browser provider fixture PASS: `/tmp/corerp-rp1-e2e-7aI66o`, 8 local HTTP model calls; recovery adds no Event/observation/utterance rows (`32:30:8`) or calls.
- M0 (52 checks), M1 evidence, M2 (26 executable references), `git diff --check` PASS.
- Full `go test ./... -count=1` PASS (storage61.607s, HTTP4.558s); `go vet ./...` PASS. Related race PASS (core1.025s, decision1.441s, storage77.539s, HTTP13.097s). Existing migration/reopen/replay checks included in full tests; new Life Context/social tests explicitly exercise reopen and projection rebuild.
- Unfiltered server/CLI race PASS (server3.038s, CLI47.084s), including existing multi-day economy/routine checks.
- Live LLM remains REQUIRED_IF_AVAILABLE / NOT VERIFIED, no configured endpoint/model/key. Local fake HTTP is not live-model evidence.

## Resolved / limitations / recovery

Rebuild tests exposed that balance projection sequence is not immutable causal provenance; economic sources now come from actual posted journal Events. Negative liability ledger balances are rendered as positive obligations. Reopen and rebuild equality tests retain those protections.

Temperament and relationships are intentionally small deterministic policy views; they are not psychological measurements. Meeting promises do not yet expire or impose automatic penalties. Broader biography/materialization, style and long-run emergence are RP-2C–E; career/culture/UI productization stay in their specified later stages. No external deployment or accounts used.

Recovery point before B: `328aead` (RP-2A). Stage checkpoint will contain this report once full verification passes; no schema rollback required.
