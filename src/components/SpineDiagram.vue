<script setup lang="ts">
/**
 * SpineDiagram — 事件脊梁
 *
 * 已提交线（COMMITTED）是一条水平不变量；
 * 权威事件排在线的上方（世界事实），
 * 决策 / 验证 / 观察 / 诊断 / 干预全部沉在线下（审计与非权威），
 * 每条线下记录用折线连到它导致的那条事件上。
 * 这张图想说明的唯一一件事：持久状态只能由验证后原子提交的事件改变。
 */
import { computed, onUnmounted, ref, watch } from 'vue'
import type { WorldRecord } from '../types'
import { kindStyle } from '../lib/format'

const props = withDefaults(
  defineProps<{
    records: WorldRecord[]
    selectedId?: string | null
    hoveredId?: string | null
    tracedIds?: string[]
    height?: number
    compact?: boolean
    /** 探针自动扫描（首屏签名交互） */
    probe?: boolean
    /** 只展示这些 trace_id（首屏单链模式） */
    traceIds?: string[]
  }>(),
  {
    selectedId: null,
    hoveredId: null,
    height: 560,
    compact: false,
    probe: false,
    traceIds: () => []
  }
)

const emit = defineEmits<{
  (e: 'select', id: string): void
  (e: 'hover', id: string | null): void
  (e: 'probe', id: string | null): void
}>()

const W = 1000
const H = computed(() => props.height)
const PAD = 66
const committedY = computed(() => Math.round(H.value * 0.42))

interface NodePos {
  record: WorldRecord
  x: number
  y: number
  plane: 'above' | 'below'
  depth: number
  parent: string | null
}

const visible = computed(() => {
  let list = [...props.records].sort((a, b) => a.record_order - b.record_order)
  if (props.traceIds.length) {
    const set = new Set(props.traceIds)
    list = list.filter((r) => set.has(r.trace_id))
  }
  return list
})

const nodes = computed<NodePos[]>(() => {
  const list = visible.value
  const events = list.filter((r) => r.record_type === 'event')
  const others = list.filter((r) => r.record_type !== 'event')
  const byId = new Map(list.map((r) => [r.record_id, r]))
  const out: NodePos[] = []

  const span = Math.max(1, W - PAD * 2)
  const step = events.length > 1 ? span / (events.length - 1) : 0

  const eventX = new Map<string, number>()
  events.forEach((r, i) => {
    const x = events.length > 1 ? PAD + i * step : W / 2
    eventX.set(r.record_id, x)
    // 事件链在已提交线上方做轻微起伏，线本身保持水平
    const y = committedY.value - 46 - (i % 3) * 15
    out.push({ record: r, x, y, plane: 'above', depth: 0, parent: null })
  })

  const depthOf = (r: WorldRecord, seen = new Set<string>()): number => {
    if (!r.causation_id || seen.has(r.record_id)) return 0
    seen.add(r.record_id)
    const p = byId.get(r.causation_id)
    if (!p) return 0
    if (p.record_type === 'event') return 1
    return 1 + depthOf(p, seen)
  }

  const childCount = new Map<string, number>()
  for (const r of others) {
    const key = r.causation_id ?? 'root'
    childCount.set(key, (childCount.get(key) ?? 0) + 1)
  }
  const childIndex = new Map<string, number>()

  for (const r of others) {
    const parentId = r.causation_id
    const parent = parentId ? byId.get(parentId) : undefined
    const anchorId = parent ? parent.record_id : null
    const anchor = parent
      ? parent.record_type === 'event'
        ? parent.record_id
        : findEventAncestor(parent, byId)
      : null
    const baseX = anchor ? (eventX.get(anchor) ?? W / 2) : W / 2
    const idx = childIndex.get(anchorId ?? 'root') ?? 0
    childIndex.set(anchorId ?? 'root', idx + 1)
    const n = childCount.get(anchorId ?? 'root') ?? 1
    const spread = n > 1 ? 30 : 0
    const x = Math.min(W - PAD + 26, Math.max(PAD - 26, baseX + (idx - (n - 1) / 2) * spread))
    const depth = depthOf(r)
    out.push({
      record: r,
      x,
      y: committedY.value + 42 + depth * 44,
      plane: 'below',
      depth,
      parent: anchorId
    })
  }
  return out
})

function findEventAncestor(r: WorldRecord, byId: Map<string, WorldRecord>, seen = new Set<string>()): string | null {
  if (!r.causation_id || seen.has(r.record_id)) return null
  seen.add(r.record_id)
  const p = byId.get(r.causation_id)
  if (!p) return null
  if (p.record_type === 'event') return p.record_id
  return findEventAncestor(p, byId, seen)
}

const posById = computed(() => new Map(nodes.value.map((n) => [n.record.record_id, n])))

function linkPath(n: (typeof nodes.value)[number]): string {
  if (!n.parent) return ''
  const p = posById.value.get(n.parent)
  if (!p) return ''
  const midY = (p.y + n.y) / 2
  return `M${p.x} ${p.y} V${midY} H${n.x} V${n.y - 7}`
}

const chainPath = computed(() => {
  const above = nodes.value.filter((n) => n.plane === 'above')
  if (above.length < 2) return ''
  return above.map((n, i) => `${i === 0 ? 'M' : 'L'}${n.x} ${n.y}`).join(' ')
})

const traced = computed(() => new Set(props.tracedIds ?? []))

function opacityOf(n: NodePos): number {
  if (traced.value.size > 1 && !traced.value.has(n.record.record_id)) return 0.16
  return 1
}

function isSelected(n: NodePos): boolean {
  return props.selectedId === n.record.record_id
}

function isHovered(n: NodePos): boolean {
  return props.hoveredId === n.record.record_id
}

/* ------------------------------------------------------------- 探针 */

const probeX = ref<number | null>(null)
const probeId = ref<string | null>(null)
let raf = 0

onUnmounted(() => stopProbe())

function startProbe() {
  stopProbe()
  if (props.probe && !document.documentElement.classList.contains('rm')) {
    const t0 = performance.now()
    const duration = 5200
    const loop = (t: number) => {
      const p = ((t - t0) % duration) / duration
      probeX.value = PAD + p * (W - PAD * 2)
      const above = nodes.value.filter((n) => n.plane === 'above')
      let nearest: NodePos | null = null
      let best = Infinity
      for (const n of above) {
        const d = Math.abs(n.x - probeX.value!)
        if (d < best) {
          best = d
          nearest = n
        }
      }
      const id = nearest && best < 46 ? nearest.record.record_id : null
      if (id !== probeId.value) {
        probeId.value = id
        emit('probe', id)
      }
      raf = requestAnimationFrame(loop)
    }
    raf = requestAnimationFrame(loop)
  }
}

function stopProbe() {
  if (raf) cancelAnimationFrame(raf)
  raf = 0
  probeX.value = null
}

watch(
  () => [props.probe, props.records.length, props.traceIds.join(',')],
  () => {
    if (props.probe) startProbe()
    else stopProbe()
  },
  { immediate: true }
)


/* --------------------------------------------------------- 交互 */

function onKey(e: KeyboardEvent, id: string) {
  const list = visible.value
  const i = list.findIndex((r) => r.record_id === id)
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    emit('select', id)
    return
  }
  const forward = e.key === 'ArrowRight' || e.key === 'ArrowDown'
  const backward = e.key === 'ArrowLeft' || e.key === 'ArrowUp'
  if (!forward && !backward) return
  e.preventDefault()
  const target = forward ? list[Math.min(list.length - 1, i + 1)] : list[Math.max(0, i - 1)]
  if (!target) return
  // 先提交选择，再移动焦点；双 rAF + 同步兜底，确保在节流环境下也生效
  emit('select', target.record_id)
  const moveFocus = () => {
    const el = document.querySelector(`[data-spine-node="${target.record_id}"]`) as SVGGElement | null
    el?.focus()
  }
  if (typeof requestAnimationFrame === 'function') {
    requestAnimationFrame(() => requestAnimationFrame(moveFocus))
  }
  // 兜底：若两帧内焦点未移动（例如页面隐藏导致 rAF 暂停），立即同步执行
  window.setTimeout(() => {
    if (document.activeElement?.getAttribute('data-spine-node') !== target.record_id) moveFocus()
  }, 64)
}
</script>

<template>
  <svg
    class="spine"
    :viewBox="`0 0 ${W} ${H}`"
    :class="{ 'spine--compact': compact }"
    preserveAspectRatio="xMidYMid meet"
    role="group"
    aria-label="事件脊梁：已提交事实在线上方，审计与观察在线下方"
    @mouseleave="emit('hover', null)"
  >
    <defs>
      <linearGradient id="spine-committed" x1="0" y1="0" x2="1" y2="0">
        <stop offset="0%" stop-color="rgba(224,160,74,0.15)" />
        <stop offset="18%" stop-color="rgba(224,160,74,0.85)" />
        <stop offset="82%" stop-color="rgba(224,160,74,0.85)" />
        <stop offset="100%" stop-color="rgba(224,160,74,0.15)" />
      </linearGradient>
      <filter id="spine-glow" x="-60%" y="-60%" width="220%" height="220%">
        <feGaussianBlur stdDeviation="4" result="b" />
        <feMerge>
          <feMergeNode in="b" />
          <feMergeNode in="SourceGraphic" />
        </feMerge>
      </filter>
    </defs>

    <!-- 平面标签 -->
    <text :x="PAD - 46" :y="committedY - 86" class="spine__plane mono">世界事实 · 权威</text>
    <text :x="PAD - 46" :y="committedY + 26" class="spine__plane mono">审计 · 非权威</text>

    <!-- 已提交线 -->
    <line
      :x1="PAD - 34"
      :y1="committedY"
      :x2="W - PAD + 34"
      :y2="committedY"
      stroke="url(#spine-committed)"
      stroke-width="1.5"
    />
    <text :x="PAD - 46" :y="committedY - 8" class="spine__committed-label mono">已提交线</text>

    <!-- 事件刻度 -->
    <g>
      <line
        v-for="n in nodes.filter((n) => n.plane === 'above')"
        :key="'tick' + n.record.record_id"
        :x1="n.x"
        :y1="committedY - 5"
        :x2="n.x"
        :y2="committedY + 5"
        stroke="rgba(224,160,74,0.5)"
        stroke-width="1"
      />
    </g>

    <!-- 事件链 -->
    <path v-if="chainPath" :d="chainPath" class="spine__chain" fill="none" />

    <!-- 因果折线 -->
    <g class="spine__links">
      <path
        v-for="n in nodes.filter((n) => n.plane === 'below' && n.parent)"
        :key="'link' + n.record.record_id"
        :d="linkPath(n)"
        :stroke="kindStyle(n.record).accent"
        :opacity="opacityOf(n) * 0.55"
        fill="none"
      />
    </g>

    <!-- 节点 -->
    <g
      v-for="n in nodes"
      :key="n.record.record_id"
      class="spine__node"
      :class="{ 'is-selected': isSelected(n), 'is-below': n.plane === 'below' }"
      :data-spine-node="n.record.record_id"
      :data-testid="'spine-node-' + n.record.record_id"
      :opacity="opacityOf(n)"
      tabindex="0"
      role="button"
      :aria-label="`${kindStyle(n.record).label} ${n.record.record_id}`"
      @click="emit('select', n.record.record_id)"
      @mouseenter="emit('hover', n.record.record_id)"
      @focus="emit('hover', n.record.record_id)"
      @keydown="onKey($event, n.record.record_id)"
    >
      <!-- 命中区 -->
      <circle :cx="n.x" :cy="n.y" r="17" fill="transparent" />
      <!-- 形状 -->
      <g v-if="n.plane === 'above'" :transform="`translate(${n.x} ${n.y}) rotate(45)`">
        <rect
          :width="isSelected(n) ? 15 : 11"
          :height="isSelected(n) ? 15 : 11"
          :x="isSelected(n) ? -7.5 : -5.5"
          :y="isSelected(n) ? -7.5 : -5.5"
          :fill="isSelected(n) ? kindStyle(n.record).accent : 'var(--ink-100)'"
          :stroke="kindStyle(n.record).accent"
          stroke-width="1.4"
        />
      </g>
      <circle
        v-else
        :cx="n.x"
        :cy="n.y"
        :r="isSelected(n) ? 7.5 : 5.5"
        :fill="isSelected(n) ? kindStyle(n.record).accent : 'var(--ink-100)'"
        :stroke="kindStyle(n.record).accent"
        stroke-width="1.4"
      />
      <!-- 选中光环 -->
      <circle
        v-if="isSelected(n)"
        :cx="n.x"
        :cy="n.y"
        r="15"
        fill="none"
        :stroke="kindStyle(n.record).accent"
        stroke-width="1"
        opacity="0.5"
        filter="url(#spine-glow)"
      />
      <!-- 序号 -->
      <text :x="n.x" :y="n.y + (n.plane === 'above' ? -14 : 19)" class="spine__seq mono">
        R{{ String(n.record.record_order).padStart(2, '0') }}
      </text>
      <!-- 标签 -->
      <text
        v-if="!compact && (isSelected(n) || isHovered(n) || n.plane === 'above')"
        :x="n.x"
        :y="n.y + (n.plane === 'above' ? 26 : 32)"
        class="spine__label mono"
        :fill="kindStyle(n.record).accent"
      >
        {{ shortLabel(n) }}
      </text>
    </g>

    <!-- 探针 -->
    <g v-if="probe && probeX !== null">
      <line
        :x1="probeX"
        :y1="committedY - 96"
        :x2="probeX"
        :y2="committedY + 40"
        stroke="rgba(79,209,197,0.55)"
        stroke-width="1"
        stroke-dasharray="3 3"
      />
      <circle :cx="probeX" :cy="committedY - 96" r="3" fill="var(--teal-400)" />
    </g>
  </svg>
</template>

<script lang="ts">
/* 纯函数辅助：在模板中直接调用，避免为每帧生成计算属性 */
function shortLabel(n: { record: WorldRecord; plane: 'above' | 'below' }): string {
  const r = n.record
  if (r.record_type === 'event') {
    const name = r.event_type.replace(/([a-z])([A-Z])/g, '$1 $2')
    return name.length > 22 ? name.slice(0, 21) + '…' : name
  }
  const map: Record<string, string> = {
    agent_decision: 'Agent 决策',
    rule_validation: '规则验证',
    observation: '观察',
    runtime_diagnostic: '诊断',
    intervention: '干预'
  }
  return map[r.record_type] ?? r.record_type
}
</script>

<style scoped>
.spine {
  width: 100%;
  height: auto;
  display: block;
  overflow: visible;
}

.spine__plane {
  font-size: 9px;
  letter-spacing: 0.16em;
  fill: var(--ink-600);
  text-transform: uppercase;
}

.spine__committed-label {
  font-size: 9px;
  letter-spacing: 0.2em;
  fill: var(--amber-400);
}

.spine__chain {
  stroke: rgba(224, 160, 74, 0.55);
  stroke-width: 1.6;
  stroke-linejoin: round;
  stroke-linecap: round;
}

.spine__node {
  cursor: pointer;
  transition: opacity var(--dur-mid) var(--ease-mech);
}

.spine__node:hover .spine__seq,
.spine__node.is-selected .spine__seq {
  fill: var(--ink-900);
}

.spine__seq {
  font-size: 8.5px;
  fill: var(--ink-600);
  text-anchor: middle;
  transition: fill var(--dur-fast) var(--ease-mech);
}

.spine__label {
  font-size: 8.5px;
  text-anchor: middle;
  letter-spacing: 0.02em;
}

.spine--compact .spine__label {
  display: none;
}

@media (prefers-reduced-motion: reduce) {
  .spine__node {
    transition: none;
  }
}
</style>
