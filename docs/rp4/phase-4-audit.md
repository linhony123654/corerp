# RP-4 acceptance audit

Status: RP-4 local acceptance PASS; checkpoint `53a5ac3`, post-commit clean worktree verified before RP-5 recon. Overall goal still includes RP-5–8 and final300-turn/30-day integration.

## Original section5 traceability

| Requirement | Current implementation and inspected evidence | Boundary |
|---|---|---|
| values/norms/customs/taboos/rituals/status symbols/group identity | `core/rp_culture.go` complete bounded contract and validation; storage immutable definitions | Descriptive fields are not invented effects; executable norm currently gift evaluation. |
| subculture/transmission/conflict/evolution | `rp_culture_evolution_test.go`: pinned parent source, unknown parent denied, version CAS, actual transmission/adoption and retained opposing evaluations | No automatic inheritance or replacement of unseen beliefs. |
| world/region/community/organization/family/individual layers | territory, organization, family, affiliation, internalization storage tests; sourced scope authority and membership | Community implements the requested Class/Community layer; family is explicitly mutual chosen-family, not a genealogy/marriage system. |
| accept/partial/oppose/rebel | core culture tests and actual gift/stance choice tests | Membership is independent of belief; none grants control over another actor's stance. |
| Authority/Role/Norm/Procedure/Enforcement/Evolution reuse | existing ACL/Events/clock/ledger; dedicated sourced office transitions, projection repair; law proposals/versions/cases | No second state/clock/money authority. |
| proposal/enactment/effective time | institution and law tests: separate proposer/legislator, future effective time, actual WaitRP boundary | Current executable prohibition is speech, not a prose-interpreted arbitrary legal code. |
| violation/enforcement/dispute | actual deliberate speech, witnessed case, separate conserved fine, own receipts, filing, independent review/refund, explicit successor delivery | No automatic punishment; same recorder executes an un-enforced violation; pending review can be explicitly resubmitted. |
| amendment/repeal | real version chain, monotonic future effective dates, original-action-time adjudication, active repeal, restart | Repeal does not delete history or automatically refund past fines. |
| law existence != obedience/text != result | deterministic lawful silence and deliberately committed speech; no fine until authorized command; failed funds guards atomic | No ACL bypass by a provider. |
| different scoped institutions | explicit world/region installation, role holders, treasury, independent law IDs/terms/fines; pure tests isolate same law ID across institutions | Configurable declarations rather than a hardcoded universal rule. No new world-pack registry was invented. |
| same act, different social evaluation | `TestRPCultureGiftChangesRealNPCChoiceAndPreservesPrivateHistory`: same real gift in positive/negative culture worlds | Not just alternate prose. |
| identification affects choice | real respond/refuse decisions committed after culture evaluations and stance changes | Historical experience cannot be rewritten by later stance. |
| law affects lawful candidates | known/effective law context, actual timed decisions, post-repeal permission restored | Technical executable list remains distinct. |
| deliberate violation and consequences | actual speech→case→fine→dispute/review/recovery, plus Career speech and culture/law announcements | Speech decoder explicitly rejects private definitions/filings and silence. |
| no omniscient knowledge | offsite/unheard rejection, authored/heard sources, private own cases, role succession without automatic case/term knowledge | Formal role and received filing are independent. |
| changes gradually learned | actual ordered law announcements and culture transmission/adoption, unseen latest version excluded, old news cannot regress known law | Tests distinguish publication, learning and adoption. |

HTTP real-store coverage spans all ten authenticated write commands, own-case query, real player speech, fine, dispute, successor appointment/forward, refund and reopen/idempotent retry. Detailed accounting rollback/conservation and balance-boundary rejection are storage tests. Financial balance0/capacity injection proves guards, not an authoritative business-spending depletion scenario.

## Required stage verification

| Check | Current result |
|---|---|
| `go test ./... -count=1 -timeout=30m` then `go vet ./...` | PASS; session91916 exited0. Storage189.294s, HTTP13.695s; all other package passes collected earlier; full vet passed. |
| `go test -race ./... -count=1 -timeout=60m -json` | PASS; session66977 exited0 at2026-09-24T08:50:20+08.259/259 top-level tests passed, including187 storage tests; no failed/skipped tests or unfinished top-level runs. Storage2918.728s. cmd/corerp-m1 has no tests. Log `/tmp/corerp-rp4-race-0aIVPf/results.jsonl`. |
| Current migrations/reopen/replay | Increment tests and complete normal/race suites PASS. Schema026 unchanged. |
| `npm run build` (vue-tsc + Vite), verify:m0/m1-evidence/m2-evidence | PASS; build4.58s, M0 52 checks, bounded M1/M2 verifiers pass (not full M2 acceptance). Session26482 terminal. |
| Browser life/recovery and local model HTTP adapter | PASS; session89263 terminal. Life `/tmp/corerp-rp1-e2e-FsFfBr` (0 calls), local model `/tmp/corerp-rp1-e2e-Mtj3Ou` (15 calls); restart counts39:30:8 unchanged; mobile/error/privacy checks pass. Not live LLM. |
| Live external model | REQUIRED_IF_AVAILABLE; no new external credentials inspected or live model claim. |
| Diff/security/staging review | PASS; reviewed tracked integration diffs and all57 changed/untracked paths: intended source/tests/docs only, no DB/config/build artifacts. Scoped filename-only credential heuristic found no matches (not proof of absence of every possible secret). |
| Phase report/commit/clean tree | PASS; checkpoint53a5ac37be6305cccb590b70e9da2dedc621ceeb; post-commit git status empty, rechecked before RP-5 recon. No push/deploy. |

## Handoff gate

Full-stage test processes terminal PASS. Scoped staging/security review, checkpoint and clean-tree confirmation complete. RP-5 may proceed serially. No production deployment is requested.

Final race reconciliation:259 top-level runs have259 matching passes;187 are storage tests. All six tested packages passed; cmd/corerp-m1 reports no tests. No failure records, skipped individual tests or unfinished top-level runs. This replaces earlier partial snapshots; no duplicate race run was started.
