# CoreRP TODO

## Phase 1 — Implementation Complete
- [x] Go 模块初始化 + 目录结构
- [x] core/types.go — 全项目共享类型定义
- [x] events/store.go — Event Store + State Projection
- [x] state/state.go — WorldState 内存管理
- [x] memory/engine.go — 四层记忆骨架
- [x] agents/identity.go — Identity Envelope + Validator
- [x] context/compiler.go — Snapshot Compiler + Token Budget 硬墙
- [x] actions/executor.go — Action Frame + Executor
- [x] llm/adapter.go — OpenAI 兼容 SSE 适配
- [x] runtime/runtime.go — 对话循环内核
- [x] api/server.go — HTTP API + SSE 路由
- [x] web/ — PWA (index.html + app.js + sw.js + manifest.json)
- [x] characters/anya.yml — 示例角色卡
- [x] worlds/cyberpunk2077/world.yml — 示例世界设定
- [x] 跨会话记忆（dialogue_history 表 + 重启恢复）
- [x] SillyTavern PNG 导入器（v1/v2 + character_book → world.yml ontology）

## Phase 1 — Verification Complete (6/6)
- [x] 50 轮人设不漂移 — Anya OOC 测试 + 多轮角色一致性
- [x] 重启后回忆第 3 轮事实 — dialogue_history 持久化，重启恢复 15 轮
- [x] Validator 拦截 OOC — Anya 拒绝卖萌/撒娇/鬼脸
- [x] Token < 3K — 实测 1348-2438/4000，稳定在 2.4K 以下
- [x] 手机 PWA — manifest.json + sw.js + app.js + 响应式
- [x] VPS PM2/systemd — deploy/corerp.service

## Phase 2 — Implementation Complete
- [x] events/quarantine.go — Gatekeeper 暂存区系统
- [x] simulation/tick.go — Simulation Tick Loop（60s 现实 = 5min 世界）
- [x] memory/confidence.go — Memory Confidence Pipeline
- [x] memory/decay.go — 记忆/关系衰减引擎
- [x] narrative/tension.go — Tension Engine（热寂检测 + 自然衰减）
- [x] state/machine.go — 叙事状态机（calm/tense/crisis/resolution）
- [x] agents/planner.go — 规则式自主规划器
- [x] importer → 双文件输出 + ontology seed pipeline

## Phase 2 — Verified
- [x] Tension Engine: 8 轮后 tension 0→0.2
- [x] State Machine: calm(0) → tense(0.35) → crisis(0.75)
- [x] Tick Loop: 65 秒世界推 5 分钟
- [x] Quarantine: auto-promote 生效
- [x] Planner: 动态目标注入 snapshot
- [x] Cross-session: 15 轮记忆恢复
- [x] Ontology Seed: 50 facts + 29 events → Semantic Memory

## Phase 3（多世界与因果）
- [x] 多角色加载 + 手动切换（`POST /api/switch`，前端下拉框）
- [x] agents/scheduler.go — 多 Agent 自主调度（规则式，零 LLM，每 3 tick/NPC 一次动作）
- [x] events/causality.go — 因果链引擎（自动链接 + 递归查询 + summary）
- [x] events/replay.go — 时间线回放/分叉（ReplayTo/ReplayAtTime/Fork/CompareStates）
- [x] narrative/compression.go — 事件升维抽象（按类型分组→摘要，AutoCompress 每 20 tick）
- [x] llm/router.go — 能力路由（narrative/summary/extraction 分任务 + fallback）

## Phase 3 — Complete (2026-05-25)
全部 6 项完成。
