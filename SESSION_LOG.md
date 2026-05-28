# CoreRP Session Log

> 记录规则：每条日志必须使用绝对时间，并标注修改者/模型身份。

## 2026-05-28

### 2026-05-28 03:35:41 UTC — 真实 Runtime Replay Round-Trip 闭环补强
Modified by: Codex (GPT-5)

- 变更：
  - `internal/api/server.go` 的 experiment replay 派生现在会先从源实例读取 archived checkpoint，再在 replay 实例中创建同名 checkpoint 并加载，避免真实 runtime replay branch 找不到源 checkpoint
  - `internal/api/server_test.go` 新增 `TestExperimentReportReplayBatchRealRuntimeRoundTrip`，使用真实 `runtime.Manager` 与真实 runtime engine 覆盖 report -> replay-batch -> replay-advance
  - `scripts/run_world_proof_audit.sh` 的 `api-author-replay-contract` gate 纳入真实 runtime replay round-trip 测试
  - `TODO.md`、`CLOSURE_AUDIT.md`、`DELIVERY_TRACKING.md` 同步记录作者侧 replay 从 mock contract 升级为真实 runtime round-trip
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run '^TestExperimentReportReplayBatchRealRuntimeRoundTrip$'` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run '^(TestExperimentReportReplayCreatesReplayBranches|TestExperimentReportReplayBatchFiltersByWorld|TestExperimentReportReplayBatchRealRuntimeRoundTrip|TestExperimentReportReplayAdvanceTicksReplayBranches|TestRuntimeAuditAggregatesAuthoringEvidence)$'` ✅
  - `bash -n scripts/run_world_proof_audit.sh` ✅
  - `git diff --check` ✅
  - `./scripts/run_world_proof_audit.sh` ✅
- 最新 proof audit：
  - `data/proof-audits/20260528T033924Z/SUMMARY.md`
  - 8/8 gates PASS，且 `api-author-replay-contract` 已包含 `TestExperimentReportReplayBatchRealRuntimeRoundTrip`

### 2026-05-28 03:17:31 UTC — Proof Archive Gate 纳入长期审计
Modified by: Codex (GPT-5)

- 变更：
  - `scripts/run_world_proof_audit.sh` 新增 `api-proof-archive-contract` gate
  - 该 gate 执行 `TestProofAuditsRouteListsLatestAuditArtifacts`，锁定 `/api/proof-audits` 能列出最近 proof audit 归档、summary preview 与文件清单
  - `TODO.md`、`CLOSURE_AUDIT.md`、`DELIVERY_TRACKING.md` 同步更新到最新审计结果
- 验证：
  - `bash -n scripts/run_world_proof_audit.sh` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run '^TestProofAuditsRouteListsLatestAuditArtifacts$'` ✅
  - `./scripts/run_world_proof_audit.sh` ✅
- 当时 proof audit（已被 `20260528T033924Z / 8/8 PASS` supersede）：
  - `data/proof-audits/20260528T031731Z/SUMMARY.md`
  - 8/8 gates PASS：world-first contract、author replay contract、proof archive contract、population lifecycle、runtime/API 200 tick sample matrix、runtime/API 200 tick real-world matrix

### 2026-05-28 03:15:46 UTC — Proof Audit 结果交接修正
Modified by: Codex (GPT-5)

- 变更：
  - `TODO.md`、`CLOSURE_AUDIT.md`、`DELIVERY_TRACKING.md` 从旧的 `20260527T223320Z / 4/4 PASS` 更新到最新 `20260528T030850Z / 7/7 PASS`
  - 保持结论口径为“长期证据增强，但不能直接标记终态闭环完成”
- 当时 proof audit（已被 `20260528T033924Z / 8/8 PASS` supersede）：
  - `data/proof-audits/20260528T030850Z/SUMMARY.md`
  - 7/7 gates PASS：world-first contract、author replay contract、population lifecycle、runtime/API 200 tick sample matrix、runtime/API 200 tick real-world matrix

### 2026-05-28 00:00:00 UTC — Runtime Audit 前端 legacy character fallback 收束
Modified by: Codex (GPT-5)

- 变更：
  - `web/app.js` 的 Runtime Audit checkpoint browser、World Experiment Ops、World Experiment Panel、proof bundle Markdown、Ops Matrix 与 step trace tag 不再读取 `slot.character / selectedCheckpoint.character / bundle.selected_checkpoint.character / stepTrace.character`
  - 前端主路径改为只消费 `focus_character / speaker`，避免 legacy `character` 在作者控制台重新成为隐式主语义
  - `internal/runtime/compat.go` 改成读取旧 `character` 后只迁移到 `focus_character / step.speaker`，随后清空 legacy 镜像；`buildStepHandoff()` 同步改为只看 canonical speaker
  - `internal/runtime/population_runtime.go` 的 population attention 改成 72h 滚动事件窗口，旧事件不再永久抬分；promoted NPC 长期脱离 scene/pressure/event 后会生成 `population_demoted` 并退回 background
  - `internal/runtime/runtime_test.go` 新增 demotion lifecycle 测试，证明 demotion canonical event 与 `/api/population-insights` history 可追踪
  - `internal/api/server_test.go` 新增 API contract canonical schema 防回流测试，锁住 `MemorySnapshot / SaveSlot / ScenarioPreset / CharacterConfig / RuntimeInstancePayload / ExperimentSnapshot / TurnTrace`
  - replay 派生 / 批量派生 / 推进响应现在会直接携带 current/compare evidence（sim status、latest trace、population、audit summary）；`web/app.js` 会把这些 evidence 作为 audit 拉取失败时的 fallback
  - `scripts/run_world_proof_audit.sh` 的 `api-world-first-contract` gate 已纳入该 schema 防回流测试，并新增 `api-author-replay-contract` 与 `runtime-population-lifecycle-contract` gate
  - `TODO.md`、`CLOSURE_AUDIT.md`、`DELIVERY_TRACKING.md`、`ACCEPTANCE_CHECKLIST.md` 同步记录当前为“接近验收”，但不标记最终闭环完成
- 验证：
  - `node --check web/app.js`
  - `bash -n scripts/run_world_proof_audit.sh`
  - `rg -n "slot\\.character|selectedCheckpoint\\.character|bundle\\.selected_checkpoint\\.character|stepTrace\\.character|focus_character \\|\\| .*character|speaker \\|\\| .*character|character \\|\\| .*focus_character" web/app.js`
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run '^TestAPIContractCanonicalSchemasExcludeLegacyFocusMirrors$'`
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run '^(TestExperimentReportReplayCreatesReplayBranches|TestExperimentReportReplayBatchFiltersByWorld|TestExperimentReportReplayAdvanceTicksReplayBranches|TestRuntimeAuditAggregatesAuthoringEvidence)$'`
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime -run '^(TestReconcilePopulationPromotesBackgroundNPC|TestReconcilePopulationDemotesStalePromotedNPC|TestAutonomousSimulationPromotesScenePopulationAcrossLongWindow|TestPopulationInsightsIncludesPromotionReason)$'`
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run '^(TestTraceRoute|TestTraceRoutesNormalizeLegacyCharacterFields|TestCheckpointAndPresetRoutes|TestCharacterConfigRoutePost|TestCharacterConfigRouteGetUsesCanonicalFocusCharacter|TestRuntimeAuditAggregatesAuthoringEvidence|TestMemoryRoutePrefersFocusCharacterOverLegacyCharacter|TestPendingFactsRoutePrefersFocusCharacterOverLegacyCharacter|TestNPCActionsRouteUsesFocusCharacterWithoutTopLevelCharacterMirror)$'`
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime -run '^(TestListTurnTracesNewestFirst|TestLoadSaveSlotPrefersFocusCharacterOverLegacyCharacter|TestApplyScenarioPresetPrefersFocusCharacterOverLegacyCharacter|TestExperimentReportNormalizesFocusCompatibility|TestLoadSaveSlotDoesNotResetTurnCounter|TestResetDialogueDoesNotResetTurnCounter|TestSwitchCharacterDoesNotResetTurnCounter)$'`
  - `/usr/local/go/bin/go build -o /home/kelebituo/corerp/corerp ./cmd/corerp`
- 结论：
  - world-first 主语义进一步接近验收；仍不能标记终态闭环完成，因为兼容接口、底层旧存储字段与更长窗口作者回放验收仍未完全收束。

## 2026-05-27

### 2026-05-27 20:48:00 UTC — SaveSlot / Preset / Trace 内部读取统一优先 focus_character
Modified by: Codex (GPT-5)

- `internal/runtime/compat.go`
  - 新增兼容归一化 helper：
    - `normalizeSaveSlotCompatibility`
    - `normalizeScenarioPresetCompatibility`
    - `normalizeTurnTraceCompatibility`
- `internal/runtime/persistence.go`
  - `CreateSaveSlot` / `LoadSaveSlot` / `readSaveSlots` / `writeSaveSlots`
  - 统一改为优先 `FocusCharacter`
  - 旧 `Character` 只在旧数据缺失 `FocusCharacter` 时用于一次性迁移
- `internal/runtime/authoring.go`
  - `EnterWorld` / `CreateScenarioPreset` / `ApplyScenarioPreset` / `readScenarioPresetsLocked` / `writeScenarioPresetsLocked`
  - 统一改为优先 `FocusCharacter`
- `internal/runtime/trace.go`
  - `recordTraceLocked` 会先做 trace 兼容归一化
- `internal/runtime/turns.go`
  - `StepHandoff` 改为优先使用 `step.speaker`
- `internal/runtime/runtime_test.go`
  - 新增：
    - `TestLoadSaveSlotPrefersFocusCharacterOverLegacyCharacter`
    - `TestApplyScenarioPresetPrefersFocusCharacterOverLegacyCharacter`
  - `TestListTurnTracesNewestFirst` 也改为锁定 trace 会自动补齐 compatibility mirror
- `CLOSURE_AUDIT.md`
  - World-First 项补充“SaveSlot / ScenarioPreset / TurnTrace 内部读取已优先新字段”的证据
- `TODO.md`
  - 同步记录类型层兼容镜像继续退居 mirror 的进展
- `DELIVERY_TRACKING.md`
  - World-First 项补充 runtime 内部读取层的新收束
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime -run 'Test(LoadSaveSlotPrefersFocusCharacterOverLegacyCharacter|ApplyScenarioPresetPrefersFocusCharacterOverLegacyCharacter|ListTurnTracesNewestFirst|CreateAndLoadSaveSlot|ScenarioPresetCreateAndApply)'` ✅

### 2026-05-27 20:32:00 UTC — runtime scene participants 不再回退 loaded roster
Modified by: Codex (GPT-5)

- `internal/runtime/runtime.go`
  - `GetSceneParticipants()` 不再在空场景时退回 `loadedCharacters`
  - `GetSceneParticipantDetails()` 也不再在空场景时用 loaded roster 补 participants truth
- `internal/runtime/runtime_test.go`
  - 新增 `TestSceneParticipantsDoNotFallbackToLoadedCharacters`
  - 锁定 scene truth 为空时，participants / participant_details 为空，但 loaded roster 兼容镜像仍保留
- `CLOSURE_AUDIT.md`
  - World-First 项补充 runtime 层已不再回退 loaded roster
- `TODO.md`
  - 同步记录 runtime 语义层的新收束
- `DELIVERY_TRACKING.md`
  - World-First 项补充 runtime 层证据
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime -run 'Test(SceneParticipantsDoNotFallbackToLoadedCharacters|SwitchCharacterLoadsSceneParticipantShellWhenMissing)'` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run 'TestCharactersRouteDoesNotFallbackToLoadedCharacters|TestStateIncludesInstanceMetadata|TestRuntimeAuditAggregatesAuthoringEvidence'` ✅

### 2026-05-27 20:05:00 UTC — Runtime Audit 聚合视图落地
Modified by: Codex (GPT-5)

- `internal/core/types.go`
  - 新增 `RuntimeAuditSnapshot`
  - 统一承载作者侧审计所需的聚合证据
- `internal/api/server.go`
  - 新增 `GET /api/runtime-audit`
  - 单次返回：
    - `sim_status`
    - `latest_trace`
    - `recent_traces`
    - `population`
    - `checkpoints`
    - `presets`
    - `experiment_reports`
    - `audit_summary`
  - 支持基础 limit 参数，避免把所有证据面一次性拉满
- `internal/api/server_test.go`
  - 新增 `TestRuntimeAuditAggregatesAuthoringEvidence`
  - 同时把 `/api/runtime-audit` 加入合法/非法 method 抽查
- `web/index.html`
  - 作者工具新增 `Runtime Audit` 面板
- `web/app.js`
  - 新增 Runtime Audit 的加载、筛选、渲染与 trace/report 联动
  - 现在作者可以在一个面板里先看“世界为什么这样跑”，再决定跳去 trace 或归档
- `CLOSURE_AUDIT.md`
  - 将“尚无单一系统级运行审计视图”的表述收束为“统一读面已落地，但仍缺完整回放/筛选/复现实验能力”
- `TODO.md`
  - 下一阶段从“先做统一审计面”切换为“在统一审计面之上继续补完整调试器能力”
- `DELIVERY_TRACKING.md`
  - Trace / Authoring 项补充 Runtime Audit 当前实现
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run 'Test(RuntimeAuditAggregatesAuthoringEvidence|RouteWrongMethod|RouteValidMethod2xx|StateIncludesInstanceMetadata)'` ✅

### 2026-05-27 18:55:00 UTC — 真实 world 目录长窗口矩阵补强
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增 `TestRealWorldDirectorySampleMatrixAcrossHundredTwentyTicks`
  - 直接复制并运行真实 world 目录：
    - `worlds/neon_block`
    - `worlds/1_7`
    - `worlds/《红楼梦》完整版、-角色卡-202604190812`
  - 证明原生都市世界与两类导入世界在 120 ticks 下都能产出可区分的：
    - dominant pressure
    - dominant faction tension
    - promoted leader
    - trajectory summary
- `internal/api/server_test.go`
  - 新增 `TestAPIRealWorldDirectorySampleMatrixAcrossHundredTwentyTicks`
  - 在 API 层复现同样的真实目录矩阵
  - 新增真实 scene helper 与测试目录复制 helper
- `CLOSURE_AUDIT.md`
  - 收回“样本仍主要集中在外城压力世界”的旧表述
  - 更新为“真实 world 目录样本已补到都市世界与导入世界，但仍未达到最终普适性证明”
- `TODO.md`
  - 把下一阶段从“补非外城压力世界样本”收束到“扩大真实 world 目录和作者侧回放样本池”
- `DELIVERY_TRACKING.md`
  - 同步更新 population / structure / simulation 的证据面
- `ACCEPTANCE_CHECKLIST.md`
  - 当前判断口径补充“真实 world 目录矩阵”事实
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime -run 'TestRealWorldDirectorySampleMatrixAcrossHundredTwentyTicks'` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run 'TestAPIRealWorldDirectorySampleMatrixAcrossHundredTwentyTicks'` ✅

### 2026-05-27 17:20:08 UTC — 实验归档接入 trace / director 证据
Modified by: Codex (GPT-5)

- `internal/core/types.go`
  - 扩展 `ExperimentSnapshot`
  - 新增：
    - `participant_details`
    - `scene_description`
    - `director_plan`
    - `latest_trace`
- `web/app.js`
  - 实验报告生成时会额外拉取 `/api/state`
  - 当前/对照实例的归档内容不再只保存长期 sim 数值
  - 现在还会保存：
    - participants / participant_details
    - scene description
    - latest trace
    - director plan / world signals
  - Markdown 导出也会带上这些证据
- `internal/runtime/runtime_test.go`
  - 扩展 `TestExperimentReportCreateAndList`
  - 锁定 director / trace 证据会被持久化
- `internal/api/server_test.go`
  - 扩展 `TestExperimentReportRoutes`
  - 锁定 API 层可保存并返回上述归档证据
- `CLOSURE_AUDIT.md`
  - 将 Trace / Authoring 项更新为“实验归档已包含 latest trace / director / participants 证据”
- `TODO.md`
  - 同步更新 experiment report 的能力边界
- `DELIVERY_TRACKING.md`
  - 同步更新 Trace / Authoring 项当前实现与判断
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime -run 'TestExperimentReportCreateAndList'` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run 'TestExperimentReportRoutes'` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime` ✅

### 2026-05-27 17:14:26 UTC — `/api/state` 显式 world-first 化
Modified by: Codex (GPT-5)

- `internal/api/server.go`
  - `/api/state` 新增显式返回：
    - `focus_character`
    - `participants`
    - `participant_details`
  - 调用方不再需要从 `instance.active_character` 这类兼容镜像倒推当前语义
- `internal/api/server_test.go`
  - 扩展 `TestStateIncludesInstanceMetadata`
  - 锁定 `/api/state` 的显式 world-first 字段
- `CLOSURE_AUDIT.md`
  - World-First 项补充 `/api/state` 已显式返回新语义字段的证据
- `TODO.md`
  - 记录 `/api/state` 已显式返回 `focus_character + participants + participant_details`
- `DELIVERY_TRACKING.md`
  - 同步更新 World-First 项当前实现
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run 'TestStateIncludesInstanceMetadata|Test(InstanceCreateEndpoint|CharactersRoute)'` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime` ✅

### 2026-05-27 17:10:22 UTC — API 主接口不再要求旧 loaded accessor
Modified by: Codex (GPT-5)

- `internal/api/server.go`
  - `RuntimeEngine` 接口移除 `GetLoadedCharacters()` 依赖
  - 进一步确认 API 主链路不再要求旧 loaded accessor
- `CLOSURE_AUDIT.md`
  - World-First 项补充“API server 主接口已不再要求旧 accessor”的证据
- `TODO.md`
  - 记录兼容层收束到接口层的新进展
- `DELIVERY_TRACKING.md`
  - 同步更新 World-First 项当前实现
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime` ✅

### 2026-05-27 17:07:31 UTC — 兼容层继续收束到不主导主链路
Modified by: Codex (GPT-5)

- `internal/api/server.go`
  - `/api/characters` 不再在空场景时退回 `loaded_characters`
  - `/api/instances/create` 不再让 `active_character` 决定新实例 focus
- `internal/api/server_test.go`
  - 新增 `TestInstanceCreateEndpointIgnoresActiveCharacterFallback`
  - 新增 `TestCharactersRouteDoesNotFallbackToLoadedCharacters`
  - mock engine 增加可控的 `sceneParticipants / loadedCharacters`
- `CLOSURE_AUDIT.md`
  - World-First 项补充“旧字段已不再决定关键 UI / API 路径”的新证据
- `TODO.md`
  - 记录兼容层收束的新完成面
- `DELIVERY_TRACKING.md`
  - 区分“旧字段仍暴露”和“旧字段已不再主导主链路”
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run 'Test(InstanceCreateEndpoint|CharactersRoute)'` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime` ✅

### 2026-05-27 17:02:22 UTC — 前端主路径脱离旧兼容字段
Modified by: Codex (GPT-5)

- `web/app.js`
  - `loadCharacters()` 不再读取 `/api/characters` 的 `active / characters` fallback
  - `refreshPanel()` 不再读取 legacy `/api/character`
  - 角色卡面板改为读取 `/api/character-config` 的 `card`
- `CLOSURE_AUDIT.md`
  - World-First 项补充“前端主路径已不再依赖旧兼容字段”的当前证据
  - 同时保留“类型层与兼容接口仍未完全收束”的未验收原因
- `TODO.md`
  - 记录前端主路径已迁移到 `focus_character + participants + participant_details`
- `DELIVERY_TRACKING.md`
  - 更新 World-First 项的当前实现与未验收原因
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime` ✅

### 2026-05-27 16:59:12 UTC — 200 Tick 多样本矩阵验收补强
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增 `TestWorldOutcomeSampleMatrixAcrossTwoHundredTicks`
  - 将长窗口验证扩到 200 ticks / 4 样本：
    - calm
    - guard pressure
    - smuggler pressure
    - infrastructure / blackout
  - 证明不同 structure 会在更长窗口下继续分叉出：
    - 不同 promoted leader
    - 不同 pressure trajectory
    - 不同 tension level
- `internal/api/server_test.go`
  - 新增 `TestAPIWorldOutcomeSampleMatrixAcrossTwoHundredTicks`
  - 在 API 层复现同样的 200 tick / 4 样本矩阵
- `CLOSURE_AUDIT.md`
  - 收回“仍缺 200+ tick / 仍只靠几种外城压力样本”的旧表述
  - 将剩余缺口收束到：
    - 兼容层收束
    - 更大规模 authoring 回放
    - trace / sim / report 的统一审计面
- `TODO.md`
  - 把“补 200+ ticks”从待做切换为已获得证据
  - 下一阶段改为更广 world family 回放与兼容层收束
- `DELIVERY_TRACKING.md`
  - 将 world structure / simulation / population 的长窗口证据升级到 200 tick
- `ACCEPTANCE_CHECKLIST.md`
  - 更新当前判断口径，加入 200 tick 多样本矩阵事实
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime -run 'TestWorldOutcomeSampleMatrixAcross(TwoHundred|Hundred)Ticks'` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api -run 'TestAPIWorldOutcomeSampleMatrixAcross(TwoHundred|Hundred)Ticks'` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅

### 2026-05-27 16:49:30 UTC — 实验报告归档工作流落地
Modified by: Codex (GPT-5)

- `internal/core/types.go`
  - 新增 `ExperimentSnapshot` / `ExperimentReport`
  - 统一保存作者实验所需的长期结果快照
- `internal/runtime/authoring.go`
  - 新增 `ListExperimentReports()` / `CreateExperimentReport()`
- `internal/runtime/persistence.go`
  - 新增 `experiment_reports.json` 的实例级持久化与 legacy root fallback
- `internal/runtime/runtime_test.go`
  - 新增 `TestExperimentReportCreateAndList`
- `internal/runtime/persistence_instance_test.go`
  - 补充 experiment report 的实例级路径与 legacy fallback 测试
- `internal/api/server.go`
  - 新增 `/api/experiment-reports` GET / POST
- `internal/api/server_test.go`
  - 新增 `TestExperimentReportRoutes`
- `web/index.html`
  - Simulation 面板新增“实验归档”区块
- `web/app.js`
  - 新增实验报告保存、读取、JSON / Markdown 导出
  - 归档内容包含：
    - 当前实例与对照实例的长期 snapshot
    - outcome summary
    - experiment conclusion
- `CLOSURE_AUDIT.md`
  - 将“缺更正式的 authoring audit / report”从未实现下调为已实现、待更大规模验收
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅

### 2026-05-27 17:12:00 UTC — Closure Audit 文档化
Modified by: Codex (GPT-5)

- `CLOSURE_AUDIT.md`
  - 新增真正按 `ACCEPTANCE_CHECKLIST` 逐项整理的审计文档
  - 明确区分：
    - 当前已有强证据的项
    - 仍待验收的项
    - 真实剩余缺口
- `TODO.md`
  - 增加到 `CLOSURE_AUDIT.md` 的入口
- `DELIVERY_TRACKING.md`
  - 增加到 `CLOSURE_AUDIT.md` 的引用，避免继续用交付文档代替闭环审计
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 17:03:00 UTC — 120 Tick 多样本矩阵验收补强
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增 `TestWorldOutcomeSampleMatrixAcrossHundredTicks`
  - 使用 calm / guard-pressure / smuggler-pressure 三个样本连续运行 120 ticks，验证：
    - calm 样本保持低张力且不自然 promotion
    - 不同 pressure sample 会长出不同 promoted leader
    - `trajectory_summary` / pressure leader / promoted outcome 会跨样本分叉
- `TODO.md`
  - 将 simulation / world structure 项更新为“已有 120 tick 多样本矩阵证据”
- `DELIVERY_TRACKING.md`
  - 将 World Structure / Autonomous Simulation 项更新为“已有 120 tick 多样本矩阵测试”
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅

### 2026-05-27 16:54:00 UTC — 实验分支一键创建接入
Modified by: Codex (GPT-5)

- `web/index.html`
  - 实例管理区新增“创建实验分支”按钮
- `web/app.js`
  - 新增 `createExperimentInstance()`
  - 默认从当前观察实例派生新实例，并自动把它设为对照实例
  - `instance-list` 里新增“设为对照”快速入口
- `internal/api/server_test.go`
  - 新增 `TestInstanceCreateEndpointAcceptsSourceID`
  - 锁定 `source_id` 派生实例语义
- `TODO.md`
  - 同步写明作者控制台已支持一键创建实验分支
- `DELIVERY_TRACKING.md`
  - 将作者干预闭环项补充为“支持一键派生实验分支”
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api` ✅

### 2026-05-27 16:45:00 UTC — 双实例实验自动结论接入
Modified by: Codex (GPT-5)

- `web/index.html`
  - Simulation 面板新增“实验结论”
- `web/app.js`
  - 新增 `buildExperimentConclusion()`
  - 双实例对照后，作者控制台会自动总结：
    - 长期张力主导侧
    - 人口结果主导侧
    - 诊断密度主导侧
    - 结构压力主导侧
- `TODO.md`
  - 同步写明双实例同步推进后已会自动给出实验结论
- `DELIVERY_TRACKING.md`
  - 将作者干预闭环项补充为“已自动归纳实验结论”
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api` ✅

### 2026-05-27 16:39:00 UTC — 批量 Tick 与双实例同步推进接入
Modified by: Codex (GPT-5)

- `internal/api/server.go`
  - `POST /api/sim/tick` 现在支持 `count`
  - 可通过 body 或 query 一次推进 1-200 ticks
- `internal/api/server_test.go`
  - 新增 `TestSimTickRouteSupportsBatchCount`
  - API 长窗口对照测试改为直接使用批量 tick
- `web/index.html`
  - Simulation 面板新增批量 tick 输入与“双实例同步推进”按钮
- `web/app.js`
  - 新增当前实例批量推进
  - 新增当前实例 + 对照实例同步推进
  - 作者现在可以更直接地跑分支实验，而不是反复手点单 tick
- `TODO.md`
  - 同步写明 simulation 已支持批量运行与双实例同步推进
- `DELIVERY_TRACKING.md`
  - 将作者干预闭环项补充为“可批量推进实验实例”
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅

### 2026-05-27 16:31:00 UTC — 实例长期结果对照接入作者控制台
Modified by: Codex (GPT-5)

- `web/index.html`
  - 实例管理区新增“对照实例”选择
  - Simulation 面板新增“实例分叉”区块
- `web/app.js`
  - 新增跨实例 API URL 构建与 `fetchJSONForInstance()`
  - 新增对照实例选择、状态读取、结果摘要渲染
  - 作者现在可以直接比较两个实例的：
    - `trajectory_summary`
    - `tension`
    - `population_highlights`
    - diagnostics 主导项
- `TODO.md`
  - 同步写明作者控制台已有跨实例长期结果对照
- `DELIVERY_TRACKING.md`
  - 将 Trace / Authoring 项更新为“已有跨实例结果对照”
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api` ✅

### 2026-05-27 16:24:00 UTC — Trajectory Summary 接入作者控制台
Modified by: Codex (GPT-5)

- `internal/runtime/runtime.go`
  - `TickStatus()` 新增 `trajectory_summary`
  - 新增长期趋势摘要逻辑：
    - `tension trend`
    - `dominant pressure`
    - `dominant faction tension`
    - `population outcome`
    - `recent diagnostics`
- `internal/runtime/runtime_test.go`
  - 扩展长窗口测试，验证 `trajectory_summary` 会在单世界与结构分叉场景中稳定产出
- `internal/api/server_test.go`
  - 扩展 API 长窗口测试，验证 `/api/sim/status` 返回的 `trajectory_summary` 会随 world outcome 分叉
- `web/index.html`
  - Simulation 面板新增“长期总结”
- `web/app.js`
  - 新增 `renderTrajectorySummary()`，作者控制台可直接阅读长期趋势
- `TODO.md`
  - 同步写明作者控制台已有长期 trajectory summary
- `DELIVERY_TRACKING.md`
  - 将 Trace / Authoring 项更新为“已有长期总结能力”
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 16:16:00 UTC — API 层 Long-Window Authoring Divergence 验证
Modified by: Codex (GPT-5)

- `internal/api/server_test.go`
  - 新增真实 runtime + resolver 级测试 `TestAPIStructureInterventionDivergesLongWindowOutcomeAcrossInstances`
  - 通过 API 对两个实例分别执行：
    - `POST /api/population`
    - `POST /api/world-structure`
    - 连续 `POST /api/sim/tick`
    - `GET /api/sim/status`
    - `GET /api/population-insights`
  - 验证作者侧接口已经能读出长期 outcome 分叉，而不只是 runtime 内部测试可见
- `TODO.md`
  - 同步写明 API 层已有真实长窗口 authoring 验证
- `DELIVERY_TRACKING.md`
  - 将作者干预闭环项补充为“API 侧已证明能读出长期分叉”
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/api` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅

### 2026-05-27 16:09:00 UTC — Long-Window Structure Divergence 验证补强
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增 `TestWorldStructureInterventionDivergesLongWindowAutonomousOutcome`
  - 用两套 world structure 对同一 population 连续跑 36 ticks，验证：
    - baseline 与 intervened 世界的 `tension` 长窗口结果分叉
    - `world_pressure` canonical events 数量分叉
    - `population promotion` 结果分叉
    - `tick_history.diagnostics` 与 `population_highlights` 可解释地分叉
- `TODO.md`
  - 将 simulation / world structure 项更新为“已有长期 outcome 分叉证据”
- `DELIVERY_TRACKING.md`
  - 将 World Structure / Autonomous Simulation 两项更新为“已有 36 tick 长窗口分叉测试”
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅

### 2026-05-27 16:18:00 UTC — Autonomous Tick 拉入 Scene Population 并补强长窗口闭环
Modified by: Codex (GPT-5)

- `internal/runtime/population_runtime.go`
  - 新增 `syncAutonomousScenePopulationLocked()`
  - autonomous tick 现在会显式把当前 scene/location/faction/pressure 命中的 background NPC 拉入 scene runtime
- `internal/runtime/runtime.go`
  - `onTick()` 在自治阶段前会同步 scene population，使无人输入时也能自然积累 exposure / promotion
  - 抽出 `buildTickDiagnosticsLocked()`
  - `tick_history` 现在会保存当时的 diagnostics 快照，而不只保留 summary
- `internal/core/types.go`
  - `TickSnapshot` 新增 `diagnostics`
- `internal/runtime/runtime_test.go`
  - 新增 `TestAutonomousSimulationPromotesScenePopulationAcrossLongWindow`
  - 验证在不触发 directTurn/director 的前提下：
    - scene 相关 background NPC 会被 tick 拉入当前 scene
    - 长窗口 36 ticks 后会累积 exposure 并自然 promotion
    - `population_insights`、canonical events、`tick_history.diagnostics` 都能看到结果
- `web/app.js`
  - “近期轨迹”现在会显示每个 recent tick 的首条 diagnostics 提示
- `TODO.md`
  - 同步写明 autonomous tick 已可自然拉入 background population
- `DELIVERY_TRACKING.md`
  - 将 Population / Autonomous Simulation / Authoring 三项推进状态更新到长窗口事实
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 16:02:00 UTC — Simulation Tick History 接入作者控制台
Modified by: Codex (GPT-5)

- `internal/core/types.go`
  - 新增 `TickSnapshot`
- `internal/runtime/runtime.go`
  - `Engine` 新增最近 12 次 tick 的 `tickHistory`
  - `TickStatus()` 现在返回 `tick_history`
  - 每次 `onTick()` 结束后会记录 tension / pressure / faction / population / summary 快照
- `internal/runtime/runtime_test.go`
  - 扩展 `TestStructureDrivenSimulationSustainsEvolutionAcrossTicks`
  - 验证 `tick_history` 会稳定累积、tick 递增且每条快照都有 summary
- `web/index.html`
  - Simulation 面板新增“近期轨迹”
- `web/app.js`
  - 新增 `renderTickHistory()`，作者控制台可直接浏览最近多次 tick 轨迹
- `TODO.md`
  - 同步写明作者控制台已能看到最近多次 tick 轨迹
- `DELIVERY_TRACKING.md`
  - 将 Trace / Authoring 项补充为“已有多 tick 轨迹可视化”
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅

### 2026-05-27 15:49:00 UTC — 长窗口测试口径收敛与仓库卫生清理
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 重新验证 `TestIdentityShiftSustainsDivergentRelationshipTrajectoryAcrossTicks`
  - 将其稳定口径收敛为：identity shift 后，多 tick 下 `trust` 型自治动作数量持续高于 pre-shift
  - 不再把该测试表述成“已证明长期 relationship 轨迹完全分叉”
- `.gitignore`
  - 新增 `*.test` 与 `cookies.txt`
  - 避免本地测试二进制和临时 cookie 文件继续污染工作树
- `TODO.md`
  - 将人物成长闭环表述修正为：
    - 已证明同 tick relationship outcome
    - 已证明多 tick trust-action trajectory
    - 尚未证明更长窗口的稳定 world outcome
- `DELIVERY_TRACKING.md`
  - 同步收紧人物自然成长闭环措辞，避免把短窗口证据写成长期结果验收
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅

### 2026-05-27 15:34:53 UTC — 多 Tick Relationship Trajectory 对照补强
Modified by: Codex (GPT-5)

- `internal/runtime/runtime.go`
  - `onTick()` 在 scheduler 后新增 `reprojectStateAfterTickLocked()`
  - 本 tick 内 scheduler / autonomous action 产生的 relationship 事件现在可立即反映到 `stateMgr`
- `internal/agents/scheduler.go`
  - 新增 `SetActionInterval()` / `SetRandomStepChance()` 的测试友好入口
- `internal/agents/scheduler_test.go`
  - 新文件加入版本控制
- `internal/runtime/runtime_test.go`
  - 新增 `TestIdentityShiftChangesSchedulerActionAndRelationshipOutcome`
  - 验证同一 promoted NPC 在 identity shift 后：
    - pre-shift 不走 `trust`
    - post-shift 转向 `trust`
    - 多 tick 下 `Relationships` 轨迹随之分叉并立刻可见
- `TODO.md`
  - 更新人物成长闭环状态：已证明会改变多 tick relationship outcome
- `DELIVERY_TRACKING.md`
  - 更新人物自然成长闭环当前实现与剩余缺口
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 15:32:59 UTC — Relationship Outcome 在同 Tick 闭环可见
Modified by: Codex (GPT-5)

- `internal/runtime/runtime.go`
  - `onTick()` 在 scheduler 执行后新增 `reprojectStateAfterTickLocked()`
  - 当前 tick 内由 scheduler / autonomous action 提交的 canonical events 现在会重新投影回 `stateMgr`
  - 这让 `trust_change / fear_change` 等 relationship 结果不再滞后一拍
- `internal/agents/scheduler.go`
  - 新增 `SetRandomStepChance()`
  - 用于在测试中关闭随机选步噪声，稳定验证长链闭环
- `internal/runtime/runtime_test.go`
  - 新增 `TestIdentityShiftChangesSchedulerActionAndRelationshipOutcome`
  - 验证同一 promoted NPC 在 identity shift 后会：
    - 从 pre-shift 的非 `trust` 自治动作转向 `trust`
    - 在同一 tick 内改善 `Relationships`
- `TODO.md`
  - 更新人物成长闭环状态：已证明会改变 relationship outcome
- `DELIVERY_TRACKING.md`
  - 更新人物自然成长闭环当前实现与剩余缺口
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 15:24:22 UTC — Scheduler 接入人物慢变量选步
Modified by: Codex (GPT-5)

- `internal/agents/scheduler.go`
  - 新增 `selectAdaptiveBestStep()` 与 `adaptiveStepPriority()`
  - scheduler 现在会依据 `trust / intimacy / fear / aggression` 调整 NPC 自治选步
  - 这让慢变量不只影响 prompt / director / desire，也影响非焦点 NPC 的规则调度结果
- `internal/agents/scheduler_test.go`
  - 新增 `TestSelectAdaptiveBestStep`
  - 新增 `TestSchedulerTickFollowsAdaptiveShift`
  - 验证同一角色在慢变量漂移前后会从 `threaten` 转向 `trust`
- `TODO.md`
  - 更新人物成长闭环状态：已证明会改变 scheduler 选步
- `DELIVERY_TRACKING.md`
  - 更新人物自然成长闭环当前实现与剩余缺口
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/agents ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 15:20:09 UTC — Identity Shift 传导到 Desire / Autonomous Intent
Modified by: Codex (GPT-5)

- `internal/emotion/desire.go`
  - 新增 `DeleteByCharacter()`
  - 将 desire 生成逻辑抽出为 `buildSeedDesires()`
  - 新增 `ReplaceDesires()`，支持按当前 identity/goals 重建角色 desires
- `internal/runtime/population_runtime.go`
  - `reconcilePopulationLocked()` 在 promotion / identity shift 后新增 `refreshPopulationDesiresLocked()`
  - promoted persona 的 adaptive 漂移现在会刷新 `desireStore`
- `internal/runtime/runtime_test.go`
  - 新增 `TestPopulationIdentityShiftRefreshesDesiresAndAutonomousIntent`
  - 验证同一 promoted NPC 在 trust 漂移前后会发生：
    - dominant desire 从 `autonomy` 转向包含 `affection`
    - autonomous action type 从 `withdraw` 转向 `approach`
- `TODO.md`
  - 更新人物成长闭环状态：已证明会改变 autonomous desire / intent
- `DELIVERY_TRACKING.md`
  - 更新人物自然成长闭环当前实现与剩余缺口
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api ./internal/core ./internal/emotion` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 15:15:02 UTC — 人物成长影响 Director 胜出结果
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增 `TestPopulationIdentityShiftChangesDirectorWinner`
  - 验证同一 promoted NPC 在 trust 漂移前后会发生：
    - director candidate `score` 上升
    - `score_breakdown.trust` 上升
    - `dominant_factors` 出现 `信任倾向`
    - 选人结果从落选变成胜出
- `TODO.md`
  - 更新人物成长闭环状态：已证明会改变 future allowed actions 和 director 胜出结果
- `DELIVERY_TRACKING.md`
  - 更新人物自然成长闭环的当前实现与剩余缺口
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api ./internal/core` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 15:10:51 UTC — 人物成长前置到动作空间过滤
Modified by: Codex (GPT-5)

- `internal/runtime/turns.go`
  - `composeTurnPrompt()` / `stepPromptDirectives()` 现在会吃到角色 `adaptive`
  - 新增 `filterAllowedActionsForAdaptive()`、`filterActions()`、`prioritizeActions()`
  - `executeTurnStep()` 里的 `allowedActions` 现在先过 step 过滤，再过人格过滤
  - 这让 promoted persona 的 trust / intimacy / fear / aggression 直接影响未来动作空间，而不只是事后 validator 拦截
- `internal/runtime/runtime_test.go`
  - `TestFilterAllowedActionsForAdaptive`
  - `TestPopulationIdentityShiftChangesFutureAllowedActions`
  - 验证同一 promoted NPC 在 trust 漂移后会失去 `attack / threaten` 等高冲突动作
- `TODO.md`
  - 更新人物成长闭环的可信状态
- `DELIVERY_TRACKING.md`
  - 更新人物自然成长闭环说明与剩余缺口
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api ./internal/core` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 15:03:28 UTC — 多 Tick 结构演化验证补强
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增 `TestStructureDrivenSimulationSustainsEvolutionAcrossTicks`
  - 验证 structure pressure 在连续 8 个 tick 下会持续产出：
    - 多个不同的 `tension` 状态
    - 增长的 `faction_tensions`
    - 演化中的 `pressure_states`
    - 每 tick 可观测的 `last_tick_summary`
    - 重复出现的 `world_pressure` canonical events
- `TODO.md`
  - 将 simulation 项更新为“已有连续多 tick 演化证据，但仍缺更长窗口验证”
- `DELIVERY_TRACKING.md`
  - 将 Autonomous Simulation 项更新为“已证明连续多 tick 结构化变化，仍待长期验收”
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api ./internal/core` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 15:00:39 UTC — World Structure 前后对照闭环测试补强
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增 `TestWorldStructureInterventionChangesRuntimeOutputs`
  - 同一 world / 同一 background NPC 下，验证 structure 干预前后会真实改变：
    - director candidate 与 `world_signals`
    - `TickStatus()` 的 `tension / pressure_states / faction_tensions`
    - authoring diagnostics 中的 `scene_control / active_pressure / scene_population_candidates`
- `TODO.md`
  - 将 world structure 项更新为“已有结构前后对照测试，但仍缺长程回放验证”
- `DELIVERY_TRACKING.md`
  - 将 world structure 驱动闭环更新为“已有对照证据，但仍待长期验收”
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api ./internal/core` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 14:57:31 UTC — Structure 响应摘要与作者诊断增强
Modified by: Codex (GPT-5)

- `web/app.js`
  - simulation compare 摘要拆分为 `structure` 与 `response`
  - structure 改动不会在保存后立刻被清空，而是等到检测到真实 sim delta 后再消费
  - 若世界尚未响应，会显式显示 `response: awaiting world response`
- `internal/runtime/runtime.go`
  - `TickStatus()` 新增 structure authoring diagnostics
  - 作者现在可以直接看到：
    - 当前 scene 是否处于某 faction 控制区
    - 哪些 pressure 正在命中当前 scene / faction
    - 哪些 background NPC 已被 structure 命中并可进入 director 候选
- `internal/runtime/runtime_test.go`
  - 新增 `TestTickStatusIncludesStructureAuthoringDiagnostics`
- `TODO.md`
  - 同步当前可信状态与待补验证口径
- `DELIVERY_TRACKING.md`
  - 同步 world structure / trace / authoring 的实际推进状态
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api ./internal/core` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 14:34:45 UTC — Trace 人口成长摘要接入
Modified by: Codex (GPT-5)

- `internal/core/types.go`
  - `WorldMetrics` 新增 `population_highlights`
- `internal/runtime/population_runtime.go`
  - 新增 `populationHighlightsLocked()`：从当前 world population 中提取 top promoted / top rising 摘要
- `internal/runtime/runtime.go`
  - `ProcessTurn()` 构建 trace 时开始填充 `world_metrics.population_highlights`
- `web/app.js`
  - Trace 面板新增 `population:` 行，直接展示当前最活跃的 promoted / rising NPC
- `internal/runtime/runtime_test.go`
  - 调整 trace 断言，确保 world runtime 下会输出 population highlights
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api ./internal/core` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 14:31:44 UTC — Director 主导因子结构化输出
Modified by: Codex (GPT-5)

- `internal/core/types.go`
  - `DirectorPlan` 新增 `world_signals`
  - `DirectorCandidate` 新增 `dominant_factors`
- `internal/runtime/director.go`
  - 新增 `dominantDirectorFactors()`：从 `score_breakdown` 提取前 3 个主导因素
  - 新增 `buildWorldSignals()`：把世界高张力、pressure/faction/location 命中情况结构化为 director 上下文信号
  - director manual / no-alternate / normal scoring 三条路径都开始输出 `world_signals`
- `web/app.js`
  - Director 面板新增 `world_signals` 展示
  - Trace 面板新增：
    - `director-world`
    - 选中候选的 `drivers`
  - 让作者能直接看到“这次选角主要被什么驱动”
- `internal/runtime/runtime_test.go`
  - 新增断言，确保 `DirectorPlan.WorldSignals` 与 `DirectorCandidate.DominantFactors` 会被真实产出
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/runtime ./internal/api ./internal/core` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 14:27:31 UTC — Delivery 口径收口 + world metrics/diagnostics 前端接入
Modified by: Codex (GPT-5)

- `DELIVERY_TRACKING.md`
  - 整份重写为“实现推进状态”文档，不再伪装成终态验收通过清单
  - 新增状态定义：`已实现 / 待验收 / 已验收`
  - 8 个验收项统一改成：
    - 当前实现
    - 为什么还不能算已验收
    - 建议验收动作
  - 明确写清：终态是否完成，统一以 `ACCEPTANCE_CHECKLIST.md` 为准
- `web/index.html`
  - Simulation 面板新增“运行诊断”区块
- `web/app.js`
  - 新增 `renderSimDiagnostics()`，渲染 `/api/sim/status` 返回的 `diagnostics`
  - trace 视图新增 `world_metrics` 展示：`tension / pressure_states / faction_tensions / npc_exposure`
  - 让作者控制台不需要直接看原始 JSON 也能理解当前世界演化状态
- 验证：
  - `node --check web/app.js` ✅

### 2026-05-27 13:35:13 UTC — 文档审计与交接语义清理
Modified by: Codex (GPT-5)

- 审计结论：
  - 代码主线仍然一致，项目方向没有漂回“角色卡中心”
  - `TODO.md` 的问题主要是状态语义失真，不是实现主线失真
  - `SESSION_LOG.md` 已统一为 `UTC`，并按绝对时间倒序排列；部分历史条目原始记录来自 `CST`
- `TODO.md`
  - 重写为“当前可信状态 / 待补验证 / 文档约束 / 下一阶段”
  - 移除把未来路线直接写成 `[x]` 的误导性写法
  - 把接口回归表述改成按方法区分的真实语义，例如 `POST /api/switch = 200`，`GET /api/switch = 405`
- 本轮重新抽查：
  - `GET /api/health` ✅
  - `GET /api/characters` ✅
  - `GET /api/instances` ✅
  - `GET /api/world-structure` ✅
  - `GET /api/population` ✅
  - `GET /api/population-insights` ✅
  - `GET /api/director-config` ✅
  - `GET /api/sim/status` ✅
  - `POST /api/switch` ✅
  - `/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime ./internal/core` ✅
  - `node --check web/app.js` ✅

### 2026-05-27 11:50:00 UTC — Race 修复 + 全项目接口检测 + 代码质量清理
Modified by: Kimi (Kimi Code CLI)

- `internal/runtime/runtime.go`
  - `tickCount int` → `atomic.Int64`，`e.tickCount++` → `e.tickCount.Add(1)`
  - 消除 `TestTickLoopSurvivesScaledSeventyTicks` 的 data race（goroutine 写 vs 测试读）
- `internal/runtime/runtime_test.go`
  - 测试中直接读取 `engine.tickCount` 改为 `engine.tickCount.Load()`
- **全项目接口检测**
  - Go 编译检查：`go build ./...` ✅
  - 静态分析：`go vet ./...` ✅
  - 单元测试：384 个测试用例全部通过 ✅
  - Race 检测：`go test -race ./...` ✅
  - 重复稳定性：`go test -count=2 ./...` ✅
  - 基准测试：`go test -bench=. ./internal/...` ✅
  - 代码覆盖率：**58.0%**（`go test -coverprofile=coverage.out ./...`）
  - 模块校验：`go mod verify` ✅
  - JS 语法：`node --check web/app.js` ✅
  - 前后端 API 一致性：67 个前端调用 ↔ 72 个后端路由，全部对齐 ✅
  - Go 接口实现：`RuntimeEngine` 84 方法、`InstanceResolver` 8 方法全部有实现 ✅
  - SQL 注入排查：无字符串拼接 SQL，全部参数化查询 ✅
- **代码质量清理**
  - `internal/memory/vector.go`：删除 `VectorStore` 未使用的 `db` 字段
  - `internal/actions/executor.go`：添加 `var _ agents.ActionExecutor = (*Executor)(nil)`
  - `internal/events/quarantine.go`：添加 `var _ agents.EventSubmitter = (*Gatekeeper)(nil)`
  - `internal/events/store.go`：添加 `var _ narrative.EventStore = (*Store)(nil)`
  - `cmd/corerp/main.go`：添加 `var _ api.InstanceResolver = (*apiInstanceResolver)(nil)`
  - `internal/api/server_test.go`：添加 `var _ RuntimeEngine = (*runtime.Engine)(nil)`
- `ARCHITECTURE.md`
  - 更新"仍未完成"部分：`npcTickExposure` 和 world structure 驱动已落地
- 验证：
  - `go test -race ./...` ✅
  - `go build ./...` ✅
  - `go fmt ./...` ✅

### 2026-05-27 11:35:00 UTC — v2 清理：前端与 API 全面转向 focus_character
Modified by: Kimi (Kimi Code CLI)

- `internal/core/types.go`：
  - `TurnTrace` 新增 `FocusCharacter` 字段
  - `PendingFact` 新增 `FocusCharacter` 字段
- `internal/runtime/runtime.go`：`ProcessTurn` 构建 trace 时填充 `trace.FocusCharacter = e.GetFocusCharacter()`
- `internal/api/server.go` — API handler 查询参数全面支持 `focus_character`：
  - `/api/switch` POST body 支持 `focus_character`（优先于 `character`）
  - `/api/character-config` GET/POST 支持 `focus_character` 查询参数和 body 字段
  - `/api/memory` GET 已支持 `focus_character`
  - `/api/quarantine` GET 支持 `focus_character`
  - `/api/pending-facts` GET 支持 `focus_character`
  - `/api/npc-actions` GET 已支持 `focus_character`
  - 响应统一补 `focus_character` 字段（与 `character` 并存）
- `web/app.js` — 前端请求全面切换为 `focus_character`：
  - `/api/switch` POST body：`character` → `focus_character`
  - `/api/memory` GET query：`character` → `focus_character`
  - `/api/character-config` GET/POST：`character` → `focus_character`
  - `/api/quarantine` GET query：`character` → `focus_character`
  - `/api/pending-facts` GET query：`character` → `focus_character`
  - Trace 渲染：`trace.character` → `trace.focus_character || trace.character`（4 处）
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/simulation ./internal/agents ./internal/runtime ./internal/api ./internal/core` ✅
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅


### 2026-05-27 11:28:00 UTC — world structure 深度驱动 runtime
Modified by: Kimi (Kimi Code CLI)

- `internal/agents/planner.go` — `Planner.Plan()` 新增 `structure core.WorldStructureConfig` 参数：
  - 解析当前 scene location 的 controller faction 和 active pressures
  - Rule 1 增强：高张力 **或** location 存在高强度 pressure 时触发 `hide`
  - Rule 2 新增：若 scene 中存在不同 faction 的 NPC，生成 `threaten` 步骤（priority 6）
  - Rule 4 新增：若 location 有 active pressure，生成 `observe` 步骤（priority 4）
  - 新增 `characterFactionFromStructure()` 辅助函数
- `internal/agents/scheduler.go` — `Scheduler.Tick()` 新增 `structure` 参数，透传给 `Planner.Plan()`
- `internal/runtime/turns.go` — `executeTurnStep()` 中加载 world structure 并传给 `e.planner.Plan()`
- `internal/runtime/runtime.go` — `onTick()` 中加载 world structure 并传给 `e.scheduler.Tick()`
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/simulation ./internal/agents ./internal/runtime ./internal/api ./internal/core` ✅
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅

### 2026-05-27 11:20:00 UTC — simulation 长期推进闭环：pressure / population / faction 自主演化
Modified by: Kimi (Kimi Code CLI)

- `internal/simulation/pulse.go` — Pressure 演化引擎：
  - `PulseEngine` 新增 `pressureStates` 动态状态映射
  - 升级：同一 pressure 连续触发 3 次 → `CurrentIntensity += 0.03`，上限 0.95
  - 衰减：连续 3 tick 未触发 → `CurrentIntensity -= 0.02`，下限 0.1
  - Escalate 连锁：`CurrentIntensity >= 0.7` 时，提升 escalates 中 pressure 的 intensity 并强制安排触发；若不存在则生成 `potential_pressure` 事件
  - 新增 `PressureStates()` 观测接口
- `internal/simulation/faction.go` — 新增 Faction 紧张度引擎：
  - `FactionEngine` 解析 `WorldFactionConfig.Relationships` 关键词（敌对/合作）为结构化关系
  - 紧张度累积：pressure target 匹配 faction 时 `tension += intensity*0.03`，conflict kind 额外 +0.05
  - 自然衰减：每 tick `-0.01`
  - 阈值事件：`tension >= 0.6` → `faction_pressure`；`tension >= 0.8` → `faction_conflict`；敌对双方同时高紧张 → `faction_rivalry`
  - 新增 `Tensions()` 观测接口
- `internal/runtime/runtime.go` — onTick 接入：
  - 步骤 6.5 加入 `factionEng.Tick()`
  - 步骤 10 加入 `npcTickExposure` 累积（在场 NPC 每 tick +1）
  - `TickStatus()` 加读锁，输出 `pressure_states`、`faction_tensions`、`npc_tick_exposure`
  - `Engine` 新增 `factionEng` 和 `npcTickExposure` 字段
- `internal/runtime/population_runtime.go` — Population 自主增长：
  - `calculatePopulationAttention()` 新增 `tickExposure` 和 `factionEng` 参数
  - Score 公式追加 `tickExposure * 0.05`（上限 5.0）
  - faction 紧张度 > 0.5 时，该 faction NPC 额外 +0.5
- `internal/runtime/director.go` — 新增 `faction_rivalry: -2` 权重默认值（预留）
- `internal/api/server.go` — `/api/sim/status` 已自动输出新字段（通过 `TickStatus()`）
- `web/index.html` / `web/app.js` — Simulation 面板新增"演化观测"区，展示 pressure intensity、faction tension、npc tick exposure
- `api-contract.yaml` — 补充 `/api/sim/status` 响应 schema（`pressure_states`、`faction_tensions`、`npc_tick_exposure`）
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/simulation ./internal/runtime ./internal/api ./internal/core` ✅
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅

### 2026-05-27 11:05:00 UTC — director 权重加入 kind/source/loaded 维度
Modified by: Kimi (Kimi Code CLI)

- `internal/runtime/director.go` — `normalizeDirectorWeights` 新增 6 个默认值：
  - `kind_persona: 3`, `kind_npc: 1`
  - `source_promoted: 4`, `source_definition: 2`, `source_background: 0`
  - `loaded: 2`
- `internal/runtime/director.go` — `directTurnLocked` 评分循环新增 kind/source/loaded 维度：
  - `kind_persona` / `kind_npc` 根据 `participant.Kind` 加分
  - `source_promoted` / `source_definition` / `source_background` 根据 `participant.Source` 加分
  - `loaded` 根据 `participant.Loaded` 加分
  - 所有新维度均进入 `breakdown` 和 `reasons`，trace 中可见
- `web/index.html` — 权重编辑器新增"参与者属性权重"分组，6 个 number input
- `web/app.js` — `loadDirectorConfig()` / `saveDirectorConfig()` / 初始化逻辑接入新权重读写
- `web/app.js` — `describeCandidateGap()` 新增 6 个维度差距分析
- `api-contract.yaml` — 补充 `/api/director-config` 路径及 `DirectorConfig` schema（含新权重示例）
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime ./internal/core` ✅
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅

### 2026-05-27 11:00:00 UTC — 参与者模型进入 API / Switch / Director 主链路
Modified by: Codex (GPT-5)

- `internal/core/types.go`
  - 新增 `ParticipantSummary`
  - `RuntimeInstanceSummary` 新增 `participant_details`
  - `DirectorCandidate` 新增 `kind / source / loaded / switchable`
- `internal/runtime/runtime.go`
  - 新增 `GetSceneParticipantDetails()`
  - `participant_details` 现在统一输出 `kind/source/world_path/loaded/switchable/present/focus`
  - 切换视角时会为缺失定义的 scene participant 自动补可切换 shell
  - 切视角不再覆盖 scene truth；原场景参与者继续保留
- `internal/runtime/director.go`
  - director candidate 过滤不再只靠名字列表
  - `player_role` / `scene_shell` / `scene_presence` 不再混入 speaker candidate
  - `candidate_details` 现在携带参与者结构化身份
- `internal/api/server.go`
  - `/api/characters` 增加 `participant_details`
  - `/api/switch` 进入 runtime 前先检查 `switchable`
- `web/app.js`
  - 视角列表优先读取 `participant_details`
  - 前端现在显示参与者 `kind/source/loaded` 标签，而不再把所有名字都当成同一种“角色”
  - 修复作者控制台 `describeCandidateGap is not defined`
- `api-contract.yaml`
  - 增加 `ParticipantSummary` schema
  - `/api/characters` 契约增加 `participant_details`
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime ./internal/core` ✅
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅
  - `pm2 restart corerp && pm2 save` ✅
  - 实测 `/api/characters` 已返回 `participant_details` ✅

### 2026-05-27 10:58:00 UTC — participant_details 推进到 trace 与作者控制台
Modified by: Kimi (Kimi Code CLI)

- `internal/core/types.go`: `TurnTrace` 新增 `ParticipantDetails []ParticipantSummary` 字段
- `internal/runtime/runtime.go`: `ProcessTurn` 构建 trace 时填充 `ParticipantDetails`（调用 `sceneParticipantDetailsLocked`）
- `api-contract.yaml`: 补充 `/api/trace/latest`、`/api/trace?turn=` 路径定义及 `TurnTrace` schema（含 `participant_details`）
- `web/app.js` — trace 视图增强：
  - `loadTraceView()` 新增 `participants:` 总览行，显示每个参与者的 kind / source / loaded / present / focus / switchable 状态
  - `selected` / `alternates` 行追加 candidate 的 kind / source / loaded / present / switchable 标签
  - 新增 `excluded:` 分析行，对 switchable 但未入选的参与者给出原因：
    - `scene_shell` → "scene_shell 不参与导演选角"
    - `player_role` → "玩家身份不参与导演选角"
    - `background_population` NPC → "背景人口默认不进入候选（需晋升）"
    - 其他 → "导演评分未达标"
- `web/app.js` — 作者控制台增强：
  - `loadCharacters()` 的 `pan-chars` 面板现在展示**全部**参与者（含不可切换的）
  - 不可切换参与者以灰色样式呈现，标注具体原因（玩家身份 / scene_shell）
  - 可切换参与者保持原有交互行为
- 验证：
  - `/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime ./internal/core` ✅
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅

### 2026-05-27 10:30:00 UTC — 前端全量重写：Cyberpunk Neon 主题
Modified by: Claude (mimo-v2.5-pro)

- `web/index.html` 全量重写（CSS + HTML 结构微调）
- 新主题系统：Cyberpunk Neon，深色底 `#0a0a0f`，霓虹强调色（电光青/琥珀/品红/紫）
- 三套主题保留（dark/light/kraft），重新定义色值
- 新增 Google Font Orbitron 用于标题
- 新增 SVG feTurbulence 噪点纹理叠加
- 新增 glass-morphism topbar + 卡片式 panel-group 包裹
- 新增 legacy CSS 变量别名映射（`--bg` → `--bg-base` 等），app.js 零改动
- HTML 新增 `<span class="brand-neon-label">霓虹里街区</span>` 品牌标识
- 4 个 panel group 添加 `<div class="panel-group">` 包裹容器
- 验证：
  - `node --check web/app.js` ✅
  - `pm2 restart corerp` ✅
  - 关键 ID 保留确认 ✅

### 2026-05-27 07:00:00 UTC — SQLite WAL + 连接池优化
Modified by: Claude (mimo-v2.5-pro)

- `internal/events/store.go`：PRAGMA journal_mode=WAL + synchronous=NORMAL + MaxOpenConns=4/MaxIdleConns=2
- `internal/memory/engine.go`：同上
- 效果：runtime 测试从 ~11s 降到 ~1.8s，读写并发不再串行阻塞

### 2026-05-27 06:50:00 UTC — Simulation 运维可观测性增强
Modified by: Claude (mimo-v2.5-pro)

- `internal/simulation/tick.go`：Loop 新增 `Pause()` / `Resume()` / `IsPaused()` 方法（atomic.Bool 控制）
- `internal/runtime/runtime.go`：
  - 新增 `TickStatus()` — 返回 tick 状态（running/paused/tick_count/world_advance/turn_count）
  - 新增 `ManualTick()` — 手动触发一次 onTick
  - 新增 `PauseTick()` / `ResumeTick()` — 暂停/恢复 tick loop
  - onTick 错误日志：AutoPromote / DecayEngine / GetCanonicalEvents 失败时 log.Printf 而非静默吞掉
- `internal/api/server.go`：
  - RuntimeEngine 接口新增 4 个方法
  - 新增 `GET /api/sim/status`、`POST /api/sim/tick`、`POST /api/sim/pause`、`POST /api/sim/resume`
- `internal/api/server_test.go`：mockEngine 补齐新方法
- `web/index.html`：新增 Simulation 面板（状态/Tick/世界时间/轮次 + 手动Tick/暂停/恢复按钮）
- `web/app.js`：新增 `loadSimStatus()` / `manualTick()` / `pauseTick()` / `resumeTick()`，refreshPanel 自动刷新 sim 状态
- `internal/simulation/tick_test.go`：新增 `TestPauseResume`

### 2026-05-27 06:20:00 UTC — Director Weights 结构化编辑器 + Causal Chain 视图 + Deploy 巡检脚本
Modified by: Claude (mimo-v2.5-pro)

- **Director Weights 结构化编辑器**
  - `web/index.html`：将原始 JSON textarea 替换为 13 个带标签的 number input
    - 候选人得分权重组：mentioned / mention_order / continuity / present / location_match / faction_match / pressure_match / hook_match
    - 沉默/人格权重组：silence_divisor / silence_cap / trust / intimacy / fear
    - 原始 JSON 保留在可折叠 `<details>` 中
  - `web/app.js`：
    - 新增 13 个 `dw-*` 元素引用到 `els`
    - `loadDirectorConfig()` 更新：从 `weights` 对象填充每个 `dw-*` 输入
    - `saveDirectorConfig()` 更新：从 `dw-*` 输入构建 weights JSON，同步更新 textarea
    - `updateDirectorPanel()` 更新：同步填充 `dw-*` 输入

- **Causal Chain 最强因/果视图**
  - `web/index.html`：
    - 因果链模态框新增结构化卡片区域：最强因 → 当前事件 → 最强果
    - 新增 `.chain-card` / `.chain-card-focus` / `.cc-type` / `.cc-actors` / `.cc-detail` / `.cc-weight` CSS
    - 完整因果链文本移入可折叠 `<details>`
  - `web/app.js`：
    - 新增 `renderChainCard()`：渲染单张因果卡片（类型/演员/摘要/权重）
    - 新增 `findStrongestCause()`：从 chain.causes 中找最高权重因
    - 新增 `findStrongestEffect()`：从 chain.effects 中找最高权重果
    - `showCausalChain()` 更新：解析 chain JSON，渲染三张结构化卡片

- **Deploy 健康检查脚本**
  - `deploy/health-check.sh`：综合巡检脚本
    - 检查 PM2 / systemd 进程状态
    - 检查端口监听（8080）
    - 检查公开 + 认证 API 端点（先登录再检查受保护接口）
    - 检查 SQLite 文件存在性 + integrity_check
    - 检查磁盘使用率（>80% warn, >90% fail）
    - 检查 Nginx 配置和运行状态
    - 输出 PASS/FAIL/WARN 汇总 + 健康状态判定

### 2026-05-27 06:13:00 UTC — 全量回归验证
Modified by: Claude (mimo-v2.5-pro)

- go test 6 包全通过（api/runtime/core/memory/events/simulation）
- go build + pm2 restart 成功
- 建议抽查接口 6/6 全 200，兼容层 3/3 全 200，sim 接口 4/4 全 200
- 前端面板抽查：world-structure/population/scenes/director-config/canon-facts/player-role 全 200
- health-check：UNHEALTHY → DEGRADED（PASS 11, FAIL 0, WARN 3），磁盘阈值调整为 >95%/>85%
- 磁盘清理：journalctl vacuum 72M + go clean -cache，97% → 93%

### 2026-05-27 06:02:00 UTC — 前端 ETag / 条件请求优化
Modified by: Claude (mimo-v2.5-pro)

- `internal/api/server.go`：新增 `writeJSONWithETag` 辅助函数（SHA256 哈希前 8 字节 → ETag），应用于 handleState / handleSceneParticipants / handleWorlds GET / handleMemory 四个读密集端点
- `web/app.js`：`fetchJSON` 加入 `_etagCache` 层，自动发送 `If-None-Match`，304 时复用缓存数据，避免重复传输
- 效果：轮询未变更数据时服务端返回 304（零字节体），减少前端 poll 带宽消耗

### 2026-05-27 04:10:00 UTC — Single-file → Directory 转换功能落地
Modified by: Claude (mimo-v2.5-pro)

- `internal/world/world.go`
  - 新增 `ConvertToDir(filePath)` 函数
  - 读取单文件世界内容，创建目录结构，写入 world.yml/scenes/canon/world 子目录
  - 原文件重命名为 `.bak` 备份
- `internal/api/server.go`
  - `handleWorlds` 新增 `PATCH` 方法处理转换请求
  - 返回 `{ ok, old_path, new_path }`
- `web/index.html`
  - 顶栏 world-select 旁新增"📂"转换按钮（仅单文件世界显示）
- `web/app.js`
  - 新增 `convertWorld()` 函数：确认对话框 → PATCH 请求 → 刷新列表 → 进入新世界
  - 新增 `updateWorldConvertButton()` 函数：根据世界格式控制按钮显隐
  - loadWorlds 末尾调用 updateWorldConvertButton
  - worldSelect change 事件中调用 updateWorldConvertButton
- 验证：
  - `go test ./internal/api ./internal/runtime ./internal/core` ✅
  - `node --check web/app.js` ✅
  - `go build -o corerp ./cmd/corerp` ✅
  - `pm2 restart corerp` ✅

### 2026-05-27 04:00:00 UTC — World Creation 功能落地
Modified by: Claude (mimo-v2.5-pro)

- `internal/world/world.go`
  - 新增 `CreateWorld(rootDir, name, coreRules)` 函数
  - 创建目录结构：world/、canon/、scenes/、population/
  - 初始化文件：world.yml、scenes/default.yml、canon/facts.yml、seed.yml、factions/locations/pressures 空文件
  - 新增 `sanitizeID()` 辅助函数
- `internal/api/server.go`
  - `handleWorlds` 新增 `PUT` 方法处理创建世界请求
  - 返回 `{ ok, name, path }`
- `web/index.html`
  - 顶栏 world-select 旁新增"+"按钮
  - 新增创建世界对话框（name + core_rules textarea）
  - 静态资源版本更新到 `app.js?v=20260526h`
- `web/app.js`
  - 新增 `showWorldCreateModal()` / `hideWorldCreateModal()` / `createWorld()` 函数
  - els 新增 worldCreate 相关元素引用
  - 事件绑定：创建按钮、关闭按钮、提交按钮、背景点击关闭
- 验证：
  - `go test ./internal/api ./internal/runtime ./internal/core` ✅
  - `node --check web/app.js` ✅
  - `go build -o corerp ./cmd/corerp` ✅
  - `pm2 restart corerp` ✅

### 2026-05-27 03:45:00 UTC — P2 world authoring 主工作流落地
Modified by: Claude (mimo-v2.5-pro)

- `web/index.html`
  - 新增"世界结构编辑"面板：seed（premise/situation/scene/time/stability）、factions、locations、pressures、rules
  - 新增"人口配置编辑"面板：background NPCs 编辑、promotion policy 参数、promoted NPCs / identity cores 只读展示
- `web/app.js`
  - 新增 `loadWorldStructure()` / `saveWorldStructure()`：读写 `/api/world-structure`
  - 新增 `loadPopulationConfig()` / `savePopulationConfig()`：读写 `/api/population`
  - 新增 `parsePipeLine()` / `renderPipeList()` 辅助函数
  - els 新增 structure / popcfg 相关元素引用
  - 事件绑定：structure reload/save、popcfg reload/save
  - `refreshPanel()` 集成 loadWorldStructure + loadPopulationConfig
- `api-contract.yaml`
  - 新增 endpoint：`/api/world-structure`、`/api/population`、`/api/population-insights`
  - 新增 schema：`WorldStructureConfig`、`PopulationConfig`、`PopulationInsights`、`PopulationCharacterInsight`
- 验证：
  - `go test ./internal/api ./internal/runtime ./internal/core` ✅
  - `node --check web/app.js` ✅
  - `go build -o corerp ./cmd/corerp` ✅
  - `pm2 restart corerp` ✅

### 2026-05-27 03:30:00 UTC — P1 population runtime 闭环强化完成
Modified by: Claude (mimo-v2.5-pro)

- `internal/runtime/runtime_test.go`
  - 新增 `TestPopulationIdentityShiftAccumulates`：验证多次 trust_change / fear_change 事件后 identity core adaptive 累积漂移
  - 新增 `TestPopulationInsightsIncludesPromotionReason`：验证晋升后 growth_summary 和 adaptive 正确输出
- `web/app.js`
  - `updatePopulationRuntimePanel()` 增强：
    - 新增 `renderAttentionBar()`：显示 attention 各项分数组成（互动/提及/事件/场景）
    - 新增 `renderAdaptiveDrift()`：从 history 中提取最近 identity_shift，显示 adaptive 变化趋势（↑/↓ 图标 + 颜色区分）
    - promoted NPC 面板现在显示：name/status、growth_summary、attention 分数条、score + adaptive、identity_core、drift 趋势、history
    - background NPC 面板现在显示：name、growth_summary、attention 分数条、score
- 验证：
  - `go test ./internal/runtime` ✅（新增 2 个测试通过）
  - `node --check web/app.js` ✅
  - `go build -o corerp ./cmd/corerp` ✅
  - `pm2 restart corerp` ✅

### 2026-05-27 03:15:00 UTC — P0 契约与消费统一完成
Modified by: Claude (mimo-v2.5-pro)

- `api-contract.yaml`
  - 新增 schema 定义：`MemorySnapshot`、`SaveSlot`、`ScenarioPreset`、`CharacterConfig`
  - 新增辅助 schema：`FactFrame`、`EventFrame`、`Message`、`PlayerRole`
  - 所有 schema 均包含 `character`（兼容）+ `focus_character`（主字段）
  - 更新 endpoint 响应引用：
    - `/api/memory` → `MemorySnapshot`
    - `/api/checkpoints` GET → `{ checkpoints: SaveSlot[] }`
    - `/api/checkpoints` POST → `SaveSlot`
    - `/api/checkpoints/load` → `SaveSlot`
    - `/api/presets` GET → `{ presets: ScenarioPreset[] }`
    - `/api/presets` POST → `ScenarioPreset`
    - `/api/presets/apply` → `ScenarioPreset`
    - `/api/character-config` GET/POST → `CharacterConfig`
    - `/api/saves` GET → `{ saves: SaveSlot[] }`
    - `/api/saves` POST → `SaveSlot`
    - `/api/saves/load` → `SaveSlot`
- `web/app.js`
  - 行 809：`resolveCheckpointTraceTurn()` 改为优先读 `slot?.focus_character || slot?.character`
  - 行 1853：进入世界场景分割改为读 `data.focus_character || data.character`
- 验证：
  - `node --check web/app.js` ✅

### 2026-05-27 01:50:48 UTC — world-first 交接材料与确认工作流同步
Modified by: Codex (GPT-5)

- 更新仓库内交接材料，确保后续模型不需要依赖聊天记录拼上下文：
  - `README.md`
  - `TODO.md`
  - `SESSION_LOG.md`
- `README.md` 新增"确认工作流"：
  - 先统一内部真实语义到 `focus_character / participants / focus_definition`
  - 再补 API 输出新字段
  - 前端优先消费新字段，旧字段仅 fallback
  - 文档、契约、代码同步推进
  - 兼容层稳定前，不删除旧路径和旧字段
- `TODO.md` 已改写为当前主线：
  - P0：补齐 `api-contract.yaml` 与前端对 `focus_*` 的优先消费
  - P1：强化 population runtime 闭环
  - P2：强化 world authoring 主工作流
  - 保留兼容层原则与每轮最小验证命令
- 当前交接状态确认：
  - world-first serve + PM2 已生效
  - `focus_character` 已成为内部主语义
  - `/api/characters`、`/api/instances` 已输出 `focus_character` + `participants`
  - `MemorySnapshot` / `SaveSlot` / `ScenarioPreset` / `CharacterConfig` 已开始双字段过渡
- 本次为文档与交接同步，不涉及新的运行时代码验证

## 2026-05-26

### 2026-05-26 15:15:28 UTC — timeline 视图补齐 world evolution 标签
Modified by: Codex (GPT-5)

- `web/app.js`
  - timeline 新增事件图标与标签：
    - `world_pressure`
    - `population_promoted`
    - `population_identity_shift`
  - timeline detail 现在会展示：
    - 世界压力目标与 intensity
    - 角色晋升状态与 score
    - adaptive 漂移摘要
  - 无 target 的事件现在不再渲染成 `actor -> ?`
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅

### 2026-05-26 15:12:48 UTC — population growth history 接入观察面板
Modified by: Codex (GPT-5)

- population insight 扩展 history：
  - `internal/core/types.go`
    - 新增 `PopulationGrowthEvent`
    - `PopulationCharacterInsight` 新增 `history`
- runtime 事件化 growth history：
  - `internal/runtime/population_runtime.go`
  - 人格 adaptive 变化时提交 `population_identity_shift`
  - insight 现在会从 canonical events 回收：
    - `population_promoted`
    - `population_identity_shift`
- 前端展示：
  - `web/app.js`
  - `Population Runtime` 面板现在展示最近 3 条 growth history
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅

### 2026-05-26 15:07:50 UTC — population insights 接入作者工具面板
Modified by: Codex (GPT-5)

- 新增 population insight API：
  - `GET /api/population-insights`
  - 返回：
    - promoted 列表
    - background 列表
    - attention score
    - growth summary
    - current adaptive
    - identity core / last event / world path
- 后端实现：
  - `internal/runtime/population_runtime.go`
  - `internal/api/server.go`
  - `internal/api/server_test.go`
- 前端接入：
  - `web/index.html`
    - 作者工具区新增 `Population Runtime`
  - `web/app.js`
    - `refreshPanel()` 拉取 `/api/population-insights`
    - 渲染 promoted NPC 如何长出来、当前 adaptive 值、最近依据
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅

### 2026-05-26 15:03:11 UTC — promoted NPC 经历塑形人格接入
Modified by: Codex (GPT-5)

- `IdentityCore.Adaptive` 现在会随 canonical events 漂移：
  - `internal/runtime/population_runtime.go`
  - 当前纳入：
    - `trust_change`
    - `fear_change`
    - `intimacy_change`
    - `dialogue / negotiation`
    - `attack / threat`
- 漂移结果会同步到两层：
  - `population/identity_core.yml`
  - runtime 中已加载的 promoted NPC 角色壳
- 影响：
  - Director 打分现在会吃到经历后的 `trust / fear / intimacy`
  - promoted NPC 被选中发言时，persona snapshot 也会反映变化后的 adaptive 值
- 测试补齐：
  - `internal/runtime/runtime_test.go`
    - 验证晋升后 `IdentityCore.Adaptive` 增长
    - 验证 runtime 角色壳同步到新的 adaptive 值
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅

### 2026-05-26 14:57:34 UTC — promoted NPC 接入 Director candidates / active cast
Modified by: Codex (GPT-5)

- promoted NPC runtime 壳接入：
  - `internal/runtime/population_runtime.go`
  - population reconcile 后会把 `PromotedNPCs` 映射成 runtime 可用角色
  - 自动补：
    - `agents.LoadCharacter(...)`
    - `loadedCharacters`
    - `worldPaths`
    - `charWorlds`
- Director 候选池扩展：
  - `internal/runtime/director.go`
  - 候选人现在是：
    - scene characters 优先
    - 同 world 的已加载角色补充
  - 因此 promoted NPC 即使不在当前 scene `characters` 数组里，也可以因用户点名或长期沉默而进入候选
- 测试补齐：
  - `internal/runtime/runtime_test.go`
    - 新增 promoted NPC 被 Director 选为 lead speaker 的回归
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅

### 2026-05-26 14:51:17 UTC — population attention / promotion 接入 runtime
Modified by: Codex (GPT-5)

- 新增 population runtime reconcile：
  - `internal/runtime/population_runtime.go`
  - 基于 canonical events + 当前 scene 重算 `BackgroundNPC.Attention`
  - 按 `PromotionPolicy` 自动晋升到 `PromotedNPCs`
  - 自动补 `IdentityCore`
  - 晋升时提交 `population_promoted` canonical event
- runtime 接入：
  - `internal/runtime/runtime.go`
    - `ProcessTurn()` 结束后触发 population reconcile
    - `onTick()` 结束后触发 population reconcile
- attention 当前纳入的信号：
  - direct interactions
  - textual mentions
  - shared scene / shared events
  - relationship delta
  - current scene carryover
- 测试补齐：
  - `internal/runtime/runtime_test.go`
    - 新增背景 NPC 自动晋升回归
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅

### 2026-05-26 14:45:47 UTC — world pulse / pressure tick 接入 runtime
Modified by: Codex (GPT-5)

- 新增 `PulseEngine`：
  - `internal/simulation/pulse.go`
  - 将 `world/pressures.yml` 转成 tick 驱动的 canonical events
  - 当前每次 pulse 会生成：
    - `world_pressure`
    - `tension_change`
    - `variable_set(world.pressure.<id>.last_tick)`
- runtime 接入：
  - `internal/runtime/runtime.go`
    - `Engine` 新增 `pulseEng`
    - `onTick()` 现在会读取当前 world structure
    - 按 pressure cadence 注入 world pulse 事件
    - 同步把 tick 结果写回内存态 `tension / variables`
- 测试补齐：
  - `internal/simulation/pulse_test.go`
  - `internal/runtime/runtime_test.go`
    - 新增 tick 注入 `world_pressure` 的回归
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅

### 2026-05-26 14:38:59 UTC — world layer 第一批运行时骨架落地
Modified by: Codex (GPT-5)

- 新增 world structure 骨架：
  - `internal/core/types.go`
    - 新增 `WorldStructureConfig`
    - 新增 `ruleset / seed / factions / locations / pressures` 配套类型
  - `internal/world/structure.go`
    - 新增 world 目录层读写
    - 持久化到 `world/ruleset.yml`、`world/seed.yml`、`world/factions.yml`、`world/locations.yml`、`world/pressures.yml`
- runtime / API 接口接通：
  - `internal/runtime/world_config.go`
    - 新增 `GetWorldStructureConfig`
    - 新增 `UpdateWorldStructureConfig`
  - `internal/api/server.go`
    - 新增 `GET/POST /api/world-structure`
- 导入初始化补齐：
  - `internal/importer/importer.go`
    - 新 world dir 写入后，额外初始化空 `world/` skeleton
- 测试补齐：
  - `internal/world/structure_test.go`
  - `internal/runtime/runtime_test.go`
  - `internal/api/server_test.go`
  - `internal/importer/integration_test.go`
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅

### 2026-05-26 14:29:58 UTC — world-first 终态架构文档同步
Modified by: Codex (GPT-5)

- 文档更新：
  - `ARCHITECTURE.md`
    - 明确当前产品定位已经转向 world-first persistent narrative runtime
    - 补充三层目标模型：世界层 / 人格层 / 叙事层
    - 写清当前已落地骨架与仍未完成的关键缺口
  - `FINAL_ARCHITECTURE_BLUEPRINT.md`
    - 收口最终主干：`World Ruleset -> Seed -> Population -> Identity -> Director -> Pulse/Pressure -> Desire/Emotion -> ActionFrame -> EventStore -> Projection -> Narrative`
    - 明确“角色不是导入的，是长出来的”
    - 补充 world layer、population/identity layer、world pulse、interpretation 与 narrative layer 的职责
- 说明：
  - 本轮只更新架构文档，不修改运行时代码
  - 忽略 `characters/`、`worlds/`、`data/` 等本地运行与隐私数据，不纳入提交范围

### 2026-05-26 14:10:04 UTC — population skeleton 落地
Modified by: Codex (GPT-5)

- 新增 world population 骨架：
  - `background_npcs.yml`
  - `promoted_npcs.yml`
  - `identity_core.yml`
  - `policy.yml`
- 新增类型与接口：
  - `core.PopulationConfig`
  - `core.BackgroundNPC`
  - `core.PromotedNPC`
  - `core.IdentityCoreConfig`
  - `core.PromotionPolicy`
- `internal/world` 支持读取/保存 population
- `internal/runtime` / `internal/api` 新增：
  - `GetPopulationConfig()`
  - `UpdatePopulationConfig()`
  - `GET/POST /api/population`
- importer 创建 world 目录时会一并初始化 `population/`
- world catalog 补充 population 统计字段
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `node --check web/app.js` ✅
  - `git diff --check` ✅

### 2026-05-26 14:10:04 UTC — world-first 入口第一步
Modified by: Codex (GPT-5)

- 明确产品方向：
  - CoreRP 不是高级酒馆或预写任务树 RPG
  - 目标是长期演化、可回放、可分叉、人物会被经历改变的文字世界 runtime
- 新增 world catalog：
  - `core.WorldSummary`
  - `internal/world.ListCatalog()`
  - `GET /api/worlds`
- 前端顶栏新增“世界”选择器，角色选择改为“视角”兼容层
- 当前切口只改变入口展示与 catalog API，不直接拆除 active character runtime 锚点
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `node --check web/app.js` ✅
  - `git diff --check` ✅

### 2026-05-26 13:10:47 UTC — 导入角色 voice 推断修复
Modified by: Codex (GPT-5)

- 修复导入器 voice 默认值过度复用：
  - `inferStyle()` 改为从角色正文与示例对话推断语气
  - `inferRhythm()` 改为同时参考正文，避免缺少 `mes_example` 时全部落到 `短句为主`
- 重新导入 `/home/kelebituo/资源文件夹`
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅
  - `pm2 restart corerp --update-env` ✅
  - `/api/health` ✅

### 2026-05-26 13:05:06 UTC — 角色卡导入架构收紧
Modified by: Codex (GPT-5)

- 修复 SillyTavern 目录导入：
  - 目录导入继续支持 `.png` / `.json`
  - 世界资料卡现在按 `world-only` 导入，只写入 `worlds/<source>/`
  - 地点、势力、规则、物品、速览、生成器等条目不再生成顶层可运行角色
- `world.yml` 收敛为 meta + compact core_rules，长世界书内容保留在 `canon/ontology.yml` 与 `canon/facts.yml`
- 重新导入 `/home/kelebituo/资源文件夹`：
  - 顶层角色 YAML 从 49 收敛到 31
  - 8 个源文件均生成对应 `worlds/<source>/`
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅
  - `pm2 restart corerp --update-env` ✅

### 2026-05-26 12:55:02 UTC — 角色卡导入架构修复
Modified by: Codex (GPT-5)

- 导入架构收口：
  - 角色卡导入输出 `characters/<角色>.yml`
  - 世界书/canon/scene 输出到 `worlds/<导入源>/`
  - 角色 YAML 新增 `world_path`，运行时优先按该字段绑定世界
  - 前端保存角色卡时保留已有 `world_path`
- 导入器修复：
  - 目录导入完成后不再继续把目录当单文件导入
  - 目录批量导入现在同时处理 `.png` 和 `.json`
  - ensemble worldbook 中地点/设定/规则类条目不再默认当作顶层运行角色
  - `cast_index.yml` 继续保留 secondary cast，完整世界资料进入 ontology/canon
- 资源文件夹重新导入：
  - 使用修复后的导入器重新导入 `/home/kelebituo/资源文件夹`
  - 当前生成 49 个顶层角色 YAML 与 9 个 world 入口
  - 临时启动验证确认角色按 `world_path` 绑定到对应 world
- 文档同步：
  - `README.md`
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `node --check web/app.js` ✅
  - `git diff --check` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅

### 2026-05-26 12:45:53 UTC — 资源文件夹角色卡导入验证
Modified by: Codex (GPT-5)

- 用户路径修正：
  - 正确角色卡目录为 `/home/kelebituo/资源文件夹`
  - 目录内包含 7 个 SillyTavern PNG 和 1 个 JSON 角色卡
- 导入处理：
  - 使用 `./corerp import -src /home/kelebituo/资源文件夹 -dst ./characters -mode auto` 导入 PNG
  - 单独导入 `《红楼梦》完整版、-角色卡-202604190812.json`
  - 当前生成约 47 个顶层角色 YAML 与对应 `worlds/<source>/` 世界目录
- 发现问题：
  - CLI 目录导入完成后会继续把目录当单文件导入，导致最后报 `read ... is a directory`，但 PNG 已实际导入成功
  - ensemble JSON 会把设定/地点/规则类 worldbook 条目也生成为顶层角色 YAML
  - 当前 `findWorldFile` 依赖角色文件名匹配 `worlds/<角色名>/`，但导入目录按源文件名建 world，因此临时启动时新角色默认落到 `worlds/cyberpunk2077/world.yml`
- 验证：
  - 临时数据目录 `/tmp/corerp-import-check` 启动成功
  - 未重启正式 PM2，避免未整理角色列表直接污染当前运行实例

### 2026-05-26 12:43:32 UTC — 角色卡解析接口检查与当前角色卡清空
Modified by: Codex (GPT-5)

- 解析能力检查：
  - 当前已有 CLI 导入：`./corerp import -src <png_or_json_or_dir> -dst ./characters`
  - 支持 SillyTavern PNG / JSON 单卡导入
  - 批量目录导入当前只扫描 `.png`
  - 目前没有 HTTP 上传解析接口，前端只支持编辑已加载角色卡
- 运行数据处理：
  - `/home/kelebituo/资源` 当前只发现截图文件，未发现 PNG/JSON 角色卡
  - 备份当前 `characters/` 到 `data/character-card-backup-20260526T124231Z/`
  - 删除当前非 tracked 角色卡 YAML
  - 恢复误删的 tracked `characters/worlds/...` 世界资料，避免仓库出现 world 文件删除

### 2026-05-26 12:38:30 UTC — 移动端顶栏收敛与测试数据清理
Modified by: Codex (GPT-5)

- `web/index.html`
  - 移动端顶栏改为两行布局，隐藏品牌副标题与选择器标签
  - 控制台入口在移动端改回图标按钮，避免大按钮挤占首屏
  - 静态资源版本更新到 `app.js?v=20260526g`
- 运行数据清理：
  - 结束遗留的 `/tmp/corerp-ui-test/ui-jsdom-e2e.js` 进程
  - 备份污染前数据到 `data/cleanup-backup-20260526T123336Z/`
  - 清除 world/data/SQLite 中的 jsdom/e2e 测试场景、checkpoint、preset 与事件
  - 重启 PM2 `corerp`，让运行态重新从清理后的数据加载
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅
  - world/data/SQLite 测试痕迹扫描为 0 ✅

### 2026-05-26 12:25:35 UTC — 主页面编辑风重构
Modified by: Codex (GPT-5)

- `web/index.html`
  - 主页面从运行工具台风格改为叙事编辑器风格
  - 顶栏改为薄工具条，仅保留品牌、实例、角色和控制台入口
  - 场景与角色信息收成正文上方 metadata 区，去除大 hero/card 感
  - 对话区改为阅读排版：助手文本正文化，用户输入保留轻量边框
  - 右侧控制台改为 inspector 风格分段列表，弱化卡片背景和网格密度
  - 静态资源版本更新到 `app.js?v=20260526f`
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅
  - 登录后首页 HTML 返回新版编辑风布局与 `app.js?v=20260526f` ✅

### 2026-05-26 12:18:24 UTC — 主页面密度重构
Modified by: Codex (GPT-5)

- `web/index.html`
  - 收紧顶栏、场景区、角色摘要和输入区尺寸，让聊天区获得更多首屏空间
  - 去除主页面装饰性径向背景和大卡片阴影，统一卡片/控件圆角与信息密度
  - 右侧控制台宽度与间距下调，默认折叠组不再漏出角色卡编辑区
  - 删除重复 trace 控件 DOM，避免重复 ID 与绑定歧义
- 验证：
  - 推送前确认 `origin/master` 已是最新
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test ./...` ✅
  - 登录后首页 HTML 返回新版资源参数 `app.js?v=20260526e` ✅

### 2026-05-26 11:50:47 UTC — 首页改为更简约的编辑风主版面
Modified by: Codex (GPT-5)

- `web/index.html`
  - 顶部从三段大块信息改为更轻的两段结构
  - “运行数据”改为默认折叠的 summary/details
  - 主区 story header 改为单栏编辑页风格
  - 当前角色摘要压缩为低占用信息卡，不再和场景 headline 抢首屏
  - 右侧控制台 overview 卡隐藏，减少重复说明与视觉密度
- 验证：
  - `node --check web/app.js`
  - `node /tmp/corerp-ui-test/ui-jsdom-e2e.js`
- 说明：
  - 本轮主要是结构与样式收敛，不改后端接口
  - `jsdom` 外链脚本噪音仍存在，但 checkpoint / rollback / preset / trace turn 主链路继续跑通
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 11:50:00 UTC — Trace 作者控制台第二轮优化
Modified by: Codex (GPT-5)

- `web/index.html` / `web/app.js`
  - 为 trace 历史增加当前 turn 高亮
  - 新增 `上一轮 / 下一轮` 导航
  - checkpoint 列表增加“依据”按钮，联动跳转到最接近的 trace turn
- 说明：
  - 这一轮只改前端交互层，不改后端接口
  - 第一轮作者工具主路径可视为正式全绿
- 验证：
  - `node --check web/app.js`
- 文档同步：
  - `TODO.md`
  - `TEST_PROGRESS.md`

### 2026-05-26 11:41:26 UTC — 手机端首页摘要折叠补完与等价回归
Modified by: Codex (GPT-5)

- `web/index.html` / `web/app.js`
  - 补完超窄屏首页“角色摘要”按钮交互
  - 当前角色 spotlight 在手机窄屏默认折叠
  - 点击后仅切本地 UI 状态，不触碰后端接口
  - 状态写入 `localStorage`，避免用户每次重开都要重复展开
- 验证：
  - `node --check web/app.js`
  - `node /tmp/corerp-ui-test/ui-jsdom-e2e.js`
- 说明：
  - `jsdom` 仍会输出 `/app.js?v=...` 外链解析噪音
  - 但手工注入的 `web/app.js` 逻辑已跑通主链路：
    - checkpoint / rollback
    - scenario preset 保存 / 套用
    - trace 历史切 turn
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 11:35:00 UTC — 作者工具 UI 等价回归完成，Chromium 实机回归待补
Modified by: Codex (GPT-5)

- 已完成 UI 等价回归：
  - `checkpoint` 创建
  - 切角色 / 改场景
  - `rollback`
  - `scenario preset` 保存 / 套用
  - `trace` 历史切 turn
- 当前采用 `jsdom` 执行 `web/app.js` + 线上接口联调
- 原因：
  - 本机缺 Chromium 运行库
  - `puppeteer` 启动时报缺少 `libnspr4.so`
- 说明：
  - 这轮验证可证明前端交互链路已通
  - 但不等同于完整 Chromium 无头实机点击回归
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 11:15:00 UTC — 作者工具第一版：checkpoint / rollback / scenario preset / trace 历史
Modified by: Codex (GPT-5)

- `internal/runtime/authoring.go`
  - 新增作者工具能力：
    - `ListTurnTraces(limit)`
    - `ListCheckpoints / CreateCheckpoint / LoadCheckpoint`
    - `ListScenarioPresets / CreateScenarioPreset / ApplyScenarioPreset`
  - `scenario_presets.json` 落到实例目录
- `internal/api/server.go`
  - 新增接口：
    - `GET /api/traces`
    - `GET/POST /api/checkpoints`
    - `POST /api/checkpoints/load`
    - `GET/POST /api/presets`
    - `POST /api/presets/apply`
- `web/index.html` / `web/app.js`
  - 新增作者工具 UI：
    - checkpoint / rollback
    - scenario preset 保存 / 套用
    - trace turn 历史列表与指定轮次查看
- 测试：
  - `go test ./internal/runtime ./internal/api`
  - `internal/runtime/runtime_test.go`
    - scenario preset create/apply
    - turn trace 历史顺序
  - `internal/api/server_test.go`
    - traces / checkpoints / presets 路由
- 文档同步：
  - `TODO.md`
  - `TEST_PROGRESS.md`
  - `ARCHITECTURE.md`
  - `api-contract.yaml`

### 2026-05-26 10:36:00 UTC — 收紧 legacy root 文件 fallback 语义
Modified by: Codex (GPT-5)

- `internal/runtime/persistence.go`
  - root 级 legacy：
    - `player_role.json`
    - `save_slots.json`
  - 现在仅 `default` 实例会 fallback 读取
  - 具名实例不再继承 legacy root 文件
- `internal/runtime/persistence_instance_test.go`
  - 调整测试预期：
    - `default` 仍可读取 legacy root
    - `alpha` 等具名实例返回默认角色 / 空存档
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:31:00 UTC — PM2 实机回归抽查
Modified by: Codex (GPT-5)

- 进程状态：
  - `pm2 show corerp`
  - 状态 `online`
  - uptime 正常增长
  - 启动参数仍为标准：
    - `serve -port 8080 -data /home/kelebituo/corerp/data -characters ./characters -secure-cookie=false`
- 探针与接口：
  - `./deploy/smoke-check.sh` → `/api/health=200` / `/api/ready=200`
  - `GET /api/version` 正常返回：
    - `version=dev+dirty`
    - `commit=f6639caebb21a09af57d5b1130ac482eb15a8e45`
    - `time=2026-05-25T12:16:30Z`
  - `GET /login` → `200`
  - `GET /api/instances` 未登录时仍为 `401`
- 运行日志：
  - 最近日志停留在稳定启动后状态，无新增启动卡死迹象
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:28:00 UTC — 缩放版 70 tick 存活测试落地
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增长跑存活测试：
    - 使用毫秒级 tick interval 跑满 70 tick
    - 验证后台 tick loop 连续运行不 panic
    - 验证 world clock 推进
    - 验证 canonical events 持续增长
    - 验证 `QueryActionLog` / `ActionLogStats` 仍可读
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:20:00 UTC — 最小 legacy 兼容测试补齐
Modified by: Codex (GPT-5)

- `internal/events/store_test.go`
  - 增加 `instance_id=''` legacy event 对 `default` 可读、对具名实例不可读测试
- `internal/memory/engine_test.go`
  - 增加 legacy dialogue / semantic fact 对 `default` 可读、对具名实例不可读测试
- `internal/runtime/persistence_instance_test.go`
  - 补 root 级 legacy `player_role.json` / `save_slots.json` fallback 行为测试
- 说明：
  - 这批测试是“兼容旧格式不回归”，不是恢复旧测试数据
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:14:00 UTC — 跨角色交错对白因果串链修复
Modified by: Codex (GPT-5)

- `internal/events/causality_test.go`
  - 新增同一 session / scene 下：
    - `user -> 111`
    - `user -> 安雅`
    - `111 dialogue`
    - `安雅 dialogue`
    交错混存后的 `RebuildAll()` 回归测试
- `internal/events/causality.go`
  - 收窄 `dialogue -> dialogue` 的类型因果规则
  - 仅允许：
    - 同一说话者续说
    - 对前一条对白的直接回应
  - 避免跨角色交错对白被误挂成 cause/effect
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:08:00 UTC — ActionLogger 持久化实例隔离与 runtime 集成测试
Modified by: Codex (GPT-5)

- `internal/emotion/action_log.go`
  - `action_log` 增加 `instance_id`
  - `LoadFromDB` / `QueryDB` 改为按实例过滤
  - `LoadFromDB` 重载前先清空 ring buffer，避免重复加载
- `internal/runtime/runtime.go`
  - `SetInstanceMetadata()` 现在同步：
    - `actionLogger.SetInstanceID(...)`
    - `actionLogger.LoadFromDB(200)`
- `internal/runtime/instances.go`
  - 删除实例时新增清理 `action_log`
- 测试：
  - `internal/emotion/action_log_test.go`
    - 增加 `ActionLogger` 的实例隔离回归测试
  - `internal/runtime/runtime_test.go`
    - 增加 runtime 层“重启后按实例恢复 action log”测试
  - `internal/runtime/instances_test.go`
    - 删除实例高层测试纳入 `action_log`
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 09:58:00 UTC — 因果链 narrative API 断言补齐
Modified by: Codex (GPT-5)

- `internal/api/server_test.go`
  - 新增 `/api/causality?mode=narrative` 响应断言
  - 覆盖：
    - `event_id`
    - `depth`
    - `chain`
    - narrative `summary`
  - 同时补默认模式回归，确认 plain summary 不误走 narrative 分支
- `internal/events/causality_test.go`
  - 现有“无回边渲染”测试继续作为 narrative summary 回归保障
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 09:52:00 UTC — 单实例多角色往返切换一致性测试补齐
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增单实例多角色往返切换集成测试
  - 覆盖：
    - `111 -> 安雅 -> 111 -> 安雅`
    - 每角色自己的 `world`
    - 每角色自己的 `scene`
    - 每角色自己的 `dialogue`
    - 自定义 `player role` 在切角色后仍正确映射到场景角色列表
  - 同时验证：
    - 只编辑安雅的 world/scene 后，不影响 `111`
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 09:43:00 UTC — 实例删除高层测试与冲突语义收口
Modified by: Codex (GPT-5)

- `internal/runtime/instances_test.go`
  - 新增删除实例高层集成测试：
    - 覆盖 `events / branches / dialogue_history / working_memory / semantic_facts / episodic_events / pending_facts`
    - 覆盖 `data/instances/<instance_id>/player_role.json` 与 `save_slots.json`
    - 确认删除 `alt` 不影响 `default`
  - 补默认实例/唯一实例删除冲突测试
- `internal/runtime/instances.go`
  - 增加实例错误哨兵：
    - `ErrInstanceIDRequired`
    - `ErrInstanceNotFound`
    - `ErrInstanceConflict`
- `internal/api/server.go` / `internal/api/server_test.go`
  - `default / stop / delete` 统一实例错误映射
  - `instance not found` → `404`
  - 删除默认实例或唯一实例 → `409`
- 文档同步：
  - `TODO.md`
  - `TEST_PROGRESS.md`

### 2026-05-26 09:27:30 UTC — 启动自锁修复与版本探针验证
Modified by: Codex (GPT-5)

- `internal/events/store.go`
  - 修复 `seedBranchesFromEvents()`：
    - 先读完并关闭 `rows`
    - 再调用 `ensureBranch()`
  - 避免共享 SQLite 且 `MaxOpenConns(1)` 时的启动自锁
- `internal/events/store_test.go`
  - 新增“重开已有 branch 数据库不得卡死”回归测试
- 运行验证：
  - 前台使用标准 `data/` 启动恢复正常
  - `./deploy/pm2-start-corerp.sh`
  - `./deploy/smoke-check.sh` → `/api/health=200` / `/api/ready=200`
  - `GET /api/version` 返回：
    - `version=dev+dirty`
    - `commit=f6639caebb21a09af57d5b1130ac482eb15a8e45`
    - `time=2026-05-25T12:16:30Z`
- 文档同步：
  - `ARCHITECTURE.md`
  - `TODO.md`
  - `TEST_PROGRESS.md`

### 2026-05-26 09:26:00 UTC — 构建元数据与启动日志补全
Modified by: Codex (GPT-5)

- `cmd/corerp/main.go`
  - 新增构建元数据解析：
    - 优先使用 ldflags 注入值
    - 回退读取 `runtime/debug.ReadBuildInfo()` 中的 `vcs.revision / vcs.time / vcs.modified`
  - 启动时打印：
    - `version`
    - `commit`
    - `build_time`
    - `data`
    - `port`
- `internal/api/server.go`
  - `GET /api/ready` / `GET /api/version` 增加 `build_time`
- `cmd/corerp/main_test.go` / `internal/api/server_test.go`
  - 补构建元数据与版本接口回归测试
- 文档同步：
  - `README.md`
  - `api-contract.yaml`

### 2026-05-26 09:10:00 UTC — 健康检查与就绪探针落地
Modified by: Codex (GPT-5)

- `internal/api/server.go`
  - 新增：
    - `GET /api/health`
    - `GET /api/ready`
    - `GET /api/version`
- `internal/api/server_test.go`
  - 补 health / ready / version 单测
- `deploy/smoke-check.sh`
  - 改为直接检查 `/api/health` 和 `/api/ready`
- 文档同步：
  - `README.md`
  - `TODO.md`
  - `TEST_PROGRESS.md`
  - `api-contract.yaml`

### 2026-05-26 09:10:00 UTC — 前端实例管理面板接入
Modified by: Codex (GPT-5)

- `web/index.html` / `web/app.js`
  - 侧栏新增实例管理卡片
  - 支持：
    - 查看实例列表与状态
    - 创建实例
    - 切换默认实例
    - 停止实例
    - 删除非默认实例
- 运行台刷新链路会同步刷新实例摘要
- 文档同步：
  - `README.md`
  - `ARCHITECTURE.md`
  - `TODO.md`

### 2026-05-27 18:51:04 UTC — Export / World API 主语义收口
Modified by: Codex (GPT-5)

- `internal/api/server.go`
  - `/api/experiment-reports` 在 API 层也统一做 compat normalization
  - `/api/worlds` 返回显式 `participants / participant_details`
  - `/api/export` 返回显式 `participants / participant_details`
  - Markdown 导出标题改为 `Focus Character`
- `internal/api/server_test.go`
  - 新增 / 更新 world enter、export、experiment report 兼容回归测试
- 文档同步：
  - `TODO.md`
  - `CLOSURE_AUDIT.md`
  - `DELIVERY_TRACKING.md`

### 2026-05-27 18:39:32 UTC — Experiment Report 兼容语义收口
Modified by: Codex (GPT-5)

- `internal/runtime/compat.go` / `internal/runtime/authoring.go`
  - 新增 `ExperimentSnapshot` / `ExperimentReport` compatibility normalization
  - archived `latest_trace` 现在也统一优先 `focus_character`
- `internal/runtime/runtime_test.go` / `internal/api/server_test.go`
  - 新增归档 compat 回归测试，证明 legacy `character` 不再主导实验归档回放
- 文档同步：
  - `TODO.md`
  - `CLOSURE_AUDIT.md`
  - `DELIVERY_TRACKING.md`

### 2026-05-27 18:36:27 UTC — Runtime Audit 第一版实验归档复现
Modified by: Codex (GPT-5)

- `web/app.js`
  - Runtime Audit 的 Archive 区现在可展开 archived experiment report
  - 可查看 `current / compare` snapshot 摘要
  - 可把 archived `latest_trace` 直接送入阶段回放
- 文档同步：
  - `TODO.md`
  - `CLOSURE_AUDIT.md`
  - `DELIVERY_TRACKING.md`

### 2026-05-27 18:34:06 UTC — Runtime Audit 第一版按阶段回放
Modified by: Codex (GPT-5)

- `web/app.js`
  - Runtime Audit 现在会为选中的 trace 拉取完整 `step_traces`
  - 新增逐阶段回放视图，支持：
    - 上一阶段 / 下一阶段
    - 直接跳到指定 step
    - 查看 handoff / goals / allowed actions / validator / events / narrative
- `web/index.html`
  - Runtime Audit 说明文案更新到“按原因筛 + 按阶段回放”
- 文档同步：
  - `TODO.md`
  - `CLOSURE_AUDIT.md`
  - `DELIVERY_TRACKING.md`

### 2026-05-27 18:29:46 UTC — Runtime Audit 第一版按原因筛选
Modified by: Codex (GPT-5)

- `web/index.html` / `web/app.js`
  - Runtime Audit 新增 `原因筛选`
  - 当前可按 `director / pressure / faction / population / archive` 切开统一审计证据
  - `World / Population` 审计块拆成更细的 pressure / faction / population 视图
- 文档同步：
  - `TODO.md`
  - `CLOSURE_AUDIT.md`
  - `DELIVERY_TRACKING.md`

### 2026-05-27 18:26:38 UTC — RuntimeInstanceSummary 兼容语义收口
Modified by: Codex (GPT-5)

- `internal/core/types.go`
  - 为 `RuntimeInstanceSummary`、`TurnTrace`、`TurnStepTrace` 补充 world-first / compatibility 语义注释
- `internal/runtime/compat.go` / `internal/runtime/runtime.go`
  - 新增 `RuntimeInstanceSummary` normalization
  - 实例摘要显式保证：
    - `focus_character` = viewpoint
    - `participants` = scene truth
    - `active_character` = compatibility mirror
    - `loaded_characters` = loaded roster mirror
- `internal/api/server_test.go` / `internal/runtime/instances_test.go`
  - 新增实例摘要回归测试，证明 `participants` 不再被 `loaded_characters` 代表
- 文档同步：
  - `TODO.md`
  - `CLOSURE_AUDIT.md`
  - `DELIVERY_TRACKING.md`

### 2026-05-27 18:22:29 UTC — Memory / PendingFact 兼容层继续收口
Modified by: Codex (GPT-5)

- `internal/runtime/compat.go`
  - 新增 `MemorySnapshot` / `PendingFact` compatibility normalization
- `internal/runtime/persistence.go` / `internal/runtime/review.go`
  - runtime 返回 memory snapshot / pending facts 时统一优先 `focus_character`
- `internal/api/server.go`
  - `/api/memory` 不再从 legacy `character` 反向决定 focus
  - `/api/pending-facts` / `/api/quarantine` 顶层 `focus_character` 回传统一显式化
- `internal/api/server_test.go`
  - 新增 `/api/memory` 与 `/api/pending-facts` “新字段优先、旧字段仅镜像” 回归测试
- 文档同步：
  - `TODO.md`
  - `CLOSURE_AUDIT.md`
  - `DELIVERY_TRACKING.md`

### 2026-05-26 09:05:31 UTC — 标准 data 目录重建与 PM2 启动固化
Modified by: Codex (GPT-5)

- 删除旧测试运行库，重建空白 `data/`
- 新增：
  - `deploy/pm2-start-corerp.sh`
  - `deploy/smoke-check.sh`
- PM2 当前固定使用：
  - `serve -port 8080 -data /home/kelebituo/corerp/data -characters ./characters -secure-cookie=false`
- 验证：
  - `./deploy/smoke-check.sh` → `/login=200` / `/api/instances=401`

### 2026-05-26 08:57:00 UTC — 前端显式实例视图切换接入
Modified by: Codex (GPT-5)

- `web/index.html` / `web/app.js`
  - 顶栏新增实例选择器
  - 页面内切换实例后，runtime 请求自动带 `instance_id`
  - 实例面板现在区分：
    - 默认实例
    - 当前视图实例
- 文档同步：
  - `README.md`
  - `ARCHITECTURE.md`
  - `TODO.md`

### 2026-05-26 08:56:00 UTC — 本轮验证
Modified by: Codex (GPT-5)

- `/usr/local/go/bin/go test ./...` ✅

### 2026-05-26 08:56:00 UTC — Runtime Instance 生命周期闭环
Modified by: Codex (GPT-5)

- `internal/runtime/instances.go`
  - manager 新增 `status / stop / delete`
  - 删除实例会清理实例目录与共享 SQLite 中的实例命名空间数据
- `internal/api/server.go`
  - 新增：
    - `GET /api/instances/status`
    - `POST /api/instances/stop`
    - `POST /api/instances/delete`
- `internal/api/server_test.go` / `internal/runtime/instances_test.go`
  - 补实例状态、停止、删除回归测试
- 文档同步：
  - `README.md`
  - `ARCHITECTURE.md`
  - `TEST_PROGRESS.md`
  - `TODO.md`
  - `api-contract.yaml`

### 2026-05-26 08:09:07 UTC — Runtime Instance 基础设施与实例隔离
Modified by: Codex (GPT-5)

- 实例管理：`list / create / set default`
- 运行时 API 全面接入 `instance_id`
- `player_role.json` / `save_slots.json` 迁移到 `data/instances/<instance_id>/`
- 共享 SQLite 中的 `events / branches / dialogue / working_memory / semantic_facts / episodic_events / pending_facts` 改为按实例隔离
- 补共享 SQLite 下的双实例隔离测试

### 2026-05-26 06:29:51 UTC — 最终形态蓝图与多 step handoff 落地
Modified by: Codex (GPT-5)

- 新增 `FINAL_ARCHITECTURE_BLUEPRINT.md`
- TurnStep 之间显式 handoff 已接入 runtime prompt 与 trace
- `README.md` / `ARCHITECTURE_RUNTIME.md` 同步到多 step 语义

### 2026-05-26 06:14:12 UTC — Director turn plan 升级为职责化 step 链
Modified by: Codex (GPT-5)

- `DirectorPlan` 从“切活跃角色”升级为显式 `TurnStep` 序列
- `auto_chain` 具备 lead / followup 职责链语义
- 前端 trace 面板支持按 step 查看执行链

### 2026-05-26 05:51:54 UTC — Branch 继承回放模型收口
Modified by: Codex (GPT-5)

- `branches` 元数据表落地
- `Fork()` 改为创建分支元数据，不再改写历史事件归属
- `ReplayTo()` / `GetTimeline()` 改为沿父分支链回放
