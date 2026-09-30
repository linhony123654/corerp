# CoreRP R1 接续提示词

独立审计使用 [dot-audit-prompt](dot-audit-prompt.md)。继续开发复制以下内容。

```text
继续 CoreRP RP Runtime R1。先阅读真实终态，不把推送当完成。保留canon优先、ledger、typed owner、恢复和private/observable边界；不扩R2/R3、插件、新世界机制、无关owner或UI大改。

目录 /home/ubuntu/corerp-preview-integration-20260929
分支 rp-preview-integration-20260929
GitHub https://github.com/linhony123654/corerp
功能源码已远端核实8acd85f51fe06630d4db9c24e48e0ce11048be4b，之后是结果文档；主审计快照ab4580d，整轮比较基点7fc3b3b。先git status/rev-parse HEAD确认实际状态，保留所有改动。用户授权审计提交/推送，未授权合并/部署/reset/rebase/强制checkout。

先读AGENTS.md与docs/rp-runtime-r1/scene-activity-acceptance-2026-10-01.md、scene-activity-context-verification-2026-10-01.json、scene-activity-context-2026-10-01.md、dot-audit-prompt.md，再读scene-activity-32-public-2026-10-01.json、scene-activity-golden-samples-2026-10-01.json及same-facts-prose-review-2026-10-01.md。所有本轮任务TERMINAL，没有待poll句柄；不要恢复旧running段或盲跑全32。

798源码/四个冻结Golden文件hash未变；runtime f4b6353465fab223d3f0df19e48819805a781d3124a873d3d197ac8318006e21。唯一NPC入口仍是BuildRPDecisionInput。最新五代码文件增量按canonical VisibleEntities过滤SceneActivities后limit10，只给当前可见进行中和自己有来源的结束历史；source_event_id/observation_basis/head明确来源、当前快照不披露未见开始时间，也不由后来可见推定他人隐藏结束。无新owner/状态/migration/Prompt链/输出schema/重试deadline变化；有效RED及最终两项定向PASS1.743s、restart/rebuild和负例保存。

后端12包同源覆盖PASS：首跑其余11包PASS/1无测试，storage默认累计10m超时；按已有30m要求复验PASS728.028s，初次FAIL保留，不能说整套单次exit0。本轮另次SQLite由测试overlay证明SQLITE_FULL code13，生产errors.go未改；不能倒推旧所有STORAGE_FAILURE均盘满。

原完整browser32终态FAIL：28轮通过/29settled，86NPCsuccess+1两attempt后120s timeout/silence，总93attempt，29真实Prose成功。原no-fallback断言失败；30–32及混合物品/meta/fault/最终restart未到。quick_check ok不能代替后置检查。不得删案例/断言或用旧续验拼新PASS。

冻结实际荣庆堂采集exit0，16记录、17NPC not_used/rp_context_not_ready/0NPC模型HTTP，G8一次真Prose但只有玩家对白/陌生人沉默；作者canon与真人rubric仍缺，体验未通过。canon-review.md只是非权威待填表；不按名字/原著补设定、不反复询问已问未答的问题。real-rongqing-world-spec/packages/before-samples/after-samples不能覆盖。新设定overlay单独标记；旧binary不接受新字段，不能把换数据后的产品验证说成同输入架构改善。

Nora/Lin/Iris同输入八轮after8/8首试、0mock、每轮1激活及明确延续来源是独立测试作者世界；after未选wait，具体wait修复由确定性测试证明。两现有style同事实真实render79→96字、世界/台词/决策/来源不变，仍机械；首次probe误读HTTP envelope的FAIL保留，公开持久化render只读恢复无追加请求，修正helper未重跑。均不是长期RP/真实Golden/真人通过。

既有V3 private+one observable/短src_N/正式schema/自己的已应用private历史继续用actual provider view及canonical/typed链验证。private不送Narrator；合法台词中的承诺/主张不自动创建物品或动作。可靠字段/引用/权限/actor/head/来源/owner/replay/visibility是硬边界；重复、漂移、答非所问、自然度/趣味是soft或人工，正则不是完整语义证明。

下一步先接dot具体findings，检查静态persona任务与动态活动、意图连续性、公开表达、台词主张/物品、旧对话检索。最小结构改法先定向验证，再冻结源码/binary、完整原32、可比较Golden/真人验收。不要堆单句Prompt或建平行RP状态。

用户模型step-5-preview，low/4096/120s/tool_call；endpoint/key只从/home/ubuntu/.local/share/corerp-preview/env内存读取，不输出/写文档/提交。Go /usr/local/go/bin/go，TMPDIR/GOTMPDIR/GOCACHE用足量/dev/shm，完整测试timeout30m，保留库/源码/binary/证据。原生GitHTTPS本轮失败；GitHub Git数据库API备用发布可精确保留blob/tree/commit SHA及+0800时间，force=false，远端确认后才对齐tracking。不能改写历史或关闭TLS验证。

R1 NOT DONE，两个出口未通过。旧预览后端仍fea2067a，本轮未部署、前端实际服务路径未核实。停止扩功能，不自动进入下一阶段。
```
