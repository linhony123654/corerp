# RP8 phase report

Status: implementation and applicable verification PASS; checkpoint metadata pending. Full scope and serial ordering
remain the original goal, not just the creator or inspector increment.

## Delivered

- Optional Studio routes separate from default Play. Authorized branch/timeline
  discovery and Event inspection; creator versus ops redaction, explicit extra
  explanation permission, local sourced grants/revocation and no provisioning API.
- Recorded candidate rejection/suppression, committed trigger/outcome, observer
  acquisition/absence and historical effective-rule provenance. Missing records
  are labelled absent, never fabricated intent/knowledge. Package inspection uses
  the event's epoch, not the current head, and validates immutable activation and
  installed contents before returning creator-only JSON.
- Explicit source-backed world creation authority; conserved initial population,
  money and stock; actual materialized people and places using existing owners.
  World-scoped queue/clock/commit checks protect administrative and other worlds.
- M0-compatible bounded declarative System/Narrative installation and initial
  activation; exact manifests/hashes/dependencies pinned to the new Rule Epoch.
  Actual NPC daily initiative budget and narrative base-style consumers.
- Atomic ready Event/player control grant/clock/lifecycle finalization, followed
  by separate player discovery and actual sessions/actions. Full-request hash and
  original stage receipts support partial workflow retry without a second database.
- Creator form, advanced resources, package presets/import, consented frozen local
  recovery and matched saved receipt. Play authorized binding picker replaces M2
  hardcoding, preserving lost-open retry and per-binding/legacy session recovery.

## Validation

Exact commands, results and failures are in [verification.md](verification.md).
Final full normal15968 PASS (storage311.224s/HTTP18.745s, all packages, exit0)
after two legacy test-fixture repairs; final wholebackend vet43859 PASS.
Relevant Studio and queue/economy race, wholebackend vet, migration/reopen/replay,
typecheck/build, configured evidence verifiers and real browser scenarios passed.
Backend/source permission checks are exercised directly, not only by hidden UI.
No live provider credentials or claims; deterministic local Runtime is used.

Failures were handled without weakening production invariants: malformed old test
queue objects were corrected to match their persisted canonical payload/status;
creator/ops metadata assertions adapted to explicit package redaction; mobile
long-ID overflow fixed; credential clear remains available during in-flight save.

## Compatibility, limits and recovery

- Schema030. Additive028/029/030 protect installed package, activation and ready
  Event sources against UPDATE/DELETE/REPLACE; this is not a claim of generic
  legacy-table immutability against a filesystem administrator.
- No arbitrary executable plugin sandbox, hot upgrades, remote publishing or new
  provider setup. The original goal explicitly defers arbitrary code/hot upgrades.
- Initial creator form uses two people/two places; larger bounded specs are API-only.
  Fixed supported calendar, instantaneous player move, no effective link-duration
  setting; foreign unsupported career phases remain explicitly denied.
- Browser configuration archives are local, unencrypted and opt-in; no credentials
  are stored. Site-data deletion removes local recovery hints, not server worlds.
  No built-in local archive export/delete control yet. Pending intents must be
  recovered before switching, and legacy bookmarks are enriched by real resume.
- Installed base narrative provenance is separate from per-turn presentation
  overlays; observing no acquisition record does not prove subjective ignorance.
- No deployment or external state change. RP7 recovery checkpoint is
  `7de4b10616a1a7adaa3f1fe973590828737f966b`; a schema030 database must not be opened
  blindly with older binaries. Preserve database backups; do not reset world facts.

All applicable verification gates passed. Record RP8 source commit, clean tree and
schema version, then enter Final Integration with the required300 turns/30 world days, multiple
restarts/clients and every named long-life scenario. RP8 completion is not whole-goal
completion; original final DoD remains unproven until those checks finish.
