<script setup lang="ts">
/**
 * EntityGrid — 实体与状态投影
 * 净资产 / 流动性 / 现金必须分开列；地位评价是主观的，按观察者分行。
 */
import { computed, ref } from 'vue'
import type { Entity } from '../types'
import { fmtNum, money } from '../lib/format'

const props = defineProps<{
  entities: Entity[]
}>()

const sortKey = ref<'net_worth_minor' | 'liquidity_minor' | 'cash_minor'>('net_worth_minor')

const sorted = computed(() => [...props.entities].sort((a, b) => b[sortKey.value] - a[sortKey.value]))

const total = computed(() => ({
  cash: props.entities.reduce((s, e) => s + e.cash_minor, 0),
  net_worth: props.entities.reduce((s, e) => s + e.net_worth_minor, 0),
  liquidity: props.entities.reduce((s, e) => s + e.liquidity_minor, 0)
}))

function ratio(e: Entity): number {
  const max = Math.max(...props.entities.map((x) => Math.max(x.net_worth_minor, x.liquidity_minor, x.cash_minor)))
  return max > 0 ? e.net_worth_minor / max : 0
}
</script>

<template>
  <div class="egrid">
    <header class="egrid__head">
      <div>
        <span class="eyebrow">ENTITY · 实体与投影</span>
        <h3 class="egrid__title">三种口径，不互相推导</h3>
      </div>
      <div class="seg" role="group" aria-label="排序">
        <button :class="{ 'is-on': sortKey === 'net_worth_minor' }" type="button" @click="sortKey = 'net_worth_minor'">净资产</button>
        <button :class="{ 'is-on': sortKey === 'liquidity_minor' }" type="button" @click="sortKey = 'liquidity_minor'">流动性</button>
        <button :class="{ 'is-on': sortKey === 'cash_minor' }" type="button" @click="sortKey = 'cash_minor'">现金</button>
      </div>
    </header>

    <div class="egrid__list" role="table" aria-label="实体列表">
      <div class="egrid__row egrid__row--head" role="row">
        <span role="columnheader">实体</span>
        <span role="columnheader" class="num">现金</span>
        <span role="columnheader" class="num">流动性</span>
        <span role="columnheader" class="num">净资产</span>
      </div>

      <div v-for="e in sorted" :key="e.id" class="egrid__row" role="row">
        <span class="egrid__name" role="cell">
          <span class="egrid__dot" :class="'arch-' + e.archetype" />
          <span class="egrid__n">{{ e.name }}</span>
          <span class="egrid__role mono">{{ e.role }}</span>
        </span>
        <span class="num mono" role="cell">{{ fmtNum(e.cash_minor) }}</span>
        <span class="num mono" role="cell">{{ fmtNum(e.liquidity_minor) }}</span>
        <span class="num mono egrid__nw" role="cell">
          <span class="egrid__bar" :style="{ width: ratio(e) * 100 + '%' }" />
          <span class="egrid__nwv">{{ fmtNum(e.net_worth_minor) }}</span>
        </span>
      </div>
    </div>

    <footer class="egrid__foot">
      <p class="egrid__note">
        净资产 = 资产估值 − 负债；流动性 = 可用现金与短期可变现资产。估值不是可直接支付的现金，
        因此<b>高净资产低流动性</b>是可表达的正常状态。
      </p>
      <dl class="egrid__totals mono">
        <div><dt>合计现金</dt><dd>{{ money(total.cash) }}</dd></div>
        <div><dt>合计流动性</dt><dd>{{ money(total.liquidity) }}</dd></div>
        <div><dt>合计净资产</dt><dd>{{ money(total.net_worth) }}</dd></div>
      </dl>
    </footer>
  </div>
</template>

<style scoped>
.egrid {
  display: grid;
  gap: var(--sp-4);
}

.egrid__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--sp-4);
}

.egrid__title {
  font-size: 19px;
  margin-top: 2px;
}

.seg {
  display: inline-flex;
  border: 1px solid var(--line-strong);
}

.seg button {
  background: none;
  border: 0;
  color: var(--ink-600);
  font-size: 11px;
  padding: 5px 11px;
  letter-spacing: 0.04em;
  transition: all var(--dur-fast) var(--ease-mech);
}

.seg button + button {
  border-left: 1px solid var(--line-strong);
}

.seg button.is-on {
  background: var(--ink-300);
  color: var(--amber-400);
}

.egrid__list {
  display: grid;
  gap: 0;
  border-top: 1px solid var(--line-strong);
}

.egrid__row {
  display: grid;
  grid-template-columns: minmax(180px, 2.2fr) 1fr 1fr 1.3fr;
  gap: var(--sp-4);
  align-items: center;
  padding: 11px 0;
  border-bottom: 1px solid var(--line);
}

.egrid__row--head {
  padding: 6px 0;
  border-bottom: 1px solid var(--line-strong);
}

.egrid__row--head span {
  font-family: var(--font-mono);
  font-size: 9.5px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--ink-600);
}

.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.egrid__name {
  display: grid;
  grid-template-columns: auto 1fr;
  grid-template-rows: auto auto;
  column-gap: 10px;
  align-items: center;
  min-width: 0;
}

.egrid__dot {
  grid-row: span 2;
  width: 6px;
  height: 6px;
  background: var(--ink-500);
}

.arch-person {
  background: var(--teal-400);
}
.arch-organization {
  background: var(--amber-400);
}
.arch-store {
  background: var(--violet-400);
}
.arch-cohort {
  background: var(--ink-500);
}

.egrid__n {
  font-size: 13.5px;
  color: var(--ink-900);
}

.egrid__role {
  font-size: 10.5px;
  color: var(--ink-600);
  grid-column: 2;
}

.egrid__nw {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: flex-end;
}

.egrid__bar {
  position: absolute;
  right: 0;
  bottom: -1px;
  height: 2px;
  background: linear-gradient(to left, var(--amber-400), rgba(224, 160, 74, 0.1));
}

.egrid__nwv {
  position: relative;
  color: var(--ink-900);
}

.egrid__foot {
  display: grid;
  gap: var(--sp-4);
  border-top: 1px solid var(--line);
  padding-top: var(--sp-4);
}

.egrid__note {
  margin: 0;
  font-size: 12px;
  color: var(--ink-700);
  line-height: 1.7;
  max-width: 76ch;
}

.egrid__note b {
  color: var(--ink-900);
}

.egrid__totals {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-5);
  margin: 0;
}

.egrid__totals div {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.egrid__totals dt {
  font-size: 10px;
  letter-spacing: 0.1em;
  color: var(--ink-600);
}

.egrid__totals dd {
  margin: 0;
  font-size: 14px;
  color: var(--ink-900);
  font-variant-numeric: tabular-nums;
}

@media (max-width: 720px) {
  .egrid__row {
    grid-template-columns: 1.4fr 1fr 1fr;
    row-gap: 2px;
  }
  .egrid__row--head span:last-child {
    display: none;
  }
  .egrid__nw {
    grid-column: 2 / -1;
    justify-content: flex-start;
  }
}
</style>
