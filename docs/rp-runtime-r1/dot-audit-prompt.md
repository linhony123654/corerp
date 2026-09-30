# 给 dot 的 CoreRP 审计提示词

请以 GitHub 分支 `rp-preview-integration-20260929` 的最新提交为准，审计 CoreRP 的 RP Runtime R1。仓库：`https://github.com/linhony123654/corerp`。审计比较基点为 `7fc3b3ba7be229b11341130d6cdd4275e231274f`；这是现有工作树的审计快照，含此前 Play、typed object、恢复等改动，不应把整个 diff 都归为最后一次修复。

我关心的是：现在为什么仍没有达到酒馆式连续角色扮演的体验，底层哪些地方需要结构性改动，哪些只是配置、模型接口或缺少作者设定。请独立查代码和真实公开样本，不以报告里的 PASS、字段数量或测试数量代替判断。

先读 `AGENTS.md`，然后按这个入口顺序读：

1. `docs/rp-runtime-r1/scene-activity-context-2026-10-01.md` 和 `scene-activity-context-verification-2026-10-01.json`：主审计快照 `ab4580d` 之后的活动可见性/事件来源修复、798源码hash、当前原32任务的准确状态。其下的 `conversation-default-review-2026-10-01.md` 与 `conversation-default-verification-2026-10-01.json` 属于主快照历史证据。另读 `same-facts-prose-review-2026-10-01.md`：同事实现有style对照，79→96字仍机械，首次采集envelope错误保留，不能冒充架构/人工体验改善。
2. `conversation-default-live-2026-09-30.json` 与 `conversation-default-after-2026-09-30.json`：同一明确测试作者世界、Studio 实际生成的包、同一八轮输入、同一 Step 模型配置的真实公开样本。前者保留原失败，后者是修复后的复评；不是冻结荣庆堂或真人验收。
3. `source-refs-32-result-2026-09-30.json`、`source-refs-32-public-2026-09-30.json` 和 `interaction-contract-postchecks-2026-09-30.json`：原完整 runner 在后置解释失败退出，主32轮成功不能冒充全套成功；后置 API 续验也不能替代当前完整浏览器32复跑。
4. `golden-baseline.md`、`real-rongqing-world-spec.json`、`real-rongqing-packages.json`、`before-samples.json`、`after-samples.json`、`source-refs-golden-samples-2026-09-30.json`：冻结配置与历史样本。真实11个NPC缺 persona、关系和称呼；最近复评17次 NOT READY、零NPC模型调用。禁止从《红楼梦》常识替作者补 canon。

重点沿以下实际调用链检查，给出文件、函数和可复核的触发条件：

- 原 `BuildRPDecisionInput`、`rp_decision_provider_view.go`、`rp_context_selection.go`：关系与称呼、实际听见的对话、相关旧对话、角色自己的 private memory、当前活动（尤其新增 `rp_scene_activity_context.go`：当前可见与自己的已提交历史、未目击开始时间/结束结果、source_event_id/observation_basis、head一致性）、版本和来源，是否真的影响决策；有没有丢失必要信息、跨角色/visibility 越界或构造平行事实源。
- `decision/chat.go`、`decision_wire.go`、`decision_sources.go`、`interaction.go`：闭合 V3 契约、private/observable 分层、短引用还原、同一 schema 在消息和传输中的呈现、工具/完整JSON兼容、修复次数和总deadline。现有诊断是否泄漏私有内容，失败是否被错误包装成存储问题。
- `rp_decision_commit.go`、`rp_initiative_commit.go`、`rp_private_decision.go`、nonverbal/owner/replay 路径：提案不能自动成为公开事实，private 不能通过 Event/Narrator/其他角色上下文外泄；重试、并发、重启和重建是否保持事实与来源。
- `rp_turn_activation.go`、`rp_conversation_focus.go`：所有听者的 hearing 与真正激活回应分开；点名、真实交流、稳定回退的优先级；等待只能延续原来已听见的来源，不能凭沉默建立关系。换人、换场、换章、过期、外部控制和未听见的对白是否正确截断。检查 UNION 来源查询的新分支及其反例。
- `src/lib/studioCreate.ts`：新生成系统包明确 `orchestrated` / responder limit 1 / version 1.1.0，内容hash包含策略；旧包、导入包和原32测试的 legacy 行为保留。是否真正解决默认群体抢答，是否引入迁移或兼容问题。
- `narrative/prose.go`、`fact_guard.go`、`agency.go`、`rp_narrative_presentation.go`：公开投影和允许的表达能力。正则事实检查究竟能保证什么，是否过度阻塞正常文学表达，是否仍允许新的可交互事实、动作目标变化、private泄漏或已提交事实改写。

还要专门判断以下未解决的体验问题：静态 persona 中的任务是否压过真实当前活动；private 意图是否只是每轮生成的标签而缺少连续性；“给你倒杯热的”之后说“热的还温着”是否把台词变成未经提交的物品事实；沉默/等待与机械笑、点头是否破坏接话；已有相关对话检索是否足以支撑长对话。请区分角色的承诺/说法与事情真的发生，不要求模型永远说真话，也不要把所有台词里的动作自动执行。

输出要求：

- 先给按严重性排列的具体 findings，每项包含路径/行号、触发例、玩家或世界影响、现有测试是否覆盖和最小可行改法。
- 区分已证实的问题、代码推断和需要真实运行才可确认的事项。没有运行的检查请写未验证。不要把报告里的历史运行当作你自己跑过。
- 说明究竟需要改变哪个抽象/状态流；最多推荐下一步1–3个改动，不给一大串后续里程碑或角色特例 Prompt。
- 判断两个出口各自缺什么。当前 R1 **NOT DONE**：完整当前32轮、作者明确 canon、可比较 Golden 与真人 RP 体验评价仍未签收；八轮测试不等于长对话或叙事体验已达标。
- 本轮先审计，不合并、不部署、不发消息；不给自动“好玩”评分器设计。目标是角色在可信且可追认的世界中连续演戏，保留账本/typed owner/恢复/private边界。
