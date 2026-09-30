import assert from 'node:assert/strict'

export async function checkStreamParser(page) {
  const result = await page.evaluate(async () => {
    const { readNarrativeStream } = await import('/src/lib/narrativeStream.ts')
    const encode = new TextEncoder()
    const line = JSON.stringify({ type: 'line', chunk: { index: 0, event_id: 'fixture-event', line: '你好\n继续。' } }) + '\n'
    const done = JSON.stringify({ type: 'done', count: 1, event_ids: ['fixture-event'], warnings: [] }) + '\n'
    let release, sawPreview
    const gate = new Promise(resolve => { release = resolve })
    const previewed = new Promise(resolve => { sawPreview = resolve })
    const source = new ReadableStream({ async start(controller) {
      // Deliberate byte-by-byte segmentation crosses Chinese UTF-8 boundaries.
      for (const byte of encode.encode(line)) controller.enqueue(new Uint8Array([byte]))
      await gate
      controller.enqueue(encode.encode(done)); controller.close()
    } })
    let completed = false
    const reading = readNarrativeStream(new Response(source, { headers: { 'Content-Type': 'application/x-ndjson' } }), lines => sawPreview(lines)).then(value => { completed = true; return value })
    const preview = await previewed
    const beforeCompletion = !completed
    release()
    const complete = await reading
    const prose = JSON.stringify({ type: 'line', chunk: { index: 0, event_ids: ['fact-1', 'fact-2'], line: '第一段。' } }) + '\n'
      + JSON.stringify({ type: 'line', chunk: { index: 1, event_ids: ['fact-1', 'fact-2'], line: '第二段。' } }) + '\n'
    const proseDone = JSON.stringify({ type: 'done', count: 2, event_ids: ['fact-1', 'fact-2'], warnings: [] }) + '\n'
    const synthetic = await readNarrativeStream(new Response(prose + proseDone, { headers: { 'Content-Type': 'application/x-ndjson' } }), () => {})
    const invalid = [line, line + done.replace('"count":1', '"count":2'), line.replace('"index":0', '"index":1') + done, line + '{"type":"error","message":"fixture interruption"}\n', line + done + line, line + done.trimEnd(),
      prose + done, prose + proseDone.replace('"fact-2"', '"other-fact"'),
      JSON.stringify({ type: 'line', chunk: { index: 0, event_ids: ['fact-1', 'fact-1'], line: '无效。' } }) + '\n' + proseDone,
      JSON.stringify({ type: 'line', chunk: { index: 0, event_id: 'fact-1', event_ids: ['fact-2'], line: '无效。' } }) + '\n' + proseDone]
    let rejected = 0
    for (const text of invalid) {
      try { await readNarrativeStream(new Response(text, { headers: { 'Content-Type': 'application/x-ndjson' } }), () => {}) }
      catch { rejected++ }
    }
    return { preview, beforeCompletion, complete, synthetic, rejected }
  })
  assert.equal(result.beforeCompletion, true, 'preview must arrive before stream ends')
  assert.deepEqual(result.preview, ['你好\n继续。'])
  assert.deepEqual(result.complete.view, { lines: ['你好\n继续。'], event_ids: ['fixture-event'], warnings: [] })
  assert.deepEqual(result.synthetic.view, { lines: ['第一段。', '第二段。'], event_ids: ['fact-1', 'fact-2'], warnings: [] })
  assert.equal(result.rejected, 10)
  console.log(JSON.stringify({ streamParser: 'PASS', source: 'explicit transport fixture', checks: ['byte-split UTF-8', 'preview before completion', 'multi-source prose', 'truncation/order/count/error/trailing-frame/evidence mismatch rejection'] }))
}
