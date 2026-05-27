# CoreRP — CLAUDE.md
# Persistent Narrative Runtime — 世界状态驱动 LLM 引擎

## 核心架构约束

- LLM 只能看到 WorldSnapshot，看不到 DB/Event/Memory 表
- LLM 只负责：选 Action Frame + 渲染叙事文本
- 所有状态变更必须经过 Action Executor → Event Commit → State Projection
- 三种真相分离：Canonical Truth（YAML） / Runtime State（投影） / Narrative Output（经 Validator）

## 技术栈

Go 1.22+ / SQLite + sqlite-vec / HTTP + SSE / 纯 HTML+JS PWA / 单二进制部署

## 编码规范

- 接口定义为 Phase 2/3 预留参数位（confidence、filters 等）
- 角色卡和世界设定用 YAML，禁止 JSON
- 默认中文回复，优先修改现有文件
- 不添加超出任务范围的注释、依赖或重构

## 文档同步（每次改代码后检查）

| 文件 | 触发条件 |
|------|---------|
| `ARCHITECTURE.md` | 分层/模块/技术选型变更 |
| `TODO.md` | 完成功能或发现新任务 |
| `SESSION_LOG.md` | bug 修复、踩坑（格式：`### YYYY-MM-DD HH:MM:SS CST — 标题` + `Modified by: <Model>`，北京时间 UTC+8） |
| `api-contract.yaml` | API 路由/请求/响应结构变更 |

## 禁止事项

- 禁止 WebSocket（用 SSE）
- 禁止外部向量数据库（只用 sqlite-vec）
- 禁止复杂前端（无立绘/BGM/动画）
