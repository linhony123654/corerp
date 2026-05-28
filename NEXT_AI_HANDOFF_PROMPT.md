# Next AI Handoff Prompt

你接手的是 `/home/kelebituo/corerp`。项目当前已经按 `ACCEPTANCE_CHECKLIST.md` 完成 world-first persistent narrative runtime 闭环验收；后续工作以扩大样本池、增强 Runtime Audit 体验和维护回归证据为主。

## 沟通方式

- 用户希望你直接推进，不要长篇空规划。
- 先检查当前状态再行动，不要相信旧对话记忆。
- 不要把后续增强项误写成闭环阻塞项；当前闭环完成判断以最新 11/11 proof audit 和已更新文档为准。
- 任何文档结论必须和当前测试、脚本、运行态证据一致。
- 代码改动后必须跑对应验证；不要每个小改动都跑全量 8 小时级测试。

## 当前已完成的重要进展

- `focus_character` 是观察视角，`participants` 是 scene truth，`participant_details` 是 switch / director / trace / UI 共用模型。
- 主 API / 前端路径已经大幅移除 legacy `character / active_character / loaded_characters` 回退。
- background NPC 已进入 population runtime，可 attention -> promotion -> director candidate，也支持长期脱离后的 demotion。
- autonomous tick 会按 scene/location/faction/pressure 拉入 background NPC，不依赖 directTurn。
- world structure 已能驱动 planner / scheduler / tick / director / diagnostics。
- Runtime Audit / Experiment Reports 已形成第一版作者审计和 replay 工作流。
- `/api/proof-audits` 已上线，之前用户报过 404，现在运行态未登录访问应为 `401 unauthorized`，不是 404。
- 真实 runtime replay round-trip 已补强：`/api/experiment-reports/replay-batch` 派生 replay branches 时会复制源实例 archived checkpoint 到 replay 实例，再加载并继续推进。

## 最新强证据

最新完整 proof audit 归档：

```text
data/proof-audits/20260528T084433Z/SUMMARY.md
Overall: PASS
11/11 gates PASS
```

当前 `scripts/run_world_proof_audit.sh` 的 11 个 gate 已完整落盘并通过：

```text
data/proof-audits/20260528T084433Z/SUMMARY.md
Overall: PASS
11/11 gates PASS
```

当前脚本的 11 个 gate：

- `api-world-first-contract`
- `api-author-replay-contract`
- `api-proof-archive-contract`
- `events-npc-scheduler-canonical-contract`
- `runtime-population-lifecycle-contract`
- `runtime-200-sample-matrix`
- `runtime-200-real-world-matrix`
- `api-200-sample-matrix`
- `api-200-real-world-matrix`
- `runtime-500-real-world-stability`
- `api-500-real-world-stability`

其中 `api-author-replay-contract` 已包含：

```text
TestExperimentReportReplayBatchRealRuntimeRoundTrip
TestAuthorWorldLevelInterventionReplayControlsRuntimeWithoutCharacterConfig
TestAuthorWorldLevelInterventionReplayMatrixAcrossWorldFamilies
```

这个测试用真实 `runtime.Manager` 与真实 runtime engine 验证 report -> replay-batch -> replay-advance，不是 mock-only。
新增 world-level authoring replay 测试还证明：作者只靠 `/api/population`、`/api/world-structure`、tick、checkpoint/report/replay 就能制造并复现 runtime 分叉，且 focus definition 不被修改；矩阵测试已扩到外城治安与港口物流两个 world family，并批量推进 replay branches 复核 audit evidence。

新增 runtime/events 证据：

```text
TestGatekeeperTreatsNPCSchedulerAsCanonicalTickEvent
TestIdentityShiftShapesLongWindowWorldOutcome
TestIdentityShiftShapesWorldOutcomeAcrossWorldFamilies
```

这证明 `npc_scheduler:*` 自治行动会进入 canonical world projection，且 promoted persona 的 adaptive 慢变量能改变后续多 tick scheduler actions、tension 与 `trajectory_summary`；矩阵测试已覆盖外城冲突与港口调度两个 world family。

## 最近验证过的命令

```bash
git diff --check
bash -n scripts/run_world_proof_audit.sh
/usr/local/go/bin/go test -count=1 ./internal/api -run '^(TestExperimentReportReplayCreatesReplayBranches|TestExperimentReportReplayBatchFiltersByWorld|TestExperimentReportReplayBatchRealRuntimeRoundTrip|TestAuthorWorldLevelInterventionReplayControlsRuntimeWithoutCharacterConfig|TestAuthorWorldLevelInterventionReplayMatrixAcrossWorldFamilies|TestExperimentReportReplayAdvanceTicksReplayBranches|TestRuntimeAuditAggregatesAuthoringEvidence|TestProofAuditsRouteListsLatestAuditArtifacts)$'
/usr/local/go/bin/go test -count=1 ./internal/events ./internal/agents ./internal/runtime -run '^(TestGatekeeperTreatsNPCSchedulerAsCanonicalTickEvent|TestSelectAdaptiveBestStep|TestSchedulerTickFollowsAdaptiveShift|TestReconcilePopulationPromotesBackgroundNPC|TestReconcilePopulationDemotesStalePromotedNPC|TestAutonomousSimulationPromotesScenePopulationAcrossLongWindow|TestPopulationInsightsIncludesPromotionReason|TestIdentityShiftShapesLongWindowWorldOutcome|TestIdentityShiftShapesWorldOutcomeAcrossWorldFamilies)$'
./scripts/run_world_proof_audit.sh
/usr/local/go/bin/go build -o /home/kelebituo/corerp/corerp ./cmd/corerp
node --check web/app.js
pm2 restart corerp && pm2 save
```

Runtime smoke：

```text
GET /api/health -> 200
GET /api/proof-audits?limit=1 -> 401 unauthorized
GET /api/runtime-audit?... -> 401 unauthorized
```

`401` 是认证保护正常；如果是 `404` 才是路由问题。

## 当前闭环结论

`ACCEPTANCE_CHECKLIST.md`、`CLOSURE_AUDIT.md`、`DELIVERY_TRACKING.md` 已按最新 11/11 proof audit 更新为“已验收”。当前可以把项目判断为终态闭环已完成。

后续增强项：

- 更大规模 world-family replay 样本，尤其是用户自建世界。
- 更多作者自建世界或真实导入世界的长期 replay 归档、派生、推进、复核。
- Runtime Audit 调试器体验继续深化。
- 人格慢变量长期塑造 world outcome 的真实导入世界 / 用户自建世界矩阵继续扩充。

## 下一步推荐路线

1. 先读：
   - `ACCEPTANCE_CHECKLIST.md`
   - `CLOSURE_AUDIT.md`
   - `DELIVERY_TRACKING.md`
   - `TODO.md`
   - `data/proof-audits/20260528T084433Z/SUMMARY.md`
2. 不要继续清理角色卡，主方向已经是 world-first，角色卡不是核心。
3. 优先补“更大规模真实 world-family replay 样本池”：
   - 用现有实验归档 / replay-batch / replay-advance 工作流扩展样本。
   - 最好新增一个可重复脚本或测试 gate，而不是只手动点 UI。
   - 目标是证明作者侧可以稳定从 archive 恢复、派生、推进、比较多个 world family。
4. 如果改 replay / runtime audit / proof archive，必须更新 `scripts/run_world_proof_audit.sh` 或新增独立 verifier。
5. 每次阶段推进后更新：
   - `TODO.md`
   - `CLOSURE_AUDIT.md`
   - `DELIVERY_TRACKING.md`
   - `SESSION_LOG.md`
6. 重要修改后继续跑 `./scripts/run_world_proof_audit.sh`，避免闭环回归。

## 常用命令

```bash
cd /home/kelebituo/corerp
git status --short
git diff --check
node --check web/app.js
/usr/local/go/bin/go build -o /home/kelebituo/corerp/corerp ./cmd/corerp
/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime
./scripts/run_world_proof_audit.sh
pm2 restart corerp && pm2 save
```

服务启动慢，重启后等 70-95 秒再查：

```bash
curl -s -i http://127.0.0.1:8080/api/health
curl -s -i http://127.0.0.1:8080/api/proof-audits?limit=1
curl -s -i 'http://127.0.0.1:8080/api/runtime-audit?trace_limit=1&checkpoint_limit=1&report_limit=1'
```

## 重要约束

- 不要用 `git reset --hard`。
- 不要回退用户或其他 AI 的改动。
- Go 工具用 `/usr/local/go/bin/go` 和 `/usr/local/go/bin/gofmt`。
- 只在里程碑需要时跑完整 proof audit；加入 500 tick gates 后可能需要 20 分钟级。
- 500 tick tests 默认会 skip，必须通过 `scripts/run_world_proof_audit.sh` 或设置 `CORERP_RUN_SLOW_PROOF_TESTS=1` 才运行。
- 终态判断只按 `ACCEPTANCE_CHECKLIST.md`，不是按 TODO 打勾数量。
