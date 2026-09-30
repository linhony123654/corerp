# RP 场景活动：同地点不等于角色知道

本轮发现并修复原 `BuildRPDecisionInput` 中的一个事实入口缺口：人物列表已按视觉感知过滤，活动列表却直接读取同地点全部进行中或最近结束的活动。闭门另一侧的NPC、未目击的结束结果都可能进入模型上下文。即使 provider view 隐藏了陌生人的名字，活动本身仍然泄漏。

一个通过正常创建、感知声明、角色分区、typed `act` 和回合提交的隔离测试复现了问题：观察者未见活动开始，原 canonical 和 provider packet 都包含了该活动。首次有效 RED 为0.693s；更早两次是测试配置错误及链接空间不足，分别保留，不能当作产品行为证据。

修复在原装配链中共用 `VisibleEntities` 的感知结果，先过滤获准条目，再执行10条场景上限。角色可以读取当前看得见的进行中活动，以及自己有来源的近期结束记录。他人未目击的结束状态不能因为后来可见而成为已知。当前活动owner没有记录他人结束目击，本次没有编造这一证据，也没有新增世界机制；自己的结束记录与 `OwnActions` 继续保留。

`RPSceneActivity` 增加 `source_event_id` 和 `observation_basis`。`current_visibility` 的时间是当前获准快照时间，不暴露未见过的开始时间；`own_action` 采用该角色自己的事件时间。来源必须与当前head、actor、activity、地点、事件类型及状态一致。字段为原Context Contract的增量，不是新的Prompt Builder或平行RP状态。

检查覆盖闭门不可见、进入同一区域后的合法当前观察、隐藏结束后再见不补历史、自己的结束结果、重启/重建，以及来源被改名和版本截断的硬拒绝。provider-view身份遮罩继续执行。没有改变NPC输出schema、private/observable提交、Narrator、owner、重试、deadline或数据库migration。

最新检查状态以 [scene-activity-context-verification](scene-activity-context-verification-2026-10-01.json)为准。定向基础测试通过后，扩大回归碰到SQLite初始化失败。诊断只在测试overlay中增加安全错误类型/代码，生产 `core/errors.go` 保持原样：本次实际为 `SQLITE_FULL`（13），临时分区最低约1MB。清理可再生成的本任务编译缓存后，当前798文件源码的首次全后端复验已有终态：11个测试包通过、1个无测试，storage触发默认累计10分钟超时。超时时正在初始化的旧测试才运行不到一秒，没有发现阻塞栈；历史完整storage约12–16分钟，因此只对这个包显式设置30分钟上限复验，728.028s终态PASS。结合首次其余11包PASS，当前源码12个测试包均有通过结果；这不是整套单次exit0，首次FAIL仍保留。backend/README已要求30分钟，产品deadline及断言保持原样；数据库、binary、源快照与失败证据保留。这个诊断不追溯替代旧报告中未确认的根因。

R1 NOT DONE。当前活动事实的权限改善不证明角色味、动态目标、长期语义记忆或文学表现力已通过。真实荣庆堂作者设定与真人Golden验收仍缺；当前原完整32轮已在第29轮因Step120秒超时退出，后置检查未到；冻结Golden17次NPC NOT READY/0模型HTTP，体验未通过。所有本轮任务已终态，完整结论见[验收报告](scene-activity-acceptance-2026-10-01.md)。此前推送的 `ab4580d` 保持为可独立审计的固定提交，本轮为它之上的独立审计修复，提交/推送状态以实际分支HEAD为准。
