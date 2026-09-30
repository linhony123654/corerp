# 最新检查点：角色就绪诊断与语义重写修复

结论：优先改角色数据就绪与RP模型适配层，保留已有可信世界提交链。用户授权调整推进顺序后，已做同一runtime的两组四轮真实对话，并修复了重复启发式拖慢合法回答的问题。R1整体仍未通过两个出口，未部署。

## 证据与判断

[短对照](role-readiness-probe-review-2026-09-30.md)使用f6c96778旧runtime/同一Step配置和四句输入。缺设定组连续强调“我们还不熟”；显式虚构测试设定组能安慰、陪伴并在末轮回顾承诺。8/8回合成功结算、9次真实HTTP模型请求、0mock；改变的是作者输入，不是运行时，不能声称新架构因果改善或荣庆堂已通过。

就绪组末轮第一次HTTP30.777s后，`repetition`信号触发第二次54.830s请求，回合共85.698s。两次均finish_reason=stop。第一份候选未公开保存，不能评价其具体文字；代码确认候选已经通过proposal合法性校验后才进入质量启发式。信号虽名为soft，却可在总预算内导致一次合法回答超时，属于执行策略问题。

世界内核方面，本轮定向来源/隐私/提交/恢复检查继续通过，没有证据要求推倒账本或owner。先前f6c96778全32第9轮是截断后超时；本修复解决另一个已确认原因，不承诺消除全部输出截断或模型超时。

## 实际改动

- `backend/internal/decision/reply_quality.go`：重复检测返回diagnostic，提供空重写反馈。格式残留提示继续使用已有一次有界修复。
- `backend/internal/decision/chat.go`：只有提供了修复反馈的质量提示才触发重写；opt-in日志注明rewrite_allowed。合法、带来源的重复回答直接返回提交链。
- `backend/internal/decision/repetition.go`：检测算法保留，说明改为只诊断。没有为“你刚才答应什么”增加词表或Prompt特例。
- `backend/internal/decision/repetition_test.go`：覆盖回顾承诺、其他问题、重复拒绝三类；迟缓的额外请求会超时，因此确实验证诊断不能制造第二次调用。完整原话、private依据及core校验保持。

NPC schema、统一Context/预算/实际provider来源gate、owner/world/head验证、private/observable隔离、Narrator、重试次数和120s上限均无变化。语义自然度信号应帮助诊断，不承担世界真伪判决。

## 验证

RED日志保留：`/tmp/corerp-r1-repetition-diagnostic-red-20260930.log`，旧代码三类均两次调用后timeout。修复后完整decision/core包PASS（0.462s/0.115s）。storage五项定向PASS7.909s，覆盖过滤上下文/提交恢复、失败非法输出安全沉默、记录原因不制造效果、实际遮罩来源gate及initiative private；HTTP endpoint路由/共享超时两项PASS1.077s。decision vet通过。

独立server构建通过，runtime SHA `cf104b95ada1f158f271b7e6ea4c26381eec2700b7cefca281204cf2ab258909`，二进制`/tmp/corerp-r1-repetition-diagnostic-runtime-20260930`。初次构建因VCS stamping exit128失败，按已有隔离构建用途使用`-buildvcs=false`，初次失败日志保留；源码/HEAD/二进制哈希另记。当前验证见[验证JSON](semantic-diagnostic-verification-2026-09-30.json)。

没有在这个局部修复后重复16分钟全storage或盲跑32。f6c96778的12包完整覆盖PASS是此前检查点；当前源码有上述定向证据，新的全后端/32/Golden未运行，不能把旧结果冒充新源码完整验收。真实短对照也在修复前f6c96778采集，所以不宣称85.7秒已被真实线上验证为更短。

## 下一步及交接

采用[体验优先路线](experience-first-route-2026-09-30.md)：先完成一个明确设定角色的可检查连续对话。角色就绪入口与现有builder的数据装配优先；由新的具体失败决定是否修改模型呈现，源码中宽工程JSON/长指令是否是主要体验原因尚未被此次数据对照证实。作者声明仍待回复，[荣庆堂候选](canon-review.md)未采纳，不按原著名字补设定。

剩余风险：Step仍可能慢/截断；窄格式提示仍可能有误判；有界候选和词法检索不覆盖任意长对话；最近三项私有意图不是完整长期目标生命周期；Narrator真实Golden仍仅两行对白，0expression；自然语言知识/关系含义没有全面可靠硬检查；没有真人rubric签收。原32及冻结Golden未通过的[九点验收记录](context-selection-acceptance-2026-09-30.md)和[Golden读样](context-selection-golden-review-2026-09-30.md)保留，状态R1 NOT DONE。

工作树保持原用户改动，分支rp-preview-integration-20260929/HEAD7fc3b3ba7be229b11341130d6cdd4275e231274f；无commit/push/部署或后续R2/R3扩张。预览服务没有更新本轮代码。
