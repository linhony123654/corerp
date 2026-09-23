# M2 工资债权一致性闭环（本地演示范围）

本记录只覆盖 `inst_m2_t09/br_main` 的 18 人固定工资合同，不是通用劳动法、破产法或清算优先级。原雇主工资义务仍按合同/期间唯一形成；T09 不新增义务，也不推算未被事件证明的个人工作史。

## 实施边界

- [016](schema-016-wage-participation.sql)保存计提时的 Cohort/具名实体来源切片、参与生效时刻和旧格式收款凭据，任何后续 T09 都不改写它们。
- [017](schema-017-wage-allocation-policy.sql)保存合同级 `worker_round_robin_v1`：累计每一货币最小单位依次分配到 18 个工位；当期剩余 Cohort 工位排前，具名实体按稳定 ID 排序。多次付款用两次累计分配之差确定每次实际收款；零资金形成欠薪，原义务保持不变。
- [018](schema-018-wage-claim-ownership.sql)保存未付工位的当前归属变化、参与返还和新格式逐工位收款。升格时每笔未清工资只转出一个当时 Cohort 持有且尚未收到的工位，转出应收严格等于剩余值。降级返还有工资收据证明的现金及当前未付工位；已付部分只保留历史，不重新生成债权。以后付款按照当前归属入账。
- [019](schema-019-bankruptcy-slot-claims.sql)在雇主破产开立同批次冻结未付工位的来源、当时债权人、已付和未付金额。无拆分时原 013 Cohort 债权快照、014 到期后分配和 015 遗产支付路径不变；有拆分债权时，旧遗产路由只写入 `slot_claim_liquidation_deferred`、零付款。开立后普通工资重试仍拒绝。
- `Store.ReadM2WageClaimStatus` 是创建者授权的存储层只读查询，返回当前工位余额和破产开立时的债权人快照。它不是对外 HTTP/玩家查询授权。

## 不变量与验证位置

| 不变量 | 约束与证据 |
| --- | --- |
| I1 单一义务 | `m2_economic_obligations` 合同/期间唯一键；追偿只更新原 `obligation_id`；拆分/转让均不插入新义务。 |
| I2 到期=已付+未付 | 累计逐工位分配、事件付款事实、收据总和、义务余额和开立快照交叉核验；纯算法测试覆盖累计 0–180。 |
| I3 升格不造价值 | T09 应收与逐义务转出工位的历史剩余值完全一致；Journal 平衡、Branch Head CAS；伪造额外归属事件被拒绝。 |
| I4 降级不删历史 | 016 切片/收据和 018 归属/收据不可变；返还只新增转移事件和带来源的现金/应收，重试不重复提交。 |
| I5 已形成份额不回写 | 016、018、019 的不可更新/删除触发器；旧来源与当前归属分别展示。 |
| I6 不倒插历史 | `ensureCohortTransitionChronology` 校验已提交时钟、事件和待处理调度边界；07:10 同时刻往返测试保留 07:11 边界。 |
| I7 失败/并发不重付 | `BEGIN IMMEDIATE`、幂等键/调度项稳定 ID、Branch Head CAS、回滚和双连接并发竞争；收据重复/孤儿验证。 |
| I8 不足额不造币 | 工资只从已过账雇主现金支付；不足额落 `partial`/`overdue` 和原义务欠薪，零付款无 Posting；测试资金桥接来自平衡的房东→雇主账本转移。 |

## 关键自动化案例

`TestM2ActiveWageParticipationAccruesAndPaysNamedWorker`、`TestM2SplitWageZeroAndMultipleLatePayments`、`TestM2SplitWageArrearsCurePreservesClaimantsAfterRestart`、`TestM2T09AfterUnsplitAccrualTransfersOnlyUnpaidClaim`、`TestM2LiveWageClaimTransfersAfterAccrualAndPaysCurrentOwner`、`TestM2OriginalNamedWorkerReturnPreservesEarnedCashAndUnpaidSlot`、`TestM2WageClaimReturnAfterPartialAndRematerialize`、`TestM2ConcurrentLiveClaimTransferCommitsOnce`、`TestM2ForgedExtraClaimTransitionCannotCreateAnotherCreditor`、`TestM2SplitBankruptcyFreezesNamedCreditorsAndDefersDistribution` 和 `TestM2WagePolicyMigrationUpgradesExistingSplitLineage`。后者从 016 边界重开数据库并继续付款；往返测试比较空重放/快照续放并在关闭重开后查询债权与付款。

2026-09-23 本地验证：`/usr/local/go/bin/go test ./... -count=1` 全包通过；`/usr/local/go/bin/go vet ./...` 通过；相关 M2 工资/T09/破产与旧式遗产路由的 `go test -race` 通过，破产后降级快照测试的最终定向 race 也通过；017–019 嵌入迁移与文档 SQL 逐字节一致。未发布、未部署。

## Deferred（不属于此闭环）

- 具名/混合债权的破产后清算、债权顺位、分配、免责及任意频率的后续付款；当前不误付，已冻结状态等待未来明确规则。
- 生产级或玩家自助债权查询授权/API、隐私策略与通知；当前仅创建者作用域的存储读。
- 任意合同、不同工资率/人数/币种的通用化，完整月内劳动合同重组，法域规则和 M2 100 人/10 店规模门。
- RP-0 仅作下一阶段只读差距核对，本闭环不实施 RP、银行、市场或 UI 功能。
