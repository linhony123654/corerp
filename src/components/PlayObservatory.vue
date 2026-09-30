<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

type Activation = { actor_ref: string; display_name: string; disposition: string; reason_code: string; activation_rank?: number }
type Action = { actor_ref: string; display_name: string; action: string; detail?: string; place_name?: string; activity?: string }
type Trace = {
  trace_id: string; sequence: number; world_time?: string; place_name?: string; stage: string
  execution_mode: string; responder_limit: number; player_action?: string
  activations: Activation[]; committed_actions: Action[]
  narrative_fallback: { used: boolean; reason_code?: string; provider_mode?: string }
}
type Observatory = {
  world_time: string; current_place: string; traces: Trace[]; next_before_sequence?: number
  projection_health: { status: string; difference_count: number; checked_through_sequence: number }
  background_health: { enabled: boolean; status: string; last_target_world_time?: string; processed_items?: number; updated_at_utc?: string }
}

const props = defineProps<{ read: (before: number) => Promise<Observatory> }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const view = ref<Observatory | null>(null), traces = ref<Trace[]>([]), next = ref(0)
const loading = ref(false), error = ref('')
let active = true

const stages: Record<string, string> = { open: '请求已记录', player_committed: '玩家行动已提交', npc_deciding: '人物选择中', npc_effects_committed: '人物行动已提交', narrative_ready: '叙述已准备', settled: '已完成' }
const modes: Record<string, string> = { legacy: '兼容模式', deterministic: '确定性', orchestrated: '编排模式', multi_agent: '多人物模式' }
const dispositions: Record<string, string> = { activated: '已激活', externally_controlled: '由外部控制', not_activated: '未激活' }
const reasons: Record<string, string> = { legacy_compatible: '兼容原有世界', direct_address: '被明确叫到', conversation_continuation: '接续上一轮交流', stable_fallback: '稳定顺序选择', external_controller: '已有控制者', responder_limit: '达到回应人数上限' }
const actions: Record<string, string> = { respond: '回应', refuse: '婉拒', silence: '保持沉默', wait: '等待', leave: '离开', act: '开始活动' }
const fallbackReasons: Record<string, string> = { provider_unavailable: '长文叙述器不可用', provider_error: '叙述器返回错误', presentation_rejected: '叙述未通过事实校验', deterministic_fallback: '改用标准叙述', record_unreadable: '回退记录无法读取' }
const backgroundStates: Record<string, string> = { disabled: '后台推进未开启', idle: '等待下一次后台推进', running: '正在推进', completed: '上次推进完成', controlled: '玩家或外部控制器正在掌控', busy: '另一工作进程正在执行', budget_exhausted: '预算已用完，等待续跑', failed: '上次推进失败' }
const time = (value?: string) => value ? value.replace('T', ' ').replace(/Z$/, '') : '尚未产生世界事件'

async function load(more = false) {
  if (loading.value) return
  loading.value = true; error.value = ''
  try {
    const result = await props.read(more ? next.value : 0)
    if (!active) return
    view.value = result
    traces.value = more ? [...traces.value, ...result.traces] : result.traces
    next.value = result.next_before_sequence || 0
  } catch (cause) {
    if (active) error.value = cause instanceof Error ? cause.message : '暂时无法读取公开观测记录。'
  } finally { if (active) loading.value = false }
}
function containTab(event: KeyboardEvent) {
  if (event.key !== 'Tab') return
  const items = dialog.value?.querySelectorAll<HTMLElement>('button:not(:disabled),summary')
  if (!items?.length) return
  const first = items[0], last = items[items.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}
onMounted(() => { dialog.value?.showModal(); void load() })
onBeforeUnmount(() => { active = false; dialog.value?.close() })
</script>

<template>
  <dialog ref="dialog" class="observatory-sheet" aria-labelledby="observatory-title" @close="emit('close')" @keydown="containTab">
    <header><div><p class="eyebrow">世界 · 公开回放</p><h2 id="observatory-title">世界观测</h2></div><button autofocus aria-label="关闭世界观测" @click="dialog?.close()">收起 ×</button></header>
    <p class="intro">查看当前会话亲历的已提交阶段；这里不展示人物私密目标、模型思维或原始证据标识。</p>
    <p v-if="loading && !view" role="status" class="state">正在核对回合与投影…</p>
    <div v-if="error" class="state error"><p role="alert">{{ error }}</p><button :disabled="loading" @click="load(false)">重新读取</button></div>
    <template v-if="view">
      <section class="health" aria-label="投影健康">
        <div><span :class="['health-dot', view.projection_health.status]" /><div><b>{{ view.projection_health.status === 'healthy' ? '投影与事件一致' : '发现投影差异' }}</b><small>核对至序列 {{ view.projection_health.checked_through_sequence }}</small></div></div>
        <p>{{ view.current_place }} · {{ time(view.world_time) }}</p>
      </section>
      <section class="background-health" aria-label="后台世界推进">
        <div><b>{{ backgroundStates[view.background_health.status] || view.background_health.status }}</b><small>{{ view.background_health.enabled ? '世界包已显式授权；有人控制时会自动暂停。' : '世界不会在无人操作时自行前进。' }}</small></div>
        <p v-if="view.background_health.last_target_world_time">目标 {{ time(view.background_health.last_target_world_time) }} · 已处理 {{ view.background_health.processed_items || 0 }} 项</p>
      </section>
      <p v-if="!traces.length" class="empty">这个会话还没有可回放的角色扮演回合。</p>
      <ol v-else class="trace-list" aria-label="公开回合记录">
        <li v-for="trace in traces" :key="trace.trace_id">
          <details>
            <summary><span><time>{{ time(trace.world_time) }}</time><b>{{ stages[trace.stage] || '公开阶段' }}</b><small>{{ trace.place_name || '位置尚未提交' }} · {{ modes[trace.execution_mode] || trace.execution_mode }}</small></span><i>序列 {{ trace.sequence }}</i></summary>
            <div class="trace-body">
              <section><h3>你的行动</h3><p>{{ trace.player_action || '尚未产生已提交的玩家行动。' }}</p></section>
              <section><h3>人物激活</h3><p v-if="!trace.activations.length" class="hint">此回合没有已记录的听者激活。</p><ul><li v-for="actor in trace.activations" :key="actor.actor_ref"><span><b>{{ actor.display_name }}</b><small>{{ reasons[actor.reason_code] || '公开原因已记录' }}</small></span><em>{{ dispositions[actor.disposition] || actor.disposition }}</em></li></ul></section>
              <section><h3>已提交行动</h3><p v-if="!trace.committed_actions.length" class="hint">没有人物行动写入这个回合。</p><ul><li v-for="action in trace.committed_actions" :key="`${action.actor_ref}:${action.action}`"><span><b>{{ action.display_name }} · {{ actions[action.action] || action.action }}</b><small>{{ action.detail || action.activity || action.place_name || '没有额外公开内容' }}</small></span></li></ul></section>
              <p v-if="trace.narrative_fallback.used" class="fallback">叙述回退：{{ fallbackReasons[trace.narrative_fallback.reason_code || ''] || '已改用标准叙述' }}<span v-if="trace.narrative_fallback.provider_mode"> · {{ trace.narrative_fallback.provider_mode }}</span></p>
            </div>
          </details>
        </li>
      </ol>
      <button v-if="next" class="load-more" :disabled="loading" @click="load(true)">{{ loading ? '正在读取…' : '读取更早记录' }}</button>
    </template>
    <footer><p>只读检查不会推进世界，也不会产生新事件。</p><button :disabled="loading" @click="load(false)">重新核对</button></footer>
  </dialog>
</template>

<style scoped>
.observatory-sheet{margin:auto;width:min(760px,calc(100% - 32px));max-height:calc(100dvh - 40px);overflow:auto;padding:28px;border:1px solid #7e493550;border-top:4px solid var(--accent);background:var(--paper);color:var(--text);box-shadow:0 20px 70px #17281b30}.observatory-sheet::backdrop{background:#18251dc0}.observatory-sheet button{min-height:44px;padding:10px 12px;border:0;border-radius:2px;color:inherit;background:transparent;font:inherit;cursor:pointer}.observatory-sheet button:hover:not(:disabled){background:#35433112}.observatory-sheet button:disabled{opacity:.48;cursor:default}.observatory-sheet :is(button,summary):focus-visible{outline:2px solid var(--accent);outline-offset:4px}header{display:flex;justify-content:space-between;align-items:flex-start;gap:18px}.eyebrow{color:var(--accent);font-size:11px;letter-spacing:2px;margin:0 0 10px}h2{font:400 30px/1.35 var(--font-serif);margin:0}.intro{font:15px/1.85 var(--font-serif);margin:20px 0;color:var(--secondary)}.state,.empty{padding:24px 0;font-size:14px;line-height:1.8}.error{color:var(--danger)}.error button{text-decoration:underline}.health{display:flex;justify-content:space-between;gap:20px;align-items:center;padding:16px 0;border-block:1px solid var(--line)}.health>div{display:flex;align-items:center;gap:10px}.health-dot{width:9px;height:9px;border-radius:50%;background:var(--danger);box-shadow:0 0 0 5px color-mix(in srgb,var(--danger) 12%,transparent)}.health-dot.healthy{background:#4c7756;box-shadow:0 0 0 5px #4c77561c}.health b,.health small{display:block}.health small,.health p{font-size:11px;color:var(--muted);margin:3px 0 0}.trace-list{list-style:none;padding:0;margin:20px 0}.trace-list>li{border-bottom:1px solid var(--line)}details summary{display:flex;justify-content:space-between;gap:18px;align-items:center;padding:17px 2px;cursor:pointer;list-style:none}details summary::-webkit-details-marker{display:none}summary span>*{display:block}summary time{font-size:11px;color:var(--muted);font-variant-numeric:tabular-nums}summary b{font-size:14px;margin:4px 0}summary small{font-size:11px;color:var(--secondary)}summary i{font-style:normal;font-size:10px;color:var(--muted);white-space:nowrap}.trace-body{padding:0 2px 20px}.trace-body section{margin-top:16px}.trace-body h3{font-size:11px;letter-spacing:.04em;color:var(--muted);margin:0 0 8px}.trace-body p{font-size:13px;line-height:1.75;margin:0}.trace-body ul{list-style:none;padding:0;margin:0}.trace-body li{display:flex;justify-content:space-between;gap:14px;padding:9px 0}.trace-body li b,.trace-body li small{display:block}.trace-body li b{font-size:13px}.trace-body li small,.hint{font-size:11px;color:var(--muted);margin-top:3px}.trace-body li em{font-style:normal;font-size:11px;color:var(--accent);white-space:nowrap}.fallback{margin-top:17px!important;padding:11px 13px;border-left:2px solid var(--accent);background:var(--soft);color:var(--secondary)}.load-more{width:100%;border-top:1px solid var(--line)!important}footer{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-top:12px;font-size:12px;color:var(--muted)}
.background-health{display:flex;justify-content:space-between;gap:18px;padding:13px 0;border-bottom:1px solid var(--line)}.background-health b,.background-health small{display:block}.background-health b{font-size:12px}.background-health small,.background-health p{font-size:11px;color:var(--muted);margin:3px 0 0}.background-health p{text-align:right}
@media(max-width:600px){.observatory-sheet{width:100%;max-width:100%;margin:auto 0 0;max-height:calc(100dvh - 24px);padding:24px 22px max(24px,env(safe-area-inset-bottom));border-inline:0;border-bottom:0}.health{align-items:flex-start;flex-direction:column;gap:8px}details summary{align-items:flex-start}.trace-body li{align-items:flex-start}}
</style>
