# CoreRP Closure Audit

> 目的：不是继续描述“做过什么”，而是按 `ACCEPTANCE_CHECKLIST.md` 逐项核对“当前是否已经足以证明闭环完成”。
> 结论先行：**当前仍不能判定为终态闭环已完成**，但多数主链已经有强证据支撑。

## 审计结论

当前状态更接近：

- `World-First 主语义`：**半完成，接近验收**
- `Population / Director / World Structure / Autonomous Simulation`：**已实现，且已有强运行证据**
- `Trace / Authoring / 作者干预`：**已形成可用工作流，但距离“完整 runtime 调试器 / 稳定运营台”还有差距**

因此当前不能把项目标记为“闭环完成”。

---

## 1. World-First 主语义

状态：`半完成`

当前证据：

- runtime 已将 `focus_character` 与 scene truth 拆开。
- `/api/characters`、`/api/instances`、前端参与者面板已接入 `participants` / `participant_details`。
- `/api/characters` 现已只返回 `focus_character / participants / participant_details`，不再在主响应里附带 `active / characters` 顶层镜像。
- 前端主读取路径已不再依赖 `/api/character`、`/api/characters.active`、`/api/characters.characters` 这类旧兼容字段。
- `/api/characters` 已不再在空场景时回退到 `loaded_characters`。
- runtime 的 `GetSceneParticipants()` / `GetSceneParticipantDetails()` 也已不再在空场景时回退到 loaded roster。
- `POST /api/instances/create` 已不再让 `active_character` 决定新实例的 focus。
- `SaveSlot`、`ScenarioPreset`、`TurnTrace` 的 runtime 内部读取已统一优先 `focus_character`，旧 `character` 只在读取旧数据时做一次性兼容迁移；`/api/checkpoints`、`/api/presets`、`/api/trace*` 公开响应不再输出 legacy `character` 镜像。
- `MemorySnapshot`、`PendingFact` 与 `/api/memory`、`/api/pending-facts`、`/api/quarantine` 顶层响应也已改为优先 `focus_character`；`/api/pending-facts` 的 fact 条目不再输出旧 `character` 镜像。
- `ExperimentSnapshot`、`ExperimentReport` 与 archived `latest_trace` 也已改为优先 `focus_character`，归档回放不会再被 legacy `character` 反向污染。
- `POST /api/worlds`、`/api/quarantine`、`/api/pending-facts`、`/api/npc-actions` 这些主接口也已移除顶层 `character` 兼容镜像，避免主消费面继续把 legacy 字段当成主语义。
- `/api/memory` 与 `/api/export` 也已移除顶层 `character` 兼容镜像；`MemorySnapshot` API 契约也已移除 legacy `character` 字段，memory 顶层只保留 `focus_character`，export 顶层只保留 `focus_definition + focus_character` 作为主语义。
- canonical `/api/focus-definition` 与 `/api/focus-definition-config` 已上线，前端与主测试已经改走新路径；`CharacterConfig` API 契约已移除 legacy `character` 字段；legacy `/api/character`、`/api/character-config` 仍在，但已更明确地退回兼容层。
- legacy `/api/character`、`/api/character-config` 现在还会显式返回 `Deprecation: true` 与 successor `Link` 头，运行态上也能看出它们只是兼容入口。
- `/api/worlds`、`/api/export` 这类作者侧公开输出也已明确站在 `focus_character / focus_definition / participants / participant_details` 上，旧 `character` 仅保留镜像。
- `RuntimeInstanceSummary` 也已明确区分：
  - `focus_character` = viewpoint
  - `participants` = scene truth
  - `participant_details` = unified participant model
- `RuntimeInstanceSummary` 主类型现在已不再携带 `active_character / loaded_characters`；前端实例创建与待审面板文案已统一改成“视角”
- `/api/instances`、`/api/instances/status`、`/api/instances/create` 与 `/api/state.instance` 这些主实例出口现在也已只暴露 canonical instance 摘要，不再把 `active_character / loaded_characters` 作为公开主载荷返回
- `/api/runtime-audit.instance` 与 `/api/experiment-reports/replay` 里的 replay branch 摘要也已改为 canonical instance payload，作者侧归档/复现链路不会再把 legacy instance 字段重新带回前端
- runtime 的 `Engine.InstanceSummary()` 现在也已停止主动填充 `active_character / loaded_characters`；主类型已不再携带这两个字段
- runtime 也已停止主动填充 `TurnTrace / TurnStepTrace / SaveSlot / ScenarioPreset / CharacterConfig / MemorySnapshot` 上的空 `character` 镜像；这些对象的 legacy 字段现在默认 `omitempty`，canonical 主路径不会再因为空字符串而把它们公开出去
- runtime compatibility normalization 现在会把旧 `character` 迁移进 `focus_character / step.speaker` 后立刻清空 legacy 镜像；旧数据仍可读，但新的 runtime trace/save/preset/pending-fact 输出不会继续携带旧字段值。
- API 层 compatibility normalization 也已改成单向兼容；`/api/trace`、`/api/checkpoints`、`/api/presets`、`/api/pending-facts` 不会再把 runtime 已退空的 legacy `character` 镜像重新补回响应
- `core` 类型层里现存的公开 `character` 字段现在也都已退成 `omitempty`；legacy 字段只有在旧值真实存在时才会进入 JSON
- API contract 已新增 canonical schema 防回流测试：`MemorySnapshot / SaveSlot / ScenarioPreset / CharacterConfig / RuntimeInstancePayload / ExperimentSnapshot / TurnTrace` 不允许重新出现 `character / active_character / loaded_characters`。
- Runtime Audit / World Experiment Panel / checkpoint browser / proof bundle / step trace 这些作者控制台主视图已移除 `slot.character / selectedCheckpoint.character / bundle.selected_checkpoint.character / stepTrace.character` 前端回退读取，主 UI 现在只消费 `focus_character / speaker` 这类 canonical 字段。
- API server 主接口也已不再要求 `GetLoadedCharacters()` 这类旧 accessor 参与主链路。
- `/api/state` 现在会显式返回 `focus_character / participants / participant_details`，调用方不再需要从 `instance.active_character` 之类兼容镜像倒推当前语义。
- 已有切换视角不收缩场景的测试与前端路径。

主要证据位置：

- `internal/runtime/runtime.go`
- `internal/api/server.go`
- `web/app.js`

仍未通过的原因：

- 兼容字段 `character / active_character / loaded_characters` 仍然存在，但其公开暴露面已继续收缩到兼容接口、兼容 accessor 与部分旧存储模型。
- 旧字段已经不再决定主要 UI / API 路径，也不再反向污染 memory / pending-fact / main instance payload / runtime-audit replay payload / Runtime Audit 前端主读取；同时 runtime 也不再主动制造 `active_character / loaded_characters`，`RuntimeInstanceSummary` 主类型也已移除这两个字段，trace/save/preset/config/memory 这批对象也不再主动填空 `character` 镜像，API 层也不会再把这些空镜像补回响应。但若干兼容接口与底层 legacy 字段仍在，因此还不能判定兼容层已经完全退居末端。

---

## 2. Population -> Persona 晋升闭环

状态：`强实现，接近验收`

当前证据：

- background NPC attention / promotion / identity core 已进入主链路。
- autonomous tick 会主动把 scene-local / pressure-hit population 拉入 runtime。
- `world_pressure` 现在也会进入 population attention 主链路，持续命中的 faction/location NPC 不会再在真实 world 目录里“有压力但不成长”。
- population attention 现在使用 72h 滚动事件窗口，旧事件不会永久抬分；promoted NPC 若长期脱离 scene/pressure/event，会退回 background，并生成 `population_demoted` canonical event，`/api/population-insights` 的 history 也能读到 demotion。
- 已有：
  - 36 tick promotion 测试
  - promotion / demotion / insights history 的 lifecycle contract 测试
  - 120 tick 多样本矩阵测试
  - 200 tick / 4 样本矩阵测试
  - 基于真实世界目录的 120 tick 矩阵测试（`neon_block`、`1_7`、`《红楼梦》完整版、-角色卡-202604190812`）
  - 基于真实世界目录的 200 tick 矩阵测试（同样覆盖 `neon_block`、`1_7`、`《红楼梦》完整版、-角色卡-202604190812`）
  - API 层长窗口测试

主要证据位置：

- `internal/runtime/population_runtime.go`
- `internal/runtime/runtime_test.go`
- `internal/api/server_test.go`

仍未通过的原因：

- 已不再只是单一 guard / smuggler 样本；真实世界目录的都市世界与两类导入世界也已经补到 runtime/API 双层 200 tick，但样本规模仍不足以证明“多数世界都能稳定自然长出主要角色”。
- demotion 已补到可解释 lifecycle contract，但还缺更大样本的真实世界长期运营验证。

---

## 3. 人物自然成长闭环

状态：`强实现，待更长窗口验收`

当前证据：

- identity drift 已影响：
  - future allowed actions
  - director winner
  - desire / autonomous intent
  - scheduler 选步
  - 同 tick relationship outcome
  - 多 tick trust-action trajectory
- 相关链路已经有 runtime / agents 测试。

主要证据位置：

- `internal/runtime/turns.go`
- `internal/runtime/population_runtime.go`
- `internal/agents/scheduler.go`
- `internal/emotion/desire.go`

仍未通过的原因：

- 现在已经能证明长期人口/结构压力会在 200 tick 内持续改写出场人物与 promoted leader，但还没有足够长窗口、足够多样本证明人格慢变量本身会长期稳定塑造 world outcome，而不是只塑造局部行为。

---

## 4. Director 选人闭环

状态：`已实现，待验收`

当前证据：

- director 已按 scene / pressure / faction / relationship / participant source 等结构因素选人。
- scene shell / player role 不再混入候选。
- candidate details / score breakdown / dominant factors / world signals 已可见。

主要证据位置：

- `internal/runtime/director.go`
- `web/app.js`
- `internal/runtime/runtime_test.go`

仍未通过的原因：

- 还缺更广样本证明 follow-up / chain speaker 长期质量稳定。
- 还没有正式证明旧兼容语义已完全退居兼容层。

---

## 5. World Structure 驱动闭环

状态：`强实现，接近验收`

当前证据：

- structure 改动会影响 planner / scheduler / tick / director / diagnostics。
- 已有：
  - 干预前后对照测试
  - 36 tick 分叉测试
  - 120 tick 多样本矩阵测试
  - 200 tick / 4 样本矩阵测试
  - 基于真实世界目录的 120 tick 矩阵测试（覆盖原生都市世界与导入世界）
  - 基于真实世界目录的 200 tick 矩阵测试（runtime / API 双层都已补上）
  - API 层长窗口分叉测试

主要证据位置：

- `internal/runtime/runtime_test.go`
- `internal/api/server_test.go`
- `web/app.js`

仍未通过的原因：

- 现在已经证明结构影响可持续到 200 tick，并且不只体现在 guard / smuggler 双样本；同时真实世界目录也已补到都市世界与导入世界，但仍缺更大样本池与作者侧复现实验来证明其普适性。

---

## 6. Autonomous Simulation 闭环

状态：`强实现，待更长窗口验收`

当前证据：

- 世界可在无用户输入时持续推进。
- tick 可产出 pressure / faction / population / event 演化。
- 可暂停、恢复、手动 tick、批量 tick、双实例同步推进。
- 已有 36 tick、120 tick、200 tick 级别证据，并已覆盖真实世界目录样本；`neon_block / 1_7 / 红楼梦导入世界` 已在 runtime/API 双层完成 200 tick 验证。

主要证据位置：

- `internal/runtime/runtime.go`
- `internal/api/server.go`
- `web/app.js`

仍未通过的原因：

- 200 tick 与真实 world family 双层证据已经补上，作者侧真实 runtime replay round-trip 也已补上；但还缺更大规模长期回放样本，来证明世界不会在更大规模运营下退化成噪音或空转。

---

## 7. Trace / Authoring 可解释闭环

状态：`强工作流，待验收`

当前证据：

- 作者可看到：
  - 单一 `runtime audit` 聚合视图（trace / sim / population / checkpoint / preset / report）
  - `runtime audit` 第一版按原因筛选（director / pressure / faction / population / archive）
  - `runtime audit` 第一版按阶段回放（基于 trace `step_traces`）
  - `runtime audit` 第一版实验归档复现（可展开 archived report，回放其中 current/compare latest trace，可直接把 archived current/compare checkpoint 恢复回实例，可一键派生新的 current/compare 复现实例继续跑；而且 archive 区还支持批量派生复现 / 批量刷新复现，并会生成 `Experiment Portfolio` 批量结果矩阵与 `World Baselines` world-family 聚合摘要，汇总 reports/worlds/replay loaded 数量、每条实验的 archived/live 主导侧，以及每个 world family 的 archived/live split、最近两次 tension/trajectory/population 漂移、稳定/波动/分叉长期状态与最新 trajectory；这些 baselines 现在还可进一步按 `world_name` 直接派生该世界、刷新该世界、推进该世界已加载 replay branches、导出该世界基线，而不必每次对所有 reports 全量 replay；其中“按世界批量派生”已收敛到正式后端接口 `/api/experiment-reports/replay-batch`，“推进 replay” 也已收敛到正式后端接口 `/api/experiment-reports/replay-advance`，不再只是前端逐条循环；同时 archive/report 列表现在也支持“聚焦该世界”，可把 portfolio、archive rows 与批量 replay workflow 收敛到单一 world scope；archive 区还内置了基础 `Checkpoint Browser`，可直接选中 checkpoint 查看 scene/focus/player-role/world-state 摘要与 `checkpoint vs live` 差异，再决定是否恢复；实验报告详情现在还内置 `Ops Matrix`，把 archived current/compare、选中 checkpoint、live replay 放进同一张对照卡，减少作者在多张卡片间手工比对；`World Experiment Ops` 卡现在还会直接列出当前 world scope 下最近几份报告、checkpoint 锚点与 replay 是否已加载，作者可从世界运营卡直接跳到目标 report，并通过 `Focused Report / Focused Checkpoint` 中心选择位同步 archive 与 runtime-audit 上下文；在这之上，archive 区现在还新增单屏 `World Experiment Panel`，把当前 live world、selected report、selected checkpoint、baseline gap 与 focused replay 摘要压进同一张卡，先把“当前世界实验到底跑到哪了”集中看清，再进入更细的 baseline/checkpoint/report/replay 证据卡，并且现在可直接从该卡推进 focused replay 或整个 world scope 下已加载的 replay branches 若干 ticks，然后立刻刷新 live divergence 证据；这些 replay branches 还可直接被切回“当前实例 / 对照实例”运营流，作者不必再回实例列表手动寻找目标分支；同时现在还能把当前 world scope 的 baseline / selected report / selected checkpoint / focused replay / live-vs-baseline / batch summary 一次导出成 proof bundle（JSON / Markdown），便于交给后续模型或人工复核；这些 baselines 还可直接导出为 JSON / Markdown 快照留档，并能进一步对照“当前实例 vs 所属 world baseline”的偏移，直接回答当前 tension 是否还在基线区间、trajectory/population 是否已经跑偏；在此基础上还能先在 archived checkpoint 层直接看到 scene/participants/pressure/faction/diagnostics/trajectory/latest-trace 的结构化差异解释；之后再读取 replay branches 的 live pressure/faction/population/diagnostic split、director/world-signal/latest-trace/population driver 证据、latest trace `step_traces` 差异，以及基于 recent ticks / recent turns 的 divergence timeline；其中 turn 分叉还可直接 drill down 到对应实例和 trace，并优先打开按 trace/step/handoff 顺序定位出来的首个分叉事件 causality chain，找不到严格命中时再回退）
  - participant details
  - director gaps / world signals
  - sim diagnostics
  - tick history
  - trajectory summary
  - 跨实例结果对照
  - 自动实验结论
  - 实验报告归档与导出
  - 实验报告中的 latest trace / director / participants 证据
  - 可重复执行的长期证据脚本 `scripts/run_world_proof_audit.sh`，能把 API world-first contract 检查（含 canonical schema 防回流）、API author replay contract 检查、API proof archive contract 检查、runtime population lifecycle contract 检查、runtime/API 两层的 200 tick sample matrix 与 real-world matrix 跑完后落盘到 `data/proof-audits/<timestamp>/`
  - replay 派生 / 批量派生 / 推进响应现在会直接携带 current/compare evidence（sim status、latest trace、population、audit summary），前端会把这些 evidence 作为 audit 拉取失败时的 fallback，减少作者侧“推进成功但证据空白”的断链
  - author replay contract 现在还包含真实 runtime round-trip：真实 current/compare 实例保存 checkpoint/report 后，可经 `/api/experiment-reports/replay-batch` 派生 replay branches，复制 archived checkpoint 锚点到 replay 实例，随后经 `/api/experiment-reports/replay-advance` 推进并返回真实 tick evidence

主要证据位置：

- `web/index.html`
- `web/app.js`
- `internal/runtime/runtime.go`
- `internal/runtime/authoring.go`
- `internal/api/server.go`
- `internal/api/server_test.go`（现已显式断言 runtime audit 会暴露 archived `current_checkpoint / compare_checkpoint`）
- `internal/api/server.go` / `internal/api/server_test.go`（现已新增 `/api/proof-audits`，可列出最近 proof audit 归档并读取 summary preview / files）
- `internal/api/server_test.go`（`TestExperimentReportReplayBatchRealRuntimeRoundTrip` 证明 replay 工作流不再只是 mock contract）
- `scripts/run_world_proof_audit.sh`
- `data/proof-audits/20260528T033924Z/SUMMARY.md`（当前最新一轮真实落盘结果：8/8 PASS，覆盖 world-first contract、author replay contract（含真实 runtime replay round-trip）、proof archive contract、population lifecycle、runtime/API 200 tick sample matrix 与 real-world matrix）

仍未通过的原因：

- 还不是完整的 runtime 调试器。
- 虽然 `runtime audit` 已经把 trace、sim、checkpoint/preset/report 聚到单一读取面，并补上了第一版按原因筛选、第一版按阶段回放，以及带 checkpoint 恢复、checkpoint 差异解释、复现实例派生、全量与按世界批量派生/批量刷新复现/批量推进 replay、批量结果矩阵、world-family 基线摘要、单屏 `World Experiment Panel`、稳定/波动/分叉长期状态、live replay driver split、latest trace `step_traces` 差异、divergence timeline、turn-level drill down 与首个分叉事件 causality chain 的第一版实验归档复现；replay API 现在也会直接随响应返回 current/compare evidence。但这些能力还不够完整，距离完整调试器级别仍有差距。

---

## 8. 作者干预闭环

状态：`强工作流，待验收`

当前证据：

- 作者可：
  - 改 world structure / population
  - 创建实验分支实例
  - 设对照实例
  - 批量推进 ticks
  - 双实例同步推进
  - 自动读取实验结论
  - 保存、回看、导出带 trace/director 证据的实验报告

主要证据位置：

- `internal/api/server.go`
- `internal/api/server_test.go`
- `web/app.js`
- `web/index.html`

仍未通过的原因：

- 还没有证明这套作者工作流在“更多世界样本 + 更长窗口”下稳定可靠。
- 报告归档、真实 runtime replay 派生与 replay 推进已经落地，但还缺更大规模实验回放来证明它足以支撑终态运营。

---

## 最终六问

### 1. 世界自己能转吗？

当前判断：**基本可以回答“是”**

### 2. 人物会被世界改变吗？

当前判断：**基本可以回答“是”**

### 3. 新主要角色能从人口层自然长出来吗？

当前判断：**在当前长窗口样本中可以回答“是”**

### 4. director 会顺着世界状态而不是角色卡来选人吗？

当前判断：**大体可以回答“是”，但仍缺更广样本**

### 5. 作者能看懂世界为什么这样运行吗？

当前判断：**部分可以，但还未达到“完整调试器”标准**

### 6. 作者能稳定干预世界，而不是靠人工救火吗？

当前判断：**已形成强工作流，但还未达到最终验收强度**

---

## 当前真正剩余缺口

不是“再补一个字段”或“再堆一个角色卡”，而是这几项：

1. 兼容层收束：证明旧 `character / active_character / loaded_characters` 已不再反向主导新语义。
2. 更大规模 authoring 回放：用已落地的实验归档去跑更多世界/更多分支，而不是只看单次 live 面板。
3. Trace / Authoring 统一化：把现有 trace、sim、checkpoint/preset/report 再收束成更系统的运行审计面。
4. 最后一次严格 closure review：按本文件逐项判断是否能把状态从“待验收”升级成“已完成”。
