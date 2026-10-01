export type NarrativeCompositionMetadata = {
  composition_version?: 'corerp.fact-composition.v1' | 'corerp.fact-composition.v2'
  fact_groups?: string[][]
}
export type NarrativeView = NarrativeCompositionMetadata & { lines: string[]; event_ids: string[]; warnings: string[]; fallback_reason?: string; render_id?: string }
export type NarrativeStreamResult = { view: NarrativeView }

function validRefs(value: unknown): value is string[] {
  return Array.isArray(value) && value.length > 0 && value.length <= 1024
    && value.every((id: unknown) => typeof id === 'string' && id.length > 0 && id.length <= 256)
}

// Both streamed and saved versioned views bind one group to each line. The
// independent event list must be covered exactly once, in its original order.
// Legacy prose can repeat sources across lines and keeps its previous behavior.
export function validateNarrativeComposition(view: { lines: string[]; event_ids?: string[]; composition_version?: string; fact_groups?: string[][] }): void {
  if (view.composition_version === undefined && view.fact_groups === undefined) return
  if (view.composition_version === 'corerp.fact-composition.v2' && Array.isArray(view.lines) && view.lines.length === 0
    && Array.isArray(view.event_ids) && view.event_ids.length === 0 && Array.isArray(view.fact_groups) && view.fact_groups.length === 0) return
  if (!['corerp.fact-composition.v1', 'corerp.fact-composition.v2'].includes(view.composition_version || '')
    || !Array.isArray(view.lines) || !view.lines.length || view.lines.length > 1024 || view.lines.some(line => typeof line !== 'string')
    || !Array.isArray(view.fact_groups) || view.fact_groups.length !== view.lines.length || !view.fact_groups.every(validRefs)) throw new Error('叙述版本或逐段来源无效，原文已保留。')
  const ordered = view.fact_groups.flat()
  // Older saved v1 history did not expose the independent list. Keep it
  // readable with structural validation; v2 must supply independent coverage.
  const eventIDs = view.event_ids === undefined ? (view.composition_version === 'corerp.fact-composition.v1' ? ordered : undefined) : view.event_ids
  if (!validRefs(eventIDs) || new Set(ordered).size !== ordered.length || ordered.length !== eventIDs.length
    || ordered.some((id, i) => id !== eventIDs[i])) throw new Error('叙述来源重复、缺失或顺序不符，原文已保留。')
}

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
  const lines: string[] = [], referenced: string[] = [], chunkGroups: string[][] = []
  let warnings: string[] = [], eventIDs: string[] = [], fallbackReason = '', renderID = '', buffer = '', bytes = 0, done = false
  let composition: NarrativeCompositionMetadata = {}
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
      chunkGroups.push(refs)
      for (const id of refs) if (!referenced.includes(id)) referenced.push(id)
      lines.push(c.line); preview([...lines])
    } else if (value.type === 'done') {
      const emptyV2 = value.composition_version === 'corerp.fact-composition.v2' && lines.length === 0
        && Array.isArray(value.event_ids) && value.event_ids.length === 0 && Array.isArray(value.fact_groups) && value.fact_groups.length === 0
      if (value.count !== lines.length || (!lines.length && !emptyV2) || !Array.isArray(value.warnings) || value.warnings.some((w: unknown) => typeof w !== 'string')
        || (!validRefs(value.event_ids) && !emptyV2) || new Set(value.event_ids).size !== value.event_ids.length
        || value.event_ids.length !== referenced.length || value.event_ids.some((id: string, i: number) => id !== referenced[i])
        || (value.fallback_reason !== undefined && (typeof value.fallback_reason !== 'string' || value.fallback_reason.length > 100))
        || (value.render_id !== undefined && (typeof value.render_id !== 'string' || !/^rpr_[a-zA-Z0-9_-]{1,120}$/.test(value.render_id)))) {
        throw new Error('叙述结束标记与已收到的来源或内容不符。')
      }
      validateNarrativeComposition({ lines, event_ids: value.event_ids, composition_version: value.composition_version, fact_groups: value.fact_groups })
      if (value.composition_version !== undefined) {
        if (value.fact_groups.some((group: string[], i: number) => group.length !== chunkGroups[i].length || group.some((id, j) => id !== chunkGroups[i][j]))) throw new Error('叙述结束标记与片段分组不符。')
        composition = { composition_version: value.composition_version, fact_groups: value.fact_groups }
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
    return { view: { lines, event_ids: eventIDs, warnings, ...composition, ...(fallbackReason ? { fallback_reason: fallbackReason } : {}), ...(renderID ? { render_id: renderID } : {}) } }
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock() }
}
