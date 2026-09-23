<script setup lang="ts">
/**
 * StoryWorkspace — 玩家工作空间（连续长文阅读 + 自由行动）
 *
 * 与观测台是两条并列的 workspace，不是「首页 + 子页面」：
 * 离开这里去 Inspector，回来恢复阅读锚点与未发送草稿。
 */
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import StoryMessageTurn from './StoryMessageTurn.vue'
import { scenes, quickActions, storyEntries, storyHistory } from '../data/story-demo'
import { META, RULESET_HASH_E1, marketQuotes, records, snapshot } from '../data/world'
import { dispatchAction, clickIntent, parseIntent } from '../lib/story'
import type { ActionIntent, NarrativeMessage, StoryViewMode } from '../types'
import { useKeyboardInset, usePrefersReducedMotion } from '../composables'
import { isNearBottom, scrollToBottom } from '../composables'

const props = defineProps<{
  /** 从 Inspector 带过来的待聚焦 record_id */
  pendingRef?: string | null
}>()

const emit = defineEmits<{
  (e: 'enterInspector', refId: string | null): void
  (e: 'consumeRef'): void
  (e: 'scrollToTop'): void
}>()

usePrefersReducedMotion()



/* ------------------------------------------------------------ 状态 */

const messages = ref<NarrativeMessage[]>([...storyHistory])
const sceneId = ref<string>(messages.value[messages.value.length - 1].scene_id)
const viewMode = ref<StoryViewMode>('scroll')
const draft = ref('')
const axis = ref<HTMLElement | null>(null)
const sideOpen = ref(false)
const enteringIds = ref<Set<string>>(new Set())
const pendingNew = ref(0)
const atBottom = ref(true)
const shakingEntry = ref<string | null>(null)

const { expanded: keyboardOpen } = useKeyboardInset()

const scene = computed(() => scenes.find((s) => s.id === sceneId.value) ?? scenes[0])
const lastWorld = computed(() => [...messages.value].reverse().find((m) => m.source === 'world'))
const latestId = computed(() => lastWorld.value?.message_id ?? null)

const ctxActions = computed(() => quickActions.filter((a) => a.scenes === '*' || a.scenes.includes(sceneId.value)))

const entryGroups = computed(() => {
  const labels: Record<string, string> = { context: '语境层', archive: '资料层', authority: '权威层' }
  const order: ('context' | 'archive' | 'authority')[] = ['context', 'archive', 'authority']
  return order.map((g) => ({
    group: g,
    label: labels[g],
    items: storyEntries.filter((e) => e.group === g)
  }))
})

/* ------------------------------------------------------ 滚动与锚点 */

let anchorTimer = 0

function onAxisScroll() {
  const el = axis.value
  if (!el) return
  atBottom.value = isNearBottom(el, 120)
  if (atBottom.value) pendingNew.value = 0
  clearTimeout(anchorTimer)
  anchorTimer = window.setTimeout(() => {
    // 会话内锚点：离开再回来时回到这里
    try {
      sessionStorage.setItem('corerp.story.anchor', JSON.stringify({ messageId: lastVisible(), scrollTop: el.scrollTop }))
    } catch {
      /* 隐私模式下忽略 */
    }
  }, 400)
}

function lastVisible(): string {
  const el = axis.value
  if (!el) return latestId.value ?? ''
  const nodes = el.querySelectorAll<HTMLElement>('[data-mid]')
  for (let i = nodes.length - 1; i >= 0; i--) {
    if (nodes[i].offsetTop <= el.scrollTop + el.clientHeight * 0.5) return nodes[i].dataset.mid!
  }
  return latestId.value ?? ''
}

function jumpToBottom() {
  const el = axis.value
  if (!el) return
  scrollToBottom(el)
  pendingNew.value = 0
  for (const m of messages.value) if (m.unread) m.unread = false
}

onMounted(() => {
  // 刷新后初始视图：有会话内阅读锚点就恢复它，否则落到最新。
  // 「回到最新」永远有明确按钮，不靠强制滚动维持。
  const el = axis.value
  if (!el) return
  el.addEventListener('scroll', onAxisScroll, { passive: true })
  requestAnimationFrame(() => {
    let restored = false
    try {
      const raw = sessionStorage.getItem('corerp.story.anchor')
      if (raw) {
        const { scrollTop } = JSON.parse(raw) as { scrollTop: number }
        if (Number.isFinite(scrollTop) && scrollTop > 4) {
          el.scrollTop = scrollTop
          restored = true
        }
      }
    } catch {
      /* 忽略 */
    }
    if (!restored) scrollToBottom(el, false)
    onAxisScroll()
  })
})

onUnmounted(() => {
  axis.value?.removeEventListener('scroll', onAxisScroll)
  clearTimeout(anchorTimer)
})

// deep：历史是原地 push 的；不深监听则「新消息到达」根本不会触发
watch(
  messages,
  async (v: NarrativeMessage[]) => {
    const last = v[v.length - 1]
    if (!last) return
    // 关键：用到达「之前」是否停在底部来决定，而不是被上一条的滚动污染。
    // 一旦用户往上读历史，任何新内容都不再拽他下来。
    const wasAtBottom = atBottom.value
    enteringIds.value.add(last.message_id)
    await nextTick()
    const el = axis.value
    if (!el) return
    if (wasAtBottom) {
      scrollToBottom(el)
    } else {
      if (!last.unread) last.unread = true
      pendingNew.value++
    }
    window.setTimeout(() => {
      enteringIds.value.delete(last.message_id)
    }, 400)
  },
  { deep: true }
)

/* ------------------------------------------------------------ 行动 */

const lastWarning = ref<string | null>(null)

function pushSystem(text: string) {
  messages.value.push({
    message_id: `msg_sys_${Date.now()}`,
    turn: messages.value.length + 1,
    source: 'system',
    world_time: snapshot.world_time,
    scene_id: sceneId.value,
    body: text,
    fact_refs: [],
    demo: true,
    unread: true
  })
}

/** 同一分发入口：自由文字 / 快捷点击都在这里 */
function run(intent: ActionIntent) {
  lastWarning.value = null
  // 玩家回合先落进历史：来源可区分，且不会被清空
  messages.value.push({
    message_id: `msg_p_${Date.now()}`,
    turn: messages.value.length + 1,
    source: 'player',
    world_time: snapshot.world_time,
    scene_id: sceneId.value,
    body: intent.raw,
    fact_refs: [],
    demo: false
  })

  const result = dispatchAction(intent, sceneId.value)

  if (!result.ok) {
    // 未支持的输入：明确反馈，不编造成功
    lastWarning.value = result.warning ?? '未支持的意图'
    pushSystem(
      `**没有理解「${escapeMd(intent.raw)}」。**\n\n` +
        '本演示只响应：观察 / 交谈 / 查看商品 / 等一会儿 / 移动 / 观测台。' +
        '其它输入不会被写成世界真相——它只会得到这条反馈。'
    )
    return
  }

  if (result.gotoScene) sceneId.value = result.gotoScene
  for (const m of result.messages) {
    messages.value.push({ ...m, turn: messages.value.length + 1 })
  }
  if (result.navigate === 'inspector') emit('enterInspector', null)
}

function escapeMd(s: string) {
  return s.replace(/([*_`|])/g, '\\$1')
}

function send() {
  const text = draft.value.trim()
  if (!text) return
  const intent = parseIntent(text)
  if (!intent) {
    // 解析不到意图 ≠ 不响应：仍然留下玩家回合和一条明确的失败反馈
    messages.value.push({
      message_id: `msg_p_${Date.now()}`,
      turn: messages.value.length + 1,
      source: 'player',
      world_time: snapshot.world_time,
      scene_id: sceneId.value,
      body: text,
      fact_refs: [],
      demo: false
    })
    lastWarning.value = '未识别'
    pushSystem(
      `**未识别「${escapeMd(text)}」。**\n\n` +
        '演示没有把它当成任何动作，也没有提交任何事件。试试「观察店里」「和赵婉说话」「去集市」。'
    )
    draft.value = ''
    return
  }
  run(intent)
  draft.value = ''
}

function onQuick(kind: ActionIntent['kind'], label: string) {
  run(clickIntent(kind, label))
}

/** Enter 发送，Shift+Enter 换行；软键盘的换行键不应误触发送 */
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    send()
  }
}

/* ------------------------------------------------------- 入口与抽屉 */

function onEntry(id: string) {
  switch (id) {
    case 'inspector':
      sideOpen.value = false
      emit('enterInspector', null)
      break
    case 'economy':
      pushSystem('**经济账本**已并入观测台（区块 04）：三口径与报价台阶见 EconomyChart。这里不重复一份。')
      sideOpen.value = false
      break
    case 'dlc':
      pushSystem('**扩展包清单**已并入观测台（区块 06）：五种包类别、能力声明与 schema/content 哈希。')
      sideOpen.value = false
      break
    case 'timeline':
      pushSystem('**时间线**已并入观测台（区块 02）：规则纪元按事件序号分段不可变，见 EpochStrata。')
      sideOpen.value = false
      break
    default: {
      // 未接通：入口可见、可点、有明确说明，但不制造假页面
      shakingEntry.value = id
      window.setTimeout(() => (shakingEntry.value = null), 300)
      pushSystem(`**${storyEntries.find((e) => e.id === id)?.label ?? id}** 未接入：本演示没有对应的可写数据，不放一个空开关在这里。`)
      sideOpen.value = false
    }
  }
}

function onFocusRef(id: string) {
  // 从叙事引用跳到观测台记录探针
  sideOpen.value = false
  emit('enterInspector', id)
}

watch(
  () => props.pendingRef,
  (v) => {
    if (v) emit('consumeRef')
  }
)

/* --------------------------------------------------------- 展示模式 */

const viewNote = computed(() =>
  viewMode.value === 'scroll'
    ? '长文卷轴：连续阅读，来源由脊线区分。'
    : '酒馆对话：同一条历史的另一种排版，不重新生成内容、不改消息顺序。'
)

function toggleView() {
  // 两种模式读同一条 messages：不重算、不清草稿、不改顺序
  viewMode.value = viewMode.value === 'scroll' ? 'tavern' : 'scroll'
}

watch(sceneId, () => {
  pendingNew.value = 0
})

defineExpose({ jumpToBottom })
</script>

<template>
  <div class="shell" :class="{ 'is-keyboard': keyboardOpen }">
    <!-- ============================================ 阅读竖轴（主角） -->
    <div class="shell__axis">
      <!-- 单条顶栏：世界时间 + 场景。移动端的抽屉按钮并进来，不叠第二条 -->
      <div class="readout">
        <span class="readout__time mono">{{ snapshot.world_time.replace('T', ' · ') }}</span>
        <span class="readout__scene">
          <span class="readout__scene-name">{{ scene.name }}</span>
          <span class="readout__scene-place">{{ scene.place }}</span>
        </span>
        <span class="readout__demo">demo</span>
        <button class="readout__panel-btn" type="button" :aria-expanded="sideOpen" @click="sideOpen = !sideOpen">
          功能{{ sideOpen ? ' ↑' : ' ↓' }}
        </button>
      </div>

      <div ref="axis" class="axis" tabindex="0" aria-label="叙事历史，可滚动阅读">
        <div class="axis__inner" :class="'axis__inner--' + viewMode">
          <StoryMessageTurn
            v-for="m in messages"
            :key="m.message_id"
            :message="m"
            :latest="m.message_id === latestId && m.source === 'world'"
            :entering="enteringIds.has(m.message_id)"
            @focus-ref="onFocusRef"
          />

          <p v-if="!messages.length" class="axis__state">
            <span class="axis__state-title">历史是空的</span>
            <span class="axis__state-text">从一个动作开始：观察、交谈，或直接输入。</span>
          </p>
        </div>

        <button
          v-if="pendingNew > 0"
          class="axis__newpulse"
          type="button"
          @click="jumpToBottom"
        >
          <span class="axis__newpulse-dot" aria-hidden="true" />
          有新内容 · 跳最新
        </button>
      </div>

      <!-- 语境动作：第一层行动，不依赖抽屉 -->
      <div class="ctx">
        <div class="ctx__label">此处可做 · CONTEXT</div>
        <div class="ctx__row">
          <button
            v-for="a in ctxActions"
            :key="a.kind"
            class="ctx__btn"
            type="button"
            :title="a.hint"
            @click="onQuick(a.kind, a.label)"
          >
            {{ a.label }}
          </button>
        </div>
      </div>

      <!-- 自由文字：与快捷点击同一个 dispatchAction() -->
      <div class="composer">
        <div class="composer__row">
          <textarea
            v-model="draft"
            class="composer__input"
            rows="1"
            placeholder="自由输入：观察店里 / 和赵婉说话 / 去集市…"
            enterkeyhint="send"
            inputmode="text"
            aria-label="自由行动输入"
            @keydown="onKeydown"
          />
          <button class="composer__send" type="button" :disabled="!draft.trim()" @click="send">行动 →</button>
        </div>
        <div class="composer__hint">
          <span>Enter 发送 · Shift+Enter 换行</span>
          <span v-if="lastWarning" class="composer__warn">· 上次输入未支持</span>
        </div>
      </div>
    </div>

    <!-- ================================== 右栏：读数 + 分层入口 -->
    <!-- 移动端遮罩：点面板外任意处收起；桌面端 CSS 隐藏 -->
    <button
      v-if="sideOpen"
      class="scrim"
      type="button"
      aria-label="关闭面板"
      @click="sideOpen = false"
    />

    <aside class="shell__side" :class="{ 'is-open': sideOpen }" aria-label="世界读数与功能入口">
      <button class="side__grip" type="button" @click="sideOpen = !sideOpen">
        {{ sideOpen ? '收起面板' : '世界读数 · 功能入口' }}
      </button>

      <div class="side__head">
        <div class="side__brand">
          <span class="side__brand-mark" aria-hidden="true" />
          <span class="side__brand-text">
            <strong>CoreRP</strong>
            <span>叙事流 · M0 演示</span>
          </span>
        </div>
      </div>

      <div class="side__body">
        <!-- 世界读数 -->
        <section class="panel">
          <div class="panel__head">
            <h2 class="panel__title">此刻</h2>
            <span class="panel__count mono">{{ messages.length }} 回合</span>
          </div>
          <div class="facts">
            <div class="facts__row">
              <span class="facts__k">world_time</span>
              <span class="facts__v facts__v--accent">{{ snapshot.world_time.replace('T', ' · ') }}</span>
            </div>
            <div class="facts__row">
              <span class="facts__k">scene</span>
              <span class="facts__v">{{ scene.name }}</span>
            </div>
            <div class="facts__row">
              <span class="facts__k">present</span>
              <span class="facts__v">{{ scene.present.join(' · ') }}</span>
            </div>
            <div class="facts__row">
              <span class="facts__k">instance</span>
              <span class="facts__v">{{ snapshot.instance_id }}</span>
            </div>
            <div class="facts__row">
              <span class="facts__k">epoch</span>
              <span class="facts__v">1 · {{ RULESET_HASH_E1.slice(0, 13) }}…</span>
            </div>
            <div class="facts__row">
              <span class="facts__k">报价</span>
              <span class="facts__v">{{ marketQuotes.length }} 条有效示例</span>
            </div>
            <div class="facts__row">
              <span class="facts__k">records</span>
              <span class="facts__v">{{ records.length }} 条构造记录</span>
            </div>
            <p class="facts__note">
              {{ META.disclaimer }}世界时间来自 demo snapshot，不是浏览器时间。
            </p>
          </div>
        </section>

        <!-- 展示模式 -->
        <section class="panel">
          <div class="panel__head">
            <h2 class="panel__title">阅读方式</h2>
          </div>
          <button class="entry" type="button" @click="toggleView">
            <span class="entry__glyph" aria-hidden="true">{{ viewMode === 'scroll' ? '≡' : '❝' }}</span>
            <span>
              <span class="entry__label">{{ viewMode === 'scroll' ? '长文卷轴' : '酒馆对话' }}</span>
              <span class="entry__blurb">{{ viewNote }}</span>
            </span>
            <span class="entry__tag">切换</span>
          </button>
          <p class="facts__note">
            两种模式读同一条历史：消息 id、顺序、文字、草稿都不变，切换不重新生成内容。
          </p>
        </section>

        <!-- 分层入口 -->
        <section v-for="g in entryGroups" :key="g.group" class="panel">
          <div class="panel__head">
            <h2 class="panel__title">{{ g.label }}</h2>
            <span class="panel__count mono">{{ g.items.length }}</span>
          </div>
          <div class="entries">
            <button
              v-for="e in g.items"
              :key="e.id"
              class="entry"
              :class="['entry--' + e.status, { 'entry--shake': shakingEntry === e.id }]"
              type="button"
              @click="onEntry(e.id)"
            >
              <span class="entry__glyph" aria-hidden="true">{{ e.status === 'ready' ? '◆' : '◇' }}</span>
              <span>
                <span class="entry__label">{{ e.label }}</span>
                <span class="entry__blurb">{{ e.blurb }}</span>
              </span>
              <span class="entry__tag">{{ e.status === 'ready' ? '可看' : '未接入' }}</span>
            </button>
          </div>
        </section>

        <p class="facts__note">
          叙事文本不会改写权威事实。点击任意 record_id 可在观测台查看同一条构造记录。
        </p>
      </div>
    </aside>
  </div>
</template>

<style scoped>
@import '../styles/story.css';
</style>
