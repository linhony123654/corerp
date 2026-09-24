import assert from 'node:assert/strict'
import { join } from 'node:path'

export async function checkMessages({ page, sql, temp, stage, expectedCount = 0 }) {
  const facts = () => sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM agent_knowledge)||':'||(SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main')")
  const before = facts()
  const bookmark = await page.evaluate(() => localStorage.getItem('corerp.play.v1'))
  const response = page.waitForResponse(r => r.url().endsWith('/rp/messages/read'))
  const opener = page.getByRole('button', { name: '手机', exact: true })
  const dialog = page.getByRole('dialog', { name: '手机消息', exact: true })
  await opener.click()
  const { data } = await (await response).json()
  assert.equal(data.messages.length, expectedCount)
  for (const message of data.messages) {
    assert.deepEqual(Object.keys(message).sort(), ['body', 'kind', 'message_id', 'sequence', 'title', 'world_time'])
    assert.equal(sql(`SELECT json_extract(payload,'$.candidate_id') FROM events WHERE event_id='${message.message_id.replaceAll("'", "''")}'`), 'entity_m2_rp_lin')
    await dialog.getByRole('heading', { name: message.title, exact: true }).waitFor()
    assert.ok((await dialog.innerText()).includes(message.body))
  }
  if (!expectedCount) await dialog.getByText('目前没有与你有关的事务通知。', { exact: true }).waitFor()
  await dialog.getByRole('status').waitFor({ state: 'hidden' })
  await dialog.getByRole('button', { name: '关闭消息' }).focus()
  await page.keyboard.press('Shift+Tab')
  assert.equal(await dialog.evaluate(el => el.contains(document.activeElement)), true)
  await dialog.evaluate(el => { el.scrollTop = 0 })
  assert.equal(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth), true)
  await page.screenshot({ path: join(temp, `messages-${stage}-mobile.png`) })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.screenshot({ path: join(temp, `messages-${stage}-desktop.png`) })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/api/v1/rp/messages/read', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { message: '测试消息读取暂不可用' } }) }), { times: 1 })
  await dialog.getByRole('button', { name: '重新查看消息' }).click()
  await dialog.getByRole('alert').waitFor()
  assert.match(await dialog.getByRole('alert').innerText(), /上次读取/)
  assert.equal(await dialog.locator('li').count(), expectedCount)
  await dialog.getByRole('button', { name: '重新查看消息' }).click()
  await dialog.getByRole('alert').waitFor({ state: 'hidden' })
  await dialog.getByRole('status').waitFor({ state: 'hidden' })
  await page.keyboard.press('Escape')
  await dialog.waitFor({ state: 'hidden' })
  assert.equal(await opener.evaluate(el => document.activeElement === el), true)
  assert.equal(facts(), before)
  assert.equal(await page.evaluate(() => localStorage.getItem('corerp.play.v1')), bookmark)
  await page.evaluate(() => window.scrollTo({ top: document.documentElement.scrollHeight }))
  console.log(JSON.stringify({ messages: 'PASS', stage, count: expectedCount, checks: ['actual own candidate messages', 'minimal DTO', 'empty/content', 'keyboard/Escape', 'error/retry retains labeled snapshot', 'no world/knowledge/bookmark mutation'] }))
  return data
}

export async function prepareMessageInvitations({ sql, credential, creatorCredential, managerCredential }) {
  const call = async (path, token, body) => {
    const binding = { instance_id: 'inst_m2_t09', branch_id: 'br_main', expected_head: Number(sql("SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main'")), idempotency_key: `messages-${path}` }
    const response = await fetch(`http://127.0.0.1:8080/api/v1/career/${path}`, { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ binding, ...body }) })
    const result = await response.json()
    assert.equal(response.status, 200, JSON.stringify(result))
    return result.data
  }
  await call('organizations/define', creatorCredential, { organization: { organization_id: 'actor_m2_coop_employer', display_name: '街区合作社', manager_principal_id: 'principal_m2_agent_bo', workplace_id: 'place_m2_work_ada' } })
  await call('positions/post', managerCredential, { posting: { position_id: 'messages-position', organization_id: 'actor_m2_coop_employer', title: '运营助理', occupation_id: 'operations', grade: 'junior', capacity: 1, daily_wage_minor: 12, required_qualifications: [] } })
  await call('applications/submit', credential, { application_id: 'messages-application', position_id: 'messages-position', candidate_id: 'entity_m2_rp_lin', statement: '我希望了解这份工作。' })
  await call('interviews/invite', managerCredential, { interview_id: 'messages-interview', application_id: 'messages-application', question: '请介绍你处理日常事务的经验。' })
  await call('interviews/answer', credential, { interview_id: 'messages-interview', answer: '我会先核对记录，再逐项处理。' })
}
