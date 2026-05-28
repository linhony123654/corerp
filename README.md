# CoreRP

CoreRP is a world-first persistent narrative runtime for LLM-driven text roleplay.

It is not a Tavern clone, a prompt template collection, or a multi-agent chat room. The target is a runnable world that can evolve over time, explain why events happened, branch into experiments, and let people emerge from world pressure instead of requiring every character to be prewritten as a role card.

Status: experimental prototype. The core runtime exists, but APIs and authoring workflows are still moving.

## Core Idea

CoreRP treats the world as the primary object:

- `world` defines rules, locations, factions, pressures, scenes, canon, and population.
- `focus_character` is the current viewpoint, not the owner of world truth.
- `participants` are the entities present in the current scene.
- `background_npcs` are low-resolution people attached to locations, factions, hooks, or pressures.
- Promoted NPCs become richer personas only after enough runtime evidence accumulates.
- LLM output is constrained to proposals and prose; committed events are the source of truth.

The central invariant:

```text
World state changes only through committed events.
```

## Current Architecture

The runtime path is:

```text
world seed + scene + population
  -> director candidate selection
  -> TurnStep serial execution
  -> ActionFrame validation
  -> event commit
  -> state / memory / population reprojection
```

Important runtime layers:

- `internal/runtime`: turn orchestration, world entry, tick loop, authoring operations.
- `internal/events`: event store, replay, branches, causality.
- `internal/state`: projection from committed events to world state.
- `internal/memory`: semantic facts, episodic events, recall.
- `internal/world`: world catalog, world structure, population files, starter creation.
- `internal/dcl`: declarative content layer packages and install logic.
- `internal/api`: HTTP/SSE API and web integration.
- `web/`: browser runtime console and authoring tools.

## World Format

Directory worlds are the main format:

```text
worlds/<world_id>/
  world.yml
  scenes/
    default.yml
  canon/
    facts.yml
    ontology.yml
  world/
    seed.yml
    locations.yml
    factions.yml
    pressures.yml
    ruleset.yml
    director.yml
    presets.yml
  population/
    background_npcs.yml
    promoted_npcs.yml
    identity_core.yml
    policy.yml
```

Single-file worlds such as `worlds/example.yml` are supported for bootstrapping and examples, but world structure and population editing are intended for directory worlds.

Generated worlds, imported worlds, characters, runtime data, backups, and local DCL registry files are ignored by Git by default. Treat them as local content unless intentionally packaged.

## Quick Start

Build and run:

```bash
/usr/local/go/bin/go build -o corerp ./cmd/corerp
./corerp serve \
  -port 8080 \
  -data ./data \
  -boot world \
  -world ./worlds/example.yml \
  -characters ./characters \
  -secure-cookie=false
```

Open:

```text
http://localhost:8080
```

If auth is enabled, the login page is `/login`. Without `CORERP_AUTH_KEY` or `-auth-key`, the development default password is `admin`.

Optional LLM environment:

```bash
export LLM_URL="https://your-api/v1"
export LLM_API_KEY="your-key"
export LLM_MODEL="model-name"
export CORERP_AUTH_KEY="your-password"
```

You can also configure LLM providers from the browser UI. When editing a saved config, leaving the API key field empty preserves the existing key.

## Main Workflows

### 1. Create A World

Use the browser entry:

```text
/world-starter.html
```

The World Starter can:

- generate a starter from a short concept locally;
- generate a richer starter through the active AI config;
- preview and edit the JSON before writing files;
- create a directory world under `worlds/`;
- enter that world through a clean, deterministic runtime instance.

The starter writes world structure, scene, rules, pressures, locations, factions, and background NPCs. It does not create role cards.

### 2. Enter A World Cleanly

The runtime has an explicit clean world-entry route:

```text
POST /api/worlds/enter-clean
```

It creates or reuses a deterministic `world-<name>-<hash>` runtime instance for the selected world. This prevents old character-loaded state from leaking into a newly selected world.

### 3. Let Population Grow

Population starts in `population/background_npcs.yml`.

Background NPCs can be pulled into the runtime when they match:

- current scene location;
- location controller / faction;
- active pressure target;
- hooks matched by user input or world events.

If attention and event exposure cross policy thresholds, they can be promoted into richer runtime personas. They can also demote when they stop being relevant over time.

### 4. Use DCL Packages

DCL packages are declarative content layers under `mods/`.

Example shape:

```text
mods/<mod_id>.dcl/
  manifest.yml
  data/
    patches/
      population/
      world/
      scenes/
      presets/
  logic/
    hooks.yml
```

DCL is intentionally data-first. The runtime does not execute arbitrary Lua/script files as trusted code. Hooks declare effects that the built-in rule engine understands.

Supported UI actions include upload ZIP, install/enable, disable, and remove.

## Runtime Data

Local runtime state lives under `data/`:

```text
data/
  memory.db
  instances/
  llm_configs.json
```

`llm_configs.json` contains provider configuration and may contain API keys. It is ignored by Git and should not be committed.

Instance-scoped runtime data is isolated by `instance_id`. Events, branches, dialogue, working memory, semantic facts, episodic events, pending facts, player role, saves, and checkpoints are treated as runtime data, not source.

For a hard reset during development, stop the server, back up `data/` and `worlds/`, then remove runtime databases and unwanted generated worlds. Keep source files and secrets separate.

## Important APIs

World and authoring:

- `GET /api/worlds`
- `PUT /api/worlds`
- `POST /api/worlds/draft`
- `POST /api/worlds/enter-clean`
- `GET/POST /api/world-structure`
- `GET/POST /api/population`
- `GET /api/population-insights`

Runtime:

- `POST /api/chat`
- `GET /api/state`
- `GET /api/characters`
- `POST /api/switch`
- `GET/POST /api/player-role`
- `GET /api/trace/latest`
- `GET /api/traces`

Instances and experiments:

- `GET /api/instances`
- `GET /api/instances/status`
- `POST /api/instances/create`
- `POST /api/instances/default`
- `POST /api/instances/stop`
- `POST /api/instances/delete`
- `GET /api/runtime-audit`
- `GET/POST /api/experiment-reports`

DCL:

- `GET /api/dcl`
- `POST /api/dcl/upload`
- `POST /api/dcl/install`
- `POST /api/dcl/remove`

Ops:

- `GET /api/health`
- `GET /api/ready`
- `GET /api/version`
- `GET /api/proof-audits`

See `api-contract.yaml` for the broader contract surface.

## Development

Build:

```bash
/usr/local/go/bin/go build -o corerp ./cmd/corerp
```

Run all tests:

```bash
/usr/local/go/bin/go test -count=1 ./...
```

Check frontend syntax:

```bash
node --check web/app.js
node --check web/world-starter.js
```

Before pushing runtime or API changes, run:

```bash
git diff --check
/usr/local/go/bin/go test -count=1 ./...
node --check web/app.js
node --check web/world-starter.js
/usr/local/go/bin/go build -o corerp ./cmd/corerp
```

Some long matrix tests use local world fixtures that are intentionally ignored by Git. If those fixture directories are absent, the tests skip those sample-specific cases instead of requiring generated user content in the repository.

## Documentation Map

- `ARCHITECTURE.md`: current system layers and major components.
- `ARCHITECTURE_RUNTIME.md`: runtime contracts, event/replay/fork constraints.
- `FINAL_ARCHITECTURE_BLUEPRINT.md`: target end-state design.
- `TODO.md`: current completion state and next work.
- `ACCEPTANCE_CHECKLIST.md`: acceptance gates.
- `DELIVERY_TRACKING.md`: delivery and evidence tracking.
- `NEXT_AI_HANDOFF_PROMPT.md`: handoff prompt for another coding agent.
- `AGENTS.md`: repository workflow and contribution rules.

README should stay short. If implementation details drift, update the architecture docs and link them here instead of turning this file into a session log.
