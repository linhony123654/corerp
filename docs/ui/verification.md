# Play 界面基准验收（2026-09-25）

唯一确认的 UI 原型是同目录的 [`corerp_themes.html`](corerp_themes.html)；已与提供的原文件逐字节核对。以 `AGENTS.md` 的 Play 规则为准，旧视觉稿、截图和设计建议不作为验收依据。原型的预设剧情、模型选择和演示思考阶段不能当作真实世界数据。

## 同视口对照

在 390×844、设备像素比 1、减少动态效果的 Chromium 中分别实际运行原型和默认入口 `/` 的 Vue Play（临时 SQLite + HTTP 服务）。`node scripts/verify-rp1-play.mjs --ui-baseline` 将配对截图写到当前输出的 `artifacts/ui/`：`reference-*` 对 `play-*`，覆盖主聊天、左抽屉、锚定菜单、设置、二级主题、四主题、人物详情及公开请求状态/摘要。人工逐张检查了最新成对截图：色板、标题/状态胶囊、抽屉宽度、菜单位置、sheet 层级、悬浮输入框和透明单行状态符合基准。内容与 sheet 高度因真实观察与原型假数据不同而不逐像素相等；真实记录不伪造说话者或模型内部思维。

同一浏览器检查在 390×500 布局视口与 390×844 布局视口、仅 `visualViewport.height` 收缩至 510 的模拟软键盘场景下，验证输入多行高度上限、无横向溢出、末条记录可滚到输入框上方以及键盘缩小时输入框仍在可见区；Enter 留在草稿并换行。截图为 `play-keyboard-multiline.png` 和 `play-visual-viewport-keyboard.png`。这是浏览器模拟，不宣称已在实体手机或特定输入法上验收。

## 当前源码回归

- `npm run build`：Vue 类型检查与 Vite 构建通过；`git diff --check` 通过。
- `node scripts/verify-rp1-play.mjs --ui-baseline`：原型/实页视觉几何、主题、键盘、焦点与真实服务端会话恢复通过。
- `node scripts/verify-rp1-play.mjs --ui-baseline --fake-model --custom-style`：本地 HTTP 模型/叙事 fixture、主题和真实客户端回归通过，**不是 live Provider**。
- `node scripts/verify-rp1-play.mjs --turn-stream --context-budget`：截断流、重启后的只读叙述恢复和错误提示标点通过。
- 2026-09-26 前端优化（移动端配置行布局修复、路由级与抽屉组件代码分割、nginx gzip 与 /assets/ 长缓存）后复跑：`--ui-baseline`、`--ui-baseline --fake-model --custom-style`、`--turn-stream --context-budget` 三个变体均 PASS；`scripts/test-ui-playwright.mjs`（模型设置全流程）与 `scripts/test-live-proxy.mjs`（线上代理实测）通过。
- 2026-09-26 对话区优化（叙述行按句型分类渲染：玩家对白右对齐带竖线、NPC 对白带首字头像与加粗说话人、世界事件居中分隔线、日期分隔仅在跨天时出现、去掉每回合重复的「世界记录」标签、流式预览复用同一分类；渲染保持原文 textContent 与 `.turn > .prose > p` 结构不变）后复跑：`--ui-baseline` 三个变体均 PASS；`rp6-regenerate-checks`、`rp6-custom-style-checks`、`rp6-budget-checks`、`rp6-long-play-checks`、`rp6-turn-stream-checks`、`rp6-stream-parser-checks`、`test-ui-playwright.mjs`、`test-live-proxy.mjs`、`verify-api-profiles.mjs` 全部通过（exit 0）；桌面 1280×800 与移动 390×844 截图人工核对通过。
- 2026-09-26 玩家自带模型闭环：请求级模型覆盖（`model={endpoint,model,api_key,timeout_seconds}`）接入 `turns/run`、`turns/resume`、`interactions/run`、`interactions/resume`、`actions/wait`、`narrative/render`、`narrative/stream`；传输层解析为内存 provider（`httpapi/rp_model.go`），不落盘、无效配置显式 400 不静默回退；前端 `act()`/`streamNarrative()` 在配置生效时自动附带。新增 `rp_model_override_test.go`：覆盖回合真实打到玩家端点（模型名/密钥/回复文本核对）、重放不重复计费、无覆盖时默认 provider 不触碰玩家端点、三类无效配置全部 400。回归：httpapi/storage/decision/narrative 包测试、`--ui-baseline` 三变体、rp6 六项、`test-ui-playwright.mjs`、`test-live-proxy.mjs`、`verify-api-profiles.mjs` 通过；预览实例已部署新二进制并实测无效覆盖被拒（INVALID_ARGUMENT）。storage 全量套件（约 1350 秒）无负载复跑全绿；此前一次并发重载下 `TestFinalWorldSeparatedFriendsRareVisit` 未命中系测试争用抖动，与本次改动无关。
- 2026-09-26 小说式呈现（full_prose）：新增 `narrative.ChatProseProvider`——模型把已结算归因事实改写成小说段落；输出校验要求对白逐字保留、不得新增引号对白、遵守禁用模式，失败在预算内重试一次再回退确定性渲染并附警告（呈现层回退，不动世界事实）。请求级 `model.full_prose=true` 或环境变量 `CORERP_NARRATIVE_PROVIDER=full_prose` 启用；前端配置表单新增「小说式呈现」开关。新增 `narrative/prose_test.go`（成文、流式、篡改/编造/遗漏对白回退、禁用模式回退）；narrative/decision/httpapi 测试、`--ui-baseline`、rp6 叙事四项、`test-ui-playwright.mjs`、`verify-api-profiles.mjs` 通过；预览实例实测 full_prose 覆盖命中模型并按段流式返回、无效配置 400。
- `node scripts/verify-rp8-play-worlds.mjs --creator-ui` 与 `--interaction-isolation`：独立创作台、跨世界切换、迟到流隔离通过。
- `node scripts/verify-rp6-work-life.mjs --messages` 与 `--long-play`：真实经济世界通知、工作、104 回合/26 次等待/重启后匿名人物来源核对通过；长程测试将新人物的可见 `person_` 句柄用于赠礼，内部数据库只用于核对其实际决策，不向玩家展示原始身份。
- 此前本轮已经通过 `verify:rp1-play`、各钱包/通讯录/工作/地图/手机、叙事设置/重新生成、空间旅程、双世界交互隔离等真实浏览器变体。

短暂的磁盘空间不足使一次组合测试没有有效输出，普通 `/tmp` 上后续重载出现空白页；将后续临时产物放在 `TMPDIR=/dev/shm` 后，上述独立变体完成并通过。未删除旧临时产物。F3 的两名独立外部模型居民各至少两次连续真实 Provider 决策仍 **NOT VERIFIED**；不得将本地 fixture 或 UI 验收计作 F3 门禁，也未开始 F4 或推送。后续经单独确认搭建了[隔离的公网试玩](public-preview.md)，它不改变上述阶段结论。
