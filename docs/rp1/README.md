# CoreRP RP-1 实施记录

RP-1 在现有 M2 世界/经济内核上增量实现 Play。RP-1A 提供会话、派生在场与玩家受限观察；RP-1B 增加玩家移动与等待；RP-1C 加入原子提交的玩家发言、实际听者认知与回合阶段；RP-1D 加入受限输入的 NPC 决策与事件提交。连续回合与前端仍未因此自动成立。

## RP-1A 数据与权限边界

- [Migration 020](schema-020-sessions.sql) 只持久化会话的世界/分支/受控实体绑定、POV、观察与回合 cursor、生命周期、创建/恢复时间及幂等键。它不保存位置、资金、职业、人物属性或世界时间。
- `world.rp.control` grant 以玩家 Principal、实例、分支和受控 Entity 为作用域。打开/读取/恢复/观察每次由后端重新验证授权；客户端不能自报 Principal 取代认证身份。有效受控人物必须是该分支中已物化、在 `agent_positions` 有真实位置的单人 Entity。
- Observation 从 `materialized_entities`、`agent_profiles`、`agent_positions`、`agent_places`、`world_clocks` 和 Branch Head 同一 SQLite 事务派生，只返回玩家身份、地点、世界时间、同地可见人物与 cursor。不会返回 NPC 账户、目标、私密知识或审计字段；会话中不复制在场名单。
- Session open 的幂等键按玩家 Principal 唯一；同键同请求返回原 Session，同键异请求拒绝。close 是不可恢复的失效；重新开始须使用新幂等键。重新观察不改变世界事实，只推进应用会话的观察 cursor。
- `BootstrapRPPlayDemo` 是明确调用的本地演示准备命令，不在 HTTP 服务启动时隐式执行。它先沿用 M2/T09 物化 Ada/Bo，再物化 Cai 与玩家 Lin，并在现有五个地点和同一位置权威中初始化他们。三名 NPC 均有稳定真实 Entity；玩家使用独立的 player Principal 与控制 grant，不是 creator grant 或角色卡。

## 本地准备与 API

```bash
cd /home/ubuntu/corerp-console/backend
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-rp1.db -action rp-prepare
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-rp1.db -action rp-travel-prepare
```

之后以同一数据库启动 `corerp-server`，并在本地开发认证映射里将某个 Bearer token 映射到 `principal_m2_rp_player`。沿用既有严格 JSON envelope、1 MiB 请求限制和认证错误映射。RP-1A 的五个 POST 路由为：

| 路由 | Body | 结果 |
| --- | --- | --- |
| `/api/v1/rp/sessions/open` | `instance_id`, `branch_id`, `entity_id`, `pov`, `idempotency_key` | Session |
| `/api/v1/rp/sessions/read` | `session_id` | Session |
| `/api/v1/rp/sessions/resume` | `session_id` | 更新恢复时间后的 Session |
| `/api/v1/rp/sessions/close` | `session_id` | 关闭的 Session |
| `/api/v1/rp/observe` | `session_id` | 最新受限 Observation |

`principal_id` 只能由认证层填充或与认证结果一致。会话 ID 不能替代授权；不同玩家不可读取彼此会话。演示的静态 Bearer 映射不是生产身份系统。

## RP-1B 行动边界

`rp-travel-prepare` 只在已有 M2/RP 世界声明咖啡馆与两个住所、两个工作地点之间的双向路线，生成一条定义 Event；`rp_place_links` 是可达拓扑，不复制人物位置。移动使用 `/api/v1/rp/actions/move`，提交 `session_id`、`from_place_id`、`to_place_id`、最新 `expected_cursor` 和 `idempotency_key`。合法动作在一笔事务中写入 `RPPlayerMoved`、现有 Agent movement/position、同地相遇知识、Branch Head 与 Outbox；不可达、旧 cursor 或位置不符会拒绝，同键重试不重复移动。

等待使用 `/api/v1/rp/actions/wait`，提交 `session_id`、RFC3339 `target_world_time`、1–10000 的 `budget`、最新 `expected_cursor` 和 `idempotency_key`。迁移 022 的 `rp_wait_intents` 仅保存重试意图，不保存第二套时钟。请求先由既有 M2 scheduler 在预算内执行到期项；若还有到期项，返回 `budget_exhausted`、当前权威时间与剩余项数，用原请求/幂等键继续。清空后才由一条 `RPWaitCompleted` Event 原子推进 `world_clocks`、Branch Head、Outbox 并完成意图。完成后相同请求返回已提交结果；同键不同参数冲突。等待未完成时，同一会话的新移动被拒绝。

## RP-1C 发言与认知边界

`/api/v1/rp/actions/speak` 接收 `session_id`、非空 `text`、可选 `speech_act`（`statement` / `question` / `request`）、最新 `expected_cursor` 和 `idempotency_key`。后端只从真实位置推导同地活跃 Entity 为实际听者；此首版没有耳语、复杂声学或额外私密通道规则。异地角色不会获得该发言的知识。

一笔事务提交 `RPSpeechAccepted` Event、迁移 023 的不可变已接受发言、同地听者的 `observation_records` / `agent_knowledge`、Session 的 `turn_cursor` / `speech_committed` 状态、Branch Head、clock lineage 与参加者限定的 Outbox。听者知识的 `claim_type=speaker_said` 只表示“这个人曾这样说”，不表示话的内容是真的。完整审计/Event 仍属世界权威，不能由可编辑 transcript 或向量索引替代。Outbox 通知可至少一次重试，消费者应按 `outbox_id` 去重；通知未送达不撤销已发生的发言与认知。

## RP-1D NPC 决策边界

后端内部 `BuildRPDecisionInput` 必须先证明该 NPC 实际听见了指定玩家 Turn 中已提交的发言，并仍与玩家同场。传给可替换 `RPDecisionProvider` 的仅有其自身身份、地点/活动、Goal、自有资产余额与币种、自身下一日程、自己已经获得的有限 Knowledge、同地可见人物、玩家已听见的发言，以及当前允许的动作和可达地点；不提供别人的隐藏资产、账户 ID、Creator 全知状态、原始数据库行或审计记录。当前没有独立关系/情节记忆权威，不能把它们虚构进输入。

Provider 只能提议 `respond`、`refuse`、`silence`、`wait` 或可达地点的 `leave`。`DecideRP` 验证提案并记录非权威审计；失败或超时返回可审计的沉默回退，不改世界。后端内部 `CommitRPDecision` 再检查输入哈希、Branch Head、玩家/NPC 位置、路线和回合，再以迁移 024 的不可变决策记录原子提交 NPC 回复/拒绝及听者 Knowledge、真实移动事实，或沉默/等待选择事件。相同玩家 Turn/NPC 的重试返回原 Event；不同效果冲突。Session 的阶段进入 `npc_effects_committed`，供后续 RP-1E 编排。当前仓库没有现成外部 LLM adapter，只有确定性和测试 Provider；真实 LLM E2E 在可用时仍需接入验证。

阶段报告见 [RP-1A](phase-a.md)、[RP-1B](phase-b.md)、[RP-1C](phase-c.md) 和 [RP-1D](phase-d.md)。后续 RP-1E–F 尚待完成，不能把当前后端行为视作整个可玩 RP-1 闭环。
