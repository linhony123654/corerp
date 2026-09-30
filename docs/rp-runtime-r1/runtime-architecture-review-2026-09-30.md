# RP 运行层架构复核 · 2026-09-30

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

结论：需要更新 RP 上下文和决策编排，现有事件账本、typed owner、观察边界和恢复链继续保留。不能把所有体验问题归结为模型，也不能把所有模型超时归结为架构。

最新增量见[可观察事实契约修复](observable-contract-repair-2026-09-30.md)：原公开 fact 丢失互动对象，任意移动还能许可相反方向/姿态，NPC wait 还会许可时间推进，均已先复现再修复。定向 core/narrative/storage、重建/重启及 vet/build PASS；最终源码完整suite七包PASS（storage877.383s、HTTP69.733s）。前一增量 Step 全32已在第28轮 timeout 后 exit1（27浏览器轮通过），其 Golden 15/17决策成功仍有陌生人、格式残片和表现力问题。最终源码Golden采集完成（17/17决策、G8真prose成功），但读样仍有盘问/拒斥/杂字和稀疏叙事，无明确体验改善；真人/作者canon gate 仍未通过。下文较早结果均为历史证据。

## 当前事实与改造位置

| 路径 | 代码现状与影响 | 更新方向及本次状态 |
| --- | --- | --- |
| `BuildRPDecisionInput` | 最近 16 条发言、40 条玩家摘录、40 项自身动作分别按时间保留；旧互动即使有账本记录，也可能不在本轮上下文 | 本次新增有来源的相关旧对话选择，仍由同一个 builder 装配；没有新建记忆数据库。后续还需统一各类上下文的优先级、去重和总预算。 |
| `RPDecisionProposal.Private` | 有私有意图、情绪和关系姿态；现已从本人不可变已应用决策中选择至多三项历史，含原对象/时间/来源 | 当前 head、完整 batch、本人 owner 和获准提案 hash 校验，不读审计候选，不进 Narrator。已完成本人过去意图延续视图；未完成/已兑现/已失效的生命周期仍未实现。 |
| `rp_turn_listener_activations` | 已有 deterministic/orchestrated/multi_agent；此前编排模式只识别熟人名字，未点名时按稳定 ID 选人。冻结荣庆堂包没指定模式，走 legacy | 当前在原激活计划内按本人已声明称呼/熟人姓名、公开交流延续、稳定回退排序。延续只来自本人实际听见且已提交的交流；076增加可审查来源，旧计划不改写。另一观察者、未提交/未听见、章节/目击离场、歧义和恢复/接管测试通过。legacy保留原调用数；原著、反向关系和NPCprivate不提供别名。短Step对照及全backend已终态通过，未宣称长对话体验通过。 |
| decision adapter | 字段约束与本地校验存在差距；模型可能先编造 basis ref，用掉修复机会后又返回不合法效果字段 | 本次让 schema 的依据引用枚举来自授权 Context，声明 2000 字对白上限；本地硬校验仍执行。新增仅布尔/长度的无效对白诊断，下一次失败才能区分空白、过长和附带动作字段。 |
| Narrator | 独立读取公开已提交事实，表情/动作可追认；事实稀疏时仍难形成丰富场面 | 保持公开投影。表现力依赖已有 observable 和作者公开风格；不能为改善文风绕过事实边界。本次未放宽事实约束。 |
| Narrator 引语解析 | 同一种引号嵌套时按首个闭引号截断，合法原话被误判成新对白，剩余引语还可能进入正文动作检测 | 本次统一为配对栈解析；只剥离完整的最外层引语，错配/未闭合仍拒绝。合法内引语可完整保留；正文外新增动作仍拦截。它修正的是言语/事实边界解析，没有放宽 observable 权限。 |
| Narrator 对白保真 | 原来由模型重抄不可改写的原话，第二轮再次回退；同回合新诊断两次均 `speech_changed`，NPC 原话本身含疑似 JSON 残片 | 本次给公开 speech fact 一个局部 `quote_token`，模型编排引用，服务器单次插入完整原话并加引号；缺失、重复、未知引用拒绝，生成后的正文继续经过原事实 guard。文本不能由表现层静默修复。不自然的 NPC 原话仍属体验问题。 |
| readiness/canon | 真正冻结世界的人设和关系为空，模型没有可靠依据把贾母当作宝玉的祖母 | 继续显式 incomplete；补充设定必须来自创作者。祖孙候选已向用户请求澄清，尚不能当作确认过的世界。 |

## 本次 Context 更新

`relevant_dialogue` 是一组较早的原话片段，含发言者、时间和源 Event ID。构造顺序是：

1. 在当前 branch/head 内取最近至多 512 条 NPC 自己说过或具有个人听觉证据的已接受对白。
2. 依据源事件的 session/parent turn 分组；每条发言都独立通过听觉过滤，不能因某轮有一条可见发言就读取该轮全部内容。
3. 使用本轮玩家原话里的词和汉字相邻词片段做确定性匹配，降低候选集中高频片段的影响；不假装能够理解同义转述。
4. 排除已经进入 recent window 的互动，最多取 4 组、合计 6000 个 Unicode 字符，保留完整单条原话和顺序。遇超预算组跳过，不截成看似完整的引语。
5. 保留稳定排序，同一 head 的重读/重启得到相同 input hash。身份遮罩和 basis 引用集合沿用现有 provider view 与 grounding gate。

它证明的是“谁说过什么”，不证明旧对白的说法是真的，也不将承诺转成已兑现的行动。它只进入 NPC 私有上下文，不自动进入 Narrator 或玩家上下文。

## 当前证据

- `TestRPRelevantDialogueRecallsOldExchangeWithHearingAndRestart`：43 轮后找回第一轮请求和 NPC 回应，此时它们已经离开 recent dialogue、player history 和 own actions；未听见的同题秘密排除，同 head 多次读取及重启 hash 一致。
- 检索预算、完整引语、近期去重和无匹配不返回内容的定向测试通过。
- decision/core/narrative 完整包测试、相关 storage 决策/叙事/可观察动作测试、vet 通过。
- 同输入授权 Event ID 枚举（含检索出的旧 Event）、空依据场景、既有非法引用拒绝/有界重试测试通过。
- `TestRPTurnActivationDirectAddressIsBoundedAndRestartStable`：规范名与重启计划保持原验证，玩家对 NPC 的“师傅”称呼可选中目标；反向“队长”不误作玩家称呼。相关执行模式、observatory、检索测试与 storage vet 通过。
- Step 网页 profile 三轮试验使用 low/4096/120s，不开启尚不可靠的关闭思考开关。该进程在第三轮被会话中断，原脚本没有完成，不能记作 pilot PASS。隔离库 `/tmp/corerp-rp3-npc-BsU1gL` 已有七次成功决策和一条结果未知回执；随后用 SHA-256 相同的运行时在同一世界续接第三轮，保留已有决定、补齐另外两人，玩家原话仍只有一份、原 Event 内容不变，Full Prose 成功。证据 `interrupted-recovery.json` 记录 `kind: interrupted_profile_recovery`；这证明该次恢复链，不能替代 32 轮或 Golden。中断回执保持未知，不伪造成功或失败。
- 完整 `go test ./internal/... -count=1 -timeout=20m` exit 0（storage 880.737s、HTTP 70.349s）；启动时尚未包含后续引号修复，该修复后的 narrative 完整包、vet 和 HTTP 叙事定向测试另外通过。
- 首次当前代码 Step 网页 profile 32 轮尝试 exit 1，隔离库 `/tmp/corerp-rp3-npc-HnoIYR`：第一轮三次决策各首试成功，Full Prose 两次校验失败并安全回退。原草稿没有留存，不能把全部原因断言为引号问题；但用实际已提交的黛玉嵌套原话构造合法正文，旧解析器确定性报“新增对白”，修订后的正反例通过。
- 引号修复后在同一回合使用空 style override 生成新叙事变体：一条新 provider receipt/一次真实 Step 请求成功，世界 Event 内容不变。最初无 override 的复核只是读取原已保存叙事，调用数为零，不能作为新模型失败或修复验证；已更正临时证据的标签。可审阅的新回执和公开原话见 [嵌套引语复核](step-nested-quote-recheck-2026-09-30.json)。该次新调用通过本地仅公开叙事数据的诊断代理，中转服务、模型与超时保持一致；不是精确的网络延迟比较。
- 引号修复后的完整 profile 32 轮在第二轮叙事回退后 exit 1（`/tmp/corerp-rp3-npc-f5UJN9`），六次 NPC 决策首试成功，第一轮 Full Prose 成功。后续同回合新诊断复现两次 `speech_changed`；拒绝类别不包含原话、模型草稿、地址、ID 或凭据，只有显式 `CORERP_NARRATIVE_DEBUG` 开启时才输出。原 NPC 已提交文本尾部含 `},"`，两个新草稿改写了输入原话；原脚本的两份失败草稿没有保存，所以不推断其所有错误。
- 对白引用与服务器插入的定向测试通过：真实 wire packet 给出来源绑定的 quote token；完整保留疑似残片；额外对白/招手/付款仍拒绝；引用不得缺失、重复或未知，原话内的字面 token 不递归执行；旧的合法直接引语继续按原 guard 验证。新实际 Step 变体首试成功，世界 Event 不变；公开证据见[对白引用复核](step-speech-reference-recheck-2026-09-30.json)。这证明不再依赖模型重抄该轮对白，没有证明 NPC 文本变自然。
- 最终代码完整内部套件exit0（storage871.580s、HTTP69.665s），日志 `/tmp/corerp-r1-internal-quotes-20260930.log`。Step profile全32实际exit1：前八个完整浏览器回合通过，第九个回合结算，但一名NPC两次尝试后达到原120s上限。共26次成功决策、1次timeout，九次Full Prose全成功；后置再生/混合动作/故障/重启块未到达。配置low/决策4096/解析4096/120s，日志 `/tmp/corerp-r1-step-profile-32-references-20260930.log`，持久结果见[全32诊断](step-current-32-result-2026-09-30.json)。故障注入脚本现会替换浏览器测试密钥，但本轮未执行到该步骤，不能计为实际通过。
- 相同最终二进制的冻结Golden已采集完成：17/17成功决策（3次用了两次尝试）、一次真实Full Prose首试成功、零非语言行动，head52；没有改canon。贾母仍追问身份，王熙凤有一条格式残片台词，持续目标和正向动作延续未体现。公开样本与读样依据见[当前Golden](step-current-golden-2026-09-30.json)、[读样记录](step-current-golden-review-2026-09-30.md)。模型与原Gemini配对不同，不作因果改善百分比；真人复评仍缺。

## 提供方参数复核

2026-09-30 阅读官方 [Step 5 Preview 模型说明](https://platform.stepfun.com/docs/zh/guides/models/step-5-preview) 与 [Chat Completions API](https://platform.stepfun.com/docs/zh/api-reference/chat/chat-completion-create)：该模型明确支持 `reasoning_effort` 的 low/medium/high；预算字段公开列为 `max_tokens`，当前兼容适配器发送 `max_completion_tokens`。本页未列 `enable_thinking`。这不证明中转忽略或不支持这些字段，也不证明关闭思考可稳定生效；只能记录参数与兼容层仍需独立实测，不能据官文缺字段归因所有截断。当前运行不改配置，原预算与重试上限保留。

随后六次最小实测中，两种字段在强制长回答时均于64 completion tokens返回`length`，有对应HTTP200/长度/usage记录。结果不支持“中转忽略当前预算字段”假设；不改字段或增加提供方特例。这仅证明这些64-token探针的行为，不证明长段延迟已修复。安全记录见[字段探针](step-token-parameter-probe-2026-09-30.json)。

## 尚未达到的出口

这轮已修复上下文检索、关系称呼激活和对白保真，但尚未证明语义记忆、持续意图或多人接话足够自然。当前修订完整32轮失败；冻结Golden完成了采集，人工体验验收仍未通过。词面检索可能漏掉转述、找不到512条候选之前的内容；NPC疑似输出残片仍原样可见，不能把表现层的事实保真当作台词质量提升。NPC/叙事的实际文本质量必须通过真人读戏检验。

后续架构工作应留在 R1 的现有路径内，按场景证据收紧：

1. 创作者明确最小人设/关系/称呼后，建立有来源的独立就绪场景；冻结的缺数据世界保持原样。没有该输入，角色关系的体验分无法成立。
2. 为同一 Context 明确优先级与总预算：本轮刺激、人设/关系和权限边界优先保留；近期、检索与自身行为按 Event 去重，预算裁剪须显式且确定，不把截取片段当完整原话。当前各窗口仍各自限量，没有统一实现。
3. 本人已应用私有意图视图已实现并通过重启/重放定向验证；它是过去决策，不是已完成行动，也不是自动续期的当前目标。未完成/完成/取消状态不得靠自由语义匹配认定。
4. 公开交流对象延续已在已有激活计划内实现；当前点名仍覆盖旧偏好。自然插话只作体验信号，不能用语义分类器禁止所有非目标发言。当前短对照和完整回归已终态通过，仍需长对话及Golden复评证明实际体验；它不是万能语义指代器。

最小提供方契约核对已完成，本人意图/主动动作的具体修复给出了新回归理由；当前修订的完整验证正在运行。之后保留新Golden公开样本和可检查的辅助读样，再等待作者canon及真人rubric。仍不增加插件、世界机制或第二套RP状态。
