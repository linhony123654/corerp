# RP-2E — Long-run life integration

Status: PASS locally, 2026-09-23. Base checkpoint `4ded11d`; schema026 unchanged. This closes RP-2, not the full living-world goal. RP-3 and later implementation has not started.

## Reuse / implementation

Explicit `rp-life-prepare` composes existing M2 population, finite employer/payroll/store/rent economy, Cai/Lin materialization, shared RP participants/routes and30-day routines. Preparation is resumable command-by-command. Actual contract timing and claim slots determine wage participation and opening receivables; no conservation checks or balances are bypassed. Existing minimal demo setup remains unchanged.

Background materialization adds Nora through the existing population/economic command and evidence-backed appointments. Own finances, employment, temperament, relationships, knowledge and work history remain derived from their existing authorities.

Play waits pin up to16 original nearby NPC identities in the time-advance Event, then the configured provider may propose initiative without a player utterance. Each proposal sees only own/visible context. Commit rechecks trigger, current scene/time, context hash, legal action, actor/world hourly cooldown and existing daily budget. Command/Event/speech or movement/hearing/audit/participant Outbox commit atomically. Reopening completes missing actors and never repeats a committed model effect. Later user actions can supersede an uncommitted opportunity. Quiet decisions produce no scene message; public history contains only accepted speech and visible movement.

## Acceptance audit

| Requirement | Authoritative evidence |
|---|---|
|50–100 continuous turns | `TestRPLifeSixtyTurnsAcrossFiveDaysWithEconomyMemoryAndRecovery`:60 settled turns, five world days, interruption within turn30 |
|3–5 NPCs / places / work unit | Ada, Bo, Cai, Nora; five actual places; existing funded employer and named wage participation |
| Cohort → individual → RP | Nora is conservatively materialized, receives sourced stable background/work appointments, participates throughout the run |
| Economic pressure → choice | Actual90minor cash motivates a need and refusal/initiative; real200minor gift relieves the need; five wages leave340minor |
| Work/schedule → choice | Own employment and upcoming appointments cause refusal; five actual work arrivals execute |
| Relationship/memory → later action | Six real insults create sourced conflict and an actual legal departure by Cai |
| NPC initiative | Nora speaks after elapsed world time, before any player utterance; production Wait/HTTP/browser path and model adapter tested |
| Ordinary quiet life | Ten empty-home turns create no NPC effects; ordinary/cooldown decisions do not fabricate a scene message |
| Restart/recovery | Mid-turn reopen preserves committed effects without more calls; partial multi-NPC wait recovery completes only the missing actor; lost HTTP/browser wait response recovers without duplicate facts |
| At least three same-initial-world runs | Four copies of one SQLite snapshot, each60 turns, identical initial replay hash |
| RNG / provider / accumulated state divergence | Seed1 sends Cai to Bo home; seed2 to Ada home; deterministic/no conflict stays at cafe; same deterministic provider plus actual insults leaves for Ada home |
| World remains consistent | Long-run projection checks/rebuild, snapshot replay, reopening and conservation assertions pass |

Diversity uses controlled seeded providers to isolate causes, not a claim of live-model quality or an early implementation of RP-5 opportunity/LOD systems. It changes committed destinations, not wording. These runs do not substitute for final300-turn/30-day acceptance after RP-8.

## Authority fix discovered by integration

A legal early work arrival formerly made the next scheduled appointment fail with `PROJECTION_DIVERGED` because origin equaled destination. The regression reproduced this failure. A verified same-place appointment now emits `AgentActivityStarted`, updates activity and completes its schedule without inventing a zero-distance movement. A damaged position projection is still rejected. Replay, snapshot replay, reopen/rebuild/compare and own work memory include the activity Event. No schema migration or global canonical-hash change.

Initial initiative recovery also caught an embedded-struct serialization mismatch: the existing canonical encoder does not flatten embedded Go structs. New initiative payloads use explicit fields; historical canonical behavior remains unchanged.

## Executed verification

- Final `go test ./... -count=1`: PASS (storage105.203s, HTTP5.566s); `go vet ./...`: PASS. Includes applicable026 migration/reopen/replay tests and all long-run variants.
- Related core/decision/storage/HTTP/CLI race: PASS (core1.051s, decision1.475s, storage351.029s, HTTP27.489s, CLI12.170s). The60-turn representative is race-covered; the four serial seeded variants are in full uncached Go rather than redundantly repeating the same paths under race.
- Subsequent early-arrival/scheduler fix focused checks PASS3.213s and race PASS15.912s, followed by the final full suite above. Unfiltered server race PASS3.032s.
- Frontend typecheck/build: PASS. Configured M0(52checks), M1 evidence and M2(26test references) verifiers: PASS. No separate lint/frontend-unit script configured.
- Current backend browser model+initiative `/tmp/corerp-rp1-e2e-IVCH7T`: PASS,19 fixture calls; pre-player-speech wait recovery preserves `14:2:1` Event/hearing/utterance counts and repeats zero committed calls. Later spoken-turn recovery preserves `46:54:11`.
- Browser relationship/restart `/tmp/corerp-rp1-e2e-xmoZeG`: PASS. Scoped style `/tmp/corerp-rp1-e2e-3gjMjI`, emergent re-encounter `/tmp/corerp-rp1-e2e-hijmMa`, deterministic initiative `/tmp/corerp-rp1-e2e-IzgFYe`: PASS. Style/emergent runs preceded only the separately reproduced/tested scheduler fix.
- `git diff --check`: PASS. All verification processes terminal before checkpoint.
- Live LLM: REQUIRED_IF_AVAILABLE / NOT VERIFIED; no task-scoped endpoint/model/key configured. The real HTTP adapter and local model fixture are verified, not remote quality.

## Recovery / next

Pre-E recovery point: `4ded11d`, schema026. No production database was modified. Keep a matching database backup for code rollback: older binaries cannot interpret new activity facts even though schema026 is unchanged. Existing wait Events without a pinned initiative roster do not retroactively trigger NPC decisions.

E checkpoint follows this report and a clean-worktree check. Next is RP-3 recon and the smallest real recruitment/employment lifecycle slice, strictly serially. Rich prose/streaming belongs to RP-6; opportunity density/LOD to RP-5; complete career and culture/law remain RP-3/RP-4. None is claimed complete here.
