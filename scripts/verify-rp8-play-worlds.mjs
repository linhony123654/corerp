import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';
import { createHash, randomBytes } from 'node:crypto';
import { chromium } from 'playwright';
import { createServer } from 'vite';

const root = resolve(import.meta.dirname, '..'), temp = await mkdtemp(join(tmpdir(), 'corerp-rp8-play-worlds-'));
const creatorUI = process.argv.includes('--creator-ui');
const interactionIsolation = process.argv.includes('--interaction-isolation');
const db = join(temp, 'world.db'), apiOrigin = 'http://127.0.0.1:4408', origin = 'http://127.0.0.1:4409';
const run = (command, args, cwd = root) => execFileSync(command, args, { cwd, stdio: 'pipe' }).toString();
for (const [name, pkg] of [['runtime', 'corerp-server'], ['m1', 'corerp-m1'], ['setup', 'corerp-m2'], ['admin', 'corerp-admin']]) run('/usr/local/go/bin/go', ['build', '-o', join(temp, name), `./cmd/${pkg}`], join(root, 'backend'));
run(join(temp, 'm1'), ['-db', db, '-action', 'inspect']);
const setup = JSON.parse(run(join(temp, 'setup'), ['-db', db, '-action', 'rp-travel-prepare']));
run(join(temp, 'admin'), ['-db', db, '-operator', 'principal_operator', '-instance', 'inst_m2_t09', '-branch', 'br_main', '-target', 'principal_creator', '-purpose', 'create_world', '-status', 'active', '-expected-head', String(setup.event_sequence), '-key', 'browser-create-authority']);
const creator = randomBytes(24).toString('hex'), player = randomBytes(24).toString('hex'), operator = randomBytes(24).toString('hex');
const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [creator]: 'principal_creator', [player]: 'principal_m2_rp_player', [operator]: 'principal_operator' }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'deterministic', CORERP_NARRATIVE_PROVIDER: 'deterministic' };
for (const key of ['CORERP_LLM_ENDPOINT', 'CORERP_LLM_MODEL', 'CORERP_LLM_API_KEY', 'CORERP_NARRATIVE_ENDPOINT', 'CORERP_NARRATIVE_MODEL', 'CORERP_NARRATIVE_API_KEY']) env[key] = '';
let runtime, vite, browser;
const start = () => spawn(join(temp, 'runtime'), ['-db', db, '-listen', '127.0.0.1:4408'], { env, stdio: 'ignore' });
async function stop(child) { if (child && child.exitCode === null && child.signalCode === null) { const ended = once(child, 'exit'); child.kill('SIGTERM'); await ended; } }
async function ready() { for (let n = 0; n < 150; n++) { try { if ((await fetch(`${apiOrigin}/readyz`)).ok) return; } catch {} await new Promise(r => setTimeout(r, 100)); } throw new Error('Runtime did not become ready'); }
// Restricted fixture canonicalizer: strings/integers/arrays/plain objects only.
const canonical = value => Array.isArray(value) ? `[${value.map(canonical).join(',')}]` : value && typeof value === 'object' ? `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonical(value[key])}`).join(',')}}` : JSON.stringify(value);
function bundle(kind) {
  const content = { version: 'corerp.studio-package.v1', ...(kind === 'system' ? { system_rules: { npc_daily_action_budget: 2 } } : { narrative_style: { version: 'corerp.style.v1', pov: 'second_person', tense: 'present', verbosity: 'terse', dialogue_ratio: 100, description_density: 0, inner_monologue_policy: 'none', prose_instructions: '', forbidden_patterns: [], narrative_pack_ref: 'builtin/plain@1' } }) };
  return { manifest: { schema_version: 'm0-draft-2026-09-22', id: `browser.${kind}`, kind, version: '1.0.0', engine_api: 'm0-draft-2026-09-22', requires: [], optional: [], capabilities: [kind === 'system' ? 'rules.npc.daily_budget' : 'narrative.style'], schema_hash: 'sha256:0b7d979a3384df06726118c293ec3533e501cd0740f0819067e8e6191c2667a0', content_hash: `sha256:${createHash('sha256').update(canonical(content)).digest('hex')}`, content_files: [`${kind}.json`] }, content };
}
const request = { authority_instance_id: 'inst_m2_t09', authority_branch_id: 'br_main', instance_id: 'browser-created-world', idempotency_key: 'browser-create', player_principal_id: 'principal_m2_rp_player', system_package: bundle('system'), narrative_package: bundle('narrative'), spec: { version: 'corerp.studio-world.v1', name: '浏览器世界', start_world_time: '2026-09-22T00:00:00Z', population: 2, opening_money_minor: 20, opening_stock_minor: 2, places: [{ key: 'home', name: '新世界的家', kind: 'home' }, { key: 'square', name: '新世界广场', kind: 'public' }], links: [{ from: 'home', to: 'square', minutes: 5 }], people: [{ key: 'lin', name: '新世界玩家', place: 'home', player: true }, { key: 'cai', name: '新世界邻居', place: 'home', player: false }] } };
try {
  runtime = start(); await ready();
  let created;
  if (!creatorUI) {
    const createdResponse = await fetch(`${apiOrigin}/api/v1/studio/worlds/create`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${creator}` }, body: JSON.stringify(request) });
    const createdEnvelope = await createdResponse.json(); assert.equal(createdResponse.status, 201, JSON.stringify(createdEnvelope));
    created = createdEnvelope.data;
  }
  vite = await createServer({ root, server: { host: '127.0.0.1', port: 4409, strictPort: true, proxy: { '/api': apiOrigin } } }); await vite.listen();
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' });
  await context.route('**/*', route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort());
  const page = await context.newPage(), errors = []; page.on('pageerror', e => errors.push(e.message));
  if (creatorUI) {
    await page.goto(`${origin}/studio/create`);
    await page.getByLabel('创建者访问凭证', { exact: true }).fill(creator);
    await page.getByLabel('授权来源世界', { exact: true }).fill('inst_m2_t09');
    await page.getByLabel('玩家身份标识', { exact: true }).fill('principal_m2_rp_player');
    await page.getByLabel('世界名称', { exact: true }).fill('浏览器创建世界');
    await page.getByLabel('玩家角色名称', { exact: true }).fill('新世界玩家');
    await page.getByLabel('邻居名称', { exact: true }).fill('新世界邻居');
    await page.getByLabel('起始居所', { exact: true }).fill('新世界的家');
    await page.getByLabel('公共地点', { exact: true }).fill('新世界广场');
    await page.getByLabel('叙事包风格', { exact: true }).selectOption('dialogue');
    await page.getByLabel('NPC 每日主动行动预算', { exact: true }).fill('2');
    assert.equal(await page.getByRole('button', { name: '保存世界与安装包', exact: true }).isDisabled(), true);
    await page.getByRole('checkbox').check();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    await page.screenshot({ path: join(temp, 'creator-draft-mobile.png'), fullPage: true });
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.screenshot({ path: join(temp, 'creator-draft-desktop.png'), fullPage: true });
    let submitted;
    await page.route('**/api/v1/studio/worlds/create', async route => {
      submitted = route.request().postDataJSON();
      const response = await route.fetch();
      const envelope = await response.json(); assert.equal(response.status(), 201, JSON.stringify(envelope));
      created = envelope.data;
      await route.abort('failed');
    }, { times: 1 });
    await page.getByRole('button', { name: '保存世界与安装包', exact: true }).click();
    await page.getByRole('alert').waitFor();
    assert.equal(await page.getByRole('heading', { name: '世界已保存', exact: true }).count(), 0);
    const counts = () => run('sqlite3', [db, 'SELECT COUNT(*) FROM world_instances; SELECT COUNT(*) FROM events; SELECT COUNT(*) FROM capability_grants;']).trim();
    const before = counts();
    await stop(runtime); runtime = start(); await ready();
    await page.reload(); assert.equal(await page.getByLabel('创建者访问凭证', { exact: true }).inputValue(), '');
    assert.equal(await page.getByRole('heading', { name: '世界已保存', exact: true }).count(), 0);
    await page.getByLabel('创建者访问凭证', { exact: true }).fill(creator);
    const retried = page.waitForRequest(r => r.url().endsWith('/studio/worlds/create'));
    await page.getByRole('button', { name: '重试 / 核对原保存请求', exact: true }).click();
    assert.deepEqual((await retried).postDataJSON(), submitted);
    await page.getByRole('heading', { name: '世界已保存', exact: true }).waitFor();
    assert.equal(counts(), before);
    await page.screenshot({ path: join(temp, 'creator-ready-desktop.png'), fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    await page.screenshot({ path: join(temp, 'creator-ready-mobile.png'), fullPage: true });
    // Clear credentials while an actual successful retry response is delayed.
    let release, received;
    const gate = new Promise(resolve => { release = resolve; });
    const obtained = new Promise(resolve => { received = resolve; });
    await page.route('**/api/v1/studio/worlds/create', async route => {
      const response = await route.fetch(); received(); await gate;
      try { await route.fulfill({ response }); } catch { /* Browser abort is expected. */ }
    });
    await page.getByRole('button', { name: '重试 / 核对原保存请求', exact: true }).click();
    await obtained;
    await page.getByRole('button', { name: '清除创建凭证', exact: true }).click(); release();
    await page.unroute('**/api/v1/studio/worlds/create');
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    assert.equal(await page.getByRole('heading', { name: '世界已保存', exact: true }).count(), 0);
    assert.equal(await page.getByLabel('创建者访问凭证', { exact: true }).inputValue(), '');
    // A replacement draft with a tampered imported package is refused before
    // any world writes; the original frozen request remains selectable.
    await page.getByRole('button', { name: '另建草稿（保留原请求）', exact: true }).click();
    await page.getByLabel('创建者访问凭证', { exact: true }).fill(creator);
    await page.getByLabel('授权来源世界', { exact: true }).fill('inst_m2_t09');
    await page.getByLabel('玩家身份标识', { exact: true }).fill('principal_m2_rp_player');
    await page.getByText('安装自定义包 JSON', { exact: true }).click();
    const invalidPackage = bundle('system'); invalidPackage.content.system_rules.npc_daily_action_budget = 9;
    await page.getByLabel('System 包 JSON', { exact: true }).fill(JSON.stringify(invalidPackage));
    await page.getByRole('checkbox').check();
    const rejected = page.waitForResponse(r => r.url().endsWith('/studio/worlds/create'));
    await page.getByRole('button', { name: '保存世界与安装包', exact: true }).click();
    assert.equal((await rejected).status(), 400); await page.getByRole('alert').waitFor();
    assert.equal(counts(), before);
    await page.getByLabel('选择保存请求', { exact: true }).selectOption(`corerp.studio.create.request.${submitted.idempotency_key}`);
    await page.getByRole('button', { name: '载入恢复请求', exact: true }).click();
    await page.getByRole('button', { name: '重试 / 核对原保存请求', exact: true }).click();
    await page.getByRole('heading', { name: '世界已保存', exact: true }).waitFor();
    assert.equal(counts(), before);
    await page.getByRole('link', { name: '使用独立玩家凭证进入 Play' }).click();
    assert.equal(await page.getByLabel('玩家访问凭证').inputValue(), '');
  }
  await page.goto(origin);
  await page.getByLabel('玩家访问凭证').fill(creator);
  await page.getByRole('button', { name: '进入世界' }).click();
  await page.getByText('暂时没有可进入的人物。', { exact: false }).waitFor();
  await page.getByLabel('玩家访问凭证').fill(player);
  await page.getByRole('button', { name: '进入世界' }).click();
  await page.getByRole('button', { name: /新世界玩家/ }).waitFor();
  assert.equal(await page.locator('.world-picker li').count(), 2);
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await page.screenshot({ path: join(temp, 'world-picker-mobile.png'), fullPage: true });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.screenshot({ path: join(temp, 'world-picker-desktop.png'), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  // Lose an actual committed open response, then reload and retry the frozen key.
  let openBody;
  await page.route('**/api/v1/rp/sessions/open', async route => {
    openBody = route.request().postDataJSON();
    assert.equal(openBody.entity_id, created.entity_id);
    const result = await route.fetch(); assert.equal(result.status(), 200);
    await route.abort('failed');
  }, { times: 1 });
  await page.getByRole('button', { name: /新世界玩家/ }).focus();
  await page.keyboard.press('Enter');
  await page.getByRole('alert').waitFor();
  const sessionCount = () => run('sqlite3', [db, 'SELECT COUNT(*) FROM rp_sessions;']).trim();
  assert.equal(sessionCount(), '1');
  await page.reload(); await page.getByLabel('玩家访问凭证').fill(player);
  const retry = page.waitForRequest(r => r.url().endsWith('/sessions/open'));
  await page.getByRole('button', { name: '进入世界' }).click();
  assert.deepEqual((await retry).postDataJSON(), openBody);
  await page.getByRole('heading', { name: '新世界的家' }).waitFor();
  assert.equal(sessionCount(), '1');
  const bookmark = await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')));
  const demoHead = () => run('sqlite3', [db, "SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main';"]).trim();
  const beforePlay = demoHead();
  const spoken = page.waitForResponse(r => r.url().endsWith('/rp/turns/run'));
  await page.getByLabel('你想说的话').fill('你好，这是我们新世界的第一天。');
  await page.getByRole('button', { name: '说出' }).click();
  const turnResponse = await spoken, turn = (await turnResponse.json()).data;
  assert.equal(turnResponse.status(), 200); assert.equal(turn.status, 'settled'); assert.ok(turn.npc_event_ids.length > 0);
  assert.equal(turn.narrative_style.verbosity, 'terse');
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending);
  await page.getByRole('button', { name: '去别处' }).click();
  await page.locator('#destinations').getByRole('button', { name: /新世界广场/ }).click();
  await page.getByRole('heading', { name: '新世界广场' }).waitFor();
  const waited = page.waitForResponse(r => r.url().endsWith('/rp/actions/wait'));
  await page.getByRole('button', { name: '等一小时', exact: true }).click();
  assert.equal((await waited).status(), 200);
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending);
  assert.equal(await page.locator('.scene time').innerText(), '01:00');
  assert.equal(demoHead(), beforePlay, 'new-world actions mutated the administrative world');
  await page.getByRole('button', { name: '切换世界', exact: true }).click();
  await page.locator('.world-picker li button').filter({ hasText: 'inst_m2_t09' }).click();
  await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor();
  await page.getByRole('button', { name: '切换世界', exact: true }).click();
  await page.getByRole('button', { name: /新世界玩家/ }).click();
  await page.getByRole('heading', { name: '新世界广场' }).waitFor();
  assert.equal(await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).session), bookmark.session);
  assert.equal(sessionCount(), '2');
  if (interactionIsolation) {
    const marker = '迟到流只属于新世界。';
    let releaseStream, streamArrived;
    const streamGate = new Promise(resolve => { releaseStream = resolve; });
    const streamReady = new Promise(resolve => { streamArrived = resolve; });
    const streamPattern = '**/api/v1/rp/narrative/stream';
    await page.route(streamPattern, async route => {
      const response = await route.fetch(); assert.equal(response.status(), 200);
      streamArrived(); await streamGate;
      try { await route.fulfill({ response }); } catch { /* page navigation aborts old stream */ }
    }, { times: 1 });
    await page.getByLabel('输入方式').selectOption('DIALOGUE');
    await page.getByLabel('你想说的话').fill(marker);
    await page.getByRole('button', { name: '说出' }).click();
    await streamReady;
    assert.equal(await page.getByRole('button', { name: '切换世界', exact: true }).isDisabled(), true, 'pending stream forbids world switch');
    const pending = await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).pending);
    assert.equal(pending.path, 'interactions/run'); assert.ok(pending.narrative_turn_id);
    await page.goto(`${origin}/studio`); releaseStream(); await page.unroute(streamPattern);
    await page.goto(origin); await page.getByLabel('玩家访问凭证').fill(player);
    await page.getByRole('button', { name: '继续这段生活' }).click();
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending);
    assert.match(await page.locator('.reading').innerText(), /迟到流只属于新世界/);
    await page.getByRole('button', { name: '切换世界', exact: true }).click();
    await page.locator('.world-picker li button').filter({ hasText: 'inst_m2_t09' }).click();
    await page.getByRole('heading', { name: 'M2 Cafe' }).waitFor();
    assert.doesNotMatch(await page.locator('.reading').innerText(), /迟到流只属于新世界/, 'late stream crossed world binding');
    await page.getByRole('button', { name: '切换世界', exact: true }).click();
    await page.getByRole('button', { name: /新世界玩家/ }).click();
    await page.getByRole('heading', { name: '新世界广场' }).waitFor();
    assert.equal(run('sqlite3', [db, `SELECT COUNT(*) FROM rp_utterances WHERE session_id='${bookmark.session}' AND speech_text='${marker}';`]).trim(), '1', 'late stream recovery repeated speech');
  }
  await stop(runtime); runtime = start(); await ready();
  await page.reload(); await page.getByLabel('玩家访问凭证').fill(player);
  await page.getByRole('button', { name: '继续这段生活' }).click();
  await page.getByRole('heading', { name: '新世界广场' }).waitFor();
  // Historical pre-picker bookmarks are upgraded from a real resume receipt.
  await page.evaluate(() => { const saved = JSON.parse(localStorage.getItem('corerp.play.v1')); delete saved.binding; localStorage.setItem('corerp.play.v1', JSON.stringify(saved)); });
  await page.goto(`${origin}/?choose_world=1`);
  await page.getByLabel('玩家访问凭证').fill(player);
  await page.getByRole('button', { name: '继续这段生活' }).click();
  await page.getByRole('button', { name: /新世界玩家/ }).waitFor();
  assert.equal(await page.locator('.world-picker li').count(), 2);
  assert.equal(sessionCount(), '2');
  // Explicit local grants on the created world, independent of source creation
  // authority. Inspect the ready event's historical package pair, not current UI.
  let inspectionHead = Number(run('sqlite3', [db, `SELECT head_sequence FROM branches WHERE instance_id='${created.instance_id}' AND branch_id='br_main';`]).trim());
  for (const target of ['principal_creator', 'principal_operator']) {
    const grant = JSON.parse(run(join(temp, 'admin'), ['-db', db, '-operator', 'principal_operator', '-instance', created.instance_id, '-branch', 'br_main', '-target', target, '-status', 'active', '-expected-head', String(inspectionHead), '-key', `inspect-created-${target}`]));
    inspectionHead = grant.event_sequence;
  }
  await page.goto(`${origin}/studio`);
  async function inspect(token) {
    await page.getByLabel('检查凭证', { exact: true }).fill(token);
    await page.getByLabel('世界实例', { exact: true }).fill(created.instance_id);
    await page.getByLabel('分支', { exact: true }).fill('br_main');
    await page.getByLabel('事件标识', { exact: true }).fill(created.ready_event_id);
    const response = page.waitForResponse(r => r.url().endsWith('/studio/events/read'));
    await page.getByRole('button', { name: '读取真实事件', exact: true }).click();
    return response;
  }
  const source = (await (await inspect(creator)).json()).data;
  assert.equal(source.rule.package_status, 'verified');
  assert.equal(source.rule.packages.system.content.system_rules.npc_daily_action_budget, 2);
  assert.equal(source.rule.packages.narrative.content.narrative_style.verbosity, 'terse');
  await page.locator('.rule-package summary').first().click();
  assert.equal(JSON.parse(await page.locator('.rule-package pre').first().innerText()).manifest.content_hash, source.rule.packages.lock.system.content_hash);
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await page.screenshot({ path: join(temp, 'rule-packages-mobile.png'), fullPage: true });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.screenshot({ path: join(temp, 'rule-packages-desktop.png'), fullPage: true });
  await page.getByRole('button', { name: '查看规则激活事件', exact: true }).focus();
  await page.keyboard.press('Enter');
  await page.getByText('此事件属于世界准备纪元，尚未使用随后激活的插件包。', { exact: true }).waitFor();
  assert.equal(await page.locator('.rule-package').count(), 0);
  const ops = (await (await inspect(operator)).json()).data;
  assert.equal(ops.rule.package_status, 'redacted'); assert.equal(ops.rule.packages, undefined);
  assert.equal(await page.locator('.rule-package').count(), 0);
  assert.equal((await inspect(player)).status(), 403);
  const stored = await page.evaluate(() => JSON.stringify({ ...localStorage, ...sessionStorage }));
  assert.ok(!stored.includes(player) && !stored.includes(creator) && !stored.includes(operator));
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ status: 'PASS', creatorUI, interactionIsolation, artifacts: temp, checks: ['real Create API', ...(creatorUI ? ['creator form + generated packages', 'explicit persistence consent', 'lost create response + runtime restart + frozen retry', 'receipt-only ready state', 'late response after credential clear', 'tampered custom package/no writes', 'archived original request recovery', 'separate Play navigation'] : []), 'separate creator/player authority', 'authorized multi-world picker', 'mobile overflow', 'keyboard choice', 'lost open response/reload/same key', 'actual NPC dialogue/installed style/move/wait', ...(interactionIsolation ? ['delayed interaction stream cancelled on navigation', 'pending blocks world switch', 'original-session recovery before switching', 'late stream absent in other world'] : []), 'unchanged administrative world', 'per-binding session recovery', 'runtime restart', 'legacy bookmark upgrade + Studio return picker', 'no persisted credentials', 'no page errors'] }, null, 2));
} finally { await browser?.close(); await vite?.close(); await stop(runtime); }
