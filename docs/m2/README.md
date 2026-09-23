# CoreRP M2 后端实现记录

M2 从 T09 Cohort 升格/降级守恒开始，随后增加双 Agent 的日程—观察—知识—遭遇纵切和可选 30 日无人运行的空间日程。它们均有后端证据；两名 Agent 移动 30 日不等于完成 100 人/10 店的经济、LOD 或主观地位验收。

## T09 最小纵切

- [Migration 006](schema-006-cohort-materialization.sql) 增加 Cohort 聚合投影、具名实体投影、不可变分配谱系和人口权威流转。
- 资产、应收、负债继续使用平衡 Posting；库存继续使用 Stock Movement；人口使用 Population Movement。不存在第二套经济账本。到期工资合同的债权划分由[Migration 014](schema-014-claim-allocations.sql)记录逐笔分配/返还谱系；[Migration 016](schema-016-wage-participation.sql)记录下一期具名工资参与和原始债权切片；[Migration 017](schema-017-wage-allocation-policy.sql)至[019](schema-019-bankruptcy-slot-claims.sql)记录部分付款规则、T09 工位债权归属变化和破产开立债权人快照。T09、应收及人口流转同批提交。
- 每次升格记录唯一 `materialization_id`、稳定 `entity_id`、分配算法版本、人口与四类经济分配，并在一个 Event Batch 内提交。
- 相同物化 ID 与相同规范负载返回原结果；负载不同明确冲突。
- 新升格/降级命令必须晚于已提交的分支事件与世界时钟，且不能越过尚未结算的到期调度项；历史幂等重试仍返回原结果。世界时间比较按 RFC 3339 表示的时刻进行，不依赖字符串时区排序。
- 降级仅可返还原始分配，或经工资收据与未付债权工位证明的工资现金/应收；无来源的其他个人资产不能吸回 Cohort。
- M2 演示 fixture 使用独立实例/分支，避免改变 M1 genesis、测试计数和历史哈希。
- [T09 可执行证据](test-evidence.json) 独立于历史 M0 `not_run` 定义，避免反写冻结基线。

## 双 Agent 最小纵切

- [Migration 007](schema-007-agent-life.sql) 增加分支作用域的 Agent 档案、地点、一次性日程、位置投影、不可变移动事实、观察证据与有限知识投影。
- Ada 与 Bo 都先通过 T09 从 Cohort 原子升格，Agent setup 不直接生成第二份人口、资产或个人历史。
- M2 执行器复用 `scheduler_items` 的稳定 `(world_time, phase_id, declared_priority, scheduler_item_id)` 顺序，但不改写已冻结验证的 M1 90 日执行器。
- 每个移动项独立提交 Event Batch、移动事实、位置、时钟、调度状态、审计与 Outbox；预算耗尽后可关闭数据库并从检查点继续。
- 只有实际同地点的 Agent 才产生 `observation_records` 与对应知识；知识读受 Principal/Capability/instance/branch/observer/fields 约束，不存在跨 Agent 全知读取。
- Encounter 是对当前已提交位置和移动证据的只读解析：允许返回空结果，不创建随机事故、不写事件、不生成叙事文本。
- Agent 位置和知识已纳入空账本重放、快照续放、投影差异检测与修复；旧快照在无 Agent 字段时保持原规范 JSON 形状。

## 30 日空间日程与可选运行进程

- 先执行显式 `agents30-prepare`：它保留首日四项日程，并通过 `AgentRoutineDefined` Event Batch、审计和 Outbox 原子声明后续 29 日的 116 项稳定日程。重复定义不重复写入，迟于首个新日程才首次定义会拒绝。
- `agents30` 可单进程一次性跑完；也可运行独立的 `agents30-drive`，通过 `-interval` 设置服务器时间的执行间隔、通过 `-batch` 限制每次处理的世界动作数。每个动作仍是独立提交，可随时停止并从数据库恢复。这个间隔只控制推进频率，不参与世界时间或调度排序。
- 首次定义在序号 5 后，30 日共 120 次移动，最终序号 125、世界时间 `2026-10-22T12:00:00Z`；首日先跑完再定义，最终序号同样为 125。观察和知识仍只来自同地点的提交事件，不会因为日程重复而造出新的资产、库存或人口。

本地初始化及完整往返：

```bash
cd /home/ubuntu/corerp-console/backend
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2.db -action bootstrap
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2.db -action roundtrip
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2-agents.db -action agents
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2-agents30.db -action agents30-prepare
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2-agents30.db -action agents30-drive -interval 1s -batch 4
```

若需通过 HTTP 操作，T09 先执行 `bootstrap`；Agent 路径先对独立数据库执行 `agents` 或 `agents30-prepare`，再用同一个 `-db` 启动 `corerp-server`。授权的 `POST /api/v1/commands/define-agent-routine` 可在首个新增日程到期前显式定义 30 日行动。普通服务器启动不会隐式添加 M2 demo 数据或后台推进。

## 明确延期

已落地的[背景经济与欠款纵切](background-economy.md)覆盖迁移 008–019；[工资债权一致性记录](wage-claim-consistency.md)列出 G1–G5/I1–I8 约束与证据。18 人工资、房租、有限商店/供应商库存、购买与消费按世界时间逐项结算，原始义务只计提一次。第 7 日的 180 工资可先付 120，再由真实维修服务收入补发 60；有具名参与时，累积逐工位策略按实际现金分配，零现金成为可追溯欠薪而非账本异常。T09 可以在已计提或已部分付款后转移一个未付工位及等额应收，降级时只返还有凭据的工资现金与剩余债权，再升格不改写原始切片。第 30 日的虚构破产开立保存 Cohort 与具名债权人的不可变债权快照，并提供受创建者权限约束的存储查询；若含具名分拆债权，第 31 日分配明确延期，绝不套用旧 Cohort 付款路径。无具名拆分时，原有 23 笔合计 4,140 欠薪快照、到期后 T09 债权分配和第 31 日 18 单位房东出资/分配路径仍保持。通用清算/免责、任意多次破产后付款、完整经济系统与法定债权顺序均未实现。可在独立数据库上执行：

```bash
cd /home/ubuntu/corerp-console/backend
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2-economy.db -action economy-day1
/usr/local/go/bin/go run ./cmd/corerp-m2 -db /tmp/corerp-m2-economy.db -action economy30
```

重复运行不重复扣款，`economy30` 也可独立在新数据库上启动；普通服务器启动不会自动创建这套演示经济。混合运行的预算单位是**调度项**（经济结算与 Agent 移动均计入），并非只有移动。M2 调度器同时拒绝跳过未知的更早任务、或在已提交的世界时间之前追溯执行。

仍延期：生产常驻调度/多进程租约、更多 Agent 动作/关系/行动预算决策、错误 Belief 与语义记忆、L0↔L2 升降级策略、完整背景 LOD 经济结算、主观地位、复杂供需以及 100 人/10 店/30 天规模门。本切片不调用 LLM。Rule Epoch 热激活仍属于 M3，Inspector 产品 UI 仍属于 M4。
