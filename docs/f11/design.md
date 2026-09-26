# F11 — Single-Source Manual & Maintainer Handoff: Design and Acceptance Ledger

Status: **LOCAL ACCEPTANCE PASS**.

## Scope and Objectives

1. **Single Source of Truth**:
   - Markdown documents in `docs/manual/` are the sole authoritative source of truth.
   - HTML (`docs/manual/dist/index.html`) is a strictly generated static artifact produced by an embedded native Go static-site generator (`backend/cmd/manual-gen/`).
   - Content is never manually maintained or duplicated across two representations.

2. **Comprehensive Four-Audience Documentation**:
   - **Player Manual (`docs/manual/player.md`)**:
     - Installation and server startup.
     - World creation and selection.
     - Play interface, Dialogue mode, Scene mode, and Long Narrative life journaling.
     - Spatial navigation: Move, travel, and Wait commands.
     - Personal context: Wallet balance, employment shifts, contact list, travel maps, and private message communication.
     - State safety: Turn retry, process restart, and narrative regeneration without semantic drift.
     - Distinction between objective world truth and subjective character claims.
   - **Creator Manual (`docs/manual/creator.md`)**:
     - World creation, seeds, and initial parameters.
     - Demographic cohorts, regions, parent-child place containment, and spatial networks.
     - Subsystem configurations: household templates, education programs, and organization agency policies.
     - DLC package management: authoring, validation, installation, and activation.
     - Narrative package styling: custom prompt templates, prose constraints, and tone profiles.
     - Studio Inspector: rule epoch verification, event timeline provenance, and branch safety.
   - **External Agent Manual (`docs/manual/external-agent.md`)**:
     - MCP server configuration, stdio transports, and tool definitions.
     - Principal authorization and controller binding (`EnrollRPExternalControllerLocal`).
     - Observation fencing: bounded perceptual context, sanitized sensory views, and privacy guarantees.
     - Shared round coordination, turn budgets, disconnect recovery, and shared clock progression.
   - **Maintainer Manual (`docs/manual/maintainer.md`)**:
     - Unidirectional state pipeline: Command -> Proposal -> Event -> Projection -> Observation.
     - Subsystem contracts: Turn lifecycle, epistemic knowledge containment, world clock, directed timed edges, households, health/condition, education/career, and organization agency.
     - SQLite database schema: strict migrations, WAL mode, transaction isolation.
     - Disaster recovery: live online backups, projection drift auditing (`CompareProjections`), and deterministic projection rebuilds (`RebuildProjections`).
     - World QA diagnostics: 14 simulation health dimensions, anomaly thresholds, and observer mode scoping.
     - Development workflow: race detection splitting, test strategies, and static verification.

3. **Offline Static Site Compiler (`backend/cmd/manual-gen/`)**:
   - Standalone native Go generator with zero external runtime dependencies.
   - Responsive UI featuring desktop sidebar navigation and mobile collapsible drawer.
   - Client-side full-text search with keyboard access (`/` to focus, `Esc` to clear).
   - Deep anchor linking (`#id`) on all markdown headings.
   - One-click copy buttons on all code snippets.
   - Zero external CDN or Google Fonts calls; strictly utilizes system font stacks.
   - Synthetic world fixtures only; zero real secrets, tokens, or private keys.

4. **Automated Reproduction Test (`TestF11TutorialReproduction`)**:
   - Verifies the entire 9-step tutorial sequence end-to-end in a clean `t.TempDir()`:
     1. Start / Open
     2. Create / Select World
     3. Play 1 Round
     4. Use Long Narrative
     5. Install Sample Pack
     6. Enable MCP Test Resident
     7. Restart (close and reopen SQLite database)
     8. Resume & Continue
     9. Open Inspector & World QA Diagnostics
