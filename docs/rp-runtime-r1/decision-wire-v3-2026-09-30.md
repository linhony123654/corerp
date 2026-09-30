# NPC 决策 wire v3 · 2026-09-30

本增量针对真实 Step 回归中 `noop_contains_effects`、非法台词字段及额外修复消耗总时限的问题。旧请求要求各种动作同时携带台词、目的地、活动等字段；合法组合仍靠提交前校验兜底。新请求在现有 adapter 内明确区分私有草案和单一 observable，不新增世界状态或 owner。

## 契约

模型请求 envelope/schema 名称为 `corerp.decision.v3` / `corerp_decision_v3`；内部 `RPDecisionInput` 及 context version 不变。

| observable 动作 | 必需字段 |
| --- | --- |
| respond / refuse | action、text、introduce_self、expression_code |
| silence / wait | action、expression_code |
| leave | action、destination_place_id |
| act | action、activity_code |

顶层只有 `private` 和 `observable`。private 保留 intent、emotion、relationship_stance 和获准来源 ID；private 仍不直接成为公开事实。schema 只提供当前合法动作及已来源化的目的地、活动、引用域；没有活动/目的地时不提供不可执行分支。beckon 需要当前对象实际可见。表达 `none` 保留既有 wire 到内部空表达的约定。

本地解析拒绝重复、未知、缺失、null、混用新旧根字段和动作不适用字段，不静默删除越界 effect。格式合法但来源、目的地或活动不合法的 proposal 仍由原 core / typed owner 拒绝。非法字段只有既有一次共享修复机会，不增加预算或超时。严格旧格式 decoder 保留以兼容旧调用；新模型请求始终要求 v3，不将不合法 v3 降级。

## 验证与限制

修改前的新协议反例为 RED，修复后完整 decision 包通过；曾有一个旧 schema 名称断言失败，更新后通过。存储集成用实际 ChatProvider、来源引用、speech+beckon 原子提交、故障后恢复与私有输出隔离验证整条链，而非只验证 JSON 字段。

[Step 官方 JSON Mode 文档](https://platform.stepfun.com/docs/guide/json_mode)要求提供格式说明、解析与校验，并提示截断可能留下不完整 JSON。该文档不足以证明 `step-5-preview` 或当前中转支持 native strict schema / anyOf；需要实际模型小样验证。任何小样只证明协议可用，不证明长对话体验或完整32轮通过。

实际 adapter 集成暴露额外结构性问题：speech + expression 已在回合叙事中出现，目击历史 UNION 又将同一 expression 单列。现在只有存在同一观察者、同一原子 batch / causation、同一 NPC 决策和已 settled 回合时才归并；未完成回合仍保留独立目击，其他观察者和独立非语言动作也保留。没有删除事实或改写 Event。故障恢复、第二客户端只读历史、重建以及既有非语言 scoped read 测试通过。

最终验收：新版完整 decision 包、存储定向集成/表达/目击测试、相关五包 vet 和 diff 检查 PASS；真实 Step 两个合成协议小样各一次 v3 响应成功。`go test ./... -count=1 -timeout=20m` 最终退出0，12个有测试的包通过（storage889.953s、HTTP69.189s），另一个包无测试。初次全包测试因磁盘空间不足失败；清理可重建Go编译缓存后重跑通过，初次失败日志保留，没有修改源码迎合环境失败。

同一 runtime `2e527072a0aafea8fd9e72c545d7f847f010d8fd5937cae407298b6f87085ff1` 的实际32轮退出1（第2轮，一次截断后120s timeout；5次决策成功/1次超时；后置检查未执行）。冻结Golden采集完成：17/17各一次成功、G8真实Prose一次成功、0 expression，仍有陌生人/盘问/缺角色关系/稀疏叙事；并有无设定依据的“廊下有茶”台词风险。见 [Golden复评](decision-wire-golden-review-2026-09-30.md)、[最终验证](decision-wire-verification-2026-09-30.json)。采集成功不等于体验通过。

所有任务进程已退出。本增量未提交、推送或部署，原冻结世界/包及before/after未改变。原 f9c6769f 报告保留为历史证据。World Integrity 的实际32轮出口、作者 canon 和真人 RP 复评均未完成，R1 NOT DONE。

## 代码确认的剩余架构缺口

1. `readRPOwnDecisionContext` 分别取16条近期对话、40条玩家旧话（每条最多350字符）、20条非台词知识、3条本人私有决策；另有最多512候选中选4组/6000字符的词法相关对话。相关对话已排除近期窗口，但全部字段仍没有共同优先级、总预算及统一去重。adapter 的128KiB是拒绝上限，不能替代上下文选择策略。下一步应在原 builder 内统一这些策略，保留重要人设、关系、当前输入、出处和权限，不新增平行链。
2. `ensureRPTurnActivationPlan` 在可编排模式下按当前直接点名优先，再按稳定角色ID排序；未点名时没有延续上一位实际交流对象的排序因素。改善对话焦点应依据玩家可知的既有公开交流，并继续固定激活计划供重试恢复使用，不能读其他 NPC private。
3. 冻结真实11角色配置仍没有 persona/关系/称呼声明。UNKNOWN/MISSING 必须继续明确，不能靠原著补齐；完成关系型体验验收需要作者明确设定。协议改进不能替代这些数据。
4. 当前真实 full32 的第2轮出现120s timeout，5次决策成功/1次超时，后置检查未执行。日志为截断后第二次请求超时；没有足够证据把耗时直接归因于上下文、schema或中转容量。需要在不记录私有正文/凭证的前提下对请求大小、预算、实际耗时做诊断，不重复盲跑32轮。记录见 [本次32轮失败](decision-wire-32-result-2026-09-30.json)。

这些是后续 R1 内的结构性工作；本增量没有声称已获得长对话体验或已解决上述缺口。
