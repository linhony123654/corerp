# F10 — World QA & Observer: Design and Acceptance Ledger

Status: **DEFINE/DESIGN**.

## Scope and Objectives

1. **World QA (Read-Only Simulation Health Diagnostics)**:
   - Provide comprehensive, automated health checks on a running world to detect "tests pass, but simulation has degraded" (e.g. population collapse, economic deadlock, relationship isolation, spatial clustering, runaway event growth, knowledge leaks).
   - Strict read-only semantics: queries committed immutable events, balances, and projections; emits zero new events and writes zero tables. It never forms a second source of world truth.
   - Computes 14 mandatory diagnostic categories:
     1. Population & Cohort Materialization (materialized entities, cohort sizes, active residents)
     2. Employment & Vacancies (active contracts, unfilled postings, applicant queue)
     3. Financial Flows & Arrears (total currency balances, payroll liabilities, unpaid obligations)
     4. Household Rent Pressure (covered vs strained vs critical distributions)
     5. Housing Coverage (residence coverage ratio, unhoused population)
     6. Commute & Transit Topology (disconnected places, anomalous edge traversals)
     7. Relationship Network (graph density, clustering, isolated entities with 0 ties)
     8. Event Density & Repetition (event frequencies, duplicate actions over world time)
     9. Agent Decision Distribution (actions by actor, decision provider ratios)
     10. Knowledge Leakage Indicators (unauthorized delivery of private information)
     11. Organization Decisions (recruitment freeze vs unfreeze vs expansion ratios)
     12. Failed / Rejected Actions (declined offers, lease defaults, arbitration disputes)
     13. Unreachable Spatial Locations (islands without inbound/outbound links)
     14. Economic Conservation (double-entry zero-sum balance and issuance consistency)
   - Evaluates configurable anomaly thresholds and flags warnings/critical anomalies without automatically mutating or forcibly correcting the world state.

2. **Observer Mode (Non-Controlling Player & Creator Perspective)**:
   - Read-only timeline and digest generation for observers who do not pilot a specific actor.
   - Structured summaries:
     - World Timeline (macro events: laws, org decisions, cohort transitions)
     - Entity Life Summary (work, residence, relationships, recent decisions)
     - Organization Summary (headcount, financials, active postings, recent reviews)
     - Relationship Change Summary (new ties, status changes)
     - World Digest (periodic macro summaries over specified world-time windows)
   - Every summarized item provides a direct link to the Studio Inspector (`InspectorEventID`).
   - Strict authorization filtering: observers see only facts permitted by their granted principal access level (creator vs operator vs public/observer).

3. **Representative Long-Run Validation**:
   - Run a multi-day simulation and execute World QA & Observer queries.
   - Verify that:
     - Residents do not degenerate into clustering in a single room/place.
     - Economic accounts and wages remain conserved and diversified.
     - Knowledge boundaries are preserved.
     - Observer summaries reflect true historical events without rewriting or fabricating history.
