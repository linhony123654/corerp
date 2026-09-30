# RP 创作入口：当前增量与交接

最新续验：当前代码四轮真实Step已成功完成，见[公开读样](role-ready-current-review-2026-09-30.md)。完整后端首次因磁盘耗尽失败后转内存临时目录重跑，当前真实32也在运行；实际句柄/终态见验证JSON。本文件以下是创作入口刚完成时的检查点，不把当时“全套未跑”当当前状态。

本轮修复了从创作到角色决策的数据缺口：原 Studio 表单只提交人物名字，已有 persona/关系/称呼声明能力没有被入口使用。现在新草稿要求填写 NPC 人设，声明相识时分别填写双方关系身份和称呼；未声明的关系保持未知。没有从《红楼梦》等模型常识补 canon。

工作目录 `/home/ubuntu/corerp-preview-integration-20260929`，分支 `rp-preview-integration-20260929`，HEAD `7fc3b3ba7be229b11341130d6cdd4275e231274f`。工作树包含大量先前修改，不干净；本轮没有 commit、push、部署或修改真实荣庆堂。最新 runtime SHA-256 `ac0af1f8a94d64a7b5b12890af8225200af2a6c3a4bdb47983030757b762a3d0`。源码、二进制、日志与冻结 hash 见[验证记录](role-create-verification-2026-09-30.json)。

## 数据与边界

- `StudioCreate.vue` 与 `studioCreate.ts` 使用现有 `StudioWorldSpec` 字段；私有 NPC 人设、可选玩家背景和供 Narrator 使用的公开措辞风格分别保存。双向关系各自填写，不推导反向角色；切回“未知”后不提交隐藏旧字段。输入沿用既有长度、Unicode 和控制字符限制。
- `StudioRPReadiness` 是纯粹的创建配置完整性报告。成功回执保留原世界 `status=ready`，另附创建者可读的 `rp_readiness.status=READY/INCOMPLETE`、NPC 缺项及已声明关系的称呼缺项。未知关系合法；可选公开风格缺失不阻塞。报告不含 persona 原文，也不是新的世界状态、实时 readiness 或 RP 质量分数。
- 旧 API 请求仍能保存并恢复，但空 persona 被显式报告为缺失。原 Studio owner 提交声明后，现有 `BuildRPDecisionInput` 从实际提交结果读取角色数据；没有新增 Context Builder、owner、迁移或模型补设定路径。
- Narrator、proposal、knowledge、private/observable 与原子提交权限保持。玩家可以引用的新增动作仍必须先提交；角色意图不因创作表单或叙述变成公开事实。
- 上一增量的重复语义信号仍只诊断，不再触发同步重写；窄格式修复及非法 proposal/source 验证保留。本轮没有再扩大模型权限。

## 验证

当前源代码的 core 完整包、Studio 创建/配置装配/故障恢复/HTTP 重启定向检查、三包 vet、原生 TypeScript 创建配置检查、脚本语法和前端类型检查/构建通过。

浏览器创建流程通过：真实 Create API、字段入库、丢失创建响应后的服务重启与同请求重试、回执 readiness、清除凭据后拒绝迟到响应、篡改包拒绝且无写入、恢复原请求、独立玩家授权、多世界选择、手机溢出检查、丢失开局响应后的幂等恢复、公开回放、NPC 对话、移动/等待、行政世界不变、会话恢复、无凭据持久化、无页面异常。

该流程使用 deterministic decision/narration，证明完整提交链和可操作性；不是新代码的真实 Step、长对话或人工体验验收。`--interaction-isolation` 未运行，不冒充通过。

初次浏览器检查暴露关系选择框可访问名称包含 option，已添加明确标签。后续截图失败暴露固定 Play 布局的正文高度为 0，以及 resize 后尚未稳定；验证改用实际视口、等待两个绘制帧，Studio/Inspector 仍拍整页。另发现既有世界观测组件的 UI 入口漏接，已恢复一个原风格按钮，无 Play 重设计。所有失败日志保留；最终运行退出 0。

为避免低磁盘空间下重复生成同一二进制，最终验证脚本在临时文件中硬链接先前此次构建的四个 Go 程序。仅替换模块路径、根目录和 build 行；全部产品流程与断言保留。原脚本仍默认从源码构建，最终临时脚本也单独记录 hash。

## 当前出口与下一步

**R1 NOT DONE，未部署。** 新源码完整 backend、真实 32 轮、冻结 Golden 均未重跑。f6 检查点的完整后端包覆盖通过、真实 32 第 9 轮截断后超时、Golden 17/17 决策成功但体验未改善，均为历史证据。不可把旧 PASS 覆盖到本轮。

此前虚构 Nora/Lin 的同模型对照表明明确设定组能够安慰并回顾承诺，缺设定组持续强调不熟；8/8 回合成功、9 次真实 HTTP，使用旧 f6 runtime。见[公开对照](role-readiness-probe-review-2026-09-30.md)。它支持先解决数据供给，但不是本轮速度或荣庆堂体验通过的证据。

下一步限定为一 NPC、4–6 轮设定齐全的真实 `step-5-preview` 对话，保留玩家实际可见文本并人工复评；若仍机械或超时，再改现有 decision adapter 的上下文呈现。短对话成立后进行长回归及冻结 Golden；不先扩 owner、插件、记忆系统或世界机制。真实荣庆堂仍缺作者声明，已问的 canon 问题不重复询问，也不把本轮调整方案的授权当作原著设定批准。四份冻结样本保持原 hash。
