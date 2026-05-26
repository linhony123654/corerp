# CoreRP Architecture

## 分层架构

```
Interface Layer (PWA / CLI / API Consumers)
    ↓ HTTP + SSE
API Gateway (无状态路由、统一认证)
    ↓
Runtime Core (叙事运行时内核)
    ├── Runtime Instance Manager (instance_id + default routing + lifecycle)
    ├── Context OS     (Snapshot Compiler + Token Budget 硬墙)
    ├── State Machine  (Variables + Constraints + Transitions)
    ├── Event Bus      (Event Store + Projector + Causality Engine)
    ├── Goal System    (Primary + Secondary + Hidden + Planner)
    ├── Action Layer   (Frame Definition + Executor + Permissions)
    ├── Memory Engine  (Short-term / Working / Semantic / Episodic)
    └── Canon Layer    (Ontology + Facts + Consistency)
    ↓
Narrative Layer (Renderer + Tension Engine + Compression)
    ↓
LLM Adapter (OpenAI / Claude / DeepSeek / Ollama / Local BGE)
    ↓
Storage (SQLite + instance namespace | YAML worlds/characters)
```

## 核心数据流

1. User Input → Intent Analysis（代码规则）
2. State Machine 读取当前世界投影
3. Goal System 激活目标 → 调整记忆检索权重
4. Memory Engine 召回相关事实（Semantic + Episodic）
5. Canon Layer 一致性检查（禁止污染 Canonical Truth）
6. Context OS 组装 WorldSnapshot（Token 预算硬墙）
7. Director 产出 TurnPlan（一个或多个顺序 TurnStep）
8. 每个 TurnStep：Snapshot Compile → LLM Adapter → Action Frame
9. Narrative Validator 拦截 / 降级 / 重写
10. Action Executor 执行世界变更 → Event Store 追加
11. 下一 TurnStep 在更新后的 Projection 上继续执行
12. State Projection 刷新 → Renderer 生成叙事文本（SSE 返回）
13. Memory Engine 提取候选记忆 → Quarantine → 延迟提交

## 技术选型

| 组件 | 选择 | 排除项 | 原因 |
|------|------|--------|------|
| 语言 | Go 1.22+ | Python / Node | 单二进制、goroutine 适合 Tick、SSE 原生 |
| DB | SQLite + sqlite-vec | PostgreSQL / MongoDB | 单文件、Git-friendly、零运维 |
| 嵌入 | BGE-small-zh-v1.5 (ONNX) | 远程 API / 大模型本地 | 512-dim、中文语义优化、零 API 成本 |
| 流式 | SSE | WebSocket / 长轮询 | 比 WS 简单、Nginx 友好 |
| 前端 | Vanilla JS PWA | React / Vue | 无框架、几百行、只负责渲染 |
| 部署 | 单二进制 + systemd/PM2 | Docker / K8s | 过度、荒谬 |

## 核心原则

1. LLM 永远不能直接定义世界真相
2. Event Store 是唯一真相源，状态是投影
3. Action Layer 是世界变更的唯一入口
4. Token 预算硬墙不可突破，超预算 panic（开发期）
5. Timeline fork 通过 `branches` 元数据建模，禁止通过改写历史事件归属来“分支”
6. Phase 1-3 全部完成：17 内部模块、24 API 端点、前端 100% 覆盖
7. 世界文件三层分离：world.yml / canon/ontology.yml / canon/facts.yml / scenes/
8. 所有配置 YAML 化，拒绝 JSON 黑盒
9. SQLite 单文件，备份 = `cp memory.db backup/`
10. 认证：HMAC session token，httpOnly cookie
11. 嵌入模型：BGE-small-zh-v1.5（512-dim，中文语义优化，零 API 成本；开发环境 fallback 2-gram）
12. Runtime Instance 是一等概念：API、事件、分支、记忆、存档必须支持 `instance_id`

## 当前实现备注

- CoreRP 已不再是“单全局 runtime”模型：
  - API 层支持 `instance_id`
  - `internal/runtime.Manager` 支持 `list / status / create / set default / stop / delete`
- 持久化当前采用混合模式：
  - 文件持久化按 `data/instances/<instance_id>/...` 分目录
  - SQLite 共享单库，但关键表按 `instance_id` 过滤
- 当前运行策略：
  - `data/` 作为唯一标准运行目录
  - PM2 固定以 `-data /home/kelebituo/corerp/data` 启动
  - 启动后通过 `deploy/smoke-check.sh` 验证 `/api/health` 与 `/api/ready`
- 作者工具当前已接通：
  - checkpoint / rollback 复用实例级 save slot 持久化
  - scenario preset 以实例级 `scenario_presets.json` 保存当前 scene / player role / active character
  - trace 支持 latest + 历史轮次列表浏览
- 仍未完成的部分：
  - 多角色在**单实例内部**仍是共享事件流上的串行 step 协议，不是每角色独立 timeline
  - 实例生命周期与前端实例管理面板已接通；运行台现在区分“默认实例”和“当前视图实例”
