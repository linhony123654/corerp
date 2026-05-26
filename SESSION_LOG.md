# CoreRP Session Log

> 记录规则：每条日志必须使用绝对时间，并标注修改者/模型身份。

## 2026-05-26

### 2026-05-26 12:55:02 UTC — 角色卡导入架构修复
Modified by: Codex (GPT-5)

- 导入架构收口：
  - 角色卡导入输出 `characters/<角色>.yml`
  - 世界书/canon/scene 输出到 `worlds/<导入源>/`
  - 角色 YAML 新增 `world_path`，运行时优先按该字段绑定世界
  - 前端保存角色卡时保留已有 `world_path`
- 导入器修复：
  - 目录导入完成后不再继续把目录当单文件导入
  - 目录批量导入现在同时处理 `.png` 和 `.json`
  - ensemble worldbook 中地点/设定/规则类条目不再默认当作顶层运行角色
  - `cast_index.yml` 继续保留 secondary cast，完整世界资料进入 ontology/canon
- 资源文件夹重新导入：
  - 使用修复后的导入器重新导入 `/home/kelebituo/资源文件夹`
  - 当前生成 49 个顶层角色 YAML 与 9 个 world 入口
  - 临时启动验证确认角色按 `world_path` 绑定到对应 world
- 文档同步：
  - `README.md`
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `node --check web/app.js` ✅
  - `git diff --check` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅

### 2026-05-26 12:45:53 UTC — 资源文件夹角色卡导入验证
Modified by: Codex (GPT-5)

- 用户路径修正：
  - 正确角色卡目录为 `/home/kelebituo/资源文件夹`
  - 目录内包含 7 个 SillyTavern PNG 和 1 个 JSON 角色卡
- 导入处理：
  - 使用 `./corerp import -src /home/kelebituo/资源文件夹 -dst ./characters -mode auto` 导入 PNG
  - 单独导入 `《红楼梦》完整版、-角色卡-202604190812.json`
  - 当前生成约 47 个顶层角色 YAML 与对应 `worlds/<source>/` 世界目录
- 发现问题：
  - CLI 目录导入完成后会继续把目录当单文件导入，导致最后报 `read ... is a directory`，但 PNG 已实际导入成功
  - ensemble JSON 会把设定/地点/规则类 worldbook 条目也生成为顶层角色 YAML
  - 当前 `findWorldFile` 依赖角色文件名匹配 `worlds/<角色名>/`，但导入目录按源文件名建 world，因此临时启动时新角色默认落到 `worlds/cyberpunk2077/world.yml`
- 验证：
  - 临时数据目录 `/tmp/corerp-import-check` 启动成功
  - 未重启正式 PM2，避免未整理角色列表直接污染当前运行实例

### 2026-05-26 12:43:32 UTC — 角色卡解析接口检查与当前角色卡清空
Modified by: Codex (GPT-5)

- 解析能力检查：
  - 当前已有 CLI 导入：`./corerp import -src <png_or_json_or_dir> -dst ./characters`
  - 支持 SillyTavern PNG / JSON 单卡导入
  - 批量目录导入当前只扫描 `.png`
  - 目前没有 HTTP 上传解析接口，前端只支持编辑已加载角色卡
- 运行数据处理：
  - `/home/kelebituo/资源` 当前只发现截图文件，未发现 PNG/JSON 角色卡
  - 备份当前 `characters/` 到 `data/character-card-backup-20260526T124231Z/`
  - 删除当前非 tracked 角色卡 YAML
  - 恢复误删的 tracked `characters/worlds/...` 世界资料，避免仓库出现 world 文件删除

### 2026-05-26 12:38:30 UTC — 移动端顶栏收敛与测试数据清理
Modified by: Codex (GPT-5)

- `web/index.html`
  - 移动端顶栏改为两行布局，隐藏品牌副标题与选择器标签
  - 控制台入口在移动端改回图标按钮，避免大按钮挤占首屏
  - 静态资源版本更新到 `app.js?v=20260526g`
- 运行数据清理：
  - 结束遗留的 `/tmp/corerp-ui-test/ui-jsdom-e2e.js` 进程
  - 备份污染前数据到 `data/cleanup-backup-20260526T123336Z/`
  - 清除 world/data/SQLite 中的 jsdom/e2e 测试场景、checkpoint、preset 与事件
  - 重启 PM2 `corerp`，让运行态重新从清理后的数据加载
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅
  - world/data/SQLite 测试痕迹扫描为 0 ✅

### 2026-05-26 12:25:35 UTC — 主页面编辑风重构
Modified by: Codex (GPT-5)

- `web/index.html`
  - 主页面从运行工具台风格改为叙事编辑器风格
  - 顶栏改为薄工具条，仅保留品牌、实例、角色和控制台入口
  - 场景与角色信息收成正文上方 metadata 区，去除大 hero/card 感
  - 对话区改为阅读排版：助手文本正文化，用户输入保留轻量边框
  - 右侧控制台改为 inspector 风格分段列表，弱化卡片背景和网格密度
  - 静态资源版本更新到 `app.js?v=20260526f`
- 验证：
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test ./...` ✅
  - `git diff --check` ✅
  - 登录后首页 HTML 返回新版编辑风布局与 `app.js?v=20260526f` ✅

### 2026-05-26 12:18:24 UTC — 主页面密度重构
Modified by: Codex (GPT-5)

- `web/index.html`
  - 收紧顶栏、场景区、角色摘要和输入区尺寸，让聊天区获得更多首屏空间
  - 去除主页面装饰性径向背景和大卡片阴影，统一卡片/控件圆角与信息密度
  - 右侧控制台宽度与间距下调，默认折叠组不再漏出角色卡编辑区
  - 删除重复 trace 控件 DOM，避免重复 ID 与绑定歧义
- 验证：
  - 推送前确认 `origin/master` 已是最新
  - `node --check web/app.js` ✅
  - `/usr/local/go/bin/go test ./...` ✅
  - 登录后首页 HTML 返回新版资源参数 `app.js?v=20260526e` ✅

### 2026-05-26 11:50:47 UTC — 首页改为更简约的编辑风主版面
Modified by: Codex (GPT-5)

- `web/index.html`
  - 顶部从三段大块信息改为更轻的两段结构
  - “运行数据”改为默认折叠的 summary/details
  - 主区 story header 改为单栏编辑页风格
  - 当前角色摘要压缩为低占用信息卡，不再和场景 headline 抢首屏
  - 右侧控制台 overview 卡隐藏，减少重复说明与视觉密度
- 验证：
  - `node --check web/app.js`
  - `node /tmp/corerp-ui-test/ui-jsdom-e2e.js`
- 说明：
  - 本轮主要是结构与样式收敛，不改后端接口
  - `jsdom` 外链脚本噪音仍存在，但 checkpoint / rollback / preset / trace turn 主链路继续跑通
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 11:41:26 UTC — 手机端首页摘要折叠补完与等价回归
Modified by: Codex (GPT-5)

- `web/index.html` / `web/app.js`
  - 补完超窄屏首页“角色摘要”按钮交互
  - 当前角色 spotlight 在手机窄屏默认折叠
  - 点击后仅切本地 UI 状态，不触碰后端接口
  - 状态写入 `localStorage`，避免用户每次重开都要重复展开
- 验证：
  - `node --check web/app.js`
  - `node /tmp/corerp-ui-test/ui-jsdom-e2e.js`
- 说明：
  - `jsdom` 仍会输出 `/app.js?v=...` 外链解析噪音
  - 但手工注入的 `web/app.js` 逻辑已跑通主链路：
    - checkpoint / rollback
    - scenario preset 保存 / 套用
    - trace 历史切 turn
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 11:50:00 UTC — Trace 作者控制台第二轮优化
Modified by: Codex (GPT-5)

- `web/index.html` / `web/app.js`
  - 为 trace 历史增加当前 turn 高亮
  - 新增 `上一轮 / 下一轮` 导航
  - checkpoint 列表增加“依据”按钮，联动跳转到最接近的 trace turn
- 说明：
  - 这一轮只改前端交互层，不改后端接口
  - 第一轮作者工具主路径可视为正式全绿
- 验证：
  - `node --check web/app.js`
- 文档同步：
  - `TODO.md`
  - `TEST_PROGRESS.md`

### 2026-05-26 11:15:00 UTC — 作者工具第一版：checkpoint / rollback / scenario preset / trace 历史
Modified by: Codex (GPT-5)

- `internal/runtime/authoring.go`
  - 新增作者工具能力：
    - `ListTurnTraces(limit)`
    - `ListCheckpoints / CreateCheckpoint / LoadCheckpoint`
    - `ListScenarioPresets / CreateScenarioPreset / ApplyScenarioPreset`
  - `scenario_presets.json` 落到实例目录
- `internal/api/server.go`
  - 新增接口：
    - `GET /api/traces`
    - `GET/POST /api/checkpoints`
    - `POST /api/checkpoints/load`
    - `GET/POST /api/presets`
    - `POST /api/presets/apply`
- `web/index.html` / `web/app.js`
  - 新增作者工具 UI：
    - checkpoint / rollback
    - scenario preset 保存 / 套用
    - trace turn 历史列表与指定轮次查看
- 测试：
  - `go test ./internal/runtime ./internal/api`
  - `internal/runtime/runtime_test.go`
    - scenario preset create/apply
    - turn trace 历史顺序
  - `internal/api/server_test.go`
    - traces / checkpoints / presets 路由
- 文档同步：
  - `TODO.md`
  - `TEST_PROGRESS.md`
  - `ARCHITECTURE.md`
  - `api-contract.yaml`

### 2026-05-26 11:35:00 UTC — 作者工具 UI 等价回归完成，Chromium 实机回归待补
Modified by: Codex (GPT-5)

- 已完成 UI 等价回归：
  - `checkpoint` 创建
  - 切角色 / 改场景
  - `rollback`
  - `scenario preset` 保存 / 套用
  - `trace` 历史切 turn
- 当前采用 `jsdom` 执行 `web/app.js` + 线上接口联调
- 原因：
  - 本机缺 Chromium 运行库
  - `puppeteer` 启动时报缺少 `libnspr4.so`
- 说明：
  - 这轮验证可证明前端交互链路已通
  - 但不等同于完整 Chromium 无头实机点击回归
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:36:00 UTC — 收紧 legacy root 文件 fallback 语义
Modified by: Codex (GPT-5)

- `internal/runtime/persistence.go`
  - root 级 legacy：
    - `player_role.json`
    - `save_slots.json`
  - 现在仅 `default` 实例会 fallback 读取
  - 具名实例不再继承 legacy root 文件
- `internal/runtime/persistence_instance_test.go`
  - 调整测试预期：
    - `default` 仍可读取 legacy root
    - `alpha` 等具名实例返回默认角色 / 空存档
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:31:00 UTC — PM2 实机回归抽查
Modified by: Codex (GPT-5)

- 进程状态：
  - `pm2 show corerp`
  - 状态 `online`
  - uptime 正常增长
  - 启动参数仍为标准：
    - `serve -port 8080 -data /home/kelebituo/corerp/data -characters ./characters -secure-cookie=false`
- 探针与接口：
  - `./deploy/smoke-check.sh` → `/api/health=200` / `/api/ready=200`
  - `GET /api/version` 正常返回：
    - `version=dev+dirty`
    - `commit=f6639caebb21a09af57d5b1130ac482eb15a8e45`
    - `time=2026-05-25T12:16:30Z`
  - `GET /login` → `200`
  - `GET /api/instances` 未登录时仍为 `401`
- 运行日志：
  - 最近日志停留在稳定启动后状态，无新增启动卡死迹象
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:28:00 UTC — 缩放版 70 tick 存活测试落地
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增长跑存活测试：
    - 使用毫秒级 tick interval 跑满 70 tick
    - 验证后台 tick loop 连续运行不 panic
    - 验证 world clock 推进
    - 验证 canonical events 持续增长
    - 验证 `QueryActionLog` / `ActionLogStats` 仍可读
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:20:00 UTC — 最小 legacy 兼容测试补齐
Modified by: Codex (GPT-5)

- `internal/events/store_test.go`
  - 增加 `instance_id=''` legacy event 对 `default` 可读、对具名实例不可读测试
- `internal/memory/engine_test.go`
  - 增加 legacy dialogue / semantic fact 对 `default` 可读、对具名实例不可读测试
- `internal/runtime/persistence_instance_test.go`
  - 补 root 级 legacy `player_role.json` / `save_slots.json` fallback 行为测试
- 说明：
  - 这批测试是“兼容旧格式不回归”，不是恢复旧测试数据
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:14:00 UTC — 跨角色交错对白因果串链修复
Modified by: Codex (GPT-5)

- `internal/events/causality_test.go`
  - 新增同一 session / scene 下：
    - `user -> 111`
    - `user -> 安雅`
    - `111 dialogue`
    - `安雅 dialogue`
    交错混存后的 `RebuildAll()` 回归测试
- `internal/events/causality.go`
  - 收窄 `dialogue -> dialogue` 的类型因果规则
  - 仅允许：
    - 同一说话者续说
    - 对前一条对白的直接回应
  - 避免跨角色交错对白被误挂成 cause/effect
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 10:08:00 UTC — ActionLogger 持久化实例隔离与 runtime 集成测试
Modified by: Codex (GPT-5)

- `internal/emotion/action_log.go`
  - `action_log` 增加 `instance_id`
  - `LoadFromDB` / `QueryDB` 改为按实例过滤
  - `LoadFromDB` 重载前先清空 ring buffer，避免重复加载
- `internal/runtime/runtime.go`
  - `SetInstanceMetadata()` 现在同步：
    - `actionLogger.SetInstanceID(...)`
    - `actionLogger.LoadFromDB(200)`
- `internal/runtime/instances.go`
  - 删除实例时新增清理 `action_log`
- 测试：
  - `internal/emotion/action_log_test.go`
    - 增加 `ActionLogger` 的实例隔离回归测试
  - `internal/runtime/runtime_test.go`
    - 增加 runtime 层“重启后按实例恢复 action log”测试
  - `internal/runtime/instances_test.go`
    - 删除实例高层测试纳入 `action_log`
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 09:58:00 UTC — 因果链 narrative API 断言补齐
Modified by: Codex (GPT-5)

- `internal/api/server_test.go`
  - 新增 `/api/causality?mode=narrative` 响应断言
  - 覆盖：
    - `event_id`
    - `depth`
    - `chain`
    - narrative `summary`
  - 同时补默认模式回归，确认 plain summary 不误走 narrative 分支
- `internal/events/causality_test.go`
  - 现有“无回边渲染”测试继续作为 narrative summary 回归保障
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 09:52:00 UTC — 单实例多角色往返切换一致性测试补齐
Modified by: Codex (GPT-5)

- `internal/runtime/runtime_test.go`
  - 新增单实例多角色往返切换集成测试
  - 覆盖：
    - `111 -> 安雅 -> 111 -> 安雅`
    - 每角色自己的 `world`
    - 每角色自己的 `scene`
    - 每角色自己的 `dialogue`
    - 自定义 `player role` 在切角色后仍正确映射到场景角色列表
  - 同时验证：
    - 只编辑安雅的 world/scene 后，不影响 `111`
- 文档同步：
  - `TEST_PROGRESS.md`

### 2026-05-26 09:43:00 UTC — 实例删除高层测试与冲突语义收口
Modified by: Codex (GPT-5)

- `internal/runtime/instances_test.go`
  - 新增删除实例高层集成测试：
    - 覆盖 `events / branches / dialogue_history / working_memory / semantic_facts / episodic_events / pending_facts`
    - 覆盖 `data/instances/<instance_id>/player_role.json` 与 `save_slots.json`
    - 确认删除 `alt` 不影响 `default`
  - 补默认实例/唯一实例删除冲突测试
- `internal/runtime/instances.go`
  - 增加实例错误哨兵：
    - `ErrInstanceIDRequired`
    - `ErrInstanceNotFound`
    - `ErrInstanceConflict`
- `internal/api/server.go` / `internal/api/server_test.go`
  - `default / stop / delete` 统一实例错误映射
  - `instance not found` → `404`
  - 删除默认实例或唯一实例 → `409`
- 文档同步：
  - `TODO.md`
  - `TEST_PROGRESS.md`

### 2026-05-26 09:27:30 UTC — 启动自锁修复与版本探针验证
Modified by: Codex (GPT-5)

- `internal/events/store.go`
  - 修复 `seedBranchesFromEvents()`：
    - 先读完并关闭 `rows`
    - 再调用 `ensureBranch()`
  - 避免共享 SQLite 且 `MaxOpenConns(1)` 时的启动自锁
- `internal/events/store_test.go`
  - 新增“重开已有 branch 数据库不得卡死”回归测试
- 运行验证：
  - 前台使用标准 `data/` 启动恢复正常
  - `./deploy/pm2-start-corerp.sh`
  - `./deploy/smoke-check.sh` → `/api/health=200` / `/api/ready=200`
  - `GET /api/version` 返回：
    - `version=dev+dirty`
    - `commit=f6639caebb21a09af57d5b1130ac482eb15a8e45`
    - `time=2026-05-25T12:16:30Z`
- 文档同步：
  - `ARCHITECTURE.md`
  - `TODO.md`
  - `TEST_PROGRESS.md`

### 2026-05-26 09:26:00 UTC — 构建元数据与启动日志补全
Modified by: Codex (GPT-5)

- `cmd/corerp/main.go`
  - 新增构建元数据解析：
    - 优先使用 ldflags 注入值
    - 回退读取 `runtime/debug.ReadBuildInfo()` 中的 `vcs.revision / vcs.time / vcs.modified`
  - 启动时打印：
    - `version`
    - `commit`
    - `build_time`
    - `data`
    - `port`
- `internal/api/server.go`
  - `GET /api/ready` / `GET /api/version` 增加 `build_time`
- `cmd/corerp/main_test.go` / `internal/api/server_test.go`
  - 补构建元数据与版本接口回归测试
- 文档同步：
  - `README.md`
  - `api-contract.yaml`

### 2026-05-26 08:56:00 UTC — Runtime Instance 生命周期闭环
Modified by: Codex (GPT-5)

- `internal/runtime/instances.go`
  - manager 新增 `status / stop / delete`
  - 删除实例会清理实例目录与共享 SQLite 中的实例命名空间数据
- `internal/api/server.go`
  - 新增：
    - `GET /api/instances/status`
    - `POST /api/instances/stop`
    - `POST /api/instances/delete`
- `internal/api/server_test.go` / `internal/runtime/instances_test.go`
  - 补实例状态、停止、删除回归测试
- 文档同步：
  - `README.md`
  - `ARCHITECTURE.md`
  - `TEST_PROGRESS.md`
  - `TODO.md`
  - `api-contract.yaml`

### 2026-05-26 09:10:00 UTC — 前端实例管理面板接入
Modified by: Codex (GPT-5)

- `web/index.html` / `web/app.js`
  - 侧栏新增实例管理卡片
  - 支持：
    - 查看实例列表与状态
    - 创建实例
    - 切换默认实例
    - 停止实例
    - 删除非默认实例
- 运行台刷新链路会同步刷新实例摘要
- 文档同步：
  - `README.md`
  - `ARCHITECTURE.md`
  - `TODO.md`

### 2026-05-26 09:05:31 UTC — 标准 data 目录重建与 PM2 启动固化
Modified by: Codex (GPT-5)

- 删除旧测试运行库，重建空白 `data/`
- 新增：
  - `deploy/pm2-start-corerp.sh`
  - `deploy/smoke-check.sh`
- PM2 当前固定使用：
  - `serve -port 8080 -data /home/kelebituo/corerp/data -characters ./characters -secure-cookie=false`
- 验证：
  - `./deploy/smoke-check.sh` → `/login=200` / `/api/instances=401`

### 2026-05-26 08:57:00 UTC — 前端显式实例视图切换接入
Modified by: Codex (GPT-5)

- `web/index.html` / `web/app.js`
  - 顶栏新增实例选择器
  - 页面内切换实例后，runtime 请求自动带 `instance_id`
  - 实例面板现在区分：
    - 默认实例
    - 当前视图实例
- 文档同步：
  - `README.md`
  - `ARCHITECTURE.md`
  - `TODO.md`

### 2026-05-26 09:10:00 UTC — 健康检查与就绪探针落地
Modified by: Codex (GPT-5)

- `internal/api/server.go`
  - 新增：
    - `GET /api/health`
    - `GET /api/ready`
    - `GET /api/version`
- `internal/api/server_test.go`
  - 补 health / ready / version 单测
- `deploy/smoke-check.sh`
  - 改为直接检查 `/api/health` 和 `/api/ready`
- 文档同步：
  - `README.md`
  - `TODO.md`
  - `TEST_PROGRESS.md`
  - `api-contract.yaml`

### 2026-05-26 08:09:07 UTC — Runtime Instance 基础设施与实例隔离
Modified by: Codex (GPT-5)

- 实例管理：`list / create / set default`
- 运行时 API 全面接入 `instance_id`
- `player_role.json` / `save_slots.json` 迁移到 `data/instances/<instance_id>/`
- 共享 SQLite 中的 `events / branches / dialogue / working_memory / semantic_facts / episodic_events / pending_facts` 改为按实例隔离
- 补共享 SQLite 下的双实例隔离测试

### 2026-05-26 06:29:51 UTC — 最终形态蓝图与多 step handoff 落地
Modified by: Codex (GPT-5)

- 新增 `FINAL_ARCHITECTURE_BLUEPRINT.md`
- TurnStep 之间显式 handoff 已接入 runtime prompt 与 trace
- `README.md` / `ARCHITECTURE_RUNTIME.md` 同步到多 step 语义

### 2026-05-26 06:14:12 UTC — Director turn plan 升级为职责化 step 链
Modified by: Codex (GPT-5)

- `DirectorPlan` 从“切活跃角色”升级为显式 `TurnStep` 序列
- `auto_chain` 具备 lead / followup 职责链语义
- 前端 trace 面板支持按 step 查看执行链

### 2026-05-26 05:51:54 UTC — Branch 继承回放模型收口
Modified by: Codex (GPT-5)

- `branches` 元数据表落地
- `Fork()` 改为创建分支元数据，不再改写历史事件归属
- `ReplayTo()` / `GetTimeline()` 改为沿父分支链回放

### 2026-05-26 08:56:00 UTC — 本轮验证
Modified by: Codex (GPT-5)

- `/usr/local/go/bin/go test ./...` ✅

### 2026-05-26 13:05:06 UTC — 角色卡导入架构收紧
Modified by: Codex (GPT-5)

- 修复 SillyTavern 目录导入：
  - 目录导入继续支持 `.png` / `.json`
  - 世界资料卡现在按 `world-only` 导入，只写入 `worlds/<source>/`
  - 地点、势力、规则、物品、速览、生成器等条目不再生成顶层可运行角色
- `world.yml` 收敛为 meta + compact core_rules，长世界书内容保留在 `canon/ontology.yml` 与 `canon/facts.yml`
- 重新导入 `/home/kelebituo/资源文件夹`：
  - 顶层角色 YAML 从 49 收敛到 31
  - 8 个源文件均生成对应 `worlds/<source>/`
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅
  - `pm2 restart corerp --update-env` ✅

### 2026-05-26 13:10:47 UTC — 导入角色 voice 推断修复
Modified by: Codex (GPT-5)

- 修复导入器 voice 默认值过度复用：
  - `inferStyle()` 改为从角色正文与示例对话推断语气
  - `inferRhythm()` 改为同时参考正文，避免缺少 `mes_example` 时全部落到 `短句为主`
- 重新导入 `/home/kelebituo/资源文件夹`
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `/usr/local/go/bin/go build -o corerp ./cmd/corerp` ✅
  - `pm2 restart corerp --update-env` ✅
  - `/api/health` ✅

### 2026-05-26 14:10:04 UTC — world-first 入口第一步
Modified by: Codex (GPT-5)

- 明确产品方向：
  - CoreRP 不是高级酒馆或预写任务树 RPG
  - 目标是长期演化、可回放、可分叉、人物会被经历改变的文字世界 runtime
- 新增 world catalog：
  - `core.WorldSummary`
  - `internal/world.ListCatalog()`
  - `GET /api/worlds`
- 前端顶栏新增“世界”选择器，角色选择改为“视角”兼容层
- 当前切口只改变入口展示与 catalog API，不直接拆除 active character runtime 锚点
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `node --check web/app.js` ✅
  - `git diff --check` ✅

### 2026-05-26 14:10:04 UTC — population skeleton 落地
Modified by: Codex (GPT-5)

- 新增 world population 骨架：
  - `background_npcs.yml`
  - `promoted_npcs.yml`
  - `identity_core.yml`
  - `policy.yml`
- 新增类型与接口：
  - `core.PopulationConfig`
  - `core.BackgroundNPC`
  - `core.PromotedNPC`
  - `core.IdentityCoreConfig`
  - `core.PromotionPolicy`
- `internal/world` 支持读取/保存 population
- `internal/runtime` / `internal/api` 新增：
  - `GetPopulationConfig()`
  - `UpdatePopulationConfig()`
  - `GET/POST /api/population`
- importer 创建 world 目录时会一并初始化 `population/`
- world catalog 补充 population 统计字段
- 验证：
  - `/usr/local/go/bin/go test ./...` ✅
  - `node --check web/app.js` ✅
  - `git diff --check` ✅
