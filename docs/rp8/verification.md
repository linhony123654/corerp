# RP8 stage verification

Status: RP8 implementation/verification/checkpoint PASS. Source commit
`86ca6f96d39e4a026f2ff73e48f5823a318cdb34`; clean tree verified after commit.
Original authority: `/tmp/0d1b0723-9876-4319-912d-c780b0c66aea.md`, sections9/12/13.
Current schema030. RP7 recovery checkpoint `7de4b10616a1a7adaa3f1fe973590828737f966b`.
No production deploy, external publication, real-account use or live-model test.

## Original scope reconciliation

| Requirement | Current evidence | Status |
|---|---|---|
|8A optional Studio, not default Play|`src/main.ts`: `/` Play, `/studio` evidence, `/studio/create` creation, `/demo` historical fixtures. Actual independent browser flows.|PASS|
|8B seven recorded explanation questions|[Acceptance matrix](inspector-acceptance.md); actual rejection/hearing/absence/committed trigger tests and real UI. No missing-records-as-omniscience claim.|PASS for recorded evidence, limits explicit|
|8B effective rules|Explicit historical epoch→activation→installation validation; exact package contents, corruption/rebuild/reopen and future-epoch isolation; creator/ops/player browser.|PASS|
|8B authority separation|Actual API denial, explicit local grants/revocation, per-page reauthorization, creator-only explanation/body and ops redaction. Tokens never become authority via navigation.|PASS|
|8C create/settings/install/Narrative/System/save/Play|Actual initial creator form uses conserved genesis/participants/spatial +validated package install/activation +sourced ready player grant; real independent player actions.|PASS for supported initial template|
|8C recovery|Seven-stage fault/reopen, frozen request hash, concurrent exact retry, UI lost save/open response, runtime/browser restart, original archive recovery, unchanged other-world head.|PASS|
|No second authority or registry rewrite|Existing cohort/ledger/population/location/clock/Event/session owners reused; installation records are immutable Events, not executable plugin code or a parallel world database.|Verified source and replay tests|
|Stage checkpoint/clean tree|Source86ca6f96d39e4a026f2ff73e48f5823a318cdb34 after all gates/report, git porcelain empty; schema030.|PASS|
|Final300turn/30days and original DoD|Separate serial phase after RP8 checkpoint.|NOT VERIFIED: not started as Final acceptance|

Optional8A module names are not interpreted as mandatory empty dashboards. The
goal explicitly defers arbitrary code sandboxing and hot rule upgrades (section11).
Initial creator UI exposes two named people and two places, bounded resources,
daily initiative budget and narrative presets/imported declarative bundles. Larger
bounded topology specs are API-only. Link minutes are not claimed as consumed
travel durations; movement is immediate. Foreign career phases still fail closed.
These are explicit supported-scope limits, not fabricated installed capabilities.

## Required checks and exact evidence

| Gate | Execution/evidence | Result |
|---|---|---|
|Full backend normal|4389 `go test ./... -count=1 -timeout 20m`: storage318.340s failed two test-only queue fixtures.|FAIL, superseding run pending|
|Failure repair|Both fixture payloads now canonical/persisted, item status `pending`; production strict queue guard unchanged. Focused60376 PASS3.446s.|PASS|
|Full current-source normal|15968 same full command after repair and historical-rule increment: storage311.224s, HTTP18.745s, all remaining packages passed, terminal exit0.|PASS|
|Wholebackend vet|Final43859 `go vet ./...`, terminal exit0 after fixture repair.|PASS|
|Relevant Studio race|11622 all `TestStudio` storage56.802s/HTTP6.041s, terminal exit0.|PASS|
|Relevant queue/economy race|26859 both repaired economic scenarios plus wrong-world/stale-queue before-write denial,53.681s.|PASS|
|Migration/reopen/replay|Studio package/activation/readiness upgrade tests, source projection comparison/rebuild, fresh current schema and reopen; all `TestStudio` race included.|PASS|
|Frontend typecheck/build|41293 `npm run build` vue-tsc +vite3.28s after final CSS.|PASS|
|Configured frontend verifiers|M0 52checks, M1 and M2 bounded-evidence scripts executed2026-09-25.|PASS, not replacements for current runtime tests|
|Real creator/Play/historical Inspector|9614 `node scripts/verify-rp8-play-worlds.mjs --creator-ui`, artifact N3zLal, immutable receipts/installed style/NPC/move/wait/restart and roles.|PASS|
|Original Inspector regression|41293 `node scripts/verify-rp8-studio.mjs`, artifact XbZFrR.|PASS|
|Original Play recovery|72055 `node scripts/verify-rp1-play.mjs`, artifact W05tp5, prior turn; no Play implementation changes afterward.|PASS|
|Visual verification|Actual1440/390 package screenshots reviewed; source ID overflow found/fixed, safe source navigation and disclosure labels visible.|PASS for affected surface; no universal accessibility certification|

Increment-specific historical failures and repairs remain in progress.md; they are
not relabelled passed. No separate lint command is configured in package.json.
Full-suite15968 is terminal, exit0. All verification processes are terminal;
no source edits after the final checks, documentation only before checkpoint.
