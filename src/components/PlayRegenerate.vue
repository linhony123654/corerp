<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import type { StyleRead } from '../lib/narrativeStyle'
import type { NarrativeStreamResult } from '../lib/narrativeStream'

const props = defineProps<{ session: string; turn: string; disabled: boolean; api: <T>(path: string, body: Record<string, unknown>) => Promise<T>; stream: (body: Record<string, unknown>, preview: (lines: string[]) => void, signal: AbortSignal) => Promise<NarrativeStreamResult> }>()
const emit = defineEmits<{ variant: [lines: string[] | null] }>()
const busy = ref(false), changed = ref(false)
const error = ref(''), warnings = ref<string[]>([])
const preview = ref<string[]>([])
let controller: AbortController | null = null
let request: Record<string, unknown> | null = null
let active = true
async function render(retry = false) {
  if (busy.value || props.disabled) return
  busy.value = true; error.value = ''
  preview.value = []; controller = new AbortController()
  if (!retry) request = null
  try {
    if (!request) {
      const { profile: p } = await props.api<StyleRead>('style/read', { session_id: props.session })
      request = { session_id: props.session, turn_run_id: props.turn, style_override: {
        context_budget_bytes: p.context_budget_bytes || 0,
        pov: p.pov, tense: p.tense, verbosity: p.verbosity, dialogue_ratio: p.dialogue_ratio, description_density: p.description_density,
        inner_monologue_policy: p.inner_monologue_policy, prose_instructions: p.prose_instructions, forbidden_patterns: p.forbidden_patterns, narrative_pack_ref: p.narrative_pack_ref
      } }
    }
    const result = await props.stream(request, lines => { if (active) preview.value = lines }, controller.signal)
    if (active) { emit('variant', result.view.lines); changed.value = true; warnings.value = result.view.warnings || [] }
  } catch (cause) { if (active) error.value = cause instanceof Error ? cause.message : '叙述暂时无法生成。' }
  finally { if (active) { busy.value = false; preview.value = [] } }
}
function restore() { emit('variant', null); changed.value = false; warnings.value = []; error.value = ''; request = null }
onBeforeUnmount(() => { active = false; controller?.abort() })
</script>

<template>
  <details class="narrative-tools">
    <summary>叙述方式<span v-if="changed"> · 当前为重新生成的展示</span></summary>
    <p>按当前叙事设置重新表达同一段事实，不重新决定人物行动。仅在此页展示，刷新后回到原叙述；本地叙述器可能产生相同文本。</p>
    <button type="button" :disabled="busy || disabled" @click="render()">{{ busy ? '正在生成叙述…' : '按当前设置重新生成' }}</button>
    <button v-if="changed" type="button" :disabled="busy" @click="restore">恢复原叙述</button>
    <p v-if="busy" role="status">正在读取同一段已发生的事实…</p>
    <section v-if="busy && preview.length" class="stream-preview" aria-label="叙述流临时预览"><p>正在接收 · 未完成的预览</p><p v-for="(line, i) in preview" :key="i">{{ line }}</p></section>
    <div v-if="error"><p class="render-error" role="alert">{{ error }}。当前文本仍保留。</p><button type="button" :disabled="busy || disabled" @click="render(true)">重试叙述读取</button></div>
    <p v-if="changed" role="status">展示已更新，世界事件和原始记录未改变。</p>
    <p v-if="warnings.length" class="capability-note">叙述器提示：{{ warnings.join('；') }}</p>
  </details>
</template>

<style scoped>
.narrative-tools { font: 12px/1.8 var(--font-body); margin-top: 14px; color: var(--muted); }.narrative-tools summary { width: fit-content; cursor: pointer; min-height: 44px; padding-block: 10px; }.narrative-tools p { font: inherit; margin: 10px 0; overflow-wrap: anywhere; }.narrative-tools button { min-height: 44px; padding: 8px 12px; font: inherit; background: transparent; color: var(--accent); border: 0; cursor: pointer; }.narrative-tools button:hover:not(:disabled) { background: #35433112; }.narrative-tools :is(button,summary):focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }.narrative-tools button:disabled { opacity: .5; cursor: default; }.narrative-tools .render-error { color: #99372f; }.capability-note { border-left: 2px solid var(--accent); padding-left: 10px; }
</style>
