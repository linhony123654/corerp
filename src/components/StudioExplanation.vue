<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'

const props = defineProps<{ token: string; instance: string; branch: string; event: string; access: string }>()
const emit = defineEmits<{ inspectEvent: [eventID: string] }>()
interface Diagnostic { record_id: string; record_order: number; authority: string; status: string; reason_code: string; action?: string; npc_entity_id?: string }
interface Observer { entity_id: string; observation_count: number; first_observed_time: string }
interface Explanation {
  committed_evidence: string
  committed_decision?: { source_kind: string; status?: string; reason_code?: string; decision_id: string; trigger_event_id: string; trigger_sequence: number; npc_entity_id: string; action: string; outcome: string }
  instance_id: string; branch_id: string; event_id: string; access_level: string
  diagnostic_evidence: string; observation_evidence: string; through_record_order: number
  next_record_order?: number; next_observer_id?: string; diagnostics: Diagnostic[]; observers?: Observer[]
}
const result = ref<Explanation | null>(null), busy = ref(false), error = ref(''), observer = ref('')
let controller: AbortController | undefined, revision = 0
const reasons: Record<string, string> = {
  action_not_legal: '动作不在当前合法候选范围', invalid_speech_fields: '发言字段不符合约束',
  movement_contains_speech: '移动候选夹带发言', destination_not_reachable: '目的地不可达',
  noop_contains_effects: '无操作候选夹带效果', unknown_action: '无法识别的动作',
  provider_failure: '决策提供方失败，已使用回退', contact_opportunity_suppressed: '当前接触机会规则要求保持安静', not_recorded: '没有记录具体原因'
}
const statuses: Record<string, string> = { validated: '候选通过验证', rejected: '候选被拒绝', provider_fallback: '提供方回退' }
const outcomes: Record<string, string> = { committed_speech: '已提交发言', committed_movement: '已提交移动', committed_silence: '已提交沉默（未发言、未移动）', committed_wait: '已提交等待（未发言、未移动）' }
function reset() { revision++; controller?.abort(); controller = undefined; result.value = null; busy.value = false; error.value = '' }
watch(() => [props.token, props.instance, props.branch, props.event, props.access], () => { reset(); observer.value = '' }, { flush: 'sync' })
watch(observer, reset, { flush: 'sync' })
onUnmounted(reset)
async function read(more: 'diagnostics' | 'observers' | null = null) {
  const previous = result.value
  if (!more) reset()
  controller?.abort(); const version = ++revision, active = new AbortController(); controller = active
  busy.value = true; error.value = ''
  const binding = { instance_id: props.instance, branch_id: props.branch, event_id: props.event }
  const timer = setTimeout(() => active.abort(), 20000)
  try {
    const response = await fetch('/api/v1/studio/events/explain', {
      method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${props.token}` },
      credentials: 'omit', redirect: 'error', cache: 'no-store', signal: active.signal,
      body: JSON.stringify({ ...binding, limit: 10, ...(observer.value.trim() ? { observer_id: observer.value.trim() } : {}), ...(more && previous ? { through_record_order: previous.through_record_order, ...(more === 'diagnostics' ? { after_record_order: previous.next_record_order } : { after_observer_id: previous.next_observer_id }) } : {}) })
    })
    if (version !== revision) return
    if (!response.ok) {
      const messages: Record<number, string> = { 401: '凭证无效，请重新输入。', 403: '需要额外的解释权限，请由本地管理员显式开通。', 404: '当前授权范围内没有找到事件或观察者。', 400: '解释查询范围或分页无效，请重新读取。' }
      error.value = messages[response.status] ?? '解释暂时不可用，请重试。'; result.value = null; return
    }
    const { data } = await response.json() as { data: Explanation }
    if (version !== revision) return
    if (!data || data.instance_id !== binding.instance_id || data.branch_id !== binding.branch_id || data.event_id !== binding.event_id || data.access_level !== props.access || data.diagnostic_evidence !== 'npc_candidate_validation_only' || !Array.isArray(data.diagnostics) || (data.observers !== undefined && !Array.isArray(data.observers)) || (more && data.through_record_order !== previous?.through_record_order)) throw new Error('scope')
    if (props.access === 'operator' && (data.committed_evidence !== 'redacted' || data.committed_decision !== undefined || data.observation_evidence !== 'redacted' || data.observers !== undefined || data.diagnostics.some(d => d.npc_entity_id !== undefined || d.action !== undefined))) throw new Error('redaction')
    result.value = more && previous ? { ...data,
      diagnostics: more === 'diagnostics' ? [...previous.diagnostics, ...data.diagnostics] : previous.diagnostics,
      observers: more === 'observers' ? [...(previous.observers ?? []), ...(data.observers ?? [])] : previous.observers,
      next_record_order: more === 'diagnostics' ? data.next_record_order : previous.next_record_order,
      next_observer_id: more === 'observers' ? data.next_observer_id : previous.next_observer_id
    } : data
  } catch {
    if (version === revision) { result.value = null; error.value = active.signal.aborted ? '解释读取超时，请重试。' : '解释连接或响应异常，已清除结果。' }
  } finally { clearTimeout(timer); if (version === revision) { busy.value = false; controller = undefined } }
}
</script>

<template>
  <section class="studio-explanation" aria-label="记录解释" :aria-busy="busy">
    <h3><span>04</span> 决策与观察证据</h3>
    <p class="boundary">独立授权 · 区分候选验证与实际提交。通过验证不等于动作已经执行；没有观察记录也不代表人物一定不知道。</p>
    <label v-if="access === 'creator'">观察者标识（可选）<input v-model="observer" maxlength="256" autocomplete="off" spellcheck="false" placeholder="留空查看已记录的观察者" /></label>
    <button type="button" :disabled="busy" @click="read()">{{ busy ? '正在读取解释…' : '读取记录解释' }}</button>
    <p v-if="error" class="explanation-error" role="alert">{{ error }}</p>
    <div v-if="result" class="explanation-records">
      <h4>已提交的 NPC 结果</h4>
      <div v-if="result.committed_decision" class="committed-relation">
        <strong>{{ outcomes[result.committed_decision.outcome] ?? '未识别的提交结果' }}</strong>
        <p>{{ result.committed_decision.npc_entity_id }} · {{ result.committed_decision.action }}</p>
        <p v-if="result.committed_decision.reason_code">{{ reasons[result.committed_decision.reason_code] ?? '没有记录具体原因' }}</p>
        <small>{{ result.committed_decision.decision_id }}</small>
        <button type="button" @click="emit('inspectEvent', result.committed_decision.trigger_event_id)">查看触发事件 #{{ result.committed_decision.trigger_sequence }}</button>
        <p class="boundary">关联来自不可变的事件与提交记录，说明触发与结果，不证明人物的主观动机。</p>
      </div>
      <p v-else>{{ result.committed_evidence === 'redacted' ? '提交关联不在 ops 诊断权限内。' : '此事件没有已记录的 NPC 提交关联；不代表其他类型的因果关系不存在。' }}</p>
      <p class="boundary">非权威诊断 · 审计截至 #{{ result.through_record_order }} · 不会推进世界</p>
      <h4>NPC 候选验证</h4>
      <p v-if="!result.diagnostics.length" role="status">此事件没有保存的候选验证记录，无法据此推断原因。</p>
      <ol><li v-for="item in result.diagnostics" :key="item.record_id"><strong>{{ statuses[item.status] ?? '无法识别的状态' }}</strong><p>{{ reasons[item.reason_code] ?? '没有记录具体原因' }}</p><small v-if="item.npc_entity_id">{{ item.npc_entity_id }} · {{ item.action }}</small><small>{{ item.record_id }}</small></li></ol>
      <button v-if="result.next_record_order" type="button" :disabled="busy" @click="read('diagnostics')">更多验证记录</button>
      <h4>已记录的观察</h4>
      <p v-if="access === 'operator'">观察者信息不在 ops 诊断权限内。</p>
      <template v-else>
        <p v-if="!result.observers?.length" role="status">没有找到观察记录；这不是“不知道”的证明。</p>
        <ul><li v-for="item in result.observers" :key="item.entity_id"><strong>{{ item.entity_id }}</strong><p>{{ item.observation_count }} 条记录 · 首次 {{ item.first_observed_time }}</p></li></ul>
        <button v-if="result.next_observer_id" type="button" :disabled="busy" @click="read('observers')">更多观察者</button>
      </template>
    </div>
  </section>
</template>

<style scoped>
.studio-explanation{border-top:1px solid var(--line);margin-top:24px;padding-top:18px;font-size:14px;line-height:1.8;overflow-wrap:anywhere}h3{font-size:16px;font-weight:500}h3 span{font:12px var(--font-mono);color:var(--amber-400);margin-right:12px}h4{font-size:14px;margin:22px 0 10px;color:var(--amber-400)}.boundary,small{color:var(--ink-700);font-size:12px}label{display:block;font-size:13px;margin:16px 0}input{display:block;box-sizing:border-box;width:100%;padding:12px;margin-top:6px;background:var(--ink-100);border:1px solid var(--ink-500);color:var(--ink-900);font:13px var(--font-mono)}button{min-height:44px;padding:8px 16px;border:1px solid var(--line-strong);background:transparent;color:var(--ink-800);font:inherit;cursor:pointer}button:disabled{opacity:.5;cursor:wait}button:focus-visible,input:focus-visible{outline:2px solid var(--teal-400);outline-offset:4px}ol,ul{list-style:none;padding:0;margin:0}li{border-left:2px solid var(--line-amber);padding:10px 16px;margin:12px 0}li p{margin:4px 0}small{display:block;font-family:var(--font-mono)}strong{font-weight:500}.explanation-error{color:var(--rose-400);border-left:3px solid var(--rose-400);padding-left:14px}
</style>
