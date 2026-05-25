# CoreRP Architecture

## 分层架构

```
Interface Layer (PWA / CLI / API Consumers)
    ↓ HTTP + SSE
API Gateway (无状态路由、统一认证)
    ↓
Runtime Core (叙事运行时内核)
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
LLM Adapter (OpenAI / Claude / DeepSeek / Ollama / Local MiniLM)
    ↓
Storage (SQLite + sqlite-vec | YAML worlds/characters)
```

## 核心数据流

1. User Input → Intent Analysis（代码规则）
2. State Machine 读取当前世界投影
3. Goal System 激活目标 → 调整记忆检索权重
4. Memory Engine 召回相关事实（Semantic + Episodic）
5. Canon Layer 一致性检查（禁止污染 Canonical Truth）
6. Context OS 组装 WorldSnapshot（Token 预算硬墙）
7. LLM Adapter 发送 Snapshot → 接收 Action Frame
8. Narrative Validator 拦截 / 降级 / 重写
9. Action Executor 执行世界变更 → Event Store 追加
10. State Projection 刷新 → Renderer 生成叙事文本（SSE 返回）
11. Memory Engine 提取候选记忆 → Quarantine → 延迟提交

## 技术选型

| 组件 | 选择 | 排除项 | 原因 |
|------|------|--------|------|
| 语言 | Go 1.22+ | Python / Node | 单二进制、goroutine 适合 Tick、SSE 原生 |
| DB | SQLite + sqlite-vec | PostgreSQL / MongoDB | 单文件、Git-friendly、零运维 |
| 嵌入 | all-MiniLM-L6-v2 (ONNX) | 远程 API / 大模型本地 | 20MB、CPU 可跑、零 API 成本 |
| 流式 | SSE | WebSocket / 长轮询 | 比 WS 简单、Nginx 友好 |
| 前端 | Vanilla JS PWA | React / Vue | 无框架、几百行、只负责渲染 |
| 部署 | 单二进制 + systemd/PM2 | Docker / K8s | 过度、荒谬 |

## 核心原则

1. LLM 永远不能直接定义世界真相
2. Event Store 是唯一真相源，状态是投影
3. Action Layer 是世界变更的唯一入口
4. Token 预算硬墙不可突破，超预算 panic（开发期）
5. Phase 1-3 全部完成：17 内部模块、24 API 端点、前端 100% 覆盖
6. 世界文件三层分离：world.yml / canon/ontology.yml / canon/facts.yml / scenes/
7. 所有配置 YAML 化，拒绝 JSON 黑盒
8. SQLite 单文件，备份 = `cp memory.db backup/`
9. 认证：HMAC session token，httpOnly cookie
10. 嵌入模型：BGE-small-zh-v1.5（512-dim，中文语义优化，零 API 成本）
