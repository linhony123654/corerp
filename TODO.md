# CoreRP TODO

## 当前状态

- [x] Persistent Narrative Runtime 基础内核
- [x] World / Scene / Canon 编辑台
- [x] Quarantine / Pending Facts 审核台
- [x] Director + TurnPlan / TurnStep 多 step 串行执行
- [x] Branch replay / diff / merge
- [x] Trace 面板与 step_traces 前端展示
- [x] Runtime Instance 多实例隔离
- [x] Runtime Instance 生命周期第一版：`list / status / create / set default / stop / delete`

## 当前架构缺口

### P0

- [x] `internal/runtime/`：为删除实例补更高层集成测试，覆盖 event/branch/memory/file 全量清理
- [x] `internal/api/server.go`：为实例删除补更明确的错误语义（如默认实例删除冲突、唯一实例删除冲突）

### P1

- [ ] `internal/runtime/`：把单实例内部多角色协作继续推进到更完整的多角色 turn 链
- [x] `web/app.js`：trace 历史列表与 turn 翻阅 UI
- [x] `internal/runtime/`：checkpoint / rollback / scenario preset 作者工具

### P2

- [ ] `internal/simulation/`：无用户输入时的长期自主事件推进
- [ ] `internal/events/causality.go`：strongest cause / strongest effect 视图
- [ ] `deploy/`：PM2 / systemd / reverse proxy 健康检查与巡检示例

## 近期完成

- [x] `data/` 已重建为新的标准运行目录
- [x] PM2 启动参数固化到标准 `data/`
- [x] `deploy/smoke-check.sh`：启动后检查 `/api/health` 与 `/api/ready`
- [x] `GET /api/health` / `GET /api/ready` / `GET /api/version`
- [x] 共享 SQLite 下的 `instance_id` 事件、分支、记忆隔离
- [x] `data/instances/<instance_id>/` 文件级持久化隔离
- [x] `POST /api/instances/create`
- [x] `POST /api/instances/default`
- [x] `GET /api/instances/status`
- [x] `POST /api/instances/stop`
- [x] `POST /api/instances/delete`
- [x] `web/` 实例管理面板，支持 create / default / stop / delete
- [x] `web/` 页面内显式实例选择，runtime 请求自动附带 `instance_id`
- [x] `GET /api/traces`：turn trace 历史列表
- [x] `GET/POST /api/checkpoints` + `POST /api/checkpoints/load`
- [x] `GET/POST /api/presets` + `POST /api/presets/apply`
- [x] `web/` 作者工具面板：
  - checkpoint / rollback
  - scenario preset 保存与套用
  - trace turn 历史浏览
- [x] `web/` trace 作者控制台第二轮：
  - 当前 turn 高亮
  - 上一轮 / 下一轮切换
  - checkpoint 与 trace 联动跳转
