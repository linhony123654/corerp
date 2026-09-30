# CoreRP R1 接续提示词

若用于独立审计，直接复制 [dot-audit-prompt](dot-audit-prompt.md)。继续开发则复制下面内容。

```text
继续 CoreRP RP Runtime R1。用户允许改变原实施顺序：先可读真实短对话，再针对结构性失败修改既有运行层，候选成立后做完整世界回归和Golden/真人验收。保留canon优先、事件账本、typed owner、恢复与private/observable边界，不扩R2/R3、插件、无关owner或UI重做。

目录 /home/ubuntu/corerp-preview-integration-20260929
分支 rp-preview-integration-20260929；GitHub https://github.com/linhony123654/corerp
用户已授权本次审计快照提交/推送；未授权合并或部署。先git status和git rev-parse HEAD读取实际状态，保留所有本地改动，禁止reset/rebase/强制checkout。文档中的7fc3b3b是Git审计比较基点，不是声称最新HEAD。

读AGENTS.md，再读：
docs/rp-runtime-r1/conversation-default-verification-2026-10-01.json
docs/rp-runtime-r1/conversation-default-review-2026-10-01.md
docs/rp-runtime-r1/dot-audit-prompt.md
796源码hash已冻结复核；runtime 0f2664e83218e22d9e11588ca5f90bf093cbc3f0bd7dc22d9179f1a2d702cf6e。
最新五文件：Studio默认系统包orchestrated/1/version1.1.0，hash包含策略，旧/导入包保留；原focus查询允许已提交wait/silence沿用其原激活计划中个人听见的speech来源。等待不能建立交流，不能复活点名新对象未获回应前的旧对象，仍遵守chapter/time/place/witness/controller边界。不是第二套RP状态/owner，不强制发言。

所有本轮任务终态，没有待poll句柄。定向focus/activation串行15.009s、backend build、Studio创建与Vue/TS检查PASS；并行首跑SQLite初始化FAIL保留，原因未确认。不要说根因已证明是盘满/底层坏了。
原真实八轮 conversation-default-live-2026-09-30.json FAIL保留：第3轮wait，第4轮stable_fallback且source为空，碰巧仍选Nora。修复后 conversation-default-after-2026-09-30.json PASS：同输入/初始设定/包/Step配置，8真实首试成功/0mock，5无点名回合有明确来源，Nora5→Iris2→Nora1、每轮1激活，旁听/未激活记录保持。复跑没有再选wait；等待修复由确定性连续wait/silence、restart/rebuild和负例证明。两组公开读样仍有机械笑/点头；旧样“热的还温着”是未经物品动作提交的台词风险。不要把合法发言等同于台词主张真实，也不能把承诺直接执行成物品/动作。

原BuildRPDecisionInput是NPC唯一入口，readiness/persona/关系称呼/实际heard/relevant/own private/activity/head/source保留，provider view遮罩选择后canonical/typed链验证。未知关系可当陌生人；persona或已有关系必要称呼缺失在HTTP前NOT READY，不从原著补canon。完整V3 private+one observable，短src_N只绑定本包获准真实事件，未知/重复/alias+raw重复拒绝。同一正式schema进入消息与传输；tool_call单函数或finish=stop完整JSONcontent同解析，不接受自由台词/半截/额外效果。private短句不是长推理，不进Narrator/Event/公开观测。玩家解释器沿已有闭合schema和候选/typed提交，不读取NPCprivate。诊断只有限类别/形状/count/hash，不持久化提案或推理原文。

当前完整后端和当前完整浏览器32未最终复跑。d329原32主段全部通过96NPC/100attempts/0decisionfallback/32Play真Prose，但整套后置混合物品解释失败exit1；STORAGE_FAILURE为解释错误通用包装，quick_check ok，不是这次数据库损坏证据。edf从冻结库副本做真实API后置续验2解释+6NPC首试通过，混合实际杯子/原话一次/精确重放/继续非台词/重启事实与已选叙事保持；不能改原失败或冒充当前完整浏览器32。

冻结荣庆堂11NPC仍缺作者人设/关系/称呼；d329复评17notready/0NPC HTTP，不是RP改善证据。四个冻结文件禁止覆盖：real-rongqing-world-spec.json / real-rongqing-packages.json / before-samples.json / after-samples.json，hash见当前manifest。新Nora/Lin/Iris世界只是明确测试作者世界，不能代替真实canon。canon-review.md只是待填写非权威表；作者设定和真人评价问题已问未答，不重复问/推定同意。不把助手读样、自动机器绿或8轮当作真人/长期RP/表现力验收。

下一步先根据dot具体审计findings定位最小结构改法。重点现有当前活动与静态persona的冲突、private意图连续性、speech主张与共享事实边界、相关记忆对长对话是否足够，不再堆单句prompt或盲目新增调度器/状态系统。每个增量须源hash/binary hash和定向证据。体验候选成立后一次最终完整世界回归/原32和可比较authored Golden，真人签收两个出口后停止R1。

模型用户指定step-5-preview，low/4096/120s、显式tool_call。endpoint/key仅从 /home/ubuntu/.local/share/corerp-preview/env 在内存读，不能输出/写文档/提交。Go用 /usr/local/go/bin/go；临时和缓存走/dev/shm（先确认空间，避免并行大构建/SQLite测试）。不能删除用户库/源码/证据。source/runtime在manifest中；未部署本轮backend，旧预览fea2067a，前端服务路径未核实。

R1 NOT DONE。不要以推送审计快照表示功能完成，不自动进入下一阶段。
```
