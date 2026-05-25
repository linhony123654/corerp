# CoreRP — Persistent Narrative Runtime

世界状态驱动的 LLM 叙事引擎。不是一个更好的 SillyTavern，而是一个**文字世界模拟器**。

**状态：Phase 1/2/3 全部完成。** 所有架构模块已实现，前端 100% 覆盖后端 API。

## 核心哲学

- **三种真相分离**：Canonical Truth（YAML定义）→ Runtime State（Event Store 投影）→ Narrative Output（LLM 生成，经 Validator 校验）
- **LLM 不直接改世界**：所有状态变更必须经过 Action Frame → Executor → Event Commit → State Projection
- **Action Layer 强制存在**：LLM 先输出结构化 ActionFrame，Runtime 执行后再渲染文本

## 快速开始

```bash
go build -o corerp ./cmd/corerp/
./corerp serve -characters ./characters
```

打开 `http://localhost:8080`。首次访问需输入密码（如果设置了 `-auth-key`）。

### 配置

```bash
export LLM_URL="https://your-api/v1"
export LLM_API_KEY="your-key"
export LLM_MODEL="model-name"
export CORERP_AUTH_KEY="your-password"  # 可选，不设则免认证
```

## 架构

```
Interface (PWA/SSE) → API Gateway (Auth) → Runtime Core
  ├── Context OS     (Snapshot Compiler + 48K/96K Budget)
  ├── State Machine  (calm/tense/crisis/resolution)
  ├── Event Bus      (Store + Projector + Causality + Replay)
  ├── Goal System    (Primary/Secondary/Hidden + Planner)
  ├── Action Layer   (Frame + Executor + Dice)
  ├── Memory Engine  (Short-term/Working/Semantic/Episodic + Vector)
  └── Canon Layer    (Ontology + Facts + 三层目录)
→ Narrative Layer (Renderer + Tension + Compression)
→ LLM Adapter (Router: narrative/summary/extraction)
→ Storage (SQLite + Vector | YAML worlds/)
```

## 前端

Swiss Modernism editorial design。三主题：暗色 / 亮色 / 牛皮纸。

右侧面板全覆盖：场景、用量、LLM 配置管理、模型拉取、时间线（可点击因果链+回放）、分叉、角色切换、NPC 动态、记忆状态、Tension 滑块、事件压缩。

## 多角色支持

- 加载 `characters/` 目录下所有 `.yml`，面板点击或下拉框切换
- 每个角色独立记忆、世界场景、对话历史、写作风格
- 非活跃角色后台规则式自主行动（Scheduler，零 LLM 消耗）
- 切换时显示"你不在时发生的事"摘要

## 世界文件结构（三层分离）

```
worlds/{name}/
├── world.yml              # meta + core_rules（世界观底层规则）
├── canon/
│   ├── ontology.yml       # 实体定义（角色/地点/物品/体系）
│   └── facts.yml          # 不可变事实（subject-predicate-object）
└── scenes/
    └── default.yml        # 场景状态（atmosphere/present_chars/tension）
```

导入角色卡自动生成三层结构。向后兼容旧版单文件 `_world.yml`。

## TRPG 骰子判定

```bash
/roll trust           # 2d6 + trust修正
/roll 3d6+fear 10     # 3d6+fear，难度10
/r d20                # 快捷方式
```

角色数值 0-10 → 修正 -3 到 +5。判定结果注入对话上下文。

## 写作指导

角色卡 YAML 支持 `writing_guide`，放入"风格约束"层（不影响事实记忆）：

```yaml
identity:
  writing_guide: |
    每轮回复 400-600 字。感官细节优先，内心活动占 30%。
    叙事节奏：环境观察 → 身体反应 → 动作 → 对话。
```

## 向量检索

- < 100 条：关键词 LIKE
- >= 100 条：自动切换向量（256-dim 2-gram 或 bge-small-zh-v1.5 512-dim）
- 质量过滤：RecallMinScore=0.30, RecallTopK=5

```bash
pip3 install sentence-transformers
python3 embed_server.py &
```

## Token 用量 + 定价

每次 LLM 调用记录到 `data/llm_usage.jsonl`。面板显示实时统计 + 费用估算，可自定义模型单价。按天/周/月/任务/模型聚合。每月自动压缩。

## 认证

```bash
./corerp serve -auth-key "your-password"
```

HMAC 签名 session token，24h TTL，httpOnly cookie。不设则免认证。

## 角色卡导入

```bash
./corerp import -src card.png -dst ./characters       # PNG
./corerp import -src card.json -dst ./characters      # JSON (v3)
./corerp import -src ./cards_dir -dst ./characters    # 批量
```

支持 SillyTavern v1/v2/v3，自动提取 character_book → 三层世界目录。

## API 端点（全部 24 个）

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/chat` | POST | SSE 流式对话 |
| `/api/state` | GET | 当前世界状态 |
| `/api/characters` | GET | 角色列表 + 当前活跃 |
| `/api/switch` | POST | 切换角色 |
| `/api/dialogue` | GET | 对话历史 |
| `/api/dialogue/reset` | POST | 重置对话 |
| `/api/world` | GET | 世界信息 |
| `/api/npc-actions` | GET | NPC 行动日志 |
| `/api/causality` | GET | 因果链查询 |
| `/api/replay` | GET | 回放世界状态 |
| `/api/fork` | POST | 创建时间线分叉 |
| `/api/timeline` | GET | 时间线事件列表 |
| `/api/branches` | GET | 分叉列表 |
| `/api/compress` | POST | 手动事件压缩 |
| `/api/compression-stats` | GET | 压缩统计 |
| `/api/usage` | GET | Token 用量 + 费用 |
| `/api/llm-configs` | GET/POST | LLM 配置管理 |
| `/api/llm-configs/<name>` | DELETE | 删除配置 |
| `/api/llm-active` | GET/POST | 活跃配置 + 定价 |
| `/api/llm-models` | GET | 拉取模型列表 |
| `/api/llm-routes` | GET | 路由表 |
| `/api/director` | POST | 调整 Tension |
| `/api/debug/memory` | GET | 调试信息 |

## 部署

```bash
sudo cp deploy/corerp.service /etc/systemd/system/
sudo systemctl enable --now corerp
cp data/memory.db backup/memory_$(date +%Y%m%d).db  # 备份
```

## 技术栈

Go 1.22+ / SQLite / SSE / Vanilla JS PWA / BGE-small-zh / Swiss Modernism / 单二进制

## 目录结构

```
corerp/
├── cmd/corerp/main.go      # CLI
├── internal/
│   ├── actions/            # Action Frame + Executor + Dice
│   ├── agents/             # Identity + Validator + Planner + Scheduler
│   ├── api/                # HTTP + SSE
│   ├── auth/               # HMAC 认证
│   ├── context/            # Snapshot Compiler + Token Budget
│   ├── core/               # 共享类型
│   ├── events/             # Store + Quarantine + Causality + Replay
│   ├── importer/           # PNG/JSON 导入 → 三层目录
│   ├── llm/                # Adapter + Router + Usage + Config
│   ├── memory/             # 四层记忆 + Vector Search
│   ├── narrative/          # Tension + Compression
│   ├── runtime/            # 对话循环内核
│   ├── simulation/         # Tick Loop
│   └── state/              # WorldState + 状态机
├── web/                    # PWA (Swiss Modernism editorial)
├── deploy/                 # systemd
├── embed_server.py         # BGE 嵌入服务
├── budgets.yml             # Token 预算配置
└── data/                   # SQLite / 用量日志 (gitignored)
```
