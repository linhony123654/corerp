/**
 * CoreRP M0 契约数据模型（演示数据，非已实现功能）
 *
 * 字段命名遵循《CoreRP v0.3.1 · M0 工程审计版》：
 *  §3  核心对象 / Command / EventBatch / Observation
 *  §9  公共记录字段 record_id / trace_id / causation_id / actor_id / world_time …
 *  §22 经济对象 Currency / Account / JournalEntry / MarketQuote / EmploymentContract
 *  §24.2 RuleEpoch / Branch / WorldDefinitionVersion
 */

export type RecordType =
  | 'event'
  | 'agent_decision'
  | 'rule_validation'
  | 'observation'
  | 'runtime_diagnostic'
  | 'intervention'

export type Authority = 'authoritative' | 'audit' | 'non-authoritative'

/** 演示层仍使用 JS number，但所有最小单位值必须通过 Number.isSafeInteger。 */
export type MinorUnits = number

export interface Principal {
  principal_id: string
  principal_type: 'player' | 'creator' | 'operator' | 'service' | 'agent'
}

export interface AccessScope {
  instance_id: string
  branch_id: string
  subject_ids: string[]
  fields: string[]
}

export interface CapabilityGrant {
  capability_id: string
  policy_version: string
  scope: AccessScope
}

export interface BaseRecord {
  record_id: string
  record_type: RecordType
  instance_id: string
  branch_id: string
  trace_id: string
  causation_id: string | null
  actor_id: string
  world_time: string
  recorded_at: string
  module: string
  schema_version: string
  authority: Authority
  rule_epoch: number
  ruleset_hash: string
  /** Inspector 审计脊梁的稳定顺序；不推进分支版本。 */
  record_order: number
  /** 只有权威事件占用事件序号；审计/观察/诊断记录必须为 null。 */
  event_sequence: number | null
}

export interface Posting {
  posting_id: string
  entry_id: string
  account_id: string
  currency_id: string
  /** 借方为正、贷方为负；同一 entry 按币种合计必须为 0。 */
  amount_minor: MinorUnits
}

/** 实物数量流转：与资金分录分离，不同单位不得相加 */
export interface StockMovement {
  movement_id: string
  sku_id: string
  quantity_minor: MinorUnits
  base_unit: string
  from_holder_id: string
  to_holder_id: string
  movement_kind: 'transfer' | 'create' | 'consume' | 'destroy'
}

export interface WorldEvent extends BaseRecord {
  record_type: 'event'
  event_sequence: number
  event_type: string
  payload: Record<string, string | number | boolean>
  /** Event 下展示的是规范化领域事实的只读投影；真实写入必须走 Commit。 */
  postings?: Posting[]
  /** 实物数量流转，与资金分录分开；不同计量单位不得相加 */
  stock_movements?: StockMovement[]
  journal_status?: 'posted'
  idempotency_key?: string
}

export interface RejectedCandidate {
  candidate: string
  stage: string
  reason: string
  code: string
}

export interface AgentDecision extends BaseRecord {
  record_type: 'agent_decision'
  goal: string
  knowledge_refs: string[]
  candidates: string[]
  selected: string
  selected_reason: string
  rejected: RejectedCandidate[]
}

export interface RuleValidation extends BaseRecord {
  record_type: 'rule_validation'
  rule_id: string
  rule_epoch: number
  subject: string
  eligible: boolean
  checks: { check: string; passed: boolean; detail: string }[]
  roll?: { dice: string; result: number; dc: number }
}

export interface ObservationRecord extends BaseRecord {
  record_type: 'observation'
  observer_id: string
  channel: string
  perceived: string
  /** 演示观察者列表；实际查询还必须经过 audience_scope。 */
  visible_to: string[]
  audience_scope: AccessScope
}

export interface RuntimeDiagnostic extends BaseRecord {
  record_type: 'runtime_diagnostic'
  severity: 'info' | 'warn' | 'error'
  message: string
  cost: { model_calls: number; tokens: number; ms: number }
}

export interface InterventionRecord extends BaseRecord {
  record_type: 'intervention'
  privilege_type: string
  authorized_by: string
  policy_version: string
  scope: string
  reason: string
  affected: string[]
  original_condition: string
  result_events: string[]
}

export type WorldRecord =
  | WorldEvent
  | AgentDecision
  | RuleValidation
  | ObservationRecord
  | RuntimeDiagnostic
  | InterventionRecord

/* ---------------------------------------------------------------- 实体 */

export interface Entity {
  id: string
  name: string
  archetype: 'person' | 'organization' | 'store' | 'cohort'
  role: string
  cash_minor: MinorUnits
  /** 净资产 = 资产估值 − 负债 */
  net_worth_minor: MinorUnits
  /** 流动性 = 可用现金与短期可变现资产 */
  liquidity_minor: MinorUnits
  /** 社会地位是多主体主观评价，不可由工资单一推导 */
  status: { observer: string; score: number; basis: string }[]
}

export interface MarketQuote {
  quote_id: string
  sku: string
  seller: string
  region: string
  price_minor: MinorUnits
  currency_id: string
  base_unit: string
  valid_from: string
  valid_until: string
  tax_included: boolean
  /** 标价 / 含税价 / 实际成交价 必须区分 */
  kind: 'list' | 'tax_included' | 'transacted'
}

export interface EmploymentContract {
  contract_id: string
  employee: string
  employer: string
  position: string
  gross_wage_minor: MinorUnits
  pay_period: string
  currency: string
  due_at: string
  arrears_minor: MinorUnits
}

/* ------------------------------------------------------------ 世界状态 */

export interface RuleEpoch {
  epoch: number
  epoch_id: string
  ruleset_hash: string
  dlc_lock: string
  activated_at_world_time: string
  activated_by_event: string
  /** 半开区间 [start_sequence, end_sequence_exclusive)；当前纪元结束值为 null。 */
  start_sequence: number
  end_sequence_exclusive: number | null
  packs: string[]
}

export interface Branch {
  branch_id: string
  label: string
  head_sequence: number
  forked_from: string | null
  forked_at_sequence: number | null
  epochs: RuleEpoch[]
}

export interface Pack {
  id: string
  kind: 'world' | 'system' | 'content' | 'narrative' | 'adapter'
  version: string
  engine_api: string
  state: 'installed' | 'enabled' | 'disabled'
  capabilities: string[]
  schema_hash: string
  content_hash: string
}

export interface WorldSnapshot {
  instance_id: string
  branch_id: string
  branch_head_sequence: number
  current_epoch_id: string
  world_time: string
  uptime_days: number
  /** 同步投影：只可由 Commit 更新，可从权威 Event/领域事实重建。 */
  projections: {
    accounts: { owner: string; balance_minor: MinorUnits; type: string; overdraft_limit_minor: MinorUnits }[]
    inventory: { sku: string; seller: string; quantity_minor: MinorUnits; base_unit: string }[]
  }
  /** 调度游标：离线批结算按因果顺序推进 */
  scheduler_cursor: string
  outbox_pending: number
}

/* ---------------------------------------------------------- 视图用结构 */

export interface SpineNode {
  record: WorldRecord
  children: SpineNode[]
  depth: number
}

export interface KnowledgeEdge {
  observer: string
  subject: string
  channel: string
  confidence: number
  evidence_refs: string[]
  /** 信念可错误：知识不等于 World Truth */
  believed: string
  truth: string | null
  accurate: boolean
}

export interface WhyNotItem {
  question: string
  /** 单靠事件账本无法回答"为什么没发生"，需要候选拒绝与调度跳过记录 */
  answer: string
  evidence_refs: string[]
  severity: 'info' | 'warn' | 'error'
}

export interface EntityLedgerRow {
  account: string
  balance_minor: MinorUnits
  type: string
  overdraft_limit_minor: MinorUnits
}

export interface EconomyPoint {
  day: number
  /** 名义口径，非实际口径 */
  household_cash: number
  store_cash: number
  firm_cash: number
  net_worth_household: number
  price_bread: number
}

/* ------------------------------------------------- 叙事流（玩家视角） */

/** 消息来源：叙事历史与世界事件账本（WorldEvent）严格分开 */
export type MessageSource = 'world' | 'player' | 'system'

/** 展示模式：长文卷轴 / 酒馆对话。两者只读同一条历史，不生成第二套消息 */
export type StoryViewMode = 'scroll' | 'tavern'

/**
 * 叙事消息。message_id 稳定，可关联可见事实引用；
 * 草稿与滚动锚点是 UI 状态，不是世界事件。
 */
export interface NarrativeMessage {
  message_id: string
  /** 演示历史内的序号，与 WorldRecord.record_order / event_sequence 无关 */
  turn: number
  source: MessageSource
  /** 世界时间（demo snapshot 口径），不是浏览器时间 */
  world_time: string
  scene_id: string
  /** 可控 Markdown 子集 */
  body: string
  /** 来源可辨识的可见事实引用，例如 rec_0017 / mq_bread_002 */
  fact_refs: string[]
  /** 演示标记：true 表示此内容为构造演示，不是内核输出 */
  demo: boolean
  /** 未读标记（跳最新后清除） */
  unread?: boolean
}

/** 行动意图：自由文字与快捷点击走同一条通道 */
export interface ActionIntent {
  kind: 'observe' | 'talk' | 'inspect_goods' | 'wait' | 'travel' | 'open_inspector' | 'help' | 'unsupported'
  label: string
  actor_id: string | null
  target_id: string | null
  params: Record<string, string | number | boolean>
  source: 'click' | 'text'
  /** 原始输入（点击时为按钮标签） */
  raw: string
  /** 演示标记：本意图只产生前端演示结果 */
  demo: true
}
