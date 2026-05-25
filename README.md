# CoreRP — Persistent Narrative Runtime

世界状态驱动的 LLM 叙事引擎。不是一个更好的 SillyTavern，而是一个**文字世界模拟器**。

## 核心哲学

- **三种真相分离**：Canonical Truth（YAML定义）→ Runtime State（事件溯源投影）→ Narrative Output（LLM生成，经Validator校验）
- **LLM不直接改世界**：所有状态变更必须经过 Action Frame → Executor → Event Commit → State Projection
- **Action Layer 强制存在**：LLM 先输出结构化 ActionFrame，Runtime 执行后再渲染文本

## 快速开始

```bash
go build -o corerp ./cmd/corerp/
./corerp serve -characters ./characters
```

打开 `http://localhost:8080`。首次访问需输入访问密码（如果设置了 `-auth-key`）。

### 配置

```bash
# LLM 连接（必需）
export LLM_URL="https://your-api/v1"
export LLM_API_KEY="your-key"
export LLM_MODEL="model-name"

# 访问密码（可选，不设则无需认证）
export CORERP_AUTH_KEY="your-password"
# 或: ./corerp serve -auth-key "your-password"
```

## 前端功能

### 主题

Header ◐ 按钮三档循环切换：暗色 → 亮色 → 牛皮纸（vintage warm）。

### 面板

右侧控制面板（移动端右下角按钮展开）：

| 区域 | 内容 |
|------|------|
| 场景 | 地点、时间、天气、叙事状态、张力指示器 |
| 用量 | 调用次数、Token总量、预估费用（可自定义模型定价） |
| LLM 配置 | 当前活跃 API、已保存配置增删、模型列表一键拉取 |
| 世界 | 世界名称、分叉数、压缩统计 |
| 时间线 | 事件流、分叉切换、创建新分叉 |
| 角色 | 角色列表、点击切换 |
| NPC 动态 | 后台角色自动行动摘要 |
| 记忆 | 事实数、检索模式（向量/关键词）、对话轮数 |

### 命令

在聊天框输入 `/roll` 进行 TRPG 判定，零 LLM 消耗。

## 多角色支持

- 加载 `characters/` 目录下所有 `.yml`，前端下拉框或面板点击切换
- 每个角色独立记忆、世界场景、对话历史、写作风格
- 非活跃角色在后台规则式自主行动（Scheduler）
- 切换时显示"你不在时发生的事"摘要

## 写作指导

角色卡 YAML 支持 `writing_guide` 字段：

```yaml
identity:
  writing_guide: |
    每轮回复 400-600 字。
    感官细节优先，内心活动占 30%。
    叙事节奏：环境观察 → 身体反应 → 动作 → 对话。
```

## TRPG 骰子判定

```bash
/roll trust           # 2d6 + trust修正
/roll 3d6+fear 10     # 3d6+fear，难度10，判定成功/失败
/r d20                # /r 快捷方式
```

角色数值（trust/fear/intimacy/debt/respect, 0-10）自动映射为骰子修正（-3 到 +5）。

## 向量检索

- 数据 < 100 条：关键词 LIKE
- 数据 >= 100 条：自动切换向量检索
  - 默认：256-dim 2-gram（纯 Go）
  - 可选：`BAAI/bge-small-zh-v1.5`（512-dim 语义嵌入）

```bash
pip3 install sentence-transformers
python3 embed_server.py &   # 首次自动下载模型 ~100MB
```

## Token 用量 + 定价

每次调用记录到 `data/llm_usage.jsonl`。面板用量区域显示实时统计和费用估算。

- 按天/周/月/任务/模型聚合
- 默认 DeepSeek 定价，面板 `¥` 按钮可自定义模型单价
- 每月自动压缩历史月份为一行 rollup

## 角色卡导入

```bash
./corerp import -src card.png -dst ./characters   # 单张
./corerp import -src ./cards_dir -dst ./characters # 批量
```

支持 SillyTavern v1/v2 PNG，自动提取 character_book → ontology → Semantic Memory。

## API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/chat` | POST | SSE 流式对话 |
| `/api/state` | GET | 当前世界状态 |
| `/api/characters` | GET | 所有角色 + 当前活跃 |
| `/api/switch` | POST | 切换角色（返回 NPC 摘要） |
| `/api/world` | GET | 世界信息 |
| `/api/dialogue` | GET | 获取对话历史（刷新恢复） |
| `/api/dialogue/reset` | POST | 重置当前角色对话 |
| `/api/npc-actions` | GET | NPC 行动日志 |
| `/api/causality` | GET | 因果链查询 `?id=evt_xxx&depth=3` |
| `/api/replay` | GET | 回放世界状态 `?id=evt_xxx` 或 `?time=d:h:m` |
| `/api/fork` | POST | 创建时间线分叉 |
| `/api/timeline` | GET | 时间线事件列表 |
| `/api/branches` | GET | 分叉列表 |
| `/api/compress` | POST | 手动事件压缩 |
| `/api/compression-stats` | GET | 压缩统计 |
| `/api/usage` | GET | Token 用量 + 费用估算 |
| `/api/llm-configs` | GET/POST | LLM 配置管理 |
| `/api/llm-configs/<name>` | DELETE | 删除配置 |
| `/api/llm-active` | GET/POST | 活跃配置 + 更新定价 |
| `/api/llm-models` | GET | 从 API 拉取模型列表 |
| `/api/llm-routes` | GET | LLM 路由表 |
| `/api/director` | POST | 调整 Tension |
| `/api/debug/memory` | GET | 调试信息 |

## 部署

```bash
sudo cp deploy/corerp.service /etc/systemd/system/
sudo systemctl enable --now corerp
```

## 技术栈

Go 1.22+ / SQLite / SSE / Vanilla JS PWA / Swiss Modernism Design / 单二进制

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
│   ├── importer/           # PNG 导入
│   ├── llm/                # Adapter + Router + Usage + Config
│   ├── memory/             # 四层记忆 + Vector Search
│   ├── narrative/          # Tension + Compression
│   ├── runtime/            # 对话循环内核
│   ├── simulation/         # Tick Loop
│   └── state/              # WorldState + 状态机
├── web/                    # PWA (Swiss Modernism editorial)
├── deploy/                 # systemd
├── embed_server.py         # BGE 嵌入服务
├── budgets.yml             # Token 预算
└── data/                   # SQLite / 用量日志 (gitignored)
```
