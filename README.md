# CoreRP · Play 与世界内核

默认入口是连接真实 Go/SQLite 世界的 **CoreRP Play**。原有 Story / Inspector 契约演示保留在 `/demo`。
技术栈遵循《CoreRP v0.3.1 · M0 工程审计版》§24.1：**Vue 3 + TypeScript + Vite**，
不引入 UI 组件库，零运行时 CDN 依赖。

> Play 已接入持久会话、同场发言/NPC 回应、移动、等待和重启恢复。默认使用确定性 Provider，也可[配置真实 LLM 兼容接口](docs/rp2/README.md)；live 模型验收仍待配置。`/demo` 仍全部是构造示例，不应作为真实世界状态。启动方式见 [Play 本地指南](docs/rp1/play.md)。

## M0 契约 RFC

2026-09-22 的 P0 审计修订已整理为独立的 [M0 契约包](docs/m0/README.md)，包括核心 RFC、SQLite DDL、JSON Schema、T01–T12 测试向量和青玄界/现代街区两个最小 World Pack。M0 文件保留其冻结时的 `not_run` 状态；当前运行结果单独记录在 [M1 测试证据映射](docs/m1/test-evidence.json)，避免反写历史基线。

当前前端 fixture 已同步以下候选语义：Inspector `record_order` 与权威 `event_sequence` 分离；Rule Epoch 使用半开区间；LawActivationEvent 属于旧纪元；金额/库存使用最小单位整数；观察记录带显式 audience scope。静态检查命令为：

```bash
npm run verify:m0
```

## M1 最小内核

[`backend/`](backend/README.md) 现在包含独立的 Go/SQLite 严格世界内核：权威 genesis、稳定调度器、90 日工资/房租/预算消费/付费补货、采购事务、快照重放与投影修复、Outbox 恢复、受控发行及最小私密读取授权。Event、分录、库存流转、投影、Branch Head 与 Outbox 均在各自写事务内同成同败；另有独立 HTTP 进程提供认证命令、查询以及按权限过滤且可断线续传的 SSE。

它仍不提供生产身份/TLS、外部 LLM、规则热激活或跨币种金融；当前静态 Bearer Token 仅供本地开发。完整范围、API 契约和可复现命令见 [backend/README.md](backend/README.md)。

## M2 后端纵切

[`docs/m2/`](docs/m2/README.md) 已实现 Cohort 守恒、双 Agent 状态化遭遇、可显式启动的 30 日空间日程，以及独立启用的 30 日工资/房租因果结算（含真实欠薪、欠租）。它不调用 LLM，也不代表完整 M2；自主决策、生产常驻调度、Belief/语义记忆、消费/库存/补货、完整 LOD 经济、主观地位和 100 人/10 店规模门仍未完成。前端 fixture 尚未接入这些接口。

## RP-1 最小可玩闭环

[RP-1A–F](docs/rp1/README.md)在同一 M2 世界上提供真实玩家绑定、受限观察、移动、调度器等待、原子发言/听者认知、NPC 决策和连续回合，以及移动端 Play。本地切片经 T09 准备 1 名玩家、3 名 NPC、5 个地点。20 回合后端测试和真实浏览器/服务重启测试均已通过；未部署生产环境。`npm run verify:rp1-play` 可复现完整浏览器恢复场景。

## 当前 RP-6 产品体验

RP-8 已通过[本地阶段验收并提交检查点](docs/rp8/phase-report.md)：可从 `/studio` 打开[真实事件检查页](docs/rp8/studio-ui.md)，默认 `/` 仍是 Play。检查权限由[本地管理工具](docs/rp8/access.md)显式配置，支持 creator 内容与 ops 脱敏边界、撤权和重启恢复。输入凭证即可[选择授权世界、分支与时间线事件](docs/rp8/discovery.md)，无需手抄 ID。`/studio/create` 已有[可恢复创建界面](docs/rp8/creator-ui.md)，调用[真实创建 API](docs/rp8/creator-api.md)安装声明式 System/Narrative 包；玩家使用独立凭证在 Play 选择获授权世界。当前界面采用两个人物、两处地点的起始模板，不支持任意可执行插件或热升级；300 回合 / 30 世界日的最终长期实玩验收尚未完成。

RP-2～RP-8 已有阶段检查点，[RP-7](docs/rp7/phase-7.md) 外部客户端接入已通过本地阶段验收；Final Integration 与总体验收尚未完成。当前 schema030 保护[新世界保存与玩家授权来源](docs/rp8/readiness.md)；schema028/029 保护[包安装与激活](docs/rp8/packages.md)，schema027 的[请求永久停用机制](docs/rp7/requests.md)保留，不回滚世界。Play 已提供按需打开的钱包、工作信息、通讯录、手机工作通知和本地地图，以及叙事预设、视角、字节预算、流式读取、重试和不回滚世界的叙述重生成。

RP-7 已提供[酒馆扩展](clients/sillytavern/README.md)、[MCP 适配器与接入说明](clients/mcp/README.md)及[CoreRP RP Skill](.agents/skills/corerp-rp/SKILL.md)。[真实三客户端同实例、同人物及重启恢复](docs/rp7/compatibility.md)与[本地阶段验证](docs/rp7/verification.md)已通过；未发布到生产，也未声称嵌套 Codex 或真实 LLM 实玩。

自定义文风的可执行范围与人物决策配置分开：[独立文风解释器](docs/rp6/custom-style.md) 需要显式配置，只解释有限呈现偏好，不任意扩写事实。默认确定性叙述器会明确提示自由文风指令仅保存、不执行。手机不是私人聊天服务，地图不展示远处人物，完整边界见 [能力核对](docs/rp6/capability-audit.md)。

[104 回合、多日世界、重启及记忆/随机行为浏览器验收](docs/rp6/long-play.md)已通过；[阶段验证记录](docs/rp6/verification.md)区分已通过和仍在运行的检查。这些是本地服务证据，不是生产部署或 live 模型质量认证。

[Final Integration 验收](docs/final/acceptance.md)已通过本地验证：9 名 NPC、1 名玩家、3 个人口群体的同一世界完成真实 Play／MCP 308 回合、32.48 世界日、四次重启，串联职业、文化、临时法律、关系变化和两次罕见探访。全量后端 406 项测试与同库重建／事件不改写审计通过。多群体初始化为显式本地命令，不会自动扩充既有世界；不代表生产部署或真实模型语言质量认证。

## 历史 `/demo` 的两个工作空间

| | Story 叙事流 | Inspector 观测台 |
|---|---|---|
| 身份 | 玩家工作空间（默认） | 演示 / 创作者权限视角 |
| 主体 | 连续长文历史 + 自由行动 | 事件脊梁等六个区块 |
| 进入 | 打开即是 | Story 右栏「观测台」入口 / 任意 record_id 引用 |
| 返回 | 「← 返回叙事流」 | 回到原阅读位置与草稿 |

两边用 `v-show` 保持存活：切换不卸载组件，**历史、草稿、滚动锚点、脊梁选中都不丢**。

## Story：连续历史，不是单场景替换

- **按 `message_id` / 序号 / 来源 / 世界时间**表达多轮历史。新增响应只**追加**，
  发送不清空旧文，不用场景摘要覆盖，不只留最近一轮。
- **30+ 回合本地演示历史**，含一条 **3,792 中文字**的长回复（验收要求 3,000–5,000）。
- **来源可辨**：玩家 = 琥珀菱形脊线，世界 = 青色圆环，系统 = 灰色三角。
  （形状 = 来源，缩进 = 时间深度；消息不做卡片。）
- **阅读锚点**：往上读历史时，新内容**不拽你下来**——出现「有新内容 · 跳最新」浮条。
- **会话内状态**：草稿、滚动位置存 `sessionStorage`，刷新后返回原位；
  无锚点时落到最新，「回到最新」始终是明确按钮。

### 自由文字与快捷点击是同一个入口

两者都构造同一个 `ActionIntent` 并进入同一个 `dispatchAction()`：

```
src/lib/story.ts  →  parseIntent() / clickIntent() / dispatchAction()
```

`ActionIntent` = `{ kind, actor_id, target_id, params, source: 'click'|'text', raw, demo: true }`。
演示支持的意图：**观察 / 交谈 / 查看商品 / 等一会儿 / 移动 / 观测台 / 帮助**。

- 未支持的输入**不会**被写成世界真相：玩家回合照常入历史，随后追加一条 system 消息
  说明「未识别」与可用意图。没有 toast，没有假成功。
- 快捷动作随**场景**出现（语境层），不是固定四宫格。

### 展示模式（两种，同一历史）

`NarrativeMessage` 只有一份。`长文卷轴` / `酒馆对话` 只改呈现方式：
消息 id、顺序、文字、草稿都不变，切换**不重新生成、不重新结算、不调模型**。
两者均已实现并有验收（`verify.cjs` 路径 C）。

### 功能分层

| 层 | 内容 | 位置 |
|---|---|---|
| 阅读 / 行动 | 连续正文 + 自由输入 + 语境动作 | 第一层，始终可达 |
| 语境层 | 经济账本、随身物品 | 右栏 / 移动端抽屉 |
| 资料层 | 任务、扩展包、时间线 | 右栏 / 抽屉 |
| 权威层 | 观测台 | 右栏 / 抽屉 |

**未接通**的入口（随身物品、任务）保持可见可点，但给出明确说明并温柔横摇一次，
**不放假开关、不占空页面、不只弹 toast**。已接通的（经济 / 扩展包 / 时间线）
指向观测台的对应区块，不复制一份数据。

## Inspector：六个区块完整保留

`SpineDiagram` / `RecordInspector` / `EpochStrata` / `KnowledgeGraph` /
`EconomyChart` / `EntityGrid` —— 从原 `App.vue` 原样提取到 `InspectorWorkspace.vue`，
示例事实、节点选择、因果链、记录探针、图形与状态语义全部不变。

从叙事流任点一个 `record_id` 徽标，会带着该 id 进入观测台并展开记录探针。

## 数据边界

- `src/data/world.ts` = 权威 / 审计示例（**未改动**）。
- `src/data/story-demo.ts` = 隔离的叙事 fixture，`fact_refs` 只做可见引用，
  不写入、不改写权威记录。
- `NarrativeMessage` 与 `WorldEvent` 是两套模型，互不覆盖。
- 玩家视角只读可见字段：看不到 NPC 隐藏资产、组织现金或后台审计字段
  （`visibleEntity()` 做过滤）。
- 世界时间来自 demo snapshot，**不是**浏览器时间。

## Markdown 渲染

`src/lib/markdown.ts` 是自写的**白名单**渲染器：它只认识一小集合法结构
（标题 / 粗斜体 / 行内代码 / 围栏代码块 / 列表 / 表格 / 分隔线 / 引用 / 段落），
其余一律当纯文本转义输出。所以 `<script>`、`[x](javascript:…)` 永远不会进入
tag 或属性位置。链接只允许 `http/https/mailto` 且强制 `rel=noopener`。
代码块、长链接、表格都限定在可横滑容器内，不会横向撑破手机。

## 移动端

- 360×800 / 390×844 / 430×932 实测：长文、输入、跳最新、返回、触控均可用。
- 单条顶栏（世界时间 + 场景 + 抽屉开关）；抽屉带遮罩，点面板外收起，
  离开工作空间时自动收起。
- 底部输入条用 `env(safe-area-inset-bottom)` 避开手势区；
  软键盘弹起时压缩阅读区，输入始终可见。
- **不是缩放桌面版**：桌面 62/38 双栏，移动端右栏重组为可上拉抽屉。

## 命令

```bash
npm ci
npm run dev          # 开发
npm run build        # vue-tsc --noEmit && vite build
npm run preview      # 预览构建产物
node verify.cjs      # 浏览器验证（需先 preview 在 :4173）
```

`verify.cjs` 用 Playwright 真实驱动页面，按任务单路径 A–F + Inspector 回归 +
可访问性逐项断言并截图到 `shots/`。**全部 40 项 PASS，无控制台错误。**

## 源码结构

```
src/
├─ App.vue                    双工作空间外壳（v-show 保活）
├─ types.ts                   M0 契约数据模型 + ActionIntent / NarrativeMessage
├─ data/
│  ├─ world.ts                权威与审计示例（未改动）
│  └─ story-demo.ts           隔离叙事 fixture + 语境动作 + 入口清单
├─ lib/
│  ├─ markdown.ts             白名单 Markdown → HTML（XSS 安全）
│  ├─ story.ts                dispatchAction() 与演示意图解析
│  └─ format.ts               记录类型语义与格式化
├─ components/
│  ├─ StoryWorkspace.vue      玩家工作空间
│  ├─ StoryMessageTurn.vue    单条叙事回合（脊线 + 衬线正文）
│  ├─ InspectorWorkspace.vue  观测工作空间（原 App.vue 六区块）
│  ├─ SpineDiagram.vue        事件脊梁
│  ├─ RecordInspector.vue     记录探针
│  ├─ EpochStrata.vue         规则纪元地层
│  ├─ KnowledgeGraph.vue      认知渗透图
│  ├─ EconomyChart.vue        经济投影与报价台阶
│  └─ EntityGrid.vue          实体三口径表
├─ composables.ts             滚动锚点 / 键盘安全区 / 减弱动态
└─ styles/                    fonts.css + base.css（令牌）+ story.css（叙事流）
```

## 视觉方向

美术决策记录在 [DESIGN.md](DESIGN.md)（概念句、主角、母题、运动语言、密度、
响应式转变、反套路自检）。父任务单明确把视觉定稿留给后续 skills 审查，
本文档不宣称它已通过最终艺术指导验收。
