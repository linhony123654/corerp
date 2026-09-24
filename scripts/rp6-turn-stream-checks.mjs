import assert from 'node:assert/strict'
import { join } from 'node:path'

export async function checkTurnStream({ page, sql, temp, credential, restart, getModelCalls }) {
  let commands = 0
  const count = request => { if (request.url().endsWith('/rp/turns/run')) commands++ }
  page.on('request', count)
  let streamRequest
  await page.route('**/api/v1/rp/narrative/stream', async route => {
    streamRequest = route.request().postDataJSON()
    const actual = await route.fetch()
    assert.equal(actual.status(), 200)
    await route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: (await actual.text()).split('\n')[0] + '\n' })
  }, { times: 1 })
  await page.getByLabel('你想说的话').fill('主对话断流后继续。')
  await page.getByRole('button', { name: '说出' }).click()
  await page.getByRole('button', { name: '继续读取叙述', exact: true }).waitFor()
  assert.match(await page.getByRole('alert').innerText(), /行动已经提交/)
  const bookmark = await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')))
  assert.equal(bookmark.pending.path, 'turns/run')
  assert.ok(bookmark.pending.narrative_turn_id)
  assert.deepEqual(streamRequest, { session_id: bookmark.session, turn_run_id: bookmark.pending.narrative_turn_id }, 'primary stream must use pinned style, not current settings')
  assert.equal(commands, 1)
  const facts = () => sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_npc_decisions)||':'||(SELECT COUNT(*) FROM rp_utterances)")
  const committed = facts(), committedCalls = getModelCalls()
  assert.equal(sql(`SELECT status FROM rp_turn_runs WHERE turn_run_id='${bookmark.pending.narrative_turn_id.replaceAll("'", "''")}'`), 'settled')
  await restart()
  await page.reload()
  await page.getByLabel('玩家访问凭证').fill(credential)
  await page.getByRole('button', { name: '继续这段生活' }).click()
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  assert.equal(commands, 1, 'presentation recovery does not resend the committed command')
  assert.equal(facts(), committed)
  assert.equal(getModelCalls(), committedCalls)
  assert.match(await page.locator('.turn').last().innerText(), /主对话断流后继续/)
  assert.equal(await page.getByLabel('你想说的话').inputValue(), '')

  // Timing fixture over actual streamed facts proves the primary reading
  // region receives partial output before the done frame. No production delay.
  await page.evaluate(() => {
    const original = window.fetch.bind(window)
    window.fetch = async (...args) => {
      if (!String(args[0]).endsWith('/rp/narrative/stream')) return original(...args)
      window.fetch = original
      const real = await original(...args), text = await real.text(), at = text.indexOf('\n') + 1
      const gate = new Promise(resolve => { window.__rp6FinishTurnStream = resolve })
      const encoder = new TextEncoder()
      return new Response(new ReadableStream({ async start(controller) {
        controller.enqueue(encoder.encode(text.slice(0, at))); await gate
        controller.enqueue(encoder.encode(text.slice(at))); controller.close()
      } }), { status: real.status, headers: real.headers })
    }
  })
  await page.getByLabel('你想说的话').fill('正在逐段接收。')
  await page.getByRole('button', { name: '说出' }).click()
  const preview = page.getByRole('region', { name: '当前回合叙述流' })
  await preview.getByText(/正在逐段接收/).waitFor()
  assert.match(await preview.innerText(), /尚未读完/)
  assert.equal(await page.getByRole('button', { name: '等一小时' }).isDisabled(), true)
  assert.ok(await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).pending.narrative_turn_id))
  await page.screenshot({ path: join(temp, 'primary-stream-mobile.png') })
  await page.evaluate(() => { window.__rp6FinishTurnStream(); delete window.__rp6FinishTurnStream })
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  assert.equal(await preview.count(), 0)
  assert.match(await page.locator('.turn').last().innerText(), /正在逐段接收/)
  assert.equal(commands, 2)
  assert.ok(!JSON.stringify(await page.context().storageState()).includes(credential))
  page.off('request', count)
  console.log(JSON.stringify({ primaryStream: 'PASS', checks: ['settled-turn durable presentation boundary', 'truncated stream', 'restart retries only read', 'no duplicate world/decision/model work', 'pinned style', 'visible partial preview before done', 'clear pending only after complete stream and refreshed history'] }))
}
