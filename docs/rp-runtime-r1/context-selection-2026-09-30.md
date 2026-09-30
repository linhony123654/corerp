# 统一RP上下文选择 · 2026-09-30

前一公开交流焦点增量是实际进展：源码f4d5051f定向、vet/build、完整backend12包PASS，Step两轮对照已复现并改善接话对象；所有任务终态，不重复启动旧检查。本增量处理原Builder的重复与统一选择，不建立记忆/世界状态或平行Prompt链。

## 验收与设计

- 普通发言及wait initiative由原数据读取/授权路径装配完毕后共用确定性选择函数；内部世界机制继续使用完整、已授权的状态，不因模型预算失去owner事实。
- 当前输入、identity/persona及readiness、所有已声明关系/称呼、世界版本/场景/时间/活动、合法行动及关键源、非台词知识等保留。必需内容自身超预算显式失败，不截断关键设定、不让模型补canon。
- 统一64KiB的序列化NPC Context预算：优先当前交流近期原话与最新可观察自身动作，再相关旧交流、本人过去私有意图、其他近期/旧话和自身行动历史。保留原候选数量上限，选择顺序确定，不把词法匹配当语义真理。
- 完整台词只存一份。同一获准原话的重复body以明确text_from_event=true（引用本条已有event_id/source_event_id）引用已选原话；时间/地点/行为/来源元数据仍在。引用必须有同包完整原话，不能引用已因预算省略的内容；片段不能当完整原话。同source的不同关系/动作/私有记忆保持独立，绝不全局按Event简单删除。
- 相关旧交流按完整组选择；原话不因总预算截短。省略与去重显式统计为bounded-candidate selection，不能声称上下文包含全部世界历史。
- NPC identity mask之后重新核实实际编码预算；提供方提出的依据先按其实际获准视图检查，再沿原raw上下文/typed提交链验证，防止使用被省略的知识。
- NPC私有内容及选择统计不进Narrator/玩家公开投影。Canon/readiness与世界验证不放松，无新owner/插件/UI机制。

## 当前实现与证据

纯选择函数已经接入原BuildRPDecisionInput与readRPInitiativeInput最终出口，内部owner读取保持完整。身份遮罩后按实际视图再选，并在canonical/typed校验前校验actual provider input。台词源身份使用Event+speaker；同一声明支持不同人或多条原话时不强行合并。private永不作为台词源。

真实28轮压力用例曾发现Life.SalientMemories仍重复当前问题；已将其speaker_said统一纳入预算及来源引用，非台词Life/工作/就业状态保留。改动后的压力初测与core PASS；新增Life/共享声明边界测试、decision/narrative包PASS（components日志）。第一次增强provider视图用例未产生raw-only遗漏，明确FAIL为fixture不足，改为真实听见的未介绍第三人后重验；不伪称这是产品失败。

新增Life/共享声明与临界alias budget/未提供依据拒绝测试通过；真实28轮storage压力同时覆盖未介绍第三人在场的ID遮罩和重启。storage相关定向77.549s、五包vet和独立构建PASS。第一次临界视图fixture未制造源省略，三个失败日志如实保留；独立临界样本精确构造budget增长，raw允许但实际视图省略的依据必须拒绝；真实压力用例则继续验证实际记忆与遮罩，不降低产品检查。

Step小样已完成（双方28轮mock历史后末轮真实调用，2/2首试成功）：Context wrapper99807→65135字节，当前问题4→1份，旧回答2→1份，双方正确回忆缺末页。见pilot JSON与review；不当真人体验或因果速度证明。完整backend90621首跑exit1，仅旧Life重复Text断言失败；已改为精确words/actor/Event/time及恢复检查，单项PASS。完整storage复验80283 exit0/984.391s，合并首跑未变生产源码的其他11包，12测试包完整覆盖PASS/1无测试；保留原完整命令exit1。当前源码全32 89506 exit1，第9轮输出截断后120s timeout，8浏览器轮通过、9结算、26成功/1失败，后置未到。冻结Golden21660 exit0完成采集，17/17决策成功、G8真Prose首试成功、0expression；角色/关系/表现力仍无明确改善，体验出口NOT PASSED，见[公开复评](context-selection-golden-review-2026-09-30.md)。所有任务终态，UI没有本轮改动，无部署/提交。

## 验证路线

先RED反例：当前/近期/旧话/知识/自身行动的重复body，完整交换与预算、共享Event的不同关系和private、同包引用、超大必需设定显式错误、不改调用者输入。之后真实storage builder、initiative、alias投影、来源gate、故障恢复/重启及被挤出近期窗口的旧交流验证；定向PASS后完整后端，再安全Step小样诊断请求大小/耗时，决定实际32/冻结Golden。原冻结四文件不改；作者canon与真人评价仍待，R1 NOT DONE。
