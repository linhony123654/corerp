<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import PlayWallet from './PlayWallet.vue'
import PlayContacts from './PlayContacts.vue'
import PlayWork from './PlayWork.vue'
import PlayMap from './PlayMap.vue'
import PlayMessages from './PlayMessages.vue'
import type { LocalMap, MapMove } from '../lib/localMap'
import PlayStyle from './PlayStyle.vue'
import PlayRegenerate from './PlayRegenerate.vue'
import { readNarrativeStream } from '../lib/narrativeStream'
import { pendingStyleKey } from '../lib/narrativeStyle'
import type { StyleScope } from '../lib/narrativeStyle'
import type { WalletSnapshot } from '../lib/wallet'

type Person = { entity_id: string; display_name: string }
type Turn = { turn_run_id: string; narrative_lines: string[]; can_regenerate: boolean }
type Observation = LocalMap & {
  decision_mode: 'deterministic' | 'chat_completions'
  narrative_mode: 'deterministic' | 'style_planner' | 'custom'
  session_id: string; controlled_entity: Person
  present_entities: Person[]
  recent_turns: Turn[]
}
type Pending = { path: string; body: Record<string, unknown>; narrative_turn_id?: string }
type Bookmark = { session: string; openKey: string; pending: Pending | null }
const storageKey = 'corerp.play.v1'
const token = ref('') // Credentials deliberately live only in this page's memory.
const observation = ref<Observation | null>(null)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const draft = ref('')
const travel = ref(false)
const walletOpen = ref(false)
const contactsOpen = ref(false)
const workOpen = ref(false)
const mapOpen = ref(false)
const messagesOpen = ref(false)
const styleOpen = ref(false), stylePending = ref(false)
const sessionScope = ref<StyleScope | null>(null)
const variants = ref<Record<string, string[]>>({})
const turnPreview = ref<string[] | null>(null)
let turnStreamController: AbortController | null = null
onBeforeUnmount(() => turnStreamController?.abort())
function setVariant(turn: string, lines: string[] | null) {
  if (lines === null) delete variants.value[turn]
  else variants.value[turn] = lines
}
const socialSeeking = ref(false)
const bookmark = ref<Bookmark>({ session: '', openKey: crypto.randomUUID(), pending: null })
try {
  const saved = JSON.parse(localStorage.getItem(storageKey) || 'null')
  if (saved && typeof saved.session === 'string' && typeof saved.openKey === 'string') bookmark.value = saved
} catch { /* A malformed bookmark cannot become world state. */ }
const clock = computed(() => observation.value?.world_time.slice(11, 16) || '—')
const date = computed(() => observation.value?.world_time.slice(0, 10) || '')
const locked = computed(() => busy.value || !!bookmark.value.pending || stylePending.value)

function save() {
  // Persist intent BEFORE sending, so a lost response can be retried with the same key.
  localStorage.setItem(storageKey, JSON.stringify(bookmark.value))
}
async function api<T>(path: string, body: Record<string, unknown>): Promise<T> {
  const response = await fetch(`/api/v1/rp/${path}`, {
    method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token.value}` },
    body: JSON.stringify(body), signal: AbortSignal.timeout(45000)
  })
  const envelope = await response.json()
  if (!response.ok) throw Object.assign(new Error(envelope.error?.message || `连接失败 (${response.status})`), { code: envelope.error?.code })
  return envelope.data as T
}
async function refresh() {
  observation.value = await api<Observation>('observe', { session_id: bookmark.value.session })
  const visible = new Set(observation.value.recent_turns.map(turn => turn.turn_run_id))
  for (const id of Object.keys(variants.value)) if (!visible.has(id)) delete variants.value[id]
}
async function streamNarrative(body: Record<string, unknown>, preview: (lines: string[]) => void, signal: AbortSignal) {
  const response = await fetch('/api/v1/rp/narrative/stream', {
    method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token.value}` },
    body: JSON.stringify(body), signal: AbortSignal.any([signal, AbortSignal.timeout(45000)])
  })
  return readNarrativeStream(response, preview)
}
function readWallet() {
  return api<WalletSnapshot>('wallet/read', { session_id: bookmark.value.session })
}
function readContacts(after: string) {
  return api<{ contacts: { entity_id: string; display_name: string; last_known_world_time: string }[]; next_after_entity_id?: string; world_time: string }>('contacts/read', { session_id: bookmark.value.session, after_entity_id: after })
}
function readWork() {
  return api<{ jobs: { contract_id: string; employer_name: string; status: string; workplace_name: string; wage_minor: string; pay_period_days: number; currency_id: string; currency_scale: number; currency_symbol: string }[]; appointments: { schedule_id: string; world_time: string; original_world_time?: string; place_name: string; activity: string }[]; more_appointments: boolean; world_time: string }>('work/read', { session_id: bookmark.value.session })
}
function readMap() {
  return api<Observation>('observe', { session_id: bookmark.value.session })
}
function readMessages(before: number) {
  return api<{ messages: { message_id: string; sequence: number; world_time: string; kind: string; title: string; body: string }[]; next_before_sequence?: number; world_time: string }>('messages/read', { session_id: bookmark.value.session, before_sequence: before })
}
function moveFromMap(request: MapMove) {
  mapOpen.value = false
  void act('actions/move', request)
}
async function guarded(work: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try { await work() } catch (e) {
    error.value = e instanceof Error ? e.message : '暂时无法连接世界，请重试。'
  } finally { busy.value = false }
}
async function connect() {
  await guarded(async () => {
    save()
    if (bookmark.value.session) {
      sessionScope.value = await api<StyleScope>('sessions/resume', { session_id: bookmark.value.session })
    } else {
      const session = await api<StyleScope>('sessions/open', {
        instance_id: 'inst_m2_t09', branch_id: 'br_main', entity_id: 'entity_m2_rp_lin',
        pov: 'second_person', idempotency_key: bookmark.value.openKey
      })
      bookmark.value.session = session.session_id; save()
      sessionScope.value = session
    }
    stylePending.value = !!localStorage.getItem(pendingStyleKey(bookmark.value.session))
    if (bookmark.value.pending) await finishPending()
    else await refresh()
    await scrollToEnd()
  })
}
async function scrollToEnd() { await nextTick(); window.scrollTo({ top: document.documentElement.scrollHeight }) }
function readableLine(line: string) {
  return line.replace(/(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}):\d{2}Z/g, '$1 $2')
}
async function finishPending(useSavedNarrative = false) {
  const pending = bookmark.value.pending
  if (!pending) return
  if (useSavedNarrative && !pending.narrative_turn_id) return
  let completedPresentation: { lines: string[]; warnings: string[] } | null = null
  if (!pending.narrative_turn_id) {
    const result = await api<{ status?: string; turn_run_id?: string }>(pending.path, pending.body)
    if (result.status === 'budget_exhausted') {
      notice.value = '时间正在推进，点击“继续未完成的行动”即可接着等待。'
      await refresh(); return
    }
    if (pending.path === 'turns/run') {
      if (result.status !== 'settled' || !result.turn_run_id) throw new Error('回合尚未确认结束，请继续原行动。')
      pending.narrative_turn_id = result.turn_run_id
      // This durable transition prevents a stream retry from rerunning actions.
      save()
    }
  }
  if (pending.narrative_turn_id && !useSavedNarrative) {
    turnPreview.value = []; turnStreamController = new AbortController()
    await scrollToEnd()
    try {
      // No override: the settled turn's pinned style owns its original narrative.
      const rendered = await streamNarrative({ session_id: bookmark.value.session, turn_run_id: pending.narrative_turn_id }, lines => {
        turnPreview.value = lines
        void scrollToEnd()
      }, turnStreamController.signal)
      completedPresentation = rendered.view
    } catch (cause) {
      // On reconnect, expose the saved-original recovery even when presentation
      // fails before this page has loaded its first observation.
      if (!observation.value) {
        try { await refresh() } catch { /* Original stream error stays actionable on reconnect. */ }
      }
      throw cause
    } finally {
      turnPreview.value = null; turnStreamController = null
    }
  }
  // Keep intent until fresh world/history is loaded; even a failed observation is retry-safe.
  await refresh()
  if (completedPresentation && pending.narrative_turn_id) setVariant(pending.narrative_turn_id, completedPresentation.lines)
  bookmark.value.pending = null; save()
  if (useSavedNarrative) notice.value = '已读取保存的原叙述，没有重做行动或调整世界事实。'
  if (pending.path === 'actions/wait') socialSeeking.value = false
  if (pending.path === 'turns/run') draft.value = ''
  else notice.value = pending.path === 'actions/move' ? '你已抵达新的地点。' : '时间已经向前。'
  if (completedPresentation?.warnings.length) notice.value = '叙述器提示：' + completedPresentation.warnings.join('；')
  travel.value = false
  await scrollToEnd()
}
async function act(path: string, values: Record<string, unknown>) {
  if (!observation.value || locked.value) return
  await guarded(async () => {
    bookmark.value.pending = { path, body: {
      session_id: bookmark.value.session, expected_cursor: observation.value!.observation_cursor,
      idempotency_key: crypto.randomUUID(), ...values
    } }
    save(); await finishPending()
  })
}
function speak() {
  if (draft.value.trim()) void act('turns/run', { text: draft.value.trim() })
}
function wait(hours: number) {
  if (!observation.value) return
  const target = new Date(Date.parse(observation.value.world_time) + hours * 3600000).toISOString().replace('.000Z', 'Z')
  void act('actions/wait', { target_world_time: target, budget: 100, ...(socialSeeking.value ? { opportunity_intent: 'social' } : {}) })
}
</script>

<template>
  <div class="play">
    <header class="play-header">
      <span class="wordmark">Core<span>RP</span><small>生活，继续发生。</small></span>
      <span class="mode">{{ observation ? observation.decision_mode === 'chat_completions' ? 'AI 人物 · 受世界规则约束' : '本地体验 · 确定性人物' : '本地体验 · 持久世界' }}</span>
    </header>

    <main v-if="!observation" class="arrival">
      <div class="chapter-mark" aria-hidden="true">◌</div>
      <p class="eyebrow">一段生活 · 从这里开始</p>
      <h1>回到世界里。</h1>
      <p class="intro">一句问候，一次相遇。<br>你离开以后，发生过的事依然留在这里。</p>
      <form class="entry" @submit.prevent="connect">
        <label for="credential">玩家访问凭证</label>
        <input id="credential" v-model="token" type="password" autocomplete="off" required placeholder="输入本地服务提供的凭证" :disabled="busy">
        <p class="hint">凭证仅在当前页面使用。人物回应方式由本地服务配置，进入后可查看。</p>
        <button class="primary" :disabled="busy || !token.trim()">{{ busy ? '正在连接…' : bookmark.session ? '继续这段生活 →' : '进入世界 →' }}</button>
      </form>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
    </main>

    <template v-else>
      <section class="scene" aria-label="当前场景">
        <div class="scene-heading">
          <p class="eyebrow">{{ date }} <span class="sun" aria-hidden="true">☼</span> <time>{{ clock }}</time></p>
          <h1>{{ observation.place_name }}</h1>
          <p class="presence">你是 {{ observation.controlled_entity.display_name }}<span> / </span>
            <template v-if="observation.present_entities.length">此刻在场：<strong v-for="person in observation.present_entities" :key="person.entity_id">{{ person.display_name }}</strong></template>
            <template v-else>此刻，只有你在这里。</template>
          </p>
        </div>
        <div class="scene-seal" aria-hidden="true"><span>此</span><span>刻</span></div>
      </section>

      <main class="reading" aria-label="对话历史">
        <p class="history-label">生活的片段 <span>最近 50 段 · 已保存</span></p>
        <div v-if="!observation.recent_turns.length" class="quiet">
          <span aria-hidden="true">—</span>
          <p>你站在{{ observation.place_name }}。<br>{{ observation.present_entities.length ? '有人与你同在。你想说些什么？' : '你可以稍作停留，或去别处走走。' }}</p>
        </div>
        <article v-for="(turn, index) in observation.recent_turns" :key="turn.turn_run_id" class="turn">
          <span class="turn-mark" aria-hidden="true">{{ String(index + 1).padStart(2, '0') }}</span>
          <p v-for="(line, i) in (variants[turn.turn_run_id] || turn.narrative_lines)" :key="i" :class="{ 'your-words': i === 0 }">{{ readableLine(line) }}</p>
          <PlayRegenerate v-if="turn.can_regenerate" :session="bookmark.session" :turn="turn.turn_run_id" :disabled="locked" :api="api" :stream="streamNarrative" @variant="setVariant(turn.turn_run_id, $event)" />
        </article>
        <section v-if="turnPreview !== null" class="turn incoming" aria-label="当前回合叙述流" aria-busy="true">
          <span class="turn-mark" aria-hidden="true">…</span>
          <p class="stream-note" role="status">行动已提交 · 正在接收叙述，尚未读完</p>
          <p v-for="(line, i) in turnPreview" :key="i" :class="{ 'your-words': i === 0 }">{{ readableLine(line) }}</p>
        </section>
      </main>

      <footer class="composer">
        <div class="composer-inner">
          <p v-if="error" class="error" role="alert">暂未完成：{{ error }}。{{ bookmark.pending?.narrative_turn_id ? '行动已经提交，继续只会读取叙述，不会重做行动。' : '已保留原行动，可安全重试。' }}</p>
          <p v-if="notice" class="notice" role="status">{{ notice }}</p>
          <p v-if="stylePending" class="notice" role="status">叙事设置尚未确认，请打开“叙事设置”继续保存，再开始新行动。</p>
          <button v-if="bookmark.pending" class="recover" :disabled="busy" @click="guarded(finishPending)">{{ busy ? '正在继续…' : bookmark.pending.narrative_turn_id ? '继续读取叙述' : '继续未完成的行动' }}</button>
          <button v-if="bookmark.pending?.narrative_turn_id && !busy" class="recover" @click="guarded(() => finishPending(true))">读取已保存原叙述</button>
          <nav class="actions" aria-label="场景行动">
            <button :disabled="locked" aria-haspopup="dialog" @click="walletOpen = true">钱包</button>
            <button :disabled="locked" aria-haspopup="dialog" @click="contactsOpen = true">通讯录</button>
            <button :disabled="locked" aria-haspopup="dialog" @click="workOpen = true">工作信息</button>
            <button :disabled="locked" aria-haspopup="dialog" @click="mapOpen = true">地图</button>
            <button :disabled="locked" aria-haspopup="dialog" @click="messagesOpen = true">手机</button>
            <button :disabled="busy || !!bookmark.pending" aria-haspopup="dialog" @click="styleOpen = true">叙事设置</button>
            <button :disabled="locked" @click="guarded(refresh)">环顾四周</button>
            <button :disabled="locked" :aria-expanded="travel" aria-controls="destinations" @click="travel = !travel">去别处 ↗</button>
            <button :disabled="locked" @click="wait(1)">等一小时</button>
            <button :disabled="locked" @click="wait(4)">等四小时</button>
          </nav>
          <div class="wait-intent">
            <label><input v-model="socialSeeking" type="checkbox" :disabled="locked" aria-describedby="wait-intent-hint">等待时，愿意和熟人聊聊</label>
            <p id="wait-intent-hint">仅用于下一次等待，不保证有人回应，也不会替你说话。</p>
          </div>
          <div v-if="travel" id="destinations" class="destinations">
            <p v-if="!observation.reachable_places.length">附近没有可以前往的地点。</p>
            <button v-for="place in observation.reachable_places" :key="place.place_id" :disabled="locked" @click="act('actions/move', { from_place_id: observation.place_id, to_place_id: place.place_id })">{{ place.display_name }} →</button>
          </div>
          <form class="speech" @submit.prevent="speak">
            <label class="sr-only" for="words">你想说的话</label>
            <textarea id="words" v-model="draft" :disabled="locked" maxlength="2000" rows="2" placeholder="说点什么，让相遇继续……" @keydown.ctrl.enter.prevent="speak" @keydown.meta.enter.prevent="speak"></textarea>
            <button class="primary" :disabled="locked || !draft.trim()">{{ busy ? '…' : '说出 →' }}</button>
          </form>
          <p class="footnote"><span>{{ busy ? '世界正在回应…' : '此处输入会作为你说出的话' }}</span><span>Ctrl / ⌘ + Enter</span></p>
        </div>
      </footer>
      <PlayWallet v-if="walletOpen" :owner="observation.controlled_entity.display_name" :read="readWallet" @close="walletOpen = false" />
      <PlayContacts v-if="contactsOpen" :read="readContacts" @close="contactsOpen = false" />
      <PlayWork v-if="workOpen" :read="readWork" @close="workOpen = false" />
      <PlayMap v-if="mapOpen" :read="readMap" @close="mapOpen = false" @move="moveFromMap" />
      <PlayMessages v-if="messagesOpen" :read="readMessages" @close="messagesOpen = false" />
      <PlayStyle v-if="styleOpen && sessionScope" :scope="{ session_id: sessionScope.session_id, instance_id: sessionScope.instance_id, branch_id: sessionScope.branch_id }" :narrative-mode="observation.narrative_mode" :api="api" @close="styleOpen = false" @pending="stylePending = $event" />
    </template>
  </div>
</template>

<style scoped>
.turn .stream-note { font: 12px/1.8 var(--font-body); color: var(--muted); }
.wait-intent { margin: 4px 0 12px; font-size: 12px; }
.wait-intent label { display: flex; align-items: center; gap: 8px; min-height: 44px; cursor: pointer; }
.wait-intent input { width: 18px; height: 18px; accent-color: var(--accent); flex: 0 0 auto; }
.wait-intent p { margin: 0; color: var(--muted); line-height: 1.7; }
.play { --paper: #f1eee5; --text: #343d35; --muted: #697065; --accent: #7e4935; min-height: 100dvh; background: var(--paper); color: var(--text); background-image: repeating-linear-gradient(0deg, transparent 0 3px, #484a3505 4px, transparent 5px); }
.play-header { max-width: 1160px; margin: auto; padding: 28px 36px; display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid #39493525; }
.wordmark { font-family: Georgia, serif; font-size: 25px; letter-spacing: -.8px; }.wordmark > span { color: var(--accent); }.wordmark small { font-family: var(--font-body); font-size: 11px; margin-left: 22px; letter-spacing: 2px; color: var(--muted); }.mode { font-size: 11px; color: var(--muted); }
.arrival { max-width: 560px; margin: 7vh auto 0; padding: 0 28px 70px; }.chapter-mark { color: var(--accent); font-size: 74px; line-height: 1.2; }.eyebrow { font-size: 11px; letter-spacing: 2px; color: var(--muted); }.play h1 { color: var(--text); font-family: var(--font-serif); font-weight: 400; font-size: clamp(32px, 5vw, 52px); line-height: 1.5; }.intro { font-family: var(--font-serif); line-height: 2; font-size: 18px; margin: 20px 0 36px; }.entry label { display: block; font-size: 13px; margin-bottom: 8px; }.entry input { width: 100%; padding: 14px; border: 1px solid #858d7f; background: #fff9; color: var(--text); font: inherit; }.hint { color: var(--muted); font-size: 12px; line-height: 1.7; margin: 12px 0 24px; }
.play button { cursor: pointer; font: inherit; color: inherit; background: none; border: 0; padding: 10px 12px; min-height: 44px; border-radius: 2px; }.play button:hover:not(:disabled) { background: #35433112; }.play button:disabled { opacity: .48; cursor: default; }.play :is(button, input, textarea):focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; }.play button.primary { background: #344638; color: #fffaf0; padding: 12px 24px; }.play button.primary:hover:not(:disabled) { background: #485b40; }
.scene { max-width: 860px; margin: auto; padding: 48px 32px 28px; display: flex; align-items: center; justify-content: space-between; gap: 16px; }.sun { color: var(--accent); padding: 0 14px; font-size: 18px; }.presence { color: var(--muted); font-size: 13px; line-height: 2; }.presence > span { padding: 0 10px; }.presence strong { font-weight: 400; color: var(--text); margin-right: 12px; white-space: nowrap; }.scene-seal { border: 1px solid #94563f80; color: var(--accent); padding: 12px 9px; font-family: var(--font-serif); font-size: 19px; transform: rotate(3deg); }.scene-seal span { display: block; }
.reading { max-width: 796px; margin: auto; padding: 0 24px 32px; min-height: 30vh; }.history-label { border-top: 1px solid #39493535; padding-top: 16px; color: var(--muted); font-size: 11px; letter-spacing: 2px; display: flex; justify-content: space-between; }.history-label span { letter-spacing: 0; }.quiet { padding: 24px 0; color: var(--muted); font-family: var(--font-serif); font-size: 18px; line-height: 2; }.quiet > span { color: var(--accent); }.turn { position: relative; padding: 22px 0 22px 32px; border-bottom: 1px solid #39493518; animation: arrive .3s ease-out; }.turn-mark { position: absolute; left: 0; top: 29px; color: #7b8073; font: 10px var(--font-mono); }.turn p { font-family: var(--font-serif); font-size: 18px; line-height: 1.95; white-space: pre-wrap; overflow-wrap: anywhere; margin: 8px 0; }.turn .your-words { color: var(--accent); font-size: 16px; }
.composer { position: sticky; bottom: 0; background: #f1eee5fa; border-top: 1px solid #39493525; padding: 12px 24px max(14px, env(safe-area-inset-bottom)); }.composer-inner { max-width: 748px; margin: auto; }.actions { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 10px; font-size: 13px; }.actions button { padding: 7px 10px; }.speech { display: flex; align-items: center; gap: 14px; border: 1px solid #8b9487; background: #fff7; padding: 10px; }.speech textarea { flex: 1; min-width: 0; resize: vertical; max-height: 180px; padding: 4px; background: none; border: 0; font: 15px/1.8 var(--font-body); color: var(--text); }.footnote { display: flex; justify-content: space-between; font-size: 10px; color: var(--muted); margin: 9px 0 0; }.destinations { display: flex; flex-wrap: wrap; gap: 8px; padding: 8px 0 16px; font-size: 13px; }.destinations button { border-bottom: 1px solid #7e493550; }.error { color: #99372f; font-size: 13px; overflow-wrap: anywhere; }.notice { color: #425c39; font-size: 13px; }.recover { color: var(--accent) !important; text-decoration: underline; }.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
@keyframes arrive { from { opacity: 0; transform: translateY(5px); } to { opacity: 1; transform: none; } }
@media (prefers-reduced-motion: reduce) { .turn { animation: none; } }
@media (max-width: 600px) { .play-header { padding: 18px 20px; }.wordmark small { display: none; }.mode { font-size: 10px; }.scene { padding: 30px 24px 14px; }.scene-seal { font-size: 15px; padding: 8px; }.reading { padding-inline: 24px; }.presence > span { padding: 0 4px; }.composer { padding-inline: 16px; }.speech { gap: 6px; }.speech button.primary { padding: 10px 14px; }.footnote span:last-child { display: none; }.turn p { font-size: 17px; }.actions { justify-content: space-between; gap: 0; }.actions button { padding-inline: 6px; }.history-label { letter-spacing: 1px; }.arrival { margin-top: 4vh; } }
</style>
