# CoreRP Platform Manual & Reference Guide

Welcome to the **CoreRP** authoritative documentation. CoreRP is an open-source, deterministic, event-sourced living world simulation and roleplay platform built on strict single-state accounting, spatial reachability, and epistemic containment.

---

## Guide Overview

The manual is divided into four primary sections tailored for different operational roles:

1. **[Player Guide](player.md)**: How to install, launch, select worlds, interact in scenes, navigate spatial maps, manage work and finances, and engage with the living world.
2. **[Creator Guide](creator.md)**: How to author new worlds, configure population cohorts, design organizations and careers, install and activate custom Packs, and use the Studio Inspector.
3. **[External Agent / MCP Guide](external-agent.md)**: How to connect autonomous AI models via the Model Context Protocol (MCP), enroll external controllers, adhere to epistemic privacy boundaries, and participate in synchronized shared rounds.
4. **[Maintainer Architecture & Ops Guide](maintainer.md)**: In-depth engineering specifications covering the event-sourced architecture (Command → Proposal → Event → Projection → Observation), SQLite schema migrations, recovery and replay, World QA health diagnostics, and production operations.

---

## Quickstart (Zero to Playing in 3 Minutes)

### Prerequisites

- **Go**: Version 1.22+ (ensure Go binary is accessible, e.g., `/usr/local/go/bin/go`)
- **Node.js**: Version 18+ (for client and MCP adapters)
- **SQLite**: Embedded via pure-Go `modernc.org/sqlite` (no native C compiler or external database daemon required)

### Step 1: Clone and Build

```bash
git clone https://github.com/corerp/corerp.git
cd corerp
/usr/local/go/bin/go build ./backend/cmd/...
```

### Step 2: Initialize a World

CoreRP ships with deterministic demo bootstraps (`inst_m1` for basic accounting; `inst_m2_t09` for full living world simulation):

```bash
# Launch the server with an in-memory or on-disk database
./backend/cmd/corerp-server/corerp-server --db /tmp/corerp-demo.db
```

### Step 3: Open an RP Session and Play

Using the HTTP API or the Play UI:

```bash
# Open an interactive roleplay session for resident player "Lin"
curl -X POST http://localhost:8080/api/v1/rp/session/open \
  -H "Authorization: Bearer principal_m2_rp_player" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "inst_m2_t09",
    "branch_id": "br_main",
    "entity_id": "entity_m2_rp_lin",
    "pov": "second_person",
    "idempotency_key": "quickstart-session-1"
  }'
```

### Step 4: Run Health Diagnostics (World QA)

Check that all 14 simulation dimensions are operating stably:

```bash
curl -X POST http://localhost:8080/api/v1/studio/qa \
  -H "Authorization: Bearer principal_test_creator" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "inst_m2_t09",
    "branch_id": "br_main"
  }'
```

---

## Key Architectural Principles

- **Single Source of Truth**: All world facts originate as immutable, sequentially ordered Events in SQLite. Projections and caches can be wiped and completely rebuilt from history at any time.
- **Epistemic Privacy**: Physical co-location, sight, and hearing are distinct from world truth. Characters only know what they have firsthand observed or received through validated communication channels.
- **Double-Entry Economic Conservation**: Every currency movement is balanced by double-entry ledger postings (`DoubleEntryBalanceZeroSum == 0`). Money cannot be created or destroyed out of thin air.
- **No Invisible Hacks**: Characters, households, jobs, and narrative DLC packs are defined purely as declarative data, evaluated deterministically by the authoritative host engine.
