# CoreRP — CLAUDE.md
# Persistent Narrative Runtime
# 本文件是 AI 编程助手的绝对指令源，优先级高于任何用户临时描述。

---

## 1. 项目定位（必须背诵）

CoreRP 不是：
- 不是 "更好的 SillyTavern"
- 不是 "多 Agent 自动聊天工具"
- 不是 "Prompt 管理 UI"

CoreRP 是：
- **Persistent Narrative Runtime**（持久叙事运行时）
- 一个**世界状态驱动 LLM** 的引擎，而非 LLM 驱动世界。
- 终极形态是 "文字世界模拟器"，当前 Phase 1 是 "最小可运行叙事内核"。

---

## 2. 核心哲学（不可违背）

### 2.1 三种真相分离
- **Canonical Truth**：世界真理，YAML/代码定义，LLM 不可覆盖。
- **Runtime State**：运行时状态，Event Store 投影，LLM 只读。
- **Narrative Output**：LLM 生成文本，**必须经过 Validator 才可能影响世界**。

### 2.2 LLM 职责边界
- LLM **只能看到 WorldSnapshot**（结构化投影），看不到数据库、看不到事件流、看不到记忆表。
- LLM **只负责两件事**：(1) 选择 Action Frame；(2) 渲染叙事文本。
- LLM **永远不直接修改世界状态**。所有状态变更必须经过 Action Executor → Event Commit → State Projection。

### 2.3 Action Layer 强制存在
- LLM 不直接输出自然语言剧情。
- LLM 先输出 **Action Frame**（JSON：actor/action/target/intensity/emotion/suggested_line）。
- Runtime 执行 Action → 变更状态 → 再让 LLM 渲染文本。
- 这是架构级要求，Phase 1 必须实现，不能省略。

---

## 3. Phase 1 边界（硬墙）

### 3.1 必须实现（P1）
- [x] 单角色、单世界、长期稳定运行。
- [x] Event Store（SQLite 追加表）+ State Projection（内存状态）。
- [x] Context OS / Snapshot Compiler（2K token 预算硬墙）。
- [x] Identity Envelope（immutable/adaptive/forbidden）。
- [x] Narrative Validator（代码规则为主，拦截 OOC）。
- [x] 四层记忆骨架：Short-term（环形）、Working（覆盖式）、Semantic（结构化事实）、Episodic（事件占位）。
- [x] Action Frame 定义 + Action Executor（switch-case 足够）。
- [x] Goal System 骨架（只读 primary goals，影响检索权重）。
- [x] HTTP API + SSE 流式输出。
- [x] 极简 PWA（index.html + app.js，纯 SSE 接收，零业务逻辑）。
- [x] 单二进制 Go 程序，SQLite 单文件，可挂 VPS。

### 3.2 明确不做（P1 禁止）
- **禁止 WebSocket**：SSE 足够，不要增加复杂度。
- **禁止多 Agent 调度**：Phase 1 只支持单角色。agents/ 目录留接口，但 scheduler.go 空文件。
- **禁止 Simulation Tick**：✅ Phase 2 已实现 Tick Loop。
- **禁止自动事件提取**：✅ Phase 2 已实现 Quarantine + Confidence Pipeline，事件先进暂存区，规则确认后转正。
- **禁止 Narrative Compression**：Phase 1 不压缩历史，Working Memory 覆盖式更新足够。
- **禁止 Causality Chain**：events/ 表预留 causes/effects JSON 字段，但 causality.go 空文件。
- **禁止复杂前端**：没有立绘、没有 BGM、没有打字机动画（纯文本 SSE 逐字输出足够）。
- **禁止向量数据库外部依赖**：只用 sqlite-vec（单文件），禁止 pgvector/Milvus/Qdrant。

---

## 4. 技术栈（锁定）

| 层级 | 技术 | 理由 |
|------|------|------|
| 后端语言 | Go 1.22+ | 单二进制、goroutine 适合 Tick、SSE 原生支持 |
| 数据库 | SQLite + sqlite-vec | 单文件、Git-friendly、零运维 |
| 嵌入模型 | BGE-small-zh-v1.5 (ONNX) | 512-dim、中文语义优化、零 API 成本 |
| 协议 | HTTP + SSE | 流式输出、比 WebSocket 简单、Nginx 友好 |
| 前端 | 纯 HTML/JS PWA | 无框架、几百行、只负责渲染 |
| 部署 | 单二进制 + Nginx 反代 | PM2/systemd 守护、VPS 常驻 |

---

## 5. 编码规范（防止熵增）

### 5.1 接口预留
定义接口时必须为 Phase 2/3 预留参数位：
```go
// 正确：预留 confidence 和 filters
type MemoryStore interface {
    Remember(fact string, character string, confidence float64) error
    Recall(query string, character string, filters ...Filter) ([]Memory, error)
}

// 错误：Phase 1 写死了，Phase 2 要重构全部调用点
```

### 5.2 空文件占位
Phase 1 不实现的文件，必须存在且只包含：
```go
package xxx

// TODO(P2): 实现 xxx 功能
// 当前保留接口占位，避免重构成本。
```

### 5.3 数据库表预留字段
Event 表必须预留：
```sql
CREATE TABLE events (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    actor TEXT,
    payload JSON,           -- 事件内容
    causes JSON,            -- TODO(P3): 因果链
    effects JSON,           -- TODO(P3): 副作用
    canonical BOOLEAN DEFAULT 0, -- 1=已确认进世界，0=暂存区
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 5.4 YAML 配置格式
角色卡和世界设定必须用 YAML，禁止 JSON（人类可读、Git diff 友好）。

---

## 6. 数据流（每次对话必须遵循）

```
User Input
    ↓
Intent Analysis（代码规则，轻量）
    ↓
State Evaluation（读取当前投影状态）
    ↓
Memory Retrieval（Semantic + Episodic 占位检索）
    ↓
Constraint Building（Identity Envelope + Goal 影响）
    ↓
Snapshot Compile（组装 WorldSnapshot，Token 预算硬墙）
    ↓
LLM Generation（输出 Action Frame）
    ↓
Narrative Validation（代码规则拦截 OOC/Forbidden）
    ↓
Action Extraction（解析 Action Frame）
    ↓
Action Executor（变更世界状态）
    ↓
Event Commit（写入 SQLite，canonical=0 进暂存，=1 进世界）
    ↓
State Projection Update（内存状态刷新）
    ↓
SSE 返回叙事文本
```

---

## 7. 与 AI 助手协作纪律

- **用户说"先做简单版"**：指 Phase 1 范围，不是放弃 Action Layer / Event Store。
- **用户说"后面再加"**：在接口处留扩展位，数据库表留字段，不要写死。
- **用户说"参考酒馆"**：只参考角色卡 YAML 格式，不参考架构（酒馆是 Prompt UI，我们是 Runtime）。
- **用户说"用向量检索"**：指 sqlite-vec，不要引入外部向量数据库。
- **用户说"多角色"**：Phase 1 明确拒绝，agents/scheduler.go 空文件占位。

---

## 8. 验证标准（Phase 1 完成定义）

必须同时满足：
1. ✅ 连续对话 50 轮，角色人设不漂移（Identity Envelope 生效）。— 多轮验证 + OOC 拦截测试通过
2. ✅ 重启服务后，角色能准确回忆第 3 轮的关键事实（Semantic Memory + Event Store 生效）。— 跨会话记忆：dialogue_history 持久化 + 重启恢复 15 轮
3. ✅ LLM 输出一次明显 OOC（如角色做 forbidden 动作），被 Validator 拦截或降级。— Anya 拒绝卖萌/撒娇/鬼脸
4. ✅ 上下文 Token 预算可配置（budgets.yml: normal=48K, full_load=96K）。— 不再使用 P1 的 4K 硬墙
5. ✅ 手机浏览器打开域名，能发消息、收 SSE 流式回复（PWA 生效）。— PWA manifest + sw.js + app.js + 响应式布局
6. ✅ VPS 上 `pm2 start ./corerp` 能常驻，SQLite 单文件可备份。— deploy/corerp.service systemd 配置

全部满足 = Phase 1 完成。

---

## 9. 文档同步工作流（强制）

**每次生成/修改代码后，必须检查以下文档是否需要同步更新。有需要则立即更新，无需则跳过。**

### 检查清单

| 文件 | 触发更新的场景 |
|------|--------------|
| `ARCHITECTURE.md` | 修改分层结构、新增/删除模块、变更技术选型 |
| `TODO.md` | 完成功能、发现新阻塞、新增待办 |
| `SESSION_LOG.md` | 每次会话结束时追加修改记录、bug 修复、已知坑；**每条记录必须写绝对时间和修改者/模型身份** |
| `api-contract.yaml` | 修改 API 路由、请求/响应结构、新增端点 |
| `budgets.yml` | 修改 Token 预算分配比例或阈值 |

### 执行方式
代码写完后，在回复用户之前，问自己：
1. 本次修改涉及架构调整吗？→ 更新 ARCHITECTURE.md
2. 本次修改完成了 TODO 中的项或发现了新任务？→ 更新 TODO.md
3. 本次会话有 bug 修复或踩坑？→ 追加 SESSION_LOG.md
4. 本次修改了 API？→ 更新 api-contract.yaml
5. 本次新增/修改了类型定义？→ 检查 internal/models/types.go 是否需要同步

### `SESSION_LOG.md` 记录格式（强制）

每次追加日志时，必须使用以下格式：

```md
### YYYY-MM-DD HH:MM:SS UTC — 标题
Modified by: <Agent/Tool> (<Model>)
```

示例：

```md
### 2026-05-26 02:35:21 UTC — 角色一致性 / 记忆隔离 / 分支稳定性修复
Modified by: Codex (GPT-5)
```

禁止只写相对时间（如“今天下午”）或只写工具名不写模型身份。

---

*Version: 2026-05-25*
*Status: Phase 1-3 Complete. 24 API endpoints. Frontend 100% coverage. Architecture fully closed.*
