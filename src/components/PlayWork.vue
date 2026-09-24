<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { formatMinorAmount } from '../lib/wallet'
type Job = { contract_id: string; employer_name: string; status: string; workplace_name: string; wage_minor: string; pay_period_days: number; currency_id: string; currency_scale: number; currency_symbol: string }
type Appointment = { schedule_id: string; world_time: string; original_world_time?: string; place_name: string; activity: string }
type Work = { jobs: Job[]; appointments: Appointment[]; more_appointments: boolean; world_time: string }
const props = defineProps<{ read: () => Promise<Work> }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const work = ref<Work | null>(null), error = ref(''), loading = ref(false)
let active = true
async function load() {
  if (loading.value) return
  loading.value = true; error.value = ''
  try {
    const result = await props.read()
    for (const job of result.jobs) formatMinorAmount(job.wage_minor, job.currency_scale)
    if (active) work.value = result
  } catch (cause) { if (active) error.value = cause instanceof Error ? cause.message : '暂时无法查看工作信息。' }
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
const statuses: Record<string, string> = { onboarding: '待入职', probation: '试用期', regular: '正式在职', active: '在职', contract_active: '合同有效', ended: '已结束', terminated: '已终止', resigned: '已离职', laid_off: '已离职' }
const activities: Record<string, string> = { work: '工作', home: '回家', present: '到场' }
onMounted(() => { dialog.value?.showModal(); void load() })
onBeforeUnmount(() => { active = false; dialog.value?.close() })
</script>

<template>
  <dialog ref="dialog" class="work-sheet" aria-labelledby="work-title" @keydown="containTab" @close="emit('close')">
    <header><div><p class="eyebrow">随身 · 工作与日程</p><h2 id="work-title">工作信息</h2></div><button autofocus aria-label="关闭工作信息" @click="dialog?.close()">收起 ×</button></header>
    <p class="intro">合同里的约定，和接下来的安排。</p>
    <p v-if="error" role="alert" class="error">{{ error }}{{ work ? '。以下仍是上次读取的记录。' : '' }}</p>
    <p v-if="loading" role="status">正在查看工作记录…</p>
    <template v-if="work">
      <section aria-label="本人合同"><h3>工作约定</h3><p v-if="!work.jobs.length" class="hint">没有查到当前有效的工作合同。</p>
        <article v-for="job in work.jobs" :key="job.contract_id" class="job"><div class="job-heading"><h4>{{ job.employer_name || '工资合同 · 单位名称未登记' }}</h4><span>{{ statuses[job.status] || '状态未标注' }}</span></div>
          <p>约定报酬 <strong>{{ formatMinorAmount(job.wage_minor, job.currency_scale) }} {{ job.currency_symbol || job.currency_id }}</strong></p>
          <p class="hint">{{ job.pay_period_days ? `计薪周期：${job.pay_period_days} 天` : '计薪周期未登记，请以合同结算为准。' }}</p>
          <p v-if="job.workplace_name" class="hint">工作地点：{{ job.workplace_name }}</p>
        </article>
        <p class="hint">约定报酬不是已到账收入；实际结算和钱包余额以已发生的记录为准。</p>
      </section>
      <section aria-label="本人近期日程"><h3>近期日程</h3><p v-if="!work.appointments.length" class="hint">当前没有待执行的日程。</p>
        <ol><li v-for="entry in work.appointments" :key="entry.schedule_id"><time>{{ time(entry.world_time) }}</time><p>{{ activities[entry.activity] || '已安排活动' }} · {{ entry.place_name }}</p><p v-if="entry.original_world_time" class="hint">已顺延，原定 {{ time(entry.original_world_time) }}</p></li></ol>
        <p v-if="work.more_appointments" class="hint">先列出最近 20 项；时间推进后可重新查看后续安排。</p>
      </section>
    </template>
    <footer><p>{{ work ? `查询于 ${time(work.world_time)} · 世界时间` : '仅查看自己的合同和日程。' }}</p><button :disabled="loading" @click="load">重新查看工作</button></footer>
  </dialog>
</template>

<style scoped>
.work-sheet { margin: auto; width: min(620px,calc(100% - 32px)); max-height: calc(100dvh - 40px); overflow: auto; padding: 28px; border: 1px solid #7e493550; border-top: 4px solid var(--accent); background: var(--paper); color: var(--text); }.work-sheet::backdrop { background: #18251dc0; }header,.job-heading { display: flex; align-items: start; justify-content: space-between; gap: 16px; }h2 { font: 400 30px/1.4 var(--font-serif); color: var(--text); margin: 0; }.eyebrow { font-size: 11px; letter-spacing: 2px; color: var(--accent); margin: 0 0 10px; }.intro { font: 15px/1.8 var(--font-serif); margin-block: 20px; }h3 { font: 400 18px var(--font-serif); color: var(--text); padding-top: 20px; border-top: 1px solid #39493535; }h4 { font-size: 15px; font-weight: 400; color: var(--text); margin: 0; }.job { padding: 16px 0; }.job p { font-size: 14px; line-height: 1.8; }.job-heading span { font-size: 12px; color: var(--accent); white-space: nowrap; }.hint,footer,.job .hint { font-size: 12px; line-height: 1.8; color: var(--muted); }strong { font-weight: 400; font-variant-numeric: tabular-nums; }ol { list-style: none; padding: 0; }li { padding-block: 12px; border-bottom: 1px solid #39493525; }li time { font-size: 12px; color: var(--muted); }li p { margin: 8px 0 0; }section,footer { overflow-wrap: anywhere; }footer { display: flex; align-items: center; flex-wrap: wrap; justify-content: space-between; gap: 10px; margin-top: 20px; }.error { color: #99372f; font-size: 13px; overflow-wrap: anywhere; }button { min-height: 44px; background: transparent; border: 0; color: inherit; font: inherit; padding: 10px 12px; cursor: pointer; }button:hover:not(:disabled) { background: #35433112; }button:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }button:disabled { opacity: .5; }
@media(max-width:600px) { .work-sheet { margin: auto 0 0; width: 100%; max-width: 100%; padding: 24px 24px max(24px,env(safe-area-inset-bottom)); border-inline: 0; border-bottom: 0; }.job-heading { flex-wrap: wrap; gap: 8px; } }
</style>
