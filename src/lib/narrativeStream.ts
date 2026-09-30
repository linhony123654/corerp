export type NarrativeStreamResult = { view: { lines: string[]; event_ids: string[]; warnings: string[]; fallback_reason?: string; render_id?: string } }

// A completed HTTP response is not enough: every segment must cite committed
// sources and the final marker must agree with their complete reference set.
export async function readNarrativeStream(response: Response, preview: (lines: string[]) => void): Promise<NarrativeStreamResult> {
  if (!response.ok) {
    const body = await response.json()
    throw new Error(body.error?.message || `叙述读取失败 (${response.status})`)
  }
  if (!response.headers.get('Content-Type')?.startsWith('application/x-ndjson') || !response.body) throw new Error('服务没有返回可读取的叙述流。')
  const reader = response.body.getReader()
  const decoder = new TextDecoder('utf-8', { fatal: true })
  const lines: string[] = [], referenced: string[] = []
  let warnings: string[] = [], eventIDs: string[] = [], fallbackReason = '', renderID = '', buffer = '', bytes = 0, done = false
  function validRefs(value: unknown): value is string[] {
    return Array.isArray(value) && value.length > 0 && value.length <= 1024
      && value.every((id: unknown) => typeof id === 'string' && id.length > 0 && id.length <= 256)
  }
  function frame(raw: string) {
    if (done) throw new Error('叙述结束标记之后还有内容。')
    const value = JSON.parse(raw)
    if (value.type === 'error') throw new Error(typeof value.message === 'string' ? value.message : '叙述传输中断。')
    if (value.type === 'line') {
      const c = value.chunk
      const hasSingle = typeof c?.event_id === 'string' && c.event_id.length > 0 && c.event_id.length <= 256
      const hasSet = validRefs(c?.event_ids) && new Set(c.event_ids).size === c.event_ids.length
      if (!c || c.index !== lines.length || typeof c.line !== 'string' || lines.length >= 1024 || hasSingle === hasSet) throw new Error('叙述片段顺序或来源无效。')
      const refs = hasSingle ? [c.event_id as string] : c.event_ids as string[]
      for (const id of refs) if (!referenced.includes(id)) referenced.push(id)
      lines.push(c.line); preview([...lines])
    } else if (value.type === 'done') {
      if (value.count !== lines.length || !lines.length || !Array.isArray(value.warnings) || value.warnings.some((w: unknown) => typeof w !== 'string')
        || !validRefs(value.event_ids) || new Set(value.event_ids).size !== value.event_ids.length
        || value.event_ids.length !== referenced.length || value.event_ids.some((id: string, i: number) => id !== referenced[i])
        || (value.fallback_reason !== undefined && (typeof value.fallback_reason !== 'string' || value.fallback_reason.length > 100))
        || (value.render_id !== undefined && (typeof value.render_id !== 'string' || !/^rpr_[a-zA-Z0-9_-]{1,120}$/.test(value.render_id)))) {
        throw new Error('叙述结束标记与已收到的来源或内容不符。')
      }
      warnings = value.warnings; eventIDs = value.event_ids; fallbackReason = value.fallback_reason || ''; renderID = value.render_id || ''; done = true
    } else throw new Error('无法识别叙述片段。')
  }
  try {
    while (true) {
      const chunk = await reader.read()
      if (chunk.done) { buffer += decoder.decode(); break }
      bytes += chunk.value.byteLength
      if (bytes > 2 * 1024 * 1024) throw new Error('本次叙述超过传输上限。')
      buffer += decoder.decode(chunk.value, { stream: true })
      let newline: number
      while ((newline = buffer.indexOf('\n')) >= 0) { frame(buffer.slice(0, newline)); buffer = buffer.slice(newline + 1) }
    }
    if (buffer || !done) throw new Error('叙述传输未完成，原文已保留，请重试。')
    return { view: { lines, event_ids: eventIDs, warnings, ...(fallbackReason ? { fallback_reason: fallbackReason } : {}), ...(renderID ? { render_id: renderID } : {}) } }
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock() }
}
