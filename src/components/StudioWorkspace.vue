<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import StudioExplanation from './StudioExplanation.vue'

interface RulePackages {
  lock: { activation_event_id: string; algorithm: string; phase: string; conflict_order: string; system: { install_event_id: string }; narrative: { install_event_id: string } }
  system: { manifest: { id: string; version: string }; content: unknown }
  narrative: { manifest: { id: string; version: string }; content: unknown }
}
interface Evidence {
  instance_id: string; branch_id: string; head_sequence: number; access_level: 'creator' | 'operator'
  event_id: string; event_sequence: number; event_type: string; world_time: string
  batch_id: string; batch_hash: string
  rule: { epoch_id: string; ruleset_hash: string; start_sequence: number; end_sequence?: number; package_status?: string; packages?: RulePackages }
  actor_id?: string; command_id?: string; causation_event_id?: string
  causation_evidence: string; payload?: unknown; redacted: boolean
}
class StudioReadError extends Error {}
interface Scope { instance_id: string; branch_id: string; label: string; head_sequence: number; access_level: string }
interface ScopeKey { instance_id: string; branch_id: string }
interface TimelineEvent { event_id: string; event_sequence: number; event_type: string; world_time: string }
const scopes = ref<Scope[]>([]), scopesChecked = ref(false), nextScope = ref<ScopeKey | undefined>()
const timeline = ref<TimelineEvent[]>([]), timelineChecked = ref(false), timelineHead = ref(0), nextBefore = ref(0), listing = ref(false)
let listingController: AbortController | undefined
let listingRevision = 0
const token = ref(''), instance = ref(''), branch = ref(''), event = ref('')
const evidence = ref<Evidence | null>(null), busy = ref(false), error = ref('')
let active: AbortController | undefined
let revision = 0
const canRead = computed(() => [token.value, instance.value, branch.value, event.value].every(v => v.trim()))
const payload = computed(() => evidence.value?.payload === undefined ? '' : JSON.stringify(evidence.value.payload, null, 2))
function invalidate() {
  revision++; active?.abort(); active = undefined; busy.value = false
  evidence.value = null; error.value = ''
}
function resetDirectory(all: boolean) {
  listingRevision++; listingController?.abort(); listingController = undefined; listing.value = false
  timeline.value = []; timelineChecked.value = false; timelineHead.value = 0; nextBefore.value = 0
  if (all) { scopes.value = []; scopesChecked.value = false; nextScope.value = undefined }
}
function clear() { invalidate(); resetDirectory(true); token.value = '' }
watch(token, () => { invalidate(); resetDirectory(true) }, { flush: 'sync' })
watch([instance, branch], () => { invalidate(); resetDirectory(false) }, { flush: 'sync' })
watch(event, invalidate, { flush: 'sync' })
onMounted(() => window.addEventListener('pagehide', clear))
onUnmounted(() => { clear(); window.removeEventListener('pagehide', clear) })
async function browse(kind: 'scopes' | 'events', more = false) {
  if (!token.value.trim() || (kind === 'events' && (!instance.value.trim() || !branch.value.trim()))) return
  if (kind === 'scopes' && !more) resetDirectory(true)
  invalidate(); listingController?.abort(); const version = ++listingRevision
  const controller = new AbortController(); listingController = controller; listing.value = true
  const binding = { instance_id: instance.value.trim(), branch_id: branch.value.trim() }
  const body = kind === 'scopes' ? { limit: 20, ...(more ? { after: nextScope.value } : {}) } : { ...binding, limit: 5, ...(more ? { through_sequence: timelineHead.value, before_sequence: nextBefore.value } : {}) }
  if (!more) { if (kind === 'scopes') { scopes.value = []; nextScope.value = undefined; scopesChecked.value = false } else { timeline.value = []; nextBefore.value = 0; timelineChecked.value = false } }
  const timer = setTimeout(() => controller.abort(), 20000)
  try {
    const response = await fetch(`/api/v1/studio/${kind === 'scopes' ? 'scopes' : 'events'}/list`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token.value}` }, credentials: 'omit', redirect: 'error', cache: 'no-store', signal: controller.signal, body: JSON.stringify(body) })
    if (!response.ok) throw new StudioReadError(response.status === 401 ? '凭证无效，请重新输入。' : response.status === 403 ? '没有此列表的检查权限，或授权已撤销。' : '列表暂时不可用，请重新读取。')
    const { data } = await response.json()
    if (version !== listingRevision) return
    if (kind === 'scopes') {
      if (!data || !Array.isArray(data.scopes)) throw new StudioReadError('列表响应格式不匹配。')
      scopes.value = more ? [...scopes.value, ...data.scopes] : data.scopes; nextScope.value = data.next_after; scopesChecked.value = true
    } else {
      if (!data || !Array.isArray(data.events) || data.instance_id !== binding.instance_id || data.branch_id !== binding.branch_id || (more && data.through_sequence !== timelineHead.value)) throw new StudioReadError('事件列表的授权范围或快照不匹配。')
      timeline.value = more ? [...timeline.value, ...data.events] : data.events; timelineHead.value = data.through_sequence; nextBefore.value = data.next_before_sequence ?? 0; timelineChecked.value = true
    }
  } catch (cause) {
    if (version === listingRevision) { const timedOut = controller.signal.aborted; invalidate(); resetDirectory(true); error.value = timedOut ? '列表读取超时，请重试。' : cause instanceof StudioReadError ? cause.message : '列表连接或响应异常，请重试。' }
  } finally { clearTimeout(timer); if (version === listingRevision) { listing.value = false; listingController = undefined } }
}
function selectScope(scope: Scope) { instance.value = scope.instance_id; branch.value = scope.branch_id; event.value = ''; void browse('events') }
function selectEvent(item: TimelineEvent) { event.value = item.event_id; void inspect() }
async function inspect() {
  invalidate()
  if (!canRead.value) return
  const version = revision
  const binding = { instance_id: instance.value.trim(), branch_id: branch.value.trim(), event_id: event.value.trim() }
  const controller = new AbortController(); active = controller; busy.value = true
  const timer = window.setTimeout(() => controller.abort(), 20000)
  try {
    const response = await fetch('/api/v1/studio/events/read', {
      method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token.value}` },
      credentials: 'omit', redirect: 'error', cache: 'no-store', signal: controller.signal, body: JSON.stringify(binding)
    })
    if (!response.ok) {
      if (response.status === 401 || response.status === 403) resetDirectory(true)
      const messages: Record<number, string> = { 401: '凭证无效，请重新输入。', 403: '没有此分支的检查权限。请由本地管理员配置；玩家凭证不能进入后台。', 404: '此授权范围内没有找到该事件。', 400: '请检查世界、分支与事件标识。' }
      throw new StudioReadError(messages[response.status] ?? '检查暂时不可用，请稍后重试。')
    }
    const { data } = await response.json() as { data: Evidence }
    if (version !== revision) return
    if (!data || data.instance_id !== binding.instance_id || data.branch_id !== binding.branch_id || data.event_id !== binding.event_id || !data.rule || !['creator', 'operator'].includes(data.access_level)) throw new StudioReadError('服务返回的证据范围不匹配。')
    if (data.access_level === 'operator' && (!data.redacted || data.payload !== undefined || data.actor_id !== undefined || data.command_id !== undefined || data.causation_event_id !== undefined || data.rule.packages !== undefined)) throw new StudioReadError('诊断响应未正确脱敏，已拒绝显示。')
    evidence.value = data
  } catch (cause) {
    if (version === revision) error.value = controller.signal.aborted ? '读取超时，请重试。' : cause instanceof StudioReadError ? cause.message : '连接或响应异常，请检查本地 Runtime 后重试。'
  } finally {
    clearTimeout(timer)
    if (version === revision) { busy.value = false; active = undefined }
  }
}
</script>

<template>
  <div class="studio">
    <header class="studio-top"><a class="studio-brand" href="/" @click="clear">Core<span>RP</span></a><span class="studio-label">STUDIO / 世界证据</span><a href="/studio/create" @click="clear">创建世界</a><a href="/" @click="clear">回到 Play ↗</a></header>
    <main>
      <div class="studio-title"><p>高级工作区 · 只读检查</p><h1>从事实，追溯世界。</h1><p>这里读取 Runtime 的真实记录，不改变时间、人物或已发生的事件。</p></div>
      <div class="studio-grid">
        <aside>
          <form @submit.prevent="inspect" aria-label="事件检查范围">
            <h2><span>01</span> 选择证据范围</h2>
            <label>检查凭证<input v-model="token" type="password" autocomplete="off" spellcheck="false" required maxlength="8192" /></label>
            <p class="hint">凭证仅留在本页内存，不与 Play 共用；离开或刷新后清除。</p>
            <div class="scope-picker">
              <button type="button" class="clear" :disabled="!token.trim() || listing" @click="browse('scopes')">列出授权世界</button>
              <p v-if="scopesChecked && !scopes.length" class="hint" role="status">没有已授权的世界分支，请联系本地管理员。</p>
              <ul v-if="scopes.length" class="studio-list"><li v-for="scope in scopes" :key="`${scope.instance_id}/${scope.branch_id}`"><button type="button" :disabled="listing" @click="selectScope(scope)"><span>{{ scope.label || scope.branch_id }}</span><small>{{ scope.instance_id }} / {{ scope.branch_id }}</small></button></li></ul>
              <button v-if="nextScope" type="button" class="clear" :disabled="listing" @click="browse('scopes', true)">更多授权分支</button>
            </div>
            <label>世界实例<input v-model="instance" autocomplete="off" spellcheck="false" required maxlength="256" /></label>
            <label>分支<input v-model="branch" autocomplete="off" spellcheck="false" required maxlength="256" /></label>
            <label>事件标识<input v-model="event" autocomplete="off" spellcheck="false" required maxlength="256" /></label>
            <button type="button" class="clear timeline-refresh" :disabled="!token.trim() || !instance.trim() || !branch.trim() || listing" @click="browse('events')">读取分支时间线</button>
            <button class="inspect" :disabled="!canRead || busy">{{ busy ? '正在读取…' : '读取真实事件' }}</button>
            <button type="button" class="clear" @click="clear">清除凭证与结果</button>
          </form>
          <details class="boundary"><summary>权限与证据边界</summary><p>creator 读取获授权的事件内容；ops 只看脱敏诊断信息。需要本地管理员显式配置检查权限，普通角色名称不代表授权。</p><p>规则摘要说明当时生效的规则集，不等于已经证明某条规则是事件的原因。</p></details>
        </aside>
        <section class="studio-result" :aria-busy="busy" aria-label="事件证据">
          <p v-if="error" class="error" role="alert">{{ error }}</p>
          <details v-if="timelineChecked" class="studio-timeline" :open="!evidence">
            <summary>分支时间线 · 截至 #{{ timelineHead }}</summary>
            <p class="hint">新到事件不会插入正在翻页的快照；重新读取时间线可查看最新记录。点选事件会再次校验权限。</p>
            <p v-if="!timeline.length" role="status">此范围还没有事件。</p>
            <ol class="studio-list"><li v-for="item in timeline" :key="item.event_id"><button type="button" :disabled="busy || listing" @click="selectEvent(item)"><span>#{{ item.event_sequence }} · {{ item.event_type }}</span><small>{{ item.world_time }} · {{ item.event_id }}</small></button></li></ol>
            <button v-if="nextBefore" type="button" class="clear" :disabled="listing" @click="browse('events', true)">更早的事件</button>
          </details>
          <div v-if="!evidence" class="empty"><span class="axis-mark">● ─ ─ ○</span><h2>{{ busy ? '核对授权与记录…' : '尚未读取事件' }}</h2><p>输入已有世界、分支和事件标识。没有证据时，不生成解释。</p></div>
          <article v-else>
            <div class="event-heading"><span class="sequence">#{{ evidence.event_sequence }}</span><div><p class="access">{{ evidence.access_level === 'creator' ? 'CREATOR · 获授权内容' : 'OPS · 脱敏诊断' }}</p><h2>{{ evidence.event_type }}</h2><time>{{ evidence.world_time }}</time></div></div>
            <p class="snapshot" role="status">读取快照 · 分支头 {{ evidence.head_sequence }} · 再次读取可检查最新授权与状态</p>
            <dl><dt>事件</dt><dd>{{ evidence.event_id }}</dd><dt>世界 / 分支</dt><dd>{{ evidence.instance_id }} / {{ evidence.branch_id }}</dd><dt>提交批次</dt><dd>{{ evidence.batch_id }}</dd><dt>批次摘要</dt><dd>{{ evidence.batch_hash }}</dd><template v-if="evidence.command_id"><dt>原始命令</dt><dd>{{ evidence.command_id }}</dd></template></dl>
            <section class="studio-rule"><h3><span>02</span> 当时生效的规则</h3><dl><dt>规则纪元</dt><dd>{{ evidence.rule.epoch_id }}</dd><dt>有效序号</dt><dd>{{ evidence.rule.start_sequence }} ≤ 事件序号 {{ evidence.rule.end_sequence ? `< ${evidence.rule.end_sequence}` : '（尚无结束边界）' }}</dd><dt>规则集摘要</dt><dd>{{ evidence.rule.ruleset_hash }}</dd></dl>
              <template v-if="!evidence.redacted && evidence.rule.package_status === 'verified' && evidence.rule.packages">
                <p class="hint">已核对该事件所属纪元的激活记录与安装内容，不是当前配置。包内规则不一定都是本事件的直接原因；单次叙述还可能使用已记录的风格覆盖。</p>
                <dl><dt>规则激活</dt><dd>{{ evidence.rule.packages.lock.activation_event_id }}</dd><dt>执行算法</dt><dd>{{ evidence.rule.packages.lock.algorithm }}</dd><dt>阶段 / 顺序</dt><dd>{{ evidence.rule.packages.lock.phase }} / {{ evidence.rule.packages.lock.conflict_order }}</dd></dl>
                <button type="button" class="clear" :disabled="busy" @click="event = evidence.rule.packages.lock.activation_event_id; inspect()">查看规则激活事件</button>
                <details v-for="kind in (['system', 'narrative'] as const)" :key="kind" class="payload rule-package"><summary>{{ kind === 'system' ? 'System 规则包' : 'Narrative 叙事包' }} · {{ evidence.rule.packages[kind].manifest.id }} @ {{ evidence.rule.packages[kind].manifest.version }}</summary><p class="hint">安装来源：{{ evidence.rule.packages.lock[kind].install_event_id }}</p><pre>{{ JSON.stringify(evidence.rule.packages[kind], null, 2) }}</pre></details>
              </template>
              <p v-else class="hint">{{ evidence.redacted ? '包内容不在诊断权限内。' : evidence.rule.package_status === 'preparation' ? '此事件属于世界准备纪元，尚未使用随后激活的插件包。' : '此纪元没有可核对的已安装包记录；不推测规则正文。' }}</p>
            </section>
            <section class="cause"><h3><span>03</span> 已记录的因果来源</h3><p v-if="evidence.causation_event_id">{{ evidence.causation_event_id }}</p><p v-else>{{ evidence.redacted ? '因果细节不在诊断权限内。' : evidence.causation_evidence === 'outside_scope' ? '因果引用位于当前授权范围之外。' : '事件未记录直接因果引用；这不代表没有原因。' }}</p></section>
            <StudioExplanation :token="token" :instance="evidence.instance_id" :branch="evidence.branch_id" :event="evidence.event_id" :access="evidence.access_level" @inspect-event="event = $event; inspect()" />
            <details v-if="!evidence.redacted" class="payload"><summary>查看原始事件内容</summary><p class="hint">原始记录可能包含角色陈述；陈述内容不自动等于事实。</p><pre>{{ payload }}</pre></details><p v-else class="redacted">事件内容已脱敏，不向 ops 展示人物或私人数据。</p>
          </article>
        </section>
      </div>
    </main>
  </div>
</template>

<style scoped>
.studio{min-height:100vh;background:var(--ink-050);color:var(--ink-900);font-family:var(--font-body)}.studio-top{max-width:1400px;margin:auto;display:flex;align-items:center;gap:24px;padding:22px 36px;border-bottom:1px solid var(--line)}a{color:var(--teal-400);text-decoration:none}.studio-top>a:last-child{margin-left:auto}.studio-brand{font-size:24px;font-weight:600;color:var(--ink-900)}.studio-brand span{color:var(--amber-400)}.studio-label,.access,.studio-title>p:first-child{font:12px var(--font-mono);letter-spacing:1.5px;color:var(--amber-400)}main{max-width:1280px;margin:auto;padding:42px 36px 70px}.studio-title{margin-bottom:38px}.studio-title h1{font-size:clamp(27px,3vw,42px);font-weight:500;letter-spacing:-1px;margin:12px 0}.studio-title>p:last-child,.hint,.snapshot,.empty p{color:var(--ink-700);line-height:1.7}.studio-grid{display:grid;grid-template-columns:300px minmax(0,1fr);gap:44px}h2{font-size:18px;font-weight:500}h2 span,h3 span{font:12px var(--font-mono);color:var(--amber-400);margin-right:12px}label{display:block;font-size:13px;margin-top:20px;color:var(--ink-800)}input{display:block;width:100%;box-sizing:border-box;margin-top:7px;padding:12px;background:var(--ink-100);border:1px solid var(--ink-500);color:var(--ink-900);font:14px var(--font-mono);border-radius:2px}input:focus-visible,button:focus-visible,a:focus-visible,summary:focus-visible{outline:2px solid var(--teal-400);outline-offset:4px}.hint{font-size:12px;margin:9px 0}.inspect,.clear{width:100%;min-height:44px;margin-top:20px;border:1px solid var(--amber-500);border-radius:2px;font:inherit;cursor:pointer}.inspect{background:var(--amber-400);color:var(--ink-000)}button:disabled{opacity:.5;cursor:wait}.clear{margin-top:10px;color:var(--ink-800);background:transparent;border-color:var(--line-strong)}.boundary{margin-top:26px;font-size:13px;color:var(--ink-700);line-height:1.7}summary{cursor:pointer;min-height:36px;line-height:36px}.studio-result{min-width:0;border-left:1px solid var(--line-strong);padding-left:34px}.empty{padding:60px 0}.axis-mark{font:34px var(--font-mono);color:var(--amber-400)}.empty h2{font-size:24px;margin-top:30px}.event-heading{display:flex;gap:22px;align-items:center;padding-bottom:22px;border-bottom:1px solid var(--line-amber)}.sequence{font:40px var(--font-mono);color:var(--amber-400)}.event-heading h2{font-size:24px;overflow-wrap:anywhere;margin:8px 0}.event-heading time{font:12px var(--font-mono);color:var(--ink-700)}.snapshot{font-size:12px;margin:18px 0}dl{display:grid;grid-template-columns:94px minmax(0,1fr);gap:14px 18px;font-size:13px;margin:24px 0}dt{color:var(--ink-700)}dd{margin:0;overflow-wrap:anywhere;font-family:var(--font-mono);line-height:1.6}.studio-rule,.cause{padding-top:18px;border-top:1px solid var(--line);margin-top:24px}h3{font-size:16px;font-weight:500}.cause p,.redacted{color:var(--ink-800);font-size:14px;line-height:1.8;overflow-wrap:anywhere}.payload{margin-top:28px;border-top:1px solid var(--line);padding-top:10px}.payload pre{white-space:pre-wrap;overflow-wrap:anywhere;font:12px/1.8 var(--font-mono);background:var(--ink-100);padding:18px;max-height:480px;overflow:auto}.error{border-left:3px solid var(--rose-400);padding:14px 18px;background:var(--ink-100);color:var(--rose-400);line-height:1.7}
@media(max-width:760px){.studio-top{padding:18px 20px;gap:12px}.studio-label{display:none}main{padding:28px 20px}.studio-title{margin-bottom:28px}.studio-grid{grid-template-columns:minmax(0,1fr);gap:30px}form{display:grid;grid-template-columns:1fr 1fr;gap:0 12px}form h2,form>label:first-of-type,form .hint,form>label:last-of-type{grid-column:1/-1}.boundary{margin-top:16px}.studio-result{border-left:0;border-top:2px solid var(--line-amber);padding:24px 0 0}.empty{padding:16px 0}.empty h2{margin-top:16px}.sequence{font-size:30px}.event-heading{gap:16px}.event-heading h2{font-size:20px}dl{grid-template-columns:72px minmax(0,1fr);gap:12px}.snapshot{line-height:1.6}}
</style>

<style scoped>
.rule-package { min-width: 0; overflow-wrap: anywhere; }
.rule-package summary { min-height: 44px; line-height: 1.7; padding-block: 8px; }
.scope-picker,.timeline-refresh{grid-column:1/-1}.studio-list{list-style:none;padding:0;margin:12px 0}.studio-list li{border-bottom:1px solid var(--line)}.studio-list button{display:block;width:100%;text-align:left;min-height:48px;padding:12px 4px;border:0;background:transparent;color:var(--ink-900);cursor:pointer;font:14px var(--font-body)}.studio-list button:hover{background:var(--ink-100)}.studio-list small{display:block;margin-top:5px;font:11px/1.6 var(--font-mono);color:var(--ink-700);overflow-wrap:anywhere}.studio-list span{overflow-wrap:anywhere}.studio-timeline{margin-bottom:24px;border-bottom:1px solid var(--line-amber);padding-bottom:20px}.studio-timeline>summary{color:var(--amber-400)}
</style>
