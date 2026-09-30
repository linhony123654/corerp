import assert from 'node:assert/strict'
import { join } from 'node:path'
import { clickPlayWait, openPlayBusiness, scrollPlayToEnd } from './play-ui-helpers.mjs'

export async function checkMap({ page, sql, temp, stage, blocked = false }) {
  const facts = () => sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main')||':'||(SELECT place_id FROM agent_positions WHERE agent_id='entity_m2_rp_lin')")
  const before = facts()
  const bookmark = await page.evaluate(() => localStorage.getItem('corerp.play.v1'))
  const response = page.waitForResponse(r => r.url().endsWith('/rp/observe'))
  const opener = page.getByRole('button', { name: '行动与功能', exact: true })
  const dialog = page.getByRole('dialog', { name: '附近地图', exact: true })
  await openPlayBusiness(page, '地图')
  const { data } = await (await response).json()
  await dialog.getByRole('heading', { name: data.place_name, exact: true }).waitFor()
  const adjacent = sql(`SELECT to_place_id FROM rp_place_links WHERE instance_id='inst_m2_t09' AND branch_id='br_main' AND from_place_id='${data.place_id.replaceAll("'", "''")}' ORDER BY to_place_id`).split('\n').filter(Boolean)
  assert.deepEqual(data.reachable_places.map(p => p.place_id), adjacent)
  assert.equal(await dialog.locator('li').count(), adjacent.length)
  for (const place of data.reachable_places) {
    assert.equal(await dialog.getByRole('button', { name: `前往 ${place.display_name}`, exact: true }).isDisabled(), !place.can_move_now)
  }
  if (blocked) {
    assert.equal(data.reachable_places.find(p => p.place_id === 'place_m2_work_ada').can_move_now, false)
    assert.match(await dialog.innerText(), /当前无法通行/)
    assert.match(await dialog.innerText(), /相邻路段施工至/)
  }
  assert.doesNotMatch(await dialog.innerText(), /entity_|event_|scheduler|observation_cursor/)
  await dialog.getByRole('button', { name: '关闭地图' }).focus()
  await page.keyboard.press('Shift+Tab')
  assert.equal(await dialog.evaluate(el => el.contains(document.activeElement)), true)
  await dialog.evaluate(el => { el.scrollTop = 0 })
  assert.equal(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth), true)
  await page.screenshot({ path: join(temp, `map-${stage}-mobile.png`) })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.screenshot({ path: join(temp, `map-${stage}-desktop.png`) })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/api/v1/rp/observe', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { message: '测试地图读取暂不可用' } }) }), { times: 1 })
  await dialog.getByRole('button', { name: '重新查看地图' }).click()
  await dialog.getByRole('alert').waitFor()
  assert.match(await dialog.getByRole('alert').innerText(), /上次读取/)
  for (const place of data.reachable_places) assert.equal(await dialog.getByRole('button', { name: `前往 ${place.display_name}`, exact: true }).isDisabled(), true)
  await dialog.getByRole('button', { name: '重新查看地图' }).click()
  await dialog.getByRole('alert').waitFor({ state: 'hidden' })
  await dialog.getByRole('status').waitFor({ state: 'hidden' })
  await page.keyboard.press('Escape')
  await dialog.waitFor({ state: 'hidden' })
  assert.equal(await opener.evaluate(el => document.activeElement === el), true)
  assert.equal(facts(), before)
  assert.equal(await page.evaluate(() => localStorage.getItem('corerp.play.v1')), bookmark)
  await scrollPlayToEnd(page)
  console.log(JSON.stringify({ map: 'PASS', stage, routes: adjacent.length, checks: ['actual adjacency and movement availability', 'no internals in diagram', 'keyboard/Escape', 'failed refresh disables old departures', 'no world/time/position/bookmark change'] }))
}

export async function checkMapWorks({ page, sql, temp, creatorCredential, credential, restart }) {
  const destinationName = sql("SELECT display_name FROM agent_places WHERE place_id='place_m2_work_ada'")
  const scope = { instance_id: 'inst_m2_t09', branch_id: 'br_main' }
  const now = sql("SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main'")
  const at = hours => new Date(Date.parse(now) + hours * 3600000).toISOString().replace('.000Z', 'Z')
  const response = await fetch('http://127.0.0.1:8080/api/v1/opportunities/transit/define', { method: 'POST', headers: { Authorization: `Bearer ${creatorCredential}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ binding: { ...scope, expected_head: Number(sql("SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main'")), idempotency_key: 'map-local-works' }, from_place_id: 'place_m2_cafe', to_place_id: 'place_m2_work_ada', starts_at: at(1), ends_at: at(2) }) })
  assert.equal(response.status, 200, await response.text())
  await page.getByRole('button', { name: '环顾四周' }).click()
  await clickPlayWait(page, 1)
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  await checkMap({ page, sql, temp, stage: 'blocked', blocked: true })
  await clickPlayWait(page, 1)
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  // Lose the committed move response. Recovery must retain the exact map cursor
  // and idempotency key and cannot move a second time after process restart.
  await openPlayBusiness(page, '地图')
  const dialog = page.getByRole('dialog', { name: '附近地图', exact: true })
  await dialog.getByRole('button', { name: `前往 ${destinationName}`, exact: true }).waitFor()
  let sent
  await page.route('**/api/v1/rp/actions/move', async route => { sent = route.request().postDataJSON(); const moved = await route.fetch(); assert.equal(moved.status(), 200); await route.abort('failed') }, { times: 1 })
  await dialog.getByRole('button', { name: `前往 ${destinationName}`, exact: true }).click()
  await page.getByRole('button', { name: '继续未完成的行动', exact: true }).waitFor()
  const count = sql("SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'")
  await restart()
  await page.reload()
  await page.getByLabel('玩家访问凭证').fill(credential)
  const replay = page.waitForRequest(r => r.url().endsWith('/rp/actions/move'))
  await page.getByRole('button', { name: '继续这段生活' }).click()
  assert.deepEqual((await replay).postDataJSON(), sent)
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  await page.getByRole('heading', { name: destinationName, exact: true }).waitFor()
  assert.equal(sql("SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'"), count)
  await checkMap({ page, sql, temp, stage: 'moved-restarted' })
  console.log(JSON.stringify({ mapWorks: 'PASS', checks: ['actual sourced roadworks disables route', 'expiry restores travel', 'map move commits through original owner', 'lost response/process+page restart retries exact key with no duplicate move'] }))
}
