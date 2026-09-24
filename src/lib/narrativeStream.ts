export type NarrativeStreamResult = { view: { lines: string[]; event_ids: string[]; warnings: string[] } }

// A completed HTTP response is not enough: the attributed sequence must end in
// an explicit done marker. Never promote a truncated preview to a saved view.
export async function readNarrativeStream(response: Response, preview: (lines: string[]) => void): Promise<NarrativeStreamResult> {
  if (!response.ok) {
    const body = await response.json()
    throw new Error(body.error?.message || `叙述读取失败 (${response.status})`)
  }
  if (!response.headers.get('Content-Type')?.startsWith('application/x-ndjson') || !response.body) throw new Error('服务没有返回可读取的叙述流。')
  const reader = response.body.getReader()
  const decoder = new TextDecoder('utf-8', { fatal: true })
  const lines: string[] = [], events: string[] = []
  let warnings: string[] = [], buffer = '', bytes = 0, done = false
  function frame(raw: string) {
    if (done) throw new Error('叙述结束标记之后还有内容。')
    const value = JSON.parse(raw)
    if (value.type === 'error') throw new Error(typeof value.message === 'string' ? value.message : '叙述传输中断。')
    if (value.type === 'line') {
      const c = value.chunk
      if (!c || c.index !== lines.length || typeof c.line !== 'string' || typeof c.event_id !== 'string' || !c.event_id || lines.length >= 1024) throw new Error('叙述片段顺序或来源无效。')
      lines.push(c.line); events.push(c.event_id); preview([...lines])
    } else if (value.type === 'done') {
      if (value.count !== lines.length || !lines.length || !Array.isArray(value.warnings) || value.warnings.some((w: unknown) => typeof w !== 'string')) throw new Error('叙述结束标记与已收到的内容不符。')
      warnings = value.warnings; done = true
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
    return { view: { lines, event_ids: events, warnings } }
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock() }
}
