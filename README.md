# CoreRP — Persistent Narrative Runtime

世界状态驱动的 LLM 叙事引擎。CoreRP 不是 prompt UI，也不是并发多 agent 群聊工具；它的目标是一个**可回放、可分叉、可解释**的文字世界运行时。

**状态：Experimental / Prototype。** 核心 runtime 已成型，仍在迭代。

## 文档导航

- [ARCHITECTURE.md](ARCHITECTURE.md): 当前分层架构与技术选型
- [ARCHITECTURE_RUNTIME.md](ARCHITECTURE_RUNTIME.md): runtime 合约、event/replay/fork 约束
- [FINAL_ARCHITECTURE_BLUEPRINT.md](FINAL_ARCHITECTURE_BLUEPRINT.md): 项目的目标终态蓝图
- [AGENTS.md](AGENTS.md): 仓库协作与贡献约束
- [SESSION_LOG.md](SESSION_LOG.md): 变更会话记录

## 核心原则

- 世界状态只允许通过 committed events 变化
- LLM 只负责受约束的规划和渲染，不直接写世界真相
- 一轮对话可以有多个角色参与，但必须按 `TurnStep` 严格串行执行
- replay、fork、trace 都必须可解释、可复现

当前 runtime 已经从“单轮单角色回复”演进为：

```text
DirectorPlan -> TurnStep[0..n] -> serial execution -> event commit -> reprojection
```

同时，runtime 现在已经引入**显式实例层**：

```text
RuntimeInstance(id) -> instance-scoped state/event/memory/save/branch data
```

目前默认实例为 `default`，绝大部分 runtime API 支持通过 `?instance_id=<id>` 显式访问指定实例。

当前运行基线：

- `data/` 已重建为新的干净运行目录
- 旧测试库已删除，不再作为恢复目标
- PM2 当前固定使用标准 `data/` 启动

如果你要看完整设计，不要继续从 README 推断，直接读 [FINAL_ARCHITECTURE_BLUEPRINT.md](FINAL_ARCHITECTURE_BLUEPRINT.md)。

## 快速开始

```bash
go build -o corerp ./cmd/corerp
./corerp serve -characters ./characters -secure-cookie=false
```

打开 `http://localhost:8080`。

```bash
export LLM_URL="https://your-api/v1"
export LLM_API_KEY="your-key"
export LLM_MODEL="model-name"
export CORERP_AUTH_KEY="your-password"  # 可选
```

默认登录页是 `/login`。如果没有显式设置 `CORERP_AUTH_KEY` 或 `-auth-key`，默认密码是 `admin`。

## 导入角色卡

当前 `import` 会把 SillyTavern PNG/JSON 卡拆成 CoreRP 的角色卡与世界卡：

```bash
./corerp import -src ./card.json -dst ./characters
./corerp import -src ./cards_dir -dst ./characters
./corerp import -src ./card.json -dst ./characters -interactive
```

默认模式是 `auto`：

- 单角色卡：`1 character + 1 world`
- 群像/大世界卡：`multiple characters + 1 world + cast_index.yml`

`-interactive` 会先展示自动判断结果，再决定是否改成 `single` 或 `ensemble`。

## 前端与运行台

`web/` 是单页运行台，不承载世界规则。当前 UI 重点是：

- 场景、角色、聊天流和输入区
- 首页已收成更简约的编辑风主版面：
  - 顶部“运行数据”默认折叠
  - 主区优先保留场景 headline、对话流和输入区
  - 超窄屏下角色摘要默认折叠
- 侧栏查看 trace、memory、timeline、LLM 配置、存档和世界信息
- 侧栏已接入实例管理，可直接 create / set default / stop / delete runtime instance
- 顶栏已支持显式实例选择，运行台请求会自动附带当前视图实例的 `instance_id`
- `step_traces` 已接入前端，可按 step 查看 speaker、kind、action、validator 和 committed events
- 作者工具已接入前端：
  - checkpoint create / rollback
  - scenario preset save / apply
  - trace turn 历史浏览
  - 当前 turn 高亮、上一轮 / 下一轮切换、checkpoint 与 trace 联动

静态资源默认禁缓存，移动端侧栏会收成抽屉。

## 主要端点

- `POST /api/chat`: SSE 流式对话
- `GET /api/health`: 存活探针
- `GET /api/ready`: 就绪探针
- `GET /api/version`: 服务版本信息（`version / commit / time`）
- `GET /api/state`: 当前世界状态
- `GET /api/instances`: 列出 runtime instances
- `GET /api/instances/status`: 查询单个实例状态
- `POST /api/instances/create`: 从现有实例创建新实例
- `POST /api/instances/default`: 切换默认实例
- `POST /api/instances/stop`: 停止实例 tick loop
- `POST /api/instances/delete`: 删除实例及其实例级数据
- `GET /api/trace/latest`: 最近一轮 trace
- `GET /api/trace?turn=<n>`: 指定轮次 trace
- `GET /api/traces`: 最近若干轮 trace 历史
- `GET/POST /api/checkpoints`: 列出或创建 checkpoint
- `POST /api/checkpoints/load`: rollback 到指定 checkpoint
- `GET/POST /api/presets`: 列出或创建 scenario preset
- `POST /api/presets/apply`: 套用 scenario preset
- `GET/POST /api/saves`: 列出或保存存档
- `POST /api/saves/load`: 载入存档
- `GET /api/characters`, `POST /api/switch`: 角色列表与切换
- `GET/POST /api/player-role`: 用户身份配置

更完整的运行时行为和接口语义，请看 [ARCHITECTURE_RUNTIME.md](ARCHITECTURE_RUNTIME.md) 与 `api-contract.yaml`。

### 实例化说明

- `data/instances/<instance_id>/`：实例级文件持久化（如 `player_role.json`、`save_slots.json`）
- SQLite 共享 `data/memory.db`，但 `events / branches / dialogue / working_memory / semantic_facts / episodic_events / pending_facts` 已按 `instance_id` 隔离
- 实例摘要现在包含 `status=running|stopped`
- 默认实例兼容旧数据：`instance_id=''` 的历史记录会被 `default` 实例读取

## 开发与验证

```bash
go test ./...
go test -race ./...
node --check web/app.js
./deploy/smoke-check.sh
```

当前 UI 回归基线：

- 浏览器自动化暂时未在这台机器上跑 Chromium
- 当前使用 `jsdom` 做等价 UI 回归，已覆盖：
  - checkpoint / rollback
  - scenario preset 保存 / 套用
  - trace 历史切 turn
  - trace 作者控制台高亮 / 轮切换 / checkpoint 联动
  - 首页折叠与瘦身后主链路可用
- 原因是本机缺 Chromium 运行库（如 `libnspr4.so`）

本地开发常用运行方式：

```bash
./corerp serve -characters ./characters -secure-cookie=false
```

PM2 重建/固化当前启动参数：

```bash
./deploy/pm2-start-corerp.sh
pm2 show corerp
```

服务启动日志现在会打印 `version / commit / build_time / data / port`，便于排查 PM2 启动与部署漂移。

## 目录概览

```text
corerp/
├── cmd/corerp/                 # CLI entry
├── internal/runtime/           # DirectorPlan / TurnStep / orchestration
├── internal/events/            # event store / replay / fork / hash
├── internal/state/             # world projection and state machine
├── internal/actions/           # ActionFrame + executor
├── internal/agents/            # identity / speaker selection / planner
├── internal/memory/            # short-term / semantic / episodic
├── internal/emotion/           # pressure / residue / unresolved threads
├── internal/api/               # HTTP + SSE routes
├── web/                        # single-page runtime console
├── characters/                 # imported/authored content
└── data/                       # SQLite and runtime data
```

## 许可证

MIT
