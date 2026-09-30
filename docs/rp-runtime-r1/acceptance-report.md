# RP Runtime R1 · 验收记录（当前完整32轮待最终复跑；RP Experience未通过）

<!-- experience-first-status -->
当前本地源码检查点为runtime `f4b63534`（798个源码文件），GitHub已发布审计基点为 `ab4580d`。唯一当前验证：[scene-activity-context-verification](scene-activity-context-verification-2026-10-01.json)，改动说明：[scene-activity-context](scene-activity-context-2026-10-01.md)。原Context Builder的活动列表之前未按感知过滤；有效RED已证明闭门另一侧活动泄漏。当前同一builder复用获准可见角色，附事件来源/快照依据，拒绝无来源及版本不一致，不扩owner/世界状态/Prompt链。最终定向1.743s PASS。首次全后端11个测试包PASS/1无测试、storage默认累计10m超时（旧测试初始化不到1秒，无阻塞栈）；同源storage30m复验4551已终态PASS（728.028s），合计12个测试包/1个无测试得到同源通过覆盖，首次失败保留而非整套单次exit0。原完整32轮82456运行中，仅替换runtime路径，断言/案例不改。仅原浏览器32任务未终态，不能签收World Integrity。

同一组已提交事实的真实Step叙事配置对照已采集：79字→96字、来源相同、世界/台词/决策不变，衔接仍机械，不能证明体验提升；[公开样本与采集限制](same-facts-prose-review-2026-10-01.md)。初次probe读错HTTP envelope的FAIL保留，公开render从持久化记录只读恢复，无额外模型调用；修正脚本未重跑。真实荣庆堂11人仍缺作者设定，人工Golden未取得。R1 NOT DONE；此增量作为独立审计快照准备，实际提交/推送状态以Git HEAD/remote为准，预览未部署。下方旧current/running均历史，以当前验证JSON为准。
<!-- /experience-first-status -->

<!-- context-selection-status -->
2026-09-30 当前增量：原BuildRPDecisionInput和initiative共用确定性64KiB选择与源/说话者去重，Life台词记忆也纳入；必要canon/owner事实不截断，缺失readiness不补原著。身份遮罩后重新选择，proposal先按actual provider view再按canonical/typed链验证。private及选择统计不进玩家/Narrator。

当前runtime `f6c9677840beaeb1100a215084cf1b37c8bbcebd8f52263504ba77d56910a67b`：core/decision/narrative、storage定向77.549s、五包vet、独立构建PASS。Step小样为双方各28轮mock+末轮真实Step，两次真实首试成功且都正确回忆旧约定；wrapper99807→65135字节、当前问题4份→1份，是去冗余证明，不能冒充体验/速度改善。完整backend90621终态exit1：只有旧Life重复Text断言失败，修订后精确words/actor/source/time与恢复检查PASS；完整storage复验80283已exit0/984.391s，合并首跑其余11包得到当前12测试包PASS/1无测试，初次失败如实保留。运行时源码与binary不变。全32 89506已exit1：8浏览器轮通过、9轮结算、26成功/1截断后120s timeout，9次真Prose成功，后置NOT REACHED。冻结Golden21660已exit0完成采集：17/17决策成功（16首试/1第二次）、G8真Prose首试成功、0expression；角色/关系/表现力仍无明确改善，RP Experience NOT PASSED。所有本轮任务终态。本轮未改UI、未部署/提交，R1 NOT DONE。

见[实现/边界](context-selection-2026-09-30.md)、[唯一当前验证](context-selection-verification-2026-09-30.json)、[Step读样](context-selection-pilot-review-2026-09-30.md)、[冻结Golden复评](context-selection-golden-review-2026-09-30.md)、[体验优先新路线](experience-first-route-2026-09-30.md)。下方conversation-focus、wire及更早current/running是各自历史检查点；f4d5051f全套不能覆盖本轮源码。作者canon和真人验收未取得，不推断祖孙关系，不进入R2/R3。
<!-- /context-selection-status -->

<!-- conversation-focus-status -->
2026-09-30 最新增量：公开交流对象延续已在原激活计划内实现（迁移076）。当前点名优先，再根据该玩家确实听见的上一轮实际回复/自己的等待回合主动开口延续，歧义稳定回退；不读取NPC私有信息，不创建第二套世界状态。legacy只调整顺序，保留全部听者调用数；deterministic定义保持不变。已覆盖另一观察者、未知身份、未听见、未提交proposal、多人歧义、目击离场、章节、世界时间、另一客户端、故障恢复/重建、实际外部接管与有旧计划的升级。

当前runtime `f4d5051f6b2e48779883c007df579508f2a4aa1f0fcc8021653e0b27a507d58b`：定向37.169s退出0、五包vet与npm build退出0。全backend36735已退出0，12个有测试包PASS（storage914.532s、HTTP71.400s），另1包无测试；真实Step两轮前后小样61116已退出0，4/4各一次success：旧版Nora→Iris且第二轮沉默，新版Nora→Nora回应，来源对应第一轮已提交且被玩家听见的台词。此为短段对象延续改善，不是长RP/文风/速度验收。原全32/冻结Golden证据保持历史，不覆盖到新源码。本增量的所有任务进程已退出；当前R1 NOT DONE，无commit/push/部署。

[实现及边界](conversation-focus-2026-09-30.md)、[当前验证](conversation-focus-verification-2026-09-30.json)、[小样](conversation-focus-pilot-2026-09-30.json)。以下decision-wire及更早的“当前/最终/running”段落均为对应历史检查点，以本块和验证JSON为准。下一步仍在R1：原builder统一优先级/去重/总预算、安全请求大小与耗时诊断，然后新的全32与冻结Golden/人工复评；作者canon未得到声明，不能补原著关系。
<!-- /conversation-focus-status -->


<!-- decision-wire-v3-status -->
2026-09-30 当前最终增量：现有 NPC adapter 已改为 v3 private + 单一 observable，严格分动作提供合法字段；修复同一 NPC expression 在 settled 回合与独立目击历史的重复呈现。完整 decision、HTTP到typed提交/恢复/第二客户端/重建、相关五包vet、两个真实Step合成协议小样PASS。

runtime `2e527072a0aafea8fd9e72c545d7f847f010d8fd5937cae407298b6f87085ff1` 的完整 `go test ./... -count=1 -timeout=20m` 退出0，12个有测试包通过（storage889.953s、HTTP69.189s），另1包无测试。初次全套因磁盘不足失败，清理可重建Go编译缓存后重跑成功；保留原失败日志。实际Step full32退出1，第2轮一次截断后120s timeout，5次成功/1次超时，后置检查未执行。冻结Golden采集退出0：17/17各一次决策成功、G8真实Prose首次成功、0 expression；关系/语气/表现力没有明确改善，还有无设定依据的“廊下有茶”台词风险。采集成功不等于RP通过。

[增量及剩余架构缺口](decision-wire-v3-2026-09-30.md)、[最终验证](decision-wire-verification-2026-09-30.json)、[32轮失败](decision-wire-32-result-2026-09-30.json)、[Golden复评](decision-wire-golden-review-2026-09-30.md)。所有本次任务进程已退出，下文f9c6769f等为历史增量。R1 NOT DONE，World Integrity实际32轮和RP Experience出口均未通过，作者canon/真人复评未取得。无commit/push/部署。

下一步限定在R1：统一原builder各窗口的优先级/去重/总预算，并依据既有公开交流延续激活对象；先做安全的请求大小/实际预算耗时诊断，不原样盲跑32，不增加Prompt特例。作者设定只能明确声明后提交；当前对话中的祖孙等候选不等于授权canon。不要进入R2/R3。
<!-- /decision-wire-v3-status -->

状态：**RP Experience Exit 未通过**。以下 Gemini 前后样本是冻结配置下的同模型可复核对照；随后改用 `step-5-preview` 暴露并修正了一处叙事事实边界漏洞，当前修订尚未通过完整 32 轮技术回归。不能把测试或本文件当作用户的人工 RP 验收。

最新实现和验证状态见[可观察事实契约修复](observable-contract-repair-2026-09-30.md)及[来源与验证检查点](observable-contract-verification-2026-09-30.json)。定向与privacy/restart检查、vet/build已通过，最终源码完整suite七包PASS（storage877.383s、HTTP69.733s）。前一增量实际Step全32在第28轮一次120s决策超时后exit1；27浏览器轮通过，83次决策成功、1次timeout、28次Full Prose成功，后置检查未执行。该二进制的冻结Golden已采集，15/17决策成功，G4/首个G5回退沉默，未生成nonverbal Event。最终源码Golden已独立采集，17/17决策成功、G8真prose首试成功；读样仍有盘问/拒斥/杂字，0手势及稀疏叙事，没有明确RP体验改善，详见 observable-contract-golden-review-2026-09-30.md。R1 NOT DONE。后文“最终状态”及其他running项均为各次运行的历史快照。

2026-09-30最终状态：完整后端内部套件通过；当前Step全32在第九轮一次120s决策超时后exit1，26次成功决策、一次timeout、九次Full Prose成功，后置检查未到达。[冻结Golden当前采集](step-current-golden-2026-09-30.json)17/17次决策成功、G8真实Full Prose首试成功，但仍有陌生人体验、格式残片、持续目标和表现力不足。详见[八场景读样记录](step-current-golden-review-2026-09-30.md)、[全32诊断](step-current-32-result-2026-09-30.json)。采集完成不是体验通过；真人评分和作者canon仍缺，不宣称R1 DONE。

## 证据与边界

- 冻结前样本：[before-samples.json](before-samples.json)（15 次玩家输入、17 次成功 NPC 决策，G8 Full Prose 成功）。
- Gemini 配对后样本：[after-samples.json](after-samples.json)，SHA-256 `924f55ba7b77ed2358d514b6c5161a3232b280174e9d8f926685af80fd3a9709`（同样场景与模型，17/17 次 NPC 决策成功、6 个可观察表情/动作独立入账，G8 Full Prose 成功且没有 fallback）。它是后续 Step 适配及叙事 guard 加强**之前**的代码。单次生成仍有随机性，前后的体验判断以重复出现的结构性问题为主。
- 两次均用 [冻结世界声明](real-rongqing-world-spec.json) 与 [冻结包](real-rongqing-packages.json) 生成隔离世界；它们不是生产世界状态。源世界 11 人的 persona 全空，未声明关系与称呼。宝玉与贾母的亲属关系**不能**从原著或角色名字中补入。
- 两次世界 head 分别为 52 与 58；后样本多出的 6 个序号正是独立提交的非语言 observable。世界输入与玩家刺激固定，运行后状态允许因新能力产生可追认差异，不把 head 相等当作体验对照前提。
- 比较模型与真实预览过去的 `gemini-3.8-flash` 不完全一致；可比较性仅限这两次配对运行。模型输出具有随机性，下面只记结构性趋势及可重复核查的文本。

## Step 5 Preview 补充诊断（不替代 Gemini 配对）

用户因 Gemini 额度不足要求改用 `step-5-preview`。中转 `/models` 列出该 ID，最小真实请求成功；这只能证明模型入口可用。冻结旧运行时的 1024-token Step 重放前四轮多数决策回退沉默，已停止，不能作为配对 baseline。当前 R1 的决策输出预算改为默认 1024、可配置 512–8192，遇明确长度截断最多加倍重试一次；引用不在授权 Context 中的依据 ID 最多重新提案一次，第二次仍无效就安全失败。两个修复均不绕过 schema、grounding 或 typed owner。决策超时仍有 2 分钟硬上限。

- [叙事边界失败样本](step5-diagnostic-before-narrator-guard.json)，SHA-256 `58e8771dc88c278d3e420b59e7cf0bf84623f643801fcebc0943852b0bc31090`：`step-5-preview`、4096-token/60 秒/low，17 次决策中 12 成功、5 超时；G8 在 NPC 沉默时把未提交的“你看着”“你不打算”写成玩家行为/心理，且错误地没有 fallback。该草稿已冻结为负例。
- [guard 修订后的完整 Step 样本](step5-after-diagnostic.json)，SHA-256 `dfd4d7ec40f2d03c6d53d9c92c277e16ecf5acd3acc12f56cbe17cb0c7b66276`：同一冻结世界、4096-token/90 秒/low，17 次决策中 15 成功、2 次 `incomplete` 回退沉默；G8 只保留玩家与 NPC 的已提交原话，无 fallback，也没有补玩家动作。两次模型输出有随机性，且配置/代码均变化，**不作因果改善百分比**。G3 仍称宝玉“不相熟”，G1 再次自报姓名；缺作者 canon 的结构性失败未解决。
- Step 样本之后又加入一次只针对 `finish_reason=length` 的有界高预算重试，定向测试通过；当前代码的再次短段 live 重放在 G6 两轮成功、G7 首轮仍超时，已停止，**没有完整 Step Golden 或 32 轮通过证据**。不把脚本退出 0、已结算回合或 fallback 沉默算作 NPC 决策成功。
- 首次 Step 32 轮浏览器回归在第 1 轮等待 `/rp/turns/run` 时超时，脚本 **exit 1**；隔离 SQLite 显示该轮随后 `settled`，3 个 NPC 决策回执均成功，叙事基础回执成功。Play 原本对整个模型回合只等 75 秒，而这次三个串行决策约用 114 秒；已按所选模型的单次超时及最多六次调用计算浏览器等待上限，并保留幂等续接。该运行没有完成 Full Prose 或后续 31 轮，不能算通过；修正后的 Step 短场景复核另记。
- 修正后 `node scripts/verify-rp3-npc-play.mjs --rongqing --pilot --prose` 用 `step-5-preview` **exit 0**，隔离证据 `/tmp/corerp-rp3-npc-0bsSKg/npc-samples.json`：3 个连续玩家回合、9/9 个成功 NPC 决策、3 次成功 Full Prose，重启后首轮对白仍在且末轮对白可见。第二轮袭人回应“二爷怕是忙昏了头，连我都没认出来”，第三轮接着提“方才还问我是谁”，并保留整理布帘的当前事务。这是有承接的短段，但其剧中关系仍来自该技术脚本的文本人设，不能替代冻结荣庆堂 G1–G8 的作者 canon 与人工验收；完整 Step 32 轮尚未重跑。
- Step 32 轮重跑通过前两轮，第 3 轮因一个 `silence/wait` 提案携带不相容效果字段而安全回退，脚本 **exit 1**；审计分类为 `proposal_noop_contains_effects`，没有把该提案提交为 observable。适配器现对这类动作/字段不相容提案给一次固定契约反馈，要求模型重新提出完整合法方案；它与私有证据引用修复共用一次无效提案重试预算，第二次仍须经过原硬校验。正反例和尝试次数测试通过。该修复之后的完整 32 轮结果尚待另记，不能把此次失败运行算作 World Integrity Exit。
- 上述无效提案修复后的完整 Step 重跑也已结束，`node scripts/verify-rp3-npc-play.mjs --rongqing --prose` **exit 1**，日志 `/tmp/corerp-r1-step-32-repair.log`、隔离库 `/tmp/corerp-rp3-npc-Cpm4Ui/world.db`。第 1 轮成功，第 2 轮三名 NPC 中 1 次成功、2 次分别重试后 `timeout`，脚本保持"不得以 fallback 冒充成功决策"的门槛而停止。只读库核查显示 2 个 settled 玩家回合、4 次成功及 2 次超时决策回执、4 次成功叙事回执；故障是 provider 在配置的调用预算内未返回有效结果，**不能据此断言世界内核出错，也不能宣称本轮 32 轮通过**。测试进程已终止。换模型后的结果必须另记模型和配置，不能与冻结 Gemini 配对样本混作同模型改善。
- 2026-09-29 晚（新编码助手会话）基于回执的诊断与复验：`config.Timeout` 是含重试的整次决策总预算，硬上限 2 分钟不变。最小探针证明 `step-5-preview` 先用推理 token 再产出正文：默认推理强度下 1024 预算返回 `finish_reason=length` 且 content 为空；同一严格 schema 加 `reasoning_effort=low` 后连续三次 `stop` 且结构合法（342–960 completion tokens）。决策配置本已支持 `CORERP_LLM_REASONING_EFFORT` 与 `CORERP_LLM_DECISION_MAX_TOKENS`，回归脚本通过 `process.env` 展开继承未列名变量，故 `low`+预算+120 秒属运行配置而非门槛放松。新增仅在 `CORERP_DECISION_DEBUG=1` 时输出的脱敏 stderr 诊断（类别/finish_reason/有无 refusal/长度，绝不含内容或凭据），回归 runtime stderr 现落入产物目录。适配器还为 private 元数据越界（`invalid_private_decision`）增加一次有界重提案反馈（仍共享一次修复预算、第二次从零硬校验），并在 decision schema 的 private 字段声明与 Go 校验一致的 `maxLength`/`maxItems` 以便严格结构化输出在服务端防超界；正反例测试通过，`go test ./internal/decision ./internal/core` 与 vet 通过。当前代码完整 `go test ./internal/... -count=1 -timeout=20m` **exit 0**（storage 844.6s、HTTP API 70.1s）。当晚 live 结果：`low`/2048/120s 三轮 pilot **exit 0**（9/9 决策、Full Prose 与重启检查通过，`/tmp/corerp-rp3-npc-Hw9EpR`，4 次截断均由既有加倍修复救回）；同配置完整 32 轮第 1 轮即因"截断→4096 修复→ungrounded 修复→第三次调用时 120 秒总预算耗尽"timeout 而 exit 1（`/tmp/corerp-rp3-npc-jFwBNj`）。`low`/4096/120s pilot **exit 0**（9/9，零修复事件，`/tmp/corerp-rp3-npc-iewDOG`），但同配置完整 32 轮在第 5 轮因一次 4096 截断后加倍至 8192 无法在剩余预算内完成而 timeout exit 1（`/tmp/corerp-rp3-npc-OBKChH`：5 个 settled 回合、14 次首轮即成功决策、1 次 timeout 回执、叙事全部成功，其余 14 次决策平均 36 秒）。结论：当前代码在两种配置下决策首轮成功率约 93–100%，但 step-5-preview 当晚推理长度抖动使单次决策偶发无法在 2 分钟硬上限内完成；失败均安全回退、世界账本与门槛行为正确。**当前代码的 32 轮通过证据仍缺**，不据任何一次中止运行宣称 World Integrity Exit。
- 2026-09-29 深夜续（2026-09-30 按原始回执更正）：`enable_thinking=false` 在最小探针有效，但真实负载未稳定抑制思考，仅保留为显式开关。六次 low/4096/120s 的完整尝试均未跑完：`1EKhAS`、`4qh8Ac`、`ngUc8j`、`MQov6v`、`YhE7Eo` 分别有一个 `timeout`；`qoZ2ql` 是两次调用后约 63 秒的 `failed`，stderr 依次记录 `ungrounded_decision` 和 `invalid_speech_fields`。这些目录均位于 `/tmp/corerp-rp3-npc-<后缀>/`，原日志 `/tmp/corerp-r1-step-32-retry.log`。因此撤回“六次全是超时”和“2 分钟上限是唯一结构性阻断”的结论。当前代码仍缺 32 轮通过证据；下一步应分别处理配置一致性、延迟与提案合法性，不能只增加超时或反复完整重跑。

## 2026-09-30 无外部额度修复

网页自选模型可显式传递 `reasoning_effort`、`disable_thinking`、`decision_max_tokens`、`interaction_max_tokens`；角色决策的 HTTP override 超时上限与适配器共同使用既有 120 秒上限，不再额外压为 60 秒。推理选项在 NPC、AUTO 和分阶段输入解析间共用请求装配；不从模型名猜默认配置，不继承其他端点的凭据或选项。旧配置保留默认行为。实现与本地验证见 [无额度推进记录](step-offline-followup-2026-09-30.md)。本次未调用真实模型，不能据此声称 Step 已稳定或长对话体验已通过。

## 2026-09-30 上下文与回应编排修复

本段是随后继续修复的独立增量，包含真实 Step 调用；上一段的“未调用”仅指网页参数传递的本地增量。

- 在唯一 builder 内增加 `relevant_dialogue`，按本轮词面主题取 NPC 自己说过或确实听见的较早互动；最多 512 候选、4 组、6000 Unicode 字符，原话与 Event ID 完整保留，不让模型摘要或补 canon。43 轮后的旧互动检索、未听见秘密排除、稳定 hash/重启、预算与近期去重测试通过。
- schema 的 `basis_event_ids` 枚举只含获准 Context 中的 Event，包含检索出的旧源；对白声明与世界校验一致的 2000 字符上限。本地 grounding/action 校验和一次共享无效提案修复预算保留。非法 speech 的诊断只含长度和字段存在性。
- 原编排激活计划现在使用玩家自己的已声明关系称呼；定向“师傅”可识别，反向“队长”不能误用。既有 responder cap、外部 owner、计划恢复不变。相关 storage 回归与 vet 通过。
- decision/core/narrative 完整包、相关 storage 测试、HTTP 模型 override/叙事定向检查通过。完整内部套件 exit 0，storage 880.737s、HTTP 70.349s（日志 `/tmp/corerp-r1-internal-20260930.log`）；它在下面的引号修复之前启动，修复后的 narrative 完整包/vet 与 HTTP 叙事检查单独通过。
- 真实 profile 三轮 pilot 在第三轮被会话中断，没有脚本 exit-zero 证据。原隔离库 `/tmp/corerp-rp3-npc-BsU1gL` 七次决策成功；相同 SHA-256 二进制随后续接同一回合，保留已有一人决定、补齐另外两人，总计九次成功 NPC 决策，玩家原话不重复、旧 Event 不改写，并成功渲染第三轮。`interrupted-recovery.json` 仅作为恢复证据；被中断调用的回执继续标记结果未知，不能从它推断实际 HTTP 次数。

这个增量证明较早原话可按权限和主题进入决策，不能证明语义记忆、持续意图或人物味已完成。私有意图的跨轮延续、统一上下文预算和未点名时自然接话仍未解决。细节及后续顺序见[架构复核](runtime-architecture-review-2026-09-30.md)，冻结 before/after 未改动。

随后 Step profile 全 32 轮尝试在第一轮 Full Prose 安全回退后 exit 1：三次 NPC 决策均首试成功，叙事两次校验失败。实际已提交的黛玉原话内含 `「有人嗎」`；以同种外引号完整包裹原话，旧校验器确定性误报新增对白。修复统一的配对/嵌套扫描后，合法原话与混合引号通过、错配拒绝，正文外招手/付款/新增对白的负例仍拒绝。用原回合生成新的叙事变体，一次真实 Step 请求成功且原 Event 不变；公开文本与回执见[复核 JSON](step-nested-quote-recheck-2026-09-30.json)。原失败草稿未记录，所以完整拒绝原因仍未知，不能把这次随机新输出当作所有旧失败的因果证明。下一次完整32轮的第二轮失败另记如下；**当前 World Integrity live32 及 RP Experience 出口尚未通过**。

该完整重跑实际在第二轮叙事回退后 exit 1（`/tmp/corerp-rp3-npc-f5UJN9`）：六次 NPC 决策均成功，第一轮 Full Prose 成功。新增同回合公开输入诊断，两次均拒绝为 `speech_changed`。源 NPC 原话含疑似 JSON 残片 `},"`；表现模型不能自行删改已提交原话。本次将公开对白改为局部引用槽，由服务器按引用逐字插入；不同 consumer 的权限、源事件、typed owner 与事实校验均未放宽。未知/重复/缺失引用和正文外新增事实负例仍拒绝，字面引用不递归执行；当前 narrative/core、相关 storage、HTTP 定向检查通过。一次新的实际 Step 变体按三个引用插入原话成功，原 Event 不变，证据见[对白引用复核](step-speech-reference-recheck-2026-09-30.json)。疑似残片原样保留，**NPC 产出不自然的问题没有被解决或隐藏**。最终代码内部套件与第三次本增量全32都已结束，结果如下，不宣称R1 DONE。

最终代码完整 `go test ./internal/... -count=1 -timeout=20m` 已 exit 0，storage 871.580s、HTTP 69.665s，日志 `/tmp/corerp-r1-internal-quotes-20260930.log`；它包含对白引用与嵌套解析增量。最新全32exit1，隔离库 `/tmp/corerp-rp3-npc-IxAIZV/world.db`：前八轮浏览器检查通过，第九轮settled但有一条两次尝试后的120s timeout；26条成功决策，九次Full Prose均成功。再生/混合/故障/重启块未到达；不得把安全fallback当作成功，也不能据此断言账本损坏。这个包测试结果不替代真实模型长段或人审出口。最终相同二进制的Golden采集17/17成功，零非语言事件；当前文本与待真人复评项另存[当前Step读样](step-current-golden-review-2026-09-30.md)，下面的表继续描述历史Gemini同模型配对。

## 场景复评

| 场景 | 冻结前 | 当前后样本 | 判断 |
| --- | --- | --- | --- |
| G1–G3 贾母与宝玉 | “只是我瞧着你眼生得很”；“想我了？…记不得在何处见过你” | G1 问“你究竟是哪一位”；G2 重复“我是贾母”；G3 说“我实在认不得你是谁” | **未解决**。关系/称呼未入世界 canon，前后均把玩家当陌生人。G2 记住了近轮自我介绍，却没有关系依据。 |
| G4 未证实的旧事 | 不承认未证实的“前儿那桩事” | “我并不记得同你说过什么事” | 知识边界保持；没有把玩家的说法当成已发生旧事。 |
| G5 当前事务 | 连续催玩家交代来意 | “我可没工夫同你在这儿东拉西扯”；下轮“既然无事，便请回吧” | 有承接但仍反复追问身份；没有可靠活动/人设支持的当下事务，人物味薄。 |
| G6 多人 | 两人自报姓名，但继续追问玩家从哪来 | 两人分别直接答“我叫晴雯／袭人”；首轮袭人沉默 | 回答更直接，但仍欠缺稳定角色关系和目标。 |
| G7 茶的可追认边界 | 明确否认递茶 | “我并没有给你递过茶”；后样本的 6 个非语言事件各有独立 Event | 没有把对白/提议冒充已完成递茶；新动作可追溯。 |
| G8 Full Prose | “你在荣庆堂里开口道…贾母…随即回应道…”两行 | “你：「找老祖宗呀」”；“贾母：「找老祖宗？我便在这里……」” | **部分改善**。同一模型下不再重复转述说话动作，原话保留、无 fallback；仍只是直接对白，缺可用表情事实与公开人设，尚谈不上有味道的戏。 |

基于固定 rubric 的暂评：知识边界与部分连续性为正向；身份/关系/称呼在当前 canon 下应标 `N/A` 或未准备，不能按小说常识给分；G8 节奏比冻结前干净，但表现力仍不足。玩家是否愿意继续玩、贾母是否“像人”，仍需用户或其他真人按 [冻结 rubric](golden-baseline.md) 评估，不能用本记录替代。

## 实现与验证状态

- Context Contract：`BuildRPDecisionInput` 现包含版本、世界 head、persona 来源、定向已声明关系与称呼、readiness；原有亲耳对白、知识和活动仍按观察边界进入。未声明为 `UNKNOWN`，空 persona 为 `MISSING`，不让模型把自身常识当 canon。Narrator 使用单独的公开已提交事实投影，不能读取 NPC 私有意图。
- Decision Contract：提案分开简短私有意图/情绪/关系姿态、证据事件引用，以及可观察 speech/action/expression。私有部分只留在受限提案/审计/不可变已应用决策中，最多三个本人过去决策进入下一次本人 Context，绝不进公开事实或 Narrator。可观察表达仍须通过 typed owner 的新鲜 head、input hash、合法动作和目击校验。
- Observable Boundary：NPC 对白与支持的表情/手势写入同一原子 batch 的两个事件，目击者、知识、自己动作、outbox 和重放按现有 `RPNonverbalAction` 规则投影。NPC 台词无论本轮还是跨轮，都需要玩家的 `speaker_said` 观察记录才进入 Narrator；表情也需要目击事件。活动、移动和可见的沉默则按**事实发生时**的移动、区域选择及视线通道事件重建该玩家的视野；后来的站位变化不会改写旧回合。远处喊话的未听见答复、隔门完成的活动、可听但不可见的沉默均有定向测试。
- Narrator：新增已提交 expression 的公开事实与自然动作词；模型仅可组织节奏、句式、连接和已公开信息的表现。Studio 世界可显式声明 `public_presentation`，作为已识别且本轮有公开事实的角色的措辞线索；它有 `StudioWorldPrepared` 来源，独立于 NPC 私有 `persona`。返回视图仍逐一引用已提交事实，保存的 render 另附公开风格来源，不把风格当成新发生的事实。若本轮至多两个公开事实且只有对白/沉默/等待、没有动作或公开人设，长文请求的**有效密度**降为对话优先，输出长度也受公开对白量约束；原世界风格声明不变。有更多公开证据时保留较长叙事。仍禁止新增人物/物品、位置变化、玩家行动、私有心理、未提交动作，以及改写已接受对白。Step 实际失败样本促使 guard 新增对玩家未提交目光、意图及重复绕话的拒绝；定向回归证明原样本被拒、短促已提交沉默可呈现。中文自然语言 guard 是有边界的安全网，**不是**对所有隐性事实创造的形式化证明。
- Hard：typed legality/owner、head/input hash、证据引用成员资格、已知关系下可精确识别的自我介绍、expression 词表/可见性、可观察事件授权、对白逐字保留与部分明确场景事实。Soft：答非所问、复读、多人轮流说话、人物味、风格与趣味性；复读修复只请求重新提案，不改世界事实。
- 叙事表达校验现在还要求表情对应**同一个**已提交角色；不能借用另一人已入账的“招手/点头”等动作。多人测试验证了这条硬边界。无法确定主语时回退或重试，不能把模糊叙述当作事实。
- 早期后样本曾 17/17 次 HTTP 400 回退：Gemini 不接受空字符串枚举值。已改为结构化输出 `none`，入口归一化为空动作；最终后样本 17/17 决策成功。该失败样本不计入体验成绩，但保留作为适配教训。
- 首次 32 轮回归在第 26 轮因一名 NPC 提案回退沉默而按脚本硬门槛停止，前 25 轮全部三人决策成功，已提交事实未回滚或串改。模型错误当时仅记录为 `provider_failure`，不能无证据断言具体原因。现已补充有限、脱敏的 provider 错误分类，并允许 `silence/wait` 附带可提交的无言表情；最终第九次回归通过。
- 第二次回归通过了前次第 26 轮，但第 29 轮两次模型调用后回退。审计错误分类为 `other`，结合调用数与现有 provider 路径，归因于重复台词启发式“改写一次仍相似即拒绝”。该分类本就应为 soft signal；现改为最多请求一次改写，第二个合法提案不再因相似度单独被阻断。旧测试的硬拒绝预期同步改为 soft，最终第九次回归通过。
- 第三次运行的 32 个正常玩家回合、后续混合杯子动作和故意 provider 故障注入均完成；临时库有 34 个 settled turn、102 个成功 NPC 决策回执、3 个故意失败回执、15 个可观察动作事件。脚本末尾以 UI“最近 50 段世界记录”含第 1 轮为断言，在长对话中误报失败；数据库确认首轮和末轮对白均各有一条。断言已改成“重启后首轮 accepted speech 仍在账本 + UI 显示末轮对白”。第四次运行的 32 轮也完成，却在再生步骤误取最后一条 NPC 表情记录；现按真实玩家回合 ID 定位。
- 第五次 32 轮脚本 **exit 0**；证据 `/tmp/corerp-rp3-npc-PoP263/r9-checks.json`。SQLite：34 个 settled 回合、102 次成功 NPC 决策、3 次故意失败回执、18 个非语言事件、1 个重新生成后选定的 render。重生成保持 world head、混合杯子动作生效、“继续剧情”未入对白、故障后已接受对白仍在、重启后已选展示仍在。该运行构建二进制后，叙事校验又收紧了多角色表情归属，所以单独又完成了最终代码的第九次回归。
- 第六次最终代码运行的 32 个正常回合全部完成（96/96 决策成功），但随后的叙事重新生成在两次模型尝试后安全回退，UI 正确保留原展示；旧脚本误等“展示已更新”而超时。回执分类仅为 `prose_unavailable`，不能凭此判定是中转故障还是未分类的草稿校验失败；相同回合/设置在隔离库中再次生成了新 render。现把未提交 expression 草稿明确归为 `prose_validation_failure`，回归脚本仅对 `prose_unavailable`/`prose_timeout` 作最多两次额外尝试，并检查每次回退均未改变世界 head 或既有展示；已知事实校验失败不进入重试。
- 第七次运行在第 26 轮出现 `prose_validation_failure`：三名 NPC 的决策成功提交，但两稿 Full Prose 均被事实校验拒绝，正式叙述安全回退为模板。该回合包含袭人的已提交微笑和宝钗的已提交点头，属于多人表情归属敏感场景。没有放松角色归属硬规则；第二稿反馈现要求把每项表情明确归给 facts 中的 actor，含混主语则省略。定向 mock 测试验证先拒后修；在第七次的隔离库对同一回合和同一模型做真实 Full Prose 重生成，第二次模型尝试成功取得 `rpr_` render，无需新世界事实。
- 第八次最终代码运行的 32 个正常回合全部成功，包括先前失败的第 26 轮。脚本在后置再生时试图从 Chromium DevTools 读取流式 NDJSON 响应体，浏览器返回 `Network.getResponseBody: No data found`；此为测试工具读取方式错误，无法据此宣称再生失败。现在改由不可变 `rp_provider_calls` 回执判断生成结果，UI 仍须证明新 render 实际显示；第九次完整脚本通过。
- 第九次此前代码的整套 `node scripts/verify-rp3-npc-play.mjs --rongqing --prose` **exit 0**，模型 `gemini-3.8-flash-high`，证据 `/tmp/corerp-rp3-npc-63jweS/r9-checks.json`；这是先前阶段的通过记录，后来叙述听觉边界和目光校验又发生了变化。
- 历史视野过滤后的此前代码重新执行相同 32 轮脚本 **exit 0**，证据 `/tmp/corerp-rp3-npc-dQfdd2/r9-checks.json`。32 个连续正常玩家回合均收到三名 NPC 成功决策与 Full Prose；SQLite 中共 34 个 settled 回合、102 次成功 NPC 决策、14 个非语言 Event。混合杯子行动、元指令不入对白、故意无效密钥造成的 3 次失败回执、再生新 render 且无 fallback、重启后对白/世界/展示恢复均通过。回归定义下的 World Integrity Exit 记 **PASS**；它不是中文可见性语义的形式化证明。
- 公开风格来源接入后的一次完整 32 轮回归在第 26 轮停于 `prose_validation_failure`，NPC 决策和表情事实均已提交。只读核查确认袭人的微笑、宝钗的点头各有独立事件。对该隔离库的诊断模型草稿显示：“宝钗保持沉默，只是点了点头”和“宝钗保持沉默，对你点头”被旧校验器错误地归给不明确的主体或玩家。修正为同一句内按明确句首主语跨逗号承接、将“对你”识别为受事，并仍拒绝另一角色借用手势；定向正反例通过。同一失败回合在复制库上用原模型重生一次，取得无 fallback 的 `rpr_` render，世界 head 不变。此失败运行**不计为**最终 32 轮通过；最终代码重跑另记。
- 公开风格/主语归属改动后的 32 轮先前曾 **exit 0**，证据 `/tmp/corerp-rp3-npc-3qaVL1/r9-checks.json`；随后叙事密度调整与技术脚本的软诊断分类使这份证据成为历史记录。
- `go test ./internal/... -count=1 -timeout=20m` 在公开风格来源接入后完整 **exit 0**（storage 852 秒；HTTP API 70 秒）；之后仅修改 Narrator 主语归属和证据密度，以及技术脚本的语义诊断分类。完整 `internal/narrative` 套件及相关存储/叙事定向测试、`go vet ./internal/core ./internal/narrative ./internal/storage`、脚本语法与 `git diff --check` 通过。最终 Golden 后样本 17/17 次决策成功、6 个独立非语言 Event，G8 无 fallback；体验出口仍未通过。
- Step 适配及叙事 guard 后，`go test ./internal/... -count=1 -timeout=20m` 再次完整 **exit 0**（storage 845 秒；HTTP API 70 秒）。这次全套运行后仅调整了 decision 适配器对 refusal/tool-call 的检查顺序及 Play 请求等待窗；前者的完整 decision/narrative 定向测试与 vet 通过，后者的 Vue 类型检查和 Vite 构建通过。上述结果不替代当前代码的 32 轮 live 回归。
- 一次对白/动作载荷拆分试验在冻结 G8 上仍产出“你开口道”，且草稿出现未提交的“看着你”；试验已撤回。由此加强目光校验，但没有把该试验当作 RP 表现改善。后来对同一已提交 G8 回合试读三种密度，`standard/long` 继续转述说话动作，`concise` 直接呈现对白；按公开事实量调整有效密度后，隔离库连续三次同模型重生均为直接对白、无 fallback、head 不变。完整冻结 Golden 后样本也呈现相同结构。这个修复改善节奏，**不能**凭两句对白生成不存在的场面感；人工 RP 评分尚未由人完成，R1 不可标记 DONE。
- 叙事密度改动后的首次 32 轮重跑在第 22 轮被旧脚本的“未点名 NPC 绝不能插话”断言停止。隔离库记录袭人插话称“宝姑娘这会儿可不在这屋里”，同时宝钗就在场并随后回答。这是明显的角色体验问题；它只是有归属的 NPC 台词，未成为世界位置事实，也未改变 Narrator 事实。轮流发言属语义判断，不能作为世界内核的硬门槛。脚本现把直接点名的回答与其他插话逐字写入 `turnTakingDiagnostics` 供人工复评，事实/提交/恢复硬检查不放松；新的完整 32 轮运行另记。
- **Step 适配前代码**的 `node scripts/verify-rp3-npc-play.mjs --rongqing --prose` **exit 0**，模型 `gemini-3.8-flash-high`，证据 `/tmp/corerp-rp3-npc-mRHtpJ/r9-checks.json`。32 个连续正常回合全通过；数据库有 34 个 settled 回合、102 次成功 NPC 决策、15 个独立非语言 Event。混合杯子动作、元指令未混入对白、3 次故意 provider 失败回执、再生成新 render 无 fallback 且 head 不变、重启后已选叙事恢复均通过。当时的 World Integrity Exit **PASS**；当前代码新增 provider 预算/有界重提案与稀疏叙事 guard，不能把该历史运行称作当前代码的 32 轮通过。本次点名宝钗/黛玉均由本人回答、旁人沉默，`turnTakingDiagnostics` 保留原样；前一次袭人的错误插话并未因此自动判为已修复。第 27 轮黛玉又把玩家含糊的怀念说成“不是真记着什么”，属于需人工判断的角色/玩家心理推断风险，不是世界事实。

## 剩余风险与下一步

当前最关键阻断是源世界没有经创作者确认的 persona、宝玉/贾母关系和称呼规则。要评估“贾母认识宝玉”这一核心命题，须先明确写出这些设定并作为新世界的源事件；随后在相同设定与模型下重跑前后对照。当前 G1 里“找老祖宗？我便在这里”是已提交的**角色话语**，不构成祖孙关系已由世界确认；NPC 仍可能说出未经证实的身份/关系主张，这类语义问题尚不能靠现有精确 hard gate 完全杜绝。即使有 canon，G8 仍需在事实边界内获得足够的公开场面依据，并经真人阅读确认。

活动与移动的可见性现由已提交历史事件确定，而非额外写入每个玩家的观察记录；这个推导必须继续接受跨区域、移动与重放场景的回归审查，不能因少数定向测试通过就宣称所有叙事可见性已形式化证明。公开风格字段必须由创作者只写入可向玩家展示的表达习惯；如果创作者误把秘密写进该字段，运行时无法替作者判定秘密。现有长期上下文增加了有来源的词面相关旧对话检索，但没有语义相关性排序、跨轮未完成意图链或统一总预算；转述、超出512条候选的旧事与多人接话仍可能丢失关键上下文。现有动作词 guard 也不可能穷尽所有中文隐性动作，应继续用实际失败样本审查，而非宣称自动语义证明。比较模型是 `gemini-3.8-flash-high`，不能将它的结果直接等同于预览历史上记录的 `gemini-3.8-flash`。

继续 R1 所需的最小创作者声明与真人复评栏位见[设定与复评单](canon-review.md)：先确认这个世界是否采用原著关系，再明确贾母和宝玉各自的人设、贾母→宝玉的关系角色、称呼与自称，并补本次 Golden 涉及的其他 NPC 必需资料。上述值应成为有来源的世界声明，不由模型从角色名字猜出。声明后可对**新世界输入**重放同样场景、请真人按冻结 rubric 读戏评分；原始旧 runtime 不接受新增设定字段，故新设定结果不得冒充其同输入 old/new 对照。真正的同设定双运行时比较需要单独构造并标明兼容对照版本。
