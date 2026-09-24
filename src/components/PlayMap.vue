<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import type { LocalMap, MapMove } from '../lib/localMap'
const props = defineProps<{ read: () => Promise<LocalMap> }>()
const emit = defineEmits<{ close: []; move: [request: MapMove] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const map = ref<LocalMap | null>(null), loading = ref(false), error = ref('')
let active = true
async function load() {
  if (loading.value) return
  loading.value = true; error.value = ''
  try { const result = await props.read(); if (active) map.value = result }
  catch (cause) { if (active) error.value = cause instanceof Error ? cause.message : '暂时无法查看地图。' }
  finally { if (active) loading.value = false }
}
function move(to: string) {
  if (!map.value || loading.value || error.value || !map.value.reachable_places.some(p => p.place_id === to && p.can_move_now)) return
  emit('move', { from_place_id: map.value.place_id, to_place_id: to, expected_cursor: map.value.observation_cursor })
}
function containTab(event: KeyboardEvent) {
  if (event.key !== 'Tab') return
  const buttons = dialog.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')
  if (!buttons?.length) return
  const first = buttons[0], last = buttons[buttons.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}
const time = (value: string) => value.replace('T', ' ').replace(/Z$/, '')
const works = (to: string) => map.value?.transit_works?.filter(w => w.from_place_id === map.value?.place_id && w.to_place_id === to) ?? []
onMounted(() => { dialog.value?.showModal(); void load() })
onBeforeUnmount(() => { active = false; dialog.value?.close() })
</script>

<template>
  <dialog ref="dialog" class="map-sheet" aria-labelledby="map-title" @keydown="containTab" @close="emit('close')">
    <header><div><p class="eyebrow">随身 · 此处与彼处</p><h2 id="map-title">附近地图</h2></div><button autofocus aria-label="关闭地图" @click="dialog?.close()">收起 ×</button></header>
    <p class="intro">从此处出发，可以走向哪里。</p>
    <p class="hint">连接示意，不代表距离或方位；只显示当前位置的相邻地点，不显示远处人物。</p>
    <p v-if="error" role="alert" class="error">{{ error }}{{ map ? '。以下仍是上次读取的地图，重新查看后才能出发。' : '' }}</p>
    <p v-if="loading" role="status">正在查看附近路线…</p>
    <section v-if="map" class="route-tree" aria-label="当前位置与相邻路线">
      <div class="origin"><span class="here" aria-hidden="true">此处</span><div><p class="hint">你在这里</p><h3>{{ map.place_name }}</h3></div></div>
      <p v-if="!map.reachable_places.length" class="empty">没有查到相邻路线。</p>
      <ul><li v-for="place in map.reachable_places" :key="place.place_id" :class="{ obstructed: !place.can_move_now }">
        <div class="destination"><h4>{{ place.display_name }}</h4><button :disabled="loading || !!error || !place.can_move_now" :aria-label="`前往 ${place.display_name}`" @click="move(place.place_id)">前往 →</button></div>
        <p class="route-state">{{ place.can_move_now ? '当前可前往' : '当前无法通行' }}</p>
        <p v-for="(work, i) in works(place.place_id)" :key="i" class="hint">相邻路段施工至 {{ time(work.ends_at) }}{{ place.can_move_now ? '；当前仍有可用通路。' : '。' }}</p>
      </li></ul>
    </section>
    <footer><p class="hint">{{ map ? `查询于 ${time(map.world_time)} · 世界时间` : '读取路线不会移动人物或推进时间。' }}<br>出发时仍会核对最新位置与通行条件。</p><button :disabled="loading" @click="load">重新查看地图</button></footer>
  </dialog>
</template>

<style scoped>
.map-sheet { margin: auto; width: min(660px,calc(100% - 32px)); max-height: calc(100dvh - 40px); overflow: auto; padding: 28px; border: 1px solid #7e493550; border-top: 4px solid var(--accent); background: var(--paper); color: var(--text); }.map-sheet::backdrop { background: #18251dc0; }header,.destination,footer { display: flex; align-items: start; justify-content: space-between; gap: 16px; }h2 { font: 400 30px/1.4 var(--font-serif); color: var(--text); margin: 0; }.eyebrow { font-size: 11px; letter-spacing: 2px; color: var(--accent); margin: 0 0 10px; }.intro { font: 15px/1.8 var(--font-serif); margin-top: 20px; }.hint { font-size: 12px; line-height: 1.8; color: var(--muted); }.origin { display: flex; gap: 18px; align-items: center; margin: 26px 0 0; }.origin p { margin: 0; }.here { border: 1px solid var(--accent); color: var(--accent); padding: 10px 8px; writing-mode: vertical-rl; font: 15px var(--font-serif); letter-spacing: 3px; }h3 { font: 400 22px/1.6 var(--font-serif); color: var(--text); margin: 0; }ul { list-style: none; padding-left: 28px; margin: 0 0 0 17px; border-left: 1px solid #7e493580; }li { position: relative; padding: 22px 0 14px; border-bottom: 1px solid #39493525; }li::before { content: ''; position: absolute; width: 22px; left: -28px; top: 44px; border-top: 1px solid #7e493580; }.obstructed::before { border-top-style: dashed; }h4 { font: 400 17px/1.6 var(--font-serif); color: var(--text); margin: 8px 0 0; overflow-wrap: anywhere; }.route-state { margin: 2px 0 0; font-size: 12px; color: var(--muted); }.obstructed .route-state { color: var(--accent); }.empty { margin: 20px 0; }footer { flex-wrap: wrap; align-items: center; border-top: 1px solid #39493535; margin-top: 24px; padding-top: 12px; font-size: 13px; }button { flex-shrink: 0; min-height: 44px; background: transparent; border: 0; color: inherit; font: inherit; padding: 10px 12px; cursor: pointer; }button:hover:not(:disabled) { background: #35433112; }button:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }button:disabled { opacity: .5; cursor: default; }.error { color: #99372f; font-size: 13px; }.error,.hint { overflow-wrap: anywhere; }
@media(max-width:600px) { .map-sheet { margin: auto 0 0; width: 100%; max-width: 100%; padding: 24px 24px max(24px,env(safe-area-inset-bottom)); border-inline: 0; border-bottom: 0; }ul { padding-left: 20px; }li::before { width: 14px; left: -20px; } }
</style>
