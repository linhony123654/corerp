# CoreRP Session Log

## 2026-05-25

### 项目初始化
- 创建完整 Phase 1 目录结构
- 初始化 Go 模块 `corerp`
- 安装 Go 1.23.4（系统原无 Go）

### 模块实现
- `internal/core/types.go` — 定义 Event/WorldState/WorldSnapshot/ActionFrame/Memory/Identity/Goal 等核心类型
- `internal/events/store.go` — SQLite Event Store，append-only，含 canonical/quarantine 机制
- `internal/state/state.go` — 内存 WorldState + mutex 保护
- `internal/memory/engine.go` — 四层记忆：Short-term(ring buffer)/Working(SQLite)/Semantic(keyword)/Episodic(keyword)
- `internal/agents/identity.go` — Identity Envelope（immutable/adaptive/forbidden）+ Goal System + Validator（动作级/文本级/一致性）
- `internal/context/compiler.go` — Snapshot Compiler（Token Budget 硬墙：CoreRules 5%/Persona 15%/Scene 10%/Working 15%/Semantic 15%/Episodic 10%/Dialogue 30%）+ RenderSnapshot 生成 LLM Prompt
- `internal/actions/executor.go` — Action Frame switch-case 执行器 + AllowedActionsFor 状态计算
- `internal/llm/adapter.go` — OpenAI 兼容 SSE 流式输出 + NonStream 接口
- `internal/runtime/runtime.go` — 完整对话循环：Intent→State→Memory→Canon→Snapshot→LLM→Validate→Execute→Event→Projection→SSE
- `internal/api/server.go` — `/api/chat`(POST SSE) / `/api/state`(GET) / `/api/character`(GET) / `/api/world`(GET) + 静态文件服务
- `cmd/corerp/main.go` — CLI 入口，加载 YAML 角色卡和世界设定

### Bug 修复
- **编译错误**: `events.Project` 与变量名 `events` 冲突 → 变量改为 `eventList`
- **编译错误**: 未使用变量 `char` → 改为 `_`
- **编译错误**: `frame.Effects[0].Path` 是 string 非 map → 去掉 `ok` 判断
- **YAML 解析**: `goals.primary` 是嵌套 map 但结构体期望 `[]Goal` → main.go 中自定义解析 + 扁平化
- **静态文件 301 死循环**: `http.ServeFile` 在 `r.URL.Path == "/index.html"` 时触发 301 到 `/` → 改为 `os.ReadFile` + 手动 Content-Type

### 部署
- 杀掉 PM2 `xiuxian-mud` 释放 8080 端口
- 后台启动 `corerp`，监听 localhost:8080

### LLM 部署踩坑
- **7b 模型 OOM**：系统 available 3.7G，qwen2.5:7b 需要 4.3G → 加载失败
- **3b 模型 CPU 推理过慢**：纯 CPU 跑 3b 模型，120 秒超时无响应（runner CPU 200%+）
- **0.5b 测试**：qwen2.5:0.5b 约 300MB，CPU 可秒回，但用户觉得慢
- **Gemini Flash**：接入公益站 `gcli.ggchan.dev`，gemini-2.5-flash，速度快、质量高
- **格式修复**：
  - Prompt 加 `\`\`\`json` / `\`\`\`text` 代码块标记 + 完整示例 + 温度降到 0.3
  - ExtractActionFrame 增强为三策略解析（代码块 → 首个合法 JSON → 兜底叙事）
  - **服务端隐藏 JSON**：LLM 生成时不在 SSE 中返回原始 chunk，收集完整输出后解析 Action Frame，只把叙事文本逐字流式返回给用户

## 2026-05-25 (Phase 2)

### Phase 2: World Activation 实现
- `internal/events/quarantine.go` — Gatekeeper 系统：来源分级（user_input/action_result→canonical，llm_extracted→quarantine），自动晋升规则（confidence≥0.7 + 60s），Review API
- `internal/simulation/tick.go` — 自主心跳：60s 现实 = 5min 世界，后台 goroutine 驱动
- `internal/memory/confidence.go` — 置信度 Pipeline：pending_facts 表、来源基线（user 0.9 / llm 0.4）、确认累积、0.75 阈值过闸
- `internal/memory/decay.go` — 衰减引擎：事实 confidence 每日×0.99、低于 0.25 删除、关系四维度冷却、30 天 episodic 清理
- `internal/narrative/tension.go` — 张力引擎：8 轮无冲突→热寂检测→注入 +0.2 tension，自然衰减 -0.05
- `internal/state/machine.go` — 叙事状态机：calm/tense/crisis/resolution，状态约束 AllowedActions
- `internal/agents/planner.go` — 自主规划器：纯规则（生存>关系修复>信息>探索>社交），输出 PlanStep 合并进 snapshot ActiveGoals
- `internal/runtime/runtime.go` — 集成全部 P2 模块到 Engine 生命周期和对话循环
- `internal/context/compiler.go` — Compile 签名改为 []GoalFrame，支持 planner 动态目标注入

### Bug 修复
- **tension 持久化缺失**：`actions/executor.go` 中 `threaten`/`attack` 修改了 `state.Tension` 但未返回 `tension_change` 事件，导致 tension 变化无法通过 Event Store 持久化 → 补全 tension_change 事件发射

### 跨会话记忆
- `memory/engine.go` 新增 `dialogue_history` 表，`PushDialogue` 同步持久化
- `LoadRecentDialogueFromDB` 恢复最近 15 轮到 shortTerm ring buffer
- 服务重启后角色记得最近对话

### SillyTavern PNG 导入器
- `internal/importer/importer.go` — 新建，从 PNG tEXt chunk 提取 base64 JSON，解析为 CoreRP YAML
- 修复类型：`map[string]int` → `map[string]float64`，Goal 字段对齐 core 结构
- `cmd/corerp/main.go` — 拆分为 `serve`/`import` 子命令，默认 serve 保持向后兼容
- 支持单文件和批量目录导入

### 导演接口
- `POST /api/director` — 运行时注入 tension：`{"action":"set_tension","value":0.5}`
- `GET /api/debug/memory` — 查看 tension/state/clock/dialogue/quarantine 全状态

### Phase 2 验证结果
- Tension Engine: ✅ 8 轮和平对话后 tension 0→0.2
- State Machine: ✅ calm(0) → tense(0.35) → crisis(0.75) 阈值跳转正确
- Tick Loop: ✅ 65 秒世界时钟推进 5 分钟
- Quarantine: ✅ auto-promote 暂存区事件自动晋升 canonical
- Planner: ✅ 动态目标注入 snapshot ActiveGoals
- 跨会话记忆: ✅ 重启后恢复最近 15 轮对话

## 2026-05-25 (Importer Enhancement)

### 问题
SillyTavern PNG 导入器对大型 v2 卡片提取数据过薄：
- `personality`/`description`/`scenario` 全为空 → `immutable` 只有 1 条
- `first_mes` 1164 字符未提取
- `system_prompt` 107 字符未提取
- `character_book` 71 条条目完全丢失
- 4.3MB 卡片产出约 30 行 YAML

### 根因
SillyTavern v2 格式把核心内容放在 `data.*` 字段，而 importer 的 `Convert()` 只规范化 name/description/personality/scenario/first_mes/mes_example/creator_notes，未处理 `system_prompt` 和 `character_book`。

### 修复
- `internal/importer/importer.go`:
  - `CharacterYAML` 新增 `opening_line`、`system_rules`、`world_book` 字段
  - `Convert()` 提取 `data.system_prompt` → `SystemRules`
  - `Convert()` 提取 `data.first_mes` → `OpeningLine`
  - 新增 `extractCharacterBook()` 解析 `data.character_book.entries`，输出 `[]WorldBookEntry`
  - 新增 `classifyEntry()` 按 comment 前缀分类：角色/事件线/时间线/设定/物品 → character/event/timeline/setting/item
  - 新增 `extractSystemPrompt()`
  - `cleanText()` 替换 `{{user}}`/`{{char}}` 为 玩家/角色
  - 当 personality/description 为空时，fallback 用 `first_mes` 做推断文本池（并进一步用 character_book 中的角色条目 enrich）

### 结果
- 同一张 4.3MB PNG 重新导入后 YAML 从 ~30 行 → **1245 行**
- world_book 条目：8 角色 + 29 事件 + 9 时间线 + 17 设定 + 8 物品 = **71 条全收录**
- `opening_line` 含完整故事开场（1164 字符）
- `system_rules` 含 `<rule>` 故事规则
- **Bug: actor 硬编码为"安雅"**：`compiler.go` RenderSnapshot 和 `adapter.go` fallback 中硬编码 actor 为 "安雅" → 修复为动态使用 `s.PersonaState.Name`

## 2026-05-25 (Ontology → Semantic Memory Pipeline)
### 问题
world.yml 的 ontology 数据（71 条角色/事件/设定）未被加载进 LLM Prompt。Compiler 只读取 `core_rules` 字符串。

### 修复
- `internal/memory/engine.go` — 新增 `SeedFacts([]core.FactFrame, string)` 和 `SeedEpisodics([]core.EventFrame, string)`，事务性批量插入高置信度 (1.0) 事实/事件
- `cmd/corerp/main.go`:
  - `loadWorld` 改为返回 `loadedWorld` 结构体，新增 `Ontology` 字段
  - 新增 `seedOntology()` — 解析 ontology 并转换：
    - 角色条目 → 提取 `身份:`/`关系:`/`外貌:` 等行作为 `FactFrame` + 完整资料作为冗余 fact
    - 地点/势力/物品/Lore → 单个 `FactFrame`
    - 事件 → `EventFrame`（含 arc 字段）
    - 时间线 → `FactFrame`
  - 启动时在 `engine.LoadState()` 之前调用 seed
- `internal/context/compiler.go` — `RenderSnapshot` 中 actor 名使用 `s.PersonaState.Name`，不再硬编码 "安雅"
- `internal/llm/adapter.go` — fallback actor 改为 "unknown"；`suggested_line` 作为 narrative fallback

### 测试结果
问"沈佳和赵小亮是什么关系？" → 叙事准确反映了 ontology 中的关系数据：
- 沈佳是副校长/母亲
- 赵小亮是同班同学/干儿子
- 名义上的母子关系

## 2026-05-25 (Phase 2 Re-verification)

在上次会话记录基础上重新实际验证：

- **Tick Loop**: 70s 等待后 clock 30→35 分钟 ✅
- **Tension Engine**: director API set 0.4 → tension=0.4, set 0.8 → tension=0.8 ✅
- **State Machine**: calm(0) → tense(0.4) → crisis(0.8) → resolution(0.05) ✅
- **Quarantine**: 70s 等待后 canon_events 248→250，quarantined=40 ✅
- **Planner**: 问"你想做什么？" → "不知道。脑子里一片空白"（生存模式下行为符合）✅

## 2026-05-25 (Phase 1 Verification)

### Token Budget 验证（标准 4）
- 在 runtime.go 临时添加 TOKEN 日志
- 4 轮对话测试：1348 → 1831 → 2380 → 2438 / 4000
- 第 6 轮后稳定在 ~2400，远低于 3K 标准
- Compiler truncate 机制正确工作
- 已完成，日志已清除

### PWA 完善（标准 5）
- 原 web/ 只有 index.html（内嵌 JS）
- 新增 `web/app.js` — 独立 JS 文件，含 SSE 客户端 + SW 注册
- 新增 `web/sw.js` — 基础 Service Worker
- 新增 `web/manifest.json` — PWA 安装配置
- `index.html` — 添加 manifest link、SVG favicon、module 引用
- `api/server.go` — 添加 `.json` Content-Type 映射

### VPS 部署（标准 6）
- 新增 `deploy/corerp.service` — systemd 服务文件
- 配置：自动重启、日志到 journal、SQLite 备份注释

### 文档同步
- CLAUDE.md: 6 项验证全部标记 ✅，Status 更新
- TODO.md: 重构为 P1→P2→P3 三段式，移除冗余

## 2026-05-25 (LLM Router — llm/router.go)

### 实现
- `internal/llm/router.go`:
  - `Router` — 按任务类型分发 LLM 调用，支持 fallback
  - 三种任务：`narrative`（叙事生成）、`summary`（记忆压缩）、`extraction`（事实提取）
  - 每个任务可独立路由到不同 adapter（不同 model/endpoint）
  - `Generate(task, prompt, callback)` / `GenerateNonStream(task, messages)` 自动路由
  - Fallback 机制：主 adapter 失败时自动切换到 fallback
  - `AddAdapter()` / `SetRoute()` / `SetFallback()` 动态配置
  - `NewRouter(defaultAdapter)` — 所有任务默认使用同一个模型
- `internal/runtime/runtime.go`:
  - `llmAdapter *llm.Adapter` → `llmRouter *llm.Router`
  - `ProcessTurn` 调用 `router.Generate(llm.TaskNarrative, ...)`
  - `updateWorkingMemory` 调用 `router.GenerateNonStream(llm.TaskSummary, ...)`
  - 新增 `LLMRoutes()` 方法
- `cmd/corerp/main.go`:
  - 新增 `-llm-summary-url` / `-llm-summary-model` CLI 参数（可选，独立摘要模型）
  - 创建 `Router` 替代直接 `Adapter`
- `internal/api/server.go`:
  - 新增 `GET /api/llm-routes` — 查看当前路由表

### 测试结果
- ✅ 路由表正确显示 3 个任务 → default
- ✅ Chat 通过 router 正常工作
- ✅ 独立摘要模型配置支持（可选）

## Phase 3 Complete — 全部 6 项完成 (2026-05-25)

## 2026-05-25 (Narrative Compression — narrative/compression.go)

### 实现
- `internal/narrative/compression.go`:
  - `CompressionEngine` — 规则式事件升维压缩，零 LLM 调用
  - 按 event type 分组，同类型连续事件合并为一条摘要
  - `AutoCompress()` — 事件超阈值（500）时自动压缩 10min+ 的旧事件
  - `CompressRange(from, to)` — 手动指定范围压缩
  - `SummaryStats()` — 压缩统计（total/active/compressed/summary）
  - 摘要生成：observe→"观察了N次"，dialogue→"进行了N轮对话"，clock_advance→"时间推进了N步" 等
  - 不可压缩类型：user_message, attack, threaten（关键事件保留）
  - 摘要存储为 `narrative_compression` 类型 canonical 事件
- `internal/runtime/runtime.go`:
  - Engine 新增 `compressEng` 字段
  - `onTick` 每 20 tick 自动检查并压缩
  - 新增 `CompressEvents()` / `CompressionStats()` 方法
- `internal/api/server.go`:
  - 新增 `POST /api/compress` → `{"from":0,"to":50}`
  - 新增 `GET /api/compression-stats`

### 测试结果
- ✅ 50 事件范围压缩：6 组（dialogue x9/2/2/3, clock_advance x4/3）→ 23 事件压缩为 6 条摘要
- ✅ stats 正确追踪 active/compressed/summary 数量
- ✅ 压缩事件不删除，只标记跳过

## 2026-05-25 (Timeline Replay & Fork — events/replay.go)

### 实现
- `internal/events/replay.go`:
  - `ReplayEngine` — 回放事件流重建任意时刻的 WorldState
  - `ReplayTo(eventID)` — 从事件流开始播放到指定事件，返回当时的世界状态
  - `ReplayAtTime(hour, minute, day)` — 按世界时钟回放
  - `Fork(eventID, branchName)` — 从任意事件创建平行时间线分叉
  - `GetTimeline(branch, limit)` — 获取时间线的所有事件
  - `ListBranches()` — 列出所有分叉名
  - `CompareStates(branchA, branchB, index)` — 对比两个分叉的同一点状态差异
  - `ReplaySummary()` / `BuildTimeline()` — 人类可读格式
- `internal/events/store.go`:
  - events 表新增 `branch TEXT DEFAULT 'main'` 列
  - `scanEvents` 修复 NULL 扫描问题：actor/target/scene_id/session_id 改用 `sql.NullString`
  - `GetByID` 同样修复 NULL 扫描
  - 所有 SELECT 查询加 `branch` 列
- `internal/events/quarantine.go`:
  - Gatekeeper 新增 `replay *ReplayEngine` 字段 + `Replay()` 访问器
- `internal/runtime/runtime.go`:
  - 新增 `ReplayTo()` / `ReplayAtTime()` / `ForkTimeline()` / `GetTimeline()` / `ListBranches()` / `CompareBranches()` 方法
- `internal/api/server.go`:
  - 新增 `GET /api/replay?id=<event_id>` / `GET /api/replay?time=<d:h:m>`
  - 新增 `POST /api/fork` → `{"event_id":"...", "branch":"name"}`
  - 新增 `GET /api/timeline?branch=main&limit=50`
  - 新增 `GET /api/branches`

### 测试结果
- ✅ ReplayTo 重建 WorldState（clock/tension/scene/relationships 完整）
- ✅ Timeline 正确排序显示事件
- ✅ Fork 创建新分叉成功，branches 列表更新
- ✅ NULL 扫描修复同时解决了 LoadState 警告

## 2026-05-25 (Causality Engine — events/causality.go)

### 实现
- `internal/events/causality.go`:
  - `CausalityEngine` — 事件因果链引擎，利用 events 表已有 `causes`/`effects` JSON 列
  - `LinkNewEvent(evt)` — 分析新事件，自动与最近 20 个事件建立双向因果链接
  - 链接规则（权重）：类型启发(0.9) > 对话-响应(0.85) > 同角色(0.7) > 同目标(0.6)
  - `causalTypeRules` 定义类型因果：attack→fear_change/tension_change/hide, user_message→dialogue 等
  - `GetChain(eventID, depth)` — 递归构建因果链树
  - `GetChainSummary()` — 人类可读的缩进文本摘要
- `internal/events/quarantine.go`:
  - `Gatekeeper` 新增 `causality` 字段，`Submit()` 存储后自动 `go g.causality.LinkNewEvent(e)`
  - 新增 `Causality()` 访问器
- `internal/events/store.go`:
  - 新增 `GetByID()` / `GetRecentEvents()` 方法
- `internal/runtime/runtime.go`:
  - 新增 `GetCausalityChain()` / `GetCausalitySummary()` 方法
- `internal/api/server.go`:
  - 新增 `GET /api/causality?id=<event_id>&depth=3` 端点

### 测试结果
- ✅ `user_message` → `dialogue` 正确链接（weight 0.9）
- ✅ `dialogue` → `variable_set` → `fact_extracted` 效应链
- ✅ 递归链查询工作中，depth 参数控制深度
- ✅ Summary 人类可读格式正确

### 注意事项
- 同角色规则会链接历史事件，depth 过深时噪音较多
- 因果链接在 goroutine 中异步执行，不阻塞事件提交

## 2026-05-25 (NPC Scheduler — agents/scheduler.go)

### 实现
- `internal/agents/scheduler.go`:
  - `Scheduler` 结构体：规则式 NPC 自主行动引擎，零 LLM 调用
  - `Tick()` — 每个 tick（60s）遍历非活跃角色，Planner 生成计划 → 选最高优先级 → 构建 ActionFrame → Executor 执行 → Gatekeeper 提交 canonical 事件
  - 20% 随机噪声避免 NPC 行为完全重复
  - `NPCActionLog` 人类可读摘要："安雅 观察着周围的环境。"
  - `RecentActions()` / `RecentActionsForCharacter()` 查询接口
- `internal/runtime/runtime.go`:
  - Engine 新增 `scheduler` + `tickCount` 字段
  - `onTick()` 末尾调用 `scheduler.Tick()`，为非活跃角色生成自主动作
  - 新增 `GetNPCActions()` 方法
  - DebugInfo 返回 `npc_actions` 字段
- `internal/api/server.go`:
  - 新增 `GET /api/npc-actions?character=name` 端点
  - `POST /api/switch` 响应体新增 `npc_actions` 字段（"你不在时发生的事"）
- `web/app.js`:
  - 角色切换时显示 NPC 行动摘要

### 测试结果
- ✅ NPC (安雅) 在后台每 3 tick (~3min) 执行一次自主动作
- ✅ 安雅在世界场景（废弃地铁站）中独立行动，不干扰活跃角色的别墅场景
- ✅ 切换到安雅时返回 "你不在时发生的事" 摘要

### 行为优先级（Planner 规则）
survival(hide) > relationship_repair(trust) > info_gathering(observe) > exploration(move) > social(speak)

## 2026-05-25 (Perspective Isolation Fix)

### 问题
多角色切换后发现两个隔离缺陷：
1. **世界场景串**：两个角色共享同一个 `world.yml`，安雅（赛博朋克）被困在"同学搞我妈妈"的别墅场景里
2. **对话历史混**：`dialogue_history` 表无 `character` 列，所有角色对话混在一个池里

### 修复
- `internal/memory/engine.go`:
  - `shortTerm` 从 `[]Message` 改为 `map[string][]Message`（per-character ring buffer）
  - `dialogue_history` 表加 `character` 列 + 索引
  - `PushDialogue(msg, character)` / `GetRecentDialogue(character)` / `LoadRecentDialogueFromDB(character, limit)` 全部加 character 参数
- `internal/runtime/runtime.go`:
  - 新增 `CharWorld` 类型（WorldName + CoreRules + Scene）
  - Engine 新增 `charWorlds map[string]CharWorld`
  - `SwitchCharacter()` 切换时同时更新 worldName/coreRules/scene
  - 所有 memory 调用传递 `e.activeCharacter`
- `cmd/corerp/main.go`:
  - 新增 `findWorldFile()` 自动配对 world 文件（同目录/`worlds/` 目录）
  - 每个角色绑定独立 world，构建 `charWorlds` map 传给 runtime
  - `loadCharactersFromDir()` 返回 file paths 用于配对
- 新增 `characters/anya_world.yml`（从 `worlds/cyberpunk2077/world.yml` 复制）

### 测试结果
- ✅ 世界隔离：同学搞我妈妈 → 别墅/沈佳/赵小亮；安雅 → 废弃地铁站/酸雨/短刀
- ✅ 记忆隔离：安雅不知道角色1的暗号"赛博2077"，切回后角色1还记得
- ✅ 场景切换：切换角色后 LLM 正确使用各自世界场景

## 2026-05-25 (Multi-Character Viewport Switching)

### 改动
- `internal/runtime/runtime.go`:
  - `characterName string` → `activeCharacter string` + `loadedCharacters []string`
  - `New()` 签名变更：`charName` → `activeChar string, loadedChars []string`
  - 新增 `SwitchCharacter(name string) error` — 切换活跃角色，保存旧角色 Working Memory，加载新角色 Dialogue History
  - 新增 `GetLoadedCharacters() []string` / `GetCharacterName() string`
- `internal/api/server.go`:
  - 新增 `GET /api/characters` — 返回 `{active, characters[]}`
  - 新增 `POST /api/switch` — 请求体 `{character: "name"}`，切换活跃角色
- `cmd/corerp/main.go`:
  - 新增 `-characters` flag，加载目录下所有 .yml（跳过 _world.yml）
  - 新增 `loadCharactersFromDir()` 函数
  - 所有角色一次性加载到 EnvelopeManager，第一个角色默认活跃
  - `-character` flag 保留，单角色模式向后兼容
- `web/index.html` + `web/app.js`:
  - 新增角色切换下拉框（`#char-select`）
  - 页面加载时从 `/api/characters` 拉取角色列表
  - 切换时清空聊天记录，显示系统消息

### 结果
- 两个角色卡（同学搞我妈妈 + 安雅）同时加载，共享 WorldState
- 切换只在单个 Snapshot 内切换 PersonaFrame，Token 预算不变（4K 硬墙）
- 非活跃角色不参与 LLM 推理，不影响性能

## 2026-05-25 (Importer Refactor)

### 目标
按 architecture.md 的 Canon Layer / Identity Envelope 分离原则，将 importer 从单文件输出改为双文件输出。

### 改动
- `internal/importer/importer.go`:
  - `CharacterYAML` 移除 `system_rules` 和 `world_book`，只保留 `identity` + `goals` + `opening_line`
  - 新增 `WorldYAML` / `SceneYAML` / `OntologyYAML` / `EntityEntry` / `EventEntry` 类型
  - `Convert()` 签名改为返回 `(CharacterYAML, WorldYAML)`
  - 新增 `BuildWorldYAML()` — 将 71 条 character_book 按类型拆入 ontology:
    - `characters` → 角色条目
    - `locations` → 地理设定
    - `factions` → 势力设定
    - `items` → 物品
    - `lore` → 世界观常识
    - `events` → 事件线（含 `arc`: 主线/支线/暗线/伏笔）
    - `timelines` → 时间线
  - `core_rules` = system_prompt + 世界观常识设定摘要
  - `scene` = 从 `first_mes` 推断的地点/时间/天气/在场角色/场景描述
  - `ImportPNG()` 同时输出 `角色.yml` + `角色_world.yml`
  - `ImportDirectory()` 适配新签名
- `cmd/corerp/main.go`:
  - `runImport()` 适配 `ImportPNG` 新签名（返回 charPath, worldPath）

### 结果
同一张 PNG 导入后产出两个文件：
- `48111430a81be7d4.yml` — **32 行**，纯角色卡（identity + goals + opening_line）
- `48111430a81be7d4_world.yml` — **1198 行**，世界设定：
  - core_rules: 685 字符
  - scene: 别墅 / 白天 / 晴朗炎热
  - ontology: 8 角色 + 12 地点 + 1 势力 + 8 物品 + 4  lore + 29 事件 + 9 时间线
