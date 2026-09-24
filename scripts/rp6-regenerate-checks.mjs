import assert from 'node:assert/strict'
import { join } from 'node:path'
import { checkStreamParser } from './rp6-stream-parser-checks.mjs'

export async function checkRegenerate({ page, sql, temp, credential, getModelCalls }) {
  await checkStreamParser(page)
  const session = await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).session)
  const facts = () => sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_npc_decisions)||':'||(SELECT COUNT(*) FROM rp_utterances)||':'||(SELECT group_concat(narrative_json) FROM rp_turn_runs)")
  const before = facts()
  const initialModelCalls = getModelCalls()
  const controls = page.locator('.turn').filter({ has: page.locator('.narrative-tools') })
  assert.ok(await controls.count() > 0)
  const waits = page.locator('.turn').filter({ hasText: '你等待至 ' })
  assert.ok(await waits.count() > 0)
  assert.equal(await waits.locator('.narrative-tools').count(), 0, 'event-only history has no unsupported render button')
  const turn = controls.last()
  const original = await turn.locator(':scope > p').allTextContents()
  const setting = await page.evaluate(async ({ session, credential }) => {
    const call = async (path, body) => {
      const response = await fetch(`/api/v1/rp/${path}`, { method: 'POST', headers: { Authorization: `Bearer ${credential}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      return { status: response.status, body: await response.json() }
    }
    const read = await call('style/read', { session_id: session })
    return call('style/set', { instance_id: 'inst_m2_t09', branch_id: 'br_main', session_id: session, scope: 'session', expected_revision: read.body.data.session_revision, idempotency_key: 'rp6-regenerate-style', patch: { pov: 'third_person', verbosity: 'detailed', description_density: 80 } })
  }, { session, credential })
  assert.equal(setting.status, 200)
  await turn.locator('summary').click()
  let failedRequest
  await page.route('**/api/v1/rp/narrative/stream', async route => {
    failedRequest = route.request().postDataJSON()
    const actual = await route.fetch()
    const first = (await actual.text()).split('\n')[0]
    // Keep an actual attributed first frame but drop the completion marker.
    await route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: first + '\n' })
  }, { times: 1 })
  await turn.getByRole('button', { name: '按当前设置重新生成' }).click()
  await turn.getByRole('alert').waitFor()
  assert.deepEqual(await turn.locator(':scope > p').allTextContents(), original)
  const retried = page.waitForRequest(r => r.url().endsWith('/rp/narrative/stream'))
  await turn.getByRole('button', { name: '重试叙述读取' }).click()
  assert.deepEqual((await retried).postDataJSON(), failedRequest, 'retry pins same turn and presentation patch')
  await turn.getByText('展示已更新，世界事件和原始记录未改变。', { exact: true }).waitFor()
  assert.match((await turn.locator(':scope > p').allTextContents()).join('\n'), /Lin说/)
  assert.equal(facts(), before, 'render cannot mutate decision/world/original narrative')
  await page.evaluate(() => window.scrollTo({ top: document.documentElement.scrollHeight }))
  assert.equal(await turn.getByRole('button', { name: '恢复原叙述' }).evaluate(el => el.getBoundingClientRect().bottom <= document.querySelector('.composer').getBoundingClientRect().top), true, 'regenerate controls reachable above composer')
  await page.screenshot({ path: join(temp, 'regenerate-mobile.png') })
  await turn.getByRole('button', { name: '恢复原叙述' }).click()
  assert.deepEqual(await turn.locator(':scope > p').allTextContents(), original)
  // Explicit timing fixture over a REAL server response, only to make the
  // transient preview observable. Production never inserts artificial delays.
  await page.evaluate(() => {
    const originalFetch = window.fetch.bind(window)
    window.fetch = async (...args) => {
      if (!String(args[0]).endsWith('/rp/narrative/stream')) return originalFetch(...args)
      window.fetch = originalFetch
      const real = await originalFetch(...args)
      const body = await real.text(), split = body.indexOf('\n') + 1
      const gate = new Promise(resolve => { window.__rp6ReleaseStream = resolve })
      const encode = new TextEncoder()
      return new Response(new ReadableStream({ async start(controller) {
        controller.enqueue(encode.encode(body.slice(0, split)))
        await gate
        controller.enqueue(encode.encode(body.slice(split))); controller.close()
      } }), { status: real.status, headers: real.headers })
    }
  })
  await turn.getByRole('button', { name: '按当前设置重新生成' }).click()
  await turn.getByRole('region', { name: '叙述流临时预览' }).waitFor()
  assert.deepEqual(await turn.locator(':scope > p').allTextContents(), original, 'partial preview must not replace complete original')
  await page.evaluate(() => window.scrollTo({ top: document.documentElement.scrollHeight }))
  await page.screenshot({ path: join(temp, 'stream-preview-mobile.png') })
  await page.evaluate(() => { window.__rp6ReleaseStream(); delete window.__rp6ReleaseStream })
  await turn.getByText('展示已更新，世界事件和原始记录未改变。', { exact: true }).waitFor()
  await page.reload()
  await page.getByLabel('玩家访问凭证').fill(credential)
  await page.getByRole('button', { name: '继续这段生活' }).click()
  await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor()
  assert.deepEqual(await controls.last().locator(':scope > p').allTextContents(), original, 'reload restores original persistent narrative')
  assert.equal(facts(), before)
  assert.equal(getModelCalls(), initialModelCalls, 'regeneration never calls the decision model')
  assert.ok(!JSON.stringify(await page.context().storageState()).includes(credential))
  await page.evaluate(() => window.scrollTo({ top: document.documentElement.scrollHeight }))
  console.log(JSON.stringify({ regenerateScenario: 'PASS', checks: ['server-declared eligibility', 'read failure preserves current text', 'retry pins same presentation request', 'actual third-person variant', 'restore original and reload', 'world/decision/transcript invariance', 'no credential persistence'] }))
}
