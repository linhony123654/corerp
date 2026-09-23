import { chromium } from 'playwright'
import { spawn, execFileSync } from 'node:child_process'
import { mkdtemp } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { randomBytes } from 'node:crypto'
import assert from 'node:assert/strict'
import { once } from 'node:events'

// Disposable real service/database; no mocks, external accounts or persisted credentials.
const root = resolve(import.meta.dirname, '..')
const temp = await mkdtemp(join(tmpdir(), 'corerp-rp1-e2e-'))
const database = join(temp, 'world.db')
const credential = randomBytes(24).toString('hex')
const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [credential]: 'principal_m2_rp_player' }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex') }
const sql = query => execFileSync('sqlite3', [database, query], { encoding: 'utf8' }).trim()
const run = (command, args, cwd = root) => execFileSync(command, args, { cwd, stdio: 'pipe' })
run('/usr/local/go/bin/go', ['build', '-o', join(temp, 'server'), './cmd/corerp-server'], join(root, 'backend'))
run('/usr/local/go/bin/go', ['build', '-o', join(temp, 'setup'), './cmd/corerp-m2'], join(root, 'backend'))
run(join(temp, 'setup'), ['-db', database, '-action', 'rp-travel-prepare'])
let server, vite, browser
const startServer = () => spawn(join(temp, 'server'), ['-db', database, '-listen', '127.0.0.1:8080'], { env, stdio: 'ignore' })
async function ready(url) {
  for (let i = 0; i < 100; i++) {
    try { await fetch(url); return } catch { await new Promise(r => setTimeout(r, 100)) }
  }
  throw new Error(`Service did not start: ${url}`)
}
async function stop(child) { if (child && child.exitCode === null) { const exited = once(child, 'exit'); child.kill('SIGTERM'); await exited } }
try {
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
  assert.match(await page.locator('.reading').innerText(), /重启以后，还记得/)
  assert.match(await page.locator('.reading').innerText(), /你前往了 Ada Home/)
  await speak('我们接着聊。')
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'mobile overflow')
  await page.screenshot({ path: join(temp, 'play-mobile.png'), fullPage: true })
  await page.screenshot({ path: join(temp, 'play-mobile-viewport.png') })
  const latestVisible = await page.locator('.turn').last().evaluate(el => el.getBoundingClientRect().bottom <= document.querySelector('.composer').getBoundingClientRect().top)
  assert.ok(latestVisible, 'latest reply must not hide beneath composer')
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.screenshot({ path: join(temp, 'play-desktop.png'), fullPage: true })
  assert.equal(errors.length, 0, errors.join('\n'))
  console.log(JSON.stringify({ status: 'PASS', artifacts: temp, session, recoveryCountsUnchanged: counts, hearingCount, checks: ['real session', 'same-place refusal and hearing', 'legal move and offsite exclusion', 'schedule affects NPC reply', 'scheduler wait', 'lost-response plus process/browser restart', 'same-key no duplicate facts', 'server-backed history', 'continue after restart', 'mobile no overflow', 'no credential persistence', 'no browser errors'] }, null, 2))
} finally {
  await browser?.close()
  await stop(server); await stop(vite)
}
