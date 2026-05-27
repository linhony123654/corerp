# CoreRP Architecture

## 当前定位

CoreRP 现在已经不是“角色卡聊天页”，但也还没到最终的长期演化世界 runtime。

当前明确方向是：

```text
world-first persistent narrative runtime
```

也就是：

- 世界是入口，不是角色列表
- 角色是世界中的实体、视角、参与者，不是产品主语
- LLM 只负责受约束的意图和表达，不拥有世界真相
- 世界真相只能通过 committed events 改变

这和常见 RPG / 酒馆类产品的差别在于：

- CoreRP 不把“预写角色卡 + 对话”当主循环
- CoreRP 不把 LLM 当状态拥有者
- CoreRP 的目标是 replay / fork / projection 驱动的世界运行时

## 三层目标模型

最终结构应稳定分成三层：

1. 世界层：决定世界如何运转
2. 人格层：决定角色为何变化
3. 叙事层：决定状态如何被说出来

对应到目标管线：

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

## 当前分层架构

```text
Interface Layer (PWA / CLI / API Consumers)
    ↓ HTTP + SSE
API Gateway (无状态路由、统一认证)
    ↓
Runtime Core (叙事运行时内核)
    ├── Runtime Instance Manager (instance_id + default routing + lifecycle)
    ├── Context OS     (Snapshot Compiler + Token Budget 硬墙)
    ├── State Machine  (Variables + Constraints + Transitions)
    ├── Event Bus      (Event Store + Projector + Causality Engine)
    ├── Action Layer   (Frame Definition + Executor + Permissions)
    ├── Memory Engine  (Short-term / Working / Semantic / Episodic)
    ├── Emotion Layer  (Pressure + Residue + Unresolved Threads)
    ├── Population Layer (background/promoted/identity skeleton)
    └── Canon Layer    (Ontology + Facts + Consistency)
    ↓
Narrative Layer (Renderer + Tension Engine + Compression)
    ↓
LLM Adapter (OpenAI / Claude / DeepSeek / Ollama / Local BGE)
    ↓
Storage (SQLite + instance namespace | YAML worlds/characters)
```

## 核心数据流

1. User Input → Intent Analysis（代码规则）
2. Director / Runtime 读取当前世界投影
3. Population / Identity / Emotion 提供当前参与者上下文
4. Memory Engine 召回相关事实（Semantic + Episodic）
5. Canon Layer 一致性检查（禁止污染 Canonical Truth）
6. Context OS 组装 WorldSnapshot（Token 预算硬墙）
7. Director 产出 TurnPlan（一个或多个顺序 TurnStep）
8. 每个 TurnStep：Snapshot Compile → LLM Adapter → Action Frame
9. Narrative Validator 拦截 / 降级 / 重写
10. Action Executor 执行世界变更 → Event Store 追加
11. Projection / Replay 刷新世界状态
12. 下一 TurnStep 在更新后的 Projection 上继续执行
13. Renderer 生成叙事文本（SSE 返回）
14. Memory / Emotion / Pressure 更新残留状态

## 当前已落地

当前代码里已经稳定存在的核心骨架：

- `EventStore + Replay/Fork + Projection Hash`
- `Emotion / Desire / Pressure` 的基础结构
- `Action Budget / Action Log`
- `DirectorPlan` 雏形
- `RuntimeInstance` 实例层
- `world-first` 入口与 world catalog
- `population/` 目录骨架与 API
- `PulseEngine` — 压力演化（升级/衰减/escalate 连锁）
- `FactionEngine` — 势力紧张度与关系动态
- `npcTickExposure` — 无用户输入时的 background NPC 自主增长

世界目录当前已经支持：

```text
worlds/<world>/
  world.yml
  canon/
  scenes/
  population/
    background_npcs.yml
    promoted_npcs.yml
    identity_core.yml
    policy.yml
```

当前 `/api/worlds` 和 `/api/population` 已经把“世界先存在、人口后生长”的方向落到接口层。

## 当前缺口

还没完成、但已经明确要补齐的部分：

1. `World Ruleset / Seed / Factions / Locations / Pressures` 已有可编辑面板，Planner 和 Scheduler 已接入 world structure 驱动
2. `Population Manager` 已有 tick 驱动的晋升流程，但降级/遗忘机制仍缺
3. `Identity Core` 还没有形成“慢变量人格骨架 + 经历塑形”的闭环
4. `World Pulse / Pressure Engine` 已成为独立 tick 子系统，但压力缓解/干预机制仍缺
5. `Interpretation / Relationship` 仍缺专门层，角色对同一事件的主观理解还不够独立
6. `Narrative Renderer` 仍偏运行台输出，还没完全形成风格化叙事渲染层

## 技术选型

| 组件 | 选择 | 排除项 | 原因 |
|------|------|--------|------|
| 语言 | Go 1.22+ | Python / Node | 单二进制、goroutine 适合 Tick、SSE 原生 |
| DB | SQLite + sqlite-vec | PostgreSQL / MongoDB | 单文件、Git-friendly、零运维 |
| 嵌入 | BGE-small-zh-v1.5 (ONNX) | 远程 API / 大模型本地 | 512-dim、中文语义优化、零 API 成本 |
| 流式 | SSE | WebSocket / 长轮询 | 比 WS 简单、Nginx 友好 |
| 前端 | Vanilla JS PWA | React / Vue | 无框架、几百行、只负责渲染 |
| 部署 | 单二进制 + systemd/PM2 | Docker / K8s | 过度、荒谬 |

## 核心原则

1. LLM 永远不能直接定义世界真相
2. Event Store 是唯一真相源，状态是投影
3. Action Layer 是世界变更的唯一入口
4. Token 预算硬墙不可突破，超预算 panic（开发期）
5. Timeline fork 通过 `branches` 元数据建模，禁止通过改写历史事件归属来“分支”
6. 人物默认不应被当成导入资产，而应被视为世界人口的晋升结果
7. 世界文件三层分离：world.yml / canon/ontology.yml / canon/facts.yml / scenes/
8. 人口层独立持久化：`population/background_npcs.yml` / `promoted_npcs.yml` / `identity_core.yml` / `policy.yml`
9. 所有配置 YAML 化，拒绝 JSON 黑盒
10. SQLite 单文件，备份 = `cp memory.db backup/`
11. 认证：HMAC session token，httpOnly cookie
12. 嵌入模型：BGE-small-zh-v1.5（512-dim，中文语义优化，零 API 成本；开发环境 fallback 2-gram）
13. Runtime Instance 是一等概念：API、事件、分支、记忆、存档必须支持 `instance_id`

## 当前实现备注

- 首页入口已开始 world-first 化：
  - `/api/worlds` 提供 world catalog
  - 顶栏支持 world selector
  - 原角色切换现在更接近“视角切换”兼容层
- `population/` 已完成第一阶段：
  - 后端可按 world 读取/保存 population 配置
  - catalog 会返回 background/promoted/identity 计数
  - 新世界导入时会初始化空 population 目录
- CoreRP 已不再是“单全局 runtime”模型：
  - API 层支持 `instance_id`
  - `internal/runtime.Manager` 支持 `list / status / create / set default / stop / delete`
- 持久化当前采用混合模式：
  - 文件持久化按 `data/instances/<instance_id>/...` 分目录
  - SQLite 共享单库，但关键表按 `instance_id` 过滤
- 当前运行策略：
  - `data/` 作为唯一标准运行目录
  - PM2 固定以 `-data /home/kelebituo/corerp/data` 启动
  - 启动后通过 `deploy/smoke-check.sh` 验证 `/api/health` 与 `/api/ready`
- 作者工具当前已接通：
  - checkpoint / rollback 复用实例级 save slot 持久化
  - scenario preset 以实例级 `scenario_presets.json` 保存当前 scene / player role / active character
  - trace 支持 latest + 历史轮次列表浏览
- 仍未完成的部分：
  - ~~population promotion 仍未接入真实 runtime attention / interaction 增长逻辑~~ ✅ 已接入：`npcTickExposure` 按 tick 累积在场次数，驱动 population attention 评分
  - 角色仍保留兼容式导入与切换，但这不是最终产品主路径
  - ~~世界层 authoring 还缺 `ruleset / seed / factions / locations / pressures` 的稳定格式~~ ✅ 已落地：`worlds/<world>/world.yml` 支持 factions/locations/pressures 编辑，Planner + Scheduler + Tick 已接入 world structure 驱动
  - 多角色在**单实例内部**仍是共享事件流上的串行 step 协议，不是每角色独立 timeline
  - 实例生命周期与前端实例管理面板已接通；运行台现在区分“默认实例”和“当前视图实例”
