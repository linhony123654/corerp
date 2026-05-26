# CoreRP Test Progress And Plan

## 当前状态

最后更新：2026-05-26

近期本地验证已完成：

- `go test ./...`
- 新增双实例共享 SQLite 隔离测试：
  - `events.Store`
  - `memory.Engine`
- PM2 托管服务可访问：`http://localhost:8080`
- 登录、角色切换、因果链、NPC 日志、对话恢复链路已做实机抽查
- “本轮依据”已做实机抽查：无 trace 时显示空态，完成一轮对话后自动刷新
- 新增作者工具接口测试：
  - `/api/traces`
  - `/api/checkpoints`
  - `/api/checkpoints/load`
  - `/api/presets`
  - `/api/presets/apply`
- UI 回归当前使用 `jsdom` 等价验证（本机缺 Chromium 运行库，`puppeteer` 启动时报缺少 `libnspr4.so`）
- trace 作者控制台第二轮优化已完成：
  - 当前 turn 高亮
  - 上一轮 / 下一轮切换
  - checkpoint 与 trace 联动
- `node --check web/app.js`
- 前端重写版 UI 已上线，手机端抽屉侧栏与时间线已做实机修补
- 手机端主页进一步压缩：
  - 当前角色摘要在超窄屏默认折叠
  - 通过“角色摘要”按钮展开 / 收起
  - 避免首页首屏过长，减少为进入设置区反复上滑
- 首页视觉继续瘦身为编辑风主版面：
  - 顶部“运行数据”改为默认折叠
  - 主区改为单栏叙事标题 + 低占用角色摘要
  - 首屏优先保留对话与输入，不让数据卡片抢主路径

## 已覆盖内容

### 1. 基础稳定性

- Event Store append / replay / snapshot / determinism
- State manager 并发安全
- Tick loop 启停与推进
- Auth 登录、cookie、安全开关
- API 方法分发和基础状态码

### 2. 近期补强

- 启动时仅在空场景下 seed `scene_init`
- 活跃角色世界上下文同步
- 禁止 legacy `dialogue_history.character=''` 自动迁移
- `ActionLogger` SQLite 持久化接入
- 因果链跨角色串线修复
- narrative 因果链去噪
- 因果链回边裁剪，避免 summary 假自环
- 因果链摘要附带文本/判定/delta/场景说明
- 事件时间解析修复（SQLite 带纳秒/时区时间）
- ontology seed 改为幂等写入，重启无 `UNIQUE constraint` 噪声
- 前端时间线切换为叙事优先视图
- LLM 配置面板删除/切换修复
- 手机端侧栏补内部关闭按钮
- narrative trace 面板接入
- `trace not found` 从报错改为空态提示
- API 显式实例路由：`instance_id`
- runtime instance manager：
  - list
  - create
  - set default
- `player_role.json` / `save_slots.json` 改为实例目录持久化
- SQLite 关键表改为 instance-scoped：
  - `events`
  - `branches`
  - `dialogue_history`
  - `working_memory`
  - `semantic_facts`
  - `episodic_events`
  - `pending_facts`

### 3. 实机确认过的接口

- `GET /`
- `GET /login`
- `GET /api/character`
- `GET /api/characters`
- `POST /api/switch`
- `GET /api/dialogue`
- `GET /api/npc-action-log`
- `GET /api/causality?id=...&mode=narrative`
- `GET /api/trace/latest`

## 当前剩余风险

### 高优先级

- 单实例内部的多角色仍是“共享事件流 + 当前角色视角切换”，不是真正的每角色时间线隔离
- 旧历史数据虽然已重建因果链，但库内仍可能存在语义不干净的老事件
- LLM 配置切换日志曾出现 `LLM switched to  @ `，说明仍需继续核查是否存在空配置切换路径

### 中优先级

- README 中测试数字和覆盖率表是阶段性快照，后续可能与实际测试数不完全同步
- `events` 表历史库缺少 `tag` 列，当前 narrative 过滤依赖事件类型回退而不是统一 schema
- runtime instances 已具备 `list/status/create/set default/stop/delete`，但前端管理面板仍缺更完整交互回归
- 新前端虽然已整体重写，但仍缺移动端真实设备全路径回归（尤其是超窄屏）
- checkpoint / preset / trace 历史 UI 已接通，但仍缺浏览器实机回归
- 当前 UI 链路已用 `jsdom` 跑通：
  - checkpoint / rollback
  - scenario preset 保存 / 套用
  - trace 历史切 turn
  - trace 作者控制台高亮 / 轮切换 / checkpoint 联动
  - 手机端主页角色摘要折叠 / 展开交互
  - 首页编辑风瘦身改版后主链路仍可用
  - 但仍不等同于真正 Chromium 实机点击回归

### 低优先级

- 前端还缺少一键查看“最强主因/主后果”的极简因果链模式
- 缺少针对 PM2 托管状态的自动健康检查

## 下一阶段测试规划

### Phase A: 数据一致性

- [x] 为多实例补更高层集成测试：
  - 同一 SQLite 下 `instance_id` 间 event/memory/branch/save 不串
  - 删除实例时 `event/branch/memory/file` 全量清理
- [x] 为多角色切换补集成测试：
  - 单实例内 `active character`、`world`、`scene`、`dialogue` 必须一致
  - 来回切换后仍保持各角色自己的 world/scene/dialogue 视图
- 为历史库迁移补测试：
- [x] 为历史库迁移补测试：
  - `instance_id=''` 的 legacy event/memory 对 `default` 仍可读
  - 具名实例不会误读 legacy SQLite 数据
  - root 级 legacy `player_role.json` / `save_slots.json` fallback 已锁定当前行为
- [x] 为 `ActionLogger` 持久化补 runtime 集成测试
  - 重启后按实例恢复
  - 同角色名跨实例不串日志
  - 删除实例时一并清理 `action_log`

### Phase B: 因果链

- [x] 增加 `mode=narrative` 的 API 断言测试
- [x] 增加跨角色事件混存下的 causality rebuild 回归测试
- 增加“主链摘要压缩”测试：
  - 只保留最强主因
  - 噪声事件不出现在 summary
- [x] 增加“无回边渲染”测试：
  - `dialogue -> user_message` 不应再次把 root dialogue 挂回去

### Phase C: 运行与部署

- [x] 增加 `pm2` 启动参数文档化检查
- [x] 增加服务启动 smoke test：
  - `GET /api/health` 必须返回 200
  - `GET /api/ready` 必须返回 200
- [x] 修复共享 SQLite + `MaxOpenConns(1)` 下的启动自锁
  - 根因：`seedBranchesFromEvents()` 在未关闭 `rows` 时再次 `Exec`
  - 回归测试：重开已有 branch 数据库时不得卡死
- [x] 增加启动后 70s tick 存活测试，防止后台 goroutine panic
  - 采用缩放版长跑测试（毫秒级 interval 跑满 70 tick）
  - 验证时钟推进、canonical event 增长、action log 统计接口仍可读

## 建议执行顺序

1. [x] 做一轮 PM2 实机回归抽查
2. [x] 收紧 legacy root 文件 fallback 语义
   - 仅 `default` 读取 root 级 `player_role.json` / `save_slots.json`
   - 具名实例不再继承 legacy root 文件
3. Narrative Compression 等真实数据后再补
4. 对作者工具做一轮浏览器实机回归：
   - checkpoint 创建 / rollback
   - scenario preset 保存 / 套用
   - trace 历史翻阅
