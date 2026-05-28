# CoreRP TODO

## 当前主线

项目主方向保持不变：

- `world-first persistent narrative runtime`
- `focus_character` 是观察视角
- `participants` 是场景在场者
- `participant_details` 是 switch / director / trace / UI 共用的结构化参与者模型
- 人物定义负责 persona seed，background NPC 通过事件与关注度自然晋升

完成标准请直接参考 [ACCEPTANCE_CHECKLIST.md](ACCEPTANCE_CHECKLIST.md)。
当前逐项审计请参考 [CLOSURE_AUDIT.md](CLOSURE_AUDIT.md)。

## 当前可信状态

以下项目已在代码中落地，并且本轮重新抽查通过：

- [x] `focus_character` 已成为内部主语义，`/api/characters` 与 `/api/instances` 输出新字段
- [x] `participant_details` 已进入 `/api/characters`、`/api/instances`、trace 视图与前端参与者面板
- [x] 前端主路径已不再依赖 `/api/character` 与 `/api/characters` 的 `active / characters` 兼容字段；角色面板改为读取 `focus_character + participants + participant_details` 与 `character-config.card`
- [x] `/api/characters` 已不再回退到 `loaded_characters`；`/api/instances/create` 也不再让 `active_character` 反向决定新实例 focus
- [x] runtime 的 `GetSceneParticipants()` 与 `GetSceneParticipantDetails()` 也已不再在空场景时回退到 `loadedCharacters`
- [x] `SaveSlot`、`ScenarioPreset`、`TurnTrace` 的 runtime 内部读取已统一优先 `FocusCharacter`；旧 `Character` 仅用于兼容迁移，`/api/checkpoints`、`/api/presets`、`/api/trace*` 公开响应不再输出 legacy `character` 镜像
- [x] `MemorySnapshot`、`PendingFact` 已统一做 compatibility normalization；`/api/memory`、`/api/pending-facts`、`/api/quarantine` 顶层响应不再把 legacy `character` 反向当成真实 focus，`/api/pending-facts` 的 fact 条目也不再输出 legacy `character` 镜像
- [x] `RuntimeInstanceSummary` 已显式标注 world-first 语义；`participants` 成为实例摘要中的 scene truth，旧 `active_character / loaded_characters` 已从主类型移除
- [x] `/api/instances`、`/api/instances/status`、`/api/instances/create` 与 `/api/state.instance` 这些主实例出口现已只返回 canonical instance 摘要，不再公开 `active_character / loaded_characters`
- [x] `/api/runtime-audit.instance` 与 `/api/experiment-reports/replay.current_instance|compare_instance` 也已切到 canonical instance 摘要，归档/复现实例链路不再重新暴露 `active_character / loaded_characters`
- [x] runtime 的 `Engine.InstanceSummary()` 现在也已停止主动填充 `active_character / loaded_characters`；主类型不再携带这两个字段
- [x] runtime 现在也已停止主动填充 `TurnTrace / TurnStepTrace / SaveSlot / ScenarioPreset / CharacterConfig / MemorySnapshot` 上的空 `character` legacy 镜像；这些对象的 legacy 字段已改成 `omitempty`，canonical 主路径默认不再输出
- [x] runtime compatibility normalization 现在会把旧 `character` 迁移进 `focus_character / step.speaker` 后立刻清空 legacy 镜像；旧数据仍可读，但新的 runtime trace/save/preset/pending-fact 输出不会继续携带旧字段值
- [x] API 层 compatibility normalization 现在也已改成单向兼容；`/api/trace`、`/api/checkpoints`、`/api/presets`、`/api/pending-facts` 不会再把 runtime 已退空的 legacy `character` 镜像重新补回响应
- [x] API contract 已新增回归守卫：`MemorySnapshot / SaveSlot / ScenarioPreset / CharacterConfig / RuntimeInstancePayload / ExperimentSnapshot / TurnTrace` 这些 canonical schema 不允许重新出现 `character / active_character / loaded_characters`
- [x] 作者控制台 Runtime Audit / World Experiment Panel / checkpoint browser / proof bundle / step trace 渲染已移除对 `slot.character / selectedCheckpoint.character / bundle.selected_checkpoint.character / stepTrace.character` 的前端回退读取；主读取路径只看 canonical `focus_character / speaker`
- [x] `core` 类型层里现存的公开 `character` 字段也都已退成 `omitempty`；legacy 字段只有在旧值真实存在时才会出现在 JSON 中
- [x] `/api/characters`、`POST /api/worlds`、`/api/quarantine`、`/api/pending-facts`、`/api/npc-actions` 这些主接口已移除顶层 `active / characters / character` 兼容镜像；legacy 镜像收敛到专用兼容路径与对象内兼容字段
- [x] `/api/memory` 与 `/api/export` 现在也已移除顶层 `character` 兼容镜像；`MemorySnapshot` API 契约也已移除 legacy `character` 字段，主语义统一收敛到 `focus_character` 与 `focus_definition`
- [x] canonical `/api/focus-definition` 与 `/api/focus-definition-config` 已上线；前端主路径和主测试已切换过去，`/api/character*` 只保留兼容入口，`CharacterConfig` API 契约已移除 legacy `character` 字段
- [x] legacy `/api/character` 与 `/api/character-config` 现在会显式返回 `Deprecation: true` 和 successor `Link` 头，canonical/legacy 边界已进入运行态
- [x] API server 的主接口不再要求 `GetLoadedCharacters()` 参与主链路
- [x] `/api/state` 已显式返回 `focus_character + participants + participant_details`
- [x] 视角切换不再覆盖 scene truth；切换 focus 后原场景参与者继续保留
- [x] director 已接入参与者模型；`player_role` / `scene_shell` / `scene_presence` 不进入 speaker candidate
- [x] director 权重已显式支持 `kind/source/loaded`
- [x] population runtime 已接入 attention / promotion / promoted persona / director candidate 主链路
- [x] autonomous tick 现在会显式把当前 scene/location/faction/pressure 命中的 background NPC 拉入 scene runtime，而不依赖先发生 directTurn
- [x] `world_pressure` 已进入 population attention 主链路；真实 world 目录里的 pressured background NPC 不会再因为只吃到 tick 而长期卡在“不晋升”
- [x] population attention 现在使用滚动事件窗口，旧事件不会永久抬分；promoted NPC 若长期脱离 scene/pressure/event 会触发 `population_demoted`，并在 `/api/population-insights` history 中可追踪
- [x] world structure API、population API、simulation API 已存在并可正常读取
- [x] API 层已通过真实 runtime 长窗口验证：作者经 `/api/world-structure`、`/api/population`、`/api/sim/tick` 干预后，`/api/sim/status` 与 `/api/population-insights` 能读出长期分叉结果
- [x] Simulation 运维接口已落地：`/api/sim/status`、`/api/sim/tick`、`/api/sim/pause`、`/api/sim/resume`
- [x] 世界结构保存后的前端 compare 摘要已区分 `structure` 与 `response`，并会等待真实世界响应后再消费变更摘要
- [x] `TickStatus()` 已能给出结构化作者诊断：scene 控制势力、命中 scene/faction 的 pressure、可被拉入候选的 background NPC
- [x] `/api/sim/status` 与作者控制台已能展示最近多次 tick 的结构化轨迹，而不只是一条 `last_tick_summary`
- [x] `/api/sim/status` 与作者控制台已能给出 `trajectory_summary`，直接总结长期 tension / pressure / promotion / diagnostics 趋势
- [x] 作者控制台已支持跨实例长期结果对照，可直接比较两个实例的 `trajectory_summary / tension / population / diagnostics`
- [x] simulation API 与作者控制台已支持批量 tick；启用对照实例时可同步推进两个实例做长期实验
- [x] 双实例同步推进后，作者控制台会自动生成实验结论，直接指出长期 tension / population / diagnostics / pressure 的主导侧
- [x] 作者控制台已支持从当前实例一键创建实验分支，并自动接入对照实验工作流
- [x] 作者控制台与 `/api/experiment-reports` 已支持实验报告归档；可保存当前/对照实例的长期结果快照、latest trace、director 与 participants 证据，并导出 JSON / Markdown
- [x] `/api/runtime-audit` 与作者控制台 Runtime Audit 面板已把 `sim/status + latest_trace/recent_traces + population-insights + checkpoints + presets + experiment-reports` 聚成单一读取面
- [x] Runtime Audit 面板已支持第一版“按原因筛选”，可按 `director / pressure / faction / population / archive` 切开当前统一审计证据
- [x] Runtime Audit 面板已支持第一版“按阶段回放”，可直接对选中 trace 的 `step_traces` 做逐阶段前后翻看
- [x] Runtime Audit 面板已支持第一版“实验归档复现”，可展开 archived experiment report，并把其中 `current/compare.latest_trace` 直接送入阶段回放
- [x] 实验归档现在会自动锚定 `current/compare checkpoint`；作者可从实验报告列表与 Runtime Audit 直接把 archived current/compare 快照恢复回实例
- [x] 实验归档现在还可一键派生“当前 / 对照”复现实例：直接从 archived checkpoint 生成新实例分支，并自动切到复现实验继续跑
- [x] 实验报告列表与 Runtime Audit 的 archive 区现在都已支持“批量派生复现 / 批量刷新复现”，作者可一次把所有带 checkpoint 的 archived report 派生为 replay branches 或批量刷新其 live 结果
- [x] “批量派生复现” 现已收敛到正式后端接口 `/api/experiment-reports/replay-batch`；前端不再自己逐条决定世界复现筛选规则，`world_name` 过滤进入 API 契约
- [x] 实验报告列表与 Runtime Audit 的 archive 区现在还会直接生成 `Experiment Portfolio` 批量结果矩阵，汇总 reports / worlds / compare / replay loaded 数量，并逐条压缩显示 archived leader、live leader、replay instance 与 trajectory 走向
- [x] `Experiment Portfolio` 之上现在还会生成 `World Baselines` 聚合摘要，按 `world_name` 汇总每个 world family 的 reports、replay loaded、archived/live tension split 与最新 trajectory
- [x] `World Baselines` 现在还会直接比较同一 `world_name` 最近两次实验的 tension / trajectory / population 漂移，作者可直接看 world family 是向哪个方向偏移
- [x] `World Baselines` 现在还会基于同一 `world_name` 的全部 reports 给出 `稳定 / 波动 / 分叉` 长期状态，并显示 tension range、trajectory variants、population variants
- [x] `World Baselines + Experiment Portfolio` 现在都可直接导出为 JSON / Markdown 基线快照，作者可把当前 world-family 基准留档给后续模型或运营回看
- [x] `World Baselines` 现在还可直接按 `world_name` 做“派生该世界 / 刷新该世界 / 导出该世界基线”，作者不必再对所有 archived reports 做全量 replay 才能复现实验族
- [x] 实验归档列表与 Runtime Audit archive 区现在都支持“聚焦该世界”；作者可以把 `Experiment Portfolio`、archive reports 与批量 replay workflow 收敛到单一 world scope，而不是一直在全量 report 列表里找
- [x] Runtime Audit 现在还能直接显示当前实例相对所属 `World Baselines` 的偏移，能看出当前张力是否落在基线区间内以及最新 trajectory / population 是不是已经跑偏
- [x] Runtime Audit archive 区现在还内置 `Checkpoint Browser`；作者可直接选中 checkpoint，查看其 scene/focus/player role/world-state 摘要，并先看“checkpoint vs live”差异，再决定是否恢复
- [x] Runtime Audit 的实验报告详情现在还内置 `Ops Matrix`，把 archived current/compare、选中 checkpoint、live replay 放进同一张对照卡，作者不再需要在多张卡片之间手工比对
- [x] `World Experiment Ops` 现在还会直接列出当前 world scope 下最近几份报告、checkpoint 锚点与 replay 是否已加载，作者可从世界运营卡直接跳到目标 report
- [x] `World Experiment Ops` 现在还带有 `Focused Report / Focused Checkpoint` 中心选择位；切换 world/report/checkpoint 时，archive 与 runtime-audit 上下文会一起同步
- [x] Runtime Audit archive 区现在新增单屏 `World Experiment Panel`，把当前 live world、selected report、selected checkpoint、baseline gap 与 focused replay 摘要压进同一张卡，作者不必再在 baseline/checkpoint/report/replay 几块之间来回找状态
- [x] Runtime Audit archive / 实验报告列表现在还能直接把 focused replay 或整个 world scope 下已加载的 replay branches 批量推进若干 ticks，并立刻刷新 live divergence 证据；世界实验工作流不再停在“派生后手动切实例推进”
- [x] replay 推进现已收敛到正式后端接口 `/api/experiment-reports/replay-advance`；前端不再自己逐个实例循环发 `/api/sim/tick` 来运营 world-scope replay branches
- [x] replay 派生 / 批量派生 / 推进响应现在会直接携带 current/compare evidence（sim status、latest trace、population、audit summary），前端会把这些 evidence 作为 audit 拉取失败时的 fallback，作者侧不会出现“推进成功但证据空白”
- [x] replay 派生现在已补真实 runtime round-trip 验证：从 archived current/compare checkpoint 派生 replay branches 后，会把源 checkpoint 锚点复制进 replay 实例、加载并继续推进；不再只靠 mock contract 证明 replay API 字段存在
- [x] author replay contract 已补 world-level authoring 实证：`TestAuthorWorldLevelInterventionReplayControlsRuntimeWithoutCharacterConfig` 只通过 `/api/population`、`/api/world-structure`、tick、checkpoint/report/replay 制造并复现长期分叉，并断言 focus definition 没有被修改，避免把“手改角色定义救场”误判成作者干预闭环；`TestAuthorWorldLevelInterventionReplayMatrixAcrossWorldFamilies` 已把该证据扩成外城治安与港口物流两个 world family 的 API 矩阵，并批量推进 replay branches 复核 audit evidence
- [x] Runtime Audit / 实验报告列表现在还能直接把 replay current / compare instance 拉回当前实例与对照实例运营流；作者不必再先去实例列表里手动找 replay 分支
- [x] Runtime Audit / World Experiment Panel 现在还能一键导出 world scope 的 proof bundle（JSON / MD），把 baseline / report / checkpoint / replay / live gap 打成单份复核包
- [x] 已新增可重复执行的长期证据脚本 `scripts/run_world_proof_audit.sh`；脚本现在同时覆盖 API world-first contract 检查（含 canonical schema 防回流）、API author replay contract 检查（含真实 runtime replay round-trip 与多 world-family world-level authoring replay）、API proof archive contract 检查、events npc scheduler canonical contract 检查、runtime population lifecycle contract 检查（含 identity slow-variable outcome）、runtime/API 两层的 200 tick sample matrix、real-world matrix 与 500 tick real-world stability；当前最新真实 proof audit 为 `data/proof-audits/20260528T084433Z/`，11/11 gates PASS
- [x] `/api/proof-audits` 已上线；作者控制台 Runtime Audit archive 区现在能直接读出最近 proof audit 归档、PASS/FAIL、摘要预览与文件清单，并可导出单次 proof audit 摘要 JSON / Markdown
- [x] 派生出的 replay branches 现在还能在实验报告列表与 Runtime Audit 中直接显示 live pressure/faction/population/diagnostic split、director/world-signal/latest-trace/population driver 证据、latest trace `step_traces` 差异，以及基于 recent ticks / recent turns 的 divergence timeline；timeline 中的 turn 分叉还可直接 drill down 到对应实例和 trace，并优先打开按 trace/step/handoff 顺序定位出来的“首个分叉事件” causality chain，找不到严格命中时再回退
- [x] `ExperimentSnapshot` / `ExperimentReport` 已统一做 compatibility normalization；归档中的 `latest_trace` 现在也会优先 `focus_character`
- [x] `/api/worlds` 与 `/api/export` 已明确以 `focus_character` / `focus_definition` 为主语义，并显式返回 `participants / participant_details`
- [x] `PUT /api/worlds` 与 `PATCH /api/worlds` 已实现
- [x] `README.md` 当前与 world-first 主路线基本一致
- [x] DCL mod 第一版已落地：`internal/dcl` 支持声明式 `manifest.yml`、world/population/scenes/presets patches、声明式 hooks、安装 registry；API 已新增 `GET /api/dcl`、`POST /api/dcl/install`、`POST /api/dcl/upload`、`POST /api/dcl/remove`；作者控制台 World 分组已提供 DCL 面板，可上传 ZIP、启用、关闭、删除安装出的 world 或删除本地 `.dcl` 包目录；样板包 `mods/looping_isekai_return.dcl/` 展示 checkpoint-loop / return-by-death inspired world pack，且不执行 Lua/脚本代码

## 已完成验收

这些方向已经按当前 `ACCEPTANCE_CHECKLIST.md` 完成闭环验收：

- [x] simulation 长期稳定性已补到 200 tick / 5 样本矩阵（新增 `48111430a81be7d4` 校园别墅世界与 `a0c85d27e38863a4` 直播顶层世界），并在 runtime / API 两层证明 structure 驱动可持续产出 `pressure_states / faction_tensions / tick_history / population promotion` 演化；真实 world 目录已从 3 个扩展到 5 个（`neon_block / 1_7 / 《红楼梦》完整版 / 48111430a81be7d4 / a0c85d27e38863a4`），且已补到 500 tick 长窗口稳定性验证（runtime/API 双层 11/11 gates PASS）
- [x] world structure 对 planner / scheduler / tick 的深度驱动已接线，并已有“同一世界前后结构对照”“不同 structure 下长期 outcome 分叉”“120 tick + 200 tick + 500 tick 多样本矩阵”以及基于 5 个真实 world 目录的 runtime/API 双层 200 tick + 500 tick 稳定性验证；最新 proof audit 为 `data/proof-audits/20260528T084433Z/`，11/11 gates PASS
- [x] population growth 闭环已成型，并已证明 scene 相关 background NPC 可在无人输入时被 tick runtime 自然拉入、累积 exposure，并通过 `world_pressure + exposure` 持续触发 promotion；promoted NPC 在长期脱离 scene/pressure/event 后也会 demotion 并留下可追踪 history；promoted persona 的 adaptive 漂移也会反向改变 future allowed actions、director 胜出结果、autonomous desire / intent、scheduler 选步、同 tick relationship outcome、多 tick trust-action trajectory，以及 2 world-family 长窗口 tension / trajectory_summary world outcome 矩阵
- [x] trace / 作者控制台已经能解释多数候选差距，并通过 Runtime Audit 聚合看到 structure 影响、最近 tick 轨迹、长期 trajectory summary、跨实例结果对照、按原因筛选、按阶段回放、director 决策解释（胜出/落选候选人 score/dominant factors/gap）、world pressure 解释（dominant pressure/tension 趋势）、faction 解释（dominant faction）、实验归档复现、checkpoint 恢复、checkpoint 差异解释、批量结果矩阵、一键派生的复现实验实例，以及 replay branches 的 live pressure/faction/population/diagnostic split、driver 证据、latest trace `step_traces` 差异、recent ticks / recent turns divergence timeline、turn-level drill down 与首个分叉事件 causality chain；API 层还已证明 world-level authoring 可在不改角色定义的情况下形成可复现 runtime 分叉

## 文档约束

- [ ] 不再把“未来路线”写成 `[x]`
- [ ] 不再把错误的 HTTP 预期写成回归结论
  - `GET /api/switch` 返回 `405` 是正确行为
  - `POST /api/switch` 返回 `200` 才表示兼容路径可用
- [ ] `SESSION_LOG.md` 当前可作为变更素材库，不可直接当严格时间线；后续需要统一时区并重排

## 下一阶段

按价值排序，后续优先做这些：

1. 继续扩大真实 authoring replay 样本池，尤其是用户自建世界；这是覆盖面增强，不是当前闭环阻塞项。
2. 扩大人格慢变量长期塑造 world outcome 的样本池；优先补真实导入世界 / 用户自建世界矩阵。
3. 继续深化 Runtime Audit 的调试器体验，并保持 `scripts/run_world_proof_audit.sh` 作为重要修改后的回归证据。

闭环完成判断已按 [ACCEPTANCE_CHECKLIST.md](ACCEPTANCE_CHECKLIST.md) 更新；后续条目按增强项管理。

## 最小验证

```bash
/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime ./internal/core
/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion
node --check web/app.js
/usr/local/go/bin/go build -o corerp ./cmd/corerp
pm2 restart corerp && pm2 save
```

## 建议抽查接口

```text
GET  /api/characters
GET  /api/instances
GET  /api/world-structure
GET  /api/population
GET  /api/population-insights
GET  /api/sim/status
POST /api/switch
```
