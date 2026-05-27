# CoreRP Final Architecture Blueprint

## 1. Project Definition

CoreRP 的最终形态不应该是“更高级的酒馆”，也不应该只是“多角色聊天器”。

它应该成为一个长期存在、可回放、可分叉、可解释的文字世界运行时。

核心定义：

- 世界先于文本存在
- 文本只是 runtime 执行结果的渲染层
- LLM 是受约束的 planner / renderer，不是世界真相源
- 重要变化必须可 replay、可解释、可 fork

一句话定义：

> CoreRP is a replayable, branchable, explainable narrative runtime for persistent text worlds.

进一步压缩成产品语言就是：

```text
角色不是导入的，是长出来的。
```

## 2. Design Goals

The final architecture should satisfy all of these:

1. The world is primary, narrative text is secondary.
2. Multiple characters may participate in one turn without losing determinism.
3. Authors can edit, inspect, fork, merge, checkpoint, and replay.
4. Runtime decisions can be explained at turn level and step level.
5. LLM output is constrained by runtime contracts instead of directly mutating state.

补充两条产品级目标：

6. Population 应先以低分辨率存在，被关注后再晋升为主要角色。
7. Character identity 应主要由经历塑造，而不是长期依赖外部角色卡导入。

## 3. Final Stack Shape

最终我建议把系统稳定成这条主干：

```text
World Ruleset
    ↓
World Seed
    ↓
Population Manager
    ↓
Identity Core
    ↓
Director / Orchestrator
    ↓
World Pulse / Pressure Engine
    ↓
NPC Desire + Emotion Engine
    ↓
ActionFrame
    ↓
Executor
    ↓
EventStore
    ↓
Projection / Replay / Fork
    ↓
Narrative Renderer
```

三层含义必须清楚：

1. 世界层：世界怎么运转
2. 人格层：角色为什么这样变
3. 叙事层：最后怎么说出来

## 4. System Overview

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
2. Population and Identity Layer
3. Runtime Instance Layer
4. Event Layer
5. Projection Layer
6. Turn Execution Layer
7. Memory / Emotion / Pressure Layer
8. Explainability / Authoring / Interface Layer

## 5. Core Architectural Principle

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

进一步约束：

- LLM 不拥有世界
- LLM 不直接改状态
- LLM 只提出意图和渲染表达
- 世界真相只能由 committed events 产生

## 6. World Layer

世界层最终应该长成：

```text
world/
  ruleset.yml
  seed.yml
  factions.yml
  locations.yml
  pressures.yml
```

定义：

- `ruleset.yml`: 这个世界允许什么，不允许什么，基本物理/社会/超自然规则是什么
- `seed.yml`: 初始局势、地点、时间、社会结构、风险基线
- `factions.yml`: 阵营、权力关系、盟友/敌对结构
- `locations.yml`: 地点、控制权、可达性、局势变化
- `pressures.yml`: 世界压力源，决定世界不会静止

这层的职责不是写文案，而是提供 runtime 真正可执行的世界约束。

## 7. Population and Identity Layer

角色来源不应再长期依赖酒馆卡导入。

更合理的生长路径是：

```text
世界先存在
↓
世界需要人口
↓
生成低分辨率 NPC
↓
玩家关注 / 事件卷入
↓
晋升成主要角色
↓
经历塑造人格
```

因此最终目录建议是：

```text
population/
  background_npcs/
  promoted_npcs/
  identity_core/
```

其中：

- `background_npcs`: 低分辨率路人，只保留最必要的社会位置和可见特征
- `promoted_npcs`: 被关注、被卷入、被世界事件抬升后的主要角色
- `identity_core`: 慢变量人格骨架，如边界感、依赖模式、羞耻阈值、忠诚模式

`PromotionPolicy` 应由 runtime attention 驱动，而不是手工“选角色上场”。

## 8. Module Dependency Model

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

## 9. Final Runtime Model

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

## 10. Canon Layer

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

## 11. Runtime Instance Layer

运行实例不是附属概念，而是世界实验、作者沙盒、分支体验的基础隔离层。

同一个 world 应该天然支持：

- 多个实验实例
- 多个作者沙盒
- 多个 checkpoint / save / branch 组合
- 不同 player perspective 的并行尝试

## 12. Event Layer

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

## 13. Projection Layer

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

## 14. Director System

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

## 15. World Pulse / Pressure Layer

如果世界没有自己的脉冲，它就会退化成被用户戳一下才动一下的聊天器。

所以最终应独立出：

- `World Pulse`: 世界 tick 时哪些局势自然推进
- `Pressure Engine`: 哪些矛盾、稀缺、威胁、期限在升高
- `Director Input`: 哪些压力会改变接下来谁被卷入

这层是 CoreRP 区别于静态 RPG 对话树的关键之一。

## 16. Desire / Emotion / Interpretation Layer

角色的变化不能只靠一句 system prompt。

最终至少要拆成：

- `Desire`: 她想要什么
- `Emotion`: 她此刻被什么触发
- `Relationship`: 她如何理解你
- `Interpretation`: 她如何主观解释刚发生的事

同一事件对不同角色应该生成不同的主观残留。

## 17. Step Execution Layer

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

## 18. Step Kinds and Action Semantics

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

## 19. Step Handoff

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

## 20. Step Result and Turn Outcome

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

## 21. Memory Layer

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

## 22. Narrative Layer

最终叙事层只负责表达，不负责拥有真相。

它应该至少包括：

- `renderer`: 把事件和状态变成自然文本
- `style`: 不同世界观的表达差异
- `leakage`: 情绪泄露、压抑、含混、言外之意

所以最终体验目标不是普通 RPG 文本框，而是一个人物会被经历改变的文字世界 runtime。

## 23. Emotion and Thread Layer

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

## 24. LLM Role Layer

LLM should remain split by task:

- `narrative`
- `summary`
- `extraction`

That means router-based role separation should be normal architecture, not optional decoration.

Boundaries:

- LLM sees snapshots, not raw event internals
- LLM emits structured action candidates, not truth
- runtime owns validation, mutation, and commit

## 25. Explainability Layer

This project should treat explainability as a first-class product feature.

Every turn should be able to explain:

- why these speakers were selected
- why these actions were allowed
- what memory was recalled
- what validator changed or blocked
- what events were committed
- what handoff reached the next step

Trace should remain structured, not free-text only.

## 26. Timeline and Branch OS

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

## 27. Authoring Console

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
