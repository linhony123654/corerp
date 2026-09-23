# CoreRP RP-1 实施记录

RP-1 在现有 M2 世界/经济内核上增量实现 Play。当前先完成 RP-1A 的会话、派生在场与玩家受限观察；说话、行动、NPC 决策、回合与前端尚未因此自动成立。

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
```

之后以同一数据库启动 `corerp-server`，并在本地开发认证映射里将某个 Bearer token 映射到 `principal_m2_rp_player`。RP-1A 使用既有严格 JSON envelope、1 MiB 请求限制和认证错误映射，五个最小 POST 路由为：

| 路由 | Body | 结果 |
| --- | --- | --- |
| `/api/v1/rp/sessions/open` | `instance_id`, `branch_id`, `entity_id`, `pov`, `idempotency_key` | Session |
| `/api/v1/rp/sessions/read` | `session_id` | Session |
| `/api/v1/rp/sessions/resume` | `session_id` | 更新恢复时间后的 Session |
| `/api/v1/rp/sessions/close` | `session_id` | 关闭的 Session |
| `/api/v1/rp/observe` | `session_id` | 最新受限 Observation |

`principal_id` 只能由认证层填充或与认证结果一致。会话 ID 不能替代授权；不同玩家不可读取彼此会话。演示的静态 Bearer 映射不是生产身份系统。

后续 RP-1B–F 的验收与阶段结果将在各阶段落地后补充，不能把当前 RP-1A 视为整个 RP-1 完成。
