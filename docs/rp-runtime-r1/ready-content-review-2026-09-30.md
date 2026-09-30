<!-- historical-checkpoint -->
本文下方为历史检查点。当前 runtime 为 `edf6750b`，任务均已终态；最新结果以 [interaction-contract-verification](interaction-contract-verification-2026-09-30.json) 与 [interaction-contract-review](interaction-contract-review-2026-09-30.md) 为准。d329 主32成功但整套后置失败，后来的API定向复验没有将原失败改写为完整PASS。旧 running 句柄不再有效。
<!-- /historical-checkpoint -->

# R1 当前验收：就绪检查与统一提案解析

结论：已确认问题位于 RP 数据就绪和模型接口的控制流；现有证据没有证明可信世界内核需要推倒。R1 仍未完成，当前预览也未部署本轮代码。最新源码、二进制和任务终态只以 [ready-content-verification](ready-content-verification-2026-09-30.json) 为准；下面区分当前修改与此前失败样本。

1. **Baseline 的结构性失败。** 冻结荣庆堂 11 个角色缺人设、关系和称呼。此前虽标记 MISSING，仍调用模型，使“资料未就绪”变成了模型根据名字补角色。9c7d372d 的同场景复评 17/17 决策成功，却仍有盘问、自我介绍、无关 `telehealth`，不能称为体验通过。模型 JSON Mode、函数调用也分别出现格式不合约；短探针成功不能代表连续对话稳定。

2. **Context Contract。** 继续复用唯一 `BuildRPDecisionInput`，canon/head、角色、定向关系/称呼、可见场景、实际听见的对白、相关旧交流、获准知识和本人已应用私有决策都保留来源。新增发送前的就绪检查：缺人设或已有关系缺称呼时停止模型调用；仅含空白的人设也算缺失。关系 UNKNOWN 允许以陌生人交互，不自动补祖孙或其他关系。

3. **NPC Decision Contract。** 原生单函数仍优先，参数须完整符合 V3 `private + observable`。针对中转没有返回函数的情况，仅在零函数、`finish=stop` 时接受完整 V3 JSON content，并通过同一解析、grounding 和世界校验；这不是把普通回复直接交给玩家。自由文本、代码围栏、半截 JSON、旧根结构、未知/多个函数、额外效果、越权依据继续拒绝。private 字段补齐通用的单行要求，原长度和一次结构修复上限不变；诊断只含长度/布尔值，不记录私有内容或推理。

4. **Private / Observable。** 两种序列化通道都只产生 proposal。角色知情范围、身份遮罩、canonical 复查、typed owner、原子事件和目击记录保持。已验证两种完整通道的真实 HTTP 提交与幂等重放，private 和无关普通文字不进入 Narrator 或公开观测。缺资料走既有安全沉默提交，但回执明确是 `not_used / rp_context_not_ready / 0 HTTP`，不会伪装成模型超时或 NPC 主动不理玩家。

5. **Narrator。** 本次没有进一步放宽叙事事实权限。仍用公开投影、服务器插入 committed 原话和事实检查；表达可以组织节奏和已提交行为，不能补可交互事实。历史 9c7d372d Golden 有 13 个已提交 expression，G8 真 Prose 首试成功，文本更长；其中抢话时序、推测情绪仍需人工审查，不能因此给表现力或边界通过分。

6. **Hard / Soft Gate。** 就绪数据、闭合结构、合法行动、授权来源、head/owner、明确关系/称呼、公开/private 分隔是硬规则。自然度、重复、主动性、人格味仍是诊断/真人评价。兼容序列化通道不降低世界合法性，语义重复信号不触发无限重写。缺资料的角色不再耗模型额度；玩家界面用原有状态样式说明需要创建者补齐设定。

7. **世界技术回归。** 初始函数检查点完整后端已 PASS（12 有测试包，storage734.958s/HTTP48.243s），原 32 轮第 8 轮私有字段失败（23 成功/1 失败，后置未到）。随后 38350e44 四轮第 3 轮因两次没有函数返回而失败，未盲跑其 32 轮。当前最终源码完整后端也已 exit0/PASS：12 个有测试包、1 无测试包，storage735.828s/HTTP49.246s；定向包与 HTTP/storage 边界 PASS。当前真实四轮 4/4、4 HTTP/0 mock/0 修复/0 回退；两次函数和两次完整 content 都通过同一提交链。最终 full32/完整套件状态见验证 JSON，未结束或失败均不算 World Integrity Exit。

8. **Golden 与人工依据。** 冻结世界/包/before/after 四文件保持原 hash。原始 Golden 的角色资料缺项不能由模型或助手替作者填充；当前就绪检查下应明确 NOT READY，不应为了“有输出”跳过 gate。本版冻结 Golden 已完成 16 样本采集：17 次 decision 全是 `not_used / rp_context_not_ready`、0 NPC HTTP；G8 Prose 一次真实调用成功，但缺 NPC 对话可渲染，因此仍是 NOT READY / RP Experience NOT PASSED。设定完整的 Nora/Lin 虚构作者世界四轮通过，认得 Lin、不替玩家编失落原因、能回忆陪聊约定；第 2–4 轮措辞和点头重复，四轮也不足以验收主动性。四个公开 gesture 已逐个核对 committed Event/玩家同场目击记录，所说“坐着陪你”仍只是台词，不额外确认姿态。原话及受限阅读评价见 [短样审查](ready-content-short-review-2026-09-30.json)。助手读样不是用户人工签收，也不是荣庆堂 canon 或长对话/表现力验收。作者设定与真人评价仍待答复，不重复询问、不假设已批准。

9. **风险与下一步。** R1 NOT DONE。当前接口对中转的适配仍需真实连续验证；旧 system package 未声明执行模式时，既有调度默认 `legacy`，会依次调用全部听者；代码已有 `orchestrated` / responder budget / 公开会话延续机制，后续应先验证现成编排配置，而非再建一套角色调度器。原 32 轮保持既有三 NPC 断言，不改用单 NPC 冒充通过；private 意图的完成/失效生命周期、语义长期记忆和 Narrator 的全面语义事实验证没有完成。旧世界缺必要数据会得到明确未就绪提示，补数据须走创作者声明。当前短样已通过，完成已启动的原 full32 和当前完整后端检查；机器通过不能替代体验改善和人工签收。不扩 R2/R3，不 commit/push，不自动部署。

测试 fixture 的调整只用于接口测试：把原来没有人设的 travel demo 换成通过正式 Studio 入口声明人设的隔离测试世界，保留提交、隐私、故障、恢复、第二客户端和重建断言；跨世界 event_sequence 不再混作同一条序列。没有修改生产 demo、预览世界或冻结 canon。
