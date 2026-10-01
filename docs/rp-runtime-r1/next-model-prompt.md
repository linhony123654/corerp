# 下一模型继续提示词

```text
继续 CoreRP RP Runtime R1。用户已委托架构/代码/文档修改，并授权既有GitHub分支与同一隔离预览发布；不得扩大到其他生产系统。目标是可信世界中连续、自然、可引用的角色扮演，两出口满足后停止，不自动进入R2。

先读 /home/ubuntu/corerp-preview-integration-20260929/AGENTS.md、docs/rp-runtime-r1/handoff-2026-10-01.md、rp-function-accuracy-2026-10-01.md、rp-function-verification-2026-10-01.json 和 .planning/2026-09-29-rp-runtime-r1 三文件顶部CURRENT。项目cwd不是/home/ubuntu。分支rp-preview-integration-20260929；核对git status/log/远端和release.json实际身份，不猜HEAD。

当前源码439个生产文件sha e2e7b01977b68a5c94b4a9561e6ba26667bfa20d0cb0954ae6c1247bd91c42ea，runtime7e8145d421fa0a8ab265af8037234652a80bc1f0b9c1165127a96791b81864ec，schema080。采集时线上还是feea/079；最新是否部署必须读独立发布回执，不能把构建说成已发布。保留19个无关untracked planning/截图，只stage明确本轮文件。

代码300597c26eeaef4089402a7a02aca253086fdb8d已精确发布、同一preview升级080并验证当前7e814；文档后续HEAD另看git。线上一真Step nod13.821s首试成功，原key/restart零重复，但仍自说“站惯了”“座儿留着”，语义NOT PASSED。看action-trigger-preview-release JSON，不能重做已完成发布或倒回79。

补充AUTO实际输入“我向贾母点头示意”，1真Step解释+1真Step NPC均首试成功，ACTION→nonverbal→canonical，22.783s，原key无重复。首harness误把解释回执查到rp_provider_calls而FAIL；其实际在rp_interaction_interpretations，同plan/key恢复复核，未重抽样。见action-trigger-auto-real JSON。只读审计没有said/private被转current_snapshot的代码证据；snapshot缺pose/seat/distance，respond.Text自由且basisID只证明可读，非每句蕴含。不要把“模型误读”说成查实的存储串位。

核心结论不是“底层全坏了”：真实断点是玩家动作提交后没触发NPC、现有witness字段投影丢失、语义hard gate误拒正常询名，以及自由speech的无依据前提。已做定向点头垂直闭环，只有这一类动作接入。其余原owner还可能只提交动作，不反应。不能把一条路线修好说成全部R1已完成。

原BuildRPDecisionInput仍唯一入口。当前单份分区character presentation/无损source-support v2压缩，131072B不变，最大128994B。061场景对象只读，未新增物件/NPC对象authority。移除known_relationship_introduction硬拒绝，介绍/familiarity/权限/来源照旧。nonverbal knowledge恢复现有action/gesture/target/time，只读角色自己的冻结observation，不从raw Event补不可见目标。

080原rp_turn_runs typed trigger：speech保留FK，nonverbal parent真实RPNonverbalAction Event，不造玩家speech/wait。目标NPC须实际frozen visual witness；旁观者不激活、external controller不代演。原RunRPTurn与RunRPNonverbalTurn共享finishRPTurn NPC提交/叙述/结算。continuation只容许同parent完整NPC batches。审计和provider receipt不能再假定parent只有speech。

NPC response仍v3 private/observable/grounding；private不上public。NPC speech/expression经原owner提交再叙述。自己动作来源自己receipt，不伪造自我witness；其他动作靠历史witness。Narrator沿用有限v2公开renderer，未放宽事实/自由prose。nod不提交agreement/pose/move。source handle存在不等于对白每个命题成立。

AUTO复用请求provider；Stop/Retire不抹已接受action；legacyraw同key不追补NPC；Play读取有turn_run_id的动作结果。六阶段恢复与HTTP/AUTO入口有证据。本版core/decision/fullHTTP77.274s/vet/frontendbuild通过，旧路径105项回归初次FAIL是schema inventory漏分类ObjectID，保留原FAIL并做显式不mask对象断言；当前完整相关复验PASS138.301s，105项+78子项，见verification。

真实step-5-preview对照：旧点头0NPC调用，当前1首试成功speech+smile、然后一轮能引用动作；重启/同key零新增effects/calls。首次采集FAIL是“历史总数必须为1”的harness错误，改成匹配action/no raw duplicate后同DB/runtime/key恢复，未再次抽样。7.659s是真生成，0.041s只是恢复读取。公开逐字页面/JSON targeted-nod-*。这不是冻结Golden或原32；人评PENDING，仍有care/listen/invitation模板和自由speech前提错误。

不要重新加入已失败并移除的额外draft-review模型回合；五臂重排/high-effort没有明确提升，失败证据均保留。不要盲跑大套来代替功能交付；先把已有动作逐条接入统一反应/下一轮可引用，再对未支持同地点走近/坐下明确能力边界。不得用creator-zone冒充playerauthority，不能补椅子canon、清历史、另建RP世界、堆人物Prompt特例或扩大owner/plugin。

自由对白准确度NOT PASSED，真正人评PENDING，R1 NOT DONE。当前没有新fullbackend/原32/full22 PASS。旧原32完成32/96成功/32Narrator后固定三听者postcheck FAIL，修正精确集合后同DB/binary零真模型后置PASS；不能借成新80的32通过。后续必须同条件Golden前后+人评，机器不能代替体验出口。

Go /usr/local/go/bin/go，backend cwd，GOCACHE=/home/ubuntu/.cache/go-build GOPROXY=off GOTOOLCHAIN=local TMPDIR=/tmp GOTMPDIR=/tmp，-p1、storage -dwarf=false、-ldflags='-s -w'；shm满/根盘紧，不删除共享DB/cache/证据。模型使用用户指定step-5-preview，凭证仅从ignored/private配置读取，绝不输出/提交。080写入后不要自动降reader或restore liveDB。发布前对现存预览库副本升级并核对heads/events/public-history/private boundaries。
```
