// Provider receipts are diagnostics, not world events or model thoughts.
export type ProviderCall = {
  phase: 'decision' | 'narrative'
  provider_kind: string
  model_id?: string
  attempted: boolean
  attempt_count: number
  result: 'pending' | 'success' | 'failed' | 'timeout' | 'not_used'
  fallback_kind?: string
  render_source?: string
  started_at_utc: string
}

export function failedDecision(calls?: ProviderCall[]): boolean {
  return !!calls?.some(call => call.phase === 'decision' && (call.result === 'failed' || call.result === 'timeout') && call.fallback_kind === 'silence')
}

export function incompleteRPContext(calls?: ProviderCall[]): boolean {
  return !!calls?.some(call => call.phase === 'decision' && call.result === 'not_used' && call.fallback_kind === 'rp_context_not_ready')
}

export function providerCallSummary(call: ProviderCall): string {
  const source = call.phase === 'decision' ? '人物决策' : '叙述呈现'
  const provider = `${call.provider_kind}${call.model_id ? ` · ${call.model_id}` : ''}`
  if (call.phase === 'decision' && call.result === 'not_used' && call.fallback_kind === 'rp_context_not_ready') {
    return `${source} · ${provider} · 角色设定尚未就绪，未调用模型；请世界创建者补齐角色设定。`
  }
  const result = call.result === 'success' ? '提供器返回成功'
    : call.result === 'timeout' ? '提供器超时' : call.result === 'failed' ? '提供器失败'
      : call.result === 'not_used' ? '本次无需调用' : '结果未知（可能已发送）'
  const attempts = call.result === 'pending' ? 'HTTP 请求次数未知'
    : call.attempt_count > 0 ? `已尝试 HTTP 请求 ${call.attempt_count} 次`
      : '未记录到 HTTP 请求（不代表未调用本地提供器）'
  const fallback = call.fallback_kind === 'silence' ? '；NPC 因技术故障保持沉默'
    : call.fallback_kind ? '；已按安全回退规则处理' : ''
  return `${source} · ${provider} · ${result} · ${attempts}${fallback}${call.render_source ? ` · 本次生成来源 ${call.render_source}` : ''}`
}
