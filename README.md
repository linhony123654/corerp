# CoreRP — Persistent Narrative Runtime

世界状态驱动的 LLM 叙事引擎。不是一个更好的 SillyTavern，而是一个**文字世界模拟器**。

## 核心哲学

- **三种真相分离**：Canonical Truth（YAML定义）→ Runtime State（事件溯源投影）→ Narrative Output（LLM生成，经Validator校验）
- **LLM不直接改世界**：所有状态变更必须经过 Action Frame → Executor → Event Commit → State Projection
- **Action Layer 强制存在**：LLM 先输出结构化 ActionFrame，Runtime 执行后再渲染文本

## 快速开始

```bash
# 编译
go build -o corerp ./cmd/corerp/

# 单角色模式
./corerp serve -character characters/example.yml -world worlds/example.yml

# 多角色模式（支持切换）
./corerp serve -characters ./characters

# 配置 LLM
export LLM_URL="https://your-llm-api/v1"
export LLM_API_KEY="your-key"
export LLM_MODEL="gemini-3-flash-preview"
```

打开 `http://localhost:8080`，PWA 可安装到手机。

## 语义嵌入（可选）

```bash
# 安装依赖并启动嵌入服务器
pip3 install sentence-transformers
python3 embed_server.py &

# CoreRP 启动时自动检测，不可用时回退 2-gram
./corerp serve -characters ./characters
```

模型：`BAAI/bge-small-zh-v1.5`（512-dim，专为中国语义优化）。首次启动自动从 HuggingFace 下载（~100MB）。

## 架构

```
Interface (PWA/SSE) → API Gateway → Runtime Core
                                        ├── Context OS (Snapshot Compiler + Token Budget)
                                        ├── State Machine (calm/tense/crisis/resolution)
                                        ├── Event Bus (Store + Projector + Causality + Replay)
                                        ├── Goal System (Primary/Secondary/Hidden + Planner)
                                        ├── Action Layer (Frame Definition + Executor)
                                        ├── Memory Engine (Short-term/Working/Semantic/Episodic + Vector)
                                        └── Canon Layer (Ontology + Facts + Consistency)
                                     → Narrative Layer (Renderer + Tension + Compression)
                                     → LLM Adapter (Router: narrative/summary/extraction)
                                     → Storage (SQLite + Vector | YAML worlds/characters)
```

## 多角色支持

- 加载 `characters/` 目录下所有 `.yml`，前端下拉框切换
- 每个角色独立记忆、世界场景、对话历史、写作风格
- 非活跃角色在后台规则式自主行动（Scheduler，零 LLM 消耗）
- 切换时显示"你不在时发生的事"摘要

## Token 预算

`budgets.yml` 可配置：

| 模式 | 总量 | 用途 |
|------|------|------|
| normal | 48K | 日常对话 |
| full_load | 96K | 角色切换/首次加载 |

分配：ontology 20% / 事实 20% / 事件 20% / 对话 15% / 角色 12% / 场景 5% / 摘要 8%

## 写作指导

角色卡 YAML 支持 `writing_guide` 字段，放入 Snapshot 的"风格约束"层（不影响角色所知事实）：

```yaml
identity:
  writing_guide: |
    每轮回复 400-600 字。
    感官细节优先，内心活动占 30%。
    叙事节奏：环境观察 → 身体反应 → 动作 → 对话。
```

## 向量检索

- 数据 < 100 条：关键词 LIKE 匹配
- 数据 >= 100 条：自动切换向量检索
  - 默认：256-dim 2-gram（纯 Go，零外部依赖）
  - 可选：`BAAI/bge-small-zh-v1.5`（512-dim 语义嵌入，"欠"↔"债务" 0.55）
  - 质量过滤：RecallMinScore=0.30，RecallTopK=5

## Token 用量追踪

每次 LLM 调用自动记录到 `data/llm_usage.jsonl`：

```bash
# 查看统计
curl http://localhost:8080/api/usage
# → {"total_calls":42, "total_tokens": 180000, "estimated_cost": "¥0.27",
#    "by_day": {...}, "by_week": {...}, "by_month": {...}}
```

- 按天/周/月/任务/模型聚合
- DeepSeek 计费标准（prompt ¥1/M, completion ¥4/M）
- 每月启动时自动压缩历史月份为一行 rollup

## 角色卡导入

```bash
# 从 SillyTavern PNG 角色卡导入
./corerp import -src character_card.png -dst ./characters

# 批量导入
./corerp import -src ./cards_dir -dst ./characters
```

支持 v1/v2 格式，自动提取 character_book → ontology → Semantic Memory。

## API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/chat` | POST | SSE 流式对话 |
| `/api/state` | GET | 当前世界状态 |
| `/api/character` | GET | 当前角色卡 |
| `/api/characters` | GET | 所有角色列表 + 当前活跃 |
| `/api/switch` | POST | 切换活跃角色（返回 NPC 摘要） |
| `/api/world` | GET | 世界名称 |
| `/api/npc-actions` | GET | NPC 后台行动日志 |
| `/api/causality` | GET | 事件因果链查询（?id=evt_xxx&depth=3） |
| `/api/replay` | GET | 回放任意时刻世界状态（?id=evt_xxx 或 ?time=d:h:m） |
| `/api/fork` | POST | 创建时间线分叉 |
| `/api/timeline` | GET | 时间线事件列表 |
| `/api/branches` | GET | 分叉列表 |
| `/api/compress` | POST | 手动事件压缩 |
| `/api/compression-stats` | GET | 压缩统计 |
| `/api/usage` | GET | Token 用量 + 费用估算 |
| `/api/llm-routes` | GET | LLM 路由表 |
| `/api/director` | POST | 导演接口（调整 Tension） |
| `/api/debug/memory` | GET | 调试信息（含 vector_search 状态） |

## 部署

```bash
# systemd
sudo cp deploy/corerp.service /etc/systemd/system/
# 编辑填入 LLM_API_KEY
sudo systemctl enable --now corerp

# SQLite 备份
cp data/memory.db backup/memory_$(date +%Y%m%d).db
```

## 技术栈

Go 1.22+ / SQLite / SSE / Vanilla JS PWA / BGE-small-zh / 单二进制部署

## 目录结构

```
corerp/
├── cmd/corerp/main.go      # CLI 入口
├── internal/
│   ├── actions/            # Action Frame + Executor
│   ├── agents/             # Identity + Validator + Planner + Scheduler
│   ├── api/                # HTTP 路由 + SSE
│   ├── context/            # Snapshot Compiler + Token Budget
│   ├── core/               # 全项目共享类型
│   ├── events/             # Event Store + Projection + Quarantine + Causality + Replay
│   ├── importer/           # SillyTavern PNG 导入
│   ├── llm/                # Adapter + Router + Usage Tracker
│   ├── memory/             # 四层记忆 + Vector Search (2-gram + bge-zh)
│   ├── narrative/          # Tension Engine + Compression
│   ├── runtime/            # 对话循环内核
│   ├── simulation/         # Tick Loop
│   └── state/              # WorldState 管理 + 状态机
├── characters/             # 角色卡 (gitignored, example.yml 除外)
├── worlds/                 # 世界设定 (gitignored, example.yml 除外)
├── web/                    # PWA (index.html + app.js + sw.js)
├── deploy/                 # systemd 配置
├── embed_server.py         # BGE 嵌入微服务
├── budgets.yml             # Token 预算配置
└── data/                   # SQLite / 用量日志 (gitignored)
```
