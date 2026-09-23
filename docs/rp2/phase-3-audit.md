# RP-3 acceptance audit

Status: original RP-3A/B/C implementation, requirement audit and local verification PASS. Checkpoint/clean-tree transition is recorded in the active plan. This does not complete RP-4–8 or Final Integration.

## Recruitment and lifecycle

All test references below are in `backend/internal/storage`, unless noted. They ran in the current full uncached Go suite; final stage evidence is recorded below.

| Required behavior | Direct test coverage |
| --- | --- |
| Organization, vacancy, posting, discovery, application | `career_recruitment_test.go`: organization/posting/application recovery, authorization and competing-head tests |
| Referral | `career_referral_test.go`: known candidate evidence and candidate's own application required |
| Qualification, interview, evaluation, offer, rejection | `career_hiring_test.go`: interview qualifications, offer decline/recovery; private evaluation and candidate/manager authority separation |
| Acceptance, onboarding, probation, schedule, wages | `career_employment_test.go`: real work/private payroll, actual appointments, capacity and aggregate-employment overlap guards |
| Regular employment, performance evidence | `career_performance_test.go`: actual work evidence, elapsed probation plus explicit authorized regularization; time alone does not regularize |
| Attendance | `career_attendance_test.go`: actual RP departure/activity affects attendance and atomic accrual |
| Leave | `career_leave_test.go`: approved leave cancels owned work while preserving agreed pay; rejection does not reserve/cancel work |
| Overtime | `career_overtime_test.go`: consent/conflicts/cancellation/leave and actual work/pay/recovery |
| Raise | `career_raise_test.go`: effective boundary and preservation of earned wages/arrears |
| Transfer, promotion, demotion | `career_position_test.go`, `career_position_accept_test.go`: grade direction, qualifications, vacancy reservation, explicit acceptance, actual activation, stale review/revoked manager rejection |
| Resignation, termination, layoff, unemployment, re-employment | `career_exit_test.go`: each exit kind, authority, final earned wage/OT, vacancy reopening, sourced unemployment/find-work goal, new recruitment/contract |
| Existing aggregate workers can actually leave | `career_aggregate_exit_test.go`: notice versus activation, preserved final claims, independent re-employment, demographic conservation, all-worker exit and scheduler recovery |

Occupation, position and declared grade remain distinct; `career_grades_test.go` validates explicit scale authority/compatibility. Promotion does not assign social standing or automatically grant independent manager authority. Position acceptance rejects superseded performance evidence; elapsed effort is not promotion authority.

## Career ↔ RP

| Requirement | Evidence and remaining boundary |
| --- | --- |
| Work affects invitations | `TestRPLifeSixtyTurnsAcrossFiveDaysWithEconomyMemoryAndRecovery` asserts a sourced work schedule changes invitation response to refusal; current browser life regression also passes. This is existing life/schedule integration, not a new career UI test. |
| Unemployment affects economy/goals | Exit tests preserve final wages/arrears, stop future accrual/work and assert sourced `find_work`; aggregate tests additionally exercise real next-job pay and no resurrected wages after demographic return. |
| Promotion changes permissions/wage/social knowledge | Position activation tests check old/new pay; `career_authority_test.go` covers role permission activation/revocation; announcement tests check actual co-located hearing, no retroactive knowledge for late arrivals, no private wage/evaluation disclosure and rebuild. |
| Boss relationship affects a career decision | `TestCareerRelationshipChangesActualOvertimeChoice`: actual gift/conflict changes optional overtime consent, work and wages; employee evidence remains private, rollback/retry/reopen covered. |
| Coworker relationship affects a career decision | `TestCareerKnownCoworkerRelationshipChangesActualOvertime`: actual Ada/Bo recruitment, unheard announcement exclusion, real gift/relationship, heard same-organization identity, accepted overtime, later conflict/decline, actual pay26 versus12, manager-private evidence exclusion, rollback/retry/restart/rebuild. Focused, expanded Career/RPLife/RPSocial regression and related race PASS. This is a local voluntary-work decision based on known workplace relationships, not knowledge of a coworker's private overtime schedule. |
| Resignation creates recruitment vacancy | Exit test asserts reopened available capacity and actual subsequent recruitment. |
| Work experience enters Memory | Employment/performance/position/exit tests assert own source-backed memories; announcement tests separately assert only witnessed attributed speech enters listeners' memory. |

AI participation in recruitment evaluation is optional in the original text. The current overtime `consider` path is an explicit versioned local fallback; it is neither a remote model invocation nor an unattended decision trigger. Do not describe it as either.

## Current verification

Latest evidence (2026-09-24):

| Check | Result |
| --- | --- |
| Full uncached `go test ./... -count=1`, then `go vet ./...` | PASS; storage172.752s, HTTP11.553s. Includes migrations, long-run life, replay/rebuild and all career tests. |
| Corrected server-test package | Normal PASS0.126s; `go test -race ./cmd/corerp-server -count=10 -timeout=5m` PASS15.913s; package vet PASS. |
| Same five-day60-turn race test after memory query restriction | PASS266.141s, versus271.060s before; no material speedup established. |
| Full race coverage with `-count=1 -timeout=60m -json`, plus corrected server package rerun | PASS by package: storage2832.907s, HTTP120.628s, CLI74.227s, core1.087s, decision1.543s; corrected server ten-repeat race15.913s above. Storage inventory reconciled172/172 top-level passes, none missing and no skipped tests. Original full command exited1 solely for the old server test; its failure is retained, not mislabeled as zero-exit success. Log: `/tmp/corerp-rp3-final-race-VyQI3c/results.jsonl`. |
| Frontend typecheck/build and M0/M1/M2 configured verifiers | PASS; frontend/build inputs unchanged since these checks. Metadata verifiers do not substitute for RP-3 acceptance. |
| Current browser `verify:rp2-life` | PASS; `/tmp/corerp-rp1-e2e-vmWpfj`, zero model calls, process/browser recovery counts39:30:8, actual schedule refusal and relationship-driven movement. |
| Current browser `verify:rp2-provider` | PASS; `/tmp/corerp-rp1-e2e-r6Zksj`,15 calls to a local HTTP fixture, recovery counts39:30:8. Not remote model verification. |
| Emergent background browser regression | PASS; `/tmp/corerp-rp1-e2e-RlMqJZ`; predates the final memory-domain filter, which did not change background generation. |

Failure history is retained, not converted to passes:

- First full race timed out at the default10m package deadline (storage600.025s).
- Second full race timed out at30m (storage1800.020s), with122/172 storage top-level tests passed; the incomplete50 were not counted as passing. Evidence: `/tmp/corerp-rp3-race-AYIfYQ/results.jsonl`, matching normal/race inventories in that directory.
- The timeout stack prompted an explicit `RPCareerFactRecorded` filter in the accepted-employment memory query. Its behavior is revalidated by the current normal suite and the same long-run race test. A single266s versus271s comparison does not prove the suite runtime problem is solved.
- The60m run exposed a server test that cancelled startup after a fixed2s, interrupting SQLite fixture seeding. Only the test was changed: observe initialization/serve entry, cancel afterward, independently bound shutdown and require its completion log. Production startup/shutdown is unchanged; ten race repetitions pass. A pre-listen log is not claimed to prove a bound network socket.

Earlier increment-specific evidence remains in `phase-3.md` and the planning progress log. Endpoint/model/key are unconfigured (presence-only check), so remote-live verification remains REQUIRED_IF_AVAILABLE / NOT VERIFIED. No new external credentials or production access are required for the remaining local race gate.

No required RP-3 behavior remains unimplemented or unverified in this audit. Next: authorized checkpoint, verify clean tree, then RP-4 read-only recon. Remote-live verification remains conditional and unavailable as disclosed; production publication is neither requested nor performed.
