import { chromium } from 'playwright'
import { spawn, execFileSync } from 'node:child_process'
import { mkdtemp } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { randomBytes } from 'node:crypto'
import assert from 'node:assert/strict'
import { once } from 'node:events'
import { createServer } from 'node:http'

// Real world/service/browser. --fake-model adds only a local model HTTP fixture;
// it verifies the adapter, not live model quality. Never inherit live model credentials.
const fakeModel = process.argv.includes('--fake-model')
const lifeScenario = process.argv.includes('--life')
const root = resolve(import.meta.dirname, '..')
const temp = await mkdtemp(join(tmpdir(), 'corerp-rp1-e2e-'))
const database = join(temp, 'world.db')
const credential = randomBytes(24).toString('hex')
const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [credential]: 'principal_m2_rp_player' }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex') }
Object.assign(env, { CORERP_DECISION_PROVIDER: 'deterministic', CORERP_LLM_ENDPOINT: '', CORERP_LLM_MODEL: '', CORERP_LLM_API_KEY: '', CORERP_LLM_TIMEOUT: '', CORERP_LLM_ATTEMPTS: '' })
const sql = query => execFileSync('sqlite3', [database, query], { encoding: 'utf8' }).trim()
const run = (command, args, cwd = root) => execFileSync(command, args, { cwd, stdio: 'pipe' })
run('/usr/local/go/bin/go', ['build', '-o', join(temp, 'server'), './cmd/corerp-server'], join(root, 'backend'))
run('/usr/local/go/bin/go', ['build', '-o', join(temp, 'setup'), './cmd/corerp-m2'], join(root, 'backend'))
run(join(temp, 'setup'), ['-db', database, '-action', 'rp-travel-prepare'])
let server, vite, browser, modelServer
let modelCalls = 0
const startServer = () => spawn(join(temp, 'server'), ['-db', database, '-listen', '127.0.0.1:8080'], { env, stdio: 'ignore' })
async function ready(url) {
  for (let i = 0; i < 100; i++) {
    try { await fetch(url); return } catch { await new Promise(r => setTimeout(r, 100)) }
  }
  throw new Error(`Service did not start: ${url}`)
}
async function stop(child) { if (child && child.exitCode === null) { const exited = once(child, 'exit'); child.kill('SIGTERM'); await exited } }
try {
  if (fakeModel) {
    modelServer = createServer(async (request, response) => {
      try {
        let raw = ''
        for await (const chunk of request) raw += chunk
        const body = JSON.parse(raw)
        assert.equal(body.response_format.json_schema.strict, true)
        const input = JSON.parse(body.messages[1].content).character
        modelCalls++
        let action = 'respond', text = '来自测试模型的问候。'
        if (input.player_speech_text.includes('借') || input.own_asset_minor < 100) { action = 'refuse'; text = '测试模型：抱歉，我现在无法答应。' }
        else if (input.next_schedule?.activity_code === 'work') { action = 'refuse'; text = '测试模型：我得先去工作，晚些再聊。' }
        response.setHeader('Content-Type', 'application/json')
        response.end(JSON.stringify({ choices: [{ finish_reason: 'stop', message: { content: JSON.stringify({ action, text, destination_place_id: '' }) } }] }))
      } catch { response.writeHead(400); response.end() }
    })
    modelServer.listen(0, '127.0.0.1'); await once(modelServer, 'listening')
    Object.assign(env, { CORERP_DECISION_PROVIDER: 'chat_completions', CORERP_LLM_ENDPOINT: `http://127.0.0.1:${modelServer.address().port}/v1/chat/completions`, CORERP_LLM_MODEL: 'test-http-model' })
  }
  server = startServer()
  vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4178', '--strictPort'], { cwd: root, stdio: 'ignore' })
  await Promise.all([ready('http://127.0.0.1:8080/api/v1/rp/observe'), ready('http://127.0.0.1:4178')])
  browser = await chromium.launch({ headless: true })
  let context = await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' })
  let page = await context.newPage()
  const errors = []
  page.on('pageerror', e => errors.push(e.message))
  await page.goto('http://127.0.0.1:4178')
  await page.screenshot({ path: join(temp, 'arrival-mobile.png'), fullPage: true })
  await page.getByLabel('玩家访问凭证').fill(credential)
  await page.getByRole('button', { name: '进入世界' }).click()
  await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor()
  assert.match(await page.locator('.mode').innerText(), fakeModel ? /AI 人物/ : /确定性人物/)
  assert.match(await page.locator('.presence').innerText(), /Cai/)
  const speak = async text => {
    const response = page.waitForResponse(r => r.url().endsWith('/rp/turns/run'))
    await page.getByLabel('你想说的话').fill(text)
    await page.getByRole('button', { name: '说出' }).click()
    const result = await (await response).json()
    assert.ok(result.data, JSON.stringify(result))
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
    return result.data
  }
  await speak('能借我一点钱吗？')
  assert.match(await page.locator('.reading').innerText(), /Cai 拒绝了/)
  const hearingCount = Number(sql("SELECT COUNT(*) FROM observation_records WHERE json_extract(claim_payload, '$.claim_type') = 'speaker_said'"))
  assert.ok(hearingCount >= 2, 'speech hearing persisted')
  await page.getByRole('button', { name: '去别处' }).click()
  await page.getByRole('button', { name: 'Ada Home' }).click()
  await page.getByRole('heading', { name: 'Ada Home' }).waitFor()
  assert.doesNotMatch(await page.locator('.presence').innerText(), /Cai/)
  await speak('你好，一起聊聊吧。')
  assert.match(await page.locator('.turn').last().innerText(), /先去工作/)
  assert.doesNotMatch(await page.locator('.turn').last().innerText(), /Cai/)
  await page.getByRole('button', { name: '等四小时' }).click()
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  assert.match(await page.locator('.turn').last().innerText(), /等待至/)
  // Cross the actual work/lunch schedule, not only an empty clock interval.
  for (let i = 0; sql("SELECT current_world_time FROM world_clocks WHERE instance_id = 'inst_m2_t09' AND branch_id = 'br_main'") < '2026-09-23T12:00:00Z'; i++) {
    assert.ok(i < 12, 'wait progression bounded')
    await page.getByRole('button', { name: '等四小时' }).click()
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  }
  await page.getByRole('button', { name: '去别处' }).click()
  await page.getByRole('button', { name: 'M2 Cafe' }).click()
  await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor()
  assert.match(await page.locator('.presence').innerText(), /Ada/)
  assert.match(await page.locator('.presence').innerText(), /Bo/)

  // Real server commits; browser deliberately loses ONLY the response.
  await page.route('**/api/v1/rp/turns/run', async route => {
    const response = await route.fetch()
    assert.equal(response.status(), 200)
    await route.abort('failed')
  }, { times: 1 })
  await page.getByLabel('你想说的话').fill('重启以后，还记得这句话吗？')
  await page.getByRole('button', { name: '说出' }).click()
  await page.getByRole('button', { name: '继续未完成的行动', exact: true }).waitFor()
  const counts = sql("SELECT (SELECT COUNT(*) FROM events) || ':' || (SELECT COUNT(*) FROM observation_records) || ':' || (SELECT COUNT(*) FROM rp_utterances)")
  const callsBeforeRestart = modelCalls
  const state = await context.storageState()
  assert.ok(!JSON.stringify(state).includes(credential), 'credential must not persist')
  const session = JSON.parse(state.origins[0].localStorage.find(x => x.name === 'corerp.play.v1').value).session
  await context.close()
  await stop(server)
  server = startServer(); await ready('http://127.0.0.1:8080/api/v1/rp/observe')
  context = await browser.newContext({ storageState: state, viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' })
  page = await context.newPage(); page.on('pageerror', e => errors.push(e.message))
  await page.goto('http://127.0.0.1:4178')
  await page.getByLabel('玩家访问凭证').fill(credential)
  await page.getByRole('button', { name: '继续这段生活' }).click()
  await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor()
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  assert.equal(await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).session), session)
  assert.equal(sql("SELECT (SELECT COUNT(*) FROM events) || ':' || (SELECT COUNT(*) FROM observation_records) || ':' || (SELECT COUNT(*) FROM rp_utterances)"), counts)
  assert.equal(modelCalls, callsBeforeRestart, 'committed NPC effects must not call the model again')
  assert.match(await page.locator('.reading').innerText(), /重启以后，还记得/)
  assert.match(await page.locator('.reading').innerText(), /你前往了 Ada Home/)
  await speak('我们接着聊。')
  if (fakeModel) { assert.ok(modelCalls > callsBeforeRestart); assert.match(await page.locator('.turn').last().innerText(), /来自测试模型的问候/) }
  if (lifeScenario) {
    for (let i = 0; i < 3; i++) {
      const result = await page.evaluate(async ({ credential, i }) => {
        const session = JSON.parse(localStorage.getItem('corerp.play.v1')).session
        const call = async (path, body) => {
          const response = await fetch(`/api/v1/rp/${path}`, { method: 'POST', headers: { Authorization: `Bearer ${credential}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
          if (!response.ok) throw new Error(`social HTTP ${response.status}`)
          return (await response.json()).data
        }
        const view = await call('observe', { session_id: session })
        return call('actions/social', { session_id: session, target_entity_id: 'entity_m2_rp_cai', action: 'insult', expected_cursor: view.observation_cursor, idempotency_key: `life-conflict-${i}` })
      }, { credential, i })
      assert.match(result.description, /冒犯/)
    }
    await page.getByRole('button', { name: '环顾四周' }).click()
    await page.getByText('Lin 对 Cai 做出了冒犯的手势。', { exact: true }).first().waitFor()
    await speak('你好。')
    assert.match(await page.locator('.turn').last().innerText(), /Cai 离开了/)
    assert.doesNotMatch(await page.locator('.presence').innerText(), /Cai/)
    await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/readyz')
    await page.reload()
    await page.getByLabel('玩家访问凭证').fill(credential)
    await page.getByRole('button', { name: '继续这段生活' }).click()
    await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor()
    assert.doesNotMatch(await page.locator('.presence').innerText(), /Cai/)
    assert.match(await page.locator('.reading').innerText(), /Cai 离开了/)
    assert.equal(Number(sql("SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id='entity_m2_rp_cai' AND json_extract(claim_payload,'$.claim_type')='interpersonal_action'")), 3)
  }
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'mobile overflow')
  await page.screenshot({ path: join(temp, 'play-mobile.png'), fullPage: true })
  await page.screenshot({ path: join(temp, 'play-mobile-viewport.png') })
  const latestVisible = await page.locator('.turn').last().evaluate(el => el.getBoundingClientRect().bottom <= document.querySelector('.composer').getBoundingClientRect().top)
  assert.ok(latestVisible, 'latest reply must not hide beneath composer')
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.screenshot({ path: join(temp, 'play-desktop.png'), fullPage: true })
  assert.equal(errors.length, 0, errors.join('\n'))
  console.log(JSON.stringify({ status: 'PASS', provider: fakeModel ? 'local HTTP fixture (not live LLM)' : 'deterministic', lifeScenario, modelCalls, artifacts: temp, session, recoveryCountsUnchanged: counts, hearingCount, checks: ['real session', 'same-place refusal and hearing', 'legal move and offsite exclusion', 'schedule affects NPC reply', 'scheduler wait', 'lost-response plus process/browser restart', 'same-key no duplicate facts or model calls', 'server-backed history', 'continue after restart', 'mobile no overflow', 'no credential persistence', 'no browser errors', ...(lifeScenario ? ['experienced conflict changes actual NPC movement', 'relationship knowledge and action persist after second restart'] : [])] }, null, 2))
} finally {
  await browser?.close()
  await stop(server); await stop(vite)
  if (modelServer) await new Promise(resolve => modelServer.close(resolve))
}
