# F12 — Integrated Long-Run & Engineering Evidence: Design and Acceptance Ledger

Status: **LOCAL ACCEPTANCE PASS**. Live Provider Status: `IMPLEMENTATION_COMPLETE / LIVE_VALIDATION_PENDING`.

## Scope and Objectives

1. **Integrated Real-World Validation Across All Mechanisms**:
   - Construct a small but comprehensive integrated world exercising all CoreRP architectural mechanisms simultaneously without relying solely on isolated unit tests.
   - Combine subsystems from F0 through F11:
     - **Author DLC Packs (F9)**: Real Content DLC pack (Retail Career catalog) and Sourced Narrative pack (Life Journaling) loaded and active.
     - **Multi-Controller Model Residency (F3, F7)**: 1 Human player (`M2RPPlayerPrincipal`) + 2 external MCP resident service controllers (`principal_mcp_resident_1`, `principal_mcp_resident_2` enrolled via `EnrollRPExternalControllerLocal`).
     - **Core NPCs**: Ada, Bo, Cai, Lin, Nora, Eli.
     - **Spatial Topology & Real Encounters (F2, F10)**: Discrete places (residence, workplace, cafe, retail store) linked by directed timed transit edges with intermediate encounter segments (`街区林荫道`). Anti-clustering verified across places.
     - **Households & Housing Pressure (F4)**: Multiple households (`household_f12_bo_ada`), shared rent lease agreements (`AgreeRPHouseholdRentLocal`), shared liabilities, and rent pressure monitoring.
     - **Education, Skills & Credentials (F6)**: Training programs, qualification assessments, credential issuance, job application gating, interviews, evaluations, and career hiring.
     - **Health, Sleep & Work Performance (F5)**: Sleep cycles, sleep debt accumulation, fatigue altering work task outcomes (`recheck_required`), and full rest recovery.
     - **Information Network & Communication Channels (F7)**: Direct messages with scheduled latency, organization layoff notices, public notices of enacted laws, recipient belief/doubt stances, and strict epistemic containment with zero information leakage.
     - **Organization Agency (F8)**: Agency operational policies, reserve target monitoring, payroll liabilities, and automated recruitment freeze/unfreeze reviews.
     - **Multi-Day Timeline & Multiple Process Restarts (F11)**: Simulation advancing 30+ world days, multiple process exit and reopen cycles, verifying zero projection drift (`CompareProjections`) and flawless historical replay (`RebuildProjections`).
     - **Simulation Health Diagnostics & World QA (F10)**: Automated read-only audit across all 14 dimensions:
       1. Population & Cohorts
       2. Employment & Vacancies
       3. Financial Flows & Arrears
       4. Household Rent Pressure
       5. Housing Coverage
       6. Commute & Transit Topology
       7. Relationship Network & Isolated Nodes
       8. Event Density & Repetition
       9. Agent Decision Distribution
       10. Knowledge Leakage Indicators (strictly 0)
       11. Organization Decisions
       12. Failed / Rejected Actions
       13. Spatial Reachability (0 orphan nodes)
       14. Economic Conservation (`DoubleEntryBalanceZeroSum == 0`)
     - **Observer Mode & Studio Inspector (F10)**: Structured read-only perspectives (Macro, Entity, Organization, Relationship, Digest) with direct canonical links (`/studio/inspect?...`) and rule epoch provenance verification (`EpochID`, `RulesetHash`).

2. **Strict Verification Tiers & Separation**:
   - **UNIT / DETERMINISTIC**: Rules, permissions, idempotency, state replay, projection rebuilds. (Verified PASS).
   - **REAL HOST / FIXTURE PROVIDER**: Real Play UI, Vue frontend build & typecheck, SillyTavern client tests, MCP stdio runtime, authenticated HTTP API routes. (Verified PASS).
   - **LIVE PROVIDER**: External LLM live calls. As no dedicated live project provider key is configured in this environment, this tier is marked `IMPLEMENTATION_COMPLETE / LIVE_VALIDATION_PENDING` without falsely asserting live completion.

3. **Complete Engineering Repository Verification**:
   - `go test` fast packages (`core`, `decision`, `narrative`, `httpapi` 54/54 tests) PASS.
   - `go vet ./...` clean with zero warnings.
   - Targeted race detection on F11/F12 suites PASS.
   - Frontend typecheck (`vue-tsc --noEmit`) and build (`vite build`) PASS.
   - MCP client test suite (`npm test` in `clients/mcp`) 4/4 PASS.
   - SillyTavern client test suite (`node --test client.test.js`) 5/5 PASS.
   - Single-source static manual build (`backend/cmd/manual-gen`) and 9-step tutorial reproduction test (`TestF11TutorialReproduction`) PASS.
