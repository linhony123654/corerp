<script setup lang="ts">
import PlayIcon from './PlayIcon.vue'
import PlayRegenerate from './PlayRegenerate.vue'
import type { NarrativeStreamResult } from '../lib/narrativeStream'
import { failedDecision, incompleteRPContext, type ProviderCall } from '../lib/providerReceipt'

type Turn = { turn_run_id: string; narrative_lines: string[]; render_id?: string; can_regenerate: boolean; provider_calls?: ProviderCall[] }
type LineKind = 'player' | 'npc' | 'event' | 'narration'
type Classified = {
  text: string
  kind: LineKind
  head?: string
  speaker?: string
  quote?: string
  initial?: string
  icon?: string
}

// Reactive props destructure (Vue 3.5): template keeps bare identifiers, script gets playerName.
const { turns, variants, preview, place, session, locked, status, playerName = '', api, stream } = defineProps<{
  turns: Turn[]; variants: Record<string, string[]>; preview: string[] | null
  place: string; session: string; locked: boolean; status: string
  playerName?: string
  api: <T>(path: string, body: Record<string, unknown>) => Promise<T>
  stream: (body: Record<string, unknown>, preview: (lines: string[]) => void, signal: AbortSignal) => Promise<NarrativeStreamResult>
}>()
const emit = defineEmits<{ read: [id: string]; thinking: []; variant: [id: string, lines: string[] | null] }>()
const readableLine = (line: string) => line.replace(/(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}):\d{2}Z/g, '$1 $2')
const isLong = (lines: string[]) => lines.join('').length > 430

/* Classify each narrative line into player speech / NPC speech / world event /
 * plain narration. The rendered text always stays identical to the source line;
 * classification only drives presentation (decorative spans and CSS). */
const headRe = /^(?:（[^）]*）|当时，|(?:在|回到)[^\n：]{1,60}?[，。]\n?)/
function classify(raw: string): Classified {
  const text = readableLine(raw)
  let rest = text
  let head = ''
  let hm: RegExpMatchArray | null
  while ((hm = rest.match(headRe))) {
    head += hm[0]
    rest = rest.slice(hm[0].length)
  }
  const speech = rest.match(/^([^\n：「]{1,24}?)(说| 回应| 拒绝了)：([\s\S]*)$/)
  if (speech && /[「“]/.test(speech[3])) {
    const name = speech[1]
    const isPlayer = name === '你' || name === '我' || (!!playerName && name === playerName)
    return {
      text,
      kind: isPlayer ? 'player' : 'npc',
      head,
      speaker: `${name}${speech[2]}：`,
      quote: speech[3],
      initial: isPlayer ? undefined : name.slice(0, 1),
    }
  }
  const ev = rest.match(/^[^\n：]{1,24}?\s?(前往了 .+|等待至 .+|离开了。|保持沉默。|选择等待。)$/)
  if (ev) {
    const action = ev[1]
    const icon = action.startsWith('前往') ? 'location' : action.startsWith('等待至') ? 'clock' : action === '保持沉默。' ? 'moon' : 'right'
    return { text, kind: 'event', icon }
  }
  return { text, kind: 'narration' }
}

const classifyTurn = (turn: Turn) => (variants[turn.turn_run_id] || turn.narrative_lines).map(classify)

/* A date separator appears only when the world date embedded in the lines
 * actually changes between consecutive turns. */
function turnDate(turn: Turn): string | null {
  for (const line of variants[turn.turn_run_id] || turn.narrative_lines) {
    const m = line.match(/(\d{4}-\d{2}-\d{2})T\d{2}:\d{2}/)
    if (m) return m[1]
  }
  return null
}
function dateSeparator(index: number): string | null {
  const current = turnDate(turns[index])
  if (!current) return null
  for (let i = index - 1; i >= 0; i--) {
    const prev = turnDate(turns[i])
    if (prev) return prev === current ? null : current
  }
  return current
}
</script>

<template>
  <div class="transcript">
    <p class="date-rule">{{ place }} · 最近 {{ turns.length }} 段世界记录</p>
    <div v-if="!turns.length" class="empty-state"><PlayIcon name="book" :size="28" /><h2>从一句话开始</h2><p>写下你的对白或行动，世界会保存已发生的事。</p></div>
    <template v-for="(turn, ti) in turns" :key="turn.turn_run_id">
      <p v-if="dateSeparator(ti)" class="date-rule turn-date">{{ dateSeparator(ti) }}</p>
      <article :id="`turn-${turn.turn_run_id}`" class="turn narrator">
        <div class="prose" :class="{ 'collapsed-prose': isLong(variants[turn.turn_run_id] || turn.narrative_lines) }">
          <p
            v-for="(line, i) in classifyTurn(turn)"
            :key="i"
            :class="`line-${line.kind}`"
            :data-initial="line.initial"
          ><template v-if="line.speaker"><span v-if="line.head" class="line-head">{{ line.head }}</span><span class="line-speaker">{{ line.speaker }}</span><span class="line-quote">{{ line.quote }}</span></template><template v-else-if="line.kind === 'event'"><PlayIcon :name="line.icon || 'right'" :size="14" /><span>{{ line.text }}</span></template><template v-else>{{ line.text }}</template></p>
        </div>
        <p v-if="incompleteRPContext(turn.provider_calls)" class="provider-alert" role="status">本轮角色设定尚未就绪，请世界创建者补齐角色设定。模型未调用；当前没有人物回复，不代表角色主动选择沉默。</p>
        <p v-else-if="failedDecision(turn.provider_calls)" class="provider-alert" role="status">本轮人物模型调用失败或超时，NPC 的沉默是技术回退，不代表角色主动选择沉默；你可以再次表达，但不会撤销已发生的回合。</p>
        <button v-if="isLong(variants[turn.turn_run_id] || turn.narrative_lines)" class="read-link" aria-haspopup="dialog" @click="emit('read', turn.turn_run_id)"><PlayIcon name="book" :size="15" /><span>阅读完整剧情</span><span>约 {{ (variants[turn.turn_run_id] || turn.narrative_lines).join('').length }} 字</span></button>
        <PlayRegenerate v-if="turn.can_regenerate" :session="session" :turn="turn.turn_run_id" :selected-render="turn.render_id" :disabled="locked" :api="api" :stream="stream" @variant="emit('variant', turn.turn_run_id, $event)" />
      </article>
    </template>
    <section v-if="status || preview !== null" class="turn incoming" aria-label="当前请求状态" :aria-busy="!!status || preview !== null">
      <button class="thinking-line" :class="{ 'is-running': !!status || preview !== null }" aria-haspopup="dialog" @click="emit('thinking')"><PlayIcon name="clock" :size="18" /><span class="thinking-copy" role="status">{{ status || '行动已提交 · 正在接收叙述' }}</span><PlayIcon name="right" :size="15" /></button>
      <div v-if="preview?.length" class="prose" role="region" aria-label="当前回合叙述流">
        <p
          v-for="(line, i) in preview.map(classify)"
          :key="i"
          :class="`line-${line.kind}`"
          :data-initial="line.initial"
        ><template v-if="line.speaker"><span v-if="line.head" class="line-head">{{ line.head }}</span><span class="line-speaker">{{ line.speaker }}</span><span class="line-quote">{{ line.quote }}</span></template><template v-else-if="line.kind === 'event'"><PlayIcon :name="line.icon || 'right'" :size="14" /><span>{{ line.text }}</span></template><template v-else>{{ line.text }}</template></p>
      </div>
    </section>
  </div>
</template>

<style scoped>
.provider-alert {
  margin: 0.7rem 0 0;
  padding: 0.55rem 0.7rem;
  border-left: 2px solid var(--accent, #9c5138);
  color: var(--text-muted, #665f57);
  font: 0.78rem/1.5 var(--ui, sans-serif);
}
</style>
