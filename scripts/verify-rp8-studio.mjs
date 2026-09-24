import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';
import { randomBytes } from 'node:crypto';
import { chromium } from 'playwright';
import { createServer } from 'vite';

const root = resolve(import.meta.dirname, '..'), temp = await mkdtemp(join(tmpdir(), 'corerp-rp8-studio-'));
const db = join(temp, 'world.db'), runtimeOrigin = 'http://127.0.0.1:4398', origin = 'http://127.0.0.1:4399';
const run = (command, args, cwd = root) => execFileSync(command, args, { cwd, stdio: 'pipe' }).toString();
for (const [name, pkg] of [['runtime', 'corerp-server'], ['m1', 'corerp-m1'], ['setup', 'corerp-m2'], ['admin', 'corerp-admin']]) run('/usr/local/go/bin/go', ['build', '-o', join(temp, name), `./cmd/${pkg}`], join(root, 'backend'));
// Existing local bootstrap creates operator attribution, not arbitrary SQL grants.
run(join(temp, 'm1'), ['-db', db, '-action', 'inspect']);
const setup = JSON.parse(run(join(temp, 'setup'), ['-db', db, '-action', 'rp-travel-prepare']));
let head = setup.event_sequence;
function configure(target, status, key, explain = false) {
  const result = JSON.parse(run(join(temp, 'admin'), ['-db', db, '-operator', 'principal_operator', '-instance', 'inst_m2_t09', '-branch', 'br_main', '-target', target, '-status', status, '-expected-head', String(head), '-key', key, ...(explain ? ['-explain'] : [])]));
  head = result.event_sequence; return result;
}
configure('principal_creator', 'active', 'studio-creator', true); configure('principal_operator', 'active', 'studio-ops');
const creator = randomBytes(24).toString('hex'), operator = randomBytes(24).toString('hex'), player = randomBytes(24).toString('hex');
const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [creator]: 'principal_creator', [operator]: 'principal_operator', [player]: 'principal_m2_rp_player' }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'deterministic', CORERP_NARRATIVE_PROVIDER: 'deterministic' };
for (const key of ['CORERP_LLM_ENDPOINT', 'CORERP_LLM_MODEL', 'CORERP_LLM_API_KEY', 'CORERP_NARRATIVE_ENDPOINT', 'CORERP_NARRATIVE_MODEL', 'CORERP_NARRATIVE_API_KEY']) env[key] = '';
let runtime, vite, browser;
const start = () => spawn(join(temp, 'runtime'), ['-db', db, '-listen', '127.0.0.1:4398'], { env, stdio: 'ignore' });
async function stop(child) { if (child && child.exitCode === null && child.signalCode === null) { const ended = once(child, 'exit'); child.kill('SIGTERM'); await ended; } }
async function ready() { for (let n = 0; n < 150; n++) { try { if ((await fetch(`${runtimeOrigin}/readyz`)).ok) return; } catch {} await new Promise(r => setTimeout(r, 100)); } throw new Error('Runtime did not become ready'); }
const snapshot = () => run('sqlite3', [db, "SELECT instance_id,branch_id,head_sequence FROM branches ORDER BY instance_id,branch_id; SELECT COUNT(*) FROM events; SELECT COUNT(*) FROM commands; SELECT COUNT(*) FROM audit_records; SELECT current_world_time FROM world_clocks ORDER BY instance_id;"]);
try {
  runtime = start(); await ready();
  vite = await createServer({ root, server: { host: '127.0.0.1', port: 4399, strictPort: true, proxy: { '/api': runtimeOrigin } } }); await vite.listen();
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.route('**/*', route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort());
  const page = await context.newPage(), errors = []; page.on('pageerror', e => errors.push(e.message));
  await page.goto(`${origin}/studio`);
  const read = page.getByRole('button', { name: '读取真实事件' });
  assert.equal(await read.isDisabled(), true);
  async function fill(token, id = setup.event_id) {
    await page.getByLabel('检查凭证', { exact: true }).fill(token);
    await page.getByLabel('世界实例', { exact: true }).fill('inst_m2_t09');
    await page.getByLabel('分支', { exact: true }).fill('br_main');
    await page.getByLabel('事件标识', { exact: true }).fill(id);
  }
  async function submit(status = 200) {
    const response = page.waitForResponse(r => r.url().endsWith('/api/v1/studio/events/read'));
    await read.click(); const res = await response; assert.equal(res.status(), status); return res;
  }
  const before = snapshot();
  // Discover a real authorized branch and select its real event, without
  // typing any world/branch/event IDs into the page.
  await page.getByLabel('检查凭证', { exact: true }).fill(creator);
  await page.getByRole('button', { name: '列出授权世界', exact: true }).click();
  await page.locator('.scope-picker .studio-list button').first().waitFor();
  assert.equal(await page.locator('.scope-picker .studio-list button').count(), 1);
  await page.locator('.scope-picker .studio-list button').first().click();
  await page.locator('.studio-timeline .studio-list li').first().waitFor();
  assert.equal(await page.locator('.studio-timeline .studio-list li').count(), 5);
  await page.getByRole('button', { name: '更早的事件', exact: true }).click();
  await page.waitForFunction(() => document.querySelectorAll('.studio-timeline .studio-list li').length === 10);
  const selectedResponse = page.waitForResponse(r => r.url().endsWith('/api/v1/studio/events/read'));
  await page.locator('.studio-timeline .studio-list button').filter({ hasText: setup.event_id }).click();
  const selected = await selectedResponse; assert.equal(selected.status(), 200);
  const first = (await selected.json()).data;
  await page.getByText('CREATOR · 获授权内容', { exact: true }).waitFor();
  assert.equal(first.event_id, setup.event_id); assert.equal(first.redacted, false);
  async function explain(status = 200) {
    const pending = page.waitForResponse(r => r.url().endsWith('/api/v1/studio/events/explain'));
    await page.getByRole('button', { name: '读取记录解释', exact: true }).click();
    const response = await pending; assert.equal(response.status(), status); return response;
  }
  await explain();
  await page.getByText('此事件没有保存的候选验证记录，无法据此推断原因。', { exact: true }).waitFor();
  await page.getByText('查看原始事件内容', { exact: true }).click();
  assert.deepEqual(JSON.parse(await page.locator('.payload pre').innerText()), first.payload);
  async function assertSectionsDoNotOverlap() {
    const rule = await page.locator('.studio-rule').boundingBox(), cause = await page.locator('.cause').boundingBox();
    assert.ok(rule && cause && rule.y + rule.height <= cause.y, 'rule evidence overlaps causal explanation');
  }
  await assertSectionsDoNotOverlap();
  await page.screenshot({ path: join(temp, 'studio-desktop.png'), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await assertSectionsDoNotOverlap();
  await page.screenshot({ path: join(temp, 'studio-mobile.png'), fullPage: true });
  await page.getByLabel('事件标识', { exact: true }).fill('unknown'); await submit(404);
  await page.getByRole('alert').waitFor(); assert.equal(await page.locator('article').count(), 0);
  await fill(operator); const ops = (await (await submit()).json()).data;
  await page.getByText('OPS · 脱敏诊断', { exact: true }).waitFor(); assert.equal(ops.redacted, true);
  for (const key of ['payload', 'actor_id', 'command_id', 'causation_event_id']) assert.equal(ops[key], undefined);
  assert.equal(await page.locator('.payload').count(), 0);
  await explain(403); await page.getByText('需要额外的解释权限，请由本地管理员显式开通。', { exact: true }).waitFor();
  await fill(player); await submit(403); await page.getByRole('alert').waitFor(); assert.equal(await page.locator('article').count(), 0);
  // An actual authorized response arrives after credentials/scope changed.
  await fill(creator);
  let release; const gate = new Promise(r => { release = r; });
  let received; const hasResponse = new Promise(r => { received = r; });
  await page.route('**/api/v1/studio/events/read', async route => { const response = await route.fetch(); received(); await gate; try { await route.fulfill({ response }); } catch {} });
  await read.click(); await hasResponse; await page.getByRole('button', { name: '清除凭证与结果' }).click(); release();
  await page.unroute('**/api/v1/studio/events/read');
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  assert.equal(await page.locator('article').count(), 0); assert.equal(await page.getByLabel('检查凭证', { exact: true }).inputValue(), '');
  // A late real directory response must also stay isolated from cleared credentials.
  await fill(creator);
  let releaseList; const listGate = new Promise(r => { releaseList = r; });
  let listReceived; const listResponse = new Promise(r => { listReceived = r; });
  await page.route('**/api/v1/studio/scopes/list', async route => { const response = await route.fetch(); listReceived(); await listGate; try { await route.fulfill({ response }); } catch {} });
  await page.getByRole('button', { name: '列出授权世界', exact: true }).click(); await listResponse;
  await page.getByRole('button', { name: '清除凭证与结果' }).click(); releaseList();
  await page.unroute('**/api/v1/studio/scopes/list');
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  assert.equal(await page.locator('.scope-picker .studio-list button').count(), 0);
  assert.equal(await page.locator('.studio-timeline').count(), 0);
  assert.equal(snapshot(), before);
  await fill(creator); await submit(); await page.reload();
  assert.equal(await page.getByLabel('检查凭证', { exact: true }).inputValue(), ''); assert.equal(await page.locator('article').count(), 0);
  const stored = await page.evaluate(() => JSON.stringify([Object.entries(localStorage), Object.entries(sessionStorage)]));
  for (const secret of [creator, operator, player]) assert.equal(stored.includes(secret), false);
  await stop(runtime); runtime = start(); await ready(); assert.equal(snapshot(), before);
  await fill(creator); const recovered = (await (await submit()).json()).data; assert.deepEqual(recovered, first);
  // Produce real candidate diagnostics and hearing records through an actual turn.
  async function rp(path, body) {
    const response = await fetch(`${runtimeOrigin}/api/v1/rp/${path}`, { method: 'POST', headers: { Authorization: `Bearer ${player}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    const value = await response.json(); assert.equal(response.status, 200, JSON.stringify(value)); return value.data;
  }
  const session = await rp('sessions/open', { instance_id: 'inst_m2_t09', branch_id: 'br_main', entity_id: 'entity_m2_rp_lin', pov: 'second_person', idempotency_key: 'studio-explanation-session' });
  const observed = await rp('observe', { session_id: session.session_id });
  const turn = await rp('turns/run', { session_id: session.session_id, expected_cursor: observed.observation_cursor, idempotency_key: 'studio-explanation-turn', text: '你好，现在有空聊聊吗？' });
  head = turn.settled_sequence;
  const explainedBefore = snapshot();
  await fill(creator, turn.player_event_id); await submit();
  const actualExplanation = (await (await explain()).json()).data;
  assert.ok(actualExplanation.diagnostics.length > 0); assert.ok(actualExplanation.observers.length > 0);
  await page.locator('.explanation-records li').first().waitFor();
  assert.match(await page.locator('.explanation-records').innerText(), /候选通过验证/);
  await page.getByLabel('观察者标识（可选）').fill('entity_m2_agent_ada');
  assert.equal(await page.locator('.explanation-records').count(), 0);
  await explain(); await page.getByText('没有找到观察记录；这不是“不知道”的证明。', { exact: true }).waitFor();
  await page.getByLabel('观察者标识（可选）').fill(''); await explain();
  await page.locator('.explanation-records li').first().waitFor();
  assert.ok(turn.npc_event_ids.length > 0);
  await fill(creator, turn.npc_event_ids[0]); await submit();
  const committedExplanation = (await (await explain()).json()).data;
  assert.equal(committedExplanation.committed_decision.trigger_event_id, turn.player_event_id);
  assert.equal(committedExplanation.committed_decision.outcome, 'committed_speech');
  await page.getByText('已提交发言', { exact: true }).waitFor();
  for (const [width, height, name] of [[1440, 1000, 'desktop'], [390, 844, 'mobile']]) {
    await page.setViewportSize({ width, height }); await assertSectionsDoNotOverlap();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    await page.screenshot({ path: join(temp, `studio-explanation-${name}.png`), fullPage: true });
  }
  const triggerRead = page.waitForResponse(r => r.url().endsWith('/api/v1/studio/events/read'));
  const triggerButton = page.getByRole('button', { name: /^查看触发事件/ });
  await triggerButton.focus(); await page.keyboard.press('Enter');
  assert.equal((await (await triggerRead).json()).data.event_id, turn.player_event_id);
  await page.getByRole('button', { name: '读取记录解释', exact: true }).waitFor();
  let releaseExplanation, receivedExplanation;
  const explanationGate = new Promise(r => { releaseExplanation = r; }), explanationReceived = new Promise(r => { receivedExplanation = r; });
  await page.route('**/api/v1/studio/events/explain', async route => { const response = await route.fetch(); receivedExplanation(); await explanationGate; try { await route.fulfill({ response }); } catch {} });
  await page.getByRole('button', { name: '读取记录解释', exact: true }).click(); await explanationReceived;
  await page.getByRole('button', { name: '清除凭证与结果' }).click(); releaseExplanation();
  await page.unroute('**/api/v1/studio/events/explain');
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  assert.equal(await page.locator('.explanation-records').count(), 0); assert.equal(snapshot(), explainedBefore);
  await fill(creator); await submit();
  configure('principal_creator', 'revoked', 'studio-revoke'); await submit(403); await page.getByRole('alert').waitFor(); assert.equal(await page.locator('article').count(), 0);
  assert.equal(await page.locator('.studio-timeline').count(), 0);
  await page.getByRole('button', { name: '列出授权世界', exact: true }).click();
  await page.getByText('没有已授权的世界分支，请联系本地管理员。', { exact: true }).waitFor();
  await page.getByRole('link', { name: '回到 Play ↗' }).click(); await page.getByLabel('玩家访问凭证').waitFor(); assert.equal(await page.locator('.studio').count(), 0);
  assert.deepEqual(errors, []); console.log(JSON.stringify({ status: 'PASS', temp, event: setup.event_id, readHead: first.head_sequence, finalHead: head, restart: true, roles: ['creator', 'operator', 'player'] }));
} finally { await browser?.close(); await vite?.close(); await stop(runtime); }
