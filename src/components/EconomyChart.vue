<script setup lang="ts">
/**
 * EconomyChart — 复式记账与经济投影
 *
 * 三重口径必须分开：现金流（现金）≠ 净资产（资产−负债）≠ 流动性（可立即动用）。
 * 价格是带有效期的报价事件，不是一条平滑曲线——涨跌处画成台阶。
 */
import { computed, ref } from 'vue'
import type { EconomyPoint } from '../types'
import { fmtNum } from '../lib/format'
const props = defineProps<{
  series: EconomyPoint[]
}>()

const W = 1000
const H = 300
const PAD = { l: 56, r: 56, t: 26, b: 34 }

type Metric = 'cash' | 'net_worth' | 'price'
const metrics: { key: Metric; label: string; color: string; fmt: (n: number) => string }[] = [
  { key: 'cash', label: '家庭现金流（文）', color: 'var(--teal-400)', fmt: (n) => fmtNum(n) },
  { key: 'net_worth', label: '家庭净资产（文）', color: 'var(--amber-400)', fmt: (n) => fmtNum(n) },
  { key: 'price', label: '面包报价（文/个）', color: 'var(--violet-400)', fmt: (n) => `${n}` }
]

const enabled = ref<Record<Metric, boolean>>({ cash: true, net_worth: true, price: true })
const hoverDay = ref<number | null>(null)

const x = (day: number) => PAD.l + ((day - 1) / Math.max(1, props.series.length - 1)) * (W - PAD.l - PAD.r)

const scaleY = (v: number, metric: Metric) => {
  let min: number, max: number
  if (metric === 'price') {
    min = 11
    max = 14
  } else if (metric === 'net_worth') {
    min = 8600
    max = 9200
  } else {
    min = 2500
    max = 3300
  }
  const t = (v - min) / (max - min)
  return H - PAD.b - t * (H - PAD.t - PAD.b)
}

const paths = computed(() =>
  metrics
    .filter((m) => enabled.value[m.key])
    .map((m) => {
      const d = props.series
        .map((p, i) => {
          const px = x(p.day)
          const py = scaleY(m.key === 'price' ? p.price_bread : m.key === 'cash' ? p.household_cash : p.net_worth_household, m.key)
          return `${i === 0 ? 'M' : 'L'}${px.toFixed(1)} ${py.toFixed(1)}`
        })
        .join(' ')
      return { ...m, d }
    })
)

/** 报价是台阶：有效期内价格不变，到期后由新事件替换 */
const priceSteps = computed(() => {
  const steps: { x1: number; x2: number; y: number; price: number }[] = []
  let i = 0
  while (i < props.series.length) {
    const price = props.series[i].price_bread
    let j = i
    while (j + 1 < props.series.length && props.series[j + 1].price_bread === price) j++
    const last = j === props.series.length - 1
    steps.push({
      x1: x(props.series[i].day) - 7,
      // 最后一段延伸至右边界，表示"当前有效报价"
      x2: last ? W - PAD.r + 14 : x(props.series[j].day),
      y: scaleY(price, 'price'),
      price
    })
    i = j + 1
  }
  return steps
})

const markers = computed(() => [
  { day: 19, label: '工资到期', color: 'var(--amber-400)', kind: 'wage' },
  { day: 20, label: '欠薪登记', color: 'var(--rose-400)', kind: 'arrears' },
  { day: 27, label: '新纪元生效', color: 'var(--violet-400)', kind: 'epoch' }
])

const hovered = computed(() => {
  if (hoverDay.value == null) return null
  return props.series.find((p) => p.day === hoverDay.value) ?? null
})

function onMove(e: MouseEvent) {
  const svg = e.currentTarget as SVGSVGElement
  const rect = svg.getBoundingClientRect()
  const px = ((e.clientX - rect.left) / rect.width) * W
  let best: number | null = null
  let bd = Infinity
  for (const p of props.series) {
    const d = Math.abs(x(p.day) - px)
    if (d < bd) {
      bd = d
      best = p.day
    }
  }
  hoverDay.value = bd < 60 ? best : null
}
</script>

<template>
  <div class="econ">
    <header class="econ__head">
      <div>
        <span class="eyebrow">LEDGER · 复式分录与投影</span>
        <h3 class="econ__title">现金流 ≠ 净资产 ≠ 流动性</h3>
      </div>
      <div class="econ__legend">
        <label v-for="m in metrics" :key="m.key" class="econ__legend-item">
          <input v-model="enabled[m.key]" type="checkbox" />
          <span class="swatch" :style="{ background: m.color }" />
          <span class="mono">{{ m.label }}</span>
        </label>
      </div>
    </header>

    <svg
      class="econ__svg"
      :viewBox="`0 0 ${W} ${H}`"
      role="img"
      aria-label="经济投影折线图"
      @mousemove="onMove"
      @mouseleave="hoverDay = null"
    >
      <!-- 网格 -->
      <g class="econ__grid">
        <line v-for="i in 4" :key="'h' + i" :x1="PAD.l" :y1="PAD.t + i * ((H - PAD.t - PAD.b) / 4)" :x2="W - PAD.r" :y2="PAD.t + i * ((H - PAD.t - PAD.b) / 4)" />
      </g>

      <!-- 世界日轴 -->
      <g class="econ__axis mono">
        <text v-for="(p, i) in series" :key="'x' + p.day" v-show="i % 4 === 0" :x="x(p.day)" :y="H - PAD.b + 16" text-anchor="middle">
          D{{ p.day }}
        </text>
      </g>

      <!-- 事件标记 -->
      <g v-for="m in markers" :key="m.kind">
        <line :x1="x(m.day)" :y1="PAD.t - 8" :x2="x(m.day)" :y2="H - PAD.b" :stroke="m.color" stroke-width="1" stroke-dasharray="3 4" opacity="0.6" />
        <text :x="x(m.day)" :y="PAD.t - 14" :fill="m.color" class="mono econ__marker" text-anchor="middle">{{ m.label }}</text>
      </g>

      <!-- 报价台阶 -->
      <g v-if="enabled.price">
        <line
          v-for="(s, i) in priceSteps"
          :key="'step' + i"
          :x1="s.x1"
          :y1="s.y"
          :x2="s.x2"
          :y2="s.y"
          stroke="var(--violet-400)"
          stroke-width="2"
          stroke-linecap="butt"
        />
        <circle
          v-for="(s, i) in priceSteps"
          :key="'stepdot' + i"
          :cx="s.x2"
          :cy="s.y"
          r="3"
          fill="var(--violet-400)"
        />
      </g>

      <!-- 折线 -->
      <path v-for="p in paths" :key="p.key" :d="p.d" fill="none" :stroke="p.color" stroke-width="1.8" stroke-linejoin="round" />

      <!-- 悬停 -->
      <g v-if="hovered">
        <line :x1="x(hovered.day)" :y1="PAD.t" :x2="x(hovered.day)" :y2="H - PAD.b" stroke="var(--ink-500)" stroke-width="1" />
        <g
          v-for="m in metrics.filter((mm) => enabled[mm.key])"
          :key="'hv' + m.key"
          :transform="`translate(${x(hovered.day) + 10} ${scaleY(m.key === 'price' ? hovered.price_bread : m.key === 'cash' ? hovered.household_cash : hovered.net_worth_household, m.key) - 4})`"
        >
          <rect x="0" y="-11" :width="m.fmt(m.key === 'price' ? hovered.price_bread : m.key === 'cash' ? hovered.household_cash : hovered.net_worth_household).length * 7 + 14" height="18" :fill="'var(--ink-200)'" :stroke="m.color" stroke-width="0.8" />
          <text x="7" y="2" class="mono econ__tip" :fill="m.color">
            {{ m.fmt(m.key === 'price' ? hovered.price_bread : m.key === 'cash' ? hovered.household_cash : hovered.net_worth_household) }}
          </text>
        </g>
      </g>
    </svg>

    <div class="econ__notes">
      <div class="econ__note">
        <span class="eyebrow">D19–D20</span>
        <p>宗门度支额度用尽 → <span class="warn">WageArrearsRecorded</span>。应收 1800 计入林巧资产侧，但不改变她的<b>现金</b>；净资产上升的同时<b>流动性</b>没有改善。</p>
      </div>
      <div class="econ__note">
        <span class="eyebrow">D28</span>
        <p>新纪元生效后旧历史不变：D1–D28 的曲线与报价台阶全部保留，价格仍按各自有效期分段读取。</p>
      </div>
      <div class="econ__note">
        <span class="eyebrow">口径</span>
        <p>名义口径，未做地区/年份校准。禁止从自然语言直接凭空改价、发工资或创造资产。</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.econ {
  display: grid;
  gap: var(--sp-4);
}

.econ__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--sp-4);
}

.econ__title {
  font-size: 19px;
  margin-top: 2px;
}

.econ__legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-4);
}

.econ__legend-item {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 11px;
  color: var(--ink-600);
  cursor: pointer;
}

.econ__legend-item input {
  appearance: none;
  width: 12px;
  height: 12px;
  border: 1px solid var(--line-strong);
  background: var(--ink-100);
  cursor: pointer;
  position: relative;
}

.econ__legend-item input:checked {
  border-color: var(--ink-700);
  background: var(--ink-300);
}

.econ__legend-item input:checked::after {
  content: '✓';
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  font-size: 9px;
  color: var(--ink-900);
}

.swatch {
  width: 14px;
  height: 2px;
}

.econ__svg {
  width: 100%;
  height: auto;
  overflow: visible;
  background: linear-gradient(to bottom, rgba(20, 26, 38, 0.5), transparent);
  border: 1px solid var(--line);
}

.econ__grid line {
  stroke: var(--line);
  stroke-width: 1;
}

.econ__axis text {
  font-size: 9.5px;
  fill: var(--ink-600);
}

.econ__marker {
  font-size: 8.5px;
  letter-spacing: 0.06em;
}

.econ__tip {
  font-size: 9.5px;
}

.econ__notes {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--sp-4);
  border-top: 1px solid var(--line);
  padding-top: var(--sp-4);
}

.econ__note p {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--ink-700);
  line-height: 1.7;
}

.econ__note b {
  color: var(--ink-900);
  font-weight: 600;
}

.econ__note .warn {
  color: var(--rose-400);
}
</style>
