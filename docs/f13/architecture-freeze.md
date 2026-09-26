# F13 — CoreRP Architecture Freeze Specification

Status: **FROZEN**. Baseline is F12 checkpoint `d625a23703c387493c13fb12376c947fce8f09b9`.

---

## 15.1 冻结核心职责 (Frozen Core Responsibilities)

经过 F0 至 F12 的全阶段架构验证与长期联合测试，CoreRP 核心权威层已实现自洽、完备与闭环。

从即日起，CoreRP 内核职责正式宣告**冻结**。内核仅且必须保留以下 24 项一级逻辑职责，严格拒绝创建任何平行状态或第二权威源：

| 序号 | 核心职责 | 权威边界与不变量 |
| --- | --- | --- |
| 1 | **World Registry** | 注册并维护 World / Branch / Epoch 元数据；严格管理时间戳与版本锁，保证分支独立与隔离。 |
| 2 | **Entity / State** | 人物、地点、组织等实体的唯一标识与基础存在状态；状态派生均以事件溯源为唯一事实。 |
| 3 | **Event Ledger** | 唯一的不可变真理账本（Single Source of Truth）；严格单调递增，持久化所有已提交事实。 |
| 4 | **Time / Scheduler** | 全局统一世界时钟与计划队列（WaitRP、Due Queue）；保证确定性推进与所有参与者共享时序。 |
| 5 | **Spatial / Travel** | 离散地点拓扑、有向时间通行边（Timed Edge）、中间遭遇点、半开区间占用与惰性实体实例化。 |
| 6 | **Rule / Institution** | 制度与法律条文制定、表决与生效机制；限定合法权力与机构权威边界。 |
| 7 | **Economy / Ownership** | 复式记账法（Double-entry Bookkeeping）；严格保持零和资金守恒，无不可解释的凭空造币或货币泄漏。 |
| 8 | **Agent Runtime** | 代理调度、自主决策循环、目标推导与行为提议；严格区隔 Human、内部 AI 与外部 MCP 控制器。 |
| 9 | **Knowledge / Belief / Memory** | 认知与信念体系；严格依附于观察通道；不可直接把外部模型幻想作为世界事实写入。 |
| 10 | **Relationship** | 人物之间的熟悉度、交往历史与印象演进；不超越感知范围直接广播。 |
| 11 | **Household Contract** | 家庭成立、同住协议、成员退出、租金分摊负债与经济生活压力传递。 |
| 12 | **Body / Health Contract** | 睡眠周期、疲劳积累对决策及工作结果的影响、轻度生理状态与康复。 |
| 13 | **Education / Skill / Qualification Contract** | 培训课程、考核评定、证书颁发与求职准入；形成经验到职业机会的权威桥梁。 |
| 14 | **Information / Communication** | 面对面交谈、点对点定向信件、一跳传闻中继、组织内部通知、法律公告；认知完全封闭，零泄漏。 |
| 15 | **Organization Agency** | 组织运营策略、储备金监控、薪资负债对冲与受权定期招聘/冻结决策。 |
| 16 | **Emergence / Cohort** | 群体生命周期、群落变迁与背景人口动态实例化。 |
| 17 | **Encounter** | 同一物理空间与通行路段上的真实相遇；杜绝跨空间瞬时全知通信。 |
| 18 | **Observation / Narrative** | 多视角感知过滤、风格化叙事与长篇纪实生成；表达形式变化绝不反向篡改世界事实。 |
| 19 | **RP Session / Turn** | 人机交互轮次、动作仲裁、输入解析、Human 裁决门闸与轮次重放。 |
| 20 | **Extension Registry** | 插件/DLC 声明契约校验、版本与哈希固化、Epoch 生命周期绑定与无宿主侵入式热装载。 |
| 21 | **Gateway / Client Protocol** | 标准化通信协议（Web API、MCP Stdio/SSE、SillyTavern 接口）。 |
| 22 | **Studio / Inspector** | 创作端与事件溯源检查器；具备规则纪元与不可变证据追溯能力。 |
| 23 | **World QA / Observer** | 14 维度只读模拟健康诊断与 5 视角宏观只读观察；无副作用，不写表。 |
| 24 | **Persistence / Recovery** | SQLite 幂等落盘、进程崩溃恢复、热重启与投影零漂移自愈重建。 |

> **冻结原则**：以后除非出现无法由上述 24 项职责表达的真实跨世界核心需求，否则**一律不新增新的一级权威层**。

---

## 15.2 后续开发原则 (Pack-First Extension Architecture)

本 Goal 完成后，所有新增玩法、业务模型与系统逻辑一律优先通过扩展包（Pack）机制承载：

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

1. **World Pack (世界总包)**:
   - 定义具体世界观的初始拓扑、规则集、起始人群与法理体系。
   - 示例：`Modern City` (现代都市), `Cultivation World` (修仙大世界), `Cyberpunk` (赛博朋克), `Fantasy Kingdom` (奇幻王国)。
2. **System Pack (系统子包)**:
   - 挂载在核心契约之上的复合业务逻辑。
   - 示例：`Healthcare` (深度医疗), `Education` (高等教育学府), `Property` (房产产权交易), `Family Life` (家族世系), `Combat` (战斗判定), `Vehicle` (载具交通), `Social Media` (社交媒体舆论)。
3. **Content Pack (内容素材包)**:
   - 纯声明式业务资源目录。
   - 示例：区域地点、商业机构、岗位目录（如 F9 Retail Career）、道具物品、NPC 种子模板。
4. **Narrative Pack (叙事表达包)**:
   - 仅负责展现层（风格、视角、详略程度），严格不得影响 Agent 决策、Event、金钱与法律事实（如 F9 Life Journal）。
5. **Technical Adapter (技术适配器)**:
   - 外部大语言模型驱动、新型前端交互客户端适配、专用向量检索或推理加速。

---

## 15.3 明确延期到未来独立 Goal (Deferred Items)

以下内容已由架构路线图明确排除在当前 Core 目标之外，严禁在未立项前私自开发：

- [ ] **公网多人 MMO 架构**（当前保持单机与受控多 Resident 模式）
- [ ] **分布式集群与多服务器同步**
- [ ] **引入 Redis / Kafka / 微服务中间件栈**
- [ ] **迁移至 PostgreSQL 外部数据库**（严格保持标准内嵌 SQLite 零依赖部署）
- [ ] **无限程序化生成地图**
- [ ] **任意第三方脚本沙箱执行**
- [ ] **WASM 运行时容器隔离**
- [ ] **运行时规则热升级**（Rules Hot Upgrade）
- [ ] **完整医学模拟与病理动力学**
- [ ] **完整婚恋 / 生育 / 遗产继承民法典**
- [ ] **50 年人口代际变迁宏观推演**
- [ ] **完整二级房地产与土地租赁衍生品市场**
- [ ] **证券交易所 / 宏观金融衍生工具**
- [ ] **真实全量现实法规与数千职业行业库**
- [ ] **1000+ 活跃并发在线 LLM Resident 吞吐**

---

## 15.4 内容阶段正式开启 (Transition to Content Phase)

本架构冻结文档生效后，项目的工程导向正式从底层内核研发切换到内容与玩法生态建设：

> **核心导向转变**：
> 从 **“Core 还缺什么？”** 彻底切换到 **“下一个要做什么 World / System Pack？”**

---

## 15.5 架构冻结宣言 (Architecture Freeze Declaration)

```text
========================================================================================
Core architecture frozen.
Future feature work defaults to Pack-first unless a concrete unmet invariant requires a core RFC.
========================================================================================
```
