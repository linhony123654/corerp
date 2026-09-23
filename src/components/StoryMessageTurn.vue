<script setup lang="ts">
/**
 * StoryMessageTurn — 一条叙事回合的呈现
 *
 * 脊线母题：形状 = 来源，缩进 = 时间深度。消息不做成卡片。
 * Markdown 只经 lib/markdown.ts 的白名单渲染（禁止任意 HTML 进 tag 位置）。
 */
import { computed } from 'vue'
import type { NarrativeMessage } from '../types'
import { renderMarkdown } from '../lib/markdown'
import { fmtFullTime } from '../lib/format'

const props = defineProps<{
  message: NarrativeMessage
  /** 是否为最新一条世界回应（首屏高对比落点） */
  latest?: boolean
  /** 时间深度缩进量（px） */
  depth?: number
  /** 追加后首次渲染时播一次沉降入场 */
  entering?: boolean
}>()

const emit = defineEmits<{
  (e: 'focusRef', id: string): void
}>()

const html = computed(() => renderMarkdown(props.message.body))

const srcLabel: Record<NarrativeMessage['source'], string> = {
  world: 'WORLD',
  player: '你',
  system: 'SYSTEM'
}

/** 表格被包一层可横滑容器，避免窄屏撑破 */
const tableHtml = computed(() => html.value.replace(/<table>/g, '<div class="prose__tablewrap"><table>').replace(/<\/table>/g, '</table></div>'))

function onClickRefs(e: MouseEvent) {
  const t = e.target as HTMLElement
  const btn = t.closest<HTMLElement>('[data-ref]')
  if (!btn) return
  emit('focusRef', btn.dataset.ref!)
}
</script>

<template>
  <article
    class="turn"
    :class="[
      'turn--' + message.source,
      { 'turn--latest': latest, 'turn--enter': entering, 'turn--unread': message.unread }
    ]"
    :data-mid="message.message_id"
    :style="{ '--depth': (depth ?? 0) * 8 + 'px', paddingLeft: 'var(--depth)' }"
    :aria-label="`第 ${message.turn} 回合 · ${srcLabel[message.source]}`"
  >
    <div class="turn__spine" aria-hidden="true">
      <span class="turn__glyph" />
    </div>

    <div class="turn__body">
      <header class="turn__meta">
        <span class="turn__src">{{ srcLabel[message.source] }}</span>
        <span class="turn__sep" aria-hidden="true">·</span>
        <span class="turn__time">{{ fmtFullTime(message.world_time) }}</span>
        <span class="turn__no mono">#{{ message.turn }}</span>
      </header>

      <div class="prose" v-html="tableHtml" @click="onClickRefs" />

      <div v-if="message.fact_refs.length" class="turn__refs">
        <button
          v-for="r in message.fact_refs"
          :key="r"
          class="turn__ref"
          type="button"
          :data-ref="r"
          :title="`在观测台查看 ${r}`"
          @click.stop="emit('focusRef', r)"
        >
          {{ r }}
        </button>
      </div>

      <p v-if="message.demo" class="turn__demo">
        <span aria-hidden="true">※</span> 演示内容 · 非内核输出
      </p>
    </div>
  </article>
</template>
