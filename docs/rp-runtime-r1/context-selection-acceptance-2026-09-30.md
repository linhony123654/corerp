# RP Runtime R1 · 统一上下文增量验收

最新源码已在用户授权的新路线下修复“重复语义信号触发同步重写”，见[最新交接](semantic-diagnostic-2026-09-30.md)。本文九点是f6c96778上下文增量的采集与验收事实；它的完整测试及Golden不覆盖最新cf104b95源码。R1仍未通过两个出口。


当前结论：架构需要补强RP运行层，可信世界内核保留。已有公开对象延续和本轮上下文选择得到可复核的结构性改善；R1尚未同时通过World Integrity和RP Experience出口，不能宣称已经达到酒馆长对话体验。

## 1. 冻结baseline暴露的问题

真实荣庆堂11角色缺persona、关系/称呼及公开表达设定；名字不等于原著canon。冻结失败文本出现重复身份介绍、把NPC台词和叙事混在一起。代码审查和单独的可复核回归另外证明了窗口限制、按ID换对象及重复/预算缺口；这些不是都由原Golden单独测出的现象。世界结算正确与角色自然扮演是两个不同出口。冻结四份原始配置/样本保持原字节，当前Step与原Gemini不能做同模型因果评分。

## 2. Context Contract

沿原BuildRPDecisionInput及initiative出口统一选择，没有新Prompt链/记忆库/世界状态。仍保留版本/head、角色/关系/称呼/readiness、世界时间/场景、实际听见的发言、获准知识、活动与合法选项。新增当前原话的源时间、Context selection policy/实际字节/省略计数。JSON编码NPC packet默认64KiB，必须数据本身超预算时显式拒绝，不截断canon或模型补设定。

完整台词按Event+speaker只保留一个body，重复视图保留时间/地点/行为/来源，text_from_event=true指向同包完整原话。不同事件、同声明的不同人/不同话、关系及private不能简单合并。Life.SalientMemories的speaker_said同样纳入；非台词owner状态和内部机制的完整读取不裁减。consumer须解析引用，空Text不代表没说话。core.ResolveRPDecisionSpeech只解析包内已授权完整原话，不查私有草稿或把截断摘录当全文。

选择为确定性启发式：近期本人的交流与最新可观察动作、完整topic旧交换、近期本人意图、其他相关历史；仍受原候选窗口约束。不是语义真理，也不宣称覆盖全部历史。身份遮罩后重选，避免实际provider JSON增长超预算。

## 3. NPC Decision Contract

沿已实现wire v3的private+单一observable：内部动机/情绪/关系姿态与依据独立，observable按respond/refuse/silence/wait/leave/act约束合法字段，表达选项通过既有非语言owner提交。本轮新增按actual provider view校验依据后，再检查canonical input与typed提交链；不能引用预算省略后的来源。原重试次数、输出预算及120s总上限不变。

## 4. observable/private边界

只有批准并提交的NPC台词、动作/受支持expression才进入世界与公开目击；proposal或private没有自动公开。最近本人意图仅从本人已应用不可变决策/完整batch/批准hash读取。Narrator和玩家仍用公开已提交投影，本轮选择统计不公开。台词Event证明“角色说过”，不证明台词声称的茶、往事或递交动作真的存在。

## 5. Narrator

既有公开Renderer可按允许的公开风格调整节奏、措辞及已提交observable的表现；对白引用由服务器插入完整原话，模型无需重抄事实。它仍不得创造可交互动作/物品/人物、改变位置、泄露private或把proposal/内心当公开事实。本轮没有为压缩Context而放松任何叙事事实guard，也没有修改Narrator源码；角色表现力必须在最终Golden里复评。

## 6. consistency hard/soft

硬规则包括合法行动及效果字段、支持的expression/目标、获准依据Event、实际consumer visibility、owner/head/hash/原子提交、公开来源和原话保真；引用缺失或同一已接受发言来源冲突直接失败。已有熟人自我介绍机械规则不等于全面语义关系判断。称呼/人格/答非所问及知识陈述的自然语言含义不能凭不可靠分类器宣称全部硬验证。重复/格式问题仍使用原一次有界soft rewrite与diagnostic；趣味与文学质量由真人判断。

## 7. 技术回归状态

core/decision/narrative包PASS，相关storage定向77.549s、五包vet、独立构建PASS。完整go test ./...首跑storage994.050s只有一项旧断言失败：要求Life重复Text。已改为完整原话+精确actor/Event/time，并保留财务权威/隐私/重启/重建检查，单项0.998s PASS；其余11测试包首跑PASS、1无测试。完整storage复验80283已exit0/984.391s。合并未变生产源码首跑的其余11包，当前12测试包完整覆盖PASS/1无测试；不是把原完整命令的exit1改成exit0。

同runtime f6c96778的原有浏览器32回归89506终态exit1：8浏览器轮通过，9轮结算；26成功NPC决策（23首试/3两次），第9轮一NPC输出截断后两次达到120s总预算并安全沉默。输入22703/request34842字节，未超Context预算。9次真FullProse首试成功（另9次deterministic回执不混算）。regenerate/mixed/continue/fault/restart后置未到达；全32 NOT PASSED，原脚本及断言没有降低，无原样重复32。终态公开样本在context-selection-32-result-2026-09-30.json。

## 8. 前后比较与人工依据

在同一合成作者配置下，f4d5051f与f6c96778双方各先通过typed HTTP提交28轮mock历史，再进行第29轮真实Step回忆。2/2真实请求首试成功，双方都完整回忆“可以带诗集，但缺末页”。Context wrapper99807→65135字节，问题副本4→1、旧回答2→1，after NPC packet65090/65536字节、46个全文引用、5项低优先级OwnActions省略。它证明预算/去重没有丢该旧约定；没有证明人物更自然或因果提速。真实请求与模拟阶段明确分别标注。

此前公开对象延续的Step短对照改善了Nora→Iris误切换，当前源码保留该能力；两类合成小样都不替代真实冻结荣庆堂或真人rubric。冻结Golden21660已exit0完成采集：17/17决策成功（16首试/1第二次）、G8真实Prose首试成功、0expression。贾母仍对“想你了”回答“老身不记得与你有旧”，凤姐仍出现敌意及把“你先处理”理解为玩家离开的台词，G8仍仅两行对白。G7局部没有重复旧版无来源茶位置暗示，但不足以证明总体改善。G2承接身份回答，而先前Step也能承接。完整[前后文本及来源表](context-selection-golden-review-2026-09-30.md)覆盖G1–G8；没有明确角色/关系/表现力改善，RP Experience NOT PASSED。缺设定维度N/A，作者设定/真人复评未取得，助手读样不冒充人工签收。所有任务终态。

## 9. 剩余问题与停止边界

缺作者canon是角色/关系验收的实际缺口；不能根据《红楼梦》先验补齐。这不是把失败归咎于作者：本轮投入较多上下文和回归工作，但没有及时交付一段有角色感的可玩对话。下一步的交付单位应收紧为明确作者声明下一个角色的连续互动，先核查G2/G3/G5/G8文本，再由具体失败决定修复；完整回归用于验证有实质改动的结果。相关旧话只覆盖有界的授权候选，词法匹配不处理所有同义转述；最近三项private不是完整长期目标生命周期。NPC可执行动作及支持表达仍受现有owner/contracts限制。Step随机输出长度与超时不因一个短对照消失。自然语言角色/知识矛盾不可能仅靠依据Event枚举完整识别。实际32、冻结Golden与真人出口未通过前R1保持NOT DONE；不进入R2/R3，不扩插件/owner/UI，不部署/commit/push。

[实现设计](context-selection-2026-09-30.md)、[唯一当前检查点](context-selection-verification-2026-09-30.json)、[Step公开对照及读样](context-selection-pilot-review-2026-09-30.md)、[冻结Golden复评](context-selection-golden-review-2026-09-30.md)、[作者及真人复评单](canon-review.md)。
