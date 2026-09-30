# CoreRP 叙事与世界运行闭环专项审计报告

> 审计日期：2026-09-26 · 审计性质：**只读**（未修改任何代码、未提交）
> 审计对象：/home/ubuntu/corerp-console（含预览服务器线上实例 world_honglou_60615b11 实测数据）
> 所有结论均给出 `文件:行号` 证据；「没有/不存在」类结论均附检索证据。
>
> ## 勘误与证据对齐（2026-09-26 二轮复核）
>
> 初版报告有两处统计表述错误，已被用户指出并经数据库逐条重新核对，现更正如下（正文相应段落已同步修改）：
>
> **勘误 1**：初版称「27 条 RPSpeechAccepted 全部来自玩家」。**错误。** 实际分解（会话 rps_043ca865，world_honglou_60615b11 / br_main，即玩家红楼会话）：
>
> | 来源 | 条数 | 说明 |
> |---|---|---|
> | 玩家（贾宝玉） | 11 | turns/run 提交的 RPSpeechAccepted |
> | NPC 回合决策 respond | 14 | CommitRPDecision 提交的 RPSpeechAccepted（王熙凤 5 + 晴雯 4 + 袭人 4 + 王熙凤另 1，写入 rp_npc_decisions） |
> | NPC wait-initiative  speech | 2 | 两次 wait（01:00、05:00）各触发 1 条王熙凤主动开白——**该路径写 RPSpeechAccepted + rp_utterances，但不写 rp_npc_decisions 行**（d.action 为空的 2 行即此） |
> | 合计 | 27 | 与事件表 27 条 RPSpeechAccepted 一一对上 |
>
> **结论修正**：对白提交链完全按设计工作，NPC 的每句回答都是 committed event 且存真实 entity_id。初版「5 世界小时只有 1 个 committed NPC 效果」亦错误——wait 期间实际提交了 3 个 NPC 效果（2 条 initiative 对白 + 1 条 silence）。
>
> **勘误 2（数据反而强化了核心结论）**：那 2 条 wait-initiative 对白在 committed event 里逐字复现了 00:00 的开场白——01:00「哟，宝兄弟怎么有空到我这儿来了？快坐，平儿，还不快给宝玉沏茶」、05:00「哟，是什么风把宝兄弟吹到我这儿来了？快坐，平儿，还不快给宝玉……」。**自主机制确实在 wait 时点火，但因为决策输入不含 NPC 自己的历史言行（P0-2），它每 4 个小时把同一场景重新开一次。**「重复请坐倒茶」不是文本去重问题，是 committed event 级别的状态缺失问题——这是本审计最有力的物证。
>
> **两处表述收窄**（接受用户指正）：
> - 「一个 text 字段 ⇒ 只能输出一句话」不成立。text 上限 2000 字（`core/rp_proposal.go:27`），schema 并未限制长度。精确的结构性限制是：**该通道只能承载台词，没有动作/神态/场景字段**；"短"来自决策任务被定义为"回一句对白"（decisionInstruction 的框架）加上输入里没有别的可演。
> - 「没有后台循环 ⇒ 不能自主模拟」不成立。wait 路径的 initiative/warm 就是自主模拟（本轮数据已证实它会真实提交事件）。真正需要证明的两件事是：①动作能否**结构化提交**（当前不能——action 枚举封闭）；②时间推进时 NPC 能否不依赖玩家逐个指挥而行动（部分能——wait initiative，但限同场景、每小时冷却、动作词汇表仅 respond/silence/wait/leave）。

---

## A. Executive Summary

**1. 当前 CoreRP 为什么像短对话机器人？**

三个结构性原因叠加，Prompt 一个都救不了：

- **决策输出 schema 只有一个 `text` 字段，且任务被定义为"回一句对白"。** NPC 决策的封闭 schema 是 `{action, text, destination_place_id}`（`backend/internal/decision/chat.go:114-124`），合法 action 只有 respond/refuse/leave/silence/wait 五种（`backend/internal/core/rp_proposal.go:25-48`）。NPC 的"表演"通道只能承载台词，schema 里**没有**承载动作、神态、场景互动的字段。需要收窄说明：text 上限 2000 字（`core/rp_proposal.go:27`），schema 并未限制一句话还是多句话——**输出短的根因是决策任务被框架化为"对玩家这句话作出回应"**（`decision/chat.go:83-88` 的 decisionInstruction），加上决策输入里没有别的可演（见 B2）。是"通道只有台词 + 任务只是回话"共同决定了短，不是长度限制。
- **叙事层的事实集就是对白集。** 一回合进 `readRPNarrativeInput`（`backend/internal/storage/rp_turn_view.go:28-114`）的事实只有 6 种 action：玩家 speak + NPC respond/refuse/silence/wait/leave。全仓库非测试代码中 `RPNarrativeFact{` 构造点仅 2 处（`rp_turn_view.go:30,51`）。**不存在"场景变化/NPC 动作/环境"这类事实类型**（`RPNarrativeFact` 结构本身没有字段可放，`backend/internal/core/rp_style.go:136-144`）。渲染器每事实一行「某某说：「…」」（`rp_style.go:230-246`），这是它的全部能力——代码注释明说 "Sparse scenes stay short; never pad"（`rp_style.go:263`）。
- **full_prose 也扩不出来。** `ChatProseProvider` 只拿到这 6 种事实，prompt 被允许"补充不涉及新事实的氛围细节"（`backend/internal/narrative/prose.go:72`），且默认关闭、校验失败即回退单行模板。

**2. 世界是否真的在玩家不说话时继续运行？**

**不是后台式的，但也不是完全静止——需要精确表述。** 服务器进程内没有任何后台循环（全 backend 仅 SSE 只读轮询 ticker，`backend/internal/storage/rp_events.go:134`；唯一的自动推进器是离线 CLI demo 工具 `backend/internal/storage/agent_driver.go`，不在服务里跑）。世界时间在玩家请求之间不前进。**但**「没有后台循环 ≠ 不能自主模拟」：wait 路径的 initiative/warm 决策是真实的自主行动，会提交 committed event（本轮线上数据已证实，见 A.3 勘误后的事实）。真正的两个缺口是：① NPC 的**动作**无法结构化提交（action 枚举封闭，只能"说/走/沉默/等待"）；② 自主行动只在玩家点"等待"时发生，且限同场景、每小时冷却、词汇表封闭。

**3. wait 是否真正推进世界？**

**半真半假，且在红楼世界里实测等于没有。** 机制上 wait 做三件事（`backend/internal/storage/rp_wait.go:59-101` + `backend/internal/storage/rp_service.go:114-138`）：drain 日程队列（确定性执行 scheduler_items，无 LLM）→ 推进时钟 → 对"hot roster"NPC 跑 initiative 决策（有 LLM）。**但**：

1. Studio 建世界不写任何日程表（`studio_*.go` 全文件 grep `agent_schedule_entries|scheduler_items` 零命中；唯一写入点是 background materialize 和 M2 demo/career 路径），所以 drain 无事可做；
2. initiative 限同场景 NPC、每小时冷却、每天 ≤ budget 次。

线上取证（会话 rps_043ca865，world_honglou_60615b11/br_main）：两次 wait（00:00→01:00→05:00，共 5 世界小时）期间，自主机制**确实点火**——王熙凤提交了 3 个 committed NPC 效果：01:00 和 05:00 各 1 条 initiative 对白 + 05:00 前 1 条 initiative silence。初版报告误读为"只有 1 个 silence"，已勘误。

**但数据恰恰把用户抱怨的"重复请坐倒茶"钉死在 committed event 级别**：那 2 条 initiative 对白在事件库里逐字复现开场白——01:00「哟，宝兄弟怎么有空到我这儿来了？快坐，平儿，还不快给宝玉沏茶去。」、05:00「哟，是什么风把宝兄弟吹到我这儿来了？快坐，平儿，还不快给宝玉倒茶来。」（event_id 以 `event_rp_initiative_` 开头，turn_id 以 `turn_rp_initiative_` 开头，确认来自 `rp_initiative_commit.go:227-234,271-275` 的提交路径）。自主引擎每 4 个小时把同一场景重新开一次，因为**王熙凤自己的历史言行虽已入账（committed event、可查询），但从不进入她自己的决策上下文**（P0-2）——对决策模型来说，"我 5 小时前请过坐、倒过茶"与开局无异。她的"忙"也从来不是世界事实——initiative 输入里她的 ActivityCode 只有 schedule drain 留下的确定性痕迹，而红楼世界无日程（见下）。

**4. Narrative 是否只是在生成 NPC speech？**

**结算层连"生成"都不是——是模板拼接。** `turns/run` 返回的 narrative 硬编码走 `DeterministicRPNarrativeProvider`（`rp_turn_view.go:25`），LLM 叙事 provider 只在 `narrative/stream` 重渲染时可选生效。

**5. Simulation → committed event → Narrative 闭环是否完整？**

**对白链的闭环是完整的、纪律是严格的**（叙事只读 committed facts，绝不回写世界，这条底线守得很好）；初版"统计与链路的矛盾"经二轮复核证实是统计误读，非链路断裂（见卷首勘误）。**但闭环有一个真实的不对称缺口**：turn 决策链把每步写入 `rp_npc_decisions`（input_hash+proposal_hash 幂等，`rp_decision_commit.go:183`），而 **initiative 链不写 `rp_npc_decisions`**（该表全仓唯一 INSERT 在 `rp_decision_commit.go:183`；`rp_initiative_commit.go` 通篇无此写入）——initiative 决策的证据链只剩 `agent_audit`，两条路径的审计纪律不一致。**断的还有闭环两端的"带宽"**：世界能提交的事件类型太窄（只有对白/移动/沉默），叙事能消费的事实类型随之太窄。

**6. 最根本的问题是 Prompt、Renderer、数据模型还是 Runtime？**

**数据模型 + Runtime，不是 Prompt。** 按严重度：①决策 action 枚举封闭（数据模型）；②NPC 无自传记忆、无 scene 状态（Runtime 缺事实类型）；③叙事事实集=对白集（数据模型）；④身份投影被 demo 硬编码锁死（Runtime）。Prompt 在这四层面前没有作用空间。

---

## B. 真实调用链

### B1. 玩家说一句对白（`turns/run`）

```
前端 src/components/PlayWorkspace.vue:349  speak()
  └─ act('turns/run', { text })            ← 「继续剧情」与普通对白完全同路径，无任何特判
后端 backend/internal/transport/httpapi/server.go:431  handleRPTurnRun
  └─ resolveRPDecisionOverride (httpapi/rp_model.go)  解析玩家自带模型为内存 provider
  └─ RPService.PlayRPTurnWith (storage/rp_service.go:55)
     └─ Store.RunRPTurn (storage/rp_turn.go:86)
        ├─ SpeakRP (storage/rp_speech.go:51)        玩家 speech 单事务提交：
        │    events: RPSpeechAccepted                payload 含真实 SpeakerEntityID（身份数据层没丢）
        │    rp_utterances + 每个 listener 写 agent_knowledge(speaker_said)
        │    UPDATE world_clocks/branches/rp_sessions
        ├─ FOR speech.ListenerIDs 中每个在场 NPC (rp_turn.go:125-161)：
        │    ├─ DecideRP (storage/rp_decision.go:254)
        │    │    BuildRPDecisionInput               组装 RPDecisionInput（见 B2）
        │    │    provider.Propose                   decision/chat.go:90 → 外部 LLM
        │    │    ValidateRPDecisionProposal          core/rp_proposal.go:14（封闭校验）
        │    └─ CommitRPDecision (storage/rp_decision_commit.go:35)
        │         respond/refuse → RPSpeechAccepted + rp_utterances
        │         leave        → RPNPCMoved（真实移动 agent_positions）
        │         silence/wait → RPNPCDecisionRecorded
        │         全部写 rp_npc_decisions（input_hash+proposal_hash 幂等）
        └─ renderRPTurnStyled (storage/rp_turn_view.go:19)
             readRPNarrativeInput                    事实集 = 玩家 speak + 本回合 NPC 决策
             → 硬编码 DeterministicRPNarrativeProvider（rp_turn_view.go:25）
             存 rp_turn_runs.narrative_json → 返回 NarrativeLines
前端随后 narrative/stream (server.go:423) 用配置的叙事 provider 重渲染同一份事实
```

### B2. NPC 决策输入到底有什么（`backend/internal/core/rp_decision.go:47-80`，组装 `backend/internal/storage/rp_decision.go:25-250`）

| 字段 | 有/没有 |
|---|---|
| 玩家当轮对白 | ✅ `PlayerSpeechText`（且有"确实听到"证据校验 `rp_decision.go:70-75`） |
| NPC 名下资产 | ✅ `OwnAssetMinor` |
| 同场可见实体 | ✅ `VisibleEntities` |
| NPC 自己的知识 | ✅ `Knowledge`：agent_knowledge 最近 20 条，**只暴露 speaker_said / agent_presence 两类**（`rp_decision.go:158-165`，其余 claim 类型被丢弃） |
| 下一日程 | ✅ `NextSchedule`（红楼世界里恒为 nil——无日程数据） |
| 历史回合 | ❌ 结构体无此字段 |
| NPC 自己上一轮说过/做过什么 | ❌ **结构性缺失**——listeners 计算排除自己（`backend/internal/storage/rp_perception.go:88-89`），NPC 自己的言行从不写进自己的 knowledge |
| scene 状态/已完成动作记录 | ❌ 无任何字段，数据库也无此类表 |

### B3. wait（`actions/wait`）

```
handleRPWait → RPService.WaitRPWith (storage/rp_service.go:77)
  └─ Store.WaitRP (storage/rp_wait.go:59)
     ├─ runAgentLifeForScope (storage/agent.go:340)   drain scheduler_items（确定性、无 LLM）
     │                                                 红楼世界：无日程 → 无事发生
     ├─ finishRPWait (rp_wait.go:267)                 提交 RPWaitCompleted + 推进 world_clocks
     │    ├─ hot initiative roster ≤16 人              readRPHotInitiativeRoster
     │    └─ warm candidates ≤4 人                     readRPWarmCandidates（确定性条件）
  └─ finishRPWaitPresentation (rp_service.go:114)
     ├─ 每个 warm NPC：runRPWarmDecision                无 LLM，确定性规则（在工作→wait 等）
     └─ 每个 initiative NPC：RunRPInitiative           有 LLM；cadence=每 actor 每小时 ≤1、
                                                        每天 < rules.npc.daily_budget（唯一消费
                                                        budget 的地方，rp_initiative_commit.go:96-110）
                                                        legal actions 仅 respond/silence/wait/leave
                                                        （rp_initiative.go:52）
                                                        commitRPInitiative（rp_initiative_commit.go:177-326）：
                                                        respond → RPSpeechAccepted + rp_utterances
                                                        （turn_id 形如 turn_rp_initiative_*，:233,272）
                                                        leave → RPNPCMoved（:236,278-281）
                                                        其余 → RPNPCDecisionRecorded（:225-226）
                                                        ⚠ 不写 rp_npc_decisions（全仓唯一 INSERT 在
                                                        rp_decision_commit.go:183）——initiative 决策
                                                        无决策证据表行，只剩 agent_audit（:283）
```

### B4. 身份解析链（结论：呈现层问题，不是数据损坏）

```
提交层：events.actor_id / payload.SpeakerEntityID / rp_utterances.speaker_entity_id
        全部存真实 entity_id（rp_speech.go:207, rp_decision_commit.go:182,191）
呈现层：rpIdentityKnown 查 rp_identity_familiarity（storage/rp_identity.go:14-21）
        不认识 → DisplayName="陌生人" + 匿名 entity_id（rp_session.go:327-338,
        rp_turn_view.go:99-112, rp_events.go:207-232）
```

事件里存的是真人——「陌生人」是观察视角脱敏。**但 familiarity 的合法写入通道只有一条**：玩家对白带 `introduce_self=true`（`rp_speech.go:218-224`，且方向是 NPC→认识玩家）。NPC 自报姓名只是文本，永不产生认识关系；Studio 世界无任何种子，且投影校验硬编码 M2 特判（`storage/rp_identity_projection.go:84`：玩家不是 `entity_m2_rp_lin` 的 `RPParticipantsInitialized` 直接判 diverged 报错；绕过事件直接补行会被判 `rp_identity_extra` 拒绝）——**Studio 世界在结构上永远无法初始化"互相认识"**。

---

## C. 问题清单

### P0 — 架构/事实一致性

**P0-1　决策动作枚举封闭：世界只能发生"说/走/沉默/等待"**

- 证据：`backend/internal/core/rp_proposal.go:25-48`（switch 只接受五种 action，default 报 unknown_action）；`backend/internal/decision/chat.go:119`（schema enum）。
- 当前行为：王熙凤"理账""叫平儿倒茶"只能作为台词文本存在；"平儿"甚至不是参与者（红楼世界 people 里没有她，不在场、无 entity），倒茶无 event、无 scene 状态、无物品变化。
- 正确行为：决策应能提交**世界动作**（如 act/perform 类 action + activity_code），经校验后成为 committed event（类比已有的 `AgentActivityStarted`），进 projection。
- 根因：RP-1 的 action 词汇表按"对话响应"设计，后续阶段（RP-2 生活层、RP-3 职业）把活动做成了**日程驱动的确定性执行**，从未把"动作"开放给决策 provider。
- 修复层级：core 提案模型 + 提交校验 + 事件类型 + projection + 叙事 fact 类型，一条链配套扩展。

**P0-2　NPC 无自传记忆：决策输入在回合间是状态不变的**

- 证据：NPC 说话/移动的 listeners 均排除自己（`backend/internal/storage/rp_perception.go:88-89`、`rp_decision_commit.go:141,317-327`）；无任何代码把 NPC 自己的 `rp_npc_decisions/rp_utterances` 回灌为自己的 knowledge；`RPDecisionInput` 无历史字段。
- 当前行为：NPC 记得"玩家要求倒茶"（玩家的话是 speaker_said claim），**不记得"我已经倒过茶"**。每轮决策面对的输入几乎相同 → 重复请坐倒茶。**线上物证**（会话 rps_043ca865）：王熙凤在 01:00、05:00 的 initiative 对白（`event_rp_initiative_*`，见 A.3）逐字重放 00:00 的开场白——"等几小时回来重新开场"不是渲染层巧合，是决策输入在 5 世界小时后与开局几乎相同的信息论必然。
- 正确行为：NPC 自己的言行应成为自己的 knowledge claim（如 `own_action` 类型），进入 decision input。进一步：speech 事件与 activity 事件必须区分——**"我叫平儿倒茶"（speech）≠ "平儿已经倒好茶"（activity completed）**，前者永远是 claim 文本，后者才是世界事实（见 E 节最小连续性设计）。
- 根因：knowledge 写入只做了"听见别人"，没做"记住自己"。
- 修复层级：storage（hearings 写入扩展）+ decision input 组装（放开 claim 类型过滤 `rp_decision.go:164`）。

**P0-3　Studio 世界没有任何"生活"播种：无日程、无认识、无人设**

- 证据：studio 流程只写实体/地点/包（`storage/studio_genesis.go`、`storage/studio_spatial.go`），grep `agent_schedule_entries|scheduler_items|agent_knowledge|familiarity` 零命中；projection 对 `RPParticipantsInitialized` 硬编码 M2 特判（`storage/rp_identity_projection.go:84-95`）。
- 当前行为：红楼世界 NPC 无日程（NextSchedule 恒 nil）→ wait 的日程 drain 空转 → 世界在玩家离开时完全静止；全员陌生人且无法合法初始化；NPC"人设"只有每回合 20 条 knowledge + 场景快照，全靠模型即兴。
- 修复层级：Studio spec 扩展（acquaintances/personas/routines 声明字段）+ 投影泛化（去掉 M2 特判，按 payload 的 NPCEntityIDs 推导）+ persona 种子写入 agent_knowledge。

### P1 — Narrative/连续性

**P1-1　叙事事实集=对白集，正文长度存在结构上限**

- 证据：`RPNarrativeFact` 仅 6 种 action（`rp_turn_view.go:56-79`）；deterministic 模板每事实一行（`backend/internal/core/rp_style.go:230-246`）；full_prose 只能在这 6 种事实上改写（`backend/internal/narrative/prose.go:68-74`）。
- 正确行为：wait drain 产生的 `AgentActivityStarted/AgentMoved`、scene 内活动事件、环境变化应作为新 fact action 进入叙事输入——正文素材来自事实，长度自然上去。
- 修复层级：fact 类型扩展（与 P0-1 共用事件源）+ 渲染模板/prose prompt。

**P1-2　LLM 叙事 provider 不在结算主路径，full_prose 默认关闭且校验过严易回退**

- 证据：`rp_turn_view.go:25` 硬编码 deterministic；`ChatProseProvider` 仅经 `model.full_prose` override 启用（`backend/internal/transport/httpapi/rp_model.go:43-69`），校验要求每条对白逐字保留、任何引号 span 必须属于已提交对白（`prose.go:255-280`）。
- 修复层级：让 studio narrative 包直接可选 full_prose（声明式开关，同 dialogue_ratio 一层的维度）。

**P1-3　世界完全请求驱动，玩家是唯一的时钟发条**

- 证据：无后台 ticker（见 B3）；`runAgentLifeForScope` 只在 wait 里被调。
- 影响：即使上面都修好，玩家不点"等待"，世界依旧冻结。"继续剧情"就是对这一点的本能补偿——它是普通对白，却承担了"让世界走一步"的职能。
- 修复层级：产品决策——可把"继续剧情"前端映射为短 wait+initiative（已是 wait 路径的现成能力），而非发明新机制；真正的后台驱动属后续阶段。

### P2 — 体验与质量

- **时间戳每行重复**：deterministic 模板 verbosity=detailed 给每行加 `（world_time）`（`backend/internal/core/rp_style.go:286-288`），同刻对白每行都带。前端可按上一行同刻去重。
- **两个 NPC 同时回话**：机制上每个 listener 独立决策、无仲裁（`rp_turn.go:125-161`）。可用 speech act + 是否被直接询问做轻量仲裁，或交给模型自律（决策 prompt 层）。
- **入口页 pending 残留导致「选择其他世界」永久禁用且无提示**（`src/components/PlayWorkspace.vue:114,599`）。

---

## D. RP Roadmap 接入矩阵

| 能力 | 已实现 | 主路径已接入 | 有真实状态 | 有 committed event | Narrative 可见 | 结论 |
|---|---|---|---|---|---|---|
| NPC decision | ✅ | ✅（speak 回合 + wait initiative） | ✅ | ✅ RPSpeechAccepted 等 | ✅ 仅对白 | **接通，但词汇表只有对白** |
| NPC autonomous activity | ⚠️ 部分 | ⚠️ 仅 wait 后 initiative/warm | ⚠️ 红楼世界实测 5 小时 3 个效果（2 条 initiative 对白 + 1 silence），但对白是开场白逐字重放 | ✅ | ❌ 活动事件不进叙事事实 | **机制存在且会点火、输入贫瘠导致重播、产出不可见** |
| wait/time advance | ✅ | ✅ | ✅ world_clocks | ✅ RPWaitCompleted | ⚠️ 仅时间词 | 接通；无日程世界=空转 |
| clocks | ✅ | ✅ | ✅ | ✅ | ❌ | 有，只作时间戳 |
| obligations | ✅（rent/promise 推导） | ✅ 进 decision input | ❌ 红楼世界为空 | ✅（有表） | ❌ | 有代码无数据（无租约/约定） |
| background NPC | ✅ | ❌ 仅显式 HTTP 触发 | ❌ | ✅ RPBackgroundMaterialized | ❌ | 有代码，studio 世界未用 |
| multi-NPC scene | ✅ | ✅ 每 listener 一决策 | ✅ | ✅ | ✅ | 接通，无回话仲裁 |
| scene continuity | ❌ | — | ❌ 无 scene 表 | — | — | **未实现**（重复请坐倒茶的直接原因） |
| relationship | ⚠️ | ⚠️ knowledge 有 interpersonal claim 但 `rp_decision.go:164` 丢弃不进 input | ✅ | ✅ | ❌ | 半接通 |
| economy | ✅ | ✅ OwnAssetMinor 进决策 | ✅ 账户 | ✅ | ❌ | 接通但不对 RP 正文贡献 |
| world pressure | ⚠️ | ⚠️ wait 提交时确定性抽签机会 | ⚠️ | ✅ | ❌ | 有框架，initiative 动作词汇表接不住 |
| Narrative Renderer | ✅ | ✅（结算硬编码 deterministic） | — | 只读 | ✅ | **输入事实集=对白集是上限** |
| identity resolution | ✅ | ✅ | ✅ | ✅（存真实 ID） | ✅（陌生人脱敏） | 数据层完好；写入通道被 demo 硬编码锁死 |
| persona | ❌ | — | ❌ | — | — | **未实现**（除 M2 demo 的种子知识） |

一句话：**大部分"能力"存在且有 committed event，但它们的产出从不进入 RP 正文；RP 正文事实源只有玩家对白和 NPC 对白。**

---

## E. 修复方案（只列方案，不执行）

**组织原则：按验收链路排序，不做阶段数量承诺。** 验收链只有一条：**Studio 创建真实人物与日常 → Play 行动 → 等待推进 → 连续长篇叙事 → 重启后延续**。每一步的验收标准是行为性的（人物真的在生活），不是测试计数。

**最小场景连续性并入本轮，不留到以后。** 四个问题各有承担者，全部基于现有活动事件与可重建投影（允许新建纯事件重建的投影表，不允许独立可改的权威状态），**不另造第二事实源**：

| 连续性问题 | 承担者 | 机制 |
|---|---|---|
| 谁在场 | 已有 | `rpPerceivedEntityIDs` / `agent_presence` claims，只需接入决策输入与叙事事实 |
| 正在做什么 | 本轮新建 | `AgentActivityStarted` 无对应 Completed = 进行中活动，可重建投影进决策输入 |
| 什么已经完成 | 本轮新建 | activity completed 事件 + NPC `own_action` knowledge claim（E-2），completed 活动留在事实窗口 |
| 哪些结果仍有效 | 本轮新建（最小版） | 已完成活动作为持续事实存在，直到被后续 committed 事件覆盖（如茶被喝掉）；**不做物品级状态机** |

（注：「本轮新建」的承担者允许是纯事件重建的投影表——见 E-2 活动契约第 4 条。）

**「我叫平儿倒茶 ≠ 茶已倒好」由事件类型区分保证**：speech 事件（王熙凤的指令）永远只是 claim 文本；act 决策（平儿执行）产生 activity 事件才是世界事实。平儿倒茶的完整因果链：平儿是 studio spec 声明的参与者（在场）→ 听到指令（speaker_said claim）→ 自己的决策提交 act(serve_tea) → `AgentActivityStarted/Completed` committed → 活动结果进权威投影 → 王熙凤下轮决策输入里可见「茶已倒好」和她自己「我已吩咐过」。**若平儿没做，"茶已倒好"在世界上不存在——这正是连续性的含义。**

**活动契约（补入，防实现走样）**：

1. **历史与当前结果分开建模。**「平儿曾经倒过茶」是历史事实，committed 后永不被覆盖；「现在还有茶可喝」是当前结果，可被后续事件改变（茶被喝掉、被打翻）。两者分别由 activity 完成事件与结果投影承担，不得混在一个字段里。
2. **活动有完整生命周期，不得把"无完成事件"默认当永久进行中。**状态机至少区分：进行中 / 完成 / 取消 / 失败。取消与失败也必须由 committed 事件表达（如执行者离开场景使未完成的活动中止），不允许靠"一直没完成"隐式表达。
3. **耗时与完成条件由规则决定，不由模型宣告。**模型提交 act 只产生 `AgentActivityStarted`（规则校验 activity_code 合法性、执行者 capability、在场证据）；完成在 world_time 经过规则确定的耗时后由**确定性逻辑**提交 `AgentActivityCompleted`（在 turn/wait 推进时检查到期活动），模型无权直接宣告成功。校验不通过的 act 走 provider_fallback 回 silence。
4. **「禁止第二事实源」禁止的是独立可改的权威状态，不是事件重建投影。**若活动/结果状态适合用专门的投影表表达（完全由 committed events 重建、无独立写入通道），应当允许建表；不得为了不建表把结构性状态硬塞进 agent_knowledge 文本。
5. **可见性按观察者收窄（防修记忆时制造全知）**：活动结果进入权威投影；**执行者**获得自身行动的 own_action 记录；**其他角色**仅在具有合法感知（在场目击）或信息传递（被转述且入 knowledge）证据时获得相关知识；叙事仍按当前视角过滤（与 E-4「玩家可见的 committed facts」同一条纪律）。禁止全量广播进所有决策输入。

### E-1　Studio 创建真实人物与日常（对应 P0-3）

- spec 增加三个声明字段：`acquaintances`（初始认识对）、`personas`（人设，播种为 agent_knowledge claims）、`routines`（日常，生成 `agent_schedule_entries`——NPC 第一次有真实日程，wait drain 不再空转）。
- `rp_identity_projection.go:84` 的 M2 硬编码特判改为按事件 payload 通用推导（M2 demo 成为其一个实例），Studio 世界的认识初始化解锁。
- 验收：建出的世界里 NPC 有日程、互相认识、有人设种子；不依赖任何 demo 路径。

### E-2　Play 行动：动作词汇表 + 自传记忆（对应 P0-1 + P0-2）

- **act 类 action 进决策词汇表**：`RPDecisionProposal` 增加 act（带 `activity_code`/目标），走现有 `ValidateRPDecisionProposalEvidence` 模式校验（capability/场景约束：平儿不能替凤姐倒茶除非她在场且接令），commit 产生 `AgentActivityStarted/Completed`（复用 RP-2 生活层已有的事件类型，不新造）。
- **NPC 自传记忆**：NPC 自己的 speech/leave/act 同时给自己写 knowledge claim（新增 `own_action` 类型），decision input 放开 `rp_decision.go:164` 的类型过滤。不新建 memory 系统。
- **身份双向打通**：决策提案加 `introduce_self` 字段（文本声明不可信，置位才是事实，提交层校验），commit 写 listener→NPC familiarity；前端暴露玩家自我介绍开关。投影 replay 天然支持，零投影改动。
- **initiative 审计补口**：initiative 链同步写 `rp_npc_decisions`（或等价证据表），消除两条路径的审计不对称（B3 发现的缺口）。
- 验收：玩家说「平儿倒茶」后，平儿能执行、事件可查、王熙凤下轮输入里可见"茶已倒好"和她自己"我已吩咐过"；不会再出现 01:00/05:00 那种逐字重放开场白。

### E-3　等待推进：initiative 接上活动（对应 P1-3 的架构内解）

- initiative/warm 的 legal actions 接入 E-2 的 act 词汇表——NPC 等待期间做的是**日程里的日常**（E-1 的 routines 终于有了消费方），而不只是重开对话。
- 「继续剧情」前端映射为短 wait + initiative 流（复用现成链路，不发明新机制）。
- 世界后台驱动（真实 ticker）仍属后续 RP 阶段，但本轮结束后"玩家是唯一定时器"的痛感应已显著下降：等待看到的将是他人在过日子，不是重播。
- 验收：wait 数小时，回来的 narrative 里有他人完成的活动事实（谁做了什么），而不是同一句开场白。

### E-4　连续长篇叙事：事实源扩容（对应 P1-1 + P1-2）

- `readRPNarrativeInput` 事实窗口从"本回合对白"扩到"上次观察以来玩家可见的全部 committed facts"：E-2/E-3 产生的活动事件、移动、在场变化、日程 drain 结果——正文素材来自事实，长度是自然结果而非凑字。
- studio narrative 包增加 `full_prose` 声明式开关（与 dialogue_ratio 同层维度）；保持现有纪律：叙事只读、校验失败回退单行模板。
- **结算先返回确定性结果、再流式生成正文的双路径本身保留，不推翻。** 两条路径存在不是问题，问题是无从核对最终显示的是什么。因此：

**长篇正文单独验收（补入，不并入"事实扩容"一笔带过）**：

1. 真实 Play 页面上确实启用了长文 provider（不是只在测试里），且可在界面上确认当前生效的叙事模式；
2. 正文能**连续组织**已知环境、角色动作、对白与场景衔接（时间推进、空间转换有交代），而不是把事件逐条改成更长措辞的流水账；
3. **禁止两种假达标**：靠无关事件堆字数；把"每轮都必须长"当质量标准（无事实支撑的回合就该短）；
4. **回退必须可查证**：full_prose 校验失败回退单行模板时，回退原因（校验失败项、输入事实摘要）写入可查的位置（turn run 元数据或服务日志带 turn_run_id），不允许悄悄退回一句话；
5. 核对对象是**用户最终看到的正文**（结算 narrative + narrative/stream 流式渲染的落点），两条路径都要核对。

- 验收：一回合并不会话只产出"某某说"单行堆叠；等待后的正文能写出"这一两个时辰里，平儿沏了茶，熙凤理完了一册账"这类**由 committed 事实支撑**的长段；回退发生时能在 turn run 元数据里查到原因。

### E-5　重启后延续

- 事件溯源天然支持：重启 = 重放投影，世界状态、认识关系、活动结果全部从 committed events 恢复。本轮需验证的只有一个点：叙事事实窗口全部从 DB 重建（`readRPNarrativeInput` 已满足，属回归确认而非新开发），会话恢复走 `sessions/open` replay。
- 验收：服务器重启后进入同一世界，人物继续各自的活动，认识关系和已完成事项不丢。

### 明确留到后续阶段（建立在 E-2 事件词汇表之上，现在做会二次返工）

scene 物品级状态（茶凉了、账本在桌上——需要对象模型）、多 NPC 回话仲裁、世界后台 ticker、真正的 faction pressure。

红线：不建第二套独立可改的权威 world state/memory/economy；叙事永不成为事实源；状态连续性靠 committed event 与可重建投影（含允许新建纯事件重建的投影表），不靠 prompt。

---

## F. 最终判断

> **如果按照当前架构继续完成剩余 RP 阶段而不处理本次发现的问题，CoreRP 最终会变成"底层系统很多、但实际体验仍是一问一答的 Tavern-lite"吗？**

**基于代码证据：会。** 理由不是推测，是三个结构性上限：

1. **决策 schema 不扩展**，RP-3 及之后的职业/日程/经济只会让更多"活动"发生在日程 drain 里——那些活动（打卡、发薪、移动）从不进入叙事事实集（`readRPNarrativeInput` 只查 `rp_utterances`+`rp_npc_decisions`），正文素材永远只有对白。世界越丰富，正文越是对白夹流水账。
2. **NPC 自传记忆缺失不补**，任何日程都救不了重复——决策输入在回合间不变，模型只能给出与上一轮统计相同的回答。这是信息论问题，不是模型聪明与否的问题。**线上数据已从"推断"升级为"物证"**：01:00/05:00 两条 `event_rp_initiative_*` 逐字重放 00:00 开场白（A.3），initiative 引擎真实运行、真实提交、真实重播。
3. **身份/认识初始化被 demo 硬编码锁死**不泛化，则每个 studio 世界永远从"满屋陌生人"开始，角色扮演的代入感上限被焊死。

反过来也要公允地说：**这次审计证实了地基是真的**——事件溯源、committed event 唯一事实源、叙事只读、幂等提交、投影校验，这些纪律在 turn 主链路里被逐文件核实严格执行（initiative 链有一个真实的不对称缺口：不写 `rp_npc_decisions`，见 B3——这是要修的点，不是理念缺陷）。这正是 Tavern 没有的东西。当前的问题不是"架构理念错了"，而是**事件词汇表太窄 + 记忆单向 + 叙事事实源=对白源**三个具体的、可修的缺口。按 E 节的链路修，每一步的改善都是行为可见的，且全部复用现有机制，不需要第二套系统。

---

## 附录：审计方法与取证

- 四路并行代码追踪（回合主链路 / 等待与自主世界 / 叙事渲染层 / 身份与连续性）+ 关键文件逐行第一手复核（`rp_turn.go`、`rp_turn_view.go`、`rp_service.go`、`rp_wait.go`、`rp_style.go`、`rp_decision_commit.go`、`decision/chat.go`）。
- 线上取证：预览服务器 SQLite（`world.db`）实际事件统计——红楼世界 `world_honglou_60615b11` 会话 rps_043ca865：27×RPSpeechAccepted（= 11 玩家 + 14 回合决策 NPC respond + 2 wait-initiative NPC respond，逐条经 rp_utterances×rp_npc_decisions 映射核对）、2×RPWaitCompleted、1×RPNPCDecisionRecorded（initiative silence；respond 型 initiative 直接记为 RPSpeechAccepted）、0×RPNPCMoved、0×AgentActivityStarted。非 fixture。
- 二轮复核追加取证：`rp_npc_decisions` 全仓唯一 INSERT 点检索（仅 `rp_decision_commit.go:183`）；2 条 orphan utterance 的 event_id/turn_id 前缀（`event_rp_initiative_*` / `turn_rp_initiative_*`）与 `rp_initiative_commit.go:233,272` 的构造规则逐字吻合，提交路径确认。
- 「没有/不存在」类结论的检索范围：全 backend 非测试代码 grep（如 `RPNarrativeFact{` 构造点仅 2 处；`INSERT INTO rp_identity_familiarity` 仅 3 处；`INSERT INTO agent_schedule_entries/scheduler_items` 仅出现在 agent.go、agent_routine.go、rp_background.go、career_*、m2_economy.go）。
