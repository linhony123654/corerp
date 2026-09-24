import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { mkdtemp, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { chromium } from 'playwright';
import { createServer } from 'vite';
import { runFinalClientStory } from './final-client-story.mjs';
import { finalLongActions, prepareFinalClientFriendships, runFinalClientLong } from './final-client-long.mjs';

// First runnable slice of Final: real Play + MCP wire + process recovery on the
// actual Final composition. This smoke run is explicitly NOT 300-turn evidence.
const root = resolve(import.meta.dirname, '..');
const temp = await mkdtemp(join(tmpdir(), 'corerp-final-clients-'));
const database = join(temp, 'world.db');
const requireMCP = createRequire(new URL('../clients/mcp/package.json', import.meta.url));
const { Client } = requireMCP('@modelcontextprotocol/client');
const { StdioClientTransport } = requireMCP('@modelcontextprotocol/client/stdio');
const token = randomBytes(24).toString('hex');
const longRequested = process.argv.includes('--long');
const storyRequested = process.argv.includes('--story') || longRequested;
const credentials = { player: token, nora: randomBytes(24).toString('hex'), ada: randomBytes(24).toString('hex'), bo: randomBytes(24).toString('hex') };
const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [token]: 'principal_m2_rp_player' }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'deterministic', CORERP_NARRATIVE_PROVIDER: 'deterministic' };
for (const key of Object.keys(env)) if (key.startsWith('CORERP_LLM_') || key.startsWith('CORERP_NARRATIVE_') && key !== 'CORERP_NARRATIVE_PROVIDER') delete env[key];
const sql = query => execFileSync('sqlite3', ['-readonly', database, query], { encoding: 'utf8' }).trim();
const build = args => execFileSync('/usr/local/go/bin/go', args, { cwd: join(root, 'backend'), stdio: 'pipe' });
let server, vite, browser, mcp, page;
const errors = [];
async function stop(child) {
  if (child && child.exitCode === null && child.signalCode === null) {
    const ended = once(child, 'exit'); child.kill('SIGTERM'); await ended;
  }
}
async function start() {
  server = spawn(join(temp, 'server'), ['-db', database, '-listen', '127.0.0.1:4198'], { env, stdio: ['ignore', 'pipe', 'pipe'] });
  let diagnostic = '';
  server.stderr.on('data', chunk => { diagnostic = (diagnostic + chunk).slice(-4000); });
  for (let i = 0; i < 100; i++) {
    if (server.exitCode !== null) throw new Error(`Runtime exited: ${diagnostic}`);
    try { if ((await fetch('http://127.0.0.1:4198/readyz', { signal: AbortSignal.timeout(1000) })).ok) return; } catch {}
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error('Final Runtime readiness timeout');
}
async function connectMCP() {
  mcp = new Client({ name: 'corerp-final-client-verifier', version: '1.0.0' });
  const transport = new StdioClientTransport({ command: process.execPath, args: [join(root, 'clients/mcp/index.js')], env: { CORERP_ORIGIN: 'http://127.0.0.1:4198', CORERP_TOKEN: token }, stderr: 'pipe' });
  try { await mcp.connect(transport); } catch (error) { await transport.close(); throw error; }
}
async function tool(name, args) {
  const result = await mcp.callTool({ name, arguments: args });
  assert.notEqual(result.isError, true, JSON.stringify(result));
  return (result.structuredContent ?? JSON.parse(result.content.find(item => item.type === 'text').text)).data;
}
async function refresh() {
  const pending = page.waitForResponse(r => r.url().endsWith('/rp/observe'));
  await page.getByRole('button', { name: '环顾四周', exact: true }).click();
  const response = await pending;
  assert.equal(response.status(), 200);
  return (await response.json()).data;
}
async function call(path, actor, body, expectedError) {
  const response = await fetch(`http://127.0.0.1:4198/api/v1/${path}`, { method: 'POST', headers: { Authorization: `Bearer ${credentials[actor]}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body), signal: AbortSignal.timeout(30000) });
  const envelope = await response.json();
  if (expectedError) { assert.equal(envelope.error?.code, expectedError, JSON.stringify(envelope)); return envelope; }
  assert.equal(response.status, 200, JSON.stringify(envelope));
  return envelope.data;
}
const command = (path, actor, body, expectedError) => call(path, actor, { binding: { instance_id: 'inst_m2_t09', branch_id: 'br_main', expected_head: Number(sql("SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main'")), idempotency_key: randomUUID() }, ...body }, expectedError);
async function say(text) {
  await refresh();
  const pending = page.waitForResponse(r => r.url().endsWith('/rp/turns/run'));
  await page.getByLabel('你想说的话').fill(text);
  await page.getByRole('button', { name: /^说出/ }).click();
  const response = await pending;
  assert.equal(response.status(), 200);
  const result = (await response.json()).data;
  assert.equal(result.status, 'settled');
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending);
  return result;
}
try {
  build(['build', '-o', join(temp, 'server'), './cmd/corerp-server']);
  build(['test', '-c', '-o', join(temp, 'fixture'), './internal/storage']);
  execFileSync(join(temp, 'fixture'), ['-test.run=^TestFinalClientFixtureExport$', `-final-client-db=${database}`, `-final-client-long=${longRequested}`], { stdio: 'pipe' });
  assert.equal(sql("SELECT COUNT(*) FROM agent_profiles WHERE status='active'"), longRequested ? '10' : '6');
  assert.equal(sql("SELECT COUNT(*) FROM cohorts WHERE instance_id='inst_m2_t09'"), '3');
  const identity = sql('SELECT agent_id,principal_id FROM agent_profiles ORDER BY agent_id');
  const fixtureEvents = sql('SELECT COUNT(*) FROM events');
  assert.throws(() => execFileSync(join(temp, 'fixture'), ['-test.run=^TestFinalClientFixtureExport$', `-final-client-db=${database}`], { stdio: 'pipe' }), 'fixture must reject existing database');
  assert.equal(sql('SELECT COUNT(*) FROM events'), fixtureEvents);
  if (storyRequested) env.CORERP_AUTH_TOKENS_JSON = JSON.stringify({ [token]: 'principal_m2_rp_player', [credentials.nora]: sql("SELECT principal_id FROM agent_profiles WHERE agent_id='entity_final_nora'"), [credentials.ada]: 'principal_m2_agent_ada', [credentials.bo]: 'principal_m2_agent_bo' });
  await start();
  vite = await createServer({ root, server: { host: '127.0.0.1', port: 4199, strictPort: true, proxy: { '/api': 'http://127.0.0.1:4198' } }, logLevel: 'error' });
  await vite.listen();
  browser = await chromium.launch({ headless: true });
  let context = await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' });
  async function enter(resume) {
    page = await context.newPage();
    page.on('pageerror', error => errors.push(error.message));
    await page.goto('http://127.0.0.1:4199');
    await page.getByLabel('玩家访问凭证').fill(token);
    await page.getByRole('button', { name: resume ? '继续这段生活 →' : '进入世界 →', exact: true }).click();
    await page.getByRole('button', { name: '环顾四周', exact: true }).waitFor();
  }
  await enter(false);
  const playSession = await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).session);
  await connectMCP();
  const worlds = await tool('corerp_worlds', {});
  const binding = worlds.bindings.find(item => item.entity_id === 'entity_m2_rp_lin');
  assert.ok(binding);
  const session = await tool('corerp_session_open', { instance_id: binding.instance_id, branch_id: binding.branch_id, entity_id: binding.entity_id, pov: 'second_person', idempotency_key: randomUUID() });
  assert.notEqual(session.session_id, playSession);
  const read = { session_id: session.session_id };
  const actions = finalLongActions(tool, read);
  if (longRequested) await prepareFinalClientFriendships(actions);
  await refresh(); // MCP setup advanced the shared cursor before Play speaks.
  const text = '今天刚搬来的 Nora 和 Eli 还习惯这里吗？';
  const pending = page.waitForResponse(r => r.url().endsWith('/rp/turns/run'));
  await page.getByLabel('你想说的话').fill(text);
  await page.getByRole('button', { name: /^说出/ }).click();
  const response = await pending;
  assert.equal(response.status(), 200);
  const playTurn = (await response.json()).data;
  assert.equal(playTurn.status, 'settled');
  await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending);
  let view = await tool('corerp_observe', read);
  assert.ok(view.recent_turns.some(turn => turn.narrative_lines.some(line => line.includes(text))));
  const speech = { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), text: '我想慢慢熟悉街区，也听听邻居们自己的打算。' };
  const turn = await tool('corerp_dialogue', speech);
  assert.equal(turn.status, 'settled');
  const fromPlay = await refresh();
  view = await tool('corerp_observe', read);
  for (const key of ['world_time', 'place_id', 'controlled_entity', 'observation_cursor']) assert.deepEqual(fromPlay[key], view[key]);
  assert.ok(fromPlay.recent_turns.some(item => item.turn_run_id === turn.turn_run_id));
  const story = storyRequested ? await runFinalClientStory({ call, command, tool, read, sql, say }) : null;
  const snapshot = observation => ({ world_time: observation.world_time, place_id: observation.place_id, controlled_entity: observation.controlled_entity, present_entities: observation.present_entities, observation_cursor: observation.observation_cursor, history: observation.recent_turns.map(item => ({ turn_run_id: item.turn_run_id, narrative_lines: item.narrative_lines })) });
  let restarts = 0;
  async function restart() {
  const finalView = snapshot(await tool('corerp_observe', read));
  assert.deepEqual(snapshot(await refresh()), finalView, 'Play and MCP diverged after story');
  const settledTurns = Number(sql("SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled'"));
  const before = sql("SELECT COUNT(*) FROM events WHERE instance_id='inst_m2_t09'");
  const balancesBefore = sql('SELECT account_id,balance_minor FROM account_balances ORDER BY account_id');
  const positionsBefore = sql('SELECT agent_id,place_id,activity_code FROM agent_positions ORDER BY agent_id');
  const saved = await context.storageState();
  for (const credential of Object.values(credentials)) assert.ok(!JSON.stringify(saved).includes(credential), 'actor credential persisted');
  await page.screenshot({ path: join(temp, 'final-client-smoke.png'), fullPage: true });
  await mcp.close(); mcp = undefined;
  await context.close();
  await stop(server); await start();
  context = await browser.newContext({ viewport: { width: 390, height: 844 }, storageState: saved, reducedMotion: 'reduce' });
  await enter(true);
  assert.equal(await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).session), playSession);
  await connectMCP(); await tool('corerp_session_resume', read);
  const replay = await tool('corerp_dialogue', speech);
  assert.equal(replay.replayed, true); assert.equal(replay.player_event_id, turn.player_event_id);
  assert.equal(sql("SELECT COUNT(*) FROM events WHERE instance_id='inst_m2_t09'"), before);
  assert.equal(sql('SELECT account_id,balance_minor FROM account_balances ORDER BY account_id'), balancesBefore);
  assert.equal(sql('SELECT agent_id,place_id,activity_code FROM agent_positions ORDER BY agent_id'), positionsBefore);
  assert.equal(sql('SELECT agent_id,principal_id FROM agent_profiles ORDER BY agent_id'), identity);
  assert.deepEqual(snapshot(await tool('corerp_observe', read)), finalView);
  assert.deepEqual(snapshot(await refresh()), finalView, 'Play recovery changed story view');
  assert.equal(Number(sql("SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled'")), settledTurns);
  restarts++;
  }
  const long = longRequested ? await runFinalClientLong({ actions, tool, read, say, sql, restart, temp, screenshot: async name => { await refresh(); await page.screenshot({ path: join(temp, `${name}.png`), fullPage: true }); } }) : null;
  await restart();
  const settledTurns = Number(sql("SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled'"));
  if (longRequested) assert.equal(settledTurns, 308);
  assert.deepEqual(errors, []);
  const result = { status: longRequested ? 'LONG_CLIENT_PASS' : storyRequested ? 'STORY_PASS' : 'SMOKE_PASS', clients: ['Play UI', 'MCP stdio'], settledTurns, story, long, restarts, sharedIdentity: true, retryWithoutDuplicate: true, artifacts: temp, limitation: longRequested ? 'Final narrative/knowledge and full regression audit remain pending; no live-model quality claim.' : 'Not the 300-turn/30-day Final acceptance; rare/long-run integration remains pending.' };
  await writeFile(join(temp, 'result.json'), JSON.stringify(result, null, 2));
  console.log(JSON.stringify(result));
} catch (error) {
  let message = String(error.message);
  for (const credential of Object.values(credentials)) message = message.replaceAll(credential, '[redacted]');
  await writeFile(join(temp, 'failure.json'), JSON.stringify({ status: 'FAIL', message, artifacts: temp }, null, 2));
  console.error(JSON.stringify({ status: 'FAIL', artifacts: temp }));
  throw error;
} finally {
  await mcp?.close(); await browser?.close(); await vite?.close(); await stop(server);
}
