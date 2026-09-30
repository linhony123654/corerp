import { chromium } from 'playwright'
import { spawn, execFileSync } from 'node:child_process'
import { mkdtemp } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { randomBytes } from 'node:crypto'
import assert from 'node:assert/strict'
import { once } from 'node:events'
import { createServer } from 'node:http'
import { checkWallet } from './rp6-wallet-checks.mjs'
import { checkStyle } from './rp6-style-checks.mjs'
import { checkRegenerate } from './rp6-regenerate-checks.mjs'
import { checkTurnStream } from './rp6-turn-stream-checks.mjs'
import { checkContextBudget } from './rp6-budget-checks.mjs'
import { checkContacts } from './rp6-contacts-checks.mjs'
import { checkWork } from './rp6-work-checks.mjs'
import { checkMap, checkMapWorks } from './rp6-map-checks.mjs'
import { checkMessages } from './rp6-messages-checks.mjs'
import { checkCustomStyle, startStylePlannerFixture } from './rp6-custom-style-checks.mjs'
import { clickPlayWait, movePlayTo, openPlayTools, openPlayBusiness, setPlayInputMode } from './play-ui-helpers.mjs'
import { checkPlayVisual } from './play-ui-visual-checks.mjs'

// Real world/service/browser. --fake-model adds only a local model HTTP fixture;
// it verifies the adapter, not live model quality. Never inherit live model credentials.
const fakeModel = process.argv.includes('--fake-model')
const lifeScenario = process.argv.includes('--life')
const emergentScenario = process.argv.includes('--emergent')
const styleScenario = process.argv.includes('--style')
const initiativeScenario = process.argv.includes('--initiative')
const visualScenario = process.argv.includes('--ui-baseline')
const walletScenario = process.argv.includes('--wallet')
const styleUIScenario = process.argv.includes('--style-ui')
const regenerateScenario = process.argv.includes('--regenerate')
const turnStreamScenario = process.argv.includes('--turn-stream')
const slowNarrativeScenario = process.argv.includes('--slow-narrative')
const contextBudgetScenario = process.argv.includes('--context-budget')
const contactsScenario = process.argv.includes('--contacts')
const workScenario = process.argv.includes('--work')
const mapScenario = process.argv.includes('--map')
const messagesScenario = process.argv.includes('--messages')
const customStyleScenario = process.argv.includes('--custom-style')
const interactionScenario = process.argv.includes('--interaction')
const root = resolve(import.meta.dirname, '..')
const temp = await mkdtemp(join(tmpdir(), 'corerp-rp1-e2e-'))
const database = join(temp, 'world.db')
const credential = randomBytes(24).toString('hex')
const creatorCredential = randomBytes(24).toString('hex')
const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [credential]: 'principal_m2_rp_player', ...(emergentScenario || initiativeScenario || mapScenario ? { [creatorCredential]: 'principal_creator' } : {}) }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex') }
Object.assign(env, { CORERP_DECISION_PROVIDER: 'deterministic', CORERP_LLM_ENDPOINT: '', CORERP_LLM_MODEL: '', CORERP_LLM_API_KEY: '', CORERP_LLM_TIMEOUT: '', CORERP_LLM_ATTEMPTS: '' })
Object.assign(env, { CORERP_NARRATIVE_PROVIDER: 'deterministic', CORERP_NARRATIVE_ENDPOINT: '', CORERP_NARRATIVE_MODEL: '', CORERP_NARRATIVE_API_KEY: '', CORERP_NARRATIVE_TIMEOUT: '', CORERP_NARRATIVE_ATTEMPTS: '' })
const sql = query => execFileSync('sqlite3', [database, query], { encoding: 'utf8' }).trim()
const run = (command, args, cwd = root) => execFileSync(command, args, { cwd, stdio: 'pipe' })
run('/usr/local/go/bin/go', ['build', '-buildvcs=false', '-o', join(temp, 'server'), './cmd/corerp-server'], join(root, 'backend'))
run('/usr/local/go/bin/go', ['build', '-buildvcs=false', '-o', join(temp, 'setup'), './cmd/corerp-m2'], join(root, 'backend'))
run(join(temp, 'setup'), ['-db', database, '-action', 'rp-travel-prepare'])
let server, vite, browser, modelServer, stylePlannerFixture
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
  if (customStyleScenario) {
    stylePlannerFixture = await startStylePlannerFixture()
    Object.assign(env, stylePlannerFixture.env)
  }
  if (fakeModel) {
    modelServer = createServer(async (request, response) => {
      try {
        let raw = ''
        for await (const chunk of request) raw += chunk
        const body = JSON.parse(raw)
        assert.equal(body.response_format.json_schema.strict, true)
        const context = JSON.parse(body.messages[1].content)
        modelCalls++
        if (context.version === 'corerp.interaction.v2') {
          const step = (kind, fields = {}) => ({ kind, target_place_id: '', target_entity_id: '', wait_hours: 0, wait_minutes: 0, speech_text: '', object_action: '', object_id: '', anchor_id: '', offer_id: '', nonverbal_action: '', gesture_code: '', ...fields })
          let proposal = { kind: 'CLARIFICATION', steps: [], clarification: 'ambiguous_intent' }
          if (context.text === '去Ada Home，随后说「重启后还记得这段行动吗？」')
            proposal = { kind: 'MIXED', steps: [step('move', { target_place_id: 'place_m2_home_ada' }), step('speech', { speech_text: '重启后还记得这段行动吗？' })], clarification: '' }
          else if (context.text === '去M2 Cafe')
            proposal = { kind: 'ACTION', steps: [step('move', { target_place_id: 'place_m2_cafe' })], clarification: '' }
          else if (['想你了。', '你是谁呀？', '等我一下，我有件事想告诉你。'].includes(context.text))
            proposal = { kind: 'DIALOGUE', steps: [step('speech', { speech_text: context.text })], clarification: '' }
          else if (['继续剧情', '接着看看后面的发展'].includes(context.text))
            proposal = { kind: 'CONTINUE', steps: [step('wait', { wait_minutes: 15 })], clarification: '' }
          else if (context.text === '我笑了笑，没有说话。')
            proposal = { kind: 'ACTION', steps: [step('nonverbal', { nonverbal_action: 'smile' })], clarification: '' }
          else if (context.text === '*看了她一眼，没有说话。*' && context.present_entities?.length)
            proposal = { kind: 'ACTION', steps: [step('nonverbal', { nonverbal_action: 'look_at', target_entity_id: context.present_entities[0].id })], clarification: '' }
          else if (context.text.includes('杯子')) proposal.clarification = 'unsupported_item'
          else if (context.text.includes('看了她一眼')) proposal.clarification = 'ambiguous_target'
          else if (context.text.includes('创建一个地点')) proposal.clarification = 'unsupported_command'
          response.setHeader('Content-Type', 'application/json')
          response.end(JSON.stringify({ choices: [{ finish_reason: 'stop', message: { content: JSON.stringify(proposal) } }] }))
          return
        }
        const input = context.character
        let action = 'respond', text = `关于“${input.player_speech_text.slice(0, 32)}”，来自测试模型的问候。`
        if (input.trigger?.kind === 'elapsed_time') {
          assert.equal(input.player_speech_text, '')
          assert.equal(input.speech_event_id, '')
          action = input.own_asset_minor < 100 ? 'respond' : 'silence'
          text = action === 'respond' ? '测试模型主动说：我得先处理手头的开销。' : ''
        }
        else if (input.player_speech_text.includes('借') || input.own_asset_minor < 100) { action = 'refuse'; text = '测试模型：抱歉，我现在无法答应。' }
        else if (input.next_schedule?.activity_code === 'work') { action = 'refuse'; text = '测试模型：我得先去工作，晚些再聊。' }
        response.setHeader('Content-Type', 'application/json')
        response.end(JSON.stringify({ choices: [{ finish_reason: 'stop', message: { content: JSON.stringify({ action, text, destination_place_id: '', activity_code: '', introduce_self: false }) } }] }))
      } catch { response.writeHead(400); response.end() }
    })
    modelServer.listen(0, '127.0.0.1'); await once(modelServer, 'listening')
    Object.assign(env, { CORERP_DECISION_PROVIDER: 'chat_completions', CORERP_LLM_ENDPOINT: `http://127.0.0.1:${modelServer.address().port}/v1/chat/completions`, CORERP_LLM_MODEL: 'test-http-model', CORERP_PROVIDER_LOCAL_ALLOWLIST: `http://127.0.0.1:${modelServer.address().port}` })
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
  if (walletScenario) await checkWallet({ page, sql, temp, stage: 'entry' })
  if (contactsScenario) await checkContacts({ page, sql, temp, stage: 'entry' })
  if (workScenario) await checkWork({ page, sql, temp, stage: 'entry', expectedJobs: 0 })
  if (mapScenario) await checkMap({ page, sql, temp, stage: 'entry' })
  if (messagesScenario) await checkMessages({ page, sql, temp, stage: 'entry' })
  const speak = async text => {
    // This R1 regression exercises explicit Speech, not the new default AUTO interpreter.
    await setPlayInputMode(page, 'speech')
    const response = page.waitForResponse(r => r.url().endsWith('/rp/turns/run'))
    await page.getByLabel('你想说的话').fill(text)
    await page.getByRole('button', { name: '说出' }).click()
    const result = await (await response).json()
    assert.ok(result.data, JSON.stringify(result))
    try {
      await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending, null, { timeout: slowNarrativeScenario ? 90_000 : 30_000 })
    } catch (error) {
      console.error(JSON.stringify({ pageErrors: errors, alert: await page.locator('[role="alert"]').allInnerTexts(), status: await page.locator('[role="status"]').allInnerTexts(), narrativeIntercepts: slowNarrativeIntercepts }))
      throw error
    }
    return result.data
  }
  if (initiativeScenario) {
    const creatorCall = async (path, body) => {
      const response = await fetch(`http://127.0.0.1:8080/api/v1/${path}`, { method: 'POST', headers: { Authorization: `Bearer ${creatorCredential}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      const result = await response.json(); assert.equal(response.status, 200, JSON.stringify(result)); return result.data
    }
    const scope = { instance_id: 'inst_m2_t09', branch_id: 'br_main' }
    const worldTime = sql("SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main'")
    const head = Number(sql("SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main'"))
    const npc = 'entity_initiative_nora'
    const materialized = await creatorCall('commands/materialize-cohort', { ...scope, command_id: 'cmd_initiative_nora', materialization_id: 'mat_initiative_nora', capability_id: 'world.cohort.materialize', idempotency_key: 'initiative-nora', expected_head: head, world_time: worldTime, source_cohort_id: 'cohort_block_a', entity_id: npc, display_name: 'Nora', population_count: 1, asset_minor: 90, inventory_minor: 1, receivable_minor: 0, liability_minor: 30, allocation_algorithm_version: 'equal-share-v1' })
    const at = hours => new Date(Date.parse(worldTime) + hours * 3600000).toISOString().replace('.000Z', 'Z')
    await creatorCall('rp/background/materialize', { ...scope, entity_id: npc, expected_head: materialized.last_sequence, idempotency_key: 'initiative-nora-background', age_min: 25, age_max: 34, residence_place_id: 'place_m2_home_bo', initial_place_id: 'place_m2_cafe', schedule: [{ world_time: at(12), place_id: 'place_m2_home_bo', activity_code: 'home' }, { world_time: at(16), place_id: 'place_m2_cafe', activity_code: 'present' }] })
    await page.getByRole('button', { name: '环顾四周' }).click()
    await page.waitForFunction(() => document.querySelector('.presence').textContent.includes('Nora'))
    await page.route('**/api/v1/rp/actions/wait', async route => {
      assert.equal(route.request().postDataJSON().opportunity_intent, 'social')
      const response = await route.fetch(); assert.equal(response.status(), 200); await route.abort('failed')
    }, { times: 1 })
    const seeking = page.getByRole('checkbox', { name: '等待时，愿意和熟人聊聊' })
    const tools = await openPlayTools(page)
    assert.equal(await seeking.isChecked(), false)
    await seeking.focus(); await page.keyboard.press('Space')
    assert.equal(await seeking.isChecked(), true)
    await tools.getByRole('button', { name: /^等四小时/ }).click()
    await page.getByRole('button', { name: '继续未完成的行动', exact: true }).waitFor()
    await openPlayTools(page)
    assert.equal(await seeking.isDisabled(), true)
    await page.getByRole('button', { name: '关闭详情' }).click()
    assert.equal(await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).pending.body.opportunity_intent), 'social')
    assert.equal(sql("SELECT json_extract(payload,'$.opportunity_intent') FROM events WHERE event_type='RPWaitCompleted' ORDER BY event_sequence DESC LIMIT 1"), 'social')
    assert.equal(sql("SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id='entity_m2_rp_lin'"), '0', 'initiative precedes all player speech')
    assert.equal(sql("SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id='entity_initiative_nora'"), '1', 'life need causes actual NPC utterance')
    const before = sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM observation_records)||':'||(SELECT COUNT(*) FROM rp_utterances)")
    const beforeCalls = modelCalls
    const saved = await context.storageState()
    assert.ok(!JSON.stringify(saved).includes(credential) && !JSON.stringify(saved).includes(creatorCredential))
    await context.close(); await stop(server)
    server = startServer(); await ready('http://127.0.0.1:8080/readyz')
    context = await browser.newContext({ storageState: saved, viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' })
    page = await context.newPage(); page.on('pageerror', e => errors.push(e.message))
    await page.goto('http://127.0.0.1:4178')
    await page.getByLabel('玩家访问凭证').fill(credential)
    await page.getByRole('button', { name: '继续这段生活' }).click()
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
    await openPlayTools(page)
    assert.equal(await page.getByRole('checkbox', { name: '等待时，愿意和熟人聊聊' }).isChecked(), false)
    await page.getByRole('button', { name: '关闭详情' }).click()
    assert.match(await page.locator('.reading').innerText(), /Nora说.*手头的开销/)
    assert.equal(sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM observation_records)||':'||(SELECT COUNT(*) FROM rp_utterances)"), before)
    assert.equal(modelCalls, beforeCalls, 'wait recovery does not repeat committed initiative model calls')
    console.log(JSON.stringify({ initiativeScenario: 'PASS', beforePlayerSpeech: true, lostWaitResponseRestart: true, factsUnchanged: before, modelCallsUnchanged: beforeCalls }))
  }
  let slowNarrativeIntercepts = 0
  if (slowNarrativeScenario) {
    await page.route('**/api/v1/rp/narrative/stream', async route => {
      const response = await route.fetch()
      assert.equal(response.status(), 200)
      await new Promise(resolve => setTimeout(resolve, 50_000))
      await route.fulfill({ response })
      slowNarrativeIntercepts++
    }, { times: 1 })
  }
  const firstTurn = await speak('能借我一点钱吗？')
  if (slowNarrativeScenario) assert.equal(slowNarrativeIntercepts, 1, 'Play did not finish a narrative response delayed beyond the old 45s browser budget')
  if (fakeModel) {
    assert.ok(firstTurn.provider_calls?.some(call => call.phase === 'decision' && call.provider_kind === 'chat_completions' && call.model_id === 'test-http-model' && call.result === 'success' && call.attempted && call.attempt_count === 1), 'real backend receipt counts the local fixture HTTP attempt')
    assert.ok(firstTurn.provider_calls?.some(call => call.phase === 'narrative' && call.provider_kind === 'deterministic' && call.render_source === 'template' && !call.attempted && call.attempt_count === 0), 'canonical narration is not attributed to the model')
  }
  assert.match(await page.locator('.reading').innerText(), /Cai 拒绝了/)
  if (visualScenario) await checkPlayVisual({ page, browser, temp })
  const hearingCount = Number(sql("SELECT COUNT(*) FROM observation_records WHERE json_extract(claim_payload, '$.claim_type') = 'speaker_said'"))
  assert.ok(hearingCount >= 2, 'speech hearing persisted')
  await movePlayTo(page, 'Ada Home')
  await page.getByRole('heading', { name: 'Ada Home' }).waitFor()
  assert.doesNotMatch(await page.locator('.presence').innerText(), /Cai/)
  await speak('你好，一起聊聊吧。')
  assert.match(await page.locator('.turn').last().innerText(), /先去工作/)
  assert.doesNotMatch(await page.locator('.turn').last().innerText(), /Cai/)
  const ordinaryWait = page.waitForRequest(r => r.url().endsWith('/rp/actions/wait'))
  const ordinaryTools = await openPlayTools(page)
  assert.equal(await ordinaryTools.getByRole('checkbox', { name: '等待时，愿意和熟人聊聊' }).isChecked(), false)
  await ordinaryTools.getByRole('button', { name: /^等四小时/ }).click()
  assert.equal((await ordinaryWait).postDataJSON().opportunity_intent, undefined)
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  assert.match(await page.locator('.turn').last().innerText(), /等待至/)
  // Cross the actual work/lunch schedule, not only an empty clock interval.
  for (let i = 0; sql("SELECT current_world_time FROM world_clocks WHERE instance_id = 'inst_m2_t09' AND branch_id = 'br_main'") < '2026-09-23T12:00:00Z'; i++) {
    assert.ok(i < 12, 'wait progression bounded')
    await clickPlayWait(page, 4)
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  }
  await movePlayTo(page, 'M2 Cafe')
  await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor()
  const lunchPresence = await page.locator('.presence').innerText()
  assert.equal((lunchPresence.match(/陌生人/g) || []).length, 2, 'sight alone must not reveal Ada or Bo')
  assert.match(lunchPresence, /Cai/, 'declared demo acquaintance remains identified')

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
  if (emergentScenario) {
    const call = async (path, body, token = creatorCredential) => {
      const response = await fetch(`http://127.0.0.1:8080/api/v1/${path}`, { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      const result = await response.json()
      assert.equal(response.status, 200, JSON.stringify(result))
      return result.data
    }
    const worldTime = sql("SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main'")
    const head = Number(sql("SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main'"))
    const scope = { instance_id: 'inst_m2_t09', branch_id: 'br_main' }
    const entity = 'entity_browser_nora'
    const materialized = await call('commands/materialize-cohort', { ...scope, command_id: 'cmd_browser_nora', materialization_id: 'mat_browser_nora', capability_id: 'world.cohort.materialize', idempotency_key: 'browser-nora', expected_head: head, world_time: worldTime, source_cohort_id: 'cohort_block_a', entity_id: entity, display_name: 'Nora', population_count: 1, asset_minor: 200, inventory_minor: 1, receivable_minor: 40, liability_minor: 30, allocation_algorithm_version: 'equal-share-v1' })
    const at = hours => new Date(Date.parse(worldTime) + hours * 3600000).toISOString().replace('.000Z', 'Z')
    const backgroundRequest = { ...scope, entity_id: entity, expected_head: materialized.last_sequence, idempotency_key: 'browser-nora-background', age_min: 25, age_max: 34, residence_place_id: 'place_m2_home_bo', initial_place_id: 'place_m2_cafe', schedule: [{ world_time: at(1), place_id: 'place_m2_home_bo', activity_code: 'home' }, { world_time: at(2), place_id: 'place_m2_cafe', activity_code: 'present' }] }
    const background = await call('rp/background/materialize', backgroundRequest)
    await page.getByRole('button', { name: '环顾四周' }).click()
    await page.waitForFunction(() => document.querySelector('.presence').textContent.includes('Nora'))
    await speak('你好，Nora。')
    assert.match(await page.locator('.turn').last().innerText(), /Nora/)
    for (const hours of [1, 2]) {
      const view = await call('rp/observe', { session_id: session }, credential)
      await call('rp/actions/wait', { session_id: session, expected_cursor: view.observation_cursor, target_world_time: at(hours), budget: 30, idempotency_key: `nora-wait-${hours}` }, credential)
      await page.getByRole('button', { name: '环顾四周' }).click()
      await page.waitForFunction(present => document.querySelector('.presence').textContent.includes('Nora') === present, hours === 2)
    }
    await speak('又见面了，Nora。')
    await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/readyz')
    await page.reload()
    await page.getByLabel('玩家访问凭证').fill(credential)
    await page.getByRole('button', { name: '继续这段生活' }).click()
    await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor()
    assert.match(await page.locator('.presence').innerText(), /Nora/)
    const restored = await call('rp/background/materialize', backgroundRequest)
    assert.equal(restored.replayed, true)
    assert.deepEqual(restored.background, background.background)
    assert.equal(Number(sql("SELECT COUNT(*) FROM materialized_entities WHERE entity_id='entity_browser_nora'")), 1)
    assert.ok(Number(sql("SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id='entity_browser_nora'")) >= 2)
    assert.ok(!JSON.stringify(await context.storageState()).includes(creatorCredential), 'creator credential never enters browser storage')
    console.log(JSON.stringify({ emergentScenario: 'PASS', checks: ['conserved Cohort materialization', 'minimal sourced background', 'RP interaction', 'schedule departure and re-encounter', 'restart stable identity and background'] }))
  }
  if (styleScenario) {
    const call = async (path, body) => {
      const response = await fetch(`http://127.0.0.1:8080/api/v1/rp/${path}`, { method: 'POST', headers: { Authorization: `Bearer ${credential}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      const value = await response.json()
      assert.equal(response.status, 200, JSON.stringify(value)); return value.data
    }
    const setting = { instance_id: 'inst_m2_t09', branch_id: 'br_main', scope: 'session', session_id: session, expected_revision: 0, idempotency_key: 'browser-style-first', patch: { pov: 'first_person', narrative_pack_ref: 'builtin/dialogue@1' } }
    await call('style/set', setting)
    await page.route('**/api/v1/rp/turns/run', async route => { await route.fetch(); await route.abort('failed') }, { times: 1 })
    await page.getByLabel('你想说的话').fill('文风重启测试。')
    await page.getByRole('button', { name: '说出' }).click()
    await page.getByRole('button', { name: '继续未完成的行动', exact: true }).waitFor()
    const pinnedCounts = sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_npc_decisions)||':'||(SELECT COUNT(*) FROM rp_utterances)")
    await call('style/set', { ...setting, expected_revision: 1, idempotency_key: 'browser-style-second', patch: { pov: 'second_person' } })
    await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/readyz')
    await page.reload(); await page.getByLabel('玩家访问凭证').fill(credential)
    await page.getByRole('button', { name: '继续这段生活' }).click()
    await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor()
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
    assert.match(await page.locator('.reading').innerText(), /我说：[\s\S]*文风重启测试/)
    assert.equal(sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_npc_decisions)||':'||(SELECT COUNT(*) FROM rp_utterances)"), pinnedCounts)
    const current = await speak('新回合使用新文风。')
    assert.equal(current.narrative_style.pov, 'second_person')
    const sameFactsBefore = sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_npc_decisions)||':'||(SELECT COUNT(*) FROM rp_utterances)")
    const variant = await call('narrative/render', { session_id: session, turn_run_id: current.turn_run_id, style_override: { pov: 'third_person', verbosity: 'detailed', description_density: 80 } })
    assert.match(variant.view.lines[0], /Lin说/)
    assert.equal(variant.view.event_ids[0], current.player_event_id)
    assert.equal(sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_npc_decisions)||':'||(SELECT COUNT(*) FROM rp_utterances)"), sameFactsBefore)
    console.log(JSON.stringify({ styleScenario: 'PASS', checks: ['session style applies to actual Play turn', 'lost-response restart preserves pinned style despite changed settings', 'next turn uses new style', 'same-event read-only variant'] }))
  }
  if (walletScenario) await checkWallet({ page, sql, temp, stage: 'restarted' })
  if (contactsScenario) await checkContacts({ page, sql, temp, stage: 'restarted' })
  if (workScenario) await checkWork({ page, sql, temp, stage: 'restarted', expectedJobs: 0 })
  if (messagesScenario) await checkMessages({ page, sql, temp, stage: 'restarted' })
  if (styleUIScenario) await checkStyle({ page, sql, temp, credential, speak, restart: async () => { await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/api/v1/rp/observe') } })
  if (regenerateScenario) await checkRegenerate({ page, sql, temp, credential, getModelCalls: () => modelCalls })
  if (turnStreamScenario) await checkTurnStream({ page, sql, temp, credential, getModelCalls: () => modelCalls, restart: async () => { await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/api/v1/rp/observe') } })
  if (contextBudgetScenario) await checkContextBudget({ page, sql, credential, getModelCalls: () => modelCalls, restart: async () => { await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/api/v1/rp/observe') } })
  if (mapScenario) await checkMapWorks({ page, sql, temp, credential, creatorCredential, restart: async () => { await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/readyz') } })
  if (customStyleScenario) await checkCustomStyle({ page, sql, temp, credential, speak, fixture: stylePlannerFixture, getModelCalls: () => modelCalls, restart: async () => { await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/readyz') } })
  if (interactionScenario) {
    await setPlayInputMode(page, 'AUTO')
    const beforeClarification = sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM agent_knowledge)||':'||(SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main')")
    await page.getByLabel('你想说或做的事').fill('继续')
    await page.getByRole('button', { name: '说出' }).click()
    await page.getByText(/请明确要说话/).waitFor()
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
    assert.equal(sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM agent_knowledge)||':'||(SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main')"), beforeClarification, 'clarification made no world effects')
    await page.route('**/api/v1/rp/interactions/run', async route => {
      const response = await route.fetch()
      assert.equal(response.status(), 200, await response.text())
      assert.equal((await response.json()).data.status, 'settled')
      await route.abort('failed')
    }, { times: 1 })
    await page.getByLabel('你想说或做的事').fill('去Ada Home，随后说「重启后还记得这段行动吗？」')
    await page.getByRole('button', { name: '说出' }).click()
    await page.getByRole('button', { name: '继续未完成的行动', exact: true }).waitFor()
    const committed = sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_utterances)||':'||(SELECT COUNT(*) FROM rp_interactions WHERE json_extract(plan_json,'$.kind')='MIXED')")
    const pendingInteraction = await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
    assert.equal(pendingInteraction.path, 'interactions/run')
    await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/readyz')
    await page.reload(); await page.getByLabel('玩家访问凭证').fill(credential)
    await page.getByRole('button', { name: '继续这段生活' }).click()
    await page.getByRole('heading', { name: 'Ada Home' }).waitFor()
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
    assert.match(await page.locator('.reading').innerText(), /重启后还记得这段行动吗/)
    assert.equal(sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_utterances)||':'||(SELECT COUNT(*) FROM rp_interactions WHERE json_extract(plan_json,'$.kind')='MIXED')"), committed, 'recovery did not duplicate mixed effects')
    assert.equal(sql("SELECT COUNT(*) FROM rp_interactions WHERE json_extract(plan_json,'$.kind')='MIXED' AND status='settled' AND next_step=2"), '1')
    await openPlayBusiness(page, '叙事设置')
    await page.getByLabel('叙述篇幅').selectOption('long')
    await page.getByRole('button', { name: '保存叙事设置' }).click()
    await page.getByText('已保存。用于之后的新段落').waitFor()
    await page.getByRole('button', { name: '关闭叙事设置' }).click()
    await setPlayInputMode(page, 'DIALOGUE')
    const longSpeech = '我把今天亲眼见到的事慢慢讲清楚。'.repeat(65)
    assert.ok([...longSpeech].length > 1000 && [...longSpeech].length < 2000)
    await page.route('**/api/v1/rp/narrative/stream', async route => {
      const response = await route.fetch()
      assert.equal(response.status(), 200)
      await route.abort('failed')
    }, { times: 1 })
    await page.getByLabel('你想说的话').fill(longSpeech)
    await page.getByRole('button', { name: '说出' }).click()
    await page.getByRole('button', { name: '继续读取叙述' }).waitFor()
    const longPending = await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
    assert.equal(longPending.path, 'interactions/run')
    assert.ok(longPending.narrative_turn_id)
    const longCommitted = sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_utterances)||':'||(SELECT COUNT(*) FROM agent_knowledge)")
    await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/readyz')
    await page.reload(); await page.getByLabel('玩家访问凭证').fill(credential)
    await page.getByRole('button', { name: '继续这段生活' }).click()
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
    assert.match(await page.locator('.turn').last().innerText(), /我把今天亲眼见到的事慢慢讲清楚/)
    assert.ok([...await page.locator('.turn').last().innerText()].length >= 1000, '1000+ characters shown in the real browser')
    assert.equal(sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_utterances)||':'||(SELECT COUNT(*) FROM agent_knowledge)"), longCommitted, 'long stream recovery has no world effects')
    assert.equal(sql(`SELECT json_extract(profile_json,'$.narrative_density') FROM rp_turn_styles WHERE turn_run_id='${longPending.narrative_turn_id}'`), 'long')
    await setPlayInputMode(page, 'AUTO')
    let committedMoveResponse
    const moveAccepted = new Promise(resolve => { committedMoveResponse = resolve })
    await page.route('**/api/v1/rp/interactions/run', async route => { const response = await route.fetch(); assert.equal(response.status(), 200); committedMoveResponse(); await route.abort('failed') }, { times: 1 })
    await page.getByLabel('你想说或做的事').fill('去M2 Cafe')
    await page.getByRole('button', { name: '说出' }).click()
    await moveAccepted
    await page.getByRole('button', { name: '尝试结束原计划' }).waitFor()
    const movedCount = sql("SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'")
    await page.getByRole('button', { name: '尝试结束原计划' }).click()
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
    await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor()
    assert.equal(sql("SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'"), movedCount, 'stop attempt did not erase or repeat settled action')
    if (fakeModel) {
      const submitAuto = async text => {
        const response = page.waitForResponse(r => r.url().endsWith('/rp/interactions/run'))
        await page.getByLabel('你想说或做的事').fill(text)
        await page.getByRole('button', { name: '说出' }).click()
        const envelope = await (await response).json()
        assert.ok(envelope.data, JSON.stringify(envelope))
        await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
        assert.equal(envelope.data.interpretation_source, 'model')
        assert.equal(envelope.data.interpretation_attempts, 1)
        return envelope.data
      }
      for (const text of ['想你了。', '你是谁呀？', '等我一下，我有件事想告诉你。']) {
        const playerMoves = sql("SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'")
        const outcome = await submitAuto(text)
        assert.equal(outcome.plan_kind, 'DIALOGUE')
        assert.equal(sql(`SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id='entity_m2_rp_lin' AND speech_text='${text}'`), '1')
        assert.equal(sql("SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved'"), playerMoves, 'speech invented player movement')
      }
      const beforeContinue = sql("SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main'")
      const beforeSpeech = sql("SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id='entity_m2_rp_lin'")
      const continued = await submitAuto('接着看看后面的发展')
      assert.equal(continued.plan_kind, 'CONTINUE')
      assert.equal(continued.outcomes[0].kind, 'wait')
      assert.equal(sql("SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main'"), new Date(Date.parse(beforeContinue) + 15 * 60000).toISOString().replace('.000Z', 'Z'))
      assert.equal(sql("SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id='entity_m2_rp_lin'"), beforeSpeech, 'continue invented player speech')
      const unchangedFacts = () => sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_utterances)||':'||(SELECT COUNT(*) FROM stock_movements)")
      for (const text of ['我把杯子轻轻推到她面前，说：“喝一点吧。”', '请帮我创建一个地点']) {
        const beforeUnsupported = unchangedFacts()
        const result = await submitAuto(text)
        assert.equal(result.status, 'clarification')
        assert.equal(unchangedFacts(), beforeUnsupported, 'unsourced item or creator command invented effects')
      }
      const silentBefore = sql("SELECT (SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id='entity_m2_rp_lin')||':'||(SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main')")
      const silentCount = Number(sql("SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction'"))
      const smile = await submitAuto('我笑了笑，没有说话。')
      assert.equal(smile.status, 'settled')
      assert.equal(smile.outcomes[0].kind, 'nonverbal')
      assert.equal(Number(sql("SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction'")), silentCount + 1)
      assert.equal(sql("SELECT (SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id='entity_m2_rp_lin')||':'||(SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main')"), silentBefore, 'neutral expression invented speech or elapsed time')
      const beforeLook = unchangedFacts()
      const look = await submitAuto('*看了她一眼，没有说话。*')
      if (look.status === 'settled') {
        assert.equal(look.outcomes[0].kind, 'nonverbal')
        assert.equal(Number(sql("SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction'")), silentCount + 2)
        assert.equal(sql("SELECT (SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id='entity_m2_rp_lin')||':'||(SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main')"), silentBefore)
      } else {
        assert.equal(look.status, 'clarification', 'unseen target cannot be inferred')
        assert.equal(unchangedFacts(), beforeLook, 'unseen target became a silent fact')
      }
    }
    console.log(JSON.stringify({ interactionScenario: 'PASS_LOCAL_FIXTURE_ONLY', clarificationNoEffects: true, recoveredMixedNoDuplicates: committed, longStreamRestartNoEffects: longCommitted, settledStopRecovered: true, unsourcedCup: 'CLARIFICATION', neutralExpression: 'COMMITTED', liveProvider: false }))
  }
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'mobile overflow')
  await page.screenshot({ path: join(temp, 'play-mobile.png'), fullPage: true })
  await page.screenshot({ path: join(temp, 'play-mobile-viewport.png') })
  const latestVisible = await page.locator('.turn').last().evaluate(el => el.getBoundingClientRect().bottom <= document.querySelector('.composer').getBoundingClientRect().top)
  assert.ok(latestVisible, 'latest reply must not hide beneath composer')
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.screenshot({ path: join(temp, 'play-desktop.png'), fullPage: true })
  assert.equal(errors.length, 0, errors.join('\n'))
  console.log(JSON.stringify({ status: 'PASS', provider: fakeModel ? 'local HTTP fixture (not live LLM)' : 'deterministic', lifeScenario, modelCalls, artifacts: temp, session, recoveryCountsUnchanged: counts, hearingCount, checks: ['real session', 'same-place refusal and hearing', 'legal move and offsite exclusion', 'schedule affects NPC reply', 'scheduler wait', 'lost-response plus process/browser restart', 'same-key no duplicate facts or model calls', 'server-backed history', 'continue after restart', 'mobile no overflow', 'no credential persistence', 'no browser errors', ...(slowNarrativeScenario ? ['50s narrative response clears pending without repeating world effects'] : []), ...(lifeScenario ? ['experienced conflict changes actual NPC movement', 'relationship knowledge and action persist after second restart'] : [])] }, null, 2))
} finally {
  await browser?.close()
  await stop(server); await stop(vite)
  if (modelServer) await new Promise(resolve => modelServer.close(resolve))
  await stylePlannerFixture?.close()
}
