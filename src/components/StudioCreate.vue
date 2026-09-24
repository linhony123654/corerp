<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { buildCreateRequest, freshStudioDraft } from '../lib/studioCreate'
import type { CreateReceipt, CreateRequest } from '../lib/studioCreate'

const draft = reactive(freshStudioDraft())
const token = ref(''), systemJSON = ref(''), narrativeJSON = ref('')
const consent = ref(false), busy = ref(false), error = ref('')
const request = ref<CreateRequest | null>(null), receipt = ref<CreateReceipt | null>(null)
const recovery = ref<{ key: string; name: string }[]>([]), recoveryChoice = ref('')
const prefix = 'corerp.studio.create.request.', currentKey = 'corerp.studio.create.current'
let controller: AbortController | null = null, revision = 0
const frozen = computed(() => busy.value || !!request.value)
const shownName = computed(() => String(request.value?.spec.name || draft.name))

function readSaved(key: string): CreateRequest {
  if (!key.startsWith(prefix)) throw new Error('恢复记录标识无效。')
  const raw = localStorage.getItem(key)
  if (!raw || new TextEncoder().encode(raw).length > 1048576) throw new Error('恢复记录缺失或过大。')
  const saved = JSON.parse(raw) as CreateRequest
  if (!saved || typeof saved.instance_id !== 'string' || typeof saved.idempotency_key !== 'string' || !saved.spec || typeof saved.spec.name !== 'string' || typeof saved.player_principal_id !== 'string' || key !== prefix + saved.idempotency_key) throw new Error('恢复记录格式不匹配。')
  return saved
}
function listSaved() {
  const items: { key: string; name: string }[] = []
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i)!
    if (!key.startsWith(prefix)) continue
    try { const saved = readSaved(key); items.push({ key, name: `${saved.spec.name} · ${saved.instance_id}` }) } catch { /* Unrelated/corrupt data is never submitted. */ }
  }
  recovery.value = items
}
try {
  listSaved()
  const key = localStorage.getItem(currentKey)
  if (key) { request.value = readSaved(key); recoveryChoice.value = key }
} catch { error.value = '本地恢复记录不可用。请检查浏览器存储设置；不会自动提交任何内容。' }

function clearCredential() {
  revision++; controller?.abort(); controller = null
  token.value = ''; busy.value = false; receipt.value = null
}
watch(token, () => { revision++; controller?.abort(); controller = null; busy.value = false; receipt.value = null; error.value = '' }, { flush: 'sync' })
window.addEventListener('pagehide', clearCredential)
onBeforeUnmount(() => { window.removeEventListener('pagehide', clearCredential); clearCredential() })
function restore() {
  if (busy.value) return
  try {
    const saved = readSaved(recoveryChoice.value)
    localStorage.setItem(currentKey, recoveryChoice.value)
    request.value = saved; receipt.value = null; error.value = ''
  } catch { error.value = '无法读取这份恢复记录，原记录未删除。' }
}
function newDraft() {
  if (busy.value) return
  try {
    localStorage.removeItem(currentKey)
    request.value = null; receipt.value = null; recoveryChoice.value = ''; consent.value = false; error.value = ''
    Object.assign(draft, freshStudioDraft()); systemJSON.value = ''; narrativeJSON.value = ''
  } catch { error.value = '无法保存草稿选择，原恢复记录未删除。' }
}
async function submit() {
  if (busy.value || !token.value.trim() || (!request.value && !consent.value)) return
  const version = ++revision
  busy.value = true; error.value = ''; receipt.value = null
  let submitted = false
  try {
    if (!request.value) {
      const prepared = await buildCreateRequest({ ...draft }, systemJSON.value, narrativeJSON.value)
      if (version !== revision) return
      const key = prefix + prepared.idempotency_key
      // Configuration only; the credential is never part of the persisted body.
      try { localStorage.setItem(key, JSON.stringify(prepared)); localStorage.setItem(currentKey, key) }
      catch { throw new Error('无法持久保存恢复请求，尚未发送。请允许本地存储后重试。') }
      request.value = prepared; recoveryChoice.value = key; listSaved()
    }
    const body = request.value!
    controller = new AbortController()
    submitted = true
    const response = await fetch('/api/v1/studio/worlds/create', {
      method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token.value}` },
      credentials: 'omit', redirect: 'error', cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(45000)]), body: JSON.stringify(body)
    })
    if (version !== revision) return
    if (!response.ok) {
      const messages: Record<number, string> = { 400: '配置或插件包未通过校验。原请求已保留；可另建草稿修正，不能修改原重试请求。', 401: '创建凭证无效。请重新输入具有创建权限的凭证。', 403: '缺少此范围的创建权限，或玩家身份不可用。请核对管理员提供的授权。', 409: '请求与已有记录冲突。请保留原请求并检查世界事件，不要用同一标识替换配置。' }
      error.value = messages[response.status] || '保存暂未确认。原请求已保留，请重试同一请求。'
      return
    }
    const data = (await response.json()).data as CreateReceipt
    if (version !== revision) return
    if (!data || data.status !== 'ready' || data.instance_id !== body.instance_id || data.branch_id !== 'br_main' || data.player_principal_id !== body.player_principal_id || typeof data.entity_id !== 'string' || !data.entity_id || typeof data.ready_event_id !== 'string' || !data.ready_event_id || !Number.isSafeInteger(data.event_sequence) || data.event_sequence < 1) {
      error.value = '保存回执与请求不匹配，尚不能确认可进入。请重试原请求。'; return
    }
    receipt.value = data
  } catch (cause) {
    if (version === revision) error.value = submitted ? '连接中断或响应异常，保存结果尚未确认。请重试原请求；不会重复创建世界。' : cause instanceof Error ? cause.message : '配置准备失败，尚未发送。'
  } finally { if (version === revision) { busy.value = false; controller = null } }
}
</script>

<template>
  <div class="creator">
    <header class="creator-top"><a href="/" @click="clearCredential">Core<span>RP</span></a><span>STUDIO / 创作</span><nav aria-label="Studio 导航"><a href="/studio" @click="clearCredential">事件检查</a><a href="/" @click="clearCredential">回到 Play ↗</a></nav></header>
    <main>
      <section class="creator-intro"><p class="creator-kicker">WORLD GENESIS / 01</p><h1>先有世界，<br>再让生活发生。</h1><p>定义起点，安装规则与叙事。保存后，玩家用自己的凭证进入。</p></section>
      <div class="creator-layout">
        <form @submit.prevent="submit">
          <fieldset class="creator-authority">
            <legend>01 / 创建身份</legend>
            <label>创建者访问凭证<input v-model="token" type="password" autocomplete="off" required></label>
            <p class="creator-hint">只在本页内存使用。创建权限须由本地管理员开通，不等同于事件检查权限。</p>
            <template v-if="!request"><div class="creator-pair"><label>授权来源世界<input v-model="draft.authorityInstance" :disabled="frozen" placeholder="管理员提供的世界实例标识" required></label><label>授权来源分支<input v-model="draft.authorityBranch" :disabled="frozen" required></label></div>
            <label>玩家身份标识<input v-model="draft.playerPrincipal" :disabled="frozen" placeholder="已存在的 player principal，不是访问凭证" required></label></template>
            <p v-else class="creator-hint">正在核对「{{ shownName }}」的原请求。配置已冻结，授权范围、玩家与完整内容见创建摘要。</p>
          </fieldset>
          <fieldset v-if="!request" :disabled="frozen">
            <legend>02 / 世界起点</legend>
            <label>世界名称<input v-model="draft.name" required maxlength="100"></label>
            <div class="creator-pair"><label>玩家角色名称<input v-model="draft.playerName" required maxlength="100"></label><label>邻居名称<input v-model="draft.neighbourName" required maxlength="100"></label><label>起始居所<input v-model="draft.home" required maxlength="100"></label><label>公共地点<input v-model="draft.square" required maxlength="100"></label></div>
            <p class="creator-hint">两名人物从居所开始生活，两处地点互相可达。当前移动立即抵达，不模拟路程耗时。起始日期固定为 2026-09-22。</p>
            <details><summary>高级资源设置</summary><div class="creator-pair"><label>总人口<input v-model.number="draft.population" type="number" min="2" max="1000000" step="1" required></label><label>初始货币总量<input v-model.number="draft.money" type="number" min="0" max="9007199254740991" step="1" required></label><label>初始物资总量<input v-model.number="draft.stock" type="number" min="0" max="9007199254740991" step="1" required></label></div><p class="creator-hint">货币与物资以最小单位计。人物从守恒人口与资源池中物化，未分配部分仍留在人口群体中。</p></details>
          </fieldset>
          <fieldset v-if="!request" :disabled="frozen">
            <legend>03 / 规则与叙事包</legend>
            <div class="creator-pair"><label>NPC 每日主动行动预算<input v-model.number="draft.budget" type="number" min="1" max="64" step="1" required></label><label><span id="creator-style-label">叙事包风格</span><select v-model="draft.narration" aria-labelledby="creator-style-label"><option value="plain">朴素叙述</option><option value="dialogue">简短对话</option><option value="detailed">较详细描写</option></select></label></div>
            <p class="creator-hint">默认生成声明式 System / Narrative 包，保存时安装并锁定版本与内容。预算是主动行动的上限，不保证行动发生；叙事不改变世界事实。</p>
            <details><summary>安装自定义包 JSON</summary><p class="creator-hint">分别粘贴完整 manifest + content，替代上方对应默认包。单包最多 64 KiB；依赖、内容哈希与能力由后端验证。仅支持声明式 system / narrative，不执行代码、SQL 或远程插件。</p><label>System 包 JSON<textarea v-model="systemJSON" rows="7" spellcheck="false" placeholder="留空使用上述预算生成的包"></textarea></label><label>Narrative 包 JSON<textarea v-model="narrativeJSON" rows="7" spellcheck="false" placeholder="留空使用上述风格生成的包"></textarea></label></details>
          </fieldset>
          <section class="creator-submit" aria-label="保存世界">
            <label v-if="!request" class="creator-consent"><input v-model="consent" type="checkbox" required :disabled="busy">同意在此浏览器保存配置与身份标识，以便中断后恢复；不保存访问凭证。</label>
            <p v-else class="creator-hint">重试将提交完全相同的配置与请求标识。另建草稿不会删除原恢复记录。</p>
            <button class="creator-primary" :disabled="busy || !token.trim() || (!request && !consent)">{{ busy ? '正在保存与核对…' : request ? '重试 / 核对原保存请求' : '保存世界与安装包' }}</button>
            <button type="button" @click="clearCredential">清除创建凭证</button>
            <p v-if="error" class="creator-error" role="alert">{{ error }}</p>
          </section>
        </form>
        <aside class="creator-summary" aria-label="创建摘要">
          <p class="creator-kicker">{{ receipt ? 'SAVED / 已确认回执' : request ? 'RECOVERY / 已保留请求' : 'DRAFT / 尚未提交' }}</p>
          <h2>{{ shownName }}</h2>
          <template v-if="request"><dl><dt>目标实例</dt><dd>{{ request.instance_id }}</dd><dt>授权范围</dt><dd>{{ request.authority_instance_id }} / {{ request.authority_branch_id }}</dd><dt>控制玩家</dt><dd>{{ request.player_principal_id }}</dd></dl><details><summary>查看冻结请求 JSON</summary><pre>{{ JSON.stringify(request, null, 2) }}</pre></details></template>
          <p v-else>此刻只是起点的声明。保存会依次准备人口与资源、安装包、锁定规则，并绑定玩家。</p>
          <section v-if="receipt" class="creator-ready" role="status"><h3>世界已保存</h3><p>已收到准备完成事件回执。玩家进入时会重新检查当前权限。</p><dl><dt>完成事件 · #{{ receipt.event_sequence }}</dt><dd>{{ receipt.ready_event_id }}</dd></dl><a href="/?choose_world=1" @click="clearCredential">使用独立玩家凭证进入 Play ↗</a></section>
          <template v-if="recovery.length"><h3>本地恢复请求</h3><label><span id="creator-recovery-label">选择保存请求</span><select v-model="recoveryChoice" :disabled="busy" aria-labelledby="creator-recovery-label"><option value="" disabled>选择一份原请求</option><option v-for="item in recovery" :key="item.key" :value="item.key">{{ item.name }}</option></select></label><button type="button" :disabled="busy || !recoveryChoice" @click="restore">载入恢复请求</button><p class="creator-hint">本地记录不代表已保存成功。重新输入创建凭证，核对原请求后再进入 Play。</p></template>
          <button v-if="request" type="button" :disabled="busy" @click="newDraft">另建草稿（保留原请求）</button>
        </aside>
      </div>
    </main>
  </div>
</template>

<style scoped>
.creator { min-height: 100dvh; background: var(--ink-050); color: var(--ink-800); }
.creator-top { min-height: 80px; display: flex; align-items: center; gap: 28px; border-bottom: 1px solid var(--line); padding: 16px max(24px, calc((100vw - 1200px) / 2)); }
.creator-top > a { font: 24px var(--font-display); color: var(--ink-900); border: 0; }.creator-top > a span { color: var(--amber-400); }.creator-top > span { font: 11px var(--font-mono); letter-spacing: 2px; }.creator-top nav { display: flex; gap: 24px; margin-left: auto; font-size: 13px; }
.creator main { max-width: 1200px; padding: 52px 24px 80px; margin: auto; }.creator-intro { padding: 0 0 36px; border-bottom: 1px solid var(--line-strong); }.creator-kicker { font: 11px/1.8 var(--font-mono); letter-spacing: 2px; color: var(--amber-400); }.creator h1 { font-size: clamp(32px, 4vw, 52px); line-height: 1.3; margin: 18px 0; font-weight: 400; }.creator-intro > p:last-child { max-width: 600px; color: var(--ink-700); }
.creator-layout { display: grid; grid-template-columns: minmax(0, 1fr) 310px; gap: 56px; align-items: start; }.creator form { min-width: 0; }.creator fieldset { min-width: 0; padding: 28px 0 32px; margin: 30px 0 0; border: 0; border-bottom: 1px solid var(--line); }.creator legend { font: 13px var(--font-mono); color: var(--amber-400); letter-spacing: 1px; }.creator label { display: grid; gap: 8px; margin: 0 0 18px; font-size: 13px; }.creator input, .creator select, .creator textarea { width: 100%; min-width: 0; min-height: 44px; border: 1px solid var(--ink-500); background: var(--ink-100); color: var(--ink-900); padding: 11px 12px; font: inherit; border-radius: 0; }.creator textarea { resize: vertical; font: 12px/1.8 var(--font-mono); }.creator input::placeholder, .creator textarea::placeholder { color: var(--ink-700); }.creator-pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 18px; }.creator-hint { color: var(--ink-700); font-size: 12px; line-height: 1.9; }.creator details { margin: 20px 0 0; }.creator summary { min-height: 44px; cursor: pointer; color: var(--teal-400); font-size: 13px; }.creator :is(button, input, select, textarea, summary, a):focus-visible { outline: 2px solid var(--teal-400); outline-offset: 4px; }.creator button { min-height: 44px; padding: 10px 14px; border: 1px solid var(--line-strong); background: transparent; color: var(--ink-900); font: inherit; margin: 0 8px 12px 0; }.creator button:hover:not(:disabled) { border-color: var(--amber-400); }.creator :disabled { opacity: .55; cursor: default; }.creator-submit { padding: 28px 0; }.creator button.creator-primary { background: var(--amber-400); color: var(--ink-000); border-color: var(--amber-400); }.creator .creator-consent { grid-template-columns: 20px 1fr; align-items: start; }.creator-consent input { min-height: 20px; height: 20px; padding: 0; accent-color: var(--amber-400); }.creator-error { color: var(--rose-400); overflow-wrap: anywhere; }
.creator-summary { margin-top: 32px; padding-left: 28px; border-left: 1px solid var(--line-amber); min-width: 0; overflow-wrap: anywhere; }.creator-summary h2 { font-size: 28px; font-weight: 400; margin: 18px 0; }.creator-summary h3 { font-size: 15px; margin: 24px 0 16px; }.creator-summary p { font-size: 13px; line-height: 1.9; }.creator-summary dt { font-size: 11px; color: var(--ink-700); margin-top: 20px; }.creator-summary dd { margin: 6px 0 0; font: 12px/1.8 var(--font-mono); }.creator-summary pre { white-space: pre-wrap; overflow-wrap: anywhere; font: 11px/1.7 var(--font-mono); max-height: 360px; overflow: auto; padding: 12px; background: var(--ink-100); }.creator-ready { border-top: 1px solid var(--line); margin: 28px 0; }.creator-ready a { display: inline-block; min-height: 44px; padding: 10px 0; color: var(--teal-400); }
@media (max-width: 800px) { .creator-layout { grid-template-columns: 1fr; gap: 0; }.creator-summary { margin-top: 8px; }.creator-top { flex-wrap: wrap; gap: 12px 20px; }.creator-top nav { gap: 16px; }.creator main { padding-top: 32px; } }
@media (max-width: 480px) { .creator-pair { grid-template-columns: 1fr; }.creator-top > span { display: none; }.creator-top nav { font-size: 12px; }.creator-summary { padding-left: 18px; } }
</style>
