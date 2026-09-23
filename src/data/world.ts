/**
 * 演示世界：Meta-1「青玄界 - 近郊村落 + 一间铁匠铺」
 * 对应 §11.1 参考场景与 §28 M1 垂直切片（1 企业 / 3 员工 / 1 房东 / 1 商店 / 90 世界日）。
 *
 * 本文件是**演示数据**，不是已实现的运行结果。
 * 所有 record_id / trace_id / causation_id / ruleset_hash 均为构造值，
 * 用于展示 M0 契约的字段结构与审计链路，不得据此声称任何代码已运行。
 */

import type {
  Branch,
  EconomyPoint,
  Entity,
  KnowledgeEdge,
  MarketQuote,
  Pack,
  RuleEpoch,
  SpineNode,
  WhyNotItem,
  WorldRecord,
  WorldSnapshot
} from '../types'

export const META = {
  version: 'v0.3.1',
  doc: 'CoreRP · M0 工程审计版',
  disclaimer: '本页为 M0 契约可视化演示：数据结构与审计链路为构造示例，不代表内核已实现或测试已运行。'
} as const

/** 构造 fixture 的规范哈希格式；不表示真实规则包已构建。 */
export const RULESET_HASH_E0 = 'sha256:f7283ea2a25010ef9f6d1808644414e354ee54fe71f070c8f328eee197375abe'
export const RULESET_HASH_E1 = 'sha256:93edcd4187d7fe3a74e8aab309745c7eba8af31ff90418a857235ee07cfb0009'

/* ------------------------------------------------------------ 实体 */

export const entities: Entity[] = [
  {
    id: 'npc_lin_qiao',
    name: '林巧',
    archetype: 'person',
    role: '铁匠铺帮工 · L1 具名人物',
    cash_minor: 3120,
    net_worth_minor: 8940,
    liquidity_minor: 3210,
    status: [
      { observer: 'npc_zhao_wan', score: 0.62, basis: '工钱稳定、手艺受称赞' },
      { observer: 'npc_he_tu', score: 0.48, basis: '见过她穿新夹袄' }
    ]
  },
  {
    id: 'npc_he_tu',
    name: '何图',
    archetype: 'person',
    role: '铁匠铺掌柜 · L2 活跃 Agent',
    cash_minor: 52400,
    net_worth_minor: 118600,
    liquidity_minor: 53100,
    status: [
      { observer: 'npc_lin_qiao', score: 0.78, basis: '掌铺多年、口碑在镇上传开' },
      { observer: 'npc_zhao_wan', score: 0.71, basis: '听说他盘下了镇东旧铺' }
    ]
  },
  {
    id: 'npc_zhao_wan',
    name: '赵婉',
    archetype: 'person',
    role: '房东 · 持有两份租约',
    cash_minor: 28900,
    net_worth_minor: 342000,
    liquidity_minor: 27400,
    status: [
      { observer: 'npc_he_tu', score: 0.66, basis: '镇上三处房产的主人' },
      { observer: 'npc_lin_qiao', score: 0.83, basis: '催租严厉但说话算数' }
    ]
  },
  {
    id: 'org_qingxuan_hall',
    name: '青玄宗外门',
    archetype: 'organization',
    role: '宗门 · L3 高影响 Agent',
    cash_minor: 1_284_000,
    net_worth_minor: 4_920_000,
    liquidity_minor: 611_000,
    status: [{ observer: 'npc_he_tu', score: 0.94, basis: '外门执事亲自来镇中采买' }]
  },
  {
    id: 'store_yishan',
    name: '铁匠铺 · 一山号',
    archetype: 'store',
    role: '商铺 · 有限库存 + 补货来源',
    cash_minor: 18740,
    net_worth_minor: 96_300,
    liquidity_minor: 18_900,
    status: []
  },
  {
    id: 'cohort_villagers',
    name: '村民（统计群体）',
    archetype: 'cohort',
    role: 'L0 统计人口 · 聚合结算',
    cash_minor: 214_500,
    net_worth_minor: 0,
    liquidity_minor: 0,
    status: []
  }
]

export const marketQuotes: MarketQuote[] = [
  {
    quote_id: 'mq_bread_001',
    sku: '粗麦面包 / 个',
    seller: 'store_yishan',
    region: '青玄界 · 近郊镇',
    price_minor: 12,
    currency_id: 'wen',
    base_unit: '个',
    valid_from: '2026-03-02T06:00',
    valid_until: '2026-03-09T06:00',
    tax_included: false,
    kind: 'list'
  },
  {
    quote_id: 'mq_bread_002',
    sku: '粗麦面包 / 个',
    seller: 'store_yishan',
    region: '青玄界 · 近郊镇',
    price_minor: 13,
    currency_id: 'wen',
    base_unit: '个',
    valid_from: '2026-03-09T06:00',
    valid_until: '2026-03-16T06:00',
    tax_included: false,
    kind: 'list'
  },
  {
    quote_id: 'mq_plow_001',
    sku: '精铁犁头 / 件',
    seller: 'store_yishan',
    region: '青玄界 · 近郊镇',
    price_minor: 3800,
    currency_id: 'wen',
    base_unit: '件',
    valid_from: '2026-03-01T08:00',
    valid_until: '2026-04-01T08:00',
    tax_included: true,
    kind: 'tax_included'
  },
  {
    quote_id: 'mq_plow_002',
    sku: '精铁犁头 / 件',
    seller: 'store_yishan',
    region: '青玄界 · 近郊镇',
    price_minor: 3550,
    currency_id: 'wen',
    base_unit: '件',
    valid_from: '2026-03-18T08:00',
    valid_until: '2026-04-18T08:00',
    tax_included: true,
    kind: 'tax_included'
  }
]

/* -------------------------------------------------------- 记录脊梁 */

const C = {
  rec: (n: number) => `rec_${String(n).padStart(4, '0')}`
}

const audienceScope = (...subject_ids: string[]) => ({
  instance_id: 'inst_qingxuan_near',
  branch_id: 'br_main',
  subject_ids,
  fields: ['observation.perceived']
})

export const records: WorldRecord[] = [
  /* ---------------- 世界日 2026-03-02 06:10 · 起点 ---------------- */
  {
    record_id: C.rec(1),
    record_type: 'event',
    event_type: 'WorldInstantiated',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_genesis',
    causation_id: null,
    actor_id: 'system',
    world_time: '2026-03-02T06:10',
    recorded_at: '2026-03-02T06:10:00.412',
    module: 'World Registry',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 1,
    event_sequence: 1,
    payload: {
      world_definition_version: 'wd_qingxuan_v3',
      population_households: 41,
      initial_currency: 'wen',
      currency_scale: '1',
      note: '创世生成结果经验证后固化为世界定义'
    }
  } as WorldRecord,

  {
    record_id: C.rec(2),
    record_type: 'rule_validation',
    rule_id: 'rule.employment.contract.open',
    rule_epoch: 0,
    subject: 'npc_lin_qiao',
    eligible: true,
    checks: [
      { check: '年龄 ≥ 14', passed: true, detail: 'world_age 19' },
      { check: '岗位存在空缺', passed: true, detail: 'position blacksmith_assistant vacant' },
      { check: '雇佣方授权', passed: true, detail: 'org_qingxuan_hall grant_authority=true' }
    ],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_genesis',
    causation_id: C.rec(1),
    actor_id: 'rule_engine',
    world_time: '2026-03-02T06:20',
    recorded_at: '2026-03-02T06:20:01.010',
    module: 'Rule Engine',
    schema_version: '0.3.1',
    authority: 'audit',
    ruleset_hash: RULESET_HASH_E0,
    record_order: 2,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(3),
    record_type: 'event',
    event_type: 'EmploymentContractSigned',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_genesis',
    causation_id: C.rec(2),
    actor_id: 'org_qingxuan_hall',
    world_time: '2026-03-02T06:25',
    recorded_at: '2026-03-02T06:25:00.778',
    module: 'Institution',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 3,
    event_sequence: 2,
    payload: {
      contract_id: 'ec_lin_001',
      employee: 'npc_lin_qiao',
      employer: 'org_qingxuan_hall',
      position: 'blacksmith_assistant',
      gross_wage: 1800,
      pay_period: 'monthly',
      currency: 'wen',
      due_at: '2026-03-31T20:00'
    },
    idempotency_key: 'idem_genesis_ec_lin_001'
  } as WorldRecord,

  /* ---------------- 2026-03-02 07:05 · 第一次发工资前的市场报价 ---------------- */
  {
    record_id: C.rec(4),
    record_type: 'event',
    event_type: 'MarketQuotePublished',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_pricing_001',
    causation_id: C.rec(1),
    actor_id: 'store_yishan',
    world_time: '2026-03-02T07:05',
    recorded_at: '2026-03-02T07:05:12.233',
    module: 'Society',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 4,
    event_sequence: 3,
    payload: {
      quote_id: 'mq_bread_001',
      sku: '粗麦面包 / 个',
      region: '青玄界 · 近郊镇',
      price: 12,
      unit: '文',
      kind: 'list',
      valid_until: '2026-03-09T06:00',
      note: '地区基准价 + 明确税费/成本 + 有限库存调价'
    }
  } as WorldRecord,

  /* ---------------- 2026-03-02 12:40 · 林巧买面包：一次完整交易 ---------------- */
  {
    record_id: C.rec(5),
    record_type: 'agent_decision',
    goal: '维持家庭月度预算（刚性支出优先）',
    knowledge_refs: ['kb_lin_quote_mq_bread_001', 'kb_lin_budget_mar'],
    candidates: ['买 2 个面包', '买 1 个面包 + 赊账', '不买，改吃存粮'],
    selected: '买 2 个面包',
    selected_reason: '报价在有效期内且现金足以覆盖；存粮不足以支撑到下次发工资',
    rejected: [
      {
        candidate: '买 1 个面包 + 赊账',
        stage: 'Rule Engine',
        reason: '商铺未授予该客户赊账额度',
        code: 'RULE_CREDIT_NOT_GRANTED'
      }
    ],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_txn_bread_001',
    causation_id: null,
    actor_id: 'npc_lin_qiao',
    world_time: '2026-03-02T12:40',
    recorded_at: '2026-03-02T12:40:03.771',
    module: 'Agent Runtime',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 5,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(6),
    record_type: 'rule_validation',
    rule_id: 'rule.market.transaction.commit',
    rule_epoch: 0,
    subject: 'cmd_buy_bread_2',
    eligible: true,
    checks: [
      { check: '买方支付能力', passed: true, detail: 'balance 3_144 ≥ 24' },
      { check: '库存可用', passed: true, detail: 'sku bread qty 40' },
      { check: '价格有效期', passed: true, detail: 'valid_until 2026-03-09T06:00' },
      { check: '预期版本匹配', passed: true, detail: 'expected_version 4 == current 4' }
    ],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_txn_bread_001',
    causation_id: C.rec(5),
    actor_id: 'rule_engine',
    world_time: '2026-03-02T12:40',
    recorded_at: '2026-03-02T12:40:03.812',
    module: 'Rule Engine',
    schema_version: '0.3.1',
    authority: 'audit',
    ruleset_hash: RULESET_HASH_E0,
    record_order: 6,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(7),
    record_type: 'event',
    event_type: 'TransactionCommitted',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_txn_bread_001',
    causation_id: C.rec(6),
    actor_id: 'store_yishan',
    world_time: '2026-03-02T12:41',
    recorded_at: '2026-03-02T12:41:00.005',
    module: 'Society',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 7,
    event_sequence: 4,
    payload: {
      sku: '粗麦面包 / 个',
      qty: 2,
      unit_price: 12,
      total: 24,
      currency: 'wen',
      buyer: 'npc_lin_qiao',
      seller: 'store_yishan',
      quote_id: 'mq_bread_001'
    },
    postings: [
      {
        posting_id: 'post_bread_001_buyer',
        entry_id: 'je_bread_001',
        account_id: 'acc_lin_qiao_cash',
        currency_id: 'wen',
        amount_minor: -24
      },
      {
        posting_id: 'post_bread_001_seller',
        entry_id: 'je_bread_001',
        account_id: 'acc_store_yishan_cash',
        currency_id: 'wen',
        amount_minor: 24
      }
    ],
    stock_movements: [
      {
        movement_id: 'stock_move_bread_001',
        sku_id: 'bread',
        quantity_minor: 2,
        base_unit: '个',
        from_holder_id: 'store_yishan',
        to_holder_id: 'npc_lin_qiao',
        movement_kind: 'transfer'
      }
    ],
    journal_status: 'posted',
    idempotency_key: 'idem_bread_lin_001'
  } as WorldRecord,

  {
    record_id: C.rec(8),
    record_type: 'observation',
    observer_id: 'npc_zhao_wan',
    channel: '同处一铺 · 目击',
    perceived: '林巧买走两个面包，与掌柜点头招呼',
    visible_to: ['npc_zhao_wan'],
    audience_scope: audienceScope('npc_zhao_wan'),
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_txn_bread_001',
    causation_id: C.rec(7),
    actor_id: 'npc_zhao_wan',
    world_time: '2026-03-02T12:41',
    recorded_at: '2026-03-02T12:41:04.900',
    module: 'Observation / Narrative',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 8,
    event_sequence: null
  } as WorldRecord,

  /* ---------------- 2026-03-05 · 攀比链：观察 → 认知 → 预算 → 选择 ---------------- */
  {
    record_id: C.rec(9),
    record_type: 'observation',
    observer_id: 'npc_he_tu',
    channel: '目击 · 街面',
    perceived: '赵婉换了新轿，抬轿人是镇外雇的',
    visible_to: ['npc_he_tu', 'npc_lin_qiao'],
    audience_scope: audienceScope('npc_he_tu', 'npc_lin_qiao'),
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_status_001',
    causation_id: null,
    actor_id: 'npc_he_tu',
    world_time: '2026-03-05T09:12',
    recorded_at: '2026-03-05T09:12:40.118',
    module: 'Observation / Narrative',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 9,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(10),
    record_type: 'agent_decision',
    goal: '在镇上的体面（参照群体：同层商铺掌柜）',
    knowledge_refs: ['kb_he_status_zhao', 'kb_he_cash_flow', 'kb_he_budget_q1'],
    candidates: ['订一顶同等规格的轿', '把铺面翻新一半', '维持现状'],
    selected: '把铺面翻新一半',
    selected_reason: '现金流仅够支付翻新订金，购轿将导致 3 月工资准备金不足',
    rejected: [
      {
        candidate: '订一顶同等规格的轿',
        stage: 'Household Budget',
        reason: '购买后流动性低于工资准备金阈值',
        code: 'BUDGET_LIQUIDITY_FLOOR'
      },
      {
        candidate: '维持现状',
        stage: 'Utility',
        reason: '地位消费效用低于参照群体压力成本',
        code: 'UTILITY_BELOW_THRESHOLD'
      }
    ],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_status_001',
    causation_id: C.rec(9),
    actor_id: 'npc_he_tu',
    world_time: '2026-03-05T14:30',
    recorded_at: '2026-03-05T14:30:22.505',
    module: 'Agent Runtime',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 10,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(11),
    record_type: 'event',
    event_type: 'RenovationOrderPlaced',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_status_001',
    causation_id: C.rec(10),
    actor_id: 'store_yishan',
    world_time: '2026-03-06T08:00',
    recorded_at: '2026-03-06T08:00:00.331',
    module: 'Society',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 11,
    event_sequence: 5,
    payload: {
      order_id: 'ord_renov_001',
      contractor: 'cohort_villagers',
      deposit: 4000,
      currency: 'wen',
      completion_due: '2026-03-20T18:00'
    },
    postings: [
      {
        posting_id: 'post_renov_001_store',
        entry_id: 'je_renov_001',
        account_id: 'acc_store_yishan_cash',
        currency_id: 'wen',
        amount_minor: -4000
      },
      {
        posting_id: 'post_renov_001_contractor',
        entry_id: 'je_renov_001',
        account_id: 'acc_cohort_villagers_cash',
        currency_id: 'wen',
        amount_minor: 4000
      }
    ],
    journal_status: 'posted',
    idempotency_key: 'idem_renov_001'
  } as WorldRecord,

  /* ---------------- 2026-03-09 · 报价更迭：不是模型重定价，是事件 ---------------- */
  {
    record_id: C.rec(12),
    record_type: 'agent_decision',
    goal: '面包毛利率维持在 22%–28%',
    knowledge_refs: ['kb_store_stock_bread', 'kb_store_cost_flour', 'kb_market_bread_quote_001'],
    candidates: ['维持 12 文', '涨到 13 文', '涨到 15 文并限购'],
    selected: '涨到 13 文',
    selected_reason: '面粉批发成本上浮 8%，维持原价将使毛利率跌破下限',
    rejected: [
      { candidate: '涨到 15 文并限购', stage: 'Rule Engine', reason: '超出地区价格波动上限', code: 'PRICE_CEILING' }
    ],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_pricing_002',
    causation_id: C.rec(4),
    actor_id: 'npc_he_tu',
    world_time: '2026-03-09T05:50',
    recorded_at: '2026-03-09T05:50:11.204',
    module: 'Agent Runtime',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 12,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(13),
    record_type: 'event',
    event_type: 'MarketQuoteSuperseded',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_pricing_002',
    causation_id: C.rec(12),
    actor_id: 'store_yishan',
    world_time: '2026-03-09T06:00',
    recorded_at: '2026-03-09T06:00:00.007',
    module: 'Society',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 13,
    event_sequence: 6,
    payload: {
      superseded_quote: 'mq_bread_001',
      new_quote: 'mq_bread_002',
      sku: '粗麦面包 / 个',
      old_price: 12,
      new_price: 13,
      region: '青玄界 · 近郊镇',
      note: '旧报价记录有效区间，不是被覆盖删除'
    }
  } as WorldRecord,

  {
    record_id: C.rec(14),
    record_type: 'observation',
    observer_id: 'npc_lin_qiao',
    channel: '购买时听闻',
    perceived: '掌柜说面粉贵了，面包要十三文',
    visible_to: ['npc_lin_qiao'],
    audience_scope: audienceScope('npc_lin_qiao'),
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_pricing_002',
    causation_id: C.rec(13),
    actor_id: 'npc_lin_qiao',
    world_time: '2026-03-09T18:20',
    recorded_at: '2026-03-09T18:20:55.640',
    module: 'Observation / Narrative',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 14,
    event_sequence: null
  } as WorldRecord,

  /* ---------------- 2026-03-15 · 灵矿枯竭：自然因果，非天道干预 ---------------- */
  {
    record_id: C.rec(15),
    record_type: 'event',
    event_type: 'OreVeinDepleted',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_vein_001',
    causation_id: null,
    actor_id: 'system',
    world_time: '2026-03-15T04:00',
    recorded_at: '2026-03-15T04:00:00.098',
    module: 'Spatial Engine',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 15,
    event_sequence: 7,
    payload: {
      vein_id: 'vein_cangshan_2',
      location: '苍山南麓 · 三号脉',
      remaining_ore: 0,
      cause: 'cumulative_extraction',
      note: '自然后果：因果来源为世界状态，不是天道主动干预'
    }
  } as WorldRecord,

  {
    record_id: C.rec(16),
    record_type: 'agent_decision',
    goal: '保障外门年度铁器供给',
    knowledge_refs: ['kb_hall_vein_2_status', 'kb_hall_supply_contract'],
    candidates: ['启用备用矿脉', '向镇上铁匠铺加价采购', '削减外门配额'],
    selected: '向镇上铁匠铺加价采购',
    selected_reason: '备用矿脉开采许可未获批准，加价采购在授权额度内',
    rejected: [
      {
        candidate: '启用备用矿脉',
        stage: 'Institution',
        reason: '缺少采矿许可程序完成记录',
        code: 'PROCEDURE_INCOMPLETE'
      }
    ],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_vein_001',
    causation_id: C.rec(15),
    actor_id: 'org_qingxuan_hall',
    world_time: '2026-03-15T10:20',
    recorded_at: '2026-03-15T10:20:31.889',
    module: 'Agent Runtime',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 16,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(17),
    record_type: 'event',
    event_type: 'PurchaseOrderAccepted',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_vein_001',
    causation_id: C.rec(16),
    actor_id: 'store_yishan',
    world_time: '2026-03-15T15:00',
    recorded_at: '2026-03-15T15:00:00.512',
    module: 'Society',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 17,
    event_sequence: 8,
    payload: {
      po_id: 'po_hall_iron_017',
      buyer: 'org_qingxuan_hall',
      seller: 'store_yishan',
      sku: '精铁犁头 / 件',
      qty: 60,
      unit_price: 3550,
      total: 213_000,
      currency: 'wen',
      quote_id: 'mq_plow_002',
      note: '超现实来源之外的真实交易，价格随供需与库存调整'
    },
    postings: [
      {
        posting_id: 'post_po_017_buyer',
        entry_id: 'je_po_017',
        account_id: 'acc_qingxuan_hall_cash',
        currency_id: 'wen',
        amount_minor: -213_000
      },
      {
        posting_id: 'post_po_017_seller',
        entry_id: 'je_po_017',
        account_id: 'acc_store_yishan_cash',
        currency_id: 'wen',
        amount_minor: 213_000
      }
    ],
    stock_movements: [
      {
        movement_id: 'stock_move_po_017',
        sku_id: 'iron_plow',
        quantity_minor: 60,
        base_unit: '件',
        from_holder_id: 'store_yishan',
        to_holder_id: 'org_qingxuan_hall',
        movement_kind: 'transfer'
      }
    ],
    journal_status: 'posted',
    idempotency_key: 'idem_po_hall_iron_017'
  } as WorldRecord,

  {
    record_id: C.rec(18),
    record_type: 'observation',
    observer_id: 'npc_lin_qiao',
    channel: '铺内听闻',
    perceived: '宗门外门一次性拉走六十件犁头，掌柜晚上数钱数到很晚',
    visible_to: ['npc_lin_qiao'],
    audience_scope: audienceScope('npc_lin_qiao'),
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_vein_001',
    causation_id: C.rec(17),
    actor_id: 'npc_lin_qiao',
    world_time: '2026-03-16T21:40',
    recorded_at: '2026-03-16T21:40:12.777',
    module: 'Observation / Narrative',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 18,
    event_sequence: null
  } as WorldRecord,

  /* ---------------- 2026-03-20 · 欠薪：违约不等于崩溃 ---------------- */
  {
    record_id: C.rec(19),
    record_type: 'event',
    event_type: 'WageDue',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_wage_arrears_001',
    causation_id: C.rec(3),
    actor_id: 'org_qingxuan_hall',
    world_time: '2026-03-20T20:00',
    recorded_at: '2026-03-20T20:00:00.221',
    module: 'Time Scheduler',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 19,
    event_sequence: 9,
    payload: {
      contract_id: 'ec_lin_001',
      due_amount: 1800,
      currency: 'wen',
      grace_until: '2026-03-27T20:00',
      pay_period: 'monthly'
    }
  } as WorldRecord,

  {
    record_id: C.rec(20),
    record_type: 'rule_validation',
    rule_id: 'rule.economy.payment.attempt',
    rule_epoch: 0,
    subject: 'wage_ec_lin_001_mar',
    eligible: false,
    checks: [
      { check: '付款方余额 ≥ 应付额', passed: false, detail: 'balance 1_284_000 − 冻结额度 900_000 = 384_000；但本笔需走宗门度支账户' },
      { check: '度支账户授权额度', passed: false, detail: 'monthly_cap 1_500_000 已用 1_512_000' },
      { check: '宽限期内', passed: true, detail: 'grace_until 2026-03-27T20:00' }
    ],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_wage_arrears_001',
    causation_id: C.rec(19),
    actor_id: 'rule_engine',
    world_time: '2026-03-20T20:00',
    recorded_at: '2026-03-20T20:00:00.340',
    module: 'Rule Engine',
    schema_version: '0.3.1',
    authority: 'audit',
    ruleset_hash: RULESET_HASH_E0,
    record_order: 20,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(21),
    record_type: 'event',
    event_type: 'WageArrearsRecorded',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_wage_arrears_001',
    causation_id: C.rec(20),
    actor_id: 'org_qingxuan_hall',
    world_time: '2026-03-20T20:00',
    recorded_at: '2026-03-20T20:00:00.512',
    module: 'Institution',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 21,
    event_sequence: 10,
    payload: {
      contract_id: 'ec_lin_001',
      employee: 'npc_lin_qiao',
      arrears_amount: 1800,
      currency: 'wen',
      grace_until: '2026-03-27T20:00',
      note: '资金不足产生状态事件，不因业务违约让进程异常崩溃'
    },
    postings: [
      {
        posting_id: 'post_wage_arrears_001_receivable',
        entry_id: 'je_wage_arrears_001',
        account_id: 'acc_lin_qiao_receivable',
        currency_id: 'wen',
        amount_minor: 1800
      },
      {
        posting_id: 'post_wage_arrears_001_payable',
        entry_id: 'je_wage_arrears_001',
        account_id: 'acc_hall_wage_payable',
        currency_id: 'wen',
        amount_minor: -1800
      }
    ],
    journal_status: 'posted',
    idempotency_key: 'idem_wage_arrears_ec_lin_001_mar'
  } as WorldRecord,

  {
    record_id: C.rec(22),
    record_type: 'agent_decision',
    goal: '在 3 月 27 日前支付房租',
    knowledge_refs: ['kb_lin_cash', 'kb_lin_receivable', 'kb_lin_rent_contract'],
    candidates: ['动用储蓄付租', '向掌柜预支', '与房东协商延期'],
    selected: '与房东协商延期',
    selected_reason: '动用储蓄将跌破家庭应急线；预支无合同依据',
    rejected: [
      {
        candidate: '向掌柜预支',
        stage: 'Institution',
        reason: '劳动合同无预支条款，且雇主当前处于欠薪状态',
        code: 'NO_CONTRACT_BASIS'
      },
      {
        candidate: '动用储蓄付租',
        stage: 'Household Budget',
        reason: '付租后流动性低于应急储备阈值',
        code: 'BUDGET_LIQUIDITY_FLOOR'
      }
    ],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_rent_chain_001',
    causation_id: C.rec(21),
    actor_id: 'npc_lin_qiao',
    world_time: '2026-03-21T19:00',
    recorded_at: '2026-03-21T19:00:44.981',
    module: 'Agent Runtime',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 22,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(23),
    record_type: 'event',
    event_type: 'RentExtensionAgreed',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_rent_chain_001',
    causation_id: C.rec(22),
    actor_id: 'npc_zhao_wan',
    world_time: '2026-03-22T10:15',
    recorded_at: '2026-03-22T10:15:00.700',
    module: 'Institution',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 23,
    event_sequence: 11,
    payload: {
      contract_id: 'rc_lin_002',
      original_due: '2026-03-25T20:00',
      extended_due: '2026-04-05T20:00',
      penalty_waived: 0,
      currency: 'wen',
      note: '受偿顺序由世界/制度包定义，不是硬编码全局优先级'
    }
  } as WorldRecord,

  /* ---------------- 2026-03-28 · 规则纪元升级：超凡觉醒 DLC ---------------- */
  {
    record_id: C.rec(24),
    record_type: 'runtime_diagnostic',
    severity: 'info',
    message: 'DLC 超凡觉醒 v0.1.0 静态冲突检查通过；暂停受影响调度 3 项',
    cost: { model_calls: 0, tokens: 0, ms: 184 },
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_epoch_1',
    causation_id: null,
    actor_id: 'extension_registry',
    world_time: '2026-03-28T02:00',
    recorded_at: '2026-03-28T02:00:01.004',
    module: 'Observability',
    schema_version: '0.3.1',
    authority: 'non-authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 24,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(25),
    record_type: 'event',
    event_type: 'LawActivationEvent',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_epoch_1',
    causation_id: C.rec(24),
    actor_id: 'system',
    world_time: '2026-03-28T02:30',
    recorded_at: '2026-03-28T02:30:00.016',
    module: 'Rule Engine',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 0,
    ruleset_hash: RULESET_HASH_E0,
    record_order: 25,
    event_sequence: 12,
    payload: {
      old_epoch: 0,
      new_epoch: 1,
      old_ruleset_hash: RULESET_HASH_E0,
      new_ruleset_hash: RULESET_HASH_E1,
      pack_lock_change: 'add transcendent_awakening@0.1.0',
      migration_ref: 'mig_awakening_001',
      note: '激活事件属于旧纪元；新规则从下一权威事件序号开始作用'
    }
  } as WorldRecord,

  {
    record_id: C.rec(26),
    record_type: 'rule_validation',
    rule_id: 'rule.law.extend.movement.flight',
    rule_epoch: 1,
    subject: 'org_qingxuan_hall',
    eligible: true,
    checks: [
      { check: '包能力声明', passed: true, detail: 'capability=world_law.extend' },
      { check: '适用主体/区域/时间', passed: true, detail: 'subject=authorized_npc region=cangshan' },
      { check: '与既有规则冲突', passed: false, detail: '未声明兼容关系的规则被拒绝' },
      { check: '优先级语义', passed: true, detail: 'priority=explicit, no load-order dependency' }
    ],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_epoch_1',
    causation_id: C.rec(25),
    actor_id: 'rule_engine',
    world_time: '2026-03-28T03:00',
    recorded_at: '2026-03-28T03:00:05.662',
    module: 'Rule Engine',
    schema_version: '0.3.1',
    authority: 'audit',
    ruleset_hash: RULESET_HASH_E1,
    record_order: 26,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(27),
    record_type: 'event',
    event_type: 'FlightComponentGranted',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_epoch_1',
    causation_id: C.rec(26),
    actor_id: 'org_qingxuan_hall',
    world_time: '2026-03-28T03:05',
    recorded_at: '2026-03-28T03:05:00.042',
    module: 'Spatial Engine',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 1,
    ruleset_hash: RULESET_HASH_E1,
    record_order: 27,
    event_sequence: 13,
    payload: {
      target: 'npc_lin_qiao',
      component: 'movement.flight',
      grant_authority: 'transcendent_awakening@0.1.0',
      constraint: '仍需有效位置、权限与事件提交；可绕过道路约束，不可绕过身份与事务',
      note: '目击者通过观察系统获取事实，媒体与组织基于有限知识自主反应'
    }
  } as WorldRecord,

  {
    record_id: C.rec(28),
    record_type: 'observation',
    observer_id: 'cohort_villagers',
    channel: '群体传播 · 集市口述',
    perceived: '林家丫头会飞了，有人说看见她从苍山那边飘回来',
    visible_to: ['cohort_villagers', 'npc_he_tu'],
    audience_scope: audienceScope('cohort_villagers', 'npc_he_tu'),
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_epoch_1',
    causation_id: C.rec(27),
    actor_id: 'cohort_villagers',
    world_time: '2026-03-29T11:00',
    recorded_at: '2026-03-29T11:00:20.135',
    module: 'Observation / Narrative',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 1,
    ruleset_hash: RULESET_HASH_E1,
    record_order: 28,
    event_sequence: null
  } as WorldRecord,

  /* ---------------- 2026-04-02 · 创作者干预：可追溯，不伪装自然事件 ---------------- */
  {
    record_id: C.rec(29),
    record_type: 'intervention',
    privilege_type: 'narrative_focus',
    authorized_by: 'world_owner',
    policy_version: 'pp_0.3.1',
    scope: 'instance=inst_qingxuan_near branch=br_main',
    reason: '世界所有者要求提高林巧的遭遇关注度',
    affected: ['npc_lin_qiao'],
    original_condition: 'encounter_weight baseline 1.0',
    result_events: ['rec_29a'],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_interv_001',
    causation_id: null,
    actor_id: 'world_owner',
    world_time: '2026-04-02T20:00',
    recorded_at: '2026-04-02T20:00:00.888',
    module: 'Gateway',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 1,
    ruleset_hash: RULESET_HASH_E1,
    record_order: 29,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(30),
    record_type: 'agent_decision',
    goal: '处理新的采买委托（遭遇关注度上调后出现）',
    knowledge_refs: ['kb_lin_flight_granted', 'kb_lin_hall_relation'],
    candidates: ['接下委托并飞往苍山', '推荐其他帮工', '拒绝'],
    selected: '接下委托并飞往苍山',
    selected_reason: '飞行组件已授予且委托在能力范围内；软特权只调节候选权重，不绕过规则',
    rejected: [],
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_interv_001',
    causation_id: C.rec(29),
    actor_id: 'npc_lin_qiao',
    world_time: '2026-04-03T07:30',
    recorded_at: '2026-04-03T07:30:11.090',
    module: 'Agent Runtime',
    schema_version: '0.3.1',
    authority: 'audit',
    rule_epoch: 1,
    ruleset_hash: RULESET_HASH_E1,
    record_order: 30,
    event_sequence: null
  } as WorldRecord,

  {
    record_id: C.rec(31),
    record_type: 'event',
    event_type: 'SchedulerSkipLogged',
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_epoch_1',
    causation_id: C.rec(25),
    actor_id: 'time_scheduler',
    world_time: '2026-04-03T23:59',
    recorded_at: '2026-04-03T23:59:59.010',
    module: 'Time Scheduler',
    schema_version: '0.3.1',
    authority: 'authoritative',
    rule_epoch: 1,
    ruleset_hash: RULESET_HASH_E1,
    record_order: 31,
    event_sequence: 14,
    payload: {
      skipped_task: 'cohort_villagers.aggregate_consumption',
      reason: '预算内跳过：当日无可结算阈值触发',
      budget_used: 0.42,
      note: '"为什么没发生"需要调度跳过记录，单靠事件账本无法回答'
    }
  } as WorldRecord,

  {
    record_id: C.rec(32),
    record_type: 'runtime_diagnostic',
    severity: 'warn',
    message: 'L2 决策器超时 1 次，回退至轻量决策器（T05 幂等重试未产生重复记账）',
    cost: { model_calls: 3, tokens: 8_412, ms: 2_310 },
    instance_id: 'inst_qingxuan_near',
    branch_id: 'br_main',
    trace_id: 'tr_interv_001',
    causation_id: C.rec(30),
    actor_id: 'agent_runtime',
    world_time: '2026-04-03T07:31',
    recorded_at: '2026-04-03T07:31:00.455',
    module: 'Observability',
    schema_version: '0.3.1',
    authority: 'non-authoritative',
    rule_epoch: 1,
    ruleset_hash: RULESET_HASH_E1,
    record_order: 32,
    event_sequence: null
  } as WorldRecord
]

/* -------------------------------------------------------- 派生结构 */

export function buildSpine(list: WorldRecord[]): SpineNode[] {
  const byId = new Map<string, SpineNode>()
  const order = [...list].sort((a, b) => a.record_order - b.record_order)
  for (const r of order) byId.set(r.record_id, { record: r, children: [], depth: 0 })

  const roots: SpineNode[] = []
  for (const node of byId.values()) {
    const parentId = node.record.causation_id
    const parent = parentId ? byId.get(parentId) : undefined
    if (parent && parent !== node) parent.children.push(node)
    else roots.push(node)
  }

  const walk = (nodes: SpineNode[], depth: number) => {
    for (const n of nodes) {
      n.depth = depth
      walk(n.children, depth + 1)
    }
  }
  walk(roots, 0)
  return roots
}

export const spine = buildSpine(records)

export const branches: Branch[] = [
  {
    branch_id: 'br_main',
    label: '主线 · 青玄界近郊',
    head_sequence: 14,
    forked_from: null,
    forked_at_sequence: null,
    epochs: [
      {
        epoch: 0,
        epoch_id: 'epoch_0',
        ruleset_hash: RULESET_HASH_E0,
        dlc_lock: 'lock_e0.json',
        activated_at_world_time: '2026-03-02T06:10',
        activated_by_event: 'rec_0001',
        start_sequence: 1,
        end_sequence_exclusive: 13,
        packs: ['world_qingxuan@0.3.0', 'system_economy@0.2.1', 'system_cultivation@0.2.0']
      },
      {
        epoch: 1,
        epoch_id: 'epoch_1',
        ruleset_hash: RULESET_HASH_E1,
        dlc_lock: 'lock_e1.json',
        activated_at_world_time: '2026-03-28T02:30',
        activated_by_event: 'rec_0025',
        start_sequence: 13,
        end_sequence_exclusive: null,
        packs: [
          'world_qingxuan@0.3.0',
          'system_economy@0.2.1',
          'system_cultivation@0.2.0',
          'transcendent_awakening@0.1.0'
        ]
      }
    ]
  },
  {
    branch_id: 'br_what_if',
    label: '分支 · 若林巧动用储蓄付租',
    head_sequence: 10,
    forked_from: 'br_main',
    forked_at_sequence: 10,
    epochs: [
      {
        epoch: 0,
        epoch_id: 'epoch_0',
        ruleset_hash: RULESET_HASH_E0,
        dlc_lock: 'lock_e0.json',
        activated_at_world_time: '2026-03-02T06:10',
        activated_by_event: 'rec_0001',
        start_sequence: 1,
        end_sequence_exclusive: null,
        packs: ['world_qingxuan@0.3.0', 'system_economy@0.2.1', 'system_cultivation@0.2.0']
      }
    ]
  }
]

export const ruleEpochs: RuleEpoch[] = branches[0].epochs

export const packs: Pack[] = [
  {
    id: 'world_qingxuan',
    kind: 'world',
    version: '0.3.0',
    engine_api: '0.3.x',
    state: 'enabled',
    capabilities: ['world.define', 'world.genesis'],
    schema_hash: 'sha256:daa7f43fcf9948ef21bfddd19ecfbf0c6a2a482ec78fbaf91b5a26e5eb245c0d',
    content_hash: 'sha256:865126bef87006709c4b1cc2e9795a8335ddbd57a7d9ba3d4b726ab48302728e'
  },
  {
    id: 'system_economy',
    kind: 'system',
    version: '0.2.1',
    engine_api: '0.3.x',
    state: 'enabled',
    capabilities: ['currency', 'account', 'journal', 'market_quote', 'employment_contract', 'budget'],
    schema_hash: 'sha256:136f8ec406a947a1a9f835ed62f6cf2106e6dd122f48dbbe550b15d5c7c489f8',
    content_hash: 'sha256:c3aaf7106f406547e14bd3211f2849910473d3f3a72d96af8598e7dc679941f0'
  },
  {
    id: 'system_cultivation',
    kind: 'system',
    version: '0.2.0',
    engine_api: '0.3.x',
    state: 'enabled',
    capabilities: ['realm', 'technique', 'ore_vein'],
    schema_hash: 'sha256:187efdd3f1b52996cb145b400c90557e674802d54c67e9cd4d3328be2e492ea7',
    content_hash: 'sha256:4d27b5a33faef513583f6ffd8d5449c6ea8cdbc1d6989b86e9e19556ec98c21d'
  },
  {
    id: 'transcendent_awakening',
    kind: 'content',
    version: '0.1.0',
    engine_api: '0.3.x',
    state: 'enabled',
    capabilities: ['world_law.extend', 'component.grant', 'quest', 'ui.page'],
    schema_hash: 'sha256:ba7a7db34c69a6f267835a7f2c1c6b2fbfa28ea76180ecd3815b8025707ee5ef',
    content_hash: 'sha256:3e5aac1a81eb6a18fc056c99ce5c8637d6164f188d97cff435e52eceb56093af'
  },
  {
    id: 'narrative_journal',
    kind: 'narrative',
    version: '0.1.4',
    engine_api: '0.3.x',
    state: 'enabled',
    capabilities: ['expression.style'],
    schema_hash: 'sha256:4c75f05a403d52cc8cf46755d82dda01ccadf11225b2fa358ac3b0efa9a91b54',
    content_hash: 'sha256:19a4dd405abb6cdb99b33611963ccbf46a2b346f7bc832a15131f7cddf01887e'
  }
]

export const snapshot: WorldSnapshot = {
  instance_id: 'inst_qingxuan_near',
  branch_id: 'br_main',
  branch_head_sequence: 14,
  current_epoch_id: 'epoch_1',
  world_time: '2026-04-03T23:59',
  uptime_days: 33,
  projections: {
    accounts: [
      { owner: 'npc_lin_qiao', balance_minor: 3120, type: 'household_cash', overdraft_limit_minor: 0 },
      { owner: 'npc_lin_qiao', balance_minor: 1800, type: 'receivable_wage', overdraft_limit_minor: 0 },
      { owner: 'npc_he_tu', balance_minor: 52400, type: 'household_cash', overdraft_limit_minor: 2000 },
      { owner: 'store_yishan', balance_minor: 18740, type: 'business_cash', overdraft_limit_minor: 0 },
      { owner: 'org_qingxuan_hall', balance_minor: 1284000, type: 'organization_cash', overdraft_limit_minor: 0 },
      { owner: 'org_qingxuan_hall', balance_minor: 1800, type: 'wage_payable', overdraft_limit_minor: 0 },
      { owner: 'cohort_villagers', balance_minor: 214500, type: 'cohort_aggregate_cash', overdraft_limit_minor: 0 }
    ],
    inventory: [
      { sku: '粗麦面包', seller: 'store_yishan', quantity_minor: 38, base_unit: '个' },
      { sku: '精铁犁头', seller: 'store_yishan', quantity_minor: 12, base_unit: '件' },
      { sku: '生铁锭', seller: 'store_yishan', quantity_minor: 240, base_unit: '斤' }
    ]
  },
  scheduler_cursor: '2026-04-03T23:59:59.010 · event_seq 14 · record_order 32',
  outbox_pending: 2
}

/* ------------------------------------------------------ 知识图谱 */

export const knowledgeEdges: KnowledgeEdge[] = [
  {
    observer: 'npc_lin_qiao',
    subject: 'store_yishan',
    channel: '铺内共事 · 直接经验',
    confidence: 0.95,
    evidence_refs: ['rec_0005', 'rec_0014'],
    believed: '掌柜 3 月进了一笔大单，手头宽裕',
    truth: 'store_yishan 现金 18_740，另有未结翻新订金',
    accurate: true
  },
  {
    observer: 'npc_lin_qiao',
    subject: 'org_qingxuan_hall',
    channel: '听闻 · 群体传播',
    confidence: 0.42,
    evidence_refs: ['rec_0018'],
    believed: '宗门很有钱，随时能发工资',
    truth: '宗门度支账户本月授权额度已用尽，导致欠薪',
    accurate: false
  },
  {
    observer: 'npc_he_tu',
    subject: 'npc_zhao_wan',
    channel: '目击 · 街面',
    confidence: 0.8,
    evidence_refs: ['rec_0009'],
    believed: '赵婉新换了轿，手头阔绰',
    truth: '赵婉净资产 342_000，但流动性 27_400，其中含三处房产抵押',
    accurate: false
  },
  {
    observer: 'cohort_villagers',
    subject: 'npc_lin_qiao',
    channel: '集市口述 · 传闻',
    confidence: 0.55,
    evidence_refs: ['rec_0028'],
    believed: '林家丫头会飞了',
    truth: 'movement.flight 组件已于 2026-03-28 授予（rec_0027）',
    accurate: true
  },
  {
    observer: 'npc_lin_qiao',
    subject: 'npc_zhao_wan',
    channel: '直接协商',
    confidence: 0.98,
    evidence_refs: ['rec_0022', 'rec_0023'],
    believed: '房东同意把房租延到 4 月 5 日',
    truth: 'RentExtensionAgreed 已提交，原到期日 2026-03-25 → 2026-04-05',
    accurate: true
  }
]

/* ------------------------------------------------ 为什么发生/没发生 */

export const whyNot: WhyNotItem[] = [
  {
    question: '这件商品为何涨价？',
    answer:
      'mq_bread_001（12 文，有效期至 03-09）到期后由 rec_0012 的 Agent 决策与 rec_0013 的 MarketQuoteSuperseded 生成 mq_bread_002（13 文）。触发条件是面粉批发成本上浮 8%，不是模型自行重定价。',
    evidence_refs: ['rec_0004', 'rec_0012', 'rec_0013', 'mq_bread_001', 'mq_bread_002'],
    severity: 'info'
  },
  {
    question: '工资为何未发？',
    answer:
      'rec_0020 规则验证未通过：度支账户月度授权额度已用尽（cap 1_500_000，已用 1_512_000）。随后 rec_0021 记录 WageArrearsRecorded，形成对林巧的应收与宗门的应付，进程未崩溃。',
    evidence_refs: ['rec_0019', 'rec_0020', 'rec_0021'],
    severity: 'warn'
  },
  {
    question: 'NPC 为什么买不起？',
    answer:
      'rec_0022 显示两个候选被拒：预支无合同依据（NO_CONTRACT_BASIS），动用储蓄会跌破应急流动性阈值（BUDGET_LIQUIDITY_FLOOR）。最终选择协商延期，产生 rec_0023。',
    evidence_refs: ['rec_0022', 'rec_0023'],
    severity: 'warn'
  },
  {
    question: '为何未发生攀比？',
    answer:
      'rec_0010 的何图确实观察到赵婉换轿并进入地位消费评估，但候选"订同等规格的轿"被 BUDGET_LIQUIDITY_FLOOR 拒绝，最终只翻新铺面。村民群体当日无触发：rec_0031 记录了 SchedulerSkipLogged（预算内跳过，无可结算阈值）。',
    evidence_refs: ['rec_0009', 'rec_0010', 'rec_0031'],
    severity: 'info'
  },
  {
    question: '谁拥有该资产？',
    answer:
      '精铁犁头 60 件随 rec_0017 的所有权转移事件由 store_yishan 划至 org_qingxuan_hall；库存投影同步为 12 件。房产归属在赵婉实体上，非工资推导。',
    evidence_refs: ['rec_0017', 'inv_plow'],
    severity: 'info'
  },
  {
    question: '某人的地位评价从何而来？',
    answer:
      'StatusPerception 按观察者分别记录：何图→赵婉 0.66（依据"三处房产"传闻），林巧→赵婉 0.83（依据直接催租经验）。同一目标在不同观察者处得分不同，且不等于净资产排序。',
    evidence_refs: ['sp_zhao_he', 'sp_zhao_lin'],
    severity: 'info'
  },
  {
    question: '矿脉为何枯竭？',
    answer:
      'rec_0015 标注 cause=cumulative_extraction，因果来源是世界状态。若日后出现天道干预，须另立事件与不同因果来源，不得把自然后果解释为地球/世界有意志。',
    evidence_refs: ['rec_0015', 'rec_0016'],
    severity: 'info'
  },
  {
    question: '这次法则是怎么生效的？',
    answer:
      'rec_0025 LawActivationEvent 是旧纪元的第 12 个权威事件，记录旧/新 ruleset_hash、包锁变化与迁移引用；新纪元（epoch 1）从 event_seq 13 起生效。rec_0026 中未声明兼容关系的规则被显式拒绝，不按加载顺序暗中决定。',
    evidence_refs: ['rec_0024', 'rec_0025', 'rec_0026', 'rec_0027'],
    severity: 'warn'
  }
]

/* ------------------------------------------------------ 经济曲线 */

export const economySeries: EconomyPoint[] = [
  { day: 1, household_cash: 3144, store_cash: 23_140, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 12 },
  { day: 4, household_cash: 3080, store_cash: 22_960, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 12 },
  { day: 7, household_cash: 3012, store_cash: 22_740, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 12 },
  { day: 10, household_cash: 2966, store_cash: 22_600, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 13 },
  { day: 13, household_cash: 2904, store_cash: 22_400, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 13 },
  { day: 16, household_cash: 2842, store_cash: 22_180, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 13 },
  { day: 19, household_cash: 2810, store_cash: 22_060, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 13 },
  { day: 22, household_cash: 2790, store_cash: 22_040, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 13 },
  { day: 25, household_cash: 2772, store_cash: 21_980, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 13 },
  { day: 28, household_cash: 2746, store_cash: 21_860, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 13 },
  { day: 31, household_cash: 2714, store_cash: 21_720, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 13 },
  { day: 34, household_cash: 2680, store_cash: 21_540, firm_cash: 1_497_000, net_worth_household: 8940, price_bread: 13 }
]
