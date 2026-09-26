# CoreRP · 严肃世界内核与沉浸式数字生命引擎

> **状态**: **全阶段架构冻结 (F0–F13 Complete · Architecture Frozen)**
> **基准**: `16ac77f` | **验收结论**: `IMPLEMENTATION_COMPLETE_WITH_LIVE_VALIDATION_PENDING`
> **核心原则**: 单一事实源（Single Source of Truth）、不可变事件账本（Event Ledger）、确定性投影自愈（Zero-Drift Projections）、多模型居民共存（Multi-Controller Residency）。

---

## 一、项目愿景与核心理念

**CoreRP** 是一个基于 **Go 1.24+ 与轻量嵌入式 SQLite** 构建的严肃角色扮演世界内核与社会动力学引擎。它旨在解决传统大模型 RP / 虚拟世界中普遍存在的“模型幻觉篡改事实”、“状态缺乏持久物理约束”、“跨智能体通信全知作弊”以及“模拟长跑经济崩溃”等根本性问题。

### 核心不变量 (Core Invariants)
1. **单一事实源 (Single Source of Truth)**: 所有的世界变化必须首先提交为不可变事件账本（`events`）中的有序分录；所有内存状态与业务表仅是只读派生投影，随时可通过清空数据表完全重建（`RebuildProjections`）。
2. **纯粹确定性与零漂移 (Deterministic & Zero Drift)**: 在 30 个世界日、经历 3 次 SQLite 强退重启的综合仿真中，`CompareProjections` 差异严格保持为 **0**。
3. **物理与认知封闭性 (Epistemic Containment)**: NPC 不具备全知能力。面对面交谈、点对点信件、一跳传闻、组织内部公告、法律通告均走物理隔离的信道分发；知识泄漏指标严格为 **0**。
4. **复式记账法经济闭环 (Double-entry Conservation)**: 每一笔工资发放、租金扣缴与商品采购均严格双向平账，资金守恒和严格为 **0**，无凭空造币。
5. **模型行为严格受制于法度 (Lawful Agent Bounds)**: 外部大语言模型（LLM）或人类玩家的提议仅作为动作意图（Proposal），必须经过系统权限、规则纪元与时序门闸校验后才可沉淀为事件，模型输出不可直接修改世界事实。

---

## 二、24 项冻结核心职责 (24 Frozen Core Responsibilities)

经过 F0 至 F13 路线图的全阶段打磨，CoreRP 内核职责已正式宣告**冻结**，后续任何新业务逻辑全面转入 **Pack-First 扩展包** 模式：

```
                ┌──────────────────────────────────────────────┐
                │          CoreRP Architecture (Frozen)        │
                │        24 Core Authoritative Responsibilities│
                └──────────────────────┬───────────────────────┘
                                       │
     ┌──────────────────┬──────────────┴─────┬─────────────────┬──────────────────┐
     ▼                  ▼                    ▼                 ▼                  ▼
┌───────────┐    ┌─────────────┐      ┌─────────────┐   ┌─────────────┐    ┌─────────────┐
│World Pack │    │ System Pack │      │Content Pack │   │Narrative Pack│   │Tech Adapter │
└───────────┘    └─────────────┘      └─────────────┘   └─────────────┘    └─────────────┘
```

| 序号 | 核心职责 | 权威边界与实现机制 |
| :--- | :--- | :--- |
| 1 | **World Registry** | 注册并维护世界、分支与时间线；严格管理 Epoch 规则锁与分支隔离。 |
| 2 | **Entity / State** | 人物、地点、组织等实体的唯一标识与基础生命周期管理。 |
| 3 | **Event Ledger** | 唯一的不可变真理账本；自增全局序列号，支持原子写入与断点回溯。 |
| 4 | **Time / Scheduler** | 全局统一离散世界时钟；多参与者共享时序，Due Queue 到期精确调度。 |
| 5 | **Spatial / Travel** | 离散地点拓扑、有向时间通行边（Timed Edge）、中间遭遇点与惰性实例化。 |
| 6 | **Rule / Institution** | 法律条例制定、议会表决生效、权能范围裁定与违宪拦截。 |
| 7 | **Economy / Ownership** | 复式记账法（Double-entry Bookkeeping），严格保持资金守恒（和为 0）。 |
| 8 | **Agent Runtime** | 代理自主决策循环、目标推导、行为提议与内部状态持久化。 |
| 9 | **Knowledge / Belief** | 严格依附于真实物理通道的认知与主观信念（相信/怀疑/否决）。 |
| 10 | **Relationship** | 人物间的熟悉度、历史交互印象与动态社交关系网络演化。 |
| 11 | **Household Contract** | 共同家庭成立、同住租金分摊负债、成员自愿退出与家庭经济压力传递。 |
| 12 | **Body / Health Contract** | 睡眠周期、睡眠负债与精力状态；疲劳对决策及排班出勤的实质性约束。 |
| 13 | **Education / Qualification** | 技能培训课程、考核评定、权威证书颁发与工作准入门槛硬核校验。 |
| 14 | **Information / Network** | 五大信道隔离分发（面谈、私信、传闻中继、组织通知、法律公告），零泄露。 |
| 15 | **Organization Agency** | 组织运营策略配置、储备金动态监控、自适应招聘扩编/冻结决策。 |
| 16 | **Emergence / Cohort** | 群体生命周期演变、人口动态迁移与背景群体按需实例化。 |
| 17 | **Encounter** | 同一物理空间及通行路段上的真实相遇判定；杜绝跨空间全知感知。 |
| 18 | **Observation / Narrative** | 多视角感知过滤、风格化长篇纪实生成；表达形式切换不篡改事实。 |
| 19 | **RP Session / Turn** | 人机交互轮次、动作意图解析、Human 门闸裁决与幂等重试。 |
| 20 | **Extension Registry** | 纯数据 DLC 扩展包声明式校验、哈希验签、Epoch 生命周期锁定与热装载。 |
| 21 | **Gateway / Client Protocol** | 标准化通信契约（REST API、MCP Stdio/SSE、SillyTavern 酒馆适配）。 |
| 22 | **Studio / Inspector** | 创作者视界与事件因果链探针；支持规则纪元穿透追溯。 |
| 23 | **World QA / Observer** | 14 维度只读模拟健康诊断系统与 5 视角宏观旁观者模式。 |
| 24 | **Persistence / Recovery** | SQLite 事务日志、崩溃后自愈重建与长期状态零漂移重放。 |

---

## 三、核心架构与系统机制

### 1. 物理拓扑与时空移动 (Spatial & Travel)
- **非连续跳跃限制**: 实体在地点间移动依赖有向时间通行边（Timed Edge）。
- **路段占用与中途遭遇**: 引入中间遭遇点（例如 `街区林荫道`），实体在行进期间处于半开区间占用状态，可在路途中被同向或逆向旅者真实目击并打断交互。
- **惰性实体实例化**: 地点在未被观测时保持轻量占位，当受到玩家或智能体探索时进行单调递增的安全实例化，杜绝重复创建。

### 2. 多控制器共存与时序轮次 (Multi-Controller & Shared Time)
- **三方平等参与**: 支持人类玩家（Human）、内部自主 NPC（Internal AI）与外部模型代理（MCP Resident Controller）共处同一世界。
- **共享轮次门闸 (Shared Round Barrier)**: 在共享时间窗口内，各参与者私密提交行动提案，必须经由人类决策门闸裁决并由调度器统一推进时间。
- **单调递增会话换代 (Generation Monotonicity)**: 当智能体控制权变更时，旧会话凭据作废，保证所有未结事务的唯一归属。

### 3. 生活契约闭环 (Household, Health, Education, Career)
- **家庭与租金共担 (Household)**: 支持多成年人共同组建家庭并签署租金协议。家庭总负债按比例分摊，失业或收入变动直接改变家庭财务压力级别（`COVERED` / `AT_RISK`），进而驱动智能体产生“寻找高薪工作”或“缩减开支”的决策意图。
- **生理健康与作息 (Body & Sleep)**: 明确的睡眠起止事件。两日连续短睡眠将累积“睡眠负债（Fatigue）”，使得智能体在傍晚自主拒绝社交邀请，并在工作岗位上产生工伤或失误重检记录。
- **教育准入与职业雇佣 (Education & Career)**: 严格区分“培训学习”、“考核发证”与“岗位录用”。未取得相关资格证书（如 `safety_training`）的候选人将在面试阶段被规则引擎硬性拒绝。

### 4. 信息网络与认知封闭 (Information Network & Containment)
- 杜绝传统游戏中的全局变量广播。系统划分为 5 类严格隔离的信道：
  1. **面对面交谈 (Co-location Speech)**: 仅物理同处于一室的实体可听见；
  2. **点对点私信 (Direct Message)**: 经由邮政/网络调度延迟送达，仅收件人可见；
  3. **一跳传闻 (One-hop Rumor)**: 须经发件人明确授权中继，才可向第三方有限转述；
  4. **组织内部通知 (Organization Announcement)**: 仅具有当前有效劳动合同的在职员工可查询；
  5. **公共法律通告 (Public Notice)**: 机构正式颁布后在公告栏公示，需主动阅读后获得。
- **主观信念分离**: 智能体对收到的信息独立记录 `believe`、`doubt` 或 `reject`，角色主观信念不代表世界客观事实。

### 5. 组织代理与自主运营 (Organization Agency)
- 商业实体依据管理层设定的运营策略（Policy）独立运转。
- 调度器定期触发组织审计，依据复式记账法下的真实账面可用现金、待付工资负债与人力规模，自动决策“扩编招聘”、“维持现状”或“冻结招募/裁员”，并向员工及市场受控发布通告。

### 6. 纯数据扩展包体系 (DLC Pack Platform)
- 实现了真正的 **Pack-First 扩展架构**。新增世界规则与玩法无需修改宿主 Go 或 Vue 代码：
  - **Content Pack**: 如 F9 Retail Career 岗位包，声明岗位、资格、工时与晋升路径；
  - **Narrative Pack**: 如 F9 Life Journal 纪实叙事包，提供个性化第一人称长文渲染风格；
- **平台生命周期防护**: 声明式依赖校验、SHA-256 结构验签、Epoch 规则锁锁定、升级失败回滚与卸载历史引用保持。

### 7. 世界体检与旁观者模式 (World QA & Observer Mode)
- **World QA 14 维度体检**: 只读诊断系统，实时扫描人口、岗位、复式分录、家庭租金、住房覆盖、交通通行边、关系图谱密度、事件多样性、智能体决策分布、知识泄漏、组织决策、失败动作、地点可达性与经济守恒。
- **Observer 5 视角旁观**: 提供宏观大事件时间线、个人生涯小传、机构收支汇总、关系变迁网与周期性世界文摘，所有条目直通 Studio Inspector 事件因果链。

---

## 四、工程验证与验收结论 (Verification Matrix)

本项目在开发全流程中执行严苛的工程自检，所有阶段产物均经自动化套件确认：

| 验证模块 | 测试范围与指令 | 运行结果与耗时 |
| :--- | :--- | :--- |
| **F12 综合长期集成** | `TestF12ComprehensiveUnifiedLongRun` (30世界日+3次重启+全子系统) | **PASS** (10.077s, 0 漂移) |
| **F11 教程全自动重现** | `TestF11TutorialReproduction` (纯净目录 9 步全生命周期) | **PASS** (0.808s) |
| **HTTP 传输与认证路由** | `go test ./internal/transport/httpapi -count=1` (54/54 用例) | **PASS** (52.245s) |
| **核心算法与决策引擎** | `go test ./internal/core ./internal/decision ./internal/narrative` | **PASS** (< 1.0s) |
| **长期并发竞态检测** | `go test -race` (针对 F10, F11, F12 核心存储与调度) | **PASS** (320.865s, 0 竞争) |
| **静态分析与代码格式** | `/usr/local/go/bin/go vet ./...` 及 `git diff --check` | **PASS** (0 错误, 0 告警) |
| **前端类型检查与打包** | `vue-tsc --noEmit && vite build` (Vue 3 生产构建) | **PASS** (84 模块编译通过) |
| **MCP 标准客户端套件** | `npm test` 在 `clients/mcp` (Stdio 协议与控制器隔离) | **PASS** (4/4 tests, 19.811s) |
| **SillyTavern 酒馆适配器** | `node --test client.test.js` 在 `clients/sillytavern` | **PASS** (5/5 tests, 0.125s) |

> **Live Model 说明**: 当前环境保持 `IMPLEMENTATION_COMPLETE / LIVE_VALIDATION_PENDING`，工程结构完全对接标准 LLM OpenAI/Anthropic/Step 网关，不伪造线上验证结论。

---

## 五、技术栈与目录结构

### 技术栈选型
- **核心后端**: Go 1.24+ / modernc.org/sqlite (纯 Go 驱动，零 CGo 依赖，极致跨平台部署)
- **前端工作台**: Vue 3 / TypeScript / Vite / TailwindCSS 原生语义样式（无臃肿重型组件库）
- **通信协议**: RESTful HTTP, Server-Sent Events (SSE), Model Context Protocol (MCP stdio/SSE)
- **文档构建**: 纯 Go 自主研发静态站点生成器 (`backend/cmd/manual-gen`)

### 核心目录一览
```text
.
├── backend/                  # Go 核心后端与世界服务
│   ├── cmd/
│   │   ├── corerp-server/    # 生产/本地 HTTP API 主服务
│   │   └── manual-gen/       # 离线单文件静态手册生成器
│   └── internal/
│       ├── core/             # 领域核心实体与 DTO 定义
│       ├── decision/         # 智能体决策提议与 Provider 适配
│       ├── narrative/        # 叙事表达流与风格过滤器
│       ├── storage/          # SQLite 引擎、56 个 Schema 迁移与事件账本
│       └── transport/httpapi # HTTP 路由、鉴权与 SSE 服务
├── clients/                  # 多端接入适配器
│   ├── mcp/                  # 官方 Model Context Protocol (MCP) 服务器
│   └── sillytavern/          # SillyTavern (酒馆) 原生对接脚本
├── docs/                     # 架构规范、各阶段验收报告与设计账本
│   ├── f0/ ~ f13/            # F0 至 F13 详尽设计账本与验收交付报告
│   │   ├── f13/architecture-freeze.md  # 24 核心职责冻结规范
│   │   └── f13/final-report.md         # 终结综合验收报告
│   └── manual/               # 单一信源 Markdown 手册正文
│       ├── index.md          # 概述与快速上手
│       ├── player.md         # 玩家指南
│       ├── creator.md        # 创作者与世界编排指南
│       ├── external-agent.md # 外部模型与 MCP 接入指南
│       ├── maintainer.md     # 系统架构与维护者交接指南
│       └── dist/index.html   # 预编译生成的单文件离线 HTML 手册 (78 KB)
├── src/                      # 前端 Vue 3 源码 (Play 客户端 + Studio 工作台)
├── public/                   # 前端静态资源
└── package.json              # 前端工程与测试依赖
```

---

## 六、快速开始 (Quickstart)

### 1. 环境准备
- 安装 **Go 1.24+** (或使用绝对路径 `/usr/local/go/bin/go`)
- 安装 **Node.js 20+** 与 `npm`

### 2. 构建与运行后端
```bash
cd backend

# 执行核心单元测试与重现测试
/usr/local/go/bin/go test -v ./internal/storage -run TestF11TutorialReproduction

# 启动 CoreRP 后端服务 (默认监听 :8080)
/usr/local/go/bin/go run ./cmd/corerp-server
```

### 3. 构建与运行前端 Play / Studio
```bash
# 在项目根目录下安装依赖
npm install

# 启动本地开发热更新服务器 (默认监听 :5173)
npm run dev

# 生产环境编译打包
npm run build
```
打开浏览器访问 `http://localhost:5173/` 即可进入沉浸式 **Play** 页面；访问 `http://localhost:5173/studio` 可进入 **Studio / Inspector** 创作与审查控制台。

### 4. 编译与浏览离线开发手册
```bash
# 运行纯 Go 静态站点生成器
cd backend && /usr/local/go/bin/go run ./cmd/manual-gen

# 生成产物位于 docs/manual/dist/index.html，可直接用任意浏览器双击打开浏览！
```

---

## 七、多端接入生态 (Clients & Interfaces)

### 1. Model Context Protocol (MCP)
CoreRP 内置完整的官方 MCP Server，可无缝挂载至 Claude Desktop、Cursor、Cline 等支持 MCP 的宿主环境中：
```bash
cd clients/mcp
npm install
npm test   # 运行标准 Stdio 自动化套件
```
详细接入参数与可用工具列表见 [clients/mcp/README.md](clients/mcp/README.md)。

### 2. SillyTavern (酒馆)
通过轻量代理脚本，可直接将 CoreRP 的角色、上下文与事件流桥接至酒馆界面：
```bash
cd clients/sillytavern
node --test client.test.js
```
详细配置见 [clients/sillytavern/README.md](clients/sillytavern/README.md)。

---

## 八、完整文档导航 (Documentation Hub)

- **核心规范**:
  - [架构冻结规范 (Architecture Freeze)](docs/f13/architecture-freeze.md)
  - [终结综合验收报告 (Final Sign-off Report)](docs/f13/final-report.md)
- **用户手册 (Markdown 单一信源)**:
  - [玩家游玩手册 (Player Guide)](docs/manual/player.md)
  - [世界创作者指南 (Creator Guide)](docs/manual/creator.md)
  - [外部 Agent 接入指南 (External Agent Guide)](docs/manual/external-agent.md)
  - [系统维护者手册 (Maintainer Manual)](docs/manual/maintainer.md)
- **编译产物**:
  - [独立静态单文件 HTML 手册 (Compiled Handbook)](docs/manual/dist/index.html)

---

## 九、架构冻结声明 (Architecture Freeze Declaration)

```text
========================================================================================
Core architecture frozen.
Future feature work defaults to Pack-first unless a concrete unmet invariant requires a core RFC.
========================================================================================
```

即刻起，CoreRP 底层内核逻辑正式停止扩充，全面转向以 **World Pack / System Pack** 为核心的内容与玩法创造时代！

---
*Co-Authored-By: Claude Code <noreply@anthropic.com>*
*Last Updated: 2026-09-26*
