<script setup lang="ts">
/**
 * InspectorWorkspace — 观测工作空间
 *
 * 从原 App.vue 的六区块整体提取而来：事件脊梁、规则纪元、认知、经济、
 * 因果/记录探针、实体与扩展包清单，语义与示例事实全部保留。
 * 与 StoryWorkspace 并列，可进可退。
 */
import { computed, onMounted, ref, watch } from 'vue'
import SpineDiagram from './SpineDiagram.vue'
import RecordInspector from './RecordInspector.vue'
import EpochStrata from './EpochStrata.vue'
import KnowledgeGraph from './KnowledgeGraph.vue'
import EconomyChart from './EconomyChart.vue'
import EntityGrid from './EntityGrid.vue'
import {
  META,
  RULESET_HASH_E0,
  RULESET_HASH_E1,
  branches,
  economySeries,
  entities,
  knowledgeEdges,
  marketQuotes,
  packs,
  records,
  snapshot,
  whyNot
} from '../data/world'
import { allKinds, fmtFullTime, shortHash } from '../lib/format'
import { usePrefersReducedMotion, useScrollProgress } from '../composables'
import type { WorldRecord } from '../types'

usePrefersReducedMotion()
const scrollProgress = useScrollProgress()

/** 首屏三条不变量（原 App.vue 的非 setup 脚本，随六区块一起迁移） */
const invariants = [
  { no: 'Ⅰ', text: '持久状态只能由验证后原子提交的事件改变。' },
  { no: 'Ⅱ', text: '世界不依赖玩家行动也能演化。' },
  { no: 'Ⅲ', text: '相同历史、规则和已记录的非确定性结果可恢复同一状态。' }
]

/* ------------------------------------------- 从叙事引用进入的定位 */

const props = withDefaults(
  defineProps<{
    /** 叙事流点击 record_id 时带入，用于选中并展开探针 */
    focusRecordId?: string | null
  }>(),
  { focusRecordId: null }
)

const emit = defineEmits<{
  (e: 'backToStory'): void
}>()

watch(
  () => props.focusRecordId,
  (id) => {
    if (id) select(id)
  }
)

/* ------------------------------------------------------- 选中与追溯 */

const selectedId = ref<string | null>('rec_0007')
const hoveredId = ref<string | null>(null)
const probedId = ref<string | null>(null)

const byId = computed(() => new Map(records.map((r) => [r.record_id, r])))
const selected = computed<WorldRecord | null>(() => (selectedId.value ? byId.value.get(selectedId.value) ?? null : null))
const kindOf = (id: string) => allKinds[byId.value.get(id)!.record_type]

const childrenOf = (id: string) => records.filter((r) => r.causation_id === id)
const parentOf = (r: WorldRecord | null) => (r?.causation_id ? byId.value.get(r.causation_id) ?? null : null)

const parent = computed(() => parentOf(selected.value))
const children = computed(() => (selected.value ? childrenOf(selected.value.record_id) : []))

/** 高亮整条因果链 */
const tracedIds = computed(() => {
  const r = selected.value
  if (!r) return []
  const set = new Set<string>([r.record_id])
  let cur: WorldRecord | undefined = r
  while (cur?.causation_id) {
    const p = byId.value.get(cur.causation_id)
    if (!p) break
    set.add(p.record_id)
    cur = p
  }
  for (const c of childrenOf(r.record_id)) set.add(c.record_id)
  return [...set]
})

/** 首屏只展示一条链：面包交易 */
const heroTraceIds = ['tr_txn_bread_001']
const heroRecords = computed(() => records.filter((r) => heroTraceIds.includes(r.trace_id)))

const heroFocusId = computed(() => probedId.value ?? hoveredId.value ?? selectedId.value)

function select(id: string) {
  selectedId.value = id
}

function onProbe(id: string | null) {
  probedId.value = id
}

/* ------------------------------------------------------------ 统计 */

const counts = computed(() => {
  const c: Record<string, number> = {}
  for (const r of records) c[r.record_type] = (c[r.record_type] ?? 0) + 1
  return c
})

const maxRecordOrder = computed(() => Math.max(...records.map((r) => r.record_order)))
const maxEventSequence = computed(() => Math.max(...records.map((r) => r.event_sequence ?? 0)))
const eventCount = computed(() => records.filter((r) => r.record_type === 'event').length)

const totalPostings = computed(
  () => records.reduce((s, r) => (r.record_type === 'event' ? s + (r.postings?.length ?? 0) : s), 0)
)

/* ------------------------------------------------------------ 视图 */

type Section = 'spine' | 'epoch' | 'knowledge' | 'economy' | 'why' | 'packs'
const sections: { id: Section; label: string; index: string }[] = [
  { id: 'spine', label: '事件脊梁', index: '01' },
  { id: 'epoch', label: '规则纪元', index: '02' },
  { id: 'knowledge', label: '认知与信念', index: '03' },
  { id: 'economy', label: '经济账本', index: '04' },
  { id: 'why', label: '为什么 / 没为什么', index: '05' },
  { id: 'packs', label: '扩展包清单', index: '06' }
]

const activeSection = ref<Section>('spine')

onMounted(() => {
  const els = sections.map((s) => document.getElementById('sec-' + s.id)).filter(Boolean) as HTMLElement[]
  const io = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (e.isIntersecting) activeSection.value = e.target.id.replace('sec-', '') as Section
      }
    },
    { rootMargin: '-40% 0px -50% 0px' }
  )
  els.forEach((el) => io.observe(el))
})

/* --------------------------------------------------------- 探针节拍 */

const probeEnabled = ref(true)
watch(
  () => activeSection.value,
  (v) => {
    probeEnabled.value = v === 'spine'
  }
)

/* ------------------------------------------------------------ 时钟 */

const clockLabel = computed(() => fmtFullTime(snapshot.world_time))
const liveTick = ref(0)
onMounted(() => {
  window.setInterval(() => liveTick.value++, 1000)
})
const probeCursor = computed(() => {
  const base = 12
  return String((base + (liveTick.value % 8) * 3) % 100).padStart(2, '0')
})
</script>

<template>
  <div class="app">
    <div class="grain" aria-hidden="true" />
    <div class="scan" aria-hidden="true" />

    <div class="rail" aria-hidden="true">
      <div class="rail__fill" :style="{ transform: `scaleX(${scrollProgress})` }" />
    </div>

    <div class="backbar">
      <button class="backbar__btn" type="button" @click="emit('backToStory')">
        <span aria-hidden="true">←</span> 返回叙事流
      </button>
      <span class="backbar__note mono">观测台为演示权限视角：叙事流里看不到这里的完整字段</span>
    </div>

    <header class="topbar">
      <div class="topbar__inner">
        <div class="brand">
          <span class="brand__mark" aria-hidden="true">
            <span class="brand__mark-line" />
          </span>
          <span class="brand__text">
            <strong>CoreRP</strong>
            <span class="mono">世界观测台 · World Inspector</span>
          </span>
        </div>

        <nav class="topnav" aria-label="区块导航">
          <a
            v-for="s in sections"
            :key="s.id"
            :href="'#sec-' + s.id"
            class="topnav__link"
            :class="{ 'is-active': activeSection === s.id }"
          >
            <span class="mono topnav__idx">{{ s.index }}</span>
            <span>{{ s.label }}</span>
          </a>
        </nav>

        <div class="topbar__status">
          <span class="status mono">
            <span class="status__pulse" :class="{ 'is-idle': !probeEnabled }" aria-hidden="true" />
            {{ probeEnabled ? 'PROBE ACTIVE' : 'IDLE' }}
          </span>
          <span class="mono topbar__seq">
            event {{ String(maxEventSequence).padStart(2, '0') }} · record {{ String(maxRecordOrder).padStart(2, '0') }}
          </span>
        </div>
      </div>
    </header>

    <main>
<!-- ================================================ 首屏 -->
<section class="hero" id="hero">
  <div class="hero__grid">
    <!-- 左：定位与三条不变量 -->
    <div class="hero__intro">
      <p class="eyebrow hero__eyebrow">{{ META.doc }} · 契约可视化演示</p>
      <h1 class="hero__title">
        <span class="hero__title-line">世界不是被讲出来的，</span>
        <span class="hero__title-line hero__title-line--2">是被<em>提交</em>出来的。</span>
      </h1>
      <p class="hero__lede">
        CoreRP 是一个 AI 原生持久化世界运行平台。玩家只是其中一个参与者——
        出门遭遇源于已有世界状态，平凡无事是被允许的结果。
        这个页面把 M0 必须冻结的契约摊开给你看：一条事件脊梁、两套规则纪元、五种认知偏差。
      </p>

      <ul class="invariants">
        <li v-for="(inv, i) in invariants" :key="i" class="invariant">
          <span class="invariant__no mono">{{ inv.no }}</span>
          <span class="invariant__text">{{ inv.text }}</span>
        </li>
      </ul>

      <div class="hero__actions">
        <a class="btn btn--primary" href="#sec-spine">进入观测台</a>
        <button class="btn" type="button" @click="select('rec_0020')">看一次欠薪如何被记录</button>
      </div>
    </div>

    <!-- 右：世界状态读数 -->
    <aside class="hero__readout" aria-label="世界实例状态">
      <div class="readout">
        <div class="readout__head">
          <span class="eyebrow">INSTANCE</span>
          <span class="mono readout__id">{{ snapshot.instance_id }}</span>
        </div>
        <dl class="readout__rows mono">
          <div><dt>branch</dt><dd>{{ snapshot.branch_id }}</dd></div>
          <div><dt>world_time</dt><dd class="readout__time">{{ clockLabel }}</dd></div>
          <div><dt>uptime</dt><dd>{{ snapshot.uptime_days }} 世界日</dd></div>
          <div><dt>epoch</dt><dd>1 · {{ shortHash(RULESET_HASH_E1) }}</dd></div>
          <div><dt>cursor</dt><dd>{{ snapshot.scheduler_cursor }}</dd></div>
          <div><dt>outbox</dt><dd :class="{ 'is-warn': snapshot.outbox_pending > 0 }">{{ snapshot.outbox_pending }} pending</dd></div>
        </dl>
        <div class="readout__counts">
          <div v-for="(c, k) in counts" :key="k" class="rcount" :style="{ '--accent': allKinds[k as keyof typeof allKinds].accent }">
            <span class="rcount__n mono">{{ c }}</span>
            <span class="rcount__l">{{ allKinds[k as keyof typeof allKinds].label }}</span>
          </div>
        </div>
      </div>
    </aside>
  </div>

  <!-- 首屏主角：单链脊梁 -->
  <div class="hero__spine">
    <div class="hero__spine-head">
      <span class="eyebrow">TRACE · tr_txn_bread_001 · 林巧的两个面包</span>
      <p class="hero__spine-note">
        探针正在扫描已提交线。线上方是世界事实，线下方是让它成立的全部审计过程。
      </p>
    </div>
    <SpineDiagram
      :records="heroRecords"
      :selected-id="heroFocusId"
      :hovered-id="hoveredId"
      :traced-ids="[]"
      :height="300"
      :probe="probeEnabled"
      :trace-ids="heroTraceIds"
      @select="select"
      @hover="(id) => (hoveredId = id)"
      @probe="onProbe"
    />
    <div class="hero__spine-foot mono">
      <span>◆ 已提交事件 {{ eventCount }}</span>
      <span>◇ 审计记录 {{ records.length - eventCount }}</span>
      <span>分录 {{ totalPostings }} 笔</span>
      <span>世界时间与服务器时间严格分离</span>
    </div>
  </div>
</section>

<!-- ============================================ 01 事件脊梁 -->
<section class="section" id="sec-spine">
  <div class="section__head">
    <span class="section__index mono">01</span>
    <div>
      <h2 class="section__title">事件脊梁</h2>
      <p class="section__sub">
        持久状态只能由验证后原子提交的事件改变。所以整张图只有一个主角：
        那条水平的<b>已提交线</b>。线以上是权威事实，线以下全部沉入审计层——
        Agent 决策、规则验证、观察记录、运行诊断、干预记录都不具备事实效力。
      </p>
    </div>
  </div>

  <div class="spine-layout">
    <div class="spine-layout__main">
      <div class="spine-layout__toolbar">
        <div class="legend">
          <span v-for="(k, key) in allKinds" :key="key" class="legend__item" :style="{ '--accent': k.accent }">
            <span class="legend__glyph">{{ k.glyph }}</span>
            <span class="legend__label">{{ k.label }}</span>
            <span class="legend__auth mono">{{ k.authority }}</span>
          </span>
        </div>
        <div class="spine-layout__hint mono">
          hover / click · ←→ 浏览 · 当前 record {{ selected ? String(selected.record_order).padStart(2, '0') : '--' }}
        </div>
      </div>

      <SpineDiagram
        :records="records"
        :selected-id="selectedId"
        :hovered-id="hoveredId"
        :traced-ids="tracedIds"
        :height="620"
        @select="select"
        @hover="(id) => (hoveredId = id)"
      />

      <div class="chain" v-if="selected">
        <span class="eyebrow">当前追溯链</span>
        <ol class="chain__list mono">
          <li v-for="id in tracedIds" :key="id" :class="{ 'is-sel': id === selectedId }">
            <button type="button" @click="select(id)">
              <span class="chain__seq">R{{ String(byId.get(id)!.record_order).padStart(2, '0') }}</span>
              <span class="chain__type" :style="{ color: kindOf(id).accent }">
                {{ kindOf(id).label }}
              </span>
              <span class="chain__id">{{ id }}</span>
            </button>
          </li>              </ol>
      </div>
    </div>

    <aside class="spine-layout__probe">
      <div class="probe__head">
        <span class="eyebrow">PROBE · 记录探针</span>
        <span class="mono probe__cursor">{{ probeCursor }}%</span>
      </div>
      <RecordInspector
        :record="selected"
        :parent="parent"
        :children="children"
        @focus-record="select"
      />
    </aside>
  </div>
</section>

<!-- ============================================ 02 规则纪元 -->
<section class="section" id="sec-epoch">
  <div class="section__head">
    <span class="section__index mono">02</span>
    <div>
      <h2 class="section__title">规则纪元与版本锁</h2>
      <p class="section__sub">
        同一分支不是"永远只有一个 ruleset_hash"。版本锁按事件序号分段不可变，
        每段有独立完整锁文件与哈希；升级产生新纪元，并留下可读取的边界事件。
      </p>
    </div>
  </div>

  <div class="epoch-layout">
    <EpochStrata
      v-for="b in branches"
      :key="b.branch_id"
      :branch="b"
      :records="records"
      :max-event-sequence="maxEventSequence"
    />
    <div class="epoch-note">
      <span class="eyebrow">边界事件 · rec_0025</span>
      <pre class="epoch-note__code mono">LawActivationEvent {
old_epoch: 0,  new_epoch: 1,
old_ruleset_hash: "{{ RULESET_HASH_E0 }}",
new_ruleset_hash: "{{ RULESET_HASH_E1 }}",
pack_lock_change: "add transcendent_awakening@0.1.0",
migration_ref: "mig_awakening_001"
}
// 激活事件是 event_seq 12，属于旧纪元；新纪元从 event_seq 13 开始。
// 无法原子切换或兼容性不明时：拒绝热激活，要求分支。</pre>
      <p class="epoch-note__text">
        分支 <span class="mono">br_what_if</span> 在 event_seq 10 分叉：如果林巧当时选择动用储蓄付租，
        历史在这里分开，两条线共享之前的全部事件。
      </p>
    </div>
  </div>
</section>

<!-- ========================================= 03 认知与信念 -->
<section class="section" id="sec-knowledge">
  <div class="section__head">
    <span class="eyebrow">03</span>
    <div>
      <h2 class="section__title">谁相信什么，与什么是真的</h2>
      <p class="section__sub">
        World Truth 是已提交事实；Knowledge 是角色接触到的信息；Belief 可错误。
        错误的信念必须被显式建模——否则"全知 NPC"会毁掉整场模拟。
      </p>
    </div>
  </div>
  <KnowledgeGraph :edges="knowledgeEdges" />
</section>

<!-- ============================================ 04 经济账本 -->
<section class="section" id="sec-economy">
  <div class="section__head">
    <span class="section__index mono">04</span>
    <div>
      <h2 class="section__title">经济账本</h2>
      <p class="section__sub">
        交易提交在同一事务内验证买方支付能力、商品库存、价格有效期、权限及预期版本，
        然后原子写入资金分录、库存转移和事件批次。重复命令通过幂等键只结算一次。
      </p>
    </div>
  </div>

  <EconomyChart :series="economySeries" />

  <div class="quotes">
    <span class="eyebrow">MARKET QUOTES · 报价带有效期</span>
    <div class="quotes__grid">
      <article v-for="q in marketQuotes" :key="q.quote_id" class="quote" :class="{ 'is-superseded': q.quote_id === 'mq_bread_001' }">
        <header class="quote__head">
          <span class="mono quote__id">{{ q.quote_id }}</span>
          <span class="quote__kind mono">{{ q.kind }}</span>
        </header>
        <h4 class="quote__sku">{{ q.sku }}</h4>
        <p class="quote__price mono">
          {{ q.price_minor }}<span class="quote__unit">{{ q.currency_id }} / {{ q.base_unit }}</span>
          <span v-if="q.tax_included" class="quote__tax">含税</span>
        </p>
        <dl class="quote__meta mono">
          <div><dt>seller</dt><dd>{{ q.seller }}</dd></div>
          <div><dt>region</dt><dd>{{ q.region }}</dd></div>
          <div><dt>valid</dt><dd>{{ q.valid_from.slice(5, 10) }} → {{ q.valid_until.slice(5, 10) }}</dd></div>
        </dl>
        <p v-if="q.quote_id === 'mq_bread_001'" class="quote__note">已被 mq_bread_002 取代（记录保留，不删除）</p>
      </article>
    </div>
  </div>
</section>

<!-- ======================================== 05 为什么 / 没为什么 -->
<section class="section" id="sec-why">
  <div class="section__head">
    <span class="section__index mono">05</span>
    <div>
      <h2 class="section__title">为什么发生，为什么没发生</h2>
      <p class="section__sub">
        单靠事件账本无法回答"为什么没发生"——还需要关键候选拒绝与调度跳过的记录。
        下面是 World Inspector 对这个 33 日世界的回答，每条都指向具体证据。
      </p>
    </div>
  </div>

  <div class="why">
    <article v-for="(w, i) in whyNot" :key="i" class="why__item" :class="'is-' + w.severity">
      <div class="why__marker" aria-hidden="true" />
      <div class="why__body">
        <h3 class="why__q">{{ w.question }}</h3>
        <p class="why__a">{{ w.answer }}</p>
        <div class="why__refs mono">
          <span v-for="r in w.evidence_refs" :key="r" class="why__ref">{{ r }}</span>
        </div>
      </div>
    </article>
  </div>
</section>

<!-- ========================================== 06 扩展包清单 -->
<section class="section" id="sec-packs">
  <div class="section__head">
    <span class="section__index mono">06</span>
    <div>
      <h2 class="section__title">扩展包清单</h2>
      <p class="section__sub">
        换世界不改内核。世界包定义初始世界，机制包定义可组合规则，
        内容包补充内容，叙事包只控制表达，适配器通过受控接口接入。
      </p>
    </div>
  </div>

  <EntityGrid :entities="entities" />

  <div class="packs">
    <table class="packtable">
      <thead>
        <tr>
          <th>包</th>
          <th>类别</th>
          <th>版本</th>
          <th>状态</th>
          <th>能力</th>
          <th class="mono">schema / content hash</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="p in packs" :key="p.id">
          <td class="mono packs__id">{{ p.id }}</td>
          <td><span class="pill" :class="'pill--' + p.kind">{{ p.kind }}</span></td>
          <td class="mono">{{ p.version }}</td>
          <td><span class="pill pill--state">{{ p.state }}</span></td>
          <td class="packs__caps mono">{{ p.capabilities.join(' · ') }}</td>
          <td class="mono packs__hash">{{ shortHash(p.schema_hash) }} / {{ shortHash(p.content_hash) }}</td>
        </tr>
      </tbody>
    </table>
  </div>

  <div class="snap">
    <span class="eyebrow">同步投影 · 只由 Commit 更新，可从权威事实重建</span>
    <div class="snap__grid">
      <div class="snap__col">
        <h4 class="snap__h">账户</h4>
        <ul class="mono">
          <li v-for="a in snapshot.projections.accounts" :key="a.owner + a.type">
            <span class="snap__k">{{ a.owner }}</span>
            <span class="snap__t">{{ a.type }}</span>
            <span class="snap__v">{{ a.balance_minor.toLocaleString('en-US') }}</span>
          </li>
        </ul>
      </div>
      <div class="snap__col">
        <h4 class="snap__h">库存</h4>
        <ul class="mono">
          <li v-for="iv in snapshot.projections.inventory" :key="iv.sku">
            <span class="snap__k">{{ iv.sku }}</span>
            <span class="snap__t">{{ iv.seller }}</span>
            <span class="snap__v" :class="{ 'is-low': iv.quantity_minor < 15 }">
              {{ iv.quantity_minor }} {{ iv.base_unit }}
            </span>
          </li>
        </ul>
      </div>
    </div>
  </div>
</section>
    </main>

    <footer class="footer">
      <div class="footer__inner">
        <p class="footer__disc">
          {{ META.disclaimer }}
        </p>
        <div class="footer__meta mono">
          <span>Vue 3 · TypeScript · Vite</span>
          <span>IBM Plex Mono / Sans（自托管 latin 子集）</span>
          <span>数据为构造示例 · ruleset_hash 为演示值</span>
        </div>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.app {
  position: relative;
  z-index: 2;
}

/* ------------------------------------------------------------- 轨 */

.rail {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: 2px;
  background: var(--ink-200);
  z-index: 50;
}

.rail__fill {
  height: 100%;
  background: linear-gradient(to right, var(--amber-500), var(--amber-400));
  transform-origin: left;
  transform: scaleX(0);
}

/* ---------------------------------------------------------- 顶栏 */

.topbar {
  position: sticky;
  top: 0;
  z-index: 40;
  background: rgba(6, 8, 13, 0.86);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--line);
}

.topbar__inner {
  max-width: var(--shell-max);
  margin: 0 auto;
  padding: var(--sp-3) var(--sp-5);
  display: flex;
  align-items: center;
  gap: var(--sp-5);
  justify-content: space-between;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: none;
}

.brand__mark {
  width: 22px;
  height: 22px;
  border: 1px solid var(--amber-400);
  display: grid;
  place-items: center;
  position: relative;
}

.brand__mark-line {
  width: 10px;
  height: 1px;
  background: var(--amber-400);
}

.brand__text {
  display: grid;
  line-height: 1.2;
}

.brand__text strong {
  font-family: var(--font-display);
  font-size: 14px;
  letter-spacing: 0.06em;
  color: var(--ink-900);
}

.brand__text .mono {
  font-size: 9.5px;
  color: var(--ink-600);
  letter-spacing: 0.08em;
}

.topnav {
  display: flex;
  gap: 2px;
  flex: 1;
  justify-content: center;
  overflow-x: auto;
  scrollbar-width: none;
}

.topnav::-webkit-scrollbar {
  display: none;
}

.topnav__link {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  padding: 5px 9px;
  font-size: 12px;
  color: var(--ink-600);
  border: 0;
  border-bottom: 1px solid transparent;
  white-space: nowrap;
  transition: all var(--dur-fast) var(--ease-mech);
}

.topnav__link:hover {
  color: var(--ink-800);
}

.topnav__link.is-active {
  color: var(--amber-400);
  border-bottom-color: var(--amber-400);
}

.topnav__idx {
  font-size: 9px;
  opacity: 0.6;
}

.topbar__status {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  flex: none;
}

.status {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 9.5px;
  letter-spacing: 0.12em;
  color: var(--teal-400);
}

.status__pulse {
  width: 6px;
  height: 6px;
  background: var(--teal-400);
  animation: pulse 1.6s var(--ease-mech) infinite;
}

.status__pulse.is-idle {
  background: var(--ink-500);
  animation: none;
}

@keyframes pulse {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.35;
    transform: scale(0.8);
  }
}

.topbar__seq {
  font-size: 10px;
  color: var(--ink-600);
}

/* ---------------------------------------------------------- 首屏 */

.hero {
  max-width: var(--shell-max);
  margin: 0 auto;
  padding: var(--sp-8) var(--sp-5) var(--sp-7);
}

.hero__grid {
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(300px, 0.85fr);
  gap: var(--sp-7);
  align-items: start;
}

.hero__eyebrow {
  margin: 0 0 var(--sp-4);
}

.hero__title {
  font-size: clamp(30px, 4.4vw, 52px);
  line-height: 1.12;
  letter-spacing: -0.03em;
  font-weight: 600;
  margin: 0 0 var(--sp-5);
}

.hero__title-line {
  display: block;
}

.hero__title-line--2 em {
  font-style: normal;
  color: var(--amber-400);
  position: relative;
}

.hero__title-line--2 em::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0.06em;
  height: 2px;
  background: var(--amber-400);
  opacity: 0.35;
}

.hero__lede {
  font-size: 15.5px;
  line-height: 1.75;
  color: var(--ink-700);
  max-width: 62ch;
  margin: 0 0 var(--sp-5);
}

.invariants {
  list-style: none;
  margin: 0 0 var(--sp-5);
  padding: 0;
  display: grid;
  gap: 0;
  border-top: 1px solid var(--line);
}

.invariant {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: var(--sp-4);
  align-items: baseline;
  padding: 10px 0;
  border-bottom: 1px solid var(--line);
}

.invariant__no {
  font-size: 12px;
  color: var(--amber-400);
  letter-spacing: 0.08em;
}

.invariant__text {
  font-size: 13.5px;
  color: var(--ink-800);
  line-height: 1.6;
}

.hero__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-3);
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--line-strong);
  background: none;
  color: var(--ink-800);
  font-size: 12.5px;
  padding: 9px 16px;
  letter-spacing: 0.02em;
  transition: all var(--dur-fast) var(--ease-mech);
}

.btn:hover {
  border-color: var(--ink-600);
  color: var(--ink-900);
}

.btn--primary {
  border-color: var(--amber-400);
  color: var(--amber-400);
  background: rgba(224, 160, 74, 0.07);
}

.btn--primary:hover {
  background: rgba(224, 160, 74, 0.14);
  color: var(--amber-400);
}

/* --------------------------------------------------- 右侧读数 */

.readout {
  border: 1px solid var(--line-strong);
  background: linear-gradient(to bottom, var(--ink-100), var(--ink-050));
  padding: var(--sp-4);
  display: grid;
  gap: var(--sp-4);
  position: relative;
}

.readout::before {
  content: '';
  position: absolute;
  top: -1px;
  left: -1px;
  width: 14px;
  height: 14px;
  border-top: 2px solid var(--amber-400);
  border-left: 2px solid var(--amber-400);
}

.readout::after {
  content: '';
  position: absolute;
  bottom: -1px;
  right: -1px;
  width: 14px;
  height: 14px;
  border-bottom: 2px solid var(--amber-400);
  border-right: 2px solid var(--amber-400);
}

.readout__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--sp-3);
  padding-bottom: var(--sp-3);
  border-bottom: 1px solid var(--line);
}

.readout__id {
  font-size: 11px;
  color: var(--amber-400);
}

.readout__rows {
  margin: 0;
  display: grid;
  gap: 7px;
  font-size: 11.5px;
}

.readout__rows div {
  display: grid;
  grid-template-columns: 88px 1fr;
  gap: var(--sp-3);
}

.readout__rows dt {
  color: var(--ink-600);
}

.readout__rows dd {
  margin: 0;
  color: var(--ink-800);
  word-break: break-all;
}

.readout__time {
  color: var(--teal-400) !important;
}

.is-warn {
  color: var(--rose-400) !important;
}

.readout__counts {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1px;
  background: var(--line);
  border-top: 1px solid var(--line);
}

.rcount {
  background: var(--ink-100);
  padding: 10px 8px;
  display: grid;
  gap: 2px;
  text-align: center;
}

.rcount__n {
  font-size: 18px;
  font-weight: 600;
  color: var(--accent);
}

.rcount__l {
  font-size: 10px;
  color: var(--ink-600);
}

/* --------------------------------------------- 首屏脊梁舞台 */

.hero__spine {
  margin-top: var(--sp-7);
  border: 1px solid var(--line);
  background:
    radial-gradient(ellipse 90% 120% at 50% 42%, rgba(224, 160, 74, 0.05), transparent 70%),
    var(--ink-050);
  padding: var(--sp-5) var(--sp-4) var(--sp-4);
}

.hero__spine-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--sp-3);
  margin-bottom: var(--sp-3);
}

.hero__spine-note {
  margin: 0;
  font-size: 11.5px;
  color: var(--ink-600);
}

.hero__spine-foot {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-4);
  margin-top: var(--sp-3);
  padding-top: var(--sp-3);
  border-top: 1px solid var(--line);
  font-size: 10.5px;
  color: var(--ink-600);
}

/* -------------------------------------------------------- 区块 */

.section {
  max-width: var(--shell-max);
  margin: 0 auto;
  padding: var(--sp-8) var(--sp-5);
  border-top: 1px solid var(--line);
}

.section__head {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: var(--sp-5);
  align-items: start;
  margin-bottom: var(--sp-6);
}

.section__index {
  font-size: 11px;
  color: var(--amber-400);
  letter-spacing: 0.16em;
  padding-top: 6px;
}

.section__title {
  font-size: clamp(22px, 2.6vw, 30px);
  letter-spacing: -0.025em;
  margin: 0 0 var(--sp-3);
}

.section__sub {
  font-size: 14px;
  line-height: 1.75;
  color: var(--ink-700);
  max-width: 78ch;
  margin: 0;
}

.section__sub b {
  color: var(--amber-400);
  font-weight: 600;
}

/* ------------------------------------------------- 脊梁主区 */

.spine-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.6fr) minmax(320px, 0.9fr);
  gap: var(--sp-5);
  align-items: start;
}

.spine-layout__toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-4);
  padding-bottom: var(--sp-3);
  border-bottom: 1px solid var(--line);
  margin-bottom: var(--sp-4);
}

.legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-4);
}

.legend__item {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  font-size: 11px;
}

.legend__glyph {
  color: var(--accent);
  font-size: 11px;
}

.legend__label {
  color: var(--ink-800);
}

.legend__auth {
  font-size: 9px;
  color: var(--ink-600);
  letter-spacing: 0.1em;
}

.spine-layout__hint {
  font-size: 10.5px;
  color: var(--ink-600);
}

.chain {
  margin-top: var(--sp-5);
  border-top: 1px solid var(--line);
  padding-top: var(--sp-4);
}

.chain__list {
  list-style: none;
  margin: var(--sp-3) 0 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.chain__list button {
  display: inline-flex;
  align-items: baseline;
  gap: 7px;
  background: var(--ink-100);
  border: 1px solid var(--line);
  padding: 4px 9px;
  font-size: 10.5px;
  color: var(--ink-600);
  transition: all var(--dur-fast) var(--ease-mech);
}

.chain__list button:hover {
  border-color: var(--line-strong);
  color: var(--ink-800);
}

.chain__list li.is-sel button {
  border-color: var(--amber-400);
  background: rgba(224, 160, 74, 0.1);
  color: var(--ink-900);
}

.chain__seq {
  color: var(--ink-600);
}

.chain__type {
  font-weight: 600;
}

.chain__id {
  color: var(--ink-600);
}

.spine-layout__probe {
  border: 1px solid var(--line-strong);
  background: var(--ink-050);
  position: sticky;
  top: 72px;
  max-height: calc(100vh - 100px);
  overflow-y: auto;
}

.probe__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--sp-3);
  padding: var(--sp-3) var(--sp-4);
  border-bottom: 1px solid var(--line);
  background: var(--ink-100);
  position: sticky;
  top: 0;
  z-index: 2;
}

.probe__cursor {
  font-size: 10px;
  color: var(--teal-400);
}

.spine-layout__probe :deep(.inspector) {
  padding: var(--sp-4);
}

/* --------------------------------------------------- 纪元区 */

.epoch-layout {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: var(--sp-6);
  align-items: start;
}

.epoch-note {
  border: 1px solid var(--line);
  background: var(--ink-050);
  padding: var(--sp-4);
  display: grid;
  gap: var(--sp-4);
}

.epoch-note__code {
  margin: 0;
  font-size: 10.5px;
  line-height: 1.75;
  color: var(--ink-800);
  white-space: pre-wrap;
  word-break: break-word;
  background: var(--ink-100);
  border-left: 2px solid var(--violet-400);
  padding: var(--sp-3);
  overflow-x: auto;
}

.epoch-note__text {
  margin: 0;
  font-size: 12px;
  color: var(--ink-700);
  line-height: 1.7;
}

.epoch-note__text .mono {
  color: var(--amber-400);
  font-size: 11.5px;
}

/* --------------------------------------------------- 报价区 */

.quotes {
  margin-top: var(--sp-7);
}

.quotes__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(230px, 1fr));
  gap: 1px;
  background: var(--line);
  border: 1px solid var(--line);
  margin-top: var(--sp-4);
}

.quote {
  background: var(--ink-050);
  padding: var(--sp-4);
  display: grid;
  gap: 8px;
  position: relative;
  transition: background var(--dur-fast) var(--ease-mech);
}

.quote:hover {
  background: var(--ink-100);
}

.quote.is-superseded {
  opacity: 0.55;
}

.quote__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--sp-3);
}

.quote__id {
  font-size: 10px;
  color: var(--ink-600);
}

.quote__kind {
  font-size: 9px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--violet-400);
}

.quote__sku {
  font-size: 14px;
  margin: 0;
}

.quote__price {
  margin: 0;
  font-size: 26px;
  font-weight: 600;
  color: var(--ink-900);
  display: flex;
  align-items: baseline;
  gap: 5px;
}

.quote__unit {
  font-size: 11px;
  color: var(--ink-600);
  font-weight: 400;
}

.quote__tax {
  font-size: 9.5px;
  color: var(--teal-400);
  border: 1px solid rgba(79, 209, 197, 0.4);
  padding: 1px 5px;
  margin-left: 4px;
}

.quote__meta {
  margin: 0;
  display: grid;
  gap: 3px;
  font-size: 10.5px;
}

.quote__meta div {
  display: grid;
  grid-template-columns: 52px 1fr;
  gap: 8px;
}

.quote__meta dt {
  color: var(--ink-600);
}

.quote__meta dd {
  margin: 0;
  color: var(--ink-700);
}

.quote__note {
  margin: 0;
  font-size: 10px;
  color: var(--amber-400);
  border-top: 1px dashed var(--line-strong);
  padding-top: 6px;
}

/* --------------------------------------------------- 为什么区 */

.why {
  display: grid;
  gap: 0;
  border-top: 1px solid var(--line);
}

.why__item {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: var(--sp-4);
  padding: var(--sp-5) 0;
  border-bottom: 1px solid var(--line);
}

.why__marker {
  width: 3px;
  background: var(--ink-500);
  align-self: stretch;
}

.why__item.is-warn .why__marker {
  background: var(--amber-400);
}

.why__item.is-error .why__marker {
  background: var(--rose-400);
}

.why__q {
  font-size: 15.5px;
  margin: 0 0 var(--sp-2);
  letter-spacing: -0.01em;
}

.why__a {
  font-size: 13px;
  line-height: 1.8;
  color: var(--ink-700);
  margin: 0 0 var(--sp-3);
  max-width: 92ch;
}

.why__refs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.why__ref {
  font-size: 10px;
  color: var(--ink-600);
  border: 1px solid var(--line);
  padding: 2px 7px;
  transition: all var(--dur-fast) var(--ease-mech);
}

.why__ref:hover {
  border-color: var(--amber-400);
  color: var(--amber-400);
}

/* --------------------------------------------------- 包清单区 */

.packs {
  margin-top: var(--sp-6);
  overflow-x: auto;
  border: 1px solid var(--line);
}

.packtable {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  min-width: 720px;
}

.packtable th {
  text-align: left;
  font-family: var(--font-mono);
  font-size: 9.5px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--ink-600);
  font-weight: 500;
  padding: 9px var(--sp-4);
  border-bottom: 1px solid var(--line-strong);
  background: var(--ink-100);
}

.packtable td {
  padding: 10px var(--sp-4);
  border-bottom: 1px solid var(--line);
  color: var(--ink-800);
  vertical-align: middle;
}

.packtable tr:last-child td {
  border-bottom: 0;
}

.packtable tbody tr {
  transition: background var(--dur-fast) var(--ease-mech);
}

.packtable tbody tr:hover {
  background: rgba(224, 160, 74, 0.04);
}

.packs__id {
  color: var(--amber-400);
}

.packs__caps {
  font-size: 10.5px;
  color: var(--ink-600);
}

.packs__hash {
  font-size: 10px;
  color: var(--ink-600);
}

.pill {
  display: inline-block;
  font-size: 10px;
  letter-spacing: 0.06em;
  padding: 2px 8px;
  border: 1px solid var(--line-strong);
  color: var(--ink-700);
}

.pill--world {
  border-color: rgba(224, 160, 74, 0.5);
  color: var(--amber-400);
}
.pill--system {
  border-color: rgba(79, 209, 197, 0.5);
  color: var(--teal-400);
}
.pill--content {
  border-color: rgba(167, 139, 250, 0.5);
  color: var(--violet-400);
}
.pill--narrative {
  border-color: rgba(163, 213, 92, 0.45);
  color: var(--lime-400);
}
.pill--adapter {
  border-color: rgba(240, 115, 106, 0.45);
  color: var(--rose-400);
}
.pill--state {
  color: var(--ink-600);
}

/* --------------------------------------------------- 快照区 */

.snap {
  margin-top: var(--sp-6);
  border: 1px solid var(--line);
  background: var(--ink-050);
  padding: var(--sp-4);
}

.snap__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--sp-5);
  margin-top: var(--sp-4);
}

.snap__h {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--ink-600);
  margin: 0 0 var(--sp-3);
  font-weight: 500;
}

.snap__col ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 0;
}

.snap__col li {
  display: grid;
  grid-template-columns: 1fr auto auto;
  gap: var(--sp-4);
  align-items: baseline;
  padding: 6px 0;
  border-bottom: 1px solid var(--line);
  font-size: 11px;
}

.snap__k {
  color: var(--ink-800);
}

.snap__t {
  font-size: 9.5px;
  color: var(--ink-600);
}

.snap__v {
  color: var(--ink-900);
  font-variant-numeric: tabular-nums;
  min-width: 64px;
  text-align: right;
}

.snap__v.is-low {
  color: var(--rose-400);
}

/* -------------------------------------------------------- 页脚 */

.footer {
  border-top: 1px solid var(--line);
  background: var(--ink-050);
}

.footer__inner {
  max-width: var(--shell-max);
  margin: 0 auto;
  padding: var(--sp-6) var(--sp-5);
  display: grid;
  gap: var(--sp-4);
}

.footer__disc {
  margin: 0;
  font-size: 12px;
  color: var(--ink-600);
  line-height: 1.7;
  max-width: 84ch;
}

.footer__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-4);
  font-size: 10px;
  color: var(--ink-500);
}

/* --------------------------------------------------- 响应式 */

@media (max-width: 1180px) {
  .spine-layout {
    grid-template-columns: 1fr;
  }
  .spine-layout__probe {
    position: static;
    max-height: none;
  }
}

@media (max-width: 1024px) {
  .hero__grid {
    grid-template-columns: 1fr;
    gap: var(--sp-6);
  }
  .topnav {
    display: none;
  }
}

@media (max-width: 720px) {
  .hero {
    padding: var(--sp-6) var(--sp-4) var(--sp-6);
  }
  .section {
    padding: var(--sp-6) var(--sp-4);
  }
  .section__head {
    grid-template-columns: 1fr;
    gap: var(--sp-2);
  }
  .section__index {
    padding-top: 0;
  }
  .hero__title {
    font-size: clamp(26px, 8vw, 36px);
  }
  .topbar__inner {
    padding: var(--sp-3) var(--sp-4);
  }
  .topbar__status {
    gap: var(--sp-2);
  }
  .topbar__seq {
    display: none;
  }
  .readout__counts {
    grid-template-columns: repeat(2, 1fr);
  }
  .why__item {
    grid-template-columns: 1fr;
    gap: var(--sp-2);
  }
  .why__marker {
    width: 32px;
    height: 3px;
  }
  .hero__spine {
    padding: var(--sp-4) var(--sp-2) var(--sp-3);
  }
}
/* ------------------------------------------------ 观测台返回条 */

.backbar {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: var(--sp-2) var(--sp-5);
  border-bottom: 1px solid var(--line);
  background: var(--ink-100);
}

.backbar__btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-family: var(--font-mono);
  font-size: 11.5px;
  color: var(--amber-400);
  background: var(--ink-200);
  border: 1px solid var(--line);
  padding: 7px 12px;
  cursor: pointer;
  transition: all var(--dur-fast) var(--ease-mech);
}

.backbar__btn:hover {
  border-color: var(--line-amber);
  background: var(--ink-300);
}

.backbar__note {
  font-size: 10.5px;
  color: var(--ink-600);
}

@media (max-width: 720px) {
  .backbar {
    padding: var(--sp-2) var(--sp-4);
  }
  .backbar__note {
    display: none;
  }
}

</style>
