# F5 design — sourced body state, not a second character script

Status: **F5 local acceptance PASS — scoped checkpoint in the containing commit**. F4 baseline is the clean,
recoverable checkpoint `7b9096f1ef2dd621ec42b4ec24081a1208d1f951`.
See `phase-report.md` for final-source evidence and the bounded MCP work
execution coverage limit.

## Reuse and boundaries

- `WaitRP` advances the shared world clock through AgentLife. It does **not**
  prove that an individual slept. Existing Background schedules do not accept
  `sleep`, and `home` is only a location/activity label, not restorative rest.
- `agent_movements` and same-place `AgentActivityStarted` Events are the
  authoritative activity history. Career attendance derives completed work
  seconds from these Events; approved leave is a separate employee request and
  manager decision. Attendance is not a work-quality or diagnosis record.
- `RPLifeContext` is serialized into replaceable provider input. An actor may
  perceive their own fatigue or symptoms; another actor may know only observed
  symptoms or a voluntarily disclosed claim. A private condition Event, exact
  severity, source ID or formal diagnosis must not appear in another actor's
  context merely because they share a place, job or household.
- Use immutable, branch-scoped Events and existing idempotency/control fences
  for new health transitions. Do not create fixed Sims-like need bars, rewrite
  old Events, infer illness from a leave reason, or let a health view move money
  or grant leave.

## Observable acceptance

1. An existing individual has source-backed condition kind/status, onset,
   bounded severity/functional impact, persistence, progression/recovery and
   optional treatment reference. Truth, observable symptom, subject knowledge
   and formal diagnosis remain separate. Exact command retries and database
   reopen preserve the same sources and derived state.
2. Two consecutive nights with explicitly sourced short sleep produce sleep
   debt and fatigue; an evening social decision changes its actual proposal
   and reason. A later, sufficiently long uninterrupted sleep lowers the
   derived debt/fatigue. Advancing time or being at home alone cannot do so.
3. The fatigued individual's next work window yields source-backed performance
   evidence distinct from attendance and wages; it does not silently reduce
   earned pay. A minor illness affects an authorized leave/work choice.
4. Boss/coworker reads contain only lawfully observed or disclosed symptoms and
   leave/attendance facts, not private health truth or an unearned diagnosis.
   The actor's model input is bounded to their own perceived functional state.
5. Interruption, wrong controller, stale head, duplicate/mismatched key,
   cross-branch access, restart and source/replay consistency are tested in
   disposable worlds. F3 shared-round and client paths must not be bypassed by
   new direct health actions.

## Ordered implementation

1. **Sleep evidence and fatigue vertical slice.** Source a current actor's
   sleep start/end at real world times under exact RP controller authority.
   The end records effective rest only up to the first sourced movement or
   other activity interruption, with a bounded maximum per interval. Derive
   sleep debt/fatigue from recent completed intervals without storing a
   mutable meter. A short two-night fixture proves pre/post decision behavior
   and long-sleep recovery. Model input carries subjective fatigue/functional
   limitation, not raw sleep Event IDs or exact private duration.
2. **Work integration — focused PASS.** Source a bounded, actual employee work-task attempt
   while a real employment shift is active; derive its task outcome from the
   actor's functional state at the attempt time. The task result is separate
   from attendance, manager judgment and wage accrual. An authorized manager
   may see an observed task result, never the private sleep source or diagnosis.
   The day-1/day-2 disposable fixture verifies the next-day outcome path and
   unchanged full attendance/base wage after accrual. The current exemplar
   task code is `routine_check`, with at most one sample per contract/day;
   `completed` versus `recheck_required` is a simulation rule, not a clinical
   prediction or manager evaluation. Severity-2 illness also yields a bounded
   `recheck_required` result, while severity-3 injury prevents the task until
   sourced recovery. Its internal manager read omits private fatigue, illness
   and sleep evidence. Source comparison and Human-gated shared work ingress
   pass focused tests; the final all-package regression also passes.
3. **Minor condition lifecycle — first source slice focused PASS.** Add bounded illness/injury onset, severity,
   symptoms, persistence, progression and recovery sources with optional
   treatment reference. A subject may perceive symptoms without a diagnosis;
   only an authorized diagnosis source can establish formal diagnosis.
   The current local-operator private Event path sources active→recovering→
   resolved minor conditions and lets the subject perceive only a symptom and
   functional impact. There is no diagnosis or treatment command yet; a
   treatment ID is refused unless its separate matching source exists. The
   illness→leave/work choice and lawful third-party symptom channel are now
   exercised through the existing Career and speech/context paths.
4. **Leave and lawful knowledge — focused PASS.** Link illness to the existing employee leave
   request/review path without making the diagnosis automatically visible.
   Boss/coworker visibility comes from observation or explicit disclosure,
   not a private condition table. Test work-versus-leave choice and privacy.
   The real fixture tests both branches: ill Ada works with a worse bounded
   task result and unchanged wages, or requests one day off with her own
   symptom-only reason, which Bo approves. The coworker cannot read private
   leave/attendance; an actual co-located speech claim can be heard through
   existing `SpeakRP`/`ReadRPContext`, without revealing condition truth or
   a formal diagnosis. A severity-3 injury prevents the sample task until
   sourced recovery. This is a local behavioral slice, not a full medical
   simulation.
5. **F3/client and stage gate.** Integrate health actions with shared action
   rounds where external controllers participate; verify actual MCP/HTTP
   boundary if exposed. Run relevant race, migration/replay, RP, Career,
   transport and full Go regression. Write phase report, then checkpoint.

   Shared ingress implementation: preserve the F3 round's one-action selection and
   Human wait priority. Extend its typed private proposal schema with
   `sleep_start`, `sleep_end` and `work_task`. A selected child uses the existing `StartRPSleep`,
   `EndRPSleep` or `AttemptRPWorkTask` Event owner with a round-derived key;
   the child verifies its pinned kind, actor, baseline head and exact request
   hash within its write transaction. A direct health request remains blocked
   in an external-controller world. The settled receipt names only the typed
   Event sequence/time and own disposition, never sleep or condition truth.
   Migration 046 widens the 040/041 SQLite CHECK constraints by rebuilding the
   round, participant and proposal FK chain transactionally while preserving
   old wait/speech/move receipts and pending proposals.
   The MCP sleep route has passed full stdio/Runtime integration; shared work
   has a focused storage recovery test. A populated 045→046 migration fixture
   preserves a settled speech receipt and pending move proposal with no FK
   violation. The authenticated HTTP work-task path passes through a real
   employment shift and Human-gated round; MCP has schema/routing checks but
   not a separate employed-resident work-task execution fixture.

The first slice is intentionally an actual action→Event→world-time→own-state→
decision→recovery path. The later work, condition, lawful-knowledge and shared
ingress increments complete the bounded F5 contract described here.

Focused `/usr/local/go/bin/go test ./internal/storage -run '^TestRPSleep' -count=1`
passes for the sleep slice, including restart/replay, authorization, rollback,
zero-time and activity-interruption cases. A real player invitation after two
short nights now passes through `SpeakRP` → `DecideRP` → `CommitRPDecision`:
Ada's rest-driven refusal is an authoritative, replay-consistent speech Event.
The same test confirms an active player-controlled Ada cannot be driven by the
internal NPC provider. The sleep source ID stays out of the actual provider
input. Shared sleep ingress is implemented and MCP-tested. Sleep thresholds are game
mechanics, not a clinical standard.
