# Living-world progress

## 2026-09-23 — goal start
- Read full new goal and verified clean baseline `e1aa4c7`. Previous goal complete; this new goal active, no budget. No live earlier process or uncommitted work.
- Reused coding-workflow; read planning/validation modules and provider call/config symbols. Began only RP-2A recon. Prior goal turn classified progress (completed RP-1 and verified state), not a wait/blocker.
- Created isolated durable plan preserving RP-2 through Final Integration scope. No business implementation yet.

## Verification
- Additional unfiltered server/CLI race PASS (3.038s/47.084s). All B required checks passed; phase report and contracts finalized. Creating authorized B checkpoint; next action is C evidence/materialization recon.
- B final full Go PASS (storage61.607s, HTTP4.558s), vet PASS, relevant race PASS (core1.025s, decision1.441s, storage77.539s, HTTP13.097s). CLI/server names do not match the RP test filter; running those packages without filter for explicit additional race coverage. Phase report written. B checkpoint next, then C read-only recon. Overall goal remains active.
- B continuation: recovered goal and actual worktree; browser life scenario PASS (`/tmp/corerp-rp1-e2e-ixVaxb`), including actual conflict-driven NPC departure and second process/browser restart; provider fixture PASS (`/tmp/corerp-rp1-e2e-7aI66o`, 8 calls). Build/typecheck and M0/M1/M2 verifiers PASS. Focused core/storage/HTTP tests PASS before continuation. Full Go/vet and related race underway.
- B actions implemented: greet/insult/apologize, conserved gift transfers, explicit meeting promise and evidence-checked fulfillment. Same transaction writes Event, participants' observations/Knowledge, optional journal, audit and Outbox. Relationships/memories/commitments are derived views, not new authority. Tests cover rollback, idempotency, stale cursor, presence, funds, fulfillment timing/duplicates, recovery/replay and private-state exclusion.
- Continuation race command first referenced nonexistent `cmd/corerp`; it failed before tests. Enumerated actual commands and corrected to `cmd/corerp-m2`; no code defect or passing-test claim from failed invocation.
- RP-2A checkpoint `328aead`, clean tree confirmed before B. B representative Life Context/Need/Goal/disposition and repeat-contact memory code underway; no later stage implemented.
- B test found life economic source IDs changed after projection rebuild: `account_balances.last_event_sequence` is rebuild metadata, not the causal posting event. Replaced source lookup with immutable posted journal entries. Debt accounts use negative ledger balances; life view converts to positive obligations. Rebuild/reopen equality test retained.
- B setup errors corrected: one atomic patch anchor had spacing mismatch; test initially used two returns for RebuildProjections (actually returns error only). No claim of full B gate yet.
- RP-2A final full Go PASS (storage58.441s, HTTP4.921s), vet PASS, related race PASS (storage68.848s, HTTP12.039s, decision1.441s, server3.057s, CLI5.039s). M0/M1/M2 verifiers and diff check PASS. Phase report updated; authorized checkpoint then RP-2B recon. Entire living-world goal remains active.
- RP-2A focused adapter/config/storage integration PASS. Initial storage test incorrectly required optional next_schedule for Cai; checked fixture and corrected assertion to actual current activity/own assets.
- Frontend typecheck/build PASS; actual browser/backend restart E2E PASS for configured local model HTTP fixture (`/tmp/corerp-rp1-e2e-fsaaVS`,8 calls) and deterministic (`/tmp/corerp-rp1-e2e-kg9UAl`,0 calls). Both keep `32:30:8` world counts during recovery; committed model calls not repeated. Final Go/vet/race running.
- Adapter first test run: storage decision tests PASS; HTTP timeout test fixture blocked its own httptest.Close waiting on an undrained request. Verified via SIGQUIT stack of the owned test process; added explicit fixture release channel. Client timeout had already returned correctly. Focused rerun next.
- Baseline Git: PASS (clean main, checkpoint `e1aa4c7`).
- New goal tests: NOT VERIFIED (implementation not begun).
