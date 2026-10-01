# CoreRP 临时公网试玩（更新于 2026-09-29）

入口：`https://code.linhony.xyz:4188/`。这是隔离的**试玩预览**，不是生产发布，也不作为 F3 live 模型验收证据。玩家凭证在服务器 `/home/ubuntu/.local/share/corerp-preview/player-token.txt`（仅本机用户可读），不写入仓库或网页；前端输入后只保留于当前页面内存。

- 页面：从当前工作树 `npm run build` 后，仅把 `dist/` 复制到 `/var/www/corerp-preview/`；Nginx 独立监听 HTTPS 4188，使用现有 `code.linhony.xyz` 有效证书。配置在 `/etc/nginx/sites-available/corerp-preview`，符号链接在 `sites-enabled`。它只将同源 `/api/` 代理到本机服务，不开放 Vite 开发服务器或项目源码。
- 后端：`corerp-preview.service` 运行在 `127.0.0.1:4189`；使用独立的 `world.db` 与本地随机玩家凭证。自 2026-09-27 Phase J 起，运营方默认配置为 `CORERP_DECISION_PROVIDER=chat_completions` 与 `CORERP_NARRATIVE_PROVIDER=full_prose`，两者使用预览专用中转配置；模型只能提交受封闭 schema 和世界规则验证的提议，不能直接写世界事实。凭证/光标密钥与模型密钥位于同目录 `env`（权限 0600），不写入仓库、网页或日志。单独的 API 鉴权和精确浏览器来源限制保持启用。
- 玩家自带模型（2026-09-26 起）：玩家在「模型与 API 设置」里设为当前生效的配置会随行动/叙述请求发送 `model={endpoint, model, api_key, timeout_seconds}`，服务端按请求在内存中构建 provider（decision 或 narrative style planner），不落盘、不写日志；配置无效返回明确错误而非静默回退到确定性模式；不带 `model` 的请求保持运营方默认。该能力仍需鉴权与精确 Origin 许可；当前后端还要求模型端点位于运营方允许的 API 路径及主机范围内，因此只向受信任的人发放玩家凭证。
- 服务已启动、故障会重启，并已 `systemctl enable corerp-preview` 设置开机自启（2026-09-26 起）。独立试玩库与既有 F3 工作树、用户世界互不混用。只打算给受信任的人提供玩家凭证；有凭证的人可在这份隔离世界里改变时间与故事。
- 2026-09-27 已部署 living-stage 更新：多人物激活、类型化场景对象、授权世界观测、显式执行模式和 SillyTavern 生命周期支持均包含在当前二进制/前端中。后台世界推进保持双重 opt-in；本预览未设置 `CORERP_BACKGROUND_INTERVAL`，因此不会在无人操作时自动推进。
- 这次升级兼容曾使用旧 E 分支 042–044 迁移标识的预览库：启动时将其归一化到 057–059，并幂等补建历史 own-action 投影。上线数据库通过 `quick_check`、外键检查和事件数守恒检查；迁移前备份位于 `/home/ubuntu/.local/share/corerp-preview/backups/20260927-1511/`，旧静态站点保留在 `/var/www/corerp-preview.prev-20260927-1511/`。
- 2026-09-27 的部署后热修复同时处理两条恢复链路：旧 own-action 的空 `place_id` 不再阻断 NPC 决策；`full_prose` 流的每个片段都携带已提交 Event 来源，且服务端提供方超时调整为 30 秒，早于浏览器 45 秒边界，以便上游缓慢时及时回退。相关备份位于 `backups/20260927-1521/` 与 `backups/20260927-1531/`。
- 2026-09-27 Phase I 当时要求 `full_prose` 连续、多段、达到最低篇幅的小说场景，不再把每条事实机械地写成带精确日期/时钟的流水账；不合格草稿会携带明确修订意见重试，仍不合格才记录原因并回退。Play 的故事菜单新增“开始新篇”：它为当前会话持久化新的叙事边界，清空可见旧对话并阻止旧对话进入后续小说上下文，但不会删除 Event Ledger、世界状态、人物关系、既有行动或角色记忆。迁移为 063；当时部署二进制 SHA-256 为 `be090fadab9486bd3f001dd015a6b0ad5e456f32736dc64de20c6af09c6c95d5`，备份位于 `/home/ubuntu/.local/share/corerp-preview/backups/20260927-164035-phase-i/`，旧静态站点位于 `/var/www/corerp-preview.prev-20260927-164035/`。
- 当前红楼试玩会话已在 sequence 75 开始新篇。立即 API 观察与 390×844 真实浏览器验收均显示最近故事为 0 段；“开始新篇”说明页明确提示保留世界事实/关系/行动/记忆，且没有浏览器错误。重置后新提交的行动会作为新篇内容正常出现，这不是删除或冻结世界。
- 2026-09-27 Phase J 修复了真实导出暴露的双重缺陷：NPC 不再使用确定性模板复读，而由 `chat_completions` 根据当前玩家话语、角色人格、已知关系与合法动作生成受验证提议；首次完成且无 style override 的叙述会写回 `rp_turn_runs.narrative_json`，成为刷新、恢复、重启、观察和导出共同使用的正式文本。当时手动重新生成仍是临时展示，不覆盖正式文本；当前画面若显示临时版本，导出会忠实导出当前画面。迁移为 064，当时部署二进制 SHA-256 为 `91a480281ece15f5c9eb2647953ab5cbec93fe04f45ef57addc546c42d2bdacd`，备份位于 `/home/ubuntu/.local/share/corerp-preview/backups/20260927-170117-phase-j/`，旧静态站点位于 `/var/www/corerp-preview.prev-20260927-170117-phase-j/`。
- 2026-09-29 已部署回复链与 RP 主链修复：玩家自然语言解释、NPC 有来源的多轮接话、模型调用回执、物件操作与停放、长篇叙事、已选改写持久化、失败恢复和章节边界已合入预览。首次正文仍保存为正式文本；手动重新生成的版本现在保存并选中，刷新或重启后仍可读，且不修改世界事件。短而完整的正文不再因固定字数或段落下限而被迫扩写；对白、事实和来源校验仍保持。集成工作树为 `/home/ubuntu/corerp-preview-integration-20260929`，沿用预览已有的 042–064 迁移，新功能排为 065–075。当前二进制 SHA-256 为 `fea2067aa393dd6608b5502f13a7ee3884d5ba5336203d167de315a99a4cbc23`；停服一致性备份位于 `/home/ubuntu/.local/share/corerp-preview/backups/20260929-1030/`，旧页面位于 `/var/www/corerp-preview.prev-20260929-1030/`。上线后 `/readyz` 与页面均为 200，未认证观测接口为 401，预览玩家令牌的现有会话观察为 200；数据库 075 的 `integrity_check=ok`、外键错误 0、Event 数仍为 164。完整 storage、HTTP、`go vet`、前端构建和公网页面浏览器检查已通过；隔离测试世界的 Gemini 3.8 Flash 完成 32 连续轮及故障/重启检查。Gemini 只用于此轮测试，预览默认模型配置没有更改。
- 2026-09-29 追加 `https://gcli.ggchan.dev/v1` 到预览服务的 `CORERP_PROVIDER_ALLOWLIST`，允许已认证玩家在「模型与 API 设置」使用该公益站的 `/v1/chat/completions` 和 `/v1/models`。服务器 DNS 解析得到公网地址，HTTPS 证书校验通过；更新后以不带密钥的预览请求探测 `/v1/models`，返回上游 401 而非本地 Endpoint 拒绝，证明许可生效，**不代表玩家密钥已验证**。此次配置更新没有执行 RP 回合，也没有修改服务默认模型；预览世界仍可由其他玩家同时推进。修改前环境文件备份在 `/home/ubuntu/.local/share/corerp-preview/backups/gcli-allowlist-20260929-111417/env`（仅本机可读）。
- 隔离真实模型验收中，“请认真告诉我，你是谁？”由 NPC 回答“我是贾母，你的老太太。宝玉，你这般认真问，我便认真答你。”；随后生成 330 字、4 段且不以时间戳开头的正式叙述。该文本与流式画面一致，并在服务重启、叙述 provider 改为 deterministic 后仍从数据库原样复用，证明没有再次调用模型或退回流水账。
- 本机实测：Nginx 配置检查通过，HTTPS 域名证书校验通过，从本机访问公网域名/IP 返回 200；未认证 API 返回 401，外域 Origin 返回 403；Playwright 用相同 HTTPS 域名完成玩家登录、真实观察和抽屉操作，无浏览器错误。**没有独立外网客户端验证云安全组放行**；若外部访问超时，检查云侧入站 TCP 4188。

运维：`sudo systemctl status corerp-preview` 查看服务，`sudo journalctl -u corerp-preview -n 50 --no-pager` 查看日志；如需暂停试玩，先执行 `sudo systemctl stop corerp-preview`，再按实际需求关闭 Nginx 的独立入口。更新页面需重新构建并仅复制 `dist/` 到 `/var/www/corerp-preview/`；源代码更改不会自动同步到已发布的静态文件或已构建的后端二进制。数据库迁移前先停服务并使用 SQLite `.backup` 生成一致快照。不要在此预览实例处理真实身份或生产凭证。

## 2026-10-01 R1 来源与会话窗口更新

最新隔离预览已采用 `feea3b32f0315d95fa447e89d48f1bae3e991e690fe0b98e9f6d20bf18eb834d` runtime 与 079 session opening 迁移；最终代码提交由公开 `release.json` 记录。433 个生产源文件摘要为 `0200985bba147b36d7cdf6578b9276e88d87a44b55e54c44410942c7bc032ee6`。默认 Step 模型、原页面与 READY 世界保持原样。新[七轮准确度对照](https://code.linhony.xyz:4188/rp-review/accuracy/)与此前完整22轮审阅分别保留。

升级前后 198 条原事件、分支 head、13 个 provider 回执和旧 canonical 正文摘要一致，24 个旧 session 保留 opening=0。新 session 的 opening=28 保持四段共享旧历史；三条旧表情没有再次出现在当前正文。实际默认 Step 烟测一次成功、无额外模型叙述，重启后正文与来源不变，没有重复世界效果或调用。旧世界 head 保持不变，配置与前端 hash 完全一致。停服一致性备份位于 `backups/20261001-source-support-079/`。

这只证明已修复的来源契约与叙述时间边界。实际回复仍从“原处”推断站姿，并重复未设定的座位；语义准确度 NOT PASSED、人工体验 PENDING、R1 NOT DONE。新的完整32/22尚未运行，不借旧结果宣称其通过。已新增079正文后不要未经验证降回078 reader 或还原 live DB。安全字段证据见 [本次发布记录](../rp-runtime-r1/source-support-preview-release-2026-10-01.json)与[当前交接](../rp-runtime-r1/handoff-2026-10-01.md)。

## 2026-10-01 首次发布的 R1 审阅预览（e3/f5dd）

以下是 10 月 1 日已激活的隔离试玩版本；上文保留为此前部署历史。主入口仍为 [CoreRP 试玩](https://code.linhony.xyz:4188/)，审阅入口为 [R1 审阅页](https://code.linhony.xyz:4188/rp-review/)。这次是可访问的产品预览发布，不代表 R1 已完成，也不代表语义准确度或角色体验验收通过。

- **发布身份与默认模型：** 已部署源提交为 `e3cdc8c9534bb8519d85c162bd1129b9f7184475`，运行二进制 SHA-256 为 `f5dd16967733d6ded29466cb8c0bdf38f7bc19cf276a69fd5dce3f244e6a0e4a`。默认模型现为 `step-5-preview`，取代旧历史段所述的原默认配置。`release.json` 与激活报告一致；就绪、主页面、审阅页检查均为 HTTP 200，已发布主页面与本次构建匹配。普通产品世界首次使用自然事实叙述；世界作者明确开启时才允许首次模型组织，产品预设未开启该选项。
- **新的已准备世界：** `world_rp_runtime_r1_20261001`，显示名称「红楼梦·大观园 · R1」。采用明确创作的十一人世界，十名 NPC 的人设、公开语气、与玩家的方向关系及称呼均 `READY`。作者源 SHA-256 为 `31b6ce345006c380e34cf1d4e8384283e45c5aaa8f34e51b9d06dfce8dc44cf7`；发布时仅在世界名加 R1 后缀，保留其余作者声明。进入指引（发布者浏览器观测确认）：在授权世界选择器选择宝玉及上述实例的绑定；本次会话已来到荣庆堂，Play 的主标题显示当前地点「荣庆堂」。
- **迁移与旧世界保全：** 已应用 076–078，最新为 `corerp-rp-narrative-artifacts-078-2026-10-01`。停服一致性备份得到确认；迁移前后事件数均为 **170**，旧分支 head 完全一致，`integrity=ok`、外键错误 0。170 指迁移阶段原有事件的保全检查；创建新世界和试玩随后会追加新事件，不表示当前总事件数冻结在 170。新世界烟测亦记录旧世界 head 保持不变，没有替换旧世界。
- **实际调用与移动端证据：** 新世界先完成两轮真实默认 Step 调用，再完成 390 像素移动端浏览器的第三轮；三个回合标识不同，呈现均为 `corerp.fact-composition.v2`。每轮 NPC 决策回执为 `chat_completions / step-5-preview`、一次尝试成功；自然叙述回执为 deterministic、零模型尝试。移动端使用服务器默认而非自带 profile，报告无横向溢出、无页面脚本错误、凭证未持久化。新世界重启检查通过：已保存公开文本保持一致，没有重复效果或模型调用。这些是发布烟测与恢复证据，不是角色理解正确的证明。
- **验收状态与未实现工作：** 激活、世界烟测及公开 release 元数据都将 `human_experience` 标为 **PENDING**。现有样本尚未通过用户体验与语义准确性验收，不得把 HTTP 200、schema 校验、来源完整或三个成功回合写成体验 PASS。后续更准确的角色理解、更充分的长期连续性及其他拟议改进仍是待实现/待验证工作，不能当作此版本已经具备或已经部署的能力。

本次文档核对只读取已有报告的安全字段，没有重跑模型、浏览器或部署。证据是本机受限发布目录 `/home/ubuntu/.local/share/corerp-preview/r1-release-20261001-e3cdc8c/` 中的 `preparation.json`、`activation.json`、`world-smoke.json`、`client-smoke.json`，以及已发布的 `release.json`。文档未复制模型端点、密钥、凭证或数据库内容；保留报告的技术 PASS 与人的 PENDING 区别。
