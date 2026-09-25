# F6 local phase report — education, evidence and qualification

Status: **local acceptance PASS**. Based on F5 checkpoint
`096b386e4ed2d82cdd76b749fa20396bb1d205cc` in the independent
`f6-education` worktree. The containing scoped commit is the F6 checkpoint;
no push or deployment is included.

## Implemented

- Private Event-backed program → enrollment → timed learner exercise →
  issuer-assessed completion/proficiency → expiring credential → issuer
  revocation. Program definition is local-operator-only and pins an active
  same-branch issuer. Fresh commands use branch head, exact idempotency and
  F3 shared-round guards; administrative actions do not advance time.
- New Career postings may opt into separate `required_credentials` by code and
  issuer. Application, offer and acceptance recheck source lineage and current
  validity. Existing manager `required_qualifications` judgments are unchanged.
- Own-learner qualification and program-discovery reads; the former shows
  bounded training/proficiency, credential state and actual F5 work-task
  experience without private fatigue/condition causes. HTTP command receipts
  omit raw evidence Event IDs and exercise answers. Source IDs remain internal
  to storage Events. No Education/Career tools were added to the RP-only MCP
  client.

## Verification evidence

- `go test ./internal/storage -run '^TestEducationCredentialUnlocksSourcedCareerApplication$' -count=1 -timeout=5m`: PASS on final source as of the source-chain validator change. Covers missing credential, actual world-time wait, premature exercise/issuance, wrong learner/issuer, prerequisite denial, stale head, mismatched key, injected rollback, valid application/offer, issuer revocation, offer/acceptance denial, expiry boundary, restart and exact replay, Compare/Rebuild and test-only corrupted credential lineage.
- `go test ./internal/storage ./internal/transport/httpapi -run '^TestEducation' -count=1 -timeout=5m`: PASS on current source. HTTP fixture covers bearer binding, program discovery, no private Event IDs/answer in receipts, wrong subject denial and real Career application after training.
- Existing F5 work-task fixture extended to check source-backed experience in the new own query without health causes; focused two-test run PASS1.228s before later source changes.
- A separate F6/F3 integration fixture verifies an active Human/external shared round refuses fresh education program/enrollment Events while allowing exact pre-round replay. Normal and race runs PASS0.430s/8.489s. This test was added after the all-package binary started, so it is separate evidence rather than part of that binary's result.
- A separate real-world-clock expiry fixture shows valid application/interview/evaluation before credential expiry, followed by source-derived `expired` qualification status and refusal to issue an offer after expiry. Focused normal PASS0.468s; the combined education race set (including this and shared-round tests) PASS26.467s storage / 10.234s HTTP.
- Final-source `/usr/local/go/bin/go test ./... -count=1 -timeout=40m` PASS: storage1421.768s, HTTP24.577s, all other packages pass. The broad binary started before the two later test-only shared-round and expiry fixtures; those fixtures have separate normal/race PASS evidence above. No product Go source changed during the broad run.
- Final-source `go vet ./internal/core ./internal/storage ./internal/transport/httpapi`, `git diff --check`, and an empty `gofmt -l` over all changed Go files PASS after the broad run.

## Limitations and deferred work

- Final-source HTTP all-tests PASS24.000s; focused storage/HTTP education race
  PASS11.295s/10.280s, plus the later combined education race set
  PASS26.467s/10.234s. The F6 local behavior and regression gate passes.
- The own qualification read returns at most 100 most recent rows per
  training, credential and work category, without a continuation cursor;
  this is a bounded recent view, not an exhaustive lifetime transcript. Career
  checks source credentials independently by requirement and do not rely on
  this display window. Pagination remains a documented improvement.
- Program authority is an explicitly local F6 grant, not F8 organization
  governance. Education is a bounded enrollment/exercise/assessment workflow,
  not a physical school timetable. Expiry is source-derived at the world
  clock; a separate full-world application-before-expiry/offer-after-expiry
  fixture now passes.
- No production or external-provider integration was requested or exercised.
