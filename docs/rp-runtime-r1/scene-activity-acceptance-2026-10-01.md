# RP Runtime R1 当前审计与验收结果

后续本地更新：本页下方是`f4b63534`的完整回归检查点；最新诊断、叙事配置与标点修复及独立两人真实Step四轮见 [卡点复查](unblock-diagnosis-2026-10-01.md)。新运行时`8366b5a8`的定向检查已通过，但没有新完整32/真实Golden/真人体验通过记录。原冻结文件与下述失败结果保持原样。

R1 未通过验收。当前源码的12个后端测试包均有通过结果；原完整浏览器32轮在第29轮因Step决策超时退出。冻结荣庆堂复评的17次NPC决策全部NOT READY，没有NPC模型请求，真人体验验收仍缺。新的活动可见性修复有效，但不能据此宣布已获得酒馆长对话体验。

功能源码固定在已远端核实的 `8acd85f51fe06630d4db9c24e48e0ce11048be4b`，运行程序SHA为 `f4b6353465fab223d3f0df19e48819805a781d3124a873d3d197ac8318006e21`。798个源码文件及四个冻结Golden文件的hash复核一致。本报告之后的提交仅整理本轮结果。所有本轮任务已经终态，未部署或进入下一阶段。

详细机器证据见 [当前验证清单](scene-activity-context-verification-2026-10-01.json)，人工可读的 [活动边界说明](scene-activity-context-2026-10-01.md)、[原32轮公开部分样本](scene-activity-32-public-2026-10-01.json)、[冻结Golden公开样本](scene-activity-golden-samples-2026-10-01.json)及[同事实叙事对照](same-facts-prose-review-2026-10-01.md)。不能用采集脚本exit0代替RP验收。

1. **Baseline暴露的问题。** 原荣庆堂没有足够的人设、关系及称呼数据，旧样本出现自我介绍、关系/称呼未形成连续上下文等表现。其回应缺乏足够作者设定支撑，不能把模型的原著常识当作当前canon。严格遵守当前canon后，本轮仍无法开展有效角色决策；G8公开输出为“你：「找老祖宗呀」 / 陌生人沉默着。”不能把沉默当成修好自我介绍。活动越过闭门的缺口来自本轮正常typed提交的独立隔离测试，不是把它冒充成原Golden失败。

2. **Context Contract。** 原 `BuildRPDecisionInput` 仍是统一入口。既有人设/关系/称呼、实际heard/relevant对话、角色自己的private历史、知识权限、readiness、head和provenance继续使用。本轮没有建立平行Builder：同地点活动以前未按感知过滤，现在复用canonical可见角色，过滤后再取10条；加入 `source_event_id`、`observation_basis`，当前可见快照不披露未目击的开始时间。他人未见的结束结果不由后来可见推定，自己的有来源结果保留。

3. **NPC Decision Contract。** 主快照已有V3的private与one observable分层、短引用还原、正式schema及自己的已应用private历史。本轮没有修改其输出、修复次数或deadline。合法来源不等于台词中的每个主张都是真实世界事实；“要给你倒茶”不能自动创建茶或执行倒茶。当前资料不能证明动态目标、长期语义记忆或角色主动性已通过。

4. **Observable/private边界。** 活动来源必须与提交事件的actor、类型、activity、地点、状态、时间、序列及当前head相符；不符即拒绝。闭门不可见、合法当前观察、隐藏结束后再见、自己的结束、来源改名/截断版本、重启和重建均由定向测试验证。没有新增owner、ledger状态或migration，也没有把NPC private送入Narrator。世界能追认已提交observable，不保证机器可穷尽判定自然语言中每个隐含新事实。

5. **Narrator。** 原公开事实renderer可组织段落、衔接、措辞及允许的公开风格，保留已提交对白；不能创造互动动作、物品、人物、位置变化、未提交proposal或private意图。本轮不修改Narrator，而对同一组已提交Nora对白和点头使用两个现有配置：79→96字、来源相同，世界/台词/决策不变。普通配置仍机械，并有引号后叠加标点；没有明显体验改善或真人通过。首次probe读错HTTP envelope的FAIL保留，成功公开render只读恢复，未追加模型请求。

6. **Consistency Gate。** 本轮新增硬规则是授权视觉范围及事件/head一致性，均能确定判定；正式响应字段、引用权限、typed owner及提交/replay/visibility规则继续作为硬边界。答非所问、重复、人设漂移、自然度与趣味性仍为soft/人工判断。不能把正则事实检查说成完整语义证明，也不能用不可靠的语义分类器阻塞正常RP。

7. **技术回归。** 最终两项活动定向测试PASS（1.743s）。全后端首跑11个测试包PASS/1无测试，storage因默认累计10分钟超时退出；该时刻旧测试才初始化不到一秒。按backend/README已有30分钟参数，仅storage同源复验PASS（728.028s），得到12包通过覆盖，初次FAIL仍保留。这不是整套单次exit0。此前本轮SQLite失败由测试overlay定位为SQLITE_FULL，生产错误格式未改；不能倒推旧全部失败的根因。

   原浏览器32保持全部案例与断言：28轮通过、29轮结算，86次NPC决策成功/1次两attempt后的120秒timeout沉默回退，总93attempt，29次真实Prose成功。原no-fallback断言失败，30–32轮、混合物品解释、meta continue、故障和最终restart未执行。`quick_check=ok`不是这些后置检查通过的替代。World Integrity Exit未通过；没有改短案例、删断言或把旧API续验拼成当前完整32成功。

8. **Golden与人工依据。** 当前原场景及包重建后采集16条公开记录，17次NPC `not_used/rp_context_not_ready`、零NPC HTTP，G8一次真实Prose成功。与先前同配置缺canon的复评相比，没有角色、关系、上下文、主动性或叙事的明确改善。八轮Nora/Lin/Iris及两个style短样是独立明确测试作者世界，不能替代真实荣庆堂、长对话或真人rubric。真人评价为PENDING，RP Experience Exit未通过。

9. **剩余问题。** 实际荣庆堂设定补齐与可检查的真人Golden评价仍未取得；Step尾部延迟会使严格全32中断；静态persona任务、持续意图、相关旧对话检索、承诺与实际物品事实、机械表情/引语及free prose事实检查的局限仍需独立审计。活动owner没有记录他人结束见证，本轮选择不披露该历史，未新建机制补齐。旧预览仍未更新，不能把GitHub推送当成部署或R1完成。

下一步先由dot针对固定功能提交及公开失败样本给出具体findings，再收敛最小的R1修复。真实设定必须由作者明确，不从名字或原著补canon；新的设定补充必须单独标记，不覆盖冻结baseline。验收仍须保留原32完整出口和可比较Golden/真人评价，当前停止扩展功能。
