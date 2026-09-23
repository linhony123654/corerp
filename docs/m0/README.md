# CoreRP M0 契约包

> 状态：评审草案，2026-09-22。它冻结候选协议；独立的 [M1 最小内核](../../backend/README.md) 已验证一个采购事务纵切，但不表示整份 RFC 已获批准或全部 M1 行为已经实现。

本目录把《CoreRP 最终架构蓝图 v0.3.1 · 精读修订版》中已经收敛的 P0 语义整理为可检查的工程输入。M0 的产物是契约、Schema、示例和测试向量；第一个可执行世界内核仍属于 M1。

## 阅读顺序

1. [核心契约 RFC](core-contract-rfc.md)：权威性、事务、时序、权限、重放与错误语义。
2. [SQLite DDL](schema.sql)：可执行的初期 Schema 草案及数据库内可执行约束。
3. [JSON Schema](core-contract.schema.json)：Command、Event Batch、Manifest 与权限对象的交换格式。
4. [测试向量](test-vectors.json)：T01–T12 的机器可读输入、断点和预期不变量。
5. [青玄界示例](examples/qingxuan/manifest.json)与[现代街区示例](examples/metro/manifest.json)：同一协议下的两个最小 World Pack。

## 决策状态

| 状态 | 内容 |
|---|---|
| 冻结候选 | Event Batch 权威提交根、不可变领域事实、可重建投影、分支头并发版本、事件序号、Rule Epoch 半开区间、稳定调度键、幂等作用域、Outbox、默认禁止透支、Principal/Capability/Scope |
| 有条件冻结 | 外部模型 attempt 的复用/重规划策略、具体角色能力矩阵、各世界的 phase 注册值、发行机构和额度 |
| 待评审 | 本 RFC 的字段命名、错误码全集、规范 JSON 的跨语言实现与测试向量 |
| 延后 | M3 热激活执行、任意运行时代码 DLC、大规模近似结算、抽象外部市场、完整 StatusPerception |

## 不在本包内

- 本目录不承载运行时；Go/SQLite 实现位于独立的 `backend/` 模块。
- 不把前端 fixture 当成真实事件、性能数据或恢复证据。
- 不批准具体世界的法律、货币发行制度或创作者权限上限。
- 不声称 90 日经济切片、法则热升级或跨数据库迁移已运行。

## 本地验证

```bash
npm run verify:m0
npm run build
cd backend && /usr/local/go/bin/go test ./...
```

`verify:m0` 只检查规范文件和前端 fixture 的静态不变量。SQLite DDL 仍可通过 `sqlite3 :memory: < docs/m0/schema.sql` 或等价驱动独立执行。M1 Go 测试另外检查嵌入迁移与本文件夹 DDL 字节一致，并覆盖采购事务的故障注入、全回滚和关闭重开恢复；这不等于 T01–T12 已全部实现。
