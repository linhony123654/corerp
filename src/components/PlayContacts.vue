<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
type Contact = { entity_id: string; display_name: string; last_known_world_time: string }
type ContactsPage = { contacts: Contact[]; next_after_entity_id?: string; world_time: string }
const props = defineProps<{ read: (after: string) => Promise<ContactsPage> }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const contacts = ref<Contact[]>([]), after = ref(''), at = ref('')
const loading = ref(false), error = ref('')
let active = true
async function load(more = false) {
  if (loading.value) return
  loading.value = true; error.value = ''
  try {
    const result = await props.read(more ? after.value : '')
    if (!active) return
    const all = more ? [...contacts.value, ...result.contacts] : result.contacts
    contacts.value = [...new Map(all.map(contact => [contact.entity_id, contact])).values()]
    after.value = result.next_after_entity_id || ''; at.value = result.world_time
  } catch (cause) { if (active) error.value = cause instanceof Error ? cause.message : '暂时无法读取通讯录。' }
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
  <dialog ref="dialog" class="contacts-sheet" aria-labelledby="contacts-title" @close="emit('close')" @keydown="containTab">
    <header><div><p class="eyebrow">随身 · 认识的人</p><h2 id="contacts-title">通讯录</h2></div><button autofocus aria-label="关闭通讯录" @click="dialog?.close()">收起 ×</button></header>
    <p class="intro">见过或听过的人。这里没有电话号码，也不显示他们此刻在哪。</p>
    <p v-if="error" class="error" role="alert">{{ error }}{{ at ? '。以下仍是上次读取的记录。' : '' }}</p>
    <p v-if="loading" role="status">正在翻阅记录…</p>
    <p v-if="!loading && !error && !contacts.length" class="empty">还没有记下认识的人。亲历的相遇会留在这里。</p>
    <ul v-if="contacts.length" :aria-busy="loading" aria-label="已知联系人">
      <li v-for="contact in contacts" :key="contact.entity_id"><strong>{{ contact.display_name }}</strong><span>最近获知 <time>{{ time(contact.last_known_world_time) }}</time></span></li>
    </ul>
    <button v-if="after" :disabled="loading" @click="load(true)">继续翻阅</button>
    <footer><p>{{ at ? `已列出 ${contacts.length} 人 · 查询于 ${time(at)}` : '只读取自己的认识记录。' }}</p><button :disabled="loading" @click="load()">重新翻阅</button></footer>
  </dialog>
</template>

<style scoped>
.contacts-sheet { margin: auto; width: min(560px,calc(100% - 32px)); max-height: calc(100dvh - 40px); overflow: auto; padding: 28px; border: 1px solid #7e493550; border-top: 4px solid var(--accent); background: var(--paper); color: var(--text); }.contacts-sheet::backdrop { background: #18251dc0; }header { display: flex; justify-content: space-between; align-items: start; gap: 16px; }.eyebrow { font-size: 11px; letter-spacing: 2px; color: var(--accent); margin: 0 0 10px; }h2 { color: var(--text); font: 400 30px/1.4 var(--font-serif); margin: 0; }.intro,.empty { font: 15px/1.9 var(--font-serif); }.intro { margin: 20px 0; }
ul { list-style: none; padding: 0; margin: 24px 0; border-top: 1px solid #39493535; }li { padding: 16px 0; border-bottom: 1px solid #39493525; display: flex; align-items: baseline; justify-content: space-between; flex-wrap: wrap; gap: 6px 16px; overflow-wrap: anywhere; }strong { font: 400 21px var(--font-serif); }li span,footer { font-size: 12px; color: var(--muted); }footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 10px; line-height: 1.8; }.error { color: #99372f; font-size: 13px; overflow-wrap: anywhere; }button { border: 0; background: transparent; color: inherit; font: inherit; min-height: 44px; padding: 10px 12px; cursor: pointer; }button:hover:not(:disabled) { background: #35433112; }button:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }button:disabled { opacity: .5; }
@media(max-width:600px) { .contacts-sheet { width: 100%; max-width: 100%; margin: auto 0 0; border-inline: 0; border-bottom: 0; padding: 24px 24px max(24px,env(safe-area-inset-bottom)); }li { display: block; }li span { display: block; margin-top: 8px; } }
</style>
