# 公开交流对象 · 真实Step两轮对照

同一合成作者声明、包和两条输入，在两个独立临时世界上使用`step-5-preview`、120秒总时限、4096输出预算、low reasoning；Narrator使用确定性renderer。前版runtime2e527072与新版f4d5051f的四次真实NPC调用均一次成功、各轮settled，没有fallback。它们是受控合成设定，不是冻结真实荣庆堂；不得将本小样算成32轮或真人RP验收。

| 检查 | 旧运行时 | 新运行时 |
| --- | --- | --- |
| 第一轮“Nora，我想跟你聊聊，最近总觉得话说不清楚。” | 点名Nora，实际回应 | 点名Nora，实际回应 |
| 第二轮“那你觉得我应该先说哪一句？” | stable_fallback选Iris，Iris沉默，交流中断 | conversation_continuation选Nora；来源精确指向第一轮Nora的已提交、被Lin听见的speech Event |
| 第二轮公开对白 | 无 | “不用挑，从你最想说的那句开始就好。” |
| 实际决策调用与回执 | 2次，各一次success | 2次，各一次success |

这个对比可复核的改善是“正在交流的人继续接住未点名追问”。选择依据来自公开事实和固定计划，不由另一个LLM猜测交流对象；三运行模式的确定性反例、接管及重启测试补充覆盖。旧版第一句还带有“Nora軽声回应”等角色名、杂字与叙述残片；新版这次未出现，但第一轮选择和输入并未因延续逻辑改变，生成又没有固定seed，不能据此宣称文风/格式残片已修好。耗时旧32.280/70.761秒、新30.249/28.283秒；不构成速度或中转稳定性结论。

本记录是助手对公开样本的辅助读样，没有替用户进行人工rubric评分。只有两轮、两名NPC、已声明泛化persona/熟悉身份；没有证明长对话、复杂指代、亲属称呼、长期目标或Narrator表现力。冻结11角色仍缺作者persona/关系/称呼声明，历史真实32轮曾超时，新源码的完整32与Golden没有通过依据；R1 NOT DONE。

测试配置曾两次在世界创建阶段因canonical内容hash拒绝，未调用模型：`full_prose=false`为typed profile的omitempty字段，已从合成包canonical输入移除。两个失败记录保留为preflight-failure/preflight-diagnosis JSON，未放宽包校验。采集器路径`/tmp/corerp-r1-conversation-focus-pilot-20260930.py`；秘密仅经已有ignored配置在进程内传递，导出不含endpoint/key/token/private。

详细公开来源、固定设定、输出与回执见[两轮小样](conversation-focus-pilot-2026-09-30.json)。
