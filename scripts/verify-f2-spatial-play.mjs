import { chromium } from 'playwright'
import { spawn, execFileSync } from 'node:child_process'
import { mkdtemp } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { randomBytes } from 'node:crypto'
import { once } from 'node:events'
import assert from 'node:assert/strict'

const root = resolve(import.meta.dirname, '..')
const temp = await mkdtemp(join(tmpdir(), 'corerp-f2-play-'))
const database = join(temp, 'world.db')
const playerToken = randomBytes(24).toString('hex')
const creatorToken = randomBytes(24).toString('hex')
const env = {
  ...process.env,
  CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [playerToken]: 'principal_m2_rp_player', [creatorToken]: 'principal_creator' }),
  CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'),
  CORERP_DECISION_PROVIDER: 'deterministic', CORERP_LLM_ENDPOINT: '', CORERP_LLM_MODEL: '', CORERP_LLM_API_KEY: '',
  CORERP_NARRATIVE_PROVIDER: 'deterministic', CORERP_NARRATIVE_ENDPOINT: '', CORERP_NARRATIVE_MODEL: '', CORERP_NARRATIVE_API_KEY: ''
}
const run = (cmd, args, cwd = root) => execFileSync(cmd, args, { cwd, stdio: 'pipe' })
const sql = query => execFileSync('sqlite3', [database, query], { encoding: 'utf8' }).trim()
run('/usr/local/go/bin/go', ['build', '-o', join(temp, 'server'), './cmd/corerp-server'], join(root, 'backend'))
run('/usr/local/go/bin/go', ['build', '-o', join(temp, 'setup'), './cmd/corerp-m2'], join(root, 'backend'))
run(join(temp, 'setup'), ['-db', database, '-action', 'rp-travel-prepare'])
const startServer = () => spawn(join(temp, 'server'), ['-db', database, '-listen', '127.0.0.1:8080'], { env, stdio: 'ignore' })
async function ready(url) {
  for (let i = 0; i < 100; i++) {
    try { const response = await fetch(url); if (response.ok) return } catch { /* retry while starting */ }
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  throw new Error(`service did not start: ${url}`)
}
async function stop(child) {
  if (child && child.exitCode === null) { const exited = once(child, 'exit'); child.kill('SIGTERM'); await exited }
}
let server, vite, browser, context
try {
  server = startServer()
  vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4178', '--strictPort'], { cwd: root, stdio: 'ignore' })
  await Promise.all([ready('http://127.0.0.1:8080/readyz'), ready('http://127.0.0.1:4178')])
  browser = await chromium.launch({ headless: true })
  context = await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' })
  let page = await context.newPage()
  const pageErrors = []
  page.on('pageerror', error => pageErrors.push(error.message))
  await page.goto('http://127.0.0.1:4178')
  await page.getByLabel('玩家访问凭证').fill(playerToken)
  await page.getByRole('button', { name: '进入世界' }).click()
  await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor()
  const creatorCall = async (path, body) => {
    const response = await fetch(`http://127.0.0.1:8080/api/v1/rp/${path}`, {
      method: 'POST', headers: { Authorization: `Bearer ${creatorToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body)
    })
    const envelope = await response.json()
    assert.equal(response.status, 200, JSON.stringify(envelope))
    return envelope.data
  }
  const scope = { instance_id: 'inst_m2_t09', branch_id: 'br_main' }
  const head = Number(sql("SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main'"))
  const segment = await creatorCall('locations/materialize', { binding: { ...scope, expected_head: head, idempotency_key: 'browser-road' }, parent_location_id: 'place_m2_cafe', slot_key: 'browser-road', candidate: { display_name: '路上的小径', generator_version: 'local-v1' } })
  await creatorCall('edges/define', { binding: { ...scope, expected_head: segment.event_sequence, idempotency_key: 'browser-edge' }, from_place_id: 'place_m2_cafe', to_place_id: 'place_m2_home_ada', segment_place_id: segment.fact.location_id, duration_minutes: 15 })
  await page.getByRole('button', { name: '地图', exact: true }).click()
  const map = page.getByRole('dialog', { name: '附近地图' })
  await map.getByText('约 15 分钟 · 途中有真实路段').waitFor()
  await page.route('**/api/v1/rp/journeys/start', async route => {
    const response = await route.fetch()
    assert.equal(response.status(), 200)
    await route.abort('failed')
  }, { times: 1 })
  await map.getByRole('button', { name: '开始旅程，前往 Ada Home' }).click()
  await page.getByRole('button', { name: '继续未完成的行动', exact: true }).waitFor()
  assert.equal(sql("SELECT COUNT(*) FROM events WHERE event_type='RPJourneyStarted'"), '1')
  assert.equal(sql("SELECT place_id FROM agent_positions WHERE agent_id='entity_m2_rp_lin'"), segment.fact.location_id)
  const saved = await context.storageState()
  assert.ok(!JSON.stringify(saved).includes(playerToken) && !JSON.stringify(saved).includes(creatorToken), 'credential persisted in browser')
  await context.close(); context = null
  await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/readyz')
  context = await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce', storageState: saved })
  page = await context.newPage()
  page.on('pageerror', error => pageErrors.push(error.message))
  await page.goto('http://127.0.0.1:4178')
  await page.getByLabel('玩家访问凭证').fill(playerToken)
  await page.getByRole('button', { name: '继续这段生活' }).click()
  await page.getByRole('heading', { name: '路上的小径' }).waitFor()
  await page.getByText('正在途中 · 预计').waitFor()
  assert.equal(sql("SELECT COUNT(*) FROM events WHERE event_type='RPJourneyStarted'"), '1', 'lost-response recovery duplicated start')
  await page.getByRole('button', { name: '取消自动抵达' }).click()
  await page.getByText('已停止自动抵达').waitFor()
  assert.equal(sql("SELECT place_id FROM agent_positions WHERE agent_id='entity_m2_rp_lin'"), segment.fact.location_id, 'cancellation teleported traveler')
  assert.equal(sql("SELECT COUNT(*) FROM rp_journeys WHERE status='cancelled' AND agent_id='entity_m2_rp_lin'"), '1')
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
  assert.equal(overflow, false, 'mobile journey UI overflow')
  assert.deepEqual(pageErrors, [])
  console.log(JSON.stringify({ spatialPlay: 'PASS', artifacts: temp, checks: ['timed route shown in local map', 'lost response plus server/page restart reuses one journey', 'active segment restored', 'cancel remains at real segment', 'mobile no overflow', 'no browser errors'] }))
} finally {
  await context?.close()
  await browser?.close()
  await stop(vite)
  await stop(server)
}
