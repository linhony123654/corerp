# CoreRP Runtime Architecture

## 1. Canonical State

世界状态唯一形态是 `core.WorldState`：

```
WorldState {
    Clock:         WorldTime          // 世界时钟（单调递增）
    Scene:         SceneState         // 当前场景
    Relationships: map[key]Relationship  // 角色间关系（trust/intimacy/fear/respect/debt）
    Variables:     map[string]any     // 通用变量
    Flags:         map[string]bool    // 二进制标记
    Tension:       float64            // 叙事张力（0-1）
}
```

**Canonical 原则**：WorldState 只能通过 Event Projection 变更，永不被直接修改。

## 2. Event Append

Event 是系统唯一写入入口：

```go
type Event struct {
    ID        string              // 全局唯一
    Type      string              // scene_init / dialogue / trust_change / tension_change / flag_set / ...
    Actor     string              // 事件主体
    Target    string              // 事件客体
    Payload   map[string]any      // 事件内容
    Canonical bool                // true=已确认进世界，false=暂存区
    Hash      string              // chained hash (SHA256 of prevHash + id + type + canonicalPayload)
    CreatedAt time.Time
}
```

**Append 规则**：
- 一旦 Append，Event 不可修改（immutable log）
- 非 canonical 事件不进 Projection，留在 Quarantine（Gatekeeper 管理）
- 每个 Event 的 Hash 链接到前一个 Event 的 Hash（防篡改链）

## 3. Projection

```go
func Project(events []Event) WorldState
```

- 遍历所有 canonical 事件，按时间顺序 apply
- 跳过 non-canonical 事件
- 投影是纯函数：相同输入 → 相同输出
- 每个事件类型的 apply 规则定义在 `applyEvent()`

## 4. Replay

```go
replayEngine.ReplayTo(eventID, branch) WorldState
replayEngine.ReplayAtTime(hour, minute, day) WorldState
```

- 重放从事件流起点到指定截断点的所有 canonical 事件
- **确定性承诺**：同一事件序列、同一截断点 → 同一 WorldState hash
- 验证方式：`CanonicalHashV1(ReplayTo(id)) == CanonicalHashV1(ReplayTo(id))` 永远成立

## 5. Fork

```go
replayEngine.Fork(eventID, branchName)
```

- 从指定事件点创建新的 `branches` 元数据记录
- 不修改已有事件归属；父分支事件仍留在原 branch
- Replay 时沿 `parent_branch -> fork_event_id` 父链回放
- Fork 点之前的祖先事件共享，Fork 点之后只读子分支自己的新增事件
- **隔离承诺**：Fork 后任一分支的增加不影响其他分支的 Projection
- 验证方式：fork 后 main hash 不变

## 6. Determinism Guarantees

系统通过 `CanonicalHashV1` 提供版本化的确定性保证：

```
CanonicalHashV1(state) → "v1:<sha256-hex>"
```

**Contract**：
1. **Map ordering**: 所有 map 序列化前按键排序
2. **Float precision**: 统一 round 到 6 位小数
3. **Version prefix**: 格式变更时增加版本号（v1 → v2），允许并存
4. **Snapshot round-trip**: `TakeSnapshot → Marshal → Unmarshal → Verify() == true`
5. **Restore parity**: `Hash(Restore(snap, tail)) == Hash(FullReplay(all))`

## 7. Snapshot Strategy

```go
type Snapshot struct {
    Version int
    Tick    int64
    State   WorldState
    Hash    string  // CanonicalHashV1(State)
}
```

- 定期（每 N 个事件或每 K 分钟）生成 Snapshot
- Snapshot 可持久化到磁盘（JSON），恢复时只需 replay 尾事件
- 验证：加载 Snapshot → replay tail → hash 必须等于 full replay hash
- 非 canonical 事件不进入 Snapshot

## 8. Immutable Event Chain

每个 Event 携带 chained hash：

```
Event[n].Hash = SHA256(Event[n-1].Hash + Event[n].ID + Event[n].Type + canonicalPayload(Event[n].Payload))
```

- Genesis event: prevHash = "genesis"
- `VerifyEventChain([]Event) int` 返回第一个无效事件索引，-1 表示全部有效
- 任何对 Event 的修改（Payload / ID / Type）都会破坏链
- 这是 fork/replay 长期可信的基础

## 9. LLM Isolation Boundary

```
LLM 可见           | LLM 不可见
-------------------|-------------------
WorldSnapshot      | Event Store
PersonaFrame       | Memory tables
SceneState         | Event stream
ActiveGoals        | State projection code
AllowedActions     | Quarantine
RecentDialogue     | Canonical facts
```

- LLM 只接收编译后的 WorldSnapshot（按 Token Budget 截断）
- LLM 输出 ActionFrame（JSON 结构化），不输出直接世界变更
- ActionFrame 经 Validator 校验 → Executor 执行 → Event Store 追加 → State Projection 更新
- 只有 canonical=true 的事件才能改变世界状态

## 9.5 Turn Execution Protocol

当前回合执行协议固定为：

```
DirectorPlan
    -> TurnStep[0]
    -> Commit Events
    -> Re-project State
    -> TurnStep[1]
    -> Commit Events
    -> Re-project State
    -> ...
```

- `DirectorPlan` 只负责决定本轮有哪些 step，不直接改 `focusCharacter`
- `TurnStep` 是最小执行单位：一次 snapshot / 一次 LLM / 一次 validator / 一次 event commit
- 多角色参与同一轮时，必须按 step 严格串行执行
- 允许多个角色参与同一轮；不允许多个角色并发写入世界状态
- `focusCharacter` 现在表示当前主视角 / 默认角色，而不是“整轮唯一发言人”
- `auto_chain` 的默认选 step 规则是：
  - 先定 `lead`
  - 再补 `addressed_reply`
  - 再补 `support_response`
  - 高张力时可补 `tension_response`
- `TurnStep.kind` 不只是 trace 元数据，也会进入 prompt 约束：
  - `lead` 必须正面回应用户主问题
  - `addressed_reply` 必须短回应被点名内容
  - `support_response` 只补关系/态度/站位
  - `tension_response` 负责张力反应，不重开话题
- `TurnStep.kind` 也会进入动作约束：
  - step 执行前先按 kind 收窄 `AllowedActions`
  - 如果 LLM 产出的 ActionFrame 超出当前 step 允许范围，runtime 会在 validator / executor 前先降级动作
- step 之间通过显式 `handoff` 交接，而不是只靠共享状态隐式感知：
  - `from speaker / kind / action / target`
  - `outcome summary`
  - `committed events` 摘录
  - `narrative` 摘要
- 后续 step 的 prompt 会直接携带这段 handoff，作为“本轮直接前情”

## 10. Failure Recovery

```
1. 加载最新 Snapshot（如果存在）
2. Replay 自 Snapshot 之后的所有 canonical 事件
3. 验证 hash：Restore(snap, tail).Hash == CanonicalHashV1(state)
4. 如果 hash 不匹配 → 全量 replay（退化到 Event Store 从头投影）
5. 验证 event chain 完整性
6. 进入正常工作循环
```

## Appendix: Hash Contract Versioning

| Version | Serialization | Notes |
|---------|--------------|-------|
| v1 | JSON(sorted keys) + float64 round(6dp) | Current |
| v2 | (reserved) | Reserved for field additions / format changes |

Upgrade rule: new code must be able to verify v1 hashes. Adding new fields to canonical types requires a version bump but must maintain backward read compatibility.

---

*Version: 2026-05-25*
*Status: Core runtime contracts defined. Implementation in `internal/events/hash.go`, `internal/events/snapshot_test.go`, `internal/events/property_test.go`.*
