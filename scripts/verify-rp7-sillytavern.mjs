import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp, cp, readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { randomBytes } from 'node:crypto';
import { chromium } from 'playwright';
import { checkRP7Actions, checkRP7BudgetWait } from './rp7-actions-checks.mjs';
import { checkRP7ChatSwitch, checkRP7AcceptedWriteChatSwitch } from './rp7-chat-switch-checks.mjs';
import { checkRP7Retirement, checkRP7OpenRetirement } from './rp7-retirement-checks.mjs';
import { checkRP7ThreeClients } from './rp7-three-client-checks.mjs';

const root = resolve(import.meta.dirname, '..');
const hostRoot = '/tmp/corerp-rp7-host-RCfN0V';
const fixture = JSON.parse(await readFile(join(root, 'clients/sillytavern/host-fixture.json'), 'utf8'));
assert.equal(JSON.parse(await readFile(join(hostRoot, 'package/package.json'), 'utf8')).version, fixture.version);
const temp = await mkdtemp(join(tmpdir(), 'corerp-rp7-extension-'));
const database = join(temp, 'world.db');
const token = randomBytes(24).toString('hex');
const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [token]: 'principal_m2_rp_player' }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'deterministic', CORERP_NARRATIVE_PROVIDER: 'deterministic' };
for (const key of ['CORERP_LLM_ENDPOINT', 'CORERP_LLM_MODEL', 'CORERP_LLM_API_KEY', 'CORERP_NARRATIVE_ENDPOINT', 'CORERP_NARRATIVE_MODEL', 'CORERP_NARRATIVE_API_KEY']) env[key] = '';
const run = (cmd, args, cwd = root) => execFileSync(cmd, args, { cwd, stdio: 'pipe' });
run('/usr/local/go/bin/go', ['build', '-o', join(temp, 'server'), './cmd/corerp-server'], join(root, 'backend'));
run('/usr/local/go/bin/go', ['build', '-o', join(temp, 'setup'), './cmd/corerp-m2'], join(root, 'backend'));
run(join(temp, 'setup'), ['-db', database, '-action', 'rp-travel-prepare']);
await cp(join(root, 'clients/sillytavern'), join(hostRoot, 'data/default-user/extensions/corerp-runtime'), { recursive: true });
let runtime, host, browser, page;
const startRuntime = () => spawn(join(temp, 'server'), ['-db', database, '-listen', '127.0.0.1:4188', '-browser-origins', 'http://127.0.0.1:4187,http://127.0.0.1:4189'], { env, stdio: 'ignore' });
const startHost = () => spawn(process.execPath, ['server.js', '--configPath', join(hostRoot, 'config.yaml')], { cwd: join(hostRoot, 'package'), stdio: 'ignore' });
const authoritySnapshot = () => run('sqlite3', [database, "SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main'; SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main'; SELECT COUNT(*) FROM events; SELECT COUNT(*) FROM materialized_entities;"]).toString();
const identitySnapshot = () => run('sqlite3', [database, 'SELECT entity_id,source_cohort_id,population_count FROM materialized_entities ORDER BY entity_id;']).toString();
async function ready(url) {
  for (let i = 0; i < 200; i++) {
    try { if ((await fetch(url)).ok) return; } catch { /* not yet */ }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error(`Service not ready: ${url}`);
}
async function stop(child) {
  if (child && child.exitCode === null && child.signalCode === null) {
    const ended = once(child, 'exit'); child.kill('SIGTERM'); await ended;
  }
}
try {
  runtime = startRuntime();
  host = startHost();
  await Promise.all([ready('http://127.0.0.1:4188/readyz'), ready('http://127.0.0.1:4187/version')]);
  browser = await chromium.launch({ headless: true });
  const browserContext = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await browserContext.route('**/*', route => ['http://127.0.0.1:4187', 'http://127.0.0.1:4188', 'http://127.0.0.1:4189'].includes(new URL(route.request().url()).origin) ? route.continue() : route.abort());
  page = await browserContext.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto('http://127.0.0.1:4187');
  const welcome = page.getByRole('heading', { name: 'Persona Name:', exact: true });
  await page.waitForFunction(() => document.querySelector('#corerp-runtime') || document.body.innerText.includes('Persona Name:'));
  if (await welcome.isVisible()) {
    await page.locator('.popup-input:visible').fill('CoreRP test player');
    await page.locator('.popup-button-ok:visible').click();
  }
  await page.waitForSelector('#corerp-runtime', { state: 'attached' });
  // Use the actual host's authenticated API to create a disposable presentation
  // card, then its real selection API. This never materializes a runtime NPC.
  const cardAvatar = await page.evaluate(async name => {
    const ctx = SillyTavern.getContext();
    const response = await fetch('/api/characters/create', { method: 'POST', headers: ctx.getRequestHeaders(), body: JSON.stringify({ ch_name: name, description: 'CoreRP test presentation only', first_mes: '世界状态由 CoreRP 提供。' }) });
    if (!response.ok) throw new Error(`fixture character ${response.status}`);
    const avatar = await response.text();
    await ctx.getCharacters();
    const id = SillyTavern.getContext().characters.findIndex(character => character.avatar === avatar);
    if (id < 0) throw new Error('fixture card missing');
    await SillyTavern.getContext().selectCharacterById(String(id));
    return avatar;
  }, `CoreRP fixture ${Date.now()}`);
  await page.waitForFunction(() => SillyTavern.getContext().getCurrentChatId() != null);
  // Open the actual host extensions drawer, not a fake extension container.
  await page.locator('#extensions-settings-button').click();
  let panel = page.locator('#corerp-runtime');
  await panel.locator('[name=origin]').fill('http://127.0.0.1:4188');
  await panel.locator('[name=token]').fill(token);
  await panel.locator('summary').click();
  await panel.locator('[name=instance]').fill('inst_m2_t09');
  await panel.locator('[name=branch]').fill('br_main');
  await panel.locator('[name=entity]').fill('entity_m2_rp_lin');
  const openRetirement = await checkRP7OpenRetirement({ page, token, cardAvatar, authoritySnapshot });
  await panel.locator('[data-action=connect]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
  assert.ok(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime?.session_id), await panel.locator('[role=status]').textContent());
  await page.waitForFunction(() => Boolean(SillyTavern.getContext().chatMetadata.corerp_runtime?.session_id) && !SillyTavern.getContext().chatMetadata.corerp_runtime.pending);
  await page.waitForFunction(() => SillyTavern.getContext().extensionPrompts.corerp_runtime?.value.includes('observer_entity_id'));
  assert.equal(await panel.locator('[name=token]').inputValue(), '');
  await panel.locator('[name=command]').fill('你好，我从酒馆进入同一个世界。');
  await panel.locator('[data-action=submit]').click();
  await page.waitForFunction(() => !SillyTavern.getContext().chatMetadata.corerp_runtime.pending && document.querySelector('#corerp-runtime pre').textContent.includes('你好，我从酒馆进入同一个世界。'));
  const binding = await page.evaluate(() => structuredClone(SillyTavern.getContext().chatMetadata.corerp_runtime));
  assert.ok(binding.session_id);
  const count = run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id='entity_m2_rp_lin';"]).toString().trim();
  assert.equal(count, '1');
  const blocked = await page.evaluate(async () => (await import('/scripts/extensions.js')).runGenerationInterceptors([], 4096, 'normal'));
  assert.equal(blocked, true);
  const hostMetadata = await page.evaluate(() => JSON.stringify({ metadata: SillyTavern.getContext().chatMetadata, settings: SillyTavern.getContext().extensionSettings, prompts: SillyTavern.getContext().extensionPrompts }));
  assert.equal(hostMetadata.includes(token), false);
  await page.screenshot({ path: join(temp, 'desktop.png') });
  await page.reload();
  await page.waitForSelector('#corerp-runtime', { state: 'attached' });
  await page.evaluate(async avatar => {
    const ctx = SillyTavern.getContext();
    await ctx.getCharacters();
    const id = SillyTavern.getContext().characters.findIndex(character => character.avatar === avatar);
    if (id < 0) throw new Error('persisted presentation card missing');
    await SillyTavern.getContext().selectCharacterById(String(id));
  }, cardAvatar);
  await page.waitForFunction(session => SillyTavern.getContext().chatMetadata.corerp_runtime?.session_id === session, binding.session_id);
  assert.equal(await page.locator('#corerp-runtime [name=token]').inputValue(), '');
  assert.equal(await page.evaluate(() => SillyTavern.getContext().extensionPrompts.corerp_runtime?.value ?? ''), '');
  await page.locator('#extensions-settings-button').click();
  await panel.locator('[name=token]').fill(token);
  await panel.locator('[data-action=connect]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && SillyTavern.getContext().extensionPrompts.corerp_runtime?.value.includes('observer_entity_id'));
  await browserContext.route('**/api/v1/rp/turns/run', async route => {
    const committed = await route.fetch();
    assert.equal(committed.status(), 200);
    await route.abort('failed'); // real commit, deliberately lost response
  }, { times: 1 });
  await panel.locator('[name=command]').fill('这条已提交的发言只应出现一次。');
  await panel.locator('[data-action=submit]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && Boolean(SillyTavern.getContext().chatMetadata.corerp_runtime.pending));
  const pending = await page.evaluate(() => structuredClone(SillyTavern.getContext().chatMetadata.corerp_runtime.pending));
  assert.equal(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id='entity_m2_rp_lin';"]).toString().trim(), '2');
  const beforeRestart = authoritySnapshot();
  const oldRuntimePID = runtime.pid, oldHostPID = host.pid;
  await page.close(); // release SSE and discard all page-memory credentials
  await Promise.all([stop(runtime), stop(host)]);
  assert.ok(runtime.exitCode !== null || runtime.signalCode !== null);
  assert.ok(host.exitCode !== null || host.signalCode !== null);
  runtime = startRuntime();
  host = startHost();
  assert.notEqual(runtime.pid, oldRuntimePID);
  assert.notEqual(host.pid, oldHostPID);
  await Promise.all([ready('http://127.0.0.1:4188/readyz'), ready('http://127.0.0.1:4187/version')]);
  assert.equal(authoritySnapshot(), beforeRestart, 'restart changed authoritative head/time/events/individuals');
  page = await browserContext.newPage();
  page.on('pageerror', error => errors.push(error.message));
  await page.goto('http://127.0.0.1:4187');
  await page.waitForSelector('#corerp-runtime', { state: 'attached' });
  panel = page.locator('#corerp-runtime');
  await page.evaluate(async avatar => {
    const ctx = SillyTavern.getContext(); await ctx.getCharacters();
    const id = SillyTavern.getContext().characters.findIndex(character => character.avatar === avatar);
    await SillyTavern.getContext().selectCharacterById(String(id));
  }, cardAvatar);
  assert.deepEqual(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.pending), pending);
  assert.equal(await panel.locator('[name=token]').inputValue(), '');
  assert.equal(await page.evaluate(() => SillyTavern.getContext().extensionPrompts.corerp_runtime?.value ?? ''), '');
  await page.locator('#extensions-settings-button').click();
  await panel.locator('[name=token]').fill(token);
  await panel.locator('[data-action=connect]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
  const acceptedBeforeRetirement = authoritySnapshot();
  await panel.locator('[data-action=retire]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && document.querySelector('#corerp-runtime [role=status]').textContent.includes('已接受'));
  assert.deepEqual(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.pending), pending);
  assert.equal(authoritySnapshot(), acceptedBeforeRetirement, 'retirement rolled back accepted speech');
  await panel.locator('[data-action=retry]').click();
  await page.waitForFunction(() => !SillyTavern.getContext().chatMetadata.corerp_runtime.pending && document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
  assert.equal(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id='entity_m2_rp_lin';"]).toString().trim(), '2');
  const unsavedText = '先保存重试记录，再把这句话提交到世界。';
  const failSave = async route => {
    const body = route.request().postDataJSON();
    if (body.chat?.[0]?.chat_metadata?.corerp_runtime?.pending?.body?.text === unsavedText) {
      await route.fulfill({ status: 503, contentType: 'application/json', body: '{"error":"test-storage-unavailable"}' });
    } else await route.continue();
  };
  await browserContext.route('**/api/chats/save', failSave);
  await panel.locator('[name=command]').fill(unsavedText);
  await panel.locator('[data-action=submit]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && document.querySelector('#corerp-runtime [role=status]').textContent.includes('未保存'));
  assert.equal(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id='entity_m2_rp_lin';"]).toString().trim(), '2');
  await browserContext.unroute('**/api/chats/save', failSave);
  await panel.locator('[data-action=retry]').click();
  await page.waitForFunction(() => !SillyTavern.getContext().chatMetadata.corerp_runtime.pending && document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
  assert.equal(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id='entity_m2_rp_lin';"]).toString().trim(), '3');
  const retirement = await checkRP7Retirement({ page, browserContext, token, authoritySnapshot });
  const actions = await checkRP7Actions({ page, token });
  assert.equal(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id='entity_m2_rp_lin';"]).toString().trim(), '4');
  const beforeSwitch = authoritySnapshot();
  const chatSwitch = await checkRP7ChatSwitch({ page, browserContext, token, cardAvatar });
  assert.equal(authoritySnapshot(), beforeSwitch, 'chat switch changed the world');
  const budgetWait = await checkRP7BudgetWait({ page });
  assert.equal(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted'; SELECT COUNT(*) FROM rp_wait_intents WHERE status='pending';"]).toString().trim(), '2\n0');
  assert.equal(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id='entity_m2_rp_lin';"]).toString().trim(), '4');
  const writeChatSwitch = await checkRP7AcceptedWriteChatSwitch({ page, browserContext, token, cardAvatar, authoritySnapshot });
  const threeClients = await checkRP7ThreeClients({ page, browserContext, token, cardAvatar, authoritySnapshot, identitySnapshot, temp, restartRuntime: async () => { const oldPID = runtime.pid; await stop(runtime); runtime = startRuntime(); assert.notEqual(runtime.pid, oldPID); await ready('http://127.0.0.1:4188/readyz'); } });
  assert.equal(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id='entity_m2_rp_lin';"]).toString().trim(), '8');
  await panel.screenshot({ path: join(temp, 'panel-desktop.png') });
  await page.setViewportSize({ width: 390, height: 844 });
  await panel.locator('[data-action=submit]').scrollIntoViewIfNeeded();
  const layout = await panel.locator('[data-action=submit]').boundingBox();
  assert.ok(layout && layout.width > 100 && layout.x >= 0 && layout.x + layout.width <= 390);
  await page.screenshot({ path: join(temp, 'mobile.png') });
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ result: 'PASS', scope: 'actual-three-client-world-and-host-recovery', hostVersion: fixture.version, runtimeAndHostRestarted: true, playerSpeechCount: 8, threeClients, openRetirement, retirement, actions, chatSwitch, writeChatSwitch, budgetWait, temp }, null, 2));
} catch (error) {
  if (page && !page.isClosed()) {
    console.error('Fixture failure:', { temp, status: await page.locator('#corerp-runtime [role=status]').textContent({ timeout: 1000 }).catch(() => 'panel unavailable') });
    // Password inputs remain masked; never log their values or network bodies.
    await page.screenshot({ path: join(temp, 'failure.png') }).catch(() => {});
  }
  throw error;
} finally {
  await browser?.close();
  await Promise.all([stop(runtime), stop(host)]);
}
