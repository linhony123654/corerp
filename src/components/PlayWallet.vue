<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { formatMinorAmount } from '../lib/wallet'
import type { WalletSnapshot } from '../lib/wallet'

const props = defineProps<{ owner: string; read: () => Promise<WalletSnapshot> }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const wallet = ref<WalletSnapshot | null>(null)
const loading = ref(false)
const error = ref('')
let mounted = true
const amount = computed(() => wallet.value ? formatMinorAmount(wallet.value.balance_minor, wallet.value.currency_scale) : '')
const snapshotTime = computed(() => wallet.value?.world_time.replace('T', ' ').replace(/Z$/, '') || '')

function containTab(event: KeyboardEvent) {
  if (event.key !== 'Tab') return
  const buttons = dialog.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')
  if (!buttons?.length) return
  const first = buttons[0], last = buttons[buttons.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}

async function refresh() {
  if (loading.value) return
  loading.value = true; error.value = ''; wallet.value = null
  try {
    const result = await props.read()
    // Validate before displaying, including exact minor-unit formatting.
    formatMinorAmount(result.balance_minor, result.currency_scale)
    if (mounted) wallet.value = result
  } catch (cause) {
    if (mounted) error.value = cause instanceof Error ? cause.message : '暂时无法读取余额。'
  } finally {
    if (mounted) loading.value = false
  }
}
onMounted(() => { dialog.value?.showModal(); void refresh() })
onBeforeUnmount(() => { mounted = false; dialog.value?.close() })
</script>

<template>
  <dialog ref="dialog" class="wallet-sheet" aria-labelledby="wallet-title" aria-describedby="wallet-owner" @close="emit('close')" @keydown="containTab">
    <header class="wallet-heading">
      <div><p class="wallet-eyebrow">随身 · 日常收支</p><h2 id="wallet-title">钱包</h2></div>
      <button type="button" autofocus aria-label="关闭钱包" @click="dialog?.close()">收起 ×</button>
    </header>
    <p id="wallet-owner" class="wallet-owner">{{ owner }} 的随身账户</p>
    <section class="wallet-content" :aria-busy="loading" aria-label="账户余额">
      <p v-if="loading" class="wallet-state" role="status">正在查看余额…</p>
      <div v-else-if="error" class="wallet-state">
        <p class="wallet-error" role="alert">暂时无法查看：{{ error }}</p>
        <p>余额未更新。重试不会花钱，也不会推进时间。</p>
        <button type="button" class="wallet-retry" @click="refresh">重新查看</button>
      </div>
      <template v-else-if="wallet">
        <p class="wallet-caption">当前余额</p>
        <p class="wallet-amount" data-testid="wallet-amount">{{ amount }} <small>{{ wallet.currency_symbol || wallet.currency_id }}</small></p>
        <p class="wallet-currency">币种：{{ wallet.currency_id }}</p>
        <p v-if="wallet.balance_minor === '0'" class="wallet-zero">账户余额为零。</p>
        <p class="wallet-asof">查看于 <time>{{ snapshotTime }}</time> · 世界时间</p>
      </template>
    </section>
    <footer class="wallet-footer"><p>只查看，不改变这段生活。</p><button type="button" :disabled="loading" @click="refresh">刷新余额 ↻</button></footer>
  </dialog>
</template>

<style scoped>
.wallet-sheet { margin: auto; width: min(520px, calc(100% - 32px)); max-height: calc(100dvh - 48px); overflow: auto; padding: 28px; border: 1px solid #7e493550; border-top: 4px solid var(--accent); background: var(--paper); color: var(--text); box-shadow: 0 20px 70px #17281b30; }
.wallet-sheet::backdrop { background: #18251dc0; }
.wallet-sheet button { min-height: 44px; padding: 10px 12px; border: 0; border-radius: 2px; color: inherit; background: transparent; font: inherit; cursor: pointer; }.wallet-sheet button:hover:not(:disabled) { background: #35433112; }.wallet-sheet button:disabled { opacity: .48; cursor: default; }.wallet-sheet button:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; }
.wallet-heading { display: flex; justify-content: space-between; align-items: flex-start; gap: 20px; }
.wallet-eyebrow { color: var(--accent); letter-spacing: 2px; font-size: 11px; margin: 0 0 10px; }
.wallet-heading h2 { color: var(--text); font: 400 30px/1.4 var(--font-serif); margin: 0; }
.wallet-owner { font-size: 13px; color: var(--muted); margin: 14px 0 26px; overflow-wrap: anywhere; }
.wallet-content { border-block: 1px solid #39493535; padding: 22px 0; min-height: 170px; }
.wallet-caption, .wallet-currency, .wallet-asof, .wallet-zero { font-size: 12px; line-height: 1.8; color: var(--muted); }
.wallet-caption { margin: 0; }.wallet-amount { font: 400 clamp(26px, 7vw, 42px)/1.5 var(--font-serif); font-variant-numeric: tabular-nums; overflow-wrap: anywhere; margin: 14px 0 8px; }.wallet-amount small { font: 14px var(--font-body); }
.wallet-currency { overflow-wrap: anywhere; }.wallet-asof { margin: 24px 0 0; }
.wallet-state { font-size: 14px; line-height: 1.9; overflow-wrap: anywhere; }.wallet-error { color: #99372f; }.wallet-retry { text-decoration: underline; }
.wallet-footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 4px; margin-top: 14px; font-size: 12px; }.wallet-footer p { color: var(--muted); }
@media (max-width: 600px) { .wallet-sheet { margin: auto 0 0; width: 100%; max-width: 100%; max-height: calc(100dvh - 24px); padding: 24px 24px max(24px, env(safe-area-inset-bottom)); border-inline: 0; border-bottom: 0; } }
</style>
