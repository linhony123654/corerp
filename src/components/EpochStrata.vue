<script setup lang="ts">
/**
 * EpochStrata — 规则纪元的"地层"剖面
 *
 * 同一分支上，版本锁按事件序号分段不可变；每一段是一层地层。
 * 升级不是覆盖，而是沉积出新的一层，并留下可读取的边界事件。
 */
import { computed, ref } from 'vue'
import type { Branch } from '../types'
import { shortHash } from '../lib/format'

const props = defineProps<{
  branch: Branch
  records: unknown[]
  maxEventSequence: number
}>()

const active = ref(0)
const packsOpen = ref<number | null>(null)

const layers = computed(() => {
  const total = Math.max(1, props.maxEventSequence)
  return props.branch.epochs.map((e, i) => {
    const from = e.start_sequence
    const endExclusive = e.end_sequence_exclusive ?? props.maxEventSequence + 1
    return {
      ...e,
      index: i,
      fromPct: ((from - 1) / total) * 100,
      toPct: ((endExclusive - 1) / total) * 100,
      span: Math.max(0, endExclusive - from),
      rangeLabel: `[${from}, ${e.end_sequence_exclusive ?? '∞'})`
    }
  })
})

const fork = computed(() => {
  if (!props.branch.forked_from) return null
  return { from: props.branch.forked_from, at: props.branch.forked_at_sequence ?? 0 }
})

function togglePacks(i: number) {
  packsOpen.value = packsOpen.value === i ? null : i
}
</script>

<template>
  <div class="strata">
    <header class="strata__head">
      <span class="eyebrow">RULE EPOCH · 规则纪元</span>
      <h3 class="strata__title">{{ branch.label }}</h3>
      <p class="strata__sub mono">
        branch={{ branch.branch_id }} · fork=
        <template v-if="fork">{{ fork.from }}@seq{{ fork.at }}</template>
        <template v-else>genesis</template>
      </p>
    </header>

    <div class="strata__body">
      <div class="strata__axis mono" aria-hidden="true">
        <span>seq 1</span>
        <span>event_seq {{ maxEventSequence }}</span>
      </div>

      <div class="strata__stack" role="list">
        <div
          v-for="l in layers"
          :key="l.epoch"
          class="strata__layer"
          :class="{ 'is-active': active === l.index }"
          role="listitem"
          :style="{ '--from': l.fromPct + '%', '--to': l.toPct + '%' }"
          @mouseenter="active = l.index"
          @click="active = l.index"
        >
          <div class="strata__band">
            <span class="strata__epoch mono">EPOCH {{ l.epoch }}</span>
            <span class="strata__hash mono">{{ shortHash(l.ruleset_hash) }}</span>
          </div>
          <div class="strata__meta mono">
            <span>event_seq {{ l.rangeLabel }}</span>
            <span>·</span>
            <span>{{ l.span }} 事件</span>
            <span>·</span>
            <span>{{ l.packs.length }} 包锁定</span>
          </div>
        </div>
      </div>

      <div class="strata__detail">
        <p class="strata__note">
          版本锁按事件序号分段不可变：旧事件按旧纪元解释，新规则只作用于生效点之后。
          升级产生新纪元并保存前后边界，而不是就地覆盖。
        </p>
        <button class="strata__packs" type="button" @click="togglePacks(active)">
          <span class="mono">{{ packsOpen === active ? '−' : '+' }}</span>
          <span>纪元 {{ layers[active]?.epoch }} 的 DLC 锁文件</span>
        </button>
        <ul v-if="packsOpen === active" class="strata__packlist mono">
          <li v-for="p in layers[active]?.packs ?? []" :key="p">
            <span class="dot" />{{ p }}
          </li>
        </ul>
        <p class="strata__law mono" v-if="layers[active]">
          生效事件 {{ layers[active].activated_by_event }} · 世界时间 {{ layers[active].activated_at_world_time }}
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.strata {
  display: grid;
  gap: var(--sp-4);
}

.strata__head {
  display: grid;
  gap: 2px;
}

.strata__title {
  font-size: 19px;
  font-weight: 600;
}

.strata__sub {
  font-size: 11px;
  color: var(--ink-600);
  margin: 0;
}

.strata__body {
  display: grid;
  gap: var(--sp-3);
}

.strata__axis {
  display: flex;
  justify-content: space-between;
  font-size: 9.5px;
  color: var(--ink-600);
  letter-spacing: 0.1em;
}

.strata__stack {
  display: grid;
  gap: 6px;
}

.strata__layer {
  position: relative;
  cursor: pointer;
  padding: 10px 0 10px 14px;
  border-left: 2px solid var(--ink-400);
  transition:
    border-color var(--dur-mid) var(--ease-mech),
    background var(--dur-mid) var(--ease-mech);
}

.strata__layer::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 2px;
  background: linear-gradient(to bottom, var(--amber-400), var(--amber-600));
  transform-origin: top;
  transform: scaleY(0);
  transition: transform var(--dur-slow) var(--ease-snap);
}

.strata__layer.is-active::before {
  transform: scaleY(1);
}

.strata__layer.is-active {
  background: linear-gradient(to right, rgba(224, 160, 74, 0.06), transparent 70%);
  border-left-color: transparent;
}

.strata__band {
  display: flex;
  align-items: baseline;
  gap: var(--sp-3);
}

.strata__epoch {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.14em;
  color: var(--amber-400);
}

.strata__hash {
  font-size: 10.5px;
  color: var(--ink-700);
}

.strata__meta {
  display: flex;
  gap: 6px;
  font-size: 10px;
  color: var(--ink-600);
  margin-top: 3px;
}

.strata__detail {
  border-top: 1px solid var(--line);
  padding-top: var(--sp-3);
  display: grid;
  gap: var(--sp-3);
}

.strata__note {
  font-size: 12px;
  color: var(--ink-700);
  margin: 0;
  line-height: 1.65;
}

.strata__packs {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  background: none;
  border: 1px solid var(--line-strong);
  color: var(--ink-800);
  padding: 6px 12px;
  font-size: 11.5px;
  letter-spacing: 0.02em;
  transition:
    border-color var(--dur-fast) var(--ease-mech),
    color var(--dur-fast) var(--ease-mech);
}

.strata__packs:hover {
  border-color: var(--amber-400);
  color: var(--amber-400);
}

.strata__packs .mono {
  color: var(--amber-400);
}

.strata__packlist {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 4px;
}

.strata__packlist li {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11px;
  color: var(--ink-700);
}

.dot {
  width: 4px;
  height: 4px;
  background: var(--teal-400);
  flex: none;
}

.strata__law {
  font-size: 10px;
  color: var(--ink-600);
  margin: 0;
}
</style>
