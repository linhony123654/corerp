# CoreRP：功能准确度与动作反应闭环，2026-10-01

已验证的世界内核有基础，但 RP 功能在输入、触发与公开呈现之间存在真实断点。本轮修复了其中一条完整路径：玩家向 NPC 点头，NPC 基于该动作的真实观察决策，原 owner 提交回应，Narrator 呈现已提交结果。真实 Step 前后对照已经出现这项功能改善；自由对白准确度和整体角色体验仍未通过，R1 NOT DONE。

这是增量报告，不是 R1 最终验收。冻结 baseline、原32和人工体验出口继续保留。

## 当前版本与证据范围

分支 `rp-preview-integration-20260929`，构建基点 `cfc6c9a7ca84a5d88b76ba2134a84fa7c3f5f579`。本轮后端生产源码 439 文件摘要 `e2e7b01977b68a5c94b4a9561e6ba26667bfa20d0cb0954ae6c1247bd91c42ea`，runtime `7e8145d421fa0a8ab265af8037234652a80bc1f0b9c1165127a96791b81864ec`，schema080。最终 Git 提交以分支实际 HEAD 为准，逐文件摘要绑定本轮测试与真实采集。

真实采集发生在隔离库；采集时线上隔离预览仍是 feea/schema079。不要用“已构建”替代“已发布”，也不要把后来部署的结果倒写成采集当时的状态。当前部署身份以预览 `/release.json` 和独立发布回执为准。


本轮交付已完成：代码提交 `300597c26eeaef4089402a7a02aca253086fdb8d` 精确发布到该分支，同一预览已升级080并运行7e814 runtime和对应前端。旧201Events/15provider receipts/66旧回合的原列与所有heads在迁移时不变。线上默认Step实际点头回应13.821s首试成功，speech与smile提交后可读，重启/原key重试零新增effects/calls。该次direct烟测后R1世界head34，其他旧worldheads不变。

该线上回复仍说“站惯了”“这座儿总给你留着”，没有相应姿态/座位canon或owner来源；旧NPC说法也不是其真实状态来源。它进一步证明动作链通过不等于自由对白准确度通过。逐字输出与独立交付回执见[action-trigger-preview-release](action-trigger-preview-release-2026-10-01.json)，[在线动作审阅](https://code.linhony.xyz:4188/rp-review/actions/)仍将人评标PENDING。后续文档提交与这个代码提交分开，不伪造新的runtime构建。


补充实际 Play AUTO 接口验证：输入“我向贾母点头示意。”，Step 一次解释为 ACTION/nonverbal，随后一次 NPC 决策，合计22.783s，两次首试成功。玩家没有被伪造成 speech，目标反应和公开v2保存，原请求重试零新增effects/calls。解释回执保存在原`rp_interaction_interpretations`，NPC回执在`rp_provider_calls`；harness首次误查同一表而FAIL，原件保留，复核两套原账后只恢复同一已接受plan/key，没有再次抽样。见[AUTO真实证据](action-trigger-auto-real-2026-10-01.json)。这证明默认AUTO入口，未声称浏览器点击或另一个model-profile override也已实测。

只读审计没有发现把`said`/历史private直接映射为`current_snapshot`的代码：它们分别在accepted_utterances/past_private；snapshot有place/activity/visible IDs/object state，没有pose、seat occupancy或室内距离。明确缺口是`respond.Text`仍为自由字符串，hard gate验证来源可读与owner，并不证明每个句子的蕴含。补physical unknown只能是软帮助；若需要某类物理断言的硬保证，它必须对应可验证typed source/服务端有限表达，不能靠claim-ID数量、关键词拒绝或再问一轮模型冒充保证。

## 为什么底层跑通了，功能仍不准确

| 已有能力 | 实际断点 | 本轮处理 |
| --- | --- | --- |
| 事件原子提交、typed owner、重启恢复 | 点头能入账，但服务不创建 NPC 反应回合；玩家看到一句动作描述就结束 | 定向点头接入原 durable turn，共享 NPC 提交与结算循环 |
| 冻结的视觉 witness 与知识记录 | NPC packet 漏掉已有观察中的 action、target、gesture 和发生时间 | 从该角色的已冻结观察投影原字段，不从 raw Event 补不可见目标 |
| 关系与身份披露规则 | `known_relationship_introduction` 误拒绝熟人间合法询名 | 移除这项不可靠语义硬拒绝，保留真实介绍、身份学习和权限校验 |
| 丰富的唯一决策 packet | 大块平铺资料难以凸显当前事实、已说的话、计划和历史私有想法；来源目录很大 | 单份分区 character presentation 与无损来源元数据压缩；真实试验未证明对白改善 |
| 来源引用和合法 speech | 引用了来源，不代表“站着”“座位留着”等每个对白命题都成立 | 明确保留为未解决准确度风险，没有伪称已修好 |
| 安全 Narrator | 可以安全渲染，不自动等于文学叙述已经有味道 | 本轮沿用有限 v2 表达，只接上动作回合，不扩大事实生成权 |

原始荣庆堂 baseline 的关键 persona/关系缺失，确实会导致陌生人式回复；后来明确作者声明的 READY 世界仍出现无依据对白，说明不能一直归因于缺配置。两天底层工作验证的是事实提交与恢复等能力；它没有自动证明每个玩家入口都能把事实交给角色，也没有证明角色自由对白的语义准确度。

## Context Contract：升级原入口

仍然只有 `BuildRPDecisionInput`。原裁剪、知识边界、来源、readiness、world head、身份投影和输入 hash 继续生效。

新增 action trigger 使用真实 `RPNonverbalAction` Event 作为 parent：`trigger.kind=nonverbal`，`observed_player_action` 包含获准的 actor、target、action/gesture、place、world time 与 source。`TurnID` 是实际动作 Event ID；三个玩家 speech 字段为空。没有伪造一句“我点头”、一次 speech turn 或一次 wait。

一般历史 knowledge 同样恢复已有非语言观察字段。字段缺失就保持缺失；target 不可见就不补 target。该角色的 knowledge、observation 和冻结 witness 必须一致。已记录的“某时看见点头”不证明现在姿态、距离或同意。

provider 只收到一份分区 presentation，当前输入、作者 canon、当前 snapshot、接受的 utterance、recorded actions、历史 private、未来 plans、typed domains 与 provenance 分开。原值和范围不变，JSON locator 映射到实际传输位置。wire evidence-support v2 可无损抽出重复 defaults/profiles/scopes，不删除来源、不提高 131072B 上限。测得最大 decisionContext 128994B；这不是整个 HTTP body 的预算。

现有061对象定义/状态只读进入同 head 的角色上下文，不添加物件或 NPC 对象操作权。该荣庆堂 fixture 没有设定座椅，这项读取不会让“有座位”自动成立。

## Decision Contract、owner 与恢复

NPC response 仍是 v3 `private + observable + grounding`。意图、情绪或关系 stance 只进入原有角色私有记录。获准 speech/expression/action 继续经过原 hash/head、source、visibility、controller 与 typed owner。

schema080 扩展原 `rp_turn_runs` 的 typed parent，speech 保留原 FK；nonverbal parent 是真实 action Event，不能借 speech 外键伪造。原 speech/NPC/narration/settlement 循环收敛到一个 `finishRPTurn`。历史 raw nonverbal 的同 key 重试保留旧结果，不追补模型反应。

本轮只让**定向点头**产生反应回合。目标 NPC 必须是这次动作的实际冻结视觉 witness；旁观者保留自己的观察，但不被自动激活。外部控制角色不会被内部模型代演。

动作先经原 nonverbal owner 提交。只有自己精确 pinned run 可以继续该 owner 的 pending fence；其他任务照常阻塞。动作之后只能接受同 parent 的完整 NPC decision batch，不能把别的等待、世界写入或控制器动作悄悄收编。Narrative-ready 写事务再次检查 continuation。

同时修复共享流程的两处实际错误：action 的决策审计不能用空 speech Event 作为 FK；provider receipt 不能只按 `player_turn_id` 查找而漏掉 action parent。失败 provider 仍保存动作并公开报告 silence/failure，不伪造成功。

六个 durable stage 的中断恢复、AUTO 原计划恢复、Stop/Retire 不抹掉已接受动作、同 key 无重复 effects/provider calls 均有定向测试。HTTP 原 nonverbal endpoint 使用同服务路径；AUTO 请求保留其 provider override。Play 读取有 `turn_run_id` 的结果，因此动作反应也能进入原叙述显示与重试入口。

## Observable/private boundary 与 Narrator

玩家动作来源、NPC speech 和 NPC smile 都先提交，公开叙述才出现。玩家自己的动作从自己的 owner receipt 追认，不伪造一份“自己目击自己”的 observation；他人动作依旧要求历史冻结 witness。Observe 只去掉已被 canonical 动作回合覆盖的 raw 重复行，保留之前真实移动及其他合法历史。

Narrator 继续只读公开 projection，不读取 NPC decision packet/private。本轮没有新放宽自由 prose：有限 v2 可改变句式、段落、动作与对白衔接，speech、actor/target、完整 source coverage 和 commit lineage 不变。不能创造姿态、走近、递物、同意或未提交神态。对白里的邀请不等于玩家已行动，也不能独立证明座椅或站姿存在。

hard rule：schema、typed parent/FK、owner、controller、head/hash、冻结 witness、source scope、已声明的称呼冲突、未提交 proposal/私有资料进入公开正文、已提交事实的改写。

soft/人工：自然度、重复、答非所问、人格漂移及自由对白中未可靠判定的前提。移除熟人询名的错误 hard rule，没有换成新的语义分类器。不能因为软信号说“通过”就认定对白正确。

## 真实 Step 前后与人工依据

[公开证据](targeted-nod-real-2026-10-01.json)与[逐字审阅页](targeted-nod-review-2026-10-01.html)固定相同作者 spec/packages、初始状态和 Step 参数，不固定模型逐字输出。

改前定向点头：0 个 NPC 决策、0 次 NPC 调用，仅“有人点了点头。”。

改后：1 次实际 Step 首试成功，NPC reply 与 smile 原子提交，公开正文包含“你向贾母点头”和贾母回应。再一轮实际 Step 回答玩家“点头不代表答应”，上下文能引用刚才动作；没有提交玩家同意或移动。两次调用没有 fallback。

第一动作真实生成 7.659s，后续实际一轮 14.824s。初次采集因 harness 错把“历史总条数必须为一”而 FAIL；此前真实移动本来就该保留。修正为匹配一个 action turn 且无 raw nod 重复，恢复同一 DB/runtime/key，第一次动作与回应没有重新抽样。恢复读取 0.041s 不能当生成耗时。初始 FAIL 和恢复回执都保留。

可以复核的改善是**动作终于产生角色反应，且下一轮能接着引用**。新对白仍有“来了就坐”“慢慢说给我听”等模板，也有无法独立验证的 NPC 自述。“像不像贾母”“有没有戏”“是否想继续”仍待玩家评价，不给自动好玩分数，不代替真人批准。

## 验证状态与下一步

完整 HTTP 包 PASS 77.274s；core/decision 包、vet、frontend build、migration/typed-parent/rollback、动作 commit/restart、源可读与反应路径通过。精确命令、测试计数、失败原件与后续复验见 [verification](rp-function-verification-2026-10-01.json)。修改共享循环后旧 speech/context/narrative/privacy/recovery/interaction 等105项及78子项整组复验PASS138.301s；初次schema inventory未分类ObjectID的FAIL保留。现有预览库副本079→080保持201旧Events、15旧provider receipts、所有旧回合列和公开历史，重启守恒，无真实模型调用，见[升级回执](action-trigger-preview-upgrade-2026-10-01.json)。

本 runtime 没有新的 full-backend、原32、完整22 Golden PASS。旧原32已完成32轮/96成功决策/32真实 Narrator，但固定三听者的后置断言 FAIL；精确听者集合修正后同 DB/同 binary/零真实模型复核 PASS。原 FAIL 保留，不能叫本轮新32通过。

被放弃的“额外 draft review”试验实际仍有无依据对白，并出现 schema failure 与额外调用延迟；已经从生产实现移除，见 [失败证据](rejected-review-accuracy-2026-10-01.json)。单纯调整 packet 顺序/更高 effort 没有明确改善，见 [分区输入采集](context-presentation-accuracy-2026-10-01.json)与[诊断](accuracy-diagnostic-2026-10-01.json)。这些都不算体验 PASS。

下一项应该把已有动作入口逐条接入统一触发与后续引用，并为尚不支持的同地点走近/坐下明确返回能力边界。不要借创建者 zone 写入代替玩家移动，不为测试临时补椅子，不建立第二套世界状态。NPC own-action 的部分结构化目标、公共作者 presentation 向 NPC 的合规使用、非点头动作与对象反应、自由对白前提准确度、文学表现力和人工复评仍未完成。

R1 两出口继续开放；完成当前功能增量不代表进入 R2。
