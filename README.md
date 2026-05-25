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
./corerp serve -character characters/anya.yml -world worlds/cyberpunk2077/world.yml

# 多角色模式（支持切换）
./corerp serve -characters ./characters -world worlds/cyberpunk2077/world.yml

# 配置 LLM
export LLM_URL="https://your-llm-api/v1"
export LLM_API_KEY="your-key"
export LLM_MODEL="gemini-3-flash-preview"
```

打开 `http://localhost:8080`，PWA 可安装到手机。

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

```bash
# 加载 characters/ 目录下所有 .yml
./corerp serve -characters ./characters

# 前端下拉框切换角色
# API: GET /api/characters, POST /api/switch
# 每个角色独立记忆/世界/对话历史
# 非活跃角色在后台规则式自主行动（scheduler）
```

## Token 预算

`budgets.yml` 配置：

| 模式 | 总量 | 用途 |
|------|------|------|
| normal | 48K | 日常对话 |
| full_load | 96K | 角色切换/首次加载 |

分配：ontology 20% / 事实 20% / 事件 20% / 对话 15% / 角色 12% / 场景 5% / 摘要 8%

## 向量检索

- 数据 < 100 条：关键词匹配
- 数据 >= 100 条：自动切换向量检索（256维 2-gram，零外部依赖）
- 升级路径：替换 `Embed()` 为 `BAAI/bge-small-zh-v1.5` ONNX 模型

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
| `/api/characters` | GET | 所有角色列表 |
| `/api/switch` | POST | 切换活跃角色 |
| `/api/world` | GET | 世界名称 |
| `/api/npc-actions` | GET | NPC 后台行动日志 |
| `/api/causality` | GET | 事件因果链查询 |
| `/api/replay` | GET | 回放任意时刻世界状态 |
| `/api/fork` | POST | 创建时间线分叉 |
| `/api/timeline` | GET | 时间线事件列表 |
| `/api/branches` | GET | 分叉列表 |
| `/api/compress` | POST | 手动事件压缩 |
| `/api/compression-stats` | GET | 压缩统计 |
| `/api/llm-routes` | GET | LLM 路由表 |
| `/api/director` | POST | 导演接口（调张力） |
| `/api/debug/memory` | GET | 调试信息 |

## 部署

```bash
# systemd
sudo cp deploy/corerp.service /etc/systemd/system/
# 编辑 /etc/systemd/system/corerp.service 填入 LLM_API_KEY
sudo systemctl enable --now corerp

# SQLite 备份
cp data/memory.db backup/memory_$(date +%Y%m%d).db
```

## 技术栈

Go 1.22+ / SQLite / SSE / Vanilla JS PWA / 单二进制部署

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
│   ├── llm/                # Adapter + Router
│   ├── memory/             # 四层记忆 + Vector Search
│   ├── narrative/          # Tension Engine + Compression
│   ├── runtime/            # 对话循环内核
│   ├── simulation/         # Tick Loop
│   └── state/              # WorldState 管理 + 状态机
├── characters/             # 角色卡 YAML
├── worlds/                 # 世界设定 YAML
├── web/                    # PWA (index.html + app.js + sw.js)
├── deploy/                 # systemd 配置
├── budgets.yml             # Token 预算配置
└── data/                   # SQLite 数据库 (gitignored)
```
