<script setup lang="ts">
/**
 * RecordInspector — 单条记录的完整展开
 * 右侧探针面板：选中脊梁上任一节点后，展示它的权威性、因果、分录与原始字段。
 */
import { computed } from 'vue'
import type { WorldRecord } from '../types'
import { allKinds, fmtFullTime, money } from '../lib/format'

const props = defineProps<{
  record: WorldRecord | null
  parent: WorldRecord | null
  children: WorldRecord[]
}>()

const emit = defineEmits<{
  (e: 'focusRecord', id: string): void
}>()

const k = computed(() => (props.record ? allKinds[props.record.record_type] : null))

const plane = computed(() => (props.record?.record_type === 'event' ? '线上方 · 权威' : '线下方 · 审计 / 非权威'))

const baseFields = computed(() => {
  const r = props.record
  if (!r) return []
  return [
    ['record_id', r.record_id],
    ['record_type', r.record_type],
    ['trace_id', r.trace_id],
    ['causation_id', r.causation_id ?? '—'],
    ['actor_id', r.actor_id],
    ['module', r.module],
    ['world_time', r.world_time],
    ['recorded_at', r.recorded_at],
    ['rule_epoch', String(r.rule_epoch)],
    ['ruleset_hash', r.ruleset_hash],
    ['record_order', String(r.record_order)],
    ['event_sequence', r.event_sequence === null ? '—' : String(r.event_sequence)],
    ['schema_version', r.schema_version],
    ['authority', r.authority]
  ]
})

const postings = computed(() => {
  const r = props.record
  if (!r || r.record_type !== 'event' || !r.postings?.length) return null
  const debit = r.postings.reduce((s, p) => s + Math.max(0, p.amount_minor), 0)
  const credit = r.postings.reduce((s, p) => s + Math.max(0, -p.amount_minor), 0)
  const byCurrency = new Map<string, number>()
  for (const posting of r.postings) {
    byCurrency.set(posting.currency_id, (byCurrency.get(posting.currency_id) ?? 0) + posting.amount_minor)
  }
  return { rows: r.postings, debit, credit, balanced: [...byCurrency.values()].every((total) => total === 0) }
})

const goods = computed(() => {
  const r = props.record
  if (!r || r.record_type !== 'event' || !r.stock_movements?.length) return null
  return r.stock_movements
})

const payloadRows = computed(() => {
  const r = props.record
  if (!r) return []
  if (r.record_type === 'event') {
    return Object.entries(r.payload).map(([key, value]) => [key, String(value)])
  }
  if (r.record_type === 'agent_decision') {
    return [
      ['goal', r.goal],
      ['knowledge_refs', r.knowledge_refs.join(', ')],
      ['candidates', r.candidates.join(' / ')],
      ['selected', r.selected],
      ['selected_reason', r.selected_reason],
      ['rejected', r.rejected.map((x) => `${x.candidate} → ${x.code}（${x.reason}）`).join(' / ') || '—']
    ] as [string, string][]
  }
  if (r.record_type === 'rule_validation') {
    return [
      ['rule_id', r.rule_id],
      ['subject', r.subject],
      ['eligible', r.eligible ? 'true' : 'false'],
      ['checks', r.checks.map((c) => `${c.passed ? '✓' : '✗'} ${c.check} — ${c.detail}`).join(' / ')]
    ] as [string, string][]
  }
  if (r.record_type === 'observation') {
    return [
      ['observer_id', r.observer_id],
      ['channel', r.channel],
      ['perceived', r.perceived],
      ['visible_to', r.visible_to.join(', ')],
      [
        'audience_scope',
        `${r.audience_scope.instance_id}/${r.audience_scope.branch_id} · subjects=${r.audience_scope.subject_ids.join(',')} · fields=${r.audience_scope.fields.join(',')}`
      ]
    ] as [string, string][]
  }
  if (r.record_type === 'runtime_diagnostic') {
    return [
      ['severity', r.severity],
      ['message', r.message],
      ['cost', `model_calls=${r.cost.model_calls} tokens=${r.cost.tokens} ms=${r.cost.ms}`]
    ] as [string, string][]
  }
  if (r.record_type === 'intervention') {
    return [
      ['privilege_type', r.privilege_type],
      ['authorized_by', r.authorized_by],
      ['policy_version', r.policy_version],
      ['scope', r.scope],
      ['reason', r.reason],
      ['affected', r.affected.join(', ')],
      ['original_condition', r.original_condition],
      ['result_events', r.result_events.join(', ')]
    ] as [string, string][]
  }
  return []
})
</script>

<template>
  <div class="inspector">
    <template v-if="record && k">
      <header class="inspector__head" :style="{ '--accent': k.accent }">
        <div class="inspector__glyph" :style="{ color: k.accent, borderColor: k.accent }">{{ k.glyph }}</div>
        <div class="inspector__title">
          <span class="eyebrow" :style="{ color: k.accent }">{{ k.label }} · {{ k.authority }}</span>
          <h3 class="inspector__name">
            {{ record.record_type === 'event' ? record.event_type : record.record_id }}
          </h3>
          <p class="inspector__plane mono">
            {{ plane }} · record {{ record.record_order }} · event {{ record.event_sequence ?? '—' }} ·
            {{ fmtFullTime(record.world_time) }}
          </p>
        </div>
      </header>

      <p class="inspector__desc">{{ k.description }}</p>

      <section v-if="postings" class="inspector__section">
        <div class="inspector__section-head">
          <span class="eyebrow">资金分录 · DOUBLE ENTRY（货币单位）</span>
          <span class="mono" :class="{ 'is-bad': !postings.balanced, 'is-good': postings.balanced }">
            {{ postings.balanced ? '借贷平衡 ✓' : '不平衡 ✗' }}
          </span>
        </div>
        <table class="ledger">
          <thead>
            <tr>
              <th>账户</th>
              <th class="num">借</th>
              <th class="num">贷</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(p, i) in postings.rows" :key="i">
              <td class="mono">{{ p.account_id }} · {{ p.currency_id }}</td>
              <td class="num mono">{{ p.amount_minor > 0 ? money(p.amount_minor) : '' }}</td>
              <td class="num mono">{{ p.amount_minor < 0 ? money(-p.amount_minor) : '' }}</td>
            </tr>
          </tbody>
          <tfoot>
            <tr>
              <td class="mono">合计</td>
              <td class="num mono">{{ money(postings.debit) }}</td>
              <td class="num mono">{{ money(postings.credit) }}</td>
            </tr>
          </tfoot>
        </table>
      </section>

      <section v-if="goods" class="inspector__section">
        <span class="eyebrow">实物转移 · 数量流转（独立于资金）</span>
        <ul class="goods">
          <li v-for="(g, i) in goods" :key="i">
            <span class="goods__item">{{ g.sku_id }}</span>
            <span class="goods__qty mono">× {{ g.quantity_minor }} {{ g.base_unit }}</span>
            <span class="goods__flow mono">{{ g.from_holder_id }} → {{ g.to_holder_id }}</span>
          </li>
        </ul>
        <p class="inspector__hint">
          商品/实物的创建、所有权转移、消耗或报废有数量流转证据，与货币分录是两条独立约束——
          面包被吃掉是实物消耗，不会自动销毁货币。
        </p>
      </section>

      <section class="inspector__section">
        <span class="eyebrow">公共字段 · §9</span>
        <dl class="kv">
          <template v-for="([key, value]) in baseFields" :key="key">
            <dt class="mono">{{ key }}</dt>
            <dd class="mono" :class="{ 'is-muted': value === '—' }">{{ value }}</dd>
          </template>
        </dl>
      </section>

      <section class="inspector__section">
        <span class="eyebrow">载荷 / 决策明细</span>
        <dl class="kv">
          <template v-for="([key, value]) in payloadRows" :key="key">
            <dt class="mono">{{ key }}</dt>
            <dd class="mono" :class="{ 'is-long': String(value).length > 42 }">{{ value }}</dd>
          </template>
        </dl>
      </section>

      <section v-if="parent || children.length" class="inspector__section">
        <span class="eyebrow">因果链</span>
        <div class="inspector__causal">
          <button v-if="parent" class="causal" type="button" @click="emit('focusRecord', parent.record_id)">
            <span class="causal__dir">↑ 直接触发</span>
            <span class="mono">{{ parent.record_id }} · {{ parent.record_type === 'event' ? parent.event_type : parent.record_type }}</span>
          </button>
          <button v-for="c in children" :key="c.record_id" class="causal" type="button" @click="emit('focusRecord', c.record_id)">
            <span class="causal__dir">↓ 后续影响</span>
            <span class="mono">{{ c.record_id }} · {{ c.record_type === 'event' ? c.event_type : c.record_type }}</span>
          </button>
        </div>
      </section>
    </template>

    <div v-else class="inspector__empty">
      <div class="inspector__empty-mark mono">◇</div>
      <p>点击脊梁上的任一节点，或按 <kbd>Tab</kbd> 聚焦后用 <kbd>←</kbd> <kbd>→</kbd> 浏览。</p>
      <p class="inspector__empty-note">探针会说明每一条记录属于哪个平面、由谁触发、产生了什么分录。</p>
    </div>
  </div>
</template>

<style scoped>
.inspector {
  display: grid;
  gap: var(--sp-4);
  align-content: start;
}

.inspector__head {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: var(--sp-4);
  align-items: start;
  padding-bottom: var(--sp-4);
  border-bottom: 1px solid var(--line);
}

.inspector__glyph {
  width: 40px;
  height: 40px;
  display: grid;
  place-items: center;
  border: 1px solid;
  font-size: 17px;
}

.inspector__title {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.inspector__name {
  font-family: var(--font-mono);
  font-size: 15px;
  font-weight: 600;
  letter-spacing: -0.01em;
  word-break: break-all;
  color: var(--ink-900);
}

.inspector__plane {
  font-size: 10.5px;
  color: var(--ink-600);
  margin: 0;
}

.inspector__desc {
  font-size: 12.5px;
  color: var(--ink-700);
  line-height: 1.7;
  margin: 0;
  border-left: 2px solid var(--line-strong);
  padding-left: var(--sp-3);
}

.inspector__section {
  display: grid;
  gap: var(--sp-3);
}

.inspector__section-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--sp-3);
}

.is-good {
  color: var(--teal-400);
  font-size: 11px;
}

.is-bad {
  color: var(--rose-400);
  font-size: 11px;
}

.ledger {
  width: 100%;
  border-collapse: collapse;
  font-size: 11.5px;
}

.ledger th {
  text-align: left;
  font-family: var(--font-mono);
  font-size: 9.5px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--ink-600);
  font-weight: 500;
  padding: 4px 6px;
  border-bottom: 1px solid var(--line);
}

.ledger td {
  padding: 5px 6px;
  border-bottom: 1px solid var(--line);
  color: var(--ink-800);
}

.ledger .num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.ledger tfoot td {
  border-top: 1px solid var(--line-strong);
  border-bottom: 0;
  color: var(--ink-900);
  font-weight: 600;
}

.kv {
  display: grid;
  grid-template-columns: minmax(96px, auto) 1fr;
  gap: 6px var(--sp-4);
  margin: 0;
  font-size: 11.5px;
}

.kv dt {
  color: var(--ink-600);
  font-size: 10.5px;
  letter-spacing: 0.02em;
  white-space: nowrap;
}

.kv dd {
  margin: 0;
  color: var(--ink-800);
  word-break: break-word;
  line-height: 1.6;
}

.kv dd.is-muted {
  color: var(--ink-500);
}

.kv dd.is-long {
  color: var(--ink-900);
  font-size: 12px;
}

.inspector__causal {
  display: grid;
  gap: 6px;
}

.goods {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 0;
  border-top: 1px solid var(--line);
}

.goods li {
  display: grid;
  grid-template-columns: 1fr auto auto;
  gap: var(--sp-4);
  align-items: baseline;
  padding: 7px 0;
  border-bottom: 1px solid var(--line);
}

.goods__item {
  font-size: 12.5px;
  color: var(--ink-900);
}

.goods__qty {
  font-size: 12px;
  color: var(--violet-400);
}

.goods__flow {
  font-size: 10.5px;
  color: var(--ink-600);
}

.inspector__hint {
  margin: 0;
  font-size: 11px;
  color: var(--ink-600);
  line-height: 1.7;
}

.causal {
  display: grid;
  gap: 2px;
  text-align: left;
  background: var(--ink-100);
  border: 1px solid var(--line);
  border-left: 2px solid var(--amber-400);
  padding: 7px 10px;
  color: var(--ink-800);
  transition: all var(--dur-fast) var(--ease-mech);
}

.causal:hover {
  background: var(--ink-200);
  border-color: var(--line-strong);
  border-left-color: var(--teal-400);
}

.causal__dir {
  font-size: 9.5px;
  letter-spacing: 0.12em;
  color: var(--ink-600);
  text-transform: uppercase;
}

.causal .mono {
  font-size: 11.5px;
  color: var(--ink-800);
}

.inspector__empty {
  display: grid;
  gap: var(--sp-3);
  place-items: center;
  text-align: center;
  padding: var(--sp-7) var(--sp-4);
  color: var(--ink-600);
}

.inspector__empty-mark {
  font-size: 34px;
  color: var(--ink-400);
}

.inspector__empty p {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.7;
  max-width: 34ch;
}

.inspector__empty kbd {
  font-family: var(--font-mono);
  font-size: 10.5px;
  border: 1px solid var(--line-strong);
  padding: 1px 5px;
  color: var(--ink-800);
}

.inspector__empty-note {
  color: var(--ink-500);
  font-size: 11.5px !important;
}

@media (max-width: 720px) {
  .kv {
    grid-template-columns: 1fr;
    gap: 2px;
  }
  .kv dt {
    margin-top: 6px;
  }
}
</style>
