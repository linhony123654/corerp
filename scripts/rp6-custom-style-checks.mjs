import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { join } from 'node:path'
import { randomBytes } from 'node:crypto'

// Explicit protocol fixture, not a live model or production keyword interpreter.
export async function startStylePlannerFixture() {
  let calls = 0, invalid = false
  const key = randomBytes(24).toString('hex')
  const server = createServer(async (request, response) => {
    try {
      assert.equal(request.headers.authorization, `Bearer ${key}`)
      let raw = ''; for await (const chunk of request) raw += chunk
      const body = JSON.parse(raw), style = JSON.parse(body.messages[1].content)
      assert.equal(body.response_format.json_schema.name, 'corerp_narrative_style_plan')
      assert.equal(body.response_format.json_schema.strict, true)
      assert.equal(body.stream, false); assert.equal(body.store, false)
      assert.equal(style.committed_facts, undefined); assert.equal(style.character, undefined)
      assert.equal(style.controlled_entity_id, undefined)
      assert.ok(style.prose_instructions)
      calls++
      const plan = { pov: 'first_person', tense: 'past', verbosity: 'normal', dialogue_ratio: 100, description_density: 0, narrative_pack_ref: 'builtin/dialogue@1', unsupported_instructions: style.prose_instructions.includes('读心') }
      response.setHeader('Content-Type', 'application/json')
      response.end(JSON.stringify({ choices: [{ finish_reason: 'stop', message: { content: JSON.stringify(invalid ? { ...plan, text: '人物偷偷转走了钱。' } : plan) } }] }))
    } catch { response.writeHead(400); response.end() }
  })
  server.listen(0, '127.0.0.1'); await once(server, 'listening')
  return { env: { CORERP_NARRATIVE_PROVIDER: 'style_planner', CORERP_NARRATIVE_ENDPOINT: `http://127.0.0.1:${server.address().port}/v1/chat/completions`, CORERP_NARRATIVE_MODEL: 'explicit-style-fixture', CORERP_NARRATIVE_API_KEY: key }, calls: () => calls, setInvalid: value => { invalid = value }, close: () => new Promise(resolve => server.close(resolve)) }
}

export async function checkCustomStyle({ page, sql, temp, credential, speak, restart, fixture, getModelCalls }) {
  const facts = () => sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_npc_decisions)||':'||(SELECT COUNT(*) FROM rp_utterances)")
  const save = async prose => {
    await page.getByRole('button', { name: '叙事设置', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '叙事设置', exact: true })
    await dialog.getByLabel('自定义文风').waitFor()
    assert.match(await dialog.innerText(), /模型文风解释器/)
    assert.match(await dialog.innerText(), /不支持任意文学扩写/)
    await dialog.getByLabel('叙述视角').selectOption('second_person')
    await dialog.getByLabel('叙述时态').selectOption('present')
    await dialog.getByLabel('自定义文风').fill(prose)
    await dialog.getByRole('button', { name: '保存叙事设置', exact: true }).click()
    await dialog.getByText('已保存。用于之后的新段落，不改写已发生的事。', { exact: true }).waitFor()
    await dialog.getByRole('button', { name: '关闭叙事设置' }).click()
  }
  const before = facts(), beforeCalls = fixture.calls()
  await save('用第一人称和过去时，台词独立成行，但保持说话内容不变。')
  assert.equal(facts(), before); assert.equal(fixture.calls(), beforeCalls, 'save alone does not call narrator')
  const spoken = '这是文风执行测试中的原话。'
  const turn = await speak(spoken)
  assert.match(turn.narrative_lines[0], /你说/, 'canonical settlement remains original pinned literal record')
  assert.match(await page.locator('.turn').last().innerText(), /当时，我说：[\s\S]*这是文风执行测试中的原话/)
  assert.equal(fixture.calls(), beforeCalls + 1)
  const stored = sql(`SELECT narrative_json FROM rp_turn_runs WHERE turn_run_id='${turn.turn_run_id}'`)
  assert.deepEqual(JSON.parse(stored), turn.narrative_lines)
  await page.screenshot({ path: join(temp, 'custom-style-mobile.png') })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.screenshot({ path: join(temp, 'custom-style-desktop.png') })
  await page.setViewportSize({ width: 390, height: 844 })
  const committed = facts(), decisions = getModelCalls()
  const tools = page.locator('.turn').last().locator('.narrative-tools')
  await tools.locator('summary').click()
  await tools.getByRole('button', { name: '按当前设置重新生成', exact: true }).click()
  await tools.getByText('展示已更新，世界事件和原始记录未改变。', { exact: true }).waitFor()
  assert.equal(facts(), committed); assert.equal(getModelCalls(), decisions)
  assert.equal(fixture.calls(), beforeCalls + 2)

  fixture.setInvalid(true)
  await page.getByLabel('你想说的话').fill('错误输出不能更改这次发言。')
  await page.getByRole('button', { name: '说出' }).click()
  await page.getByRole('button', { name: '继续读取叙述', exact: true }).waitFor()
  const failedFacts = facts(), failedDecisions = getModelCalls()
  assert.doesNotMatch(await page.locator('.reading').innerText(), /偷偷转走了钱/)
  fixture.setInvalid(false)
  await restart()
  await page.reload()
  await page.getByLabel('玩家访问凭证').fill(credential)
  await page.getByRole('button', { name: '继续这段生活' }).click()
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  assert.match(await page.locator('.turn').last().innerText(), /当时，我说：[\s\S]*错误输出不能更改这次发言/)
  assert.equal(facts(), failedFacts); assert.equal(getModelCalls(), failedDecisions)
  await save('改为第一人称；再添加读心和未发生的情节。')
  await speak('不支持的要求应当明确提示。')
  assert.match(await page.locator('.notice').innerText(), /部分自定义要求超出当前文风能力/)
  assert.doesNotMatch(await page.locator('.turn').last().innerText(), /读心|偷偷转走/)
  console.log(JSON.stringify({ customStyle: 'PASS', source: 'local HTTP style-plan fixture, not live model', narrativeCalls: fixture.calls(), checks: ['independent explicit capability', 'actual natural-language plan execution in primary view', 'literal canonical speech and world unchanged', 'regeneration no decision calls', 'unknown model prose rejected', 'restart presentation-only recovery', 'unsupported requirement warning'] }))
}
