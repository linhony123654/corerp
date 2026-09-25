<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { pendingStyleKey, stylePresets } from '../lib/narrativeStyle'
import type { StyleProfile, StyleRead, StyleScope, StyleWrite } from '../lib/narrativeStyle'

const props = defineProps<{ scope: StyleScope; narrativeMode?: string; api: <T>(path: string, body: Record<string, unknown>) => Promise<T> }>()
const emit = defineEmits<{ close: []; pending: [value: boolean] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const profile = ref<StyleProfile | null>(null)
const revision = ref(0)
const pending = ref<StyleWrite | null>(null)
const busy = ref(false), conflict = ref(false)
const error = ref(''), notice = ref('')
let active = true
const key = pendingStyleKey(props.scope.session_id)

function containTab(event: KeyboardEvent) {
  if (event.key !== 'Tab') return
  const controls = [...(dialog.value?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled)') || [])].filter(el => !el.closest('fieldset:disabled'))
  const first = controls[0], last = controls[controls.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
}
async function load() {
  const result = await props.api<StyleRead>('style/read', { session_id: props.scope.session_id })
  if (!Number.isSafeInteger(result.session_revision) || result.session_revision < 0) throw new Error('缺少有效的设置版本，请检查服务。')
  if (active) {
    const p = result.profile
    // A resolved profile also has a version field; it is not part of a patch.
    profile.value = { context_budget_bytes: p.context_budget_bytes || 0, narrative_density: p.narrative_density || '', pov: p.pov, tense: p.tense, verbosity: p.verbosity, dialogue_ratio: p.dialogue_ratio, description_density: p.description_density, inner_monologue_policy: p.inner_monologue_policy, prose_instructions: p.prose_instructions, forbidden_patterns: p.forbidden_patterns, narrative_pack_ref: p.narrative_pack_ref }
    revision.value = result.session_revision
  }
}
async function readLatest() {
  if (busy.value) return
  busy.value = true; error.value = ''; profile.value = null
  try { await load() } catch (cause) { if (active) error.value = message(cause) }
  finally { if (active) busy.value = false }
}
function message(cause: unknown) { return cause instanceof Error ? cause.message : '暂时无法连接，请重试。' }
function preset(name: keyof typeof stylePresets) {
  if (profile.value) Object.assign(profile.value, stylePresets[name], { prose_instructions: '' })
  notice.value = ''
}
async function save() {
  if (busy.value || conflict.value || !profile.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    if (!pending.value) {
      const request: StyleWrite = { ...props.scope, scope: 'session', expected_revision: revision.value, idempotency_key: crypto.randomUUID(), patch: JSON.parse(JSON.stringify(profile.value)) }
      // Persist before dispatch. If storage fails, no request is sent.
      localStorage.setItem(key, JSON.stringify(request))
      pending.value = request; emit('pending', true)
    }
    await props.api('style/set', pending.value)
    await load() // Keep exact intent until authoritative read succeeds too.
    localStorage.removeItem(key)
    if (active) { pending.value = null; emit('pending', false); notice.value = '已保存。用于之后的新段落，不改写已发生的事。' }
  } catch (cause) {
    if (active) {
      conflict.value = (cause as { code?: string })?.code === 'BRANCH_VERSION_CONFLICT'
      error.value = conflict.value ? '设置已在别处更新。这次保存未覆盖它，请重新读取后再修改。' : message(cause)
    }
  } finally { if (active) busy.value = false }
}
async function discardConflict() {
  // Only a definite server conflict permits discarding this uncommitted intent.
  try { localStorage.removeItem(key) } catch (cause) { error.value = message(cause); return }
  pending.value = null; emit('pending', false); conflict.value = false
  await readLatest()
}
onMounted(async () => {
  dialog.value?.showModal()
  try {
    const raw = localStorage.getItem(key)
    if (raw) {
      const request = JSON.parse(raw) as StyleWrite
      if (request.session_id !== props.scope.session_id || request.instance_id !== props.scope.instance_id || request.branch_id !== props.scope.branch_id || request.scope !== 'session' || !request.idempotency_key || !request.patch) throw new Error('待恢复设置与当前会话不符，请保留记录并检查本地数据。')
      pending.value = request; profile.value = request.patch; revision.value = request.expected_revision
      emit('pending', true); notice.value = '有一份保存结果尚未确认，继续保存会复用原请求。'
    } else await readLatest()
  } catch (cause) { error.value = message(cause) }
})
onBeforeUnmount(() => { active = false; dialog.value?.close() })
</script>

<template>
  <dialog ref="dialog" class="style-sheet" aria-labelledby="style-title" @keydown="containTab" @cancel="busy && !!pending && $event.preventDefault()" @close="emit('close')">
    <header><div><p class="eyebrow">阅读 · 叙述方式</p><h2 id="style-title">叙事设置</h2></div><button autofocus :disabled="busy && !!pending" aria-label="关闭叙事设置" @click="dialog?.close()">收起 ×</button></header>
    <p class="intro">只调整这段生活的表达，不替人物作决定。</p>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <p v-if="busy" role="status">{{ pending ? '正在确认保存…' : '正在读取设置…' }}</p>
    <button v-if="conflict" @click="discardConflict">放弃本次修改，读取最新设置</button>
    <button v-else-if="!profile && !busy" @click="readLatest">重新读取设置</button>
    <form v-if="profile" @submit.prevent="save">
      <fieldset :disabled="busy || !!pending"><legend class="sr-only">叙事偏好</legend>
        <div class="presets" aria-label="风格预设"><button type="button" @click="preset('plain')">日常</button><button type="button" @click="preset('dialogue')">对话</button><button type="button" @click="preset('detailed')">细叙</button></div>
        <div class="fields"><label>叙述视角<select v-model="profile.pov"><option value="first_person">第一人称 · 我</option><option value="second_person">第二人称 · 你</option><option value="third_person">第三人称 · 人物名字</option></select></label>
          <label>回应详略<select v-model="profile.verbosity"><option value="terse">简短</option><option value="normal">适中</option><option value="detailed">详细</option></select></label>
          <label>叙述篇幅<select v-model="profile.narrative_density" aria-describedby="density-hint"><option value="">沿用旧设置</option><option value="concise">简洁</option><option value="standard">标准</option><option value="long">长篇（仅有事实时展开）</option></select></label>
          <label>叙述时态<select v-model="profile.tense"><option value="present">现在</option><option value="past">过去</option></select></label>
        </div>
        <p id="density-hint" class="hint">只影响之后新段落的呈现，不推进时间。长篇模式不会为凑字数编造事件或心理。</p>
        <label>对话比例偏好 · {{ profile.dialogue_ratio }}%<input v-model.number="profile.dialogue_ratio" type="range" min="0" max="100"></label>
        <label>场景描写密度 · {{ profile.description_density }}%<input v-model.number="profile.description_density" type="range" min="0" max="100"></label>
        <label>叙述上下文预算<select v-model.number="profile.context_budget_bytes" aria-describedby="context-budget-hint"><option :value="0">默认 · 64 KiB</option><option :value="4096">4 KiB</option><option :value="16384">16 KiB</option><option :value="65536">64 KiB</option><option :value="262144">256 KiB</option></select></label>
        <p id="context-budget-hint" class="hint">限制整段叙述输入的 UTF-8 JSON 字节数（1 KiB = 1024 字节），不是模型 token。超限不会删掉对白或阻止行动提交，可改读已保存原文；不改变人物决策上下文。</p>
        <label>自定义文风<textarea v-model="profile.prose_instructions" maxlength="2000" rows="3" aria-describedby="style-capability" placeholder="记录你希望的叙述方式"></textarea></label>
      </fieldset>
      <p v-if="narrativeMode === 'style_planner'" id="style-capability" class="hint">当前使用模型文风解释器：自定义指令会在上述设置基础上调整视角、时态、详略、对话排版和已知场景呈现；未指定的设置保留。不支持任意文学扩写、改写台词或补写心理。超出能力会提示；模型只接收文风偏好，不接收人物事实或对白。自定义呈现仅在本页展示，刷新后回到保存的原叙述。这与人物决策是否使用 AI 无关。</p>
      <p v-else-if="narrativeMode === 'custom'" id="style-capability" class="hint">当前使用独立叙述执行器，但它未声明自定义文风能力；保存偏好不代表所有要求都会执行。请以实际输出与执行器提示为准。</p>
      <p v-else id="style-capability" class="hint">当前使用本地事实叙述器：支持视角、详略及有限的场景表达；比例是偏好，不会删掉真实对白。自由文风指令可保存，但暂不会被执行。这与人物是否使用 AI 无关。</p>
      <p class="hint">保存将把表单中的偏好设为当前会话设置；地点专属风格如存在，仍优先适用。未确认的保存请求会留在此浏览器中以便恢复，不含访问凭证。</p>
      <footer><button class="save-style" :disabled="busy || conflict">{{ pending ? '继续保存原设置' : '保存叙事设置' }}</button><span>已发生的事不会回滚</span></footer>
    </form>
  </dialog>
</template>

<style scoped>
.style-sheet { width: min(640px, calc(100% - 32px)); max-height: calc(100dvh - 40px); overflow: auto; margin: auto; padding: 28px; border: 1px solid #7e493550; border-top: 4px solid var(--accent); background: var(--paper); color: var(--text); }.style-sheet::backdrop { background: #18251dc0; }
header { display: flex; justify-content: space-between; align-items: start; gap: 16px; }h2 { color: var(--text); font: 400 30px/1.4 var(--font-serif); margin: 0; }.eyebrow { color: var(--accent); font-size: 11px; letter-spacing: 2px; margin: 0 0 10px; }.intro { font: 15px/1.8 var(--font-serif); }
button, select, textarea { color: inherit; font: inherit; }button { background: transparent; border: 0; min-height: 44px; padding: 10px 12px; cursor: pointer; }button:hover:not(:disabled) { background: #35433112; }:is(button,input,select,textarea):focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }button:disabled, fieldset:disabled { opacity: .55; }
fieldset { min-width: 0; border: 0; padding: 0; margin: 20px 0; }.presets { display: flex; gap: 12px; border-block: 1px solid #39493535; margin-bottom: 20px; }.fields { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }label { display: block; font-size: 13px; line-height: 1.8; margin: 0 0 16px; }select,textarea { display: block; width: 100%; min-height: 44px; border: 1px solid #858d7f; background: #fff7; padding: 10px; }textarea { resize: vertical; }input[type=range] { display: block; width: 100%; height: 44px; accent-color: var(--accent); }
.hint { font-size: 12px; line-height: 1.8; color: var(--muted); }.error { color: #99372f; font-size: 13px; overflow-wrap: anywhere; }.notice { color: #425c39; font-size: 13px; }footer { display: flex; align-items: center; flex-wrap: wrap; gap: 16px; border-top: 1px solid #39493535; padding-top: 16px; }footer span { color: var(--muted); font-size: 11px; }.save-style { background: #344638; color: #fffaf0; }.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
@media (max-width:600px) { .style-sheet { margin: auto 0 0; width: 100%; max-width: 100%; padding: 24px 24px max(24px,env(safe-area-inset-bottom)); border-inline: 0; border-bottom: 0; }.fields { grid-template-columns: 1fr; gap: 0; } }
</style>
