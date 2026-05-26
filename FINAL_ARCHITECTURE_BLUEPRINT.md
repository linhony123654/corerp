# CoreRP Final Architecture Blueprint

## 1. Project Definition

CoreRP's best final form is not "a better chat UI" and not "a multi-agent roleplay toy".

It should become a **persistent narrative runtime**:

- the world exists before the text
- text is the rendered result of runtime execution
- LLM is a bounded planner/renderer, not the truth source
- every important change is replayable, explainable, and branchable

One-line definition:

> CoreRP is a replayable, branchable, explainable narrative runtime for persistent text worlds.

## 2. Design Goals

The final architecture should satisfy all of these:

1. The world is primary, narrative text is secondary.
2. Multiple characters may participate in one turn without losing determinism.
3. Authors can edit, inspect, fork, merge, checkpoint, and replay.
4. Runtime decisions can be explained at turn level and step level.
5. LLM output is constrained by runtime contracts instead of directly mutating state.

## 3. System Overview

```text
Author / Player / Tick / System Trigger
    -> API / Runtime Console
    -> Runtime Instance
    -> DirectorPlan
    -> TurnStep[0..n]
    -> Step Execution Engine
    -> Event Commit
    -> State Projection
    -> Memory / Causality / Trace Update
    -> SSE / API Response
```

The system should be understood as 8 layers:

1. Canon Layer
2. Runtime Instance Layer
3. Event Layer
4. Projection Layer
5. Turn Execution Layer
6. Memory and Emotion Layer
7. Explainability and Authoring Layer
8. Interface Layer

## 4. Core Architectural Principle

The core invariant is:

```text
World changes only through committed events.
```

Corollaries:

- LLM never directly writes world state.
- WorldState is always a projection.
- Replay is authoritative.
- Director decides steps, not truth.
- Each step is serial, never concurrent world mutation.

## 5. Module Dependency Model

```text
core
  <- events
  <- state
  <- actions
  <- agents
  <- memory
  <- emotion
  <- narrative
  <- runtime
  <- api
  <- llm
  <- world

world
  -> core

events
  -> core

state
  -> core
  -> events

actions
  -> core

agents
  -> core
  -> goalexpr

memory
  -> core

emotion
  -> core
  -> memory

context
  -> core

llm
  -> core

narrative
  -> core
  -> events

runtime
  -> core
  -> world
  -> events
  -> state
  -> actions
  -> agents
  -> memory
  -> emotion
  -> context
  -> llm
  -> narrative

api
  -> core
  -> runtime
  -> auth

web
  -> api
```

Rules:

- `core/` contains only protocols and shared types.
- `runtime/` is the orchestration kernel.
- `events/`, `state/`, and `actions/` must stay deterministic and runtime-owned.
- `api/` is transport only.
- `web/` consumes runtime APIs and must not own world logic.

## 6. Final Runtime Model

The runtime should execute a turn using one stable protocol:

```text
Trigger
-> DirectorPlan
-> TurnStep[0]
-> StepResult[0]
-> StepHandoff[0]
-> TurnStep[1]
-> StepResult[1]
-> StepHandoff[1]
-> ...
-> TurnOutcome
```

Supported triggers:

- `user_input`
- `tick`
- `npc_autonomy`
- `system_action`

## 7. Canon Layer

The Canon Layer defines stable world truth and author-owned constraints.

Inputs:

- `world.yml`
- `canon/facts.yml`
- `canon/ontology.yml`
- `scenes/*.yml`
- character cards
- runtime budgets and rules

Responsibilities:

- define world rules
- define base identities
- define stable facts and scene templates
- define the boundary LLM must not overwrite

Canon is not prompt text. It is runtime-owned source material.

## 8. Event Layer

The Event Layer is the only write-entry path.

Recommended long-term event shape:

```text
Event
  id
  branch
  type
  actor
  target
  payload
  causes[]
  effects[]
  canonical
  tag
  hash
  created_at
```

The event system should support:

- append-only storage
- deterministic serialization
- branch inheritance
- chained hash validation
- causality links
- snapshot compatibility
- narrative/system/tick/maintenance tags

## 9. Projection Layer

Projection converts canonical events into runtime state.

Requirements:

- pure function
- same event stream -> same state
- LLM-independent
- replay-authoritative

Recommended final `WorldState` coverage:

- clock
- scene
- relationships
- flags
- variables
- tension
- cast presence
- optional inventories/resources
- optional references to unresolved threads

## 10. Runtime Instance Layer

The final architecture should not be "one global engine with one current character".

It should introduce first-class runtime instances:

```text
RuntimeInstance
  instance_id
  world_id
  branch
  player_role
  active_cast
  current_state_hash
  current_turn
  memory_scope
  trace_history
  save_slots
  checkpoints
```

Why this matters:

- same world can have multiple experiments
- author sandboxes become possible
- multi-user isolation becomes possible
- saves/checkpoints stop being ad hoc
- branch operations become instance-aware

## 11. Director System

The Director is not a "multi-agent chat router".

It is a **dramatic step planner**.

Responsibilities:

- choose which characters participate
- assign `lead`, `addressed_reply`, `support_response`, `tension_response`
- cap turn width
- assign budget mode
- produce explainable step reasons

Director output:

```text
DirectorPlan
  mode
  trigger
  previous_speaker
  candidates[]
  steps[]
  reason
```

The current preferred step kinds are:

- `lead`
- `addressed_reply`
- `support_response`
- `tension_response`
- `followup` as generic fallback

## 12. Step Execution Layer

This is the most important runtime path.

Each `TurnStep` should always run through the same lifecycle:

1. resolve speaker context
2. build snapshot
3. inject step-role directives
4. inject step handoff
5. filter allowed actions
6. LLM produces `ActionFrame + narrative`
7. validator checks result
8. runtime normalizes or downgrades illegal actions
9. executor emits events
10. commit events
11. produce `StepResult`
12. emit `StepHandoff`

This protocol must be identical for:

- single-speaker turns
- multi-step turns
- future NPC/system-triggered turns

## 13. Step Kinds and Action Semantics

Step kinds must affect three things:

### 13.1 Selection semantics

- who speaks first
- who responds second
- who is allowed to escalate tension

### 13.2 Prompt semantics

- `lead` must directly address the user’s main input
- `addressed_reply` must respond briefly to the explicit mention
- `support_response` must add attitude, relationship, or stance
- `tension_response` must carry escalation, pressure, caution, or de-escalation

### 13.3 Action semantics

- `addressed_reply`: prefer `speak`, `trust`, `negotiate`
- `support_response`: prefer `trust`, `speak`, `negotiate`
- `tension_response`: prefer `threaten`, `hide`, `attack`, `speak`
- `lead`: keeps the broadest action surface

If LLM outputs an out-of-role action, runtime should downgrade it before commit.

## 14. Step Handoff

Later steps should not rely only on implicit world updates.

They should receive explicit handoff data:

```text
StepHandoff
  from_speaker
  step_index
  kind
  action
  target
  outcome_summary
  narrative
  events[]
```

Why this is better:

- followups know what just happened
- response steps become less guessy
- trace becomes inspectable
- later author tooling can show step-to-step causation

## 15. Step Result and Turn Outcome

The architecture should formalize the output of execution.

Recommended long-term structures:

```text
StepResult
  step
  snapshot_summary
  action_frame
  validator_result
  committed_events[]
  narrative
  handoff_out

TurnOutcome
  turn
  plan
  step_results[]
  final_state_hash
  final_narrative
  created_at
```

`TurnOutcome` should become the stable aggregation object for:

- turn history
- save summaries
- replay summaries
- author console drilldown
- branch diff explanations

## 16. Memory Layer

Memory should remain layered:

- short-term
- working memory
- semantic facts
- episodic events

Properties:

- memory is not canon
- semantic memory should remain attributable
- episodic memory should remain event-linked
- working memory should remain replaceable and summarizable

## 17. Emotion and Thread Layer

This should stay partially separate from canonical world truth.

Recommended subdomains:

- emotional residue
- desire store
- pressure model
- unresolved threads
- delayed reactions

Recommended influence path:

```text
events
  -> episodic memory
  -> semantic candidates
  -> emotional residue
  -> unresolved threads
  -> desire / pressure
  -> director and autonomy inputs
```

Emotion may influence runtime decisions, but should not silently overwrite canonical state.

## 18. LLM Role Layer

LLM should remain split by task:

- `narrative`
- `summary`
- `extraction`

That means router-based role separation should be normal architecture, not optional decoration.

Boundaries:

- LLM sees snapshots, not raw event internals
- LLM emits structured action candidates, not truth
- runtime owns validation, mutation, and commit

## 19. Explainability Layer

This project should treat explainability as a first-class product feature.

Every turn should be able to explain:

- why these speakers were selected
- why these actions were allowed
- what memory was recalled
- what validator changed or blocked
- what events were committed
- what handoff reached the next step

Trace should remain structured, not free-text only.

## 20. Timeline and Branch OS

This is one of CoreRP's biggest differentiators.

The final system should support:

- replay to event
- replay to world time
- create branch
- inherited branch lineage
- branch diff
- selective merge
- checkpoint
- restore
- scenario preset

This makes the system useful for both players and authors.

## 21. Authoring Console

The final frontend should behave more like a narrative control console than a chat page.

Three target views:

### 21.1 Play View

- dialogue flow
- current scene
- current step speaker
- simplified trace

### 21.2 Author Console

- world editor
- scene editor
- character editor
- canon fact editor
- save/checkpoint tools
- branch diff and merge

### 21.3 Debug / Explain View

- director plan
- step traces
- handoff chain
- causality graph
- memory layers
- validator hits
- committed events

## 22. API Grouping

The final API should be grouped by responsibility.

### Runtime API

- `/api/chat`
- `/api/state`
- `/api/turn/latest`
- `/api/turn/:id`
- `/api/trace/latest`
- `/api/trace?turn=n`

### World API

- `/api/world-config`
- `/api/scenes`
- `/api/canon-facts`
- `/api/characters`
- `/api/character-config`

### Timeline API

- `/api/timeline`
- `/api/replay`
- `/api/fork`
- `/api/branches`
- `/api/branches/diff`
- `/api/branches/merge`

### Memory API

- `/api/memory`
- `/api/quarantine`
- `/api/pending-facts`
- `/api/npc-action-log`

### Authoring API

- `/api/saves`
- `/api/saves/load`
- `/api/saves/diff`
- `/api/checkpoints`
- `/api/checkpoints/restore`
- `/api/scenario-presets`

### Ops API

- `/api/usage`
- `/api/llm-routes`
- `/api/health`
- `/api/ready`
- `/api/version`

## 23. Phased Roadmap

### Phase A: Finish current runtime contract

- formalize `TurnOutcome`
- surface handoff in frontend trace
- improve step result aggregation
- deepen kind-aware validator logic

### Phase B: Introduce runtime instances

- create `RuntimeInstance`
- make memory scope instance-aware
- make saves/checkpoints instance-aware
- support multiple simultaneous experiments

### Phase C: Strengthen author tooling

- checkpoint and rollback
- scenario presets
- branch merge UI
- trace history browser
- causality summary tools

### Phase D: Long-running world autonomy

- idle world advancement
- NPC autonomous event scheduling
- offscreen scene evolution
- long-run compression and summary

## 24. What Not To Do

To preserve the project's value, avoid these:

### Do not make multiple agents concurrently mutate world state

This breaks determinism, replay trust, causality integrity, and testability.

### Do not let LLM directly write world truth

That collapses the runtime boundary.

### Do not move core logic into the frontend

That splits runtime semantics.

### Do not over-distribute the system too early

The hardest problem here is protocol stability, not infra scale.

### Do not flatten emotion into canonical truth

Truth, memory, and emotion must remain distinct layers.

## 25. Immediate Priority List

If building toward this blueprint, the next best structural moves are:

1. formalize `TurnOutcome`
2. introduce `RuntimeInstance`
3. visualize handoff in the frontend
4. deepen kind-aware validator semantics
5. add checkpoint and rollback

## 26. Final Product Positioning

The strongest final positioning for CoreRP is:

> CoreRP is a persistent narrative runtime that allows multi-character text worlds to run as replayable, branchable, explainable systems.

That is stronger and more defensible than:

- AI chat app
- character roleplay tool
- multi-agent conversation sandbox

Because its unique value is not "it can say something interesting".

Its unique value is:

- it can run a world
- preserve causality
- branch timelines
- explain decisions
- support authors and players at the same time

