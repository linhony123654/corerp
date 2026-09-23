# CoreRP M1 实现记录

本目录记录 M1 运行时在冻结 M0 契约之上的增量迁移与可执行证据。M0 的 `docs/m0/schema.sql` 继续作为不可改写的 migration 001；M1 通过后续 migration 演进，不把新实现反写成历史基线。

## Recovery foundation

- [Migration 002](schema-002-recovery.sql) 为经哈希校验的快照增加数据库内 payload 存储。
- [Migration 003](schema-003-strict-world.sql) 增加世界时钟、有限 phase 注册、经济主体、工资/房租合同与义务、报价、家庭预算和模拟运行检查点。
- [Migration 004](schema-004-obligation-accounting.sql) 增加工资/房租的费用、应付、应收、收入科目映射，以及每次支付尝试的不可变结算证据。
- [Migration 005](schema-005-authorization-issuance.sql) 增加 Principal、实例/分支/主体/字段 Scope、发行政策累计额度和 InterventionRecord。
- Demo 世界的期初资金和库存由 sequence 1 的 `WorldInitialized` Event、平衡 Posting 和 `create` Stock Movement 表达，不再只有无法重放的投影种子。
- M1 普通重放折叠已提交 Posting 与 Stock Movement；M2 migration 006 另增加 Population Movement。重放不重新执行规则、随机或外部模型。
- Outbox 发布允许至少一次重送；消费者必须按 `outbox_id` 去重。
- [M1 测试证据映射](test-evidence.json) 不改写历史 M0 向量，逐项关联当前 Go 集成测试；T10 后端 SSE 已通过，T09 在 M1 记录中仍保持阶段延期并由独立 [M2 证据](../m2/test-evidence.json) 接续。

严格 90 日时钟、工资/房租义务、预算消费、带来源补货、创作者受控发行、私密查询及后端 HTTP/SSE 均已由集成测试覆盖。仍未完成的是前端接线、生产身份/运维、LLM/Agent、运行时法则热激活与跨币种金融。
