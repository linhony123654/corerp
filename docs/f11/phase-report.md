# F11 local phase report — Single-Source Manual & Maintainer Handoff

Status: **local acceptance PASS**. Baseline is F10 checkpoint `233ef44415cf2db4330be4b5668b3dc887cb0da3`. The containing scoped commit is the F11 checkpoint; verify its clean worktree before entering F12. No push or release.

## Implemented and demonstrated

- **Single-Source Documentation Pipeline**:
  - Markdown files under `docs/manual/` serve as the sole source of truth:
    - `index.md`: Quickstart overview, architecture summary, and audience portal.
    - `player.md`: Complete player manual covering startup, play modes, long narratives, spatial travel, finances, and truth-vs-claim distinction.
    - `creator.md`: Complete creator manual covering world authoring, demographic cohorts, spatial containment, subsystem configuration, DLC packs, and Studio Inspector.
    - `external-agent.md`: Complete external agent manual covering MCP integration, controller binding, sensory fencing, and shared round coordination.
    - `maintainer.md`: Complete engineering reference covering event sourcing pipeline, migrations, projection rebuilds, backup, World QA, and race splitting.
  - Native Go static site compiler (`backend/cmd/manual-gen/main.go`):
    - Parses structured Markdown files and generates responsive offline HTML artifact (`docs/manual/dist/index.html`, 78,135 bytes).
    - Desktop sidebar navigation with active section highlighting and collapsible mobile drawer.
    - Full-text search with instant filtering and keyboard shortcuts (`/` to search, `Esc` to dismiss).
    - Anchor links on all headings for direct navigation.
    - Copy buttons on all code blocks with clipboard integration.
    - Fully offline: zero external CDN or Google Fonts dependencies, using pure system font stacks.
    - Zero token, key, or sensitive secret leakage; strictly synthetic fixtures.

- **Automated 9-Step Reproduction Verification**:
  - `backend/internal/storage/f11_reproduction_test.go` (`TestF11TutorialReproduction`) runs the exact 9-step tutorial sequence from the manual in a clean `t.TempDir()`:
    1. **Start / Open**: Bootstraps store, runs migrations, verifies route topology.
    2. **Create / Select World**: Grants creator capability, creates world with Life Journaling narrative pack.
    3. **Play 1 Round**: Opens RP session, observes initial sensory state, plays utterance with deterministic settlement.
    4. **Use Long Narrative**: Generates and verifies long life journal narrative lines.
    5. **Install Sample Pack**: Validates and installs content DLC bundle containing retail career catalogs.
    6. **Enable MCP Test Resident**: Registers service principal and enrolls external controller binding for resident entity.
    7. **Restart (Process Exit & Reopen)**: Closes SQLite database connection completely and reopens fresh store from disk.
    8. **Resume & Continue**: Resumes active session and successfully plays subsequent turn without state loss.
    9. **Open Inspector & World QA**: Grants inspector capability, reads World QA diagnostics across 14 dimensions (confirming `HEALTHY`, zero balance drift, zero knowledge leaks), reads macro observer perspective with Inspector links, and inspects event rule provenance.

## Verification

### Requirement mapping

| Requirement | Implementation and Evidence |
| --- | --- |
| Markdown as single source of truth | `docs/manual/{index,player,creator,external-agent,maintainer}.md`; HTML compiled via `backend/cmd/manual-gen/` |
| Player guide | `docs/manual/player.md`; verified by `TestF11TutorialReproduction` steps 1-4, 8 |
| Creator guide | `docs/manual/creator.md`; verified by `TestF11TutorialReproduction` steps 2, 5, 9 |
| External Agent & MCP guide | `docs/manual/external-agent.md`; verified by `TestF11TutorialReproduction` step 6 |
| Maintainer & Ops guide | `docs/manual/maintainer.md`; verified by `TestF11TutorialReproduction` steps 7, 9 |
| Responsive sidebar & search UI | `backend/cmd/manual-gen/main.go`; verified by `TestManualGenerationOutputAndIntegrity` |
| Zero external CDN/font dependencies | Verified by `TestManualGenerationOutputAndIntegrity` |
| Anchor links & code copy buttons | Verified by `TestManualGenerationOutputAndIntegrity` |
| 9-step tutorial reproduction test | `TestF11TutorialReproduction` in `backend/internal/storage/f11_reproduction_test.go` (PASS in 0.849s) |

### Test suite results

- **Manual generator tests**: `backend/cmd/manual-gen/...` PASS 2/2 tests (0.010s).
- **Reproduction test**: `backend/internal/storage/f11_reproduction_test.go` PASS (0.849s).
- **Static checks**: `go vet ./...` PASS (0 errors); `git diff --check` PASS (clean).
