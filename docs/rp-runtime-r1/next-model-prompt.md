# CoreRP R1 接续提示词

复制以下提示词；最新证据优先于历史记录中的运行句柄和 runtime。

```text
继续 CoreRP RP Runtime R1。项目 /home/ubuntu/corerp-preview-integration-20260929，分支 rp-preview-integration-20260929；先核对 git status/HEAD 和 AGENTS.md。当前阶段优先解释与修复已实现功能为什么不准确，不默认新增长期目标系统，不推翻可信世界内核。

先读 docs/rp-runtime-r1/architecture-accuracy-2026-10-01.md、architecture-accuracy-verification-2026-10-01.json、architecture.md、audits/2026-10-01-tasks.json 与五份审计最后的修复/复核结论。本轮基点739a3ad。精确命令和源码fingerprint见verification；旧unblock-diagnosis及32/Golden/Step样本是历史证据。

已经修复：
1. NPC provider view以短事务读取同一head/familiarity/alias，typed exact person字段投影，原话/片段/来源ID/private主观文字不再被substring替换。真实view policy=v2，最后commit仍重查head/hash/owner。
2. 原builder优先恢复完整authorized split-recent问答，不依赖字面匹配；RecentContext通过normalization/provider view，selection v2按整组预算，不能用半条recent回复代替放不下的整组。未听见/跨branch/head内容仍排除。
3. 新fresh Narrator采用fact-composition.v1，模型只引用整条事实、选有限template和分组；服务端绑定说话人/动作/target/refusal/逐字原话。这是显式披露的有限准确性基础，尚不能自由文学改写或提供角色措辞，不能冒充R1体验改善。旧saved prose保留unversioned读法。
4. 未发布的077记录canonical/variant独立fact-only来源序列+groups；每个reload路径比对独立序列，canonical read另核重建facts；重启拒绝unknown/duplicate/reorder groups，style来源不计作事实。fresh/cached stream的文本、来源组、capability提示一致且cached不再调用模型；turn replay也保留限制提示。

本轮core/decision/narrative完整包、定向storage/HTTP、legacy迁移/restart/所有durable-stage恢复和vet PASS；零真实RP模型调用。新source未跑完整backend/原32/冻结Golden，未部署，R1 NOT DONE。再次核对在线后端PID722907、binarySHA fea2067a；前端实际服务版本未核实。不要把GitHub推送写成已经部署。

冻结荣庆堂11个角色含玩家1/NPC10；10NPC缺作者persona，关系/称呼未声明。最近历史Golden17 NOT READY、0NPC HTTP。不能按《红楼梦》常识补canon，也不能把独立测试作者候选替换冻结原world/sample四文件。最近原32第29轮deadline失败，剩余/postchecks未到；保留该失败。人工体验验收未取得。

下一步只选高信息量的体验路径：在明确作者测试fixture内检查真实authorized packet与固定短对话的关系/称呼、问答资格、拒绝/动作下一轮引用，再决定source-safe expressive contract。已有private last3只是历史sketch，general goal lifecycle暂缓；old lexical retrieval和上游遗漏诊断/SQL成本有明确限制。机器不能将任意knowledge/relationship语义视为绝对可证；不要通过不可靠语义gate阻塞正常RP。

继续保留typed owner、ledger、world验证、private/public、并发/重试/恢复和模型配置边界。先复现具体承诺再修，在必要局部检查通过后停止重复测试。原32与可比较Golden+真人rubric仍是最终出口，完成验收报告后停止，不自动进R2/R3。

Go /usr/local/go/bin/go；本机/dev/shm已近满，TMPDIR=/tmp GOTMPDIR=/tmp，保留现有GOCACHE=/dev/shm/corerp-r1-private-bounds-gocache-20260930；不得清掉共享证据/DB/cache。需要时降低debug构建体积并串行compile，不删断言。多个不相关旧planning目录及shots基线图原本就untracked，保留不提交。

真实RP provider使用用户指定step-5-preview；凭证只从ignored配置内存读取，不输出/记录/提交。是否有额度应以当次条件为准，不用真实模型盲跑大套。GitHub branch发布有既有授权，部署是不同工作；文档记录本增量尚未发布运行服务。
```
