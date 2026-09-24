// Separate real economic bootstrap: the minimal travel demo intentionally has
// no wage participation. Do not patch database projections to manufacture a job.
import { chromium } from 'playwright'
import { spawn, execFileSync } from 'node:child_process'
import { mkdtemp } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { randomBytes } from 'node:crypto'
import { once } from 'node:events'
import assert from 'node:assert/strict'
import { checkWork } from './rp6-work-checks.mjs'
import { checkMessages, prepareMessageInvitations } from './rp6-messages-checks.mjs'
import { prepareLongPlay, playLongSegment, verifyLongPlay } from './rp6-long-play-checks.mjs'

const root = resolve(import.meta.dirname, '..')
const temp = await mkdtemp(join(tmpdir(), 'corerp-rp6-work-'))
const database = join(temp, 'world.db')
const credential = randomBytes(24).toString('hex')
const messagesScenario = process.argv.includes('--messages')
const longPlayScenario = process.argv.includes('--long-play')
const creatorCredential = randomBytes(24).toString('hex'), managerCredential = randomBytes(24).toString('hex')
const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [credential]: 'principal_m2_rp_player', ...(messagesScenario || longPlayScenario ? { [creatorCredential]: 'principal_creator', [managerCredential]: 'principal_m2_agent_bo' } : {}) }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'deterministic', CORERP_LLM_ENDPOINT: '', CORERP_LLM_MODEL: '', CORERP_LLM_API_KEY: '', CORERP_LLM_TIMEOUT: '', CORERP_LLM_ATTEMPTS: '' }
const sql = query => execFileSync('sqlite3', [database, query], { encoding: 'utf8' }).trim()
Object.assign(env, { CORERP_NARRATIVE_PROVIDER: 'deterministic', CORERP_NARRATIVE_ENDPOINT: '', CORERP_NARRATIVE_MODEL: '', CORERP_NARRATIVE_API_KEY: '', CORERP_NARRATIVE_TIMEOUT: '', CORERP_NARRATIVE_ATTEMPTS: '' })
for (const [binary, source] of [['server', './cmd/corerp-server'], ['setup', './cmd/corerp-m2']]) execFileSync('/usr/local/go/bin/go', ['build', '-o', join(temp, binary), source], { cwd: join(root, 'backend'), stdio: 'pipe' })
execFileSync(join(temp, 'setup'), ['-db', database, '-action', 'rp-life-prepare'], { stdio: 'pipe' })
let server, vite, browser
const startServer = () => spawn(join(temp, 'server'), ['-db', database, '-listen', '127.0.0.1:8080'], { env, stdio: 'ignore' })
async function ready(url) {
  for (let i = 0; i < 100; i++) { try { await fetch(url); return } catch { await new Promise(resolve => setTimeout(resolve, 100)) } }
  throw new Error(`Service did not start: ${url}`)
}
async function stop(child) { if (child && child.exitCode === null) { const ended = once(child, 'exit'); child.kill('SIGTERM'); await ended } }
try {
  server = startServer()
  vite = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '4178', '--strictPort'], { cwd: root, stdio: 'ignore' })
  await Promise.all([ready('http://127.0.0.1:8080/readyz'), ready('http://127.0.0.1:4178')])
  browser = await chromium.launch({ headless: true })
  let context = await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' })
  let page = await context.newPage()
  const errors = []
  const enter = async label => {
    page.on('pageerror', error => errors.push(error.message))
    await page.goto('http://127.0.0.1:4178')
    await page.getByLabel('玩家访问凭证').fill(credential)
    await page.getByRole('button', { name: label }).click()
    await page.getByRole('heading', { name: 'M2 Cafe', exact: true }).waitFor()
  }
  await enter('进入世界')
  const longPlay = longPlayScenario ? await prepareLongPlay({ page, sql, credential, creatorCredential }) : null
  if (messagesScenario) await checkMessages({ page, sql, temp, stage: 'empty' })
  await checkWork({ page, sql, temp, stage: 'before-effective', expectedJobs: 0 })
  // Participation begins next day, not at materialization. Advance through the
  // real player wait action instead of changing the clock or projection by SQL.
  for (let hour = 0; hour < 24; hour += 4) {
    let response = page.waitForResponse(r => r.url().endsWith('/rp/actions/wait'))
    await page.getByRole('button', { name: '等四小时', exact: true }).click()
    let result = await (await response).json()
    for (let drain = 0; result.data?.status === 'budget_exhausted' && drain < 20; drain++) {
      response = page.waitForResponse(r => r.url().endsWith('/rp/actions/wait'))
      await page.getByRole('button', { name: '继续未完成的行动', exact: true }).click()
      result = await (await response).json()
    }
    assert.ok(result.data && result.data.status !== 'budget_exhausted', JSON.stringify(result))
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
  }
  let originalMessages
  if (messagesScenario) {
    await prepareMessageInvitations({ sql, credential, creatorCredential, managerCredential })
    originalMessages = await checkMessages({ page, sql, temp, stage: 'invited', expectedCount: 2 })
  }
  const original = await checkWork({ page, sql, temp, stage: 'funded', expectedJobs: 1 })
  if (longPlay) await playLongSegment({ page, sql, temp, state: longPlay, segment: 'before-restart' })
  const restartWork = longPlay ? await checkWork({ page, sql, temp, stage: 'long-before-restart', expectedJobs: 1 }) : original
  const saved = await context.storageState()
  assert.ok(!JSON.stringify(saved).includes(credential), 'credential must not persist')
  await stop(server); server = startServer(); await ready('http://127.0.0.1:8080/readyz')
  await context.close()
  context = await browser.newContext({ viewport: { width: 390, height: 844 }, storageState: saved, reducedMotion: 'reduce' })
  page = await context.newPage()
  await enter('继续这段生活')
  const recovered = await checkWork({ page, sql, temp, stage: 'funded-restarted', expectedJobs: 1 })
  assert.deepEqual(recovered, restartWork, 'same world returns same work after process/browser restart')
  if (messagesScenario) {
    const recoveredMessages = await checkMessages({ page, sql, temp, stage: 'invited-restarted', expectedCount: 2 })
    assert.deepEqual(recoveredMessages, originalMessages)
  }
  if (longPlay) {
    await playLongSegment({ page, sql, temp, state: longPlay, segment: 'after-restart' })
    console.log(JSON.stringify({ longPlay: 'UI_JOURNEY_PASS', ...verifyLongPlay({ sql, state: longPlay }), artifacts: temp, limitation: 'Initial gifts/background/source definitions are declared fixture setup, not UI actions. No live-model or statistical-frequency claim; inspect screenshots separately.' }))
  }
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
  assert.deepEqual(errors, [])
  console.log(JSON.stringify({ status: 'PASS', provider: 'deterministic (not live LLM)', artifacts: temp, checks: ['real economic bootstrap contract', 'process and browser restart equality', 'no credential persistence', 'no browser errors'] }))
} finally {
  await browser?.close()
  await stop(server); await stop(vite)
}
