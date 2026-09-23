# CoreRP M0 核心契约 RFC

> 状态：评审草案；基线：CoreRP v0.3.1 精读修订版（2026-09-22）。本文中的“必须/不得”表示候选规范要求，不表示运行时已经实现。

## 1. 范围与非目标

本 RFC 冻结 M0 最小不可再删协议：Command、权威提交、事件和审计排序、Branch/Rule Epoch、幂等与 attempt、SQLite 事务、会计和库存事实、Outbox、权限、规范哈希、重放和错误语义。

M0 不实现世界模拟、LLM Agent、90 日经济运行、规则热激活、任意 DLC 代码、近似群体结算或 PostgreSQL 迁移。相关类型只保留不会妨碍后续实现的边界。

## 2. 术语与最高不变量

| 术语 | 规范含义 |
|---|---|
| Command | 主体希望执行的意图；自身不是事实 |
| Proposal Evidence | 规则、随机源或外部模型产生的候选及证据；提交前不具权威性 |
| Event Batch / Commit Record | 一次原子权威提交的因果根 |
| Event | Batch 内按顺序提交的事实；推进分支事件序号 |
| Domain Fact | 引用 Event 的规范化不可变事实，例如 JournalEntry、Posting、StockMovement |
| Projection | 可从权威提交重建的余额、库存余量、实体状态或查询索引 |
| Audit Record | 决策、拒绝、观察、诊断或干预证据；拥有 `record_order`，不推进分支事件序号 |
| Branch Head | 分支最后一个已提交权威事件序号，也是 Command 的乐观并发版本 |
| Rule Epoch | 分支上规则包锁和 `ruleset_hash` 不变的事件序号半开区间 |

三个最高不变量：

1. 持久世界状态只能由验证后原子提交的 Event Batch 改变；模块不得直接写投影制造事实。
2. 一个 Batch 的事件、领域事实、投影变更、调度状态和 Outbox 同成同败。
3. 普通重放只应用已提交结果，不重新调用模型、不重新取随机数、不读取外部实时价格。

## 3. 标识、时间与整数

- 所有 ID 是稳定、不透明、区分大小写的字符串；不得从显示名称推导身份。
- `world_time` 是世界时钟值，`recorded_at` 是服务器记录时间，两者不得互换。M0 交换格式使用带显式时区的 RFC 3339 字符串；SQLite 内保存规范化 UTC 微秒整数及原始世界时钟字段的方案由实现 RFC 固化。
- 金额使用货币最小单位整数；数量使用 SKU 声明的基础单位整数。禁止二进制浮点记账。
- 实现必须对加减乘、聚合和转换做溢出检查。JSON 交换值限制在语言间可无损表达的安全范围；更大值须使用十进制字符串并由后续版本显式协商，M0 不允许静默截断。

## 4. 权威提交模型

Event Batch 是提交边界和因果根。JournalEntry/Posting、StockMovement 等领域事实属于同一 Commit Record，必须引用 Batch 内 Event，且没有独立写入口。账户余额、库存余量、实体状态和搜索索引是同步投影，可删除并从权威事实重建。

一个成功 Batch 必须满足：

- 引用一个成功认领的 Command attempt；
- `expected_head` 等于进入最终事务时的 `branch.head_sequence`；
- 包含至少一个 Event，Event 获得连续 `event_sequence`；
- 全部 Event 引用在其序号处生效的同一 Rule Epoch；M3 激活批次是唯一允许携带纪元切换元数据的特殊 Batch；
- 领域事实通过提交门禁；
- Outbox 与世界事实同事务写入；
- 分支头通过条件更新从 `expected_head` 前移到 Batch 最后事件序号，影响行数必须为 1。

Audit Record 使用独立 `record_order`。Audit Record 可引用 Event、Command、attempt 或其他审计记录，但不得占用 `event_sequence`，不得改变 `branch_head_sequence`。

## 5. Command、幂等与 attempt

幂等作用域固定为：

```text
(instance_id, branch_id, command_type, idempotency_key)
```

请求先按 §13 规范化并计算 `request_hash`：

- 同一作用域、相同哈希：返回已有状态或原结果；
- 同一作用域、不同哈希：拒绝为 `IDEMPOTENCY_PAYLOAD_MISMATCH`；
- 不得通过换 actor、重排 JSON key 或添加无意义空白规避比较。

每次执行有独立 attempt。状态机为：

```text
claimed -> proposing -> ready -> committed
                    \-> rejected
                    \-> expired
```

认领记录必须有租约和恢复策略。同一 Command 同时最多一个有效 attempt。进程在外部模型调用期间崩溃后，接管者只能在请求哈希一致的前提下恢复、复用已持久化 Proposal Evidence，或明确终止旧 attempt 后创建新 attempt；不得让永久 `pending` 阻塞幂等键。

版本冲突后是否允许复用 Proposal、重新规划或直接拒绝由 `command_policy` 决定并写入审计。重新调用模型所得内容必须属于新 attempt，不能伪装成原 attempt 的确定性重试。

## 6. SQLite 最终事务

外部模型、叙事生成、SSE 和网络 I/O 不得在持锁写事务内执行。建议流程：

1. 短事务认领 Command 与 attempt，然后释放锁。
2. 读取 Snapshot/Projection 和当前 Rule Epoch，在事务外构造 Proposal Evidence。
3. 使用单写协调进入 `BEGIN IMMEDIATE`。
4. 重新校验 Principal、Capability、Scope、请求哈希、报价、余额、库存、规则纪元和 `expected_head`。
5. 分配连续事件序号；写 Batch、Events、随机/模型已决定结果和领域事实。
6. 将 JournalEntry 从 `draft` 关闭为 `posted`；执行会计、库存、权限和溢出门禁。
7. 以条件更新写投影、调度游标、Outbox 和 Branch Head。
8. `COMMIT`；提交后异步发布 Outbox。

失败切面：

| 断点 | 必须结果 |
|---|---|
| COMMIT 前崩溃 | 无 Batch、分录、库存、投影或 Outbox 半提交 |
| COMMIT 后、发布前崩溃 | 世界提交保留；恢复器重发 Outbox；消费者按 outbox ID 去重 |
| 分支头条件更新失败 | 整个事务回滚，返回版本冲突 |
| 投影损坏 | 从权威记录重建，报告摘要差异，不反向修改历史事实 |

## 7. Event Sequence、Rule Epoch 与调度顺序

新分支 `head_sequence = 0`，初始 Rule Epoch 从事件序号 1 开始。Rule Epoch 使用 `[start_sequence, end_sequence)`：区间连续、不重叠，同一 Event 只属于一个纪元；当前纪元 `end_sequence = null`。

M3 若执行规则激活，LawActivationEvent 所在 Batch 属于旧纪元。旧纪元在该 Batch 最后事件序号之后闭合，新纪元从下一事件序号开始。迁移结果由版本化内核迁移协议和 `migration_contract_hash` 解释。M0 只冻结边界；M1 不执行热激活。

自主调度项的稳定排序键为：

```text
(world_time, phase_id, declared_priority, scheduler_item_id)
```

`phase_id` 注册表、跨包优先级冲突规则和调度算法版本进入 `ruleset_hash`。不得以包加载顺序、SQL 无序返回、进程地址、服务器墙钟或随机 map 顺序破平。外部用户 Command 的竞争结果以实际提交序号为准并被记录，不冒充可重新模拟的自主调度确定性。

## 8. 会计门禁

- `Currency.scale` 被权威提交引用后不可暗改。
- Posting 使用带符号 `amount_minor`；M0 约定借方为正、贷方为负。
- JournalEntry 采用 `draft -> posted`。每个币种至少两条有效 Posting，按币种合计必须为零，账户币种必须匹配；posted 后禁止改删。
- 跨币种交易分别在每个币种内平衡，并记录报价、兑换腿、时间和舍入尾差账户；不同币种金额不得相加。
- 账户默认不可为负。透支需要显式账户政策、授权主体、额度和生效规则。
- 工资义务由 `(contract_id, period_start, period_end)` 唯一标识。计提只确认一次费用与应付；支付只清偿义务，不再次确认费用。
- 货币发行、注销和信用创造需要类型化机构、Capability、限额及事件来源；不得用无含义的无限负余额账户掩盖发行。

SQLite 可约束键、外键、单行范围和不可变触发器；按币种跨行平衡、授权和溢出仍由提交门禁验证。DDL 见 [schema.sql](schema.sql)。

## 9. 库存门禁

- `ProductSKU.quantity_scale` 被引用后不可暗改。
- StockMovement 的数量为正整数，普通转移必须有不同的来源和去向。
- 创建只能从类型化 Source 流向普通 Holder；消耗/报废只能从普通 Holder 流向类型化 Sink。
- Source/Sink 的使用需要 Capability 和来源原因；它们不是普通交易可用的无限库存。
- 普通 Holder 的库存余量不得为负。写入阶段使用版本比较或条件更新并检查影响行数，防止两个 Command 同时卖出最后一件商品。

## 10. Outbox 与观察发布

Outbox 项按 `(event_id, topic, audience_scope_hash)` 唯一，与 Batch 同事务写入。发布语义是至少一次；消费者按 `outbox_id` 去重。发布失败不得回滚世界事件。

SSE 游标至少包含实例、分支和最后可见事件序号。重连只补发 Principal 有权观察的内容；不能因为内部事件序号有间隙而泄露被过滤事件的载荷或存在性。

## 11. 权限与 Inspector

所有 Command、Query、Inspector 和订阅入口必须接收：

```text
Principal + Capability + Scope(instance, branch, subject, fields)
```

玩家、创作者和运维诊断是不同 Principal 类别。审计证据存在不代表默认可读。玩家不得读取隐藏资产、私密认知、全量 Agent 输入或未观察到的拒绝原因；运维视图受 RBAC 和脱敏约束。SQLite 不提供本项目需要的自动行级权限，应用查询边界必须集中执行过滤。

硬特权可绕过明确列出的世界内约束，但不能绕过 Principal、Scope、实例隔离、幂等、事务、身份引用和历史完整性。所有影响状态、概率分布、规则结果或分支的干预都需要 InterventionRecord。

## 12. 随机、模型与可重放证据

已提交 Event 保存结果、时间、排序、Rule Epoch、输入引用以及必要的随机/模型证明。普通重放不需要历史模型在线，但继续提交新命令需要当前纪元精确规则包。

存档缺少历史可执行规则包时：

- 权威 Event 和领域事实完整：允许只读查看和普通重放；
- 当前纪元包缺失：禁止继续写入；
- 全量重新模拟：要求取得所有相关规则包，并输出到独立实验分支。

叙事文本只读取经过权限过滤的 Observation。文本本身不得反写事实；含事实主张的模型输出必须成为待验证 Proposal。

## 13. 规范编码与哈希

M0 候选统一使用 RFC 8785 JSON Canonicalization Scheme 的 UTF-8 字节作为 JSON 哈希输入，摘要格式为：

```text
sha256:<64 个小写十六进制字符>
```

核心金额和数量只允许整数；不得依赖浮点格式差异。对象 key 顺序和空白不得影响哈希，数组顺序具有语义。`ruleset_hash` 至少覆盖精确包锁、规则/调度算法版本、phase 注册表和冲突决议；`content_hash` 的文件清单与排除规则写入 Manifest，不能把包含自身 hash 字段的 Manifest 直接递归散列。

## 14. 错误响应

错误对象最少包含 `code`、`message_key`、`retryable`、`command_id`、`attempt_id`、`trace_id` 和安全的 `details`。不得把敏感规则输入或隐藏实体状态塞入玩家可见错误。

| Code | Retryable | 含义 |
|---|---:|---|
| `IDEMPOTENCY_PAYLOAD_MISMATCH` | 否 | 同幂等作用域出现不同请求摘要 |
| `COMMAND_ATTEMPT_ACTIVE` | 是 | 另一个有效 attempt 持有租约 |
| `BRANCH_VERSION_CONFLICT` | 视策略 | `expected_head` 已过期 |
| `RULE_EPOCH_MISMATCH` | 视策略 | Proposal 使用的规则纪元已变化 |
| `PERMISSION_DENIED` | 否 | Principal/Capability/Scope 不允许操作 |
| `QUOTE_EXPIRED` | 可重新规划 | 报价在最终事务中失效 |
| `INSUFFICIENT_FUNDS` | 可重新规划 | 余额及许可额度不足 |
| `INSUFFICIENT_STOCK` | 可重新规划 | 库存条件更新失败 |
| `JOURNAL_UNBALANCED` | 否 | 分录无法按币种平衡 |
| `INTEGER_OVERFLOW` | 否 | 金额或数量运算越界 |
| `PACKAGE_MISSING_CURRENT_EPOCH` | 否 | 当前纪元精确规则包不可用 |
| `OUTBOX_DELIVERY_FAILED` | 是 | 提交已成功，发布等待重试；不得向调用方伪装成世界提交失败 |

## 15. Go 接口草案

```go
type Snapshot struct {
    InstanceID   string
    BranchID     string
    HeadSequence uint64
    EpochID      string
    RulesetHash  string
}

type CommitPlan struct {
    CommandID         string
    AttemptID         string
    ExpectedHead      uint64
    EpochID           string
    Events            []EventDraft
    JournalEntries    []JournalDraft
    StockMovements    []StockMovementDraft
    SchedulerMutations []SchedulerMutation
    OutboxMessages    []OutboxDraft
}

type WorldStore interface {
    Snapshot(ctx context.Context, instanceID, branchID string) (Snapshot, error)
    Claim(ctx context.Context, cmd Command) (Attempt, error)
    Commit(ctx context.Context, plan CommitPlan) (CommitResult, error)
    Replay(ctx context.Context, instanceID, branchID string, through uint64) (State, error)
}
```

只有 `Commit` 可以写权威事实和同步投影。业务模块不得获得通用 SQL 写句柄；读取接口也必须携带 Principal/Scope。

## 16. 阶段边界

| 阶段 | 本 RFC 要求 |
|---|---|
| M0 | 审阅并冻结字段、DDL、错误、哈希、测试向量和两个世界样本 |
| M1 | 实现无 LLM 的严格 90 日小世界、事务、恢复和 T01–T08 |
| M2 | LOD、StatusPerception、外部/抽象市场和规模验证 |
| M3 | Quest/UI/受控计算、法则热激活执行和迁移 |
| M4 | Inspector 产品 UI、多客户端与运维体验 |

仍需产品批准：多人世界所有者权限、创作者能力上限、具体 phase 注册表、各世界发行机构/透支政策、版本冲突后的 Proposal 复用策略。实现者不得自行把这些待决项变成默认产品行为。

## 17. 验收入口

机器可读测试见 [test-vectors.json](test-vectors.json)。T01–T12 在运行前都只能标为“定义完成/未运行”；前端 fixture 验证不能替代 SQLite 故障注入、崩溃恢复和重放一致性测试。
