<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import PlayIcon from './PlayIcon.vue'
import PlaySymbols from './PlaySymbols.vue'
import PlayComposer from './PlayComposer.vue'
import PlayTranscript from './PlayTranscript.vue'
import '../styles/play.css'
import '../styles/play-integration.css'
// Feature sheets load on demand instead of inflating the first paint.
const PlayWallet = defineAsyncComponent(() => import('./PlayWallet.vue'))
const PlayContacts = defineAsyncComponent(() => import('./PlayContacts.vue'))
const PlayWork = defineAsyncComponent(() => import('./PlayWork.vue'))
const PlayMap = defineAsyncComponent(() => import('./PlayMap.vue'))
const PlayMessages = defineAsyncComponent(() => import('./PlayMessages.vue'))
const PlayObservatory = defineAsyncComponent(() => import('./PlayObservatory.vue'))
const PlayStyle = defineAsyncComponent(() => import('./PlayStyle.vue'))
const PlayModelSettings = defineAsyncComponent(() => import('./PlayModelSettings.vue'))
import type { LocalMap, MapMove } from '../lib/localMap'
import { readNarrativeStream, validateNarrativeComposition, type NarrativeCompositionMetadata } from '../lib/narrativeStream'
import { pendingStyleKey } from '../lib/narrativeStyle'
import type { StyleScope } from '../lib/narrativeStyle'
import type { WalletSnapshot } from '../lib/wallet'
import { providerCallSummary, type ProviderCall } from '../lib/providerReceipt'
import { getActiveProfile, profileModelOverride, type ApiProfile } from '../lib/apiProfiles'

type Person = { entity_id: string; display_name: string }
type Turn = NarrativeCompositionMetadata & { turn_run_id: string; narrative_lines: string[]; event_ids?: string[]; can_regenerate: boolean; provider_calls?: ProviderCall[] }
type SceneObjectAction = 'open' | 'close' | 'switch_on' | 'switch_off'
type SceneObject = { object_id: string; key: string; display_name: string; kind: 'door' | 'container' | 'light'; state: 'open' | 'closed' | 'on' | 'off'; actions: SceneObjectAction[] }
type Observation = LocalMap & {
  decision_mode: 'deterministic' | 'chat_completions'
  narrative_mode: 'deterministic' | 'style_planner' | 'full_prose' | 'custom'
  session_id: string; controlled_entity: Person
  present_entities: Person[]
  scene_objects: SceneObject[]
  recent_turns: Turn[]
}
type Pending = { path: string; body: Record<string, unknown>; narrative_turn_id?: string; interaction_status?: string }
type InteractionResult = { status: string; plan_kind: string; clarification?: string; pause_reason?: string; interpretation_source: 'model' | 'offline_rules' | 'legacy_rules'; interpretation_attempts: number; outcomes: { kind: string; turn_run_id?: string; world_time?: string }[] }
type Binding = { instance_id: string; branch_id: string; entity_id: string }
type AvailableBinding = Binding & { display_name: string }
type Discovery = { bindings: AvailableBinding[]; next_after?: Binding }
type Session = StyleScope & { controlled_entity_id: string }
type Bookmark = { session: string; openKey: string; pending: Pending | null; binding?: Binding; opening?: boolean }
type Theme = 'claude' | 'cream' | 'sage' | 'white'
type View = 'location' | 'people' | 'environment' | 'world' | 'history' | 'tools' | 'models' | 'mode' | 'settings' | 'appearance' | 'reader' | 'thinking' | 'rename' | 'chapter' | 'clear'
type Appearance = { theme: Theme; font: 'serif' | 'sans'; fontSize: number; titles: Record<string, string>; pins: string[] }
const themes: { id: Theme; name: string; description: string; paper: string; surface: string; accent: string }[] = [
  { id: 'claude', name: 'Claude 风格', description: '暖米灰底色 · 陶土点缀', paper: '#F2F0E9', surface: '#FAF9F6', accent: '#9C5138' },
  { id: 'cream', name: '奶油纸', description: '浅杏纸色 · 温和棕灰', paper: '#F3EDDE', surface: '#FBF7ED', accent: '#895B31' },
  { id: 'sage', name: '雾青', description: '浅灰绿底色 · 低饱和青色', paper: '#EDF1EC', surface: '#F8FAF6', accent: '#39756A' },
  { id: 'white', name: '纯净白', description: '中性浅白 · 克制灰阶', paper: '#F7F7F5', surface: '#FFFFFF', accent: '#596958' }
]
const appearanceKey = 'corerp.play.appearance.v1'
const appearance = ref<Appearance>({ theme: 'claude', font: 'serif', fontSize: 17, titles: {}, pins: [] })
try {
  const saved = JSON.parse(localStorage.getItem(appearanceKey) || 'null')
  if (saved && typeof saved === 'object') {
    if (themes.some(t => t.id === saved.theme)) appearance.value.theme = saved.theme
    if (saved.font === 'serif' || saved.font === 'sans') appearance.value.font = saved.font
    if (Number.isInteger(saved.fontSize)) appearance.value.fontSize = Math.max(15, Math.min(21, saved.fontSize))
    if (saved.titles && typeof saved.titles === 'object' && !Array.isArray(saved.titles))
      appearance.value.titles = Object.fromEntries(Object.entries(saved.titles).filter(([key, value]) => key.length < 300 && typeof value === 'string' && value.length <= 48)) as Record<string, string>
    if (Array.isArray(saved.pins)) appearance.value.pins = saved.pins.filter((value: unknown) => typeof value === 'string' && value.length < 300)
  }
} catch { /* Browser preferences do not grant world authority. */ }
watch(appearance, value => {
  try { localStorage.setItem(appearanceKey, JSON.stringify(value)) } catch { /* Keep this page usable without preference storage. */ }
  const color = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
  if (color) color.content = themes.find(t => t.id === value.theme)?.paper || themes[0]!.paper
}, { deep: true, immediate: true })
const storageKey = 'corerp.play.v1'
const token = ref('') // Credentials deliberately live only in this page's memory.
const activeProfile = ref<ApiProfile | null>(null)
const activeModelName = computed(() => activeProfile.value ? (activeProfile.value.model || activeProfile.value.name) : '')
function refreshActiveProfile() {
  activeProfile.value = getActiveProfile()
}
const bindings = ref<AvailableBinding[]>([])
const nextBinding = ref<Binding | undefined>()
const discovered = ref(false)
const choosingWorld = ref(false)
const preferWorldPicker = ref(new URLSearchParams(location.search).get('choose_world') === '1')
watch(token, () => { bindings.value = []; nextBinding.value = undefined; discovered.value = false })
const observation = ref<Observation | null>(null)
const busy = ref(false)
const error = ref('')
const errorSentence = computed(() => /[。！？!?．.]$/.test(error.value.trim()) ? error.value.trim() : `${error.value.trim()}。`)
const notice = ref('')
const draft = ref('')
const inputMode = ref<'speech' | 'AUTO' | 'DIALOGUE' | 'SCENE'>('AUTO')
const travel = ref(false)
const walletOpen = ref(false)
const contactsOpen = ref(false)
const workOpen = ref(false)
const mapOpen = ref(false)
const messagesOpen = ref(false)
const observatoryOpen = ref(false)
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
try { stylePending.value = !!bookmark.value.session && !!localStorage.getItem(pendingStyleKey(bookmark.value.session)) }
catch { error.value = '浏览器本地存储不可用，无法安全保存恢复请求。' }
const clock = computed(() => observation.value?.world_time.slice(11, 16) || '—')
const date = computed(() => observation.value?.world_time.slice(0, 10) || '')
const locked = computed(() => busy.value || !!bookmark.value.pending || stylePending.value)
const canChooseWorld = computed(() => !locked.value && !bookmark.value.opening && (!bookmark.value.session || !!bookmark.value.binding))
const bindingKey = (binding: Binding) => JSON.stringify([binding.instance_id, binding.branch_id, binding.entity_id])
const archiveKey = (binding: Binding) => `${storageKey}.binding.${bindingKey(binding)}`

function save() {
  // Persist intent BEFORE sending, so a lost response can be retried with the same key.
  localStorage.setItem(storageKey, JSON.stringify(bookmark.value))
  if (bookmark.value.binding) localStorage.setItem(archiveKey(bookmark.value.binding), JSON.stringify(bookmark.value))
}
async function api<T>(path: string, body: Record<string, unknown>): Promise<T> {
  // A single turn may ask three NPCs in sequence, with one bounded retry each.
  // Wait for the committed result; the saved idempotency key still handles a
  // lost response without submitting the player's action again.
  const model = body.model as { timeout_seconds?: number } | undefined
  const perCallSeconds = Math.min(120, Math.max(15, Number(model?.timeout_seconds) || 60))
  const timeoutMs = MODEL_PATHS.has(path) ? Math.min(600000, 30000 + 6 * perCallSeconds * 1000) : 45000
  const response = await fetch(`/api/v1/rp/${path}`, {
    method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token.value}` },
    body: JSON.stringify(body), signal: AbortSignal.timeout(timeoutMs), credentials: 'omit', redirect: 'error', cache: 'no-store'
  })
  const envelope = await response.json()
  if (!response.ok) throw Object.assign(new Error(envelope.error?.message || `连接失败 (${response.status})`), { code: envelope.error?.code })
  return envelope.data as T
}
async function refresh() {
  const next = await api<Observation>('observe', { session_id: bookmark.value.session })
  for (const turn of next.recent_turns) validateNarrativeComposition({ ...turn, lines: turn.narrative_lines })
  observation.value = next
  variants.value = {} // The server's selected render is the durable display truth.
}
async function streamNarrative(body: Record<string, unknown>, preview: (lines: string[]) => void, signal: AbortSignal) {
  const model = profileModelOverride(activeProfile.value)
  const response = await fetch('/api/v1/rp/narrative/stream', {
    method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token.value}` },
    // Full Prose permits up to three 90s provider attempts. Keep the browser
    // alive through that bounded server window; the user's stop action still
    // aborts via the separate controller.
    body: JSON.stringify(model ? { ...body, model } : body), signal: AbortSignal.any([signal, AbortSignal.timeout(300000)])
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
function readObservatory(before: number) {
  return api<{
    world_time: string; current_place: string; next_before_sequence?: number
    projection_health: { status: string; difference_count: number; checked_through_sequence: number }
    background_health: { enabled: boolean; status: string; last_target_world_time?: string; processed_items?: number; updated_at_utc?: string }
    traces: { trace_id: string; sequence: number; world_time?: string; place_name?: string; stage: string; execution_mode: string; responder_limit: number; player_action?: string; activations: { actor_ref: string; display_name: string; disposition: string; reason_code: string; activation_rank?: number }[]; committed_actions: { actor_ref: string; display_name: string; action: string; detail?: string; place_name?: string; activity?: string }[]; narrative_fallback: { used: boolean; reason_code?: string; provider_mode?: string } }[]
  }>('observatory/read', { session_id: bookmark.value.session, before_sequence: before, limit: 20 })
}
function moveFromMap(request: MapMove) {
  mapOpen.value = false
  const { journey, ...movement } = request
  void act(journey ? 'journeys/start' : 'actions/move', movement)
}
const sceneObjectActionLabels: Record<SceneObjectAction, string> = { open: '打开', close: '关上', switch_on: '开灯', switch_off: '关灯' }
const sceneObjectStateLabels: Record<SceneObject['state'], string> = { open: '已打开', closed: '已关闭', on: '亮着', off: '熄灭' }
function interactSceneObject(objectID: string, action: SceneObjectAction) {
  void act('actions/object', { object_id: objectID, action })
}
async function guarded(work: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try { await work() } catch (e) {
    error.value = e instanceof Error ? e.message : '暂时无法连接世界，请重试。'
  } finally { busy.value = false }
}
async function discoverBindings(more = false) {
  const result = await api<Discovery>('bindings/list', { limit: 20, ...(more && nextBinding.value ? { after: nextBinding.value } : {}) })
  bindings.value = more ? [...bindings.value, ...result.bindings] : result.bindings
  nextBinding.value = result.next_after
  discovered.value = true
}
async function enterWorld() {
    save()
    if (bookmark.value.session) {
      const session = await api<Session>('sessions/resume', { session_id: bookmark.value.session })
      sessionScope.value = session
      bookmark.value.binding = { instance_id: session.instance_id, branch_id: session.branch_id, entity_id: session.controlled_entity_id }
      save()
    } else {
      if (!bookmark.value.binding) throw new Error('请先选择获授权的世界与人物。')
      // Freeze the binding and key before the request: a lost response must
      // never let the same key be reused for a different world.
      bookmark.value.opening = true; save()
      const session = await api<Session>('sessions/open', {
        ...bookmark.value.binding,
        pov: 'second_person', idempotency_key: bookmark.value.openKey
      })
      bookmark.value.session = session.session_id; bookmark.value.opening = false; save()
      sessionScope.value = session
    }
    stylePending.value = !!localStorage.getItem(pendingStyleKey(bookmark.value.session))
    if (bookmark.value.pending) await finishPending()
    else await refresh()
    choosingWorld.value = false
    preferWorldPicker.value = false
    await scrollToEnd()
}
async function selectBinding(binding: Binding) {
  binding = { instance_id: binding.instance_id, branch_id: binding.branch_id, entity_id: binding.entity_id }
  if (bookmark.value.binding && bindingKey(bookmark.value.binding) === bindingKey(binding)) {
    await enterWorld(); return
  }
  if (bookmark.value.pending || bookmark.value.opening || stylePending.value) throw new Error('请先恢复未完成的会话或行动，再切换世界。')
  save()
  let saved: Bookmark | null = null
  try { saved = JSON.parse(localStorage.getItem(archiveKey(binding)) || 'null') } catch { /* Ignore malformed local data. */ }
  bookmark.value = saved && typeof saved.session === 'string' && typeof saved.openKey === 'string' && saved.binding && bindingKey(saved.binding) === bindingKey(binding)
    ? saved : { session: '', openKey: crypto.randomUUID(), pending: null, binding }
  observation.value = null; variants.value = {}; sessionScope.value = null
  draft.value = ''; socialSeeking.value = false; travel.value = false
  await enterWorld()
}
async function chooseWorld() {
  if (!canChooseWorld.value) return
  choosingWorld.value = true
  await guarded(() => discoverBindings())
}
async function connect() {
  await guarded(async () => {
    if (preferWorldPicker.value && !bookmark.value.pending && !bookmark.value.opening && !stylePending.value) {
      // Upgrade a legacy bookmark before archiving it. Never silently overwrite
      // an old session whose binding has not yet been confirmed by the server.
      if (bookmark.value.session && !bookmark.value.binding) {
        const session = await api<Session>('sessions/resume', { session_id: bookmark.value.session })
        bookmark.value.binding = { instance_id: session.instance_id, branch_id: session.branch_id, entity_id: session.controlled_entity_id }; save()
      }
      await discoverBindings()
      if (bindings.value.length === 1 && !nextBinding.value) await selectBinding(bindings.value[0]!)
      return
    }
    if (bookmark.value.session || bookmark.value.opening) { await enterWorld(); return }
    await discoverBindings()
    if (bindings.value.length === 1 && !nextBinding.value) await selectBinding(bindings.value[0]!)
  })
}
const scrollport = ref<HTMLElement | null>(null)
const dock = ref<HTMLElement | null>(null)
const app = ref<HTMLElement | null>(null)
const compact = ref(false), nearBottom = ref(true)
async function scrollToEnd() { await nextTick(); scrollport.value?.scrollTo({ top: scrollport.value.scrollHeight }) }
function onScroll() {
  const pane = scrollport.value
  if (!pane) return
  compact.value = pane.scrollTop > 40 || (compact.value && pane.scrollTop > 16)
  nearBottom.value = pane.scrollHeight - pane.clientHeight - pane.scrollTop < 75
}
function readableLine(line: string) {
  return line.replace(/(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}):\d{2}Z/g, '$1 $2')
}
async function finishPending(useSavedNarrative = false) {
  const pending = bookmark.value.pending
  if (!pending) return
  if (!MODEL_PATHS.has(pending.path) && 'model' in pending.body) { delete pending.body.model; save() }
  if (useSavedNarrative && !pending.narrative_turn_id) return
  let completedPresentation: { lines: string[]; warnings: string[] } | null = null
  let interpretationSource = ''
  let interactionKind = ''
  let interactionAction = ''
  if (!pending.narrative_turn_id) {
    const result = await api<{ status?: string; turn_run_id?: string } & Partial<InteractionResult>>(pending.path, pending.body)
    if (result.status === 'budget_exhausted') {
      notice.value = '时间正在推进，点击“继续未完成的行动”即可接着等待。'
      await refresh(); return
    }
    if (pending.path === 'interactions/run') {
      interpretationSource = result.interpretation_source || ''
      interactionKind = result.plan_kind || ''
      interactionAction = result.outcomes?.slice(-1)[0]?.kind || ''
      if (result.status === 'paused') {
        pending.interaction_status = 'paused'; save()
        notice.value = result.pause_reason || '原计划已暂停。已发生的行动不会回滚；可结束计划后重新选择。'
        await refresh(); return
      }
      if (result.status === 'clarification') {
        notice.value = (interpretationSource === 'model' ? '模型理解 · ' : '有限离线规则 · ') + (result.clarification || '请明确下一步行动。')
        await refresh()
        bookmark.value.pending = null; save()
        return
      }
      if (result.status !== 'settled') throw new Error('交互尚未确认结束，请继续原请求。')
      const speech = result.outcomes?.slice().reverse().find(outcome => outcome.kind === 'speech')
      if (speech?.turn_run_id) {
        pending.narrative_turn_id = speech.turn_run_id
        save() // Do not rerun the interaction if the narrative stream fails.
      }
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
  if (pending.narrative_turn_id && useSavedNarrative) {
    await api('narrative/select', { session_id: bookmark.value.session, turn_run_id: pending.narrative_turn_id, render_id: '' })
  }
  // Keep intent until fresh world/history is loaded; even a failed observation is retry-safe.
  await refresh()
  if (completedPresentation && pending.narrative_turn_id) setVariant(pending.narrative_turn_id, completedPresentation.lines)
  bookmark.value.pending = null; save()
  if (useSavedNarrative) notice.value = '已读取保存的原叙述，没有重做行动或调整世界事实。'
  if (pending.path === 'actions/wait') socialSeeking.value = false
  if (pending.path === 'turns/run' || pending.path === 'interactions/run') draft.value = ''
  if (pending.path === 'sessions/chapter/start') {
    variants.value = {}; turnPreview.value = null; readerTurn.value = ''; draft.value = ''
    notice.value = '新篇已开始。旧对话不再进入当前记录和后续叙事窗口；世界事实与人物记忆仍然保留。'
  }
  if (pending.path === 'interactions/run' && !pending.narrative_turn_id) notice.value = interactionKind === 'CONTINUE' ? '世界时间已推进 15 分钟；没有生成你的台词。' : interactionAction === 'nonverbal' ? '无声动作已提交；没有生成台词，也未推进世界时间。' : interactionAction === 'object' ? '物件行动已提交；提出、同意和实际交付是不同事实。' : '行动已按顺序完成。'
  else if (pending.path !== 'turns/run' && pending.path !== 'interactions/run' && pending.path !== 'sessions/chapter/start') {
    notice.value = pending.path === 'actions/move' ? '你已抵达新的地点。'
      : pending.path === 'actions/nonverbal' ? '无声动作已提交，世界时间没有推进。'
      : pending.path === 'actions/object' ? '场景中的物件状态已经改变。'
      : pending.path === 'journeys/start' ? '你已进入真实路段；世界时间继续推进后才会抵达。'
      : pending.path === 'journeys/cancel' ? '已停止自动抵达；你仍在当前路段。'
      : '时间已经向前。'
  }
  if (completedPresentation?.warnings.length) notice.value = '叙述器提示：' + completedPresentation.warnings.join('；')
  if (interpretationSource === 'offline_rules') notice.value = [notice.value, '本轮使用有限离线规则，并非模型语义理解。'].filter(Boolean).join(' · ')
  travel.value = false
  await scrollToEnd()
}
// Only provider-invoking paths accept a model override; purely deterministic
// endpoints (move, scene objects, journeys) reject unknown fields under strict decoding.
const MODEL_PATHS = new Set(['interactions/run', 'interactions/resume', 'turns/run', 'turns/resume', 'actions/wait'])
async function act(path: string, values: Record<string, unknown>) {
  if (!observation.value || locked.value) return
  const model = MODEL_PATHS.has(path) ? profileModelOverride(activeProfile.value) : undefined
  await guarded(async () => {
    bookmark.value.pending = { path, body: {
      session_id: bookmark.value.session, expected_cursor: observation.value!.observation_cursor,
      idempotency_key: crypto.randomUUID(), ...values, ...(model ? { model } : {})
    } }
    save(); await finishPending()
  })
}
function speak() {
  if (!draft.value.trim()) return
  if (inputMode.value === 'speech') void act('turns/run', { text: draft.value.trim() })
  else void act('interactions/run', { text: draft.value.trim(), interaction_mode: inputMode.value })
}
async function stopInteraction() {
  const pending = bookmark.value.pending
  if (!pending || pending.path !== 'interactions/run' || pending.narrative_turn_id) return
  const result = await api<InteractionResult>('interactions/stop', { session_id: pending.body.session_id, idempotency_key: pending.body.idempotency_key })
  if (result.status === 'settled') { await finishPending(); return }
  if (result.status !== 'stopped' && result.status !== 'clarification') throw new Error('原计划状态尚未确定，请保留请求并继续恢复。')
  await refresh()
  bookmark.value.pending = null; save()
  notice.value = result.status === 'stopped' ? '已结束原计划；已完成的行动仍保留在世界里。' : result.clarification || '请明确下一步。'
}
function wait(hours: number) {
  if (!observation.value) return
  const target = new Date(Date.parse(observation.value.world_time) + hours * 3600000).toISOString().replace('.000Z', 'Z')
  void act('actions/wait', { target_world_time: target, budget: 100, ...(socialSeeking.value ? { opportunity_intent: 'social' } : {}) })
}
// 「继续剧情」is a short explicit wait: warm and initiative work drains through
// the same committed wait event as any other time advance.
function continueStory() {
  if (!observation.value) return
  const target = new Date(Date.parse(observation.value.world_time) + 15 * 60000).toISOString().replace('.000Z', 'Z')
  void act('actions/wait', { target_world_time: target, budget: 100, opportunity_intent: 'social' })
}

const title = computed(() => appearance.value.titles[bookmark.value.binding ? bindingKey(bookmark.value.binding) : ''] || observation.value?.place_name || 'CoreRP')
const pinned = computed(() => !!bookmark.value.binding && appearance.value.pins.includes(bindingKey(bookmark.value.binding)))
const listedBindings = computed(() => [...bindings.value].sort((a, b) => Number(appearance.value.pins.includes(bindingKey(b))) - Number(appearance.value.pins.includes(bindingKey(a)))))
const status = computed(() => {
  if (turnPreview.value !== null) return '行动已提交 · 正在接收叙述'
  if (busy.value && bookmark.value.pending?.narrative_turn_id) return '行动已提交 · 正在读取叙述'
  if (busy.value && bookmark.value.pending) return '请求已发出 · 等待服务器确认'
  if (bookmark.value.pending?.interaction_status === 'paused') return '计划已暂停 · 已发生的行动仍保留'
  if (bookmark.value.pending) return '结果未确认 · 请继续原请求'
  if (busy.value) return '正在读取当前观察'
  return ''
})
const overlay = ref<'drawer' | 'menu' | 'sheet' | null>(null)
const view = ref<View>('location')
const sheetTitles: Record<View, string> = { location: '地点与可前往的区域', people: '在场人物', environment: '环境与状态', world: '世界', history: '故事记录', tools: '行动与功能', models: '模型与 API 设置', mode: '输入方式', settings: '阅读与故事设置', appearance: '主题与外观', reader: '完整剧情', thinking: '公开状态摘要', rename: '重命名显示名称', chapter: '开始新篇？', clear: '清除此设备的会话？' }
const sheetStack = ref<{ view: View; turn: string }[]>([])
const readerTurn = ref('')
const renameTitle = ref('')
const fullReader = ref(false)
const menu = ref<HTMLElement | null>(null)
const surface = ref<HTMLElement | null>(null)
let returnFocus: HTMLElement | null = null
function closeOverlay(restore = true) {
  if (!overlay.value) return
  const focus = returnFocus
  overlay.value = null; sheetStack.value = []; fullReader.value = false
  returnFocus = null
  if (restore) void nextTick(() => (focus?.isConnected ? focus : document.querySelector<HTMLElement>('.play-app .more-button'))?.focus({ preventScroll: true }))
}
async function showOverlay(which: 'drawer' | 'menu') {
  if (overlay.value === which) { closeOverlay(); return }
  if (!overlay.value) returnFocus = document.activeElement as HTMLElement
  overlay.value = which; sheetStack.value = []
  await nextTick()
  if (which === 'menu') positionMenu()
  const panel = which === 'menu' ? menu.value : surface.value
  panel?.focus({ preventScroll: true })
}
function openSheet(next: View, turn = '', nested = false) {
  if (!overlay.value) returnFocus = document.activeElement as HTMLElement
  if (nested && overlay.value === 'sheet') sheetStack.value.push({ view: view.value, turn: readerTurn.value })
  else sheetStack.value = []
  view.value = next; readerTurn.value = turn; fullReader.value = false
  if (next === 'rename') renameTitle.value = title.value
  overlay.value = 'sheet'
  void nextTick(() => surface.value?.focus({ preventScroll: true }))
}
function goBack() {
  const previous = sheetStack.value.pop()
  if (previous) { view.value = previous.view; readerTurn.value = previous.turn; fullReader.value = false; void nextTick(() => surface.value?.focus({ preventScroll: true })) }
  else closeOverlay()
}
function positionMenu() {
  const target = menu.value, anchor = document.querySelector<HTMLElement>('.play-app .more-button'), root = app.value
  if (!target || !anchor || !root) return
  const a = root.getBoundingClientRect(), b = anchor.getBoundingClientRect()
  target.style.left = `${Math.max(9, Math.min(b.right - a.left - target.offsetWidth, root.clientWidth - target.offsetWidth - 9))}px`
  target.style.top = `${Math.max(7, Math.min(b.bottom - a.top + 3, root.clientHeight - target.offsetHeight - 9))}px`
}
function onOverlayKey(event: KeyboardEvent) {
  if (!overlay.value) return
  if (event.key === 'Escape') { event.preventDefault(); overlay.value === 'sheet' ? goBack() : closeOverlay(); return }
  const panel = overlay.value === 'menu' ? menu.value : surface.value
  if (!panel) return
  const items = [...panel.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), textarea:not(:disabled), a[href], [tabindex="0"]')].filter(el => el.tabIndex >= 0 && el.getClientRects().length > 0)
  if (overlay.value === 'menu' && ['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
    event.preventDefault()
    const index = items.indexOf(document.activeElement as HTMLElement)
    items[event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : event.key === 'ArrowDown' ? (index + 1) % items.length : (index + items.length - 1) % items.length]?.focus()
  }
  if (event.key !== 'Tab') return
  if (!items.length) { event.preventDefault(); return }
  const index = items.indexOf(document.activeElement as HTMLElement)
  if (event.shiftKey && index <= 0) { event.preventDefault(); items.at(-1)?.focus() }
  else if (!event.shiftKey && (index === items.length - 1 || index === -1)) { event.preventDefault(); items[0]?.focus() }
}
function openBusiness(which: 'wallet' | 'contacts' | 'work' | 'map' | 'messages' | 'observatory' | 'style') {
  closeOverlay(false)
  if (which === 'wallet') walletOpen.value = true
  if (which === 'contacts') contactsOpen.value = true
  if (which === 'work') workOpen.value = true
  if (which === 'map') mapOpen.value = true
  if (which === 'messages') messagesOpen.value = true
  if (which === 'observatory') observatoryOpen.value = true
  if (which === 'style') styleOpen.value = true
}
function closeBusiness(which: 'wallet' | 'contacts' | 'work' | 'map' | 'messages' | 'observatory' | 'style') {
  if (which === 'wallet') walletOpen.value = false
  if (which === 'contacts') contactsOpen.value = false
  if (which === 'work') workOpen.value = false
  if (which === 'map') mapOpen.value = false
  if (which === 'messages') messagesOpen.value = false
  if (which === 'observatory') observatoryOpen.value = false
  if (which === 'style') styleOpen.value = false
  void nextTick(() => document.querySelector<HTMLElement>('.play-app .composer-tools [aria-label="行动与功能"]')?.focus({ preventScroll: true }))
}
function selectPlace(to: string) {
  const place = observation.value?.reachable_places.find(item => item.place_id === to)
  if (!observation.value || !place || locked.value || observation.value.active_journey || (!place.can_move_now && !place.can_start_journey)) return
  const from = observation.value.place_id
  closeOverlay(false)
  void act(place.can_start_journey ? 'journeys/start' : 'actions/move', { from_place_id: from, to_place_id: to })
}
function switchWorld() { if (canChooseWorld.value) { closeOverlay(false); void chooseWorld() } }
function togglePin() {
  const key = bookmark.value.binding && bindingKey(bookmark.value.binding)
  if (!key) return
  appearance.value.pins = pinned.value ? appearance.value.pins.filter(id => id !== key) : [...appearance.value.pins, key]
  closeOverlay()
}
function rename() {
  const key = bookmark.value.binding && bindingKey(bookmark.value.binding), name = renameTitle.value.trim().slice(0, 48)
  if (!key || !name) return
  appearance.value.titles[key] = name
  closeOverlay()
}
function exportHistory() {
  if (!observation.value) return
  const lines = [`# ${title.value}`, '', '> CoreRP · 当前观察到的最近 50 段叙述；不是完整世界档案。若本页正在展示手动重新生成的版本，导出内容与当前画面一致。', '']
  for (const turn of observation.value.recent_turns) lines.push('## 世界记录', '', ...(variants.value[turn.turn_run_id] || turn.narrative_lines), '')
  const file = new Blob(['﻿' + lines.join('\n')], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(file), anchor = document.createElement('a')
  anchor.href = url; anchor.download = `${title.value.replace(/[\\/:*?"<>|\u0000-\u001f]/g, '_') || 'CoreRP'}.md`
  document.body.append(anchor); anchor.click(); anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 10000)
  closeOverlay()
}
function clearLocalSession() {
  if (locked.value || bookmark.value.opening) return
  const key = bookmark.value.binding && archiveKey(bookmark.value.binding)
  try { localStorage.removeItem(storageKey); if (key) localStorage.removeItem(key) }
  catch { error.value = '无法清除此浏览器的会话记录，请检查存储权限。'; closeOverlay(); return }
  bookmark.value = { session: '', openKey: crypto.randomUUID(), pending: null }
  observation.value = null; sessionScope.value = null; variants.value = {}; draft.value = ''
  discovered.value = false; bindings.value = []; choosingWorld.value = false
  closeOverlay(false)
}
async function startNewChapter() {
  await act('sessions/chapter/start', {})
  if (!bookmark.value.pending) closeOverlay()
}
const selectedTurn = computed(() => observation.value?.recent_turns.find(turn => turn.turn_run_id === readerTurn.value))
function changeFontSize(delta: number) { appearance.value.fontSize = Math.max(15, Math.min(21, appearance.value.fontSize + delta)) }
function themeKey(event: KeyboardEvent) {
  if (!['ArrowDown', 'ArrowUp', 'ArrowRight', 'ArrowLeft', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const index = themes.findIndex(theme => theme.id === appearance.value.theme)
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? themes.length - 1
    : (index + (event.key === 'ArrowUp' || event.key === 'ArrowLeft' ? -1 : 1) + themes.length) % themes.length
  appearance.value.theme = themes[next]!.id
  void nextTick(() => document.querySelector<HTMLElement>('.play-app .theme-choice[aria-checked="true"]')?.focus({ preventScroll: true }))
}
let sheetDrag: { y: number; id: number } | null = null
function startSheetDrag(event: PointerEvent) {
  if (fullReader.value || event.button !== 0) return
  if (surface.value) surface.value.style.transition = 'none'
  sheetDrag = { y: event.clientY, id: event.pointerId }
  const handle = event.currentTarget as HTMLElement
  handle.setPointerCapture(event.pointerId)
}
function moveSheetDrag(event: PointerEvent) {
  if (sheetDrag?.id === event.pointerId && surface.value) surface.value.style.transform = `translateY(${Math.max(0, event.clientY - sheetDrag.y)}px)`
}
function endSheetDrag(event: PointerEvent) {
  if (!sheetDrag || sheetDrag.id !== event.pointerId) return
  const distance = event.clientY - sheetDrag.y
  sheetDrag = null
  if (distance > 85) {
    if (surface.value) {
      surface.value.style.transition = 'transform 240ms cubic-bezier(0.25, 0.1, 0.25, 1)'
      surface.value.style.transform = 'translate3d(0, 100%, 0)'
    }
    closeOverlay()
  } else if (surface.value) {
    surface.value.style.transition = 'transform 260ms cubic-bezier(0.16, 1, 0.3, 1)'
    surface.value.style.transform = ''
    window.setTimeout(() => { if (surface.value) surface.value.style.transition = '' }, 260)
  }
}
let dockObserver: ResizeObserver | null = null
let initialHeight = window.innerHeight
function measureDock() {
  if (!app.value || !dock.value) return
  const wasAtBottom = !scrollport.value || scrollport.value.scrollHeight - scrollport.value.clientHeight - scrollport.value.scrollTop < 16
  const bottom = parseFloat(getComputedStyle(dock.value).bottom) || 25
  app.value.style.setProperty('--dock-clearance', `${Math.ceil(dock.value.offsetHeight + bottom + 24)}px`)
  if (wasAtBottom) void scrollToEnd()
  onScroll()
}
function viewportChanged() {
  if (!app.value) return
  const vv = window.visualViewport
  if (vv && Math.abs(vv.scale - 1) < .05) {
    app.value.style.setProperty('--app-height', `${vv.height}px`)
    app.value.style.setProperty('--app-top', `${vv.offsetTop}px`)
    const editing = ['INPUT', 'TEXTAREA'].includes(document.activeElement?.tagName || '')
    app.value.classList.toggle('keyboard-open', editing && vv.height < initialHeight * .8)
  } else if (!vv) app.value.style.setProperty('--app-height', `${window.innerHeight}px`)
  requestAnimationFrame(() => { measureDock(); if (overlay.value === 'menu') positionMenu() })
}
function orientationChanged() { window.setTimeout(() => { initialHeight = window.innerHeight; viewportChanged() }, 250) }
watch(dock, node => { dockObserver?.disconnect(); if (node && window.ResizeObserver) { dockObserver = new ResizeObserver(measureDock); dockObserver.observe(node) } void nextTick(measureDock) })
onMounted(() => {
  refreshActiveProfile()
  window.addEventListener('storage', refreshActiveProfile)
  document.addEventListener('keydown', onOverlayKey)
  window.visualViewport?.addEventListener('resize', viewportChanged)
  window.visualViewport?.addEventListener('scroll', viewportChanged)
  window.addEventListener('resize', viewportChanged)
  window.addEventListener('orientationchange', orientationChanged)
  viewportChanged()
})
onBeforeUnmount(() => {
  dockObserver?.disconnect()
  window.removeEventListener('storage', refreshActiveProfile)
  document.removeEventListener('keydown', onOverlayKey)
  window.visualViewport?.removeEventListener('resize', viewportChanged)
  window.visualViewport?.removeEventListener('scroll', viewportChanged)
  window.removeEventListener('resize', viewportChanged)
  window.removeEventListener('orientationchange', orientationChanged)
})
</script>

<template>
  <div ref="app" class="play-app" :data-theme="appearance.theme" :style="{ '--reading-size': `${appearance.fontSize}px`, '--reading-font': appearance.font === 'sans' ? 'var(--ui)' : 'var(--serif)' }">
    <PlaySymbols />
    <template v-if="!observation || choosingWorld">
      <div class="entry-chrome"><strong>CoreRP</strong><span>一个持续发生的世界</span></div>
      <main class="arrival" aria-label="进入世界">
        <p class="entry-kicker">继续故事</p>
        <h1>回到世界里。</h1>
        <p class="entry-intro">一段对话，一次相遇。进入后，你只能操作这份凭证获授权的人物。</p>
        <form class="entry" @submit.prevent="connect">
          <label for="credential">玩家访问凭证</label>
          <input id="credential" v-model="token" type="password" autocomplete="off" required placeholder="输入本地服务提供的凭证" :disabled="busy">
          <p class="hint">凭证只在当前页面内存中使用，不会写入浏览器存储。</p>
          <button class="solid-button" :disabled="busy || !token.trim()">{{ busy ? '正在连接…' : bookmark.session ? '继续这段生活 →' : '进入世界 →' }}</button>
          <button v-if="bookmark.session && bookmark.binding" type="button" class="flat-button" :disabled="!canChooseWorld || !token.trim()" @click="chooseWorld">选择其他世界</button>
        </form>
        <section v-if="discovered" class="world-picker" aria-label="获授权的世界与人物">
          <h2>选择一段生活</h2>
          <p class="hint">仅列出这份玩家凭证获授权的人物。进入时仍会核对权限。</p>
          <p v-if="!bindings.length" role="status">暂时没有可进入的人物。请确认世界已保存并授予玩家控制权限。</p>
          <ul><li v-for="binding in listedBindings" :key="bindingKey(binding)">
            <button :disabled="busy || !!bookmark.pending || !!bookmark.opening || stylePending" @click="guarded(() => selectBinding(binding))"><strong>{{ binding.display_name }}</strong><span>{{ binding.instance_id }} / {{ binding.branch_id }}</span></button>
          </li></ul>
          <button v-if="nextBinding" :disabled="busy" @click="guarded(() => discoverBindings(true))">加载更多人物</button>
        </section>
        <p v-if="bookmark.opening" class="hint" role="status">已保留进入世界的原请求。继续只会重试原请求。</p>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
      </main>
    </template>
    <template v-else>
      <div class="workspace" :inert="!!overlay">
        <main ref="scrollport" class="scrollport reading" aria-label="故事对话" tabindex="0" @scroll.passive="onScroll">
          <PlayTranscript :turns="observation.recent_turns" :variants="variants" :preview="turnPreview" :place="observation.place_name" :session="bookmark.session" :locked="locked" :status="status" :player-name="observation.controlled_entity.display_name" :api="api" :stream="streamNarrative" @variant="(id, lines) => setVariant(id, lines)" @read="openSheet('reader', $event)" @thinking="openSheet('thinking')" />
        </main>
        <header class="top-chrome scene" :class="{ compact }">
          <div class="top-glass" aria-hidden="true" />
          <div class="header">
            <button class="icon-button" aria-label="打开侧边栏" aria-controls="play-drawer" :aria-expanded="overlay === 'drawer'" @click="showOverlay('drawer')"><PlayIcon name="menu" /></button>
            <div class="heading"><h1>{{ title }}</h1><p class="presence">你是 {{ observation.controlled_entity.display_name }}<span v-if="observation.present_entities.length"> · 此刻在场：{{ observation.present_entities.map(p => p.display_name).join('、') }}</span><span v-else> · 此刻只有你</span></p><span class="mode">{{ activeProfile || observation.decision_mode === 'chat_completions' ? '已配置 AI 人物 · 成功情况见调用回执' : '本地体验 · 确定性人物' }}</span></div>
            <button class="icon-button more-button" aria-label="故事操作" aria-haspopup="menu" aria-controls="play-menu" :aria-expanded="overlay === 'menu'" @click="showOverlay('menu')"><PlayIcon name="more" /></button>
          </div>
          <nav class="capsules" aria-label="当前场景">
            <button class="capsule" aria-haspopup="dialog" @click="openSheet('location')"><PlayIcon name="location" :size="13" /><span>{{ observation.place_name }}</span><PlayIcon name="down" :size="10" /></button>
            <button class="capsule" aria-haspopup="dialog" @click="openSheet('people')"><PlayIcon name="people" :size="13" /><span>在场 {{ observation.present_entities.length }}</span><PlayIcon name="down" :size="10" /></button>
            <button class="capsule" aria-haspopup="dialog" @click="openSheet('environment')"><PlayIcon name="moon" :size="13" /><time :datetime="observation.world_time">{{ date }} · {{ clock }}</time></button>
          </nav>
        </header>
        <div class="bottom-haze" aria-hidden="true" />
        <button v-if="!nearBottom" class="jump-button" aria-label="回到最新消息" @click="scrollToEnd"><PlayIcon name="down" :size="18" /></button>
        <div ref="dock" class="composer-dock">
          <div v-if="error || notice || stylePending || observation.active_journey || bookmark.pending" class="composer-alerts">
            <p v-if="error" class="error" role="alert">暂未完成：{{ errorSentence }}{{ bookmark.pending?.narrative_turn_id ? '行动已提交；重新读取不会重做行动。' : bookmark.pending ? '原请求已保留；重试不会重复执行。' : '请检查连接后重试。' }}</p>
            <p v-if="notice" class="notice" role="status">{{ notice }}</p>
            <p v-if="stylePending" class="notice" role="status">叙事设置尚未确认，请从“行动与功能”继续保存。</p>
            <p v-if="observation.active_journey" class="journey-status" role="status">正在途中 · 预计 {{ readableLine(observation.active_journey.scheduled_arrival_at) }} 抵达。<button :disabled="locked" @click="act('journeys/cancel', { journey_id: observation.active_journey.journey_id })">取消自动抵达</button></p>
            <div v-if="bookmark.pending" class="recovery-actions">
              <button v-if="bookmark.pending.interaction_status !== 'paused'" class="recover" :disabled="busy" @click="guarded(finishPending)">{{ busy ? '正在继续…' : bookmark.pending.narrative_turn_id ? '继续读取叙述' : '继续未完成的行动' }}</button>
              <button v-if="bookmark.pending.path === 'interactions/run' && !bookmark.pending.narrative_turn_id" class="recover" :disabled="busy" @click="guarded(stopInteraction)">{{ bookmark.pending.interaction_status === 'paused' ? '结束已暂停的计划' : '尝试结束原计划（已发生的不回滚）' }}</button>
              <button v-if="bookmark.pending.narrative_turn_id && !busy" class="recover" @click="guarded(() => finishPending(true))">读取已保存原叙述</button>
            </div>
          </div>
          <PlayComposer v-model:draft="draft" :locked="locked" :busy="busy" :streaming="turnPreview !== null" :decision-mode="observation.decision_mode" :input-mode="inputMode" :active-model-name="activeModelName" @send="speak" @refresh="guarded(refresh)" @abort="turnStreamController?.abort()" @open="openSheet" />
        </div>
      </div>
      <Transition name="scrim">
        <div v-if="overlay" class="scrim" :class="{ 'menu-scrim': overlay === 'menu' }" @click="closeOverlay()" />
      </Transition>
      <Transition name="drawer">
        <aside v-if="overlay === 'drawer'" id="play-drawer" ref="surface" class="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title" tabindex="-1">
          <div class="drawer-brand"><strong id="drawer-title">CoreRP</strong><button class="icon-button" aria-label="关闭侧边栏" @click="closeOverlay()"><PlayIcon name="close" /></button></div>
          <nav class="drawer-nav" aria-label="主导航">
            <button class="nav-item current" @click="closeOverlay()"><PlayIcon name="chat" />当前故事</button>
            <button class="nav-item" @click="openSheet('world')"><PlayIcon name="world" />世界</button>
            <button class="nav-item" @click="openSheet('people')"><PlayIcon name="people" />角色</button>
            <button class="nav-item" @click="openSheet('history')"><PlayIcon name="memory" />记忆</button>
            <button class="nav-item" @click="openSheet('tools')"><PlayIcon name="plus" />行动与功能</button>
          </nav>
          <div class="drawer-history"><h2 class="drawer-section-title">已授权的世界</h2><div class="recent-list">
            <button class="recent-item selected" aria-current="true" @click="closeOverlay()"><i class="recent-indicator" /><span>{{ title }}</span></button>
            <button v-for="binding in listedBindings.filter(b => bindingKey(b) !== (bookmark.binding && bindingKey(bookmark.binding)))" :key="bindingKey(binding)" class="recent-item" :disabled="!canChooseWorld" @click="closeOverlay(false); guarded(() => selectBinding(binding))"><i class="recent-indicator" /><span>{{ appearance.titles[bindingKey(binding)] || binding.display_name }}</span></button>
            <button class="recent-item" :disabled="!canChooseWorld" @click="switchWorld"><PlayIcon name="plus" :size="12" /><span>选择其他世界</span></button>
          </div></div>
          <div class="drawer-footer"><p class="local-label"><i />人物决策：{{ activeProfile ? (activeProfile.name + ' (' + activeProfile.model + ')') : (observation.decision_mode === 'chat_completions' ? '服务器配置的 AI' : '确定性规则') }}</p><button class="profile-button" @click="openSheet('settings')"><span class="profile-avatar">{{ observation.controlled_entity.display_name.slice(0, 1) }}</span><span class="profile-name"><b>{{ observation.controlled_entity.display_name }}</b><small>主题 · 阅读与界面</small></span><PlayIcon name="settings" :size="17" /></button><a class="new-story-button" href="/studio"><PlayIcon name="plus" :size="17" />在创作台创建世界</a></div>
        </aside>
      </Transition>
      <Transition name="menu">
        <div v-if="overlay === 'menu'" id="play-menu" ref="menu" class="popover" role="menu" aria-label="故事操作" tabindex="-1">
          <p class="menu-title">{{ title }}</p>
          <button class="menu-item" role="menuitem" @click="exportHistory"><PlayIcon name="export" :size="17" />导出当前记录</button>
          <button class="menu-item" role="menuitem" @click="openSheet('rename')"><PlayIcon name="edit" :size="17" />重命名显示名称</button>
          <button class="menu-item" role="menuitem" @click="togglePin"><PlayIcon name="pin" :size="17" />{{ pinned ? '取消置顶' : '置顶世界' }}</button>
          <button class="menu-item" role="menuitem" @click="openSheet('settings')"><PlayIcon name="settings" :size="17" />故事设置</button>
          <button class="menu-item" role="menuitem" @click="openSheet('history')"><PlayIcon name="history" :size="17" />故事记录</button>
          <button class="menu-item" role="menuitem" :disabled="locked || !!bookmark.opening" @click="openSheet('chapter')"><PlayIcon name="plus" :size="17" />开始新篇</button>
          <div class="menu-divider" role="separator" />
          <button class="menu-item danger" role="menuitem" :disabled="locked || !!bookmark.opening" @click="openSheet('clear')"><PlayIcon name="trash" :size="17" />清除此设备的会话</button>
        </div>
      </Transition>
      <Transition name="sheet">
        <section v-if="overlay === 'sheet'" ref="surface" class="sheet" :class="{ 'reader-full': fullReader, 'thinking-sheet': view === 'thinking' }" role="dialog" aria-modal="true" aria-labelledby="sheet-title" tabindex="-1">
          <div class="sheet-handle" aria-hidden="true" @pointerdown="startSheetDrag" @pointermove="moveSheetDrag" @pointerup="endSheetDrag" @pointercancel="endSheetDrag" />
          <header class="sheet-header">
            <button v-if="sheetStack.length" class="icon-button" aria-label="返回上一层" @click="goBack"><PlayIcon name="back" :size="19" /></button>
            <div class="sheet-title-group"><h2 id="sheet-title">{{ sheetTitles[view] }}</h2><p v-if="view === 'reader'">{{ title }}</p><p v-else-if="view === 'thinking'">只展示已确认的公开状态</p></div>
            <button v-if="view === 'reader'" class="icon-button" :aria-label="fullReader ? '退出全屏阅读' : '全屏阅读'" @click="fullReader = !fullReader"><PlayIcon name="book" :size="19" /></button>
            <button class="icon-button" aria-label="关闭详情" @click="closeOverlay()"><PlayIcon name="close" :size="19" /></button>
          </header>
          <div :key="view" class="sheet-content" tabindex="0">
            <template v-if="view === 'location'">
              <h3 class="section-heading">你的位置</h3><div class="detail-row"><span class="row-symbol"><PlayIcon name="location" :size="18" /></span><span class="row-copy"><strong>{{ observation.place_name }}</strong><small>{{ observation.active_journey ? '正在真实路段途中' : '当前已确认的位置' }}</small></span></div>
              <h3 class="section-heading">附近</h3><p v-if="!observation.reachable_places.length" class="detail-note">附近没有可前往的地点。</p>
              <button v-for="place in observation.reachable_places" :key="place.place_id" class="detail-row clickable" :disabled="locked || !!observation.active_journey || (!place.can_move_now && !place.can_start_journey)" @click="selectPlace(place.place_id)"><span class="row-symbol"><PlayIcon name="right" :size="18" /></span><span class="row-copy"><strong>{{ place.display_name }}</strong><small>{{ place.can_start_journey ? `约 ${place.travel_minutes} 分钟 · 启程后才会抵达` : place.can_move_now ? '当前可前往' : '当前不可通行' }}</small></span><PlayIcon name="right" :size="17" /></button>
              <p class="detail-note">选择可通行地点将提交真实移动或旅程；出发前会再次核对世界状态。</p>
              <button class="detail-row clickable" @click="openBusiness('map')"><span class="row-symbol"><PlayIcon name="map" :size="18" /></span><span class="row-copy"><strong>查看附近地图</strong><small>路线、施工与当前可达性</small></span></button>
            </template>
            <template v-else-if="view === 'people'">
              <p class="detail-note">只展示当前观察中可见的人物，不据外观推断其身份或想法。</p><p v-if="!observation.present_entities.length" class="detail-note">此刻没有其他人在场。</p>
              <div v-for="person in observation.present_entities" :key="person.entity_id" class="detail-row"><span class="row-symbol initial-symbol">{{ person.display_name.slice(0, 1) }}</span><span class="row-copy"><strong>{{ person.display_name }}</strong><small>当前场景中可见</small></span></div>
              <button class="flat-button" @click="openBusiness('contacts')">查看自己的通讯录</button>
            </template>
            <template v-else-if="view === 'environment'">
              <div class="detail-row"><span class="row-symbol"><PlayIcon name="moon" :size="18" /></span><span class="row-copy"><strong>{{ readableLine(observation.world_time) }}</strong><small>当前世界时间</small></span></div>
              <div class="detail-row"><span class="row-symbol"><PlayIcon name="location" :size="18" /></span><span class="row-copy"><strong>{{ observation.place_name }}</strong><small>当前位置</small></span></div>
              <template v-if="observation.scene_objects.length">
                <h3 class="section-heading">场景物件</h3>
                <div v-for="object in observation.scene_objects" :key="object.object_id" class="detail-row scene-object-row">
                  <span class="row-symbol"><PlayIcon name="world" :size="18" /></span>
                  <span class="row-copy"><strong>{{ object.display_name }}</strong><small>{{ sceneObjectStateLabels[object.state] }}</small></span>
                  <button v-for="action in object.actions" :key="action" class="flat-button scene-object-action" :disabled="locked" @click="interactSceneObject(object.object_id, action)">{{ sceneObjectActionLabels[action] }}</button>
                </div>
              </template>
              <p class="detail-note">只显示服务端确认、且你在当前位置能感知和操作的状态；不推断未观察到的环境。</p>
            </template>
            <template v-else-if="view === 'world'"><p class="detail-note">当前连接的是已授权的持久世界；本页只显示你的观察与最近记录。</p><div class="detail-row"><span class="row-symbol"><PlayIcon name="world" :size="18" /></span><span class="row-copy"><strong>{{ title }}</strong><small>你是 {{ observation.controlled_entity.display_name }} · {{ observation.place_name }}</small></span></div><button class="detail-row clickable" @click="openSheet('location', '', true)"><span class="row-symbol"><PlayIcon name="map" :size="18" /></span><span class="row-copy"><strong>位置与附近路线</strong></span><PlayIcon name="right" :size="17" /></button><button class="detail-row clickable" @click="openSheet('history', '', true)"><span class="row-symbol"><PlayIcon name="memory" :size="18" /></span><span class="row-copy"><strong>最近故事记录</strong></span><PlayIcon name="right" :size="17" /></button><button class="flat-button" :disabled="!canChooseWorld" @click="switchWorld">切换获授权的世界</button></template>
            <template v-else-if="view === 'history'"><p class="detail-note">显示服务端返回的最近 {{ observation.recent_turns.length }} 段；不是完整世界档案。</p><p v-if="!observation.recent_turns.length" class="detail-note">还没有故事记录。</p><button v-for="(turn, i) in observation.recent_turns" :key="turn.turn_run_id" class="timeline-entry clickable" @click="openSheet('reader', turn.turn_run_id, true)"><small>第 {{ i + 1 }} 段</small><p>{{ (variants[turn.turn_run_id] || turn.narrative_lines).join(' ').slice(0, 75) }}{{ (variants[turn.turn_run_id] || turn.narrative_lines).join(' ').length > 75 ? '…' : '' }}</p></button></template>
            <template v-else-if="view === 'tools'">
              <h3 class="section-heading">行动</h3><div class="tool-grid"><button class="tool-card" :disabled="locked" @click="closeOverlay(false); guarded(refresh)"><PlayIcon name="eye" :size="19" /><b>环顾四周</b><small>重新读取当前观察</small></button><button class="tool-card" @click="openSheet('location', '', true)"><PlayIcon name="location" :size="19" /><b>移动</b><small>查看真实可达路线</small></button><button class="tool-card" :disabled="locked" @click="closeOverlay(false); continueStory()"><PlayIcon name="continue" :size="19" /><b>继续剧情</b><small>过一刻钟 · 生活继续发生</small></button><button class="tool-card" :disabled="locked" @click="closeOverlay(false); wait(1)"><PlayIcon name="moon" :size="19" /><b>等一小时</b><small>按世界时间推进</small></button><button class="tool-card" :disabled="locked" @click="closeOverlay(false); wait(4)"><PlayIcon name="moon" :size="19" /><b>等四小时</b><small>按世界时间推进</small></button></div>
              <div class="wait-intent"><label><input v-model="socialSeeking" type="checkbox" :disabled="locked" aria-describedby="wait-intent-hint">等待时，愿意和熟人聊聊</label><p id="wait-intent-hint">仅用于上方快捷等待；不保证有人回应，也不会替你说话。</p></div>
              <h3 class="section-heading">随身与记录</h3><div class="tool-grid"><button class="tool-card" @click="openBusiness('wallet')"><PlayIcon name="bag" :size="19" /><b>钱包</b><small>本人余额</small></button><button class="tool-card" @click="openBusiness('contacts')"><PlayIcon name="people" :size="19" /><b>通讯录</b><small>已知联系人</small></button><button class="tool-card" @click="openBusiness('work')"><PlayIcon name="book" :size="19" /><b>工作信息</b><small>合同与日程</small></button><button class="tool-card" @click="openBusiness('messages')"><PlayIcon name="chat" :size="19" /><b>手机</b><small>事务通知与回执</small></button></div>
              <button class="detail-row clickable" @click="openBusiness('observatory')"><span class="row-symbol"><PlayIcon name="eye" :size="18" /></span><span class="row-copy"><strong>世界观测</strong><small>查看已提交行动与公开回放</small></span><PlayIcon name="right" :size="17" /></button>
              <h3 class="section-heading">调用回执</h3>
              <button class="detail-row clickable" @click="openSheet('thinking', '', true)"><span class="row-symbol"><PlayIcon name="clock" :size="18" /></span><span class="row-copy"><strong>查看本角色的模型调用记录</strong><small>仅显示本角色可读的最近回合；不展示私密提示词或密钥</small></span><PlayIcon name="right" :size="17" /></button>
              <h3 class="section-heading">设置</h3>
              <button class="detail-row clickable" @click="openSheet('models', '', true)"><span class="row-symbol"><PlayIcon name="settings" :size="18" /></span><span class="row-copy"><strong>模型与 API 设置</strong><small>{{ activeProfile ? (activeProfile.name + ' · ' + activeProfile.model) : (observation.decision_mode === 'chat_completions' ? '系统默认 · AI 人物' : '系统默认 · 确定性人物') }}</small></span><PlayIcon name="right" :size="17" /></button>
              <button class="detail-row clickable" :disabled="busy || !!bookmark.pending" @click="openBusiness('style')"><span class="row-symbol"><PlayIcon name="settings" :size="18" /></span><span class="row-copy"><strong>叙事设置</strong><small>保存前后均按真实会话版本核对</small></span></button>
              <p class="detail-note">未提供插件运行接口；这里仅集中现有真实能力。</p>
            </template>
            <template v-else-if="view === 'models'">
              <PlayModelSettings :server-decision-mode="observation.decision_mode" :server-narrative-mode="observation.narrative_mode" :auth-token="token" @select="activeProfile = $event" @update="refreshActiveProfile" />
            </template>
            <template v-else-if="view === 'mode'"><p class="detail-note">输入方式决定提交到哪个既有行动接口。模糊的行动不会擅自执行。</p><div role="radiogroup" aria-label="输入方式"><button v-for="choice in [{ id: 'speech', title: '只说话', desc: '作为你说出的话发送' }, { id: 'AUTO', title: '自然输入', desc: '由服务端解析说话或已知行动' }, { id: 'DIALOGUE', title: '明确说话', desc: '明确表达为对白' }, { id: 'SCENE', title: '场景指令', desc: '明确表达行动意图' }]" :key="choice.id" class="detail-row clickable option-row" role="radio" :aria-checked="inputMode === choice.id" @click="inputMode = choice.id as typeof inputMode; closeOverlay()"><span class="row-symbol"><PlayIcon name="book" :size="18" /></span><span class="row-copy"><strong>{{ choice.title }}</strong><small>{{ choice.desc }}</small></span><PlayIcon v-if="inputMode === choice.id" name="check" :size="17" /></button></div></template>
            <template v-else-if="view === 'settings'"><h3 class="section-heading">外观</h3><div class="setting-group"><button class="detail-row clickable" @click="openSheet('appearance', '', true)"><span class="row-symbol"><PlayIcon name="palette" :size="18" /></span><span class="row-copy"><strong>主题与外观</strong><small>{{ themes.find(t => t.id === appearance.theme)?.name }}</small></span><PlayIcon name="right" :size="17" /></button></div><h3 class="section-heading">阅读</h3><div class="setting-group"><div class="detail-row"><span class="row-copy"><strong>正文字号</strong></span><div class="type-controls"><button aria-label="缩小正文字号" @click="changeFontSize(-1)">A−</button><span>{{ appearance.fontSize }}</span><button aria-label="放大正文字号" @click="changeFontSize(1)">A+</button></div></div><div class="detail-row"><span class="row-copy"><strong>正文字体</strong></span><div class="segmented"><button :class="{ selected: appearance.font === 'serif' }" @click="appearance.font = 'serif'">宋体</button><button :class="{ selected: appearance.font === 'sans' }" @click="appearance.font = 'sans'">系统</button></div></div></div><h3 class="section-heading">当前会话</h3><div class="setting-group"><button class="detail-row clickable" @click="openSheet('models', '', true)"><span class="row-symbol"><PlayIcon name="settings" :size="18" /></span><span class="row-copy"><strong>模型与 API 设置</strong><small>{{ activeProfile ? (activeProfile.name + ' · ' + activeProfile.model) : (observation.decision_mode === 'chat_completions' ? '系统默认 · AI 人物' : '系统默认 · 确定性人物') }}</small></span><PlayIcon name="right" :size="17" /></button><button class="detail-row clickable" :disabled="busy || !!bookmark.pending" @click="openBusiness('style')"><span class="row-symbol"><PlayIcon name="book" :size="18" /></span><span class="row-copy"><strong>叙事设置</strong><small>服务端保存，只影响之后的段落</small></span><PlayIcon name="right" :size="17" /></button></div><p class="detail-note">主题、阅读字号和本机显示名称仅保存在此浏览器。世界事件与叙事设置仍以服务端为准。</p></template>
            <template v-else-if="view === 'appearance'"><div class="theme-preview" aria-label="当前主题预览"><div class="theme-preview-meta"><PlayIcon name="book" :size="13" />阅读预览</div><p class="theme-preview-quote">“这是一段阅读效果预览。”</p><p class="theme-preview-text">背景、正文与输入框会同步换色；故事内容不变。</p><div class="theme-mini-compose"><PlayIcon name="plus" :size="13" /><span>你想做什么……</span><i class="theme-mini-send"><PlayIcon name="send" :size="13" /></i></div></div><div class="theme-choices" role="radiogroup" aria-label="浅色主题" @keydown="themeKey"><button v-for="theme in themes" :key="theme.id" class="theme-choice" role="radio" :aria-label="theme.name + (theme.id === 'claude' ? '，默认主题' : '')" :aria-checked="appearance.theme === theme.id" :tabindex="appearance.theme === theme.id ? 0 : -1" @click="appearance.theme = theme.id"><span class="theme-swatches" :style="{ '--swatch-paper': theme.paper, '--swatch-surface': theme.surface, '--swatch-accent': theme.accent }"><i class="theme-swatch" /><i class="theme-swatch" /></span><span class="theme-choice-copy"><span class="theme-choice-title">{{ theme.name }}<span v-if="theme.id === 'claude'" class="theme-default-tag">默认</span></span><small>{{ theme.description }}</small></span><span class="theme-choice-check"><PlayIcon name="check" :size="12" /></span></button></div><p class="theme-footnote">点选即可预览。透明状态线保持不变。</p></template>
            <template v-else-if="view === 'reader'"><article v-if="selectedTurn" class="reader-content"><div class="reader-byline">世界记录 · {{ observation.place_name }}</div><div class="prose"><p v-for="(line, i) in (variants[selectedTurn.turn_run_id] || selectedTurn.narrative_lines)" :key="i">{{ readableLine(line) }}</p></div></article><p v-else class="detail-note">这段记录已不在最近 50 段观察中，请重新查看故事记录。</p></template>
            <template v-else-if="view === 'thinking'">
              <ol class="summary-timeline">
                <li class="summary-step is-current"><span class="summary-mark"><PlayIcon name="clock" :size="17" /></span><div class="summary-copy"><b>{{ status || '当前没有进行中的请求' }}</b><p>{{ bookmark.pending?.narrative_turn_id ? '已保存已提交回合的标识；后续读取不会再次提交行动。' : bookmark.pending ? '请求键已保存在本机，尚不能断言服务端是否已接受。' : '只展示世界接口的公开结果，不展示模型内部思维。' }}</p></div></li>
                <li v-for="(turn, index) in observation.recent_turns.filter(item => item.provider_calls?.length).slice(-10)" :key="turn.turn_run_id" class="summary-step"><span class="summary-mark"><PlayIcon name="book" :size="17" /></span><div class="summary-copy"><b>最近回合 {{ index + 1 }} · 服务端调用回执</b><p v-for="(call, i) in turn.provider_calls" :key="i">{{ providerCallSummary(call) }}</p></div></li>
              </ol>
              <p class="summary-note">“已配置”不等于“已调用”或“调用成功”。回执来自当前获授权角色的服务端记录；正在进行中的调用可能尚无最终结果。这里不展示提示词、密钥或模型内部思维。</p>
            </template>
            <template v-else-if="view === 'rename'"><form @submit.prevent="rename"><label class="form-label" for="rename-title">此浏览器的显示名称</label><input id="rename-title" v-model="renameTitle" class="text-field" maxlength="48" required><p class="detail-note">仅改变本机标题；不会更改世界、人物或服务端记录。</p><div class="form-actions"><button type="button" class="flat-button" @click="closeOverlay()">取消</button><button class="solid-button">保存</button></div></form></template>
            <template v-else-if="view === 'chapter'"><p class="detail-note">从现在开始一段新的对话。当前故事记录会清空，旧对话也不会再进入后续小说叙述的上下文。</p><p class="detail-note">这不会删除世界事件、人物关系、已发生的行动或角色记忆；它只是为当前会话划出一个新的叙事起点。</p><p class="detail-note">有未完成的行动或恢复请求时不能开始新篇。</p><div class="form-actions"><button class="flat-button" @click="closeOverlay()">继续当前篇</button><button class="solid-button" :disabled="locked || !!bookmark.opening" @click="startNewChapter">确认开始新篇</button></div></template>
            <template v-else-if="view === 'clear'"><p class="detail-note">只清除此浏览器当前世界的会话书签；不会删除服务端世界或故事记录。清除后重新进入需要有效玩家凭证。</p><p class="detail-note">有未确认的行动或叙事设置时不允许清除，以免丢失恢复请求。</p><div class="form-actions"><button class="flat-button" @click="closeOverlay()">保留会话</button><button class="danger-button" :disabled="locked || !!bookmark.opening" @click="clearLocalSession">确认清除此设备的会话</button></div></template>
          </div>
          <footer v-if="view === 'appearance' || view === 'reader'" class="sheet-footer"><span v-if="view === 'appearance'" class="theme-save-status" role="status">已应用 · 此浏览器的阅读偏好</span><div v-else class="type-controls"><button aria-label="缩小正文字号" @click="changeFontSize(-1)">A−</button><span>{{ appearance.fontSize }}</span><button aria-label="放大正文字号" @click="changeFontSize(1)">A+</button></div><button class="solid-button" @click="closeOverlay()">{{ view === 'appearance' ? '完成' : '返回对话' }}</button></footer>
        </section>
      </Transition>
      <PlayWallet v-if="walletOpen" :owner="observation.controlled_entity.display_name" :read="readWallet" @close="closeBusiness('wallet')" />
      <PlayContacts v-if="contactsOpen" :read="readContacts" @close="closeBusiness('contacts')" />
      <PlayWork v-if="workOpen" :read="readWork" @close="closeBusiness('work')" />
      <PlayMap v-if="mapOpen" :read="readMap" @close="closeBusiness('map')" @move="moveFromMap" />
      <PlayMessages v-if="messagesOpen" :read="readMessages" @close="closeBusiness('messages')" />
      <PlayObservatory v-if="observatoryOpen" :read="readObservatory" @close="closeBusiness('observatory')" />
      <PlayStyle v-if="styleOpen && sessionScope" :scope="{ session_id: sessionScope.session_id, instance_id: sessionScope.instance_id, branch_id: sessionScope.branch_id }" :narrative-mode="observation.narrative_mode" :api="api" @close="closeBusiness('style')" @pending="stylePending = $event" />
    </template>
  </div>
</template>
