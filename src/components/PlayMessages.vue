<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
type Message = { message_id: string; sequence: number; world_time: string; kind: string; title: string; body: string }
type MessagesPage = { messages: Message[]; next_before_sequence?: number; world_time: string }
const props = defineProps<{ read: (before: number) => Promise<MessagesPage> }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const messages = ref<Message[]>([]), before = ref(0), at = ref('')
const loading = ref(false), error = ref('')
let active = true
async function load(more = false) {
  if (loading.value) return
  loading.value = true; error.value = ''
  try {
    const result = await props.read(more ? before.value : 0)
    if (!active) return
    const all = more ? [...messages.value, ...result.messages] : result.messages
    messages.value = [...new Map(all.map(message => [message.message_id, message])).values()]
    before.value = result.next_before_sequence || 0; at.value = result.world_time
  } catch (cause) { if (active) error.value = cause instanceof Error ? cause.message : '暂时无法读取消息。' }
  finally { if (active) loading.value = false }
}
function containTab(event: KeyboardEvent) {
  if (event.key !== 'Tab') return
  const items = dialog.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')
  if (!items?.length) return
  const first = items[0], last = items[items.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}
const time = (value: string) => value.replace('T', ' ').replace(/Z$/, '')
onMounted(() => { dialog.value?.showModal(); void load() })
onBeforeUnmount(() => { active = false; dialog.value?.close() })
</script>

<template>
  <dialog ref="dialog" class="messages-sheet" aria-labelledby="messages-title" @close="emit('close')" @keydown="containTab">
    <header><div><p class="eyebrow">随身 · 通知与回执</p><h2 id="messages-title">手机消息</h2></div><button autofocus aria-label="关闭消息" @click="dialog?.close()">收起 ×</button></header>
    <p class="intro">与你有关的事务，按发生时间留在这里。</p>
    <p class="hint">这里整理已发生的工作通知与本人回执，不是私人聊天或短信投递记录。旧邀约不代表仍然有效；此处不能发送或答复消息。</p>
    <p v-if="error" class="error" role="alert">{{ error }}{{ at ? '。以下仍是上次读取的记录。' : '' }}</p>
    <p v-if="loading" role="status">正在查看消息…</p>
    <p v-if="!loading && !error && !messages.length" class="empty">目前没有与你有关的事务通知。</p>
    <ol v-if="messages.length" :aria-busy="loading" aria-label="事务通知记录">
      <li v-for="message in messages" :key="message.message_id"><time>{{ time(message.world_time) }} · 世界时间</time><h3>{{ message.title }}</h3><p class="body">{{ message.body }}</p></li>
    </ol>
    <button v-if="before" :disabled="loading" @click="load(true)">查看更早消息</button>
    <footer><p>{{ at ? `已列出 ${messages.length} 条 · 查询于 ${time(at)}` : '只读取本人的事务。' }}<br>重新查看可获取新通知。</p><button :disabled="loading" @click="load()">重新查看消息</button></footer>
  </dialog>
</template>

<style scoped>
.messages-sheet { margin: auto; width: min(620px,calc(100% - 32px)); max-height: calc(100dvh - 40px); overflow: auto; padding: 28px; border: 1px solid #7e493550; border-top: 4px solid var(--accent); background: var(--paper); color: var(--text); }.messages-sheet::backdrop { background: #18251dc0; }header { display: flex; justify-content: space-between; align-items: start; gap: 16px; }.eyebrow { font-size: 11px; letter-spacing: 2px; color: var(--accent); margin: 0 0 10px; }h2 { color: var(--text); font: 400 30px/1.4 var(--font-serif); margin: 0; }.intro,.empty { font: 15px/1.9 var(--font-serif); }.intro { margin: 20px 0 12px; }.hint { font-size: 12px; line-height: 1.8; color: var(--muted); }
ol { list-style: none; padding: 0; margin: 24px 0; border-top: 1px solid #39493535; }li { padding: 22px 0; border-bottom: 1px solid #39493525; overflow-wrap: anywhere; }h3 { font: 400 20px/1.6 var(--font-serif); color: var(--text); margin: 8px 0; }.body { font: 14px/1.9 var(--font-body); white-space: pre-wrap; margin: 0; }time,footer { font-size: 12px; color: var(--muted); }footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 10px; line-height: 1.8; }.error { color: #99372f; font-size: 13px; overflow-wrap: anywhere; }button { border: 0; background: transparent; color: inherit; font: inherit; min-height: 44px; padding: 10px 12px; cursor: pointer; }button:hover:not(:disabled) { background: #35433112; }button:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }button:disabled { opacity: .5; }
@media(max-width:600px) { .messages-sheet { width: 100%; max-width: 100%; margin: auto 0 0; border-inline: 0; border-bottom: 0; padding: 24px 24px max(24px,env(safe-area-inset-bottom)); } }
</style>
