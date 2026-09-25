# F6 design — learning evidence, credentials and Career qualification

Status: **locally verified; containing scoped commit is the F6 checkpoint**. Baseline is the clean F5 checkpoint
`096b386e4ed2d82cdd76b749fa20396bb1d205cc` in the independent
`f6-education` worktree. No checkpoint, push or deployment has occurred.

## Source map and compatibility boundary

- A Career posting currently carries `RequiredQualifications []string`.
  `CareerQualificationAssessment` is explicitly a manager's organization-
  specific judgment based on an actual interview answer; it is **not** a
  portable skill, certificate or license. Existing application submission
  accepts a candidate's claim, while a failed manager assessment blocks an
  offer. Preserve this legacy organization-assessment path and historical
  Event payloads.
- Add a separate, opt-in `RequiredCredentials` list to new posting Events:
  each item names a qualification code and accepted issuer. It is additive to
  `RequiredQualifications`, never a silent reinterpretation of those strings.
  A credential requirement is checked from authoritative Education Events at
  application and again before offer/acceptance. A manager's `Passed: true`
  assessment or AI advice cannot substitute for a missing, expired or revoked
  credential. Already accepted legacy contracts are not retrospectively
  invalidated.
- Reuse the private Event command transaction, branch/head/idempotency fences,
  current candidate authorization and existing Career posting/application/
  offer ownership. Do not grant a trainee issuer authority, infer proficiency
  from a job title or application statement, or publish private learner
  records as general knowledge. F6 issuer registration is an explicitly local,
  program-scoped operator grant; full organization governance belongs to F8.
  Administrative enrollment/issuance does not advance world time. Any future
  externally controlled physical training action must respect F3 shared-round
  authority rather than becoming a direct clock or movement bypass.

## Observable F6 acceptance

1. A scoped, authorized issuer defines a bounded training program with a
   qualification code, prerequisites and completion evidence rule. A current
   learner enrolls. Completion is a later sourced Event referencing that
   enrollment and actual work/assessment evidence; simply waiting or stating
   “I trained” is insufficient. Exact retries and reopen preserve lineage.
2. Completion produces bounded skill/proficiency evidence. An authorized
   issuer may issue a separately sourced certificate/license only when its
   prerequisites and completion evidence are current; its validity and
   explicit revocation are source-derived. A private qualification query
   distinguishes training progress, proficiency evidence, experience and
   active credential, with issuer and expiry.
3. A real new posting requires one verified credential. The same candidate
   cannot enter its application flow before training, then can apply after
   sourced completion/issuance. Revocation or expiry before offer/acceptance
   blocks progress. Manager judgment still controls the existing interview
   evaluation; the credential is a prerequisite, not an automatic hire.
4. Wrong issuer, wrong learner, foreign branch, forged prerequisite/evidence,
   stale head, duplicate/mismatched key, interrupted transaction and replay
   corruption fail without granting a qualification. An unrelated Agent/model
   cannot enumerate another learner's private records or raw evidence IDs.
5. Experience evidence is sourced from actual work (the existing F5 bounded
   work-task Event or Career attendance), not a static résumé field. An
   authorized verifier may use only the bounded outcome/attendance view, not
   the task's private fatigue or illness causes. Its interpretation as a
   credential must pass issuer/prerequisite policy; a past job title alone
   is not enough.

## Ordered vertical slice and gates

1. Define the minimum program/enrollment/learner-exercise/completion/skill/
   certificate Event contracts and issuer capability. The program Event pins
   the local operator's chosen active issuer; a learner answer is separately
   sourced after enrollment and a bounded training interval; only that
   issuer's later reasoned assessment can complete the course. Prefer deriving current state from Events
   initially; add a migration/projection only if query scale or referential
   constraints require one. Source Event ID remains internal provenance, not
   the external model's generic qualification read.
2. Prove a disposable missing-credential→enroll→complete→issue→apply path at
   the real Career owner. Validate prerequisite, time/evidence, revocation and
   replay as the path grows. Keep the first program bounded (e.g.
   `safety_training`), not a complete school timetable.
3. Add role-scoped qualification query and the relevant authenticated HTTP /
   MCP boundary only if exposed. Verify fresh authority, privacy, restart,
   Compare/Rebuild, race and final Career/RP/all-package regressions. Write
   the phase report and checkpoint only after the F6 gate passes.

The design deliberately separates `training completed`, `skill/proficiency
evidence`, `experience evidence` and `credential valid now`. No one of these
claims automatically implies another.

## Implemented contract and current boundaries

- `RPEducationFactRecorded` private Events source program, enrollment,
  exercise, completion, credential and revocation. The local operator's
  program Event grants only its named, active same-branch issuer. Learner
  enrollment/exercise use current candidate control. Administrative commands
  neither move an actor nor advance world time and reject active F3 shared
  rounds. The exercise requires the program's minimum elapsed world time and
  a separately submitted answer; the issuer's later reasoned assessment pins
  its Event. Completion and issuance requests use business enrollment IDs;
  storage resolves and pins the exact source Event IDs internally.
- New posting `required_credentials` entries contain `{code, issuer_id}` and
  are independently checked at application, offer and acceptance. The
  historical `required_qualifications` manager-assessment path remains intact.
  A revoked or expired credential does not satisfy the new gate. Issuer
  completion and certificate issuance recheck program prerequisites.
- `ReadEducationQualification` is an own-learner query with bounded training,
  proficiency, credential validity and sourced F5 work-task outcomes. It does
  not return exercise answers, raw Event IDs, or fatigue/condition causes.
  `DiscoverEducationPrograms` allows the same current learner to page
  published program terms by credential code and issuer; no private learner
  records appear in that market read.
- Authenticated HTTP routes are `POST /api/v1/education/programs/define-local`,
  `/programs/query`, `/enrollments/start`, `/exercises/submit`,
  `/training/complete`, `/credentials/issue`, `/credentials/revoke`, and
  `/qualifications/own`. All command requests carry the existing career
  binding (`instance_id`, `branch_id`, `expected_head`, `idempotency_key`);
  the bearer principal is bound server-side. External command receipts expose
  only kind, business record ID, head sequence, world time and replay status.
  The internal `EducationRecord` with source IDs is not returned by HTTP.
  There is no MCP education tool yet; MCP currently exposes RP actions, not
  Career or Education commands.

Full all-package regression, focused race, HTTP authorization/restart,
source-corruption and active-shared-round negative checks pass, as detailed in
`phase-report.md`. The current qualification
query returns at most 100 recent rows per category and does not yet provide a
pagination cursor; do not interpret it as an exhaustive lifetime transcript.
