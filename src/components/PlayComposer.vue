<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import PlayIcon from './PlayIcon.vue'

const props = defineProps<{ locked: boolean; busy: boolean; streaming: boolean; decisionMode: string; inputMode: string; activeModelName?: string }>()
const emit = defineEmits<{ send: []; refresh: []; abort: []; open: [view: 'tools' | 'models' | 'mode'] }>()
const draft = defineModel<string>('draft', { required: true })
const input = ref<HTMLTextAreaElement | null>(null)
const filled = computed(() => !!draft.value.trim())
const modeLabel = computed(() => ({ speech: '只说话', AUTO: '自然输入', DIALOGUE: '明确说话', SCENE: '场景指令' })[props.inputMode] || '自然输入')
const modelLabel = computed(() => {
  if (props.activeModelName && props.activeModelName.trim()) return props.activeModelName.trim()
  return props.decisionMode === 'chat_completions' ? 'AI 人物' : '确定性人物'
})

function fit() {
  if (!input.value) return
  input.value.style.height = 'auto'
  const viewport = window.visualViewport
  const short = window.innerHeight < 520 || !!viewport && Math.abs(viewport.scale - 1) < .05 && viewport.height < window.innerHeight * .8
  input.value.style.height = `${Math.min(Math.max(input.value.scrollHeight, 37), short ? 76 : 132)}px`
}
watch(draft, () => void nextTick(fit), { flush: 'post' })
onMounted(() => { window.visualViewport?.addEventListener('resize', fit); window.addEventListener('resize', fit) })
onBeforeUnmount(() => { window.visualViewport?.removeEventListener('resize', fit); window.removeEventListener('resize', fit) })
function submit() {
  if (props.streaming) emit('abort')
  else if (!props.locked && filled.value) emit('send')
  else if (!props.locked) emit('refresh')
}
function keydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.isComposing) {
    event.preventDefault()
    submit()
  }
}
function focus() { input.value?.focus({ preventScroll: true }) }
function keepFocus(event: PointerEvent) { if (document.activeElement === input.value) event.preventDefault() }
defineExpose({ focus })
</script>

<template>
  <form class="composer" @submit.prevent="submit">
    <label for="words" class="sr-only">{{ inputMode === 'speech' || inputMode === 'DIALOGUE' ? '你想说的话' : '你想说或做的事' }}</label>
    <textarea id="words" ref="input" v-model="draft" maxlength="2000" rows="1" :disabled="locked" :placeholder="inputMode === 'speech' || inputMode === 'DIALOGUE' ? '你想说什么……' : '你想做什么……'" @keydown="keydown" @focus="fit" />
    <div class="composer-tools">
      <button type="button" class="circle-button" title="行动与功能" aria-label="行动与功能" aria-haspopup="dialog" @click="emit('open', 'tools')"><PlayIcon name="plus" /></button>
      <button type="button" class="model-pill" :title="`模型：${modelLabel} · 点击打开模型与 API 设置`" :aria-label="`模型：${modelLabel} · 打开模型与 API 设置`" aria-haspopup="dialog" @click="emit('open', 'models')"><span>{{ modelLabel }}</span><PlayIcon name="down" :size="12" /></button>
      <div class="tool-spacer" />
      <button type="button" class="circle-button mode-button" :class="{ 'is-custom': inputMode !== 'speech' }" :title="`输入方式：${modeLabel}`" :aria-label="`输入方式：${modeLabel}`" aria-haspopup="dialog" @click="emit('open', 'mode')"><PlayIcon name="book" /></button>
      <button type="submit" class="primary-button" :class="{ 'has-text': filled, 'is-thinking': streaming }" :disabled="busy && !streaming || locked && !streaming" :aria-label="streaming ? '停止读取叙述' : filled ? '说出' : '环顾四周'" :title="streaming ? '停止读取；已提交的行动不会撤销' : filled ? '发送（Ctrl / ⌘ + Enter）' : '环顾四周（不推进时间）'" @pointerdown="keepFocus">
        <PlayIcon v-if="streaming" name="stop" />
        <PlayIcon v-else-if="filled" name="send" />
        <PlayIcon v-else name="continue" />
      </button>
    </div>
  </form>
</template>
