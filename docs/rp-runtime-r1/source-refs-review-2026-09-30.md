<!-- historical-checkpoint -->
本文下方为历史检查点。当前 runtime 为 `edf6750b`，任务均已终态；最新结果以 [interaction-contract-verification](interaction-contract-verification-2026-09-30.json) 与 [interaction-contract-review](interaction-contract-review-2026-09-30.md) 为准。d329 主32成功但整套后置失败，后来的API定向复验没有将原失败改写为完整PASS。旧 running 句柄不再有效。
<!-- /historical-checkpoint -->

# R1 当前验收：短来源引用与统一提案契约

当前开发检查点 `d329e35c`，R1 NOT DONE。唯一源码和任务状态见 [source-refs-verification](source-refs-verification-2026-09-30.json)。当前分支 `rp-preview-integration-20260929`，HEAD `7fc3b3ba7be229b11341130d6cdd4275e231274f`；工作树不干净，108 个 tracked 改动、203 个 untracked 条目，保留全部既有修改。793 个冻结源码文件复核无差异，四个 Golden 冻结文件 hash 不变。没有部署本轮 backend，也没有 commit/push。

本轮证据没有证明事件账本、typed owner 和恢复内核需要推倒；确实暴露了 RP 数据就绪和模型接口契约问题。此前实施过多围绕工程检查，短对话反馈进入得太晚，不能把大量代码与回归次数算成玩家体验进展。按用户最新授权，路线已改为真实短对话 → 针对实际失败修改现有 adapter → 原世界回归 → 冻结 Golden/人工复评。旧 `f85f7a50` 全后端通过但原 32 轮第 17 轮失败，失败保留，不能算最终验收。

1. **Baseline 问题。** 冻结荣庆堂的 11 个角色缺人设、关系和称呼，旧流程仍调用模型。最新 f85 长回归还暴露来源引用和提案结构易错：首次引用不获准或重复，第二次连结构也失败。没有保存原 private 文本，无法断言旧问题一定是抄错长 ID 或中转丢弃工具；来源 schema 与 core gate 使用同一个允许集合，未发现它们冲突。
2. **Context。** 原 `BuildRPDecisionInput` 仍是唯一事实入口，保留 head/version、来源、角色获准知识、实际听见对白、关系和私有状态；缺关键资料提前 NOT READY。新增仅用于 NPC provider 的 `grounding_sources`，把当前包中的获准 Event ID 绑定为短 `src_N` 引用，原 Character、对白与事实保持原样。它不持久化，不成为第二套 RP 状态，也不给 Narrator 额外信息。
3. **Decision。** 仍为完整 V3 private + 单一 observable。同一个编译好的 `proposal_schema` 现在进入每种传输的模型消息，也用于原生函数/JSON Schema 声明，JSON Mode 不再另拼一份 schema prompt。模型输出短引用，服务端绑定为真实来源，再走原校验；获准 raw ID 兼容旧 V3，未知或重复仍拒绝，不近似匹配、不剥字段、不去重救回复。
4. **Private / Observable。** 短引用只改善模型接口表示。core evidence、provider-view/canonical 双重检查、owner、事务提交与目击链保持。所有传输的单位测试及真实 HTTP 两种通道已验证获准绑定与幂等重放，private 仍不进入公开响应/观测。当前真实四轮的四个神态各有 committed 事件和玩家 `co_location` 目击记录，见[公开文本与动作审核](source-refs-short-review-2026-09-30.json)；不是 Narrator 临时创造的动作。对白中的自述仍只是归属于说话者的发言，不自动证明位置、日程或所述事实成立。
5. **Narrator。** 本增量没有进一步扩大事实权限；公开投影、committed 原话和行为仍是输入。先前 Golden 的抢话时序和推测情绪仍需人工检查；没有因此声称叙事语义安全或表现力通过。
6. **Hard / Soft。** readiness、闭合结构、获准来源、可确定的关系/称呼冲突、世界版本/owner/提交及公开投影边界仍是硬规则。获准引用不等于对白每个语义主张均已证明，不能把这套 gate 写成完整的知识/叙事语义验证器。重复、人格味、自然度、主动性仍是辅助诊断/人工评价。新增未知/重复引用计数和提案结构布尔诊断，区分 schema 错误；不打印私有文本、推理或引用值，不增加无限重写。
7. **世界回归。** 当前 decision 完整包 PASS，HTTP 完整包 48.071s PASS，storage 的模型提交/隐私/私有记忆/重放/重启定向 9.050s PASS，五包 vet 与四二进制构建 PASS。当前完整后端首跑在 storage/HTTP 初始化失败，其余10包 PASS；原失败单测单独重现通过，两包在不改代码/断言的情况下串行完整复验53555通过（storage729.598s/HTTP44.059s），合并为当前12测试包全套覆盖/1无测试包，保留首跑失败，原因未确认。原 32 轮终态只见验证 JSON；f85 完整后端 PASS 不能覆盖新增接口代码。f85 原 32 轮为 16 浏览器轮通过、17 轮结算、50 decision 成功/1 失败、17 真 Prose 成功，后置未到。
8. **Golden / 人工依据。** 当前 d329 冻结复评采集已完成：17 个 NPC decision 全是 `not_used / rp_context_not_ready`、0 NPC HTTP；15 次 deterministic、G8 一次真 Prose 成功，但无角色对话可渲染，RP Experience 未通过。旧 baseline 的缺数据继续生成现象现在被明确拦截，这是安全/诊断改善，不给角色趣味分。当前同设定 Nora/Lin 四轮是真实 Step 调用，3 次原生函数、1 次完整 content，13 个短引用、0 未知/重复引用、0 修复/回退。公开读样可复核熟悉称呼、情绪承接和回忆承诺，仍能看见“陪你说话”等重复及对今晚日程的宽泛自述。见[真实短样](role-ready-source-refs-2026-09-30.json)及[辅助读样](source-refs-short-review-2026-09-30.json)。这只是测试作者的补充世界、助手辅助审核，不能冒充荣庆堂 canon、长 RP、Expressive Narrator 或真人签收。
9. **剩余问题和下一步。** 当前长回归终态以验证 JSON 为准；NPC 私有意图生命周期、语义长期记忆、Narrator 的全面语义事实验证未完成。辅助抽读当前技术长样第 1/8/16/17/23 轮，熟悉称呼和部分情绪回应成立，但三人逐个发言、反复拉回诗笺/帘子/茶盏任务，神态逐句追加，仍明显机械；这是体验欠账，不能被技术通过冲销。此 fixture 把活动写入固定 persona；它和冻结的真实 11 NPC 配置不是同一个世界。旧包未声明执行模式会走串行全部听者的 legacy；已有 orchestrated/预算/会话延续机制，应先验证现成配置和已提交活动状态如何影响下一轮，不能把新调度器扩进此轮。先由世界作者在已有创作入口补齐并明确提交缺失的人设/关系/称呼，以另标的 authored 配置复评同样场景，原冻结 baseline 保留；再让真人依据公开前后文本评角色味和连续互动。作者 canon 和真人 RP 评价仍未取得，不重复询问、不推定批准。不继续无代码变化的重复全套，也不再堆单句 prompt。R1 两出口未满足；不扩 R2，不 commit/push 或自动部署。
