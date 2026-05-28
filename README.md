# CoreRP — Persistent Narrative Runtime

世界状态驱动的 LLM 叙事引擎。CoreRP 不是 prompt UI，也不是并发多 agent 群聊工具；它的目标是一个**可回放、可分叉、可解释**的文字世界运行时。

**状态：Experimental / Prototype。** 核心 runtime 已成型，仍在迭代。

## 产品方向

CoreRP 不是“高级酒馆”，也不以预写任务树为核心。目标体验是：

```text
一个会长期演化、可回放、可分叉、人物会被经历改变的文字世界 runtime。
```

当前方向是 world-first：

- 世界是入口，角色是世界中的实体和运行时视角
- 导入内容优先沉淀为 world seed / canon / scene，而不是直接变成可选角色
- NPC 后续应从低分辨率 population 中被玩家关注或事件卷入，再晋升为主要角色
- LLM 不拥有世界，只提出 ActionFrame 和渲染表达；世界真相只能由 committed events 改变

这也意味着一个重要边界：

- 人物定义负责“这个人是谁、怎么说、容易被什么影响”
- 场景负责“现在在哪、谁在场、局势怎样发展”
- 运行时不应继续由人物定义主导场景真相

当前 world 目录已预留 `population/`：

```text
worlds/<world>/
  world/
    director.yml
  population/
    background_npcs.yml
    promoted_npcs.yml
    identity_core.yml
    policy.yml
```

这层用于承载“低分辨率 NPC -> 晋升主要角色 -> 经历塑造人格”的骨架。

`world/director.yml` 用于定义该世界自己的 director 风格，例如更偏向：

- 现场人物
- 当前 pressure 相关人物
- 势力控制链上的人物
- hook 命中的人物

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

当前默认运行倾向：

- Director 默认为 `auto_chain`
- 一轮允许 1-2 个发言 step，优先从当前场景角色和已晋升 population 中选择
- 背景人口为空时，会按当前 world/scene 结构自动补一批低分辨率 NPC，避免 runtime 永远没有“可生长的人”

当前 `participants` 语义也已经开始从“纯名字列表”过渡到“结构化参与者”：

- 旧字段 `participants` / `characters` 仍保留，继续输出字符串数组做兼容
- 新字段 `participant_details` 已输出结构化参与者摘要
- 每个参与者会显式标注：
  - `kind`: `player | npc | persona`
  - `source`: `player_role | character_definition | promoted_population | background_population | scene_shell | scene_presence`
  - `loaded / switchable / present / focus`

这层的目的不是多一份 UI 数据，而是让 switch、director、作者控制台最终共享同一套参与者语义。

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
./corerp serve -boot world -world ./worlds/neon_block -secure-cookie=false
```

打开 `http://localhost:8080`。

```bash
export LLM_URL="https://your-api/v1"
export LLM_API_KEY="your-key"
export LLM_MODEL="model-name"
export CORERP_AUTH_KEY="your-password"  # 可选
```

默认登录页是 `/login`。如果没有显式设置 `CORERP_AUTH_KEY` 或 `-auth-key`，默认密码是 `admin`。

## 导入人物定义

当前 `import` 会把 SillyTavern PNG/JSON 卡拆成 CoreRP 的人物定义与 world seed：

```bash
./corerp import -src ./card.json -dst ./characters
./corerp import -src ./cards_dir -dst ./characters
./corerp import -src ./card.json -dst ./characters -interactive
```

默认模式是 `auto`：

- 单角色输入：`1 character + 1 world`
- 群像/大世界卡：`primary cast characters + 1 world + cast_index.yml`
- 世界资料卡：`world-only`，只写入 `worlds/<source>/`，不生成顶层可选视角

导入目录会处理 `.png` 和 `.json`。每个生成人物定义都会写入 `world_path`，运行时优先按该字段绑定到导入生成的 `worlds/<source>/` 世界目录；世界书里的地点、设定、规则、物品和事件保留在 world ontology/canon 中，不应作为顶层可选视角加载。

`-interactive` 会先展示自动判断结果，再决定是否改成 `single` 或 `ensemble`。

### 人物定义与场景的职责

导入后的人物定义仍然有用，但它的职责已经收窄：

- 提供人物初始 identity：气质、表达风格、禁忌、少量 immutable/adaptive
- 提供导入时的 world seed 线索

它不再应该在运行阶段反复覆盖当前 scene。当前 scene 的推荐优先级是：

```text
projected scene > active world default scene > imported card fallback
```

也就是说：

- 人物定义可以作为导入期和兜底期的来源
- 运行中的地点、时间、天气、在场人物、局势，应以 `world/scene + projection + committed events` 为准

## 前端与运行台

`web/` 是单页运行台，不承载世界规则。当前 UI 重点是：

- 场景、视角、聊天流和输入区
- 顶栏已开始转向 world-first，新增世界目录 selector；参与者 selector 暂时保留为兼容视角切换
- 首页已收成更简约的编辑风主版面：
  - 顶部“运行数据”默认折叠
  - 主区优先保留场景 headline、对话流和输入区
  - 超窄屏下视角摘要默认折叠
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
- `GET /api/worlds`: 列出本地 world catalog
- `GET/POST /api/population`: 读取或更新当前世界的人口层配置
- `GET /api/population-insights`: 读取当前世界的人口晋升/生长观测数据
- `GET/POST /api/world-structure`: 读取或更新当前世界的 world seed / factions / locations / pressures
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
- `GET /api/characters`, `POST /api/switch`: 当前场景参与者与视角切换
- `GET/POST /api/player-role`: 用户身份配置
- `GET /api/sim/status`, `POST /api/sim/tick`, `POST /api/sim/pause`, `POST /api/sim/resume`: simulation 运维接口

更完整的运行时行为和接口语义，请看 [ARCHITECTURE_RUNTIME.md](ARCHITECTURE_RUNTIME.md) 与 `api-contract.yaml`。

### 参与者模型说明

当前推荐把参与者分成四层看：

- `focus_character`: 当前观察视角
- `participants`: 当前 scene 里的在场实体
- `participant_details`: 参与者结构化摘要，供 switch / director / UI 共用
- `character definition / promoted persona / background npc / scene shell`: 这些是参与者的来源，不是同一种东西

当前规则：

- 切视角不会再重写 scene 真相；原本在场的人会继续留在 `participants`
- `scene_presence` / `scene_shell` 可以作为场景存在，但不会自动进入 director 候选
- `player_role` 参与者不可切换
- director 当前优先从 `persona`、已晋升 population、命中当前 pressure/location 的背景 NPC 里选 speaker

### 实例化说明

- `data/instances/<instance_id>/`：实例级文件持久化（如 `player_role.json`、`save_slots.json`）
- SQLite 共享 `data/memory.db`，但 `events / branches / dialogue / working_memory / semantic_facts / episodic_events / pending_facts` 已按 `instance_id` 隔离
- 实例摘要现在包含 `status=running|stopped`
- 默认实例兼容旧数据：`instance_id=''` 的历史记录会被 `default` 实例读取

## 确认工作流

后续修改默认按这条路线推进，避免架构语义反复切换：

1. 先保证内部真实语义统一到 `focus_character / participants / focus_definition`。
2. 再给 API 输出补新字段；旧字段如 `active_character / character / loaded_characters` 只保留兼容镜像。
3. 前端始终优先读取新字段，旧字段只作为 fallback。
4. 文档和契约描述跟着代码一起更新，避免出现“代码已经 world-first，文档还在角色卡中心”的漂移。
5. 最后才考虑新增 v2 路径或删减兼容层；在此之前不要贸然删除旧 API 路径和旧 JSON 字段。

推荐执行顺序：

1. 补 `api-contract.yaml` 和导出结构里的 `focus_*` 字段。
2. 前端把 `focus_*` / `participants` 作为第一读取来源。
3. runtime/core 注释、handler、兼容别名继续去 `active character` 语义。
4. 强化 population runtime 闭环：background NPC -> promoted persona -> director candidate -> 持久化。
5. 强化 world authoring 工作流，让 world/scene/population 成为主入口。
6. 把 `participants` 从字符串列表彻底收口到结构化参与者模型。
7. 兼容层稳定后，再评估是否新增纯 `focus-*` 新路径。

硬约束：

- 不回退 world-first 架构。
- 不删除旧 API 路径和旧 JSON 字段，只能把它们降级为兼容层。
- 新实现优先使用 `focus_character`、`participants`、`focus_definition`。
- `loaded_characters`、`active_character`、`character` 只做兼容镜像，不再驱动主逻辑。

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

后续每完成一批修改，最少执行：

```bash
/usr/local/go/bin/go test -count=1 ./internal/api ./internal/runtime ./internal/core
node --check web/app.js
/usr/local/go/bin/go build -o corerp ./cmd/corerp
pm2 restart corerp && pm2 save
```

建议追加抽查：

```bash
curl /api/characters
curl /api/instances
curl '/api/memory?facts=1&episodic=1&dialogue=1'
curl /api/checkpoints
curl /api/presets
curl '/api/export?format=json&limit=1'
```

预期原则：

- 新字段存在
- 旧字段仍兼容存在
- 前端优先显示新字段
- PM2 服务能正常启动

本地开发常用运行方式：

```bash
./corerp serve -boot world -world ./worlds/neon_block -secure-cookie=false
```

兼容旧模式时，仍可显式使用人物定义启动：

```bash
./corerp serve -boot character -characters ./characters -secure-cookie=false
```

PM2 重建/固化当前启动参数：

```bash
./deploy/pm2-start-corerp.sh
pm2 show corerp
```

服务启动日志现在会打印 `version / commit / build_time / data / port`，便于排查 PM2 启动与部署漂移。

## DCL Mods

CoreRP 支持第一版声明式 DCL world pack。DCL 不执行脚本，只安装 YAML
patch 和 hook 声明，避免把 mod 变成不受控代码入口。

```text
mods/
└── looping_isekai_return.dcl/
    ├── manifest.yml
    ├── patches/
    │   ├── world.yml
    │   ├── population.yml
    │   ├── scenes.yml
    │   └── presets.yml
    └── logic/
        └── hooks.yml
```

API：

```text
GET  /api/dcl
POST /api/dcl/install   {"id":"looping_isekai_return","overwrite":false}
POST /api/dcl/upload    multipart file=<zip>, overwrite=false
POST /api/dcl/remove    {"id":"looping_isekai_return","delete_world":false,"delete_package":false}
```

安装后会生成一个普通 world 目录，后续仍走 world-first runtime、checkpoint、
experiment report 和 replay workflow。
作者控制台的 World 分组提供 DCL 面板，可上传 ZIP、查看详情、启用、关闭、
删除安装出的 world，或删除本地 `.dcl` 包目录。可以同时启用多个 DCL；每个
DCL 安装为独立 world，运行时通过 World 下拉选择进入，当前版本不会把多个
DCL 自动合并到同一个 world。

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
├── internal/dcl/               # declarative DCL loader
├── web/                        # single-page runtime console
├── mods/                       # installable DCL world packs
├── characters/                 # imported/authored content
└── data/                       # SQLite and runtime data
```

## 许可证

MIT
