<script setup lang="ts">
/**
 * KnowledgeGraph — 知识渗透图
 *
 * World Truth / Knowledge / Belief 三者分离：
 * 信念可错误，且错误本身要可见。这里把"观察者相信的"与"世界真实的"并排画出来，
 * 错误认知用虚线与警示色标出——玩家视角下不得直接读取隐藏事实。
 */
import { computed, ref } from 'vue'
import type { KnowledgeEdge } from '../types'
import { pct } from '../lib/format'

const props = defineProps<{
  edges: KnowledgeEdge[]
}>()

const activeIndex = ref(0)
const showTruth = ref(false)
const filter = ref<'all' | 'accurate' | 'wrong'>('all')

const W = 1000
const H = 340
const CX = W / 2

const truthY = 44
const observerY = H - 62

const rows = computed(() => {
  const list = props.edges
    .map((e, i) => ({ e, i }))
    .filter(({ e }) => (filter.value === 'all' ? true : filter.value === 'accurate' ? e.accurate : !e.accurate))
  const gap = list.length > 1 ? (W - 200) / (list.length - 1) : 0
  return list.map(({ e, i }, idx) => ({
    ...e,
    i,
    x: list.length > 1 ? 100 + idx * gap : CX,
    y: observerY,
    truthX: CX + (idx - (list.length - 1) / 2) * 120,
    truthY,
    accurate: e.accurate
  }))
})

const active = computed(() => props.edges[activeIndex.value] ?? null)

const wrongCount = computed(() => props.edges.filter((e) => !e.accurate).length)
</script>

<template>
  <div class="kgraph">
    <header class="kgraph__head">
      <div>
        <span class="eyebrow">KNOWLEDGE · 认知与信念</span>
        <h3 class="kgraph__title">谁相信什么，与什么是真的</h3>
      </div>
      <div class="kgraph__controls">
        <div class="seg" role="group" aria-label="过滤认知条目">
          <button :class="{ 'is-on': filter === 'all' }" type="button" @click="filter = 'all'">全部</button>
          <button :class="{ 'is-on': filter === 'accurate' }" type="button" @click="filter = 'accurate'">相符</button>
          <button :class="{ 'is-on': filter === 'wrong' }" type="button" @click="filter = 'wrong'">
            错误信念 {{ wrongCount }}
          </button>
        </div>
        <label class="toggle">
          <input v-model="showTruth" type="checkbox" />
          <span class="mono">显示 World Truth</span>
        </label>
      </div>
    </header>

    <svg
      class="kgraph__svg"
      :viewBox="`0 0 ${W} ${H}`"
      role="img"
      aria-label="认知渗透图：观察者、信念与世界真相的连线"
    >
      <defs>
        <linearGradient id="kg-truth" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="rgba(224,160,74,0.9)" />
          <stop offset="100%" stop-color="rgba(224,160,74,0.15)" />
        </linearGradient>
      </defs>

      <!-- 世界真相平面 -->
      <line :x1="60" :y1="truthY" :x2="W - 60" :y2="truthY" stroke="url(#kg-truth)" stroke-width="1.2" />
      <text :x="60" :y="truthY - 14" class="kgraph__plane mono" fill="var(--amber-400)">WORLD TRUTH · 已提交事实</text>

      <!-- 观察者平面 -->
      <line :x1="60" :y1="observerY" :x2="W - 60" :y2="observerY" stroke="var(--line-strong)" stroke-width="1" />
      <text :x="60" :y="observerY + 20" class="kgraph__plane mono" fill="var(--ink-600)">
        OBSERVERS · 有限知识 / 信念
      </text>

      <!-- 连线 -->
      <g v-for="r in rows" :key="r.i" :class="{ 'is-dim': activeIndex !== r.i }">
        <path
          :d="`M${r.x} ${r.y - 8} C${r.x} ${r.y - 60}, ${r.truthX} ${truthY + 60}, ${r.truthX} ${truthY + 8}`"
          fill="none"
          :stroke="r.accurate ? 'var(--teal-500)' : 'var(--rose-500)'"
          stroke-width="1.2"
          :stroke-dasharray="r.accurate ? '' : '5 4'"
          :opacity="activeIndex === r.i ? 0.95 : 0.32"
        />
        <!-- 置信度节点 -->
        <circle :cx="r.x" :cy="r.y - 8" :r="4 + r.confidence * 4" :fill="r.accurate ? 'var(--teal-400)' : 'var(--rose-400)'" />
        <text :x="r.x" :y="r.y + 16" class="kgraph__node mono" :fill="r.accurate ? 'var(--teal-400)' : 'var(--rose-400)'">
          {{ r.observer.replace('npc_', '').replace('cohort_', '').replace('org_', '') }}
        </text>
        <text :x="r.x" :y="r.y + 30" class="kgraph__conf mono">{{ pct(r.confidence) }}</text>

        <!-- 真相锚点 -->
        <g v-if="showTruth">
          <rect
            :x="r.truthX - 5"
            :y="truthY - 5"
            width="10"
            height="10"
            fill="var(--ink-100)"
            stroke="var(--amber-400)"
            stroke-width="1.2"
            :transform="`rotate(45 ${r.truthX} ${truthY})`"
          />
          <text :x="r.truthX" :y="truthY - 16" class="kgraph__truth mono" text-anchor="middle" fill="var(--amber-400)">
            {{ r.accurate ? '相符' : '错误信念' }}
          </text>
        </g>
      </g>

      <!-- 空状态 -->
      <text v-if="rows.length === 0" :x="CX" :y="H / 2" class="kgraph__empty mono" text-anchor="middle">
        该过滤条件下没有认知记录
      </text>
    </svg>

    <div class="kgraph__panel">
      <div class="kgraph__tabs" role="tablist">
        <button
          v-for="(e, i) in edges"
          :key="i"
          class="kgraph__tab"
          :class="{ 'is-on': activeIndex === i }"
          role="tab"
          :aria-selected="activeIndex === i"
          type="button"
          @click="activeIndex = i"
        >
          <span class="mono">{{ e.observer.replace(/^(npc|cohort|org)_/, '') }}</span>
          <span class="kgraph__tab-sub mono">→ {{ e.subject.replace(/^(npc|cohort|org|store)_/, '') }}</span>
        </button>
      </div>

      <div v-if="active" class="kgraph__detail">
        <div class="kgraph__row">
          <span class="eyebrow">渠道</span>
          <span class="mono">{{ active.channel }}</span>
        </div>
        <div class="kgraph__row">
          <span class="eyebrow">信念</span>
          <span class="kgraph__belief" :class="{ 'is-wrong': !active.accurate }">{{ active.believed }}</span>
        </div>
        <div class="kgraph__row" v-if="showTruth">
          <span class="eyebrow">真相</span>
          <span class="kgraph__truth-text">{{ active.truth }}</span>
        </div>
        <div class="kgraph__row" v-else>
          <span class="eyebrow">证据</span>
          <span class="mono kgraph__ev">{{ active.evidence_refs.join(' · ') }}</span>
        </div>
        <p class="kgraph__hint">
          信念可错误；未打开「显示 World Truth」时，本面板只呈现该观察者的认知与证据引用——
          这正是玩家视角与创作者视角的边界。
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.kgraph {
  display: grid;
  gap: var(--sp-5);
}

.kgraph__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--sp-4);
}

.kgraph__title {
  font-size: 19px;
  margin-top: 2px;
}

.kgraph__controls {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  flex-wrap: wrap;
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

.toggle {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 11px;
  color: var(--ink-600);
  cursor: pointer;
  user-select: none;
}

.toggle input {
  appearance: none;
  width: 26px;
  height: 14px;
  border: 1px solid var(--line-strong);
  background: var(--ink-100);
  position: relative;
  cursor: pointer;
  transition: background var(--dur-fast) var(--ease-mech);
}

.toggle input::after {
  content: '';
  position: absolute;
  top: 1px;
  left: 1px;
  width: 10px;
  height: 10px;
  background: var(--ink-600);
  transition: transform var(--dur-fast) var(--ease-snap);
}

.toggle input:checked {
  background: rgba(224, 160, 74, 0.18);
  border-color: var(--amber-400);
}

.toggle input:checked::after {
  transform: translateX(12px);
  background: var(--amber-400);
}

.toggle input:checked + .mono {
  color: var(--amber-400);
}

.kgraph__svg {
  width: 100%;
  height: auto;
  overflow: visible;
}

.kgraph__plane {
  font-size: 9px;
  letter-spacing: 0.16em;
}

.kgraph__node {
  font-size: 10px;
  text-anchor: middle;
}

.kgraph__conf {
  font-size: 8.5px;
  fill: var(--ink-600);
  text-anchor: middle;
}

.kgraph__truth {
  font-size: 8.5px;
  letter-spacing: 0.08em;
}

.kgraph__empty {
  font-size: 12px;
  fill: var(--ink-600);
}

.kgraph__panel {
  display: grid;
  gap: var(--sp-4);
  border-top: 1px solid var(--line);
  padding-top: var(--sp-4);
}

.kgraph__tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.kgraph__tab {
  background: var(--ink-100);
  border: 1px solid var(--line);
  color: var(--ink-600);
  padding: 5px 10px;
  font-size: 11px;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 1px;
  transition: all var(--dur-fast) var(--ease-mech);
}

.kgraph__tab:hover {
  border-color: var(--line-strong);
  color: var(--ink-800);
}

.kgraph__tab.is-on {
  border-color: var(--teal-500);
  color: var(--teal-400);
  background: rgba(79, 209, 197, 0.08);
}

.kgraph__tab-sub {
  font-size: 9px;
  opacity: 0.7;
}

.kgraph__detail {
  display: grid;
  gap: var(--sp-3);
  max-width: 720px;
}

.kgraph__row {
  display: grid;
  grid-template-columns: 74px 1fr;
  gap: var(--sp-4);
  align-items: baseline;
}

.kgraph__row .mono {
  font-size: 12px;
  color: var(--ink-800);
}

.kgraph__belief {
  font-size: 14px;
  color: var(--ink-900);
  line-height: 1.55;
}

.kgraph__belief.is-wrong {
  color: var(--rose-400);
}

.kgraph__truth-text {
  font-size: 13px;
  color: var(--amber-400);
  line-height: 1.6;
}

.kgraph__ev {
  font-size: 11.5px;
  color: var(--ink-700);
}

.kgraph__hint {
  font-size: 11.5px;
  color: var(--ink-600);
  margin: 0;
  border-left: 2px solid var(--line-strong);
  padding-left: var(--sp-3);
  line-height: 1.7;
}
</style>
