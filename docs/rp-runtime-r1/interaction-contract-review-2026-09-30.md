<!-- historical-checkpoint -->
历史 edf6750b 检查点。当前结果为 [conversation-default-review](conversation-default-review-2026-10-01.md) 和 [conversation-default-verification](conversation-default-verification-2026-10-01.json)。以下未commit/未运行等描述只适用于当时。
<!-- /historical-checkpoint -->

# R1 当前结果：接口契约修复已落地，体验出口未通过

当前 runtime `edf6750b`，分支 `rp-preview-integration-20260929`，HEAD `7fc3b3ba7be229b11341130d6cdd4275e231274f`。唯一当前源码与验证状态为 [interaction-contract-verification](interaction-contract-verification-2026-09-30.json)。所有本轮任务已结束，R1 NOT DONE；没有部署本轮 backend，没有 commit/push，保留既有脏工作树。

本轮没有发现必须推倒事件账本或 typed owner 的依据。实际发现的是 RP 数据就绪、模型接口契约与行为节奏的缺口。此前把工程回归当作主要推进单位，短对话和公开读样进入太晚；代码增量不能代表玩家体验已经改善。路线已改为一段真实会话 → 针对具体失败修改已有运行层 → 世界回归 → Golden/真人验收。现在保留原内核，把下一步收敛到现有发言者选择、动态目标与设定装配。

1. **Golden baseline 的结构性失败。** 冻结的真实荣庆堂有 11 个角色缺少人设、关系和称呼，旧流程仍允许模型回应。当前明确 NOT READY，模型不再补原著 canon。另一个可复核故障来自技术长样：d329 的 32 个主回合、96 次 NPC 决策全部成功，但后置“推杯子并说话”被物品字段校验拒绝。数据库完整性检查 `ok`；一次解释调用失败、没有接受计划或混合效果。API 将这类模型解释失败报成 `STORAGE_FAILURE`，不代表这次已证明数据库损坏。`provider_proposal_object_fields` 不能进一步确定是哪一个字段缺失或冲突，因为没有保存原模型提案。
2. **Context Contract。** NPC 仍以原 `BuildRPDecisionInput` 为唯一事实入口，保留获准人设、关系、称呼、听见的对白、知识、记忆、版本、来源和 readiness；provider-only 短引用还原为真实事件后校验。已有玩家输入解释器仍只接收授权场景和物品候选，不读取 NPC private。现在它的同一份正式 `proposal_schema` 同时进入模型消息和传输声明，和 NPC 接口采用同一原则；不是第二套世界状态或事实来源。原 Gemini staged 分支保持原行为。
3. **Decision Contract。** NPC 维持完整 V3 private + 单一 observable。玩家解释器维持闭合 DIALOGUE/ACTION/MIXED/CONTINUE/CLARIFICATION 与原 typed steps，动作后的原话由服务端绑定。只调整契约呈现及安全诊断；没有删除不合法字段来救提案，没有扩大动作集合，也没有提高重试数或 deadline。
4. **Observable / Private 边界。** provider-view/canonical、来源、owner、事务提交和目击链保持。当前真实短样四个神态各有 committed 事件及玩家 `co_location` 目击。混合行为复测核对了真实杯子锚点、原话仅一次、精确重放不增加事件/解释请求、重启保持位置和叙事选择。NPC 自述是获准发言，不能仅凭台词证明日程、玩家站位或物品存在；元数据 private 不进入 Narrator/公开观测。
5. **Narrator 能力和限制。** 先前公开风格、措辞、节奏和已提交 observable 的渲染保留，本增量没有扩大事实权限。新物品、角色、未提交动作、位置变化与 private 泄漏仍禁止。实际读样仍多是对白加神态；不声称文学表现力验收通过。
6. **Consistency Gate。** 闭合结构、来源、可确定的关系/称呼冲突、权限、版本、typed owner 和公开投影是硬规则；重复、自然度、人格味、主动性是辅助诊断/真人评价。获准来源引用并不证明全部对白语义正确。新增解释失败类别和最多四个 step 的字段存在布尔诊断；不记录原话、ID 值、未知字段名、提案或隐藏推理。隐私日志测试通过。
7. **技术回归。** 当前 edf：decision 完整包 0.535s PASS、interaction 存储定向 15.357s PASS、decision vet/build PASS；HTTP 首跑初始化 SQLite 失败，保留日志，同源串行完整复验 43.801s PASS，首次原因未确认。上一 d329 的完整后端有同源 12 测试包全套覆盖/1 无测试包，不能称作当前新代码单命令全套通过。d329 原 32 轮主段为 96 成功、100 次尝试、0 决策回退、32 次 Play 真 Prose；重生成/选择检查通过，整套 runner 因后置物品解释失败 exit1，后续继续/故障/重启未到。当前 edf 在该冻结数据库的副本上做真实 API 定向续验：2 次解释和 6 次 NPC 请求均首试成功，混合动作+原话+精确重放、继续无台词、重启事实/物品/已选叙事全部 PASS。原失败没有改成 PASS；尚未完成当前 edf 的整套浏览器 32 轮复跑，故技术完整出口仍未签收。
8. **Golden 前后与人工依据。** 四个冻结文件原 hash 保持。d329 冻结复评采集完成：17 个 decision 为 `not_used / rp_context_not_ready`，0 NPC HTTP；G8 真 Prose 一次，但没有角色对话。这是缺数据防护改善，RP Experience NOT PASSED。Nora/Lin 同设定真实四轮可复核熟悉称呼、情绪承接与回忆承诺，仍重复且仅为测试作者世界。当前技术长样第 31 轮能追忆首轮原话；第 1/8/16/17/23/31/32 轮公开文本也暴露任务复读、三人逐个答话及机械神态。见[公开 32 轮](source-refs-32-public-2026-09-30.json)和[短样辅助读样](source-refs-short-review-2026-09-30.json)。这是助手读样，不是真人签收，也不是冻结荣庆堂的角色体验改善证据。
9. **剩余问题与下一步。** 原 32 整套终态 FAIL、edf 仅后置 API 定向通过、SQLite 初始化失败原因未确认；长期语义记忆、完整意图生命周期和全面叙事语义验证未完成。真实 canon 与真人体验评价问题已问过未答，不重复询问或推定批准。下一开发单位是一名主要交流者的 6–10 轮：先验证已有 orchestrated/会话延续配置，避免所有听者每轮抢答；把固定 persona 里的活动和当前已提交状态区分开，观察目标是否随互动变化，保持来源和 private 边界。作者明确提交真实人设/关系/称呼后，以另标 authored 配置复评同样 Golden，原 baseline 保留。先得到公开可读、想继续聊的样本，再冻结候选跑一次最终整套；缺输入期间不再重复无变化全套或加单句 prompt。两个出口真实通过后停止，不扩 R2/R3。

当前已实现、已验证和发布状态严格分开。预览 PID722907 的 backend hash 仍是 `fea2067a`；本轮 edf 后端没有出现在预览服务中，前端实际服务路径未由该探针证明。

证据：[原 32 失败](source-refs-32-result-2026-09-30.json)、[当前真实后置复验](interaction-contract-postchecks-2026-09-30.json)、[当前源码/任务](interaction-contract-verification-2026-09-30.json)。初次 API probe 使用错误测试字段 `mode`，在请求边界即拒绝、0 新模型调用；修正为既有 `interaction_mode` 后通过，初始记录另存，不属于产品契约修复。
