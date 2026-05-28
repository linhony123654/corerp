# CoreRP Delivery Tracking

> 这份文档记录的是**实现推进状态**，不是终态闭环验收结果。
> 终态是否真正完成，统一以 `ACCEPTANCE_CHECKLIST.md` 为准。
> 当前逐项证据与剩余缺口，统一以 `CLOSURE_AUDIT.md` 为准。

## 状态定义

- `已实现`：代码和链路已经存在
- `待验收`：机制已接入，但还没有达到终态闭环判断标准
- `已验收`：满足 `ACCEPTANCE_CHECKLIST.md` 对应项的完成标准

当前项目整体判断：

- 功能骨架：完成度高
- 终态闭环：多数仍处于 `已实现 / 待验收`

---

## 验收项 1：World-First 主语义

- 状态：`已实现，待验收`

当前实现：

- 前端主读取路径已经以 `focus_character` 为主
- 前端主读取路径已不再依赖 `/api/character` 与 `/api/characters` 的 `active / characters` fallback
- `/api/characters` 主响应已移除顶层 `active / characters` 兼容镜像，只保留 `focus_character / participants / participant_details`
- `/api/characters` 已不再在空场景时退回 `loaded_characters`
- runtime 的 scene participants / participant details 也已不再在空场景时退回 loaded roster
- `SaveSlot` / `ScenarioPreset` / `TurnTrace` 的 runtime 内部读取也已统一改为优先 `focus_character`；`/api/checkpoints`、`/api/presets`、`/api/trace*` 公开响应不再输出 legacy `character` 镜像
- `MemorySnapshot` / `PendingFact` 与 `/api/memory`、`/api/pending-facts`、`/api/quarantine` 顶层 focus 回传也已统一优先 `focus_character`；`/api/pending-facts` 的 fact 条目不再输出 legacy `character` 镜像
- `RuntimeInstanceSummary` 已明确以 `participants` 表示 scene truth，主类型不再携带 `active_character / loaded_characters`
- `/api/instances`、`/api/instances/status`、`/api/instances/create` 与 `/api/state.instance` 这些主实例出口也已改成 canonical instance 摘要，不再公开 `active_character / loaded_characters`
- `/api/runtime-audit.instance` 与 `/api/experiment-reports/replay` 返回的 replay branch 摘要也已改成 canonical instance payload，作者侧归档/复现主链不再重新暴露 legacy instance 字段
- runtime 的 `InstanceSummary()` 已停止主动填充 `active_character / loaded_characters`，主类型也已移除这两个字段
- runtime 也已停止主动填充 `TurnTrace / TurnStepTrace / SaveSlot / ScenarioPreset / CharacterConfig / MemorySnapshot` 上的空 `character` 镜像；这些对象的 legacy 字段现在默认 `omitempty`
- runtime compatibility normalization 现在会把旧 `character` 迁移进 `focus_character / step.speaker` 后立刻清空 legacy 镜像；旧数据仍可读，但新的 runtime trace/save/preset/pending-fact 输出不会继续携带旧字段值
- API 层 compatibility normalization 也已改成单向兼容；`/api/trace`、`/api/checkpoints`、`/api/presets`、`/api/pending-facts` 不会再把 runtime 已退空的 legacy `character` 镜像补回响应
- API contract 已新增 canonical schema 防回流测试：`MemorySnapshot / SaveSlot / ScenarioPreset / CharacterConfig / RuntimeInstancePayload / ExperimentSnapshot / TurnTrace` 不允许重新出现 `character / active_character / loaded_characters`
- Runtime Audit / World Experiment Panel / checkpoint browser / proof bundle / step trace 等作者控制台主视图已移除 `slot.character / selectedCheckpoint.character / bundle.selected_checkpoint.character / stepTrace.character` 前端回退读取，主 UI 现在只消费 `focus_character / speaker`
- `core` 类型层里现存的公开 `character` 字段也都已退成 `omitempty`
- `ExperimentSnapshot` / `ExperimentReport` 与 archived `latest_trace` 也已统一优先 `focus_character`
- `POST /api/worlds`、`/api/quarantine`、`/api/pending-facts`、`/api/npc-actions` 这些主接口也已移除顶层 `character` 兼容镜像
- `/api/memory` 与 `/api/export` 这两个公开主接口也已移除顶层 `character` 兼容镜像；`MemorySnapshot` API 契约也已移除 legacy `character` 字段
- canonical `/api/focus-definition` 与 `/api/focus-definition-config` 已成为主消费路径；`CharacterConfig` API 契约已移除 legacy `character` 字段，legacy `/api/character`、`/api/character-config` 仅保留兼容入口
- legacy `/api/character`、`/api/character-config` 现在还会显式返回 `Deprecation: true` 与 successor `Link` 头
- `/api/worlds` / `/api/export` 已明确以 `focus_character / focus_definition` 为主语义，并显式返回 participants / participant_details
- `/api/instances/create` 已不再让 `active_character` 决定实例 focus
- API server 主接口已不再要求 `GetLoadedCharacters()` 参与主链路
- `/api/state` 已显式返回 `focus_character / participants / participant_details`
- API 主读取路径已经以 `focus_character` 为主
- `/api/characters`、`/api/instances`、trace 已接入 `participant_details`
- runtime 已把 `focus_character`、`participants`、scene truth 拆开

为什么还不能算已验收：

- 兼容字段仍大量存在
- 旧 `character / active_character / loaded_characters` 公开面已继续收缩，runtime 也不再主动制造 `active_character / loaded_characters`，`RuntimeInstanceSummary` 主类型已移除这两个字段，并停止主动填空 `character` 镜像；runtime/API 层和 Runtime Audit 前端主读取也不会再把这些空镜像补回主路径，但还没有完全退化为纯兼容层
- 前端与关键 API 主路径虽已切走旧字段，memory / pending-facts / instance summary 这类晚到路径也已收口；但类型层与兼容接口仍未完全收束

建议验收动作：

- 用只读 `focus_* + participants + participant_details` 的前端路径跑完整流程
- 验证切视角不会再改变 scene truth

---

## 验收项 2：Population -> Persona 晋升闭环

- 状态：`已实现，待验收`

当前实现：

- `calculatePopulationAttention()` 已接入多种 attention 驱动因素
- autonomous tick 现在会显式把 scene/location/faction/pressure 命中的 background NPC 拉入 scene runtime
- `world_pressure` 现在也会进入 population attention，真实 world 目录里的受压 faction/location NPC 不会再出现“世界压力在跑，但人口层不长”的断链
- promoted NPC 可进入 runtime 与 director candidate
- 已有 demotion 机制；attention 使用 72h 滚动事件窗口，promoted NPC 长期脱离 scene/pressure/event 后会退回 background，并留下 `population_demoted` canonical event 与 insights history
- `population-insights` 能观察到 background / promoted / identity 数据

为什么还不能算已验收：

- 已有 30+ tick 长窗口测试，以及 120 tick、200 tick、多样本矩阵和基于真实 world 目录的 `neon_block / 1_7 / 《红楼梦》完整版、-角色卡-202604190812` runtime/API 双层 200 tick 验证，证明不经过 directTurn/director 也能把 scene 相关 background NPC 自然拉入、累积 exposure 并触发 promotion；但仍缺更大样本池证明“新的主要人物能稳定自然长出来”
- promotion/demotion 已有 lifecycle contract 验证，但还缺更大样本的真实世界长期运营回放
- 目前更像“有机制”，不等于“世界人口真的会稳定生长”

建议验收动作：

- 跑长期 tick
- 连续观察 `/api/population-insights`
- 确认 attention、promotion、demotion、identity 变化是可解释的，不是噪音

---

## 验收项 3：人物自然成长闭环

- 状态：`已实现，待验收`

当前实现：

- `evolveIdentityCore()` 已扩展到更多 adaptive traits
- 已加入 `slowFactor`
- 角色会根据事件和 role 获得 trait 偏移
- promoted persona 的 adaptive 已前置进入 step allowed-actions 过滤，而不只是事后 validator 拦截
- runtime 已有前后对照测试，证明同一 promoted NPC 在 trust 漂移后会失去 `attack / threaten` 等高冲突动作
- runtime 已有前后对照测试，证明同一 promoted NPC 在 trust 漂移后会改变 director 候选得分并从落选变成胜出
- identity shift 现在会刷新 `desireStore`
- runtime 已有前后对照测试，证明同一 promoted NPC 在 trust 漂移后会从 `autonomy -> withdraw` 转向 `affection -> approach`
- scheduler 已接入 adaptive-aware 选步
- `internal/agents` 已有前后对照测试，证明同一角色在慢变量漂移前后会从 `threaten` 转向 `trust`
- tick 后提交的 scheduler / autonomous 事件现在会重新投影回 `stateMgr`
- runtime 已有长链测试，证明同一 promoted NPC 在 identity shift 后会从非 trust 自治动作转向 `trust`，并在同一 tick 内改善 `Relationships`
- runtime 已有多 tick 长链测试，证明 identity shift 之后会持续增加 `trust` 型自治动作，而不是只影响单次决策

为什么还不能算已验收：

- “人物被经历持续塑形”目前仍偏机制级实现，不是长期结果级确认
- 已证明人物变化会影响后续动作空间、director 结果、autonomous intent、scheduler 选步、同 tick relationship outcome 与多 tick trust-action trajectory，但还没有充分证明会长期稳定影响更长窗口 world outcome
- 还缺少足够长的运行样本证明这不是局部数值波动

建议验收动作：

- 对同一 promoted persona 做多轮、多 tick 观察
- 验证 adaptive 变化是否持续进入行为与关系结果

---

## 验收项 4：Director 选人闭环

- 状态：`已实现，待验收`

当前实现：

- director 已使用 `kind / source / loaded / present / pressure / faction / hook` 等维度
- `relationshipWeight()` 已修正为归一化值
- candidate details、gap 分析、未入选原因已进入 trace / 前端

为什么还不能算已验收：

- 还没有充分证明 director 已经主要由世界状态而不是兼容角色语义驱动
- follow-up speaker 的长期质量还需要更多运行样本
- 当前解释能力已经存在，但还没达到“稳定可诊断”的终态标准

建议验收动作：

- 连续抽查 `/api/trace/latest`
- 对比 scene / pressure / faction / relationship 变化前后的选角结果

---

## 验收项 5：World Structure 驱动闭环

- 状态：`已实现，待验收`

当前实现：

- world structure 已可编辑
- planner / scheduler / runtime tick 已开始消费 factions / locations / pressures
- 新规则已推动更多行为类型进入计划层
- 前端 simulation compare 已能把 `structure` 改动和后续 `response` 分开显示
- `TickStatus()` 已能提示当前 scene 控制势力、命中中的 pressure、相关 background NPC 候选
- runtime 已有“同一世界、同一人口、结构干预前后”对照测试，证明 structure 变化会同步改变：
  - director candidate / world signals
  - tick tension / pressure_states / faction_tensions
  - authoring diagnostics
- runtime 已有 36 tick 长窗口对照测试，证明不同 structure 会进一步拉开：
  - `world_pressure` 事件数量
  - population promotion 结果
  - tick history diagnostics
  - 最终 tension / highlights
- runtime 已有 120 tick、200 tick、多结构样本矩阵测试，证明这些差异不是只存在于单个 36 tick 样本

为什么还不能算已验收：

- 当前更像“结构已接线”，还不是“世界结构已稳定主导世界行为”
- 虽然已经有前后对照、36 tick 长窗口分叉、120 tick / 200 tick 多样本矩阵，以及真实 world 目录样本在 runtime/API 双层 200 tick 验证，但样本总量仍偏小，暂时还不能证明结构变化会稳定主导所有长期事件与调度

建议验收动作：

- 修改 structure 后重复触发 tick / turn
- 观察 action log、trace、sim status 是否产生稳定差异

---

## 验收项 6：Autonomous Simulation 闭环

- 状态：`已实现，待验收`

当前实现：

- tick loop 已持续推进世界
- `PulseEngine`、`FactionEngine`、`npcTickExposure` 已接入
- `CooldownTicks` 已加入，减轻 pressure 抖动
- `/api/sim/status` 已能返回 pressure / faction / exposure / diagnostics
- runtime 已有多 tick 演化测试，证明 structure pressure 会在连续 tick 下重复触发并持续改变 `pressure_states / faction_tensions / last_tick_summary`
- runtime 已有 30+ tick 长窗口测试，证明无人输入时 scene 相关 background NPC 会被拉入当前 scene runtime，并在持续 exposure 下自然触发 promotion
- runtime 已有双世界 36 tick 长窗口对照，证明作者改 structure 后，自治世界的长期 outcome 会产生可观测分叉
- runtime 已有多样本 120 tick 与 200 tick 矩阵，证明不同 world structure 会在更长窗口下稳定产生不同 promoted leader / pressure leader / trajectory summary

为什么还不能算已验收：

- 当前已经能说明“世界会自己动并连续产生结构化变化”，并且证据已扩到 200 tick / 4 样本与真实 world 目录矩阵，且 runtime/API 双层都已补上 200 tick；但还不能说明“在更广世界类型和更大规模作者运营下仍稳定且有意义”

建议验收动作：

- 跑 30-50+ ticks
- 观察 pressure、faction、population 是否持续演化且可解释

---

## 验收项 7：Trace / Authoring 可解释闭环

- 状态：`已实现，待验收`

当前实现：

- trace 已显示 participants、director candidates、selected/alternates、excluded reasons
- `TurnTrace` 已加入 `world_metrics`
- checkpoint / preset / trace 历史已有联动骨架
- sim diagnostics 已从纯数值告警扩展到部分 world-structure / population 诊断
- `/api/sim/status` 与作者控制台已能展示最近多次 tick 轨迹，而不只是一条最近摘要
- `tick_history` 现在还会保留每个 recent tick 的 diagnostics 快照，作者可以回看“当时为什么报警”
- `/api/sim/status` 现在还会返回 `trajectory_summary`，作者不需要逐条读 history 也能看到长期 tension / pressure / promotion 趋势
- 作者控制台现在还能直接对照两个实例的长期结果，不需要手动切来切去比摘要
- simulation API 已支持批量 `count` tick；作者控制台可对当前实例或“当前 + 对照实例”同步推进多 ticks
- 双实例实验后，作者控制台会自动生成“实验结论”，不需要作者自己手工归纳哪一侧主导了长期分叉
- 作者控制台已支持从当前实例直接派生实验分支，减少手工创建与绑定步骤
- 作者控制台与 `/api/experiment-reports` 已支持实验报告归档，可保存当前/对照实例的长期结果快照、latest trace、director 与 participants 证据，并导出 JSON / Markdown
- 实验报告归档现在还会自动创建 `current/compare checkpoint`，并支持从实验报告列表与 Runtime Audit 直接恢复 archived current/compare 实例快照
- 实验报告归档现在还支持一键派生 replay branches：直接从 archived current/compare checkpoint 生成新的 current/compare 实例并继续推进
- 实验报告列表与 Runtime Audit archive 区现在还支持批量派生复现 / 批量刷新复现，可一次把所有带 checkpoint 的 archived reports 派生为 replay branches 或刷新 live 结果
- 批量派生复现现已收敛到正式后端接口 `/api/experiment-reports/replay-batch`，按 `world_name` 的世界级实验复现不再只是前端本地循环
- 实验报告列表与 Runtime Audit archive 区现在还会生成 `Experiment Portfolio` 批量结果矩阵，直接汇总 reports/worlds/replay loaded 数量，以及每条实验的 archived/live 主导侧与 trajectory 走向
- `Experiment Portfolio` 之上现在还会生成 `World Baselines` 聚合摘要，按 `world_name` 汇总每个 world family 的 reports、replay loaded、archived/live split、最近两次 tension/trajectory/population 漂移、稳定/波动/分叉长期状态与最新 trajectory
- `World Baselines + Experiment Portfolio` 现在还可直接导出为 JSON / Markdown 基线快照，方便把当前 world-family 基准留档与交接
- `World Baselines` 现在还支持按 `world_name` 直接派生复现实验、刷新该世界 live replay，并导出该世界单独的 baseline 快照
- 实验归档列表与 Runtime Audit archive 区现在还支持“聚焦该世界”，可把 portfolio、archive rows 与批量 replay workflow 收敛到单一 world scope
- Runtime Audit archive 区现在还内置基础 `Checkpoint Browser`，可直接选中 checkpoint 查看 scene/focus/player role/world-state 摘要与 `checkpoint vs live` 差异，再决定是否恢复
- 实验报告详情现在还内置 `Ops Matrix`，把 archived current/compare、选中 checkpoint、live replay 放进同一张对照卡，减少作者在多张卡片间手工比对
- `World Experiment Ops` 现在还会直接列出当前 world scope 下最近几份报告、checkpoint 锚点与 replay 是否已加载，作者可从世界运营卡直接跳到目标 report
- `World Experiment Ops` 现在还带有 `Focused Report / Focused Checkpoint` 中心选择位；切换 world/report/checkpoint 时，archive 与 runtime-audit 上下文会一起同步
- Runtime Audit archive 区现在还新增单屏 `World Experiment Panel`，把当前 live world、selected report、selected checkpoint、baseline gap 与 focused replay 摘要收拢到一张卡里，先回答“这个 world family 现在到底跑到了哪一步”
- Runtime Audit archive 与实验报告列表现在还能直接推进 focused replay 或整个 world scope 下已加载的 replay branches 若干 ticks，并立刻刷新 live divergence 证据；世界实验工作流从“派生/刷新”继续延伸到“推进并复核”
- replay 推进现在也已收敛到正式后端接口 `/api/experiment-reports/replay-advance`，前端不再自己逐个实例循环发 `/api/sim/tick`
- replay 派生 / 批量派生 / 推进响应现在会直接携带 current/compare evidence（sim status、latest trace、population、audit summary），前端会把这些 evidence 作为 audit 拉取失败时的 fallback
- replay 派生现在已补真实 runtime round-trip 验证：真实 current/compare 实例保存 checkpoint/report 后，API 能派生 replay branches、复制 archived checkpoint 锚点、加载并继续推进，而不只是 mock contract 通过
- Runtime Audit 与实验报告列表现在还能直接把 replay current / compare instance 切回当前实例与对照实例运营流，archive 工作流和实例运营流之间的切换更短
- Runtime Audit / World Experiment Panel 现在还能直接导出 world scope 的 proof bundle（JSON / MD），把 baseline / report / checkpoint / replay / live gap 汇成单份复核材料
- 已新增长期证据脚本 `scripts/run_world_proof_audit.sh`；脚本现在同时覆盖 API world-first contract 检查（含 canonical schema 防回流）、API author replay contract 检查（含真实 runtime replay round-trip）、API proof archive contract 检查、runtime population lifecycle contract 检查、runtime/API 两层的 200 tick sample matrix 与 real-world matrix；当前最新真实落盘结果 `data/proof-audits/20260528T033924Z/` 为 8/8 PASS
- `/api/proof-audits` 现已把这些长期 proof audit 归档正式暴露给作者控制台；Runtime Audit archive 区可直接看到最近几轮 PASS/FAIL、summary preview 与文件清单，并导出单次 proof audit 摘要
- Runtime Audit 现在还能直接显示“当前实例 vs 所属 world baseline”的偏移，回答当前 tension 是否还在基线区间、trajectory/population 是否已经跑偏
- 派生出来的 replay branches 现在还能在实验报告列表与 Runtime Audit 中直接显示 live pressure/faction/population/diagnostic split，以及 director/world-signal/latest-trace/population driver 证据、latest trace `step_traces` 差异、recent ticks / recent turns divergence timeline；timeline 中的 turn 分叉还能直接 drill down 到对应实例和 trace，并优先打开按 trace/step/handoff 顺序定位出来的首个分叉事件 causality chain，找不到严格命中时再回退
- `/api/runtime-audit` 与作者控制台 Runtime Audit 面板已把 `sim/status`、latest/recent trace、population insights、checkpoints、presets、experiment reports 聚成单一读取面
- Runtime Audit 面板已支持第一版按原因筛选，可按 `director / pressure / faction / population / archive` 切开证据
- Runtime Audit 面板已支持第一版按阶段回放，可对选中 trace 的 `step_traces` 做逐阶段翻看
- Runtime Audit 面板已支持第一版实验归档复现，可展开 archived experiment report，把其中 latest trace 直接送入阶段回放，并从 archived checkpoint 直接恢复实例、一键派生复现实验实例、批量派生/批量刷新复现实验；同时还能先在 archived checkpoint 层直接查看 scene/participants/pressure/faction/diagnostics/trajectory/latest-trace 的结构化差异解释，并用 `Experiment Portfolio` 批量结果矩阵与 `World Baselines` 聚合摘要汇总多实验结果，再进一步读取 replay branches 的 live 结构化对照结果、driver 证据、latest trace `step_traces` 差异、divergence timeline、turn-level drill down 与首个分叉事件 causality chain

为什么还不能算已验收：

- 当前已经从“很多分散面板”推进到“单一聚合审计面”，但还不是“作者能系统诊断世界”
- 统一审计面已补上第一版按原因筛选、第一版按阶段回放和第一版实验归档复现，并能在 archived checkpoint 层直接解释差异、派生 replay branches、全量或按世界批量派生/批量刷新/批量推进 replay、汇总批量结果矩阵与 world-family 基线摘要、通过单屏 `World Experiment Panel` 收拢 live/report/checkpoint/replay 状态、读取其 live 结构化对照结果、driver 证据、latest trace `step_traces` 差异、divergence timeline、turn-level drill down 与首个分叉事件 causality chain，但仍缺更完整的 runtime 调试器能力

建议验收动作：

- 让作者基于 trace 独立回答：
  - 为什么这个人上场
  - 为什么另一个人没上场
  - 世界为什么朝这个方向变

---

## 验收项 8：作者干预闭环

- 状态：`已实现，待验收`

当前实现：

- world structure / population / scenes / player role / presets / checkpoints 已有作者入口
- `TickStatus()` 已加入 `diagnostics`
- 作者已经能做基础干预和观测
- 结构保存后，作者已能持续看到“改了什么”以及“世界是否已经响应”
- API 层已有真实长窗口测试，证明作者通过 `/api/world-structure` + `/api/population` 干预后，`/api/sim/status` / `/api/population-insights` 能读出长期结果分叉
- 作者现在还能把双实例长期实验保存成正式报告，后续回看或导出，不必依赖一次性 live 面板

为什么还不能算已验收：

- 还没有证明作者主要靠世界级 authoring 就能稳定调控 runtime
- 目前仍无法确认是否已经摆脱“手改角色定义救场”的依赖
- 实验报告归档与真实 runtime replay round-trip 已经落地，但还缺更多真实 world family 的回放来证明这套工作流足够稳定

建议验收动作：

- 用 world/population/scene/preset 干预代替角色卡修改
- 观察 runtime 是否能按预期响应

---

## 当前结论

如果按“是否写过实现”来问：

- 8 项基本都已经推进到了 `已实现`

如果按“是否达到终态闭环”来问：

- 8 项大多仍然只是 `待验收`

因此，这份文档当前不支持下面这种说法：

```text
终态闭环 1-8 已全部完成。
```

更准确的说法是：

```text
终态闭环 1-8 已全部推进到实现层；
是否完成，仍需按 ACCEPTANCE_CHECKLIST.md 逐项验收。
```

---

## 最小验证

```bash
/usr/local/go/bin/go build -o corerp ./cmd/corerp
/usr/local/go/bin/go test -race ./...
node --check web/app.js
pm2 restart corerp
```

建议结合这些接口抽查：

```text
GET  /api/characters
GET  /api/instances
GET  /api/trace/latest
GET  /api/population-insights
GET  /api/sim/status
POST /api/sim/tick
POST /api/switch
```
