import assert from 'node:assert/strict';
import test from 'node:test';
import { execFileSync, spawn } from 'node:child_process';
import { mkdtemp } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { randomBytes, randomUUID } from 'node:crypto';
import { createServer } from 'node:net';
import { createServer as createHTTPServer } from 'node:http';
import { once } from 'node:events';
import { Client } from '@modelcontextprotocol/client';
import { StdioClientTransport } from '@modelcontextprotocol/client/stdio';

const root = resolve(import.meta.dirname, '../..');
async function freePort() {
  const server = createServer(); server.listen(0, '127.0.0.1'); await once(server, 'listening');
  const port = server.address().port; await new Promise(resolve => server.close(resolve)); return port;
}
async function stop(child) { if (child.exitCode === null && child.signalCode === null) { const ended = once(child, 'exit'); child.kill('SIGTERM'); await ended; } }
async function ready(origin) {
  for (let i = 0; i < 150; i++) {
    try { if ((await fetch(`${origin}/readyz`)).ok) return; } catch { /* starting */ }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error('temporary runtime did not become ready');
}
async function connect(origin, token, modern = false) {
  const client = new Client({ name: 'corerp-integration', version: '1.0.0' }, modern ? { versionNegotiation: { mode: { pin: '2026-07-28' } } } : {});
  const transport = new StdioClientTransport({ command: process.execPath, args: [join(import.meta.dirname, 'index.js')], env: { CORERP_ORIGIN: origin, CORERP_TOKEN: token }, stderr: 'pipe' });
  let stderr = ''; transport.stderr?.on('data', chunk => { stderr += chunk.toString(); });
  try { await client.connect(transport); } catch (error) { await transport.close(); throw error; }
  return { client, stderr: () => stderr };
}
function data(result) {
  assert.notEqual(result.isError, true, JSON.stringify(result));
  const envelope = result.structuredContent ?? JSON.parse(result.content.find(block => block.type === 'text').text);
  return envelope.data;
}

test('actual MCP stdio → authenticated Runtime → same authoritative world/recovery', { timeout: 150_000 }, async t => {
  const temp = await mkdtemp(join(tmpdir(), 'corerp-rp7-mcp-'));
  const database = join(temp, 'world.db'); const executable = join(temp, 'server'); const setup = join(temp, 'setup');
  const run = (command, args) => execFileSync(command, args, { cwd: join(root, 'backend'), stdio: 'pipe' });
  run('/usr/local/go/bin/go', ['build', '-o', executable, './cmd/corerp-server']);
  run('/usr/local/go/bin/go', ['build', '-o', setup, './cmd/corerp-m2']);
  run(setup, ['-db', database, '-action', 'rp-travel-prepare']);
  const token = randomBytes(24).toString('hex'), creatorToken = randomBytes(24).toString('hex');
  const origin = `http://127.0.0.1:${await freePort()}`;
  const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [token]: 'principal_m2_rp_player', [creatorToken]: 'principal_creator' }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'deterministic', CORERP_NARRATIVE_PROVIDER: 'deterministic' };
  for (const name of ['CORERP_LLM_ENDPOINT', 'CORERP_LLM_MODEL', 'CORERP_LLM_API_KEY', 'CORERP_NARRATIVE_ENDPOINT', 'CORERP_NARRATIVE_MODEL', 'CORERP_NARRATIVE_API_KEY']) env[name] = '';
  const runtime = spawn(executable, ['-db', database, '-listen', new URL(origin).host], { env, stdio: 'ignore' });
  t.after(() => stop(runtime)); await ready(origin);
  let connected = await connect(origin, token); t.after(() => connected.client.close());
  const tools = await connected.client.listTools();
  assert.equal(tools.tools.length, 17);
  assert.ok(tools.tools.every(tool => !JSON.stringify(tool.inputSchema).includes('principal_id')));
  const call = async (name, args) => data(await connected.client.callTool({ name, arguments: args }));
  const worlds = await call('corerp_worlds', {});
  assert.equal(worlds.bindings.length, 1);
  const { instance_id, branch_id, entity_id } = worlds.bindings[0];
  const session = await call('corerp_session_open', { instance_id, branch_id, entity_id, pov: 'second_person', idempotency_key: randomUUID() });
  const read = { session_id: session.session_id };
  let view = await call('corerp_observe', read);
  const speech = { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), text: '我从真正的 MCP 工具进入同一个世界。' };
  const turn = await call('corerp_dialogue', speech); assert.equal(turn.status, 'settled');
  const retry = await call('corerp_dialogue', speech); assert.equal(retry.player_event_id, turn.player_event_id); assert.equal(retry.replayed, true);
  await connected.client.close(); connected = await connect(origin, token);
  await call('corerp_session_resume', read);
  const resumed = await call('corerp_turn_resume', { ...read, idempotency_key: speech.idempotency_key }); assert.equal(resumed.player_event_id, turn.player_event_id);
  const accepted = await call('corerp_request_retire', { ...read, operation: 'dialogue', idempotency_key: speech.idempotency_key }); assert.equal(accepted.status, 'completed');
  const context = await call('corerp_context', read); assert.equal(context.observer_entity_id, entity_id); assert.ok(context.facts.length > 0);
  const events = await call('corerp_events', { ...read, limit: 50 }); assert.equal(events.protocol_version, 'corerp.client.v1');
  const forged = await connected.client.callTool({ name: 'corerp_observe', arguments: { ...read, principal_id: 'principal_creator' } }); assert.equal(forged.isError, true);
  const mismatch = await connected.client.callTool({ name: 'corerp_dialogue', arguments: { ...speech, text: '不得使用相同 key 修改已接受发言。' } }); assert.equal(mismatch.isError, true); assert.match(JSON.stringify(mismatch), /IDEMPOTENCY_PAYLOAD_MISMATCH/u);
  view = await call('corerp_observe', read);
  const stale = await connected.client.callTool({ name: 'corerp_command', arguments: { operation: 'move', request: { ...read, expected_cursor: speech.expected_cursor, idempotency_key: randomUUID(), from_place_id: view.place_id, to_place_id: 'place_m2_home_ada' } } }); assert.equal(stale.isError, true);
  await call('corerp_command', { operation: 'social', request: { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), action: 'greet', target_entity_id: 'entity_m2_rp_cai' } });
  view = await call('corerp_observe', read);
  await call('corerp_command', { operation: 'move', request: { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), from_place_id: view.place_id, to_place_id: 'place_m2_home_ada' } });
  view = await call('corerp_observe', read);
  const wait = { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), target_world_time: '2026-09-23T08:00:00Z', budget: 1 };
  let waited = await call('corerp_wait', wait); assert.equal(waited.status, 'budget_exhausted');
  assert.equal((await call('corerp_request_retire', { ...read, operation: 'wait', idempotency_key: wait.idempotency_key })).status, 'in_progress');
  for (let i = 0; i < 20 && waited.status !== 'completed'; i++) waited = await call('corerp_wait', wait);
  assert.equal(waited.status, 'completed');
  const defaultMode = await call('corerp_interaction_default', read); assert.equal(defaultMode.interaction_mode, 'AUTO');
  const setMode = await call('corerp_interaction_default_set', { ...read, interaction_mode: 'DIALOGUE', expected_revision: defaultMode.revision, idempotency_key: randomUUID() });
  assert.equal(setMode.interaction_mode, 'DIALOGUE');
  view = await call('corerp_observe', read);
  const mixed = { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), interaction_mode: 'SCENE', text: '去M2 Cafe，随后说「这是同一世界里的混合行动。」' };
  const interaction = await call('corerp_interaction', mixed);
  assert.equal(interaction.status, 'settled'); assert.equal(interaction.plan_kind, 'MIXED'); assert.deepEqual(interaction.outcomes.map(outcome => outcome.kind), ['move', 'speech']);
  assert.equal((await call('corerp_interaction_resume', { ...read, idempotency_key: mixed.idempotency_key })).replayed, true);
  assert.equal((await call('corerp_request_retire', { ...read, operation: 'interaction', idempotency_key: mixed.idempotency_key })).status, 'completed');
  assert.equal((await call('corerp_interaction_stop', { ...read, idempotency_key: mixed.idempotency_key })).status, 'settled');
  view = await call('corerp_observe', read);
  const waitSpeech = await call('corerp_interaction', { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), interaction_mode: 'SCENE', text: '等1小时，随后说「等过一小时我们再谈。」' });
  assert.equal(waitSpeech.status, 'settled'); assert.deepEqual(waitSpeech.outcomes.map(outcome => outcome.kind), ['wait', 'speech']);
  assert.ok(waitSpeech.outcomes[0].settled_sequence >= waitSpeech.outcomes[0].event_sequence);
  assert.ok(waitSpeech.outcomes[1].event_sequence > waitSpeech.outcomes[0].settled_sequence);
  const retiredKey = randomUUID();
  assert.equal((await call('corerp_request_retire', { operation: 'open', idempotency_key: retiredKey })).status, 'retired');
  const late = await connected.client.callTool({ name: 'corerp_session_open', arguments: { instance_id, branch_id, entity_id, pov: 'first_person', idempotency_key: retiredKey } }); assert.equal(late.isError, true); assert.match(JSON.stringify(late), /REQUEST_RETIRED/u);
  const foreign = await connect(origin, creatorToken); t.after(() => foreign.client.close());
  const denied = await foreign.client.callTool({ name: 'corerp_context', arguments: read }); assert.equal(denied.isError, true); assert.match(JSON.stringify(denied), /NOT_FOUND/u);

  // SDK defaults to the legacy initialize handshake; also pin a genuinely
  // modern connection so both protocol eras are tested, not merely advertised.
  const modern = await connect(origin, token, true); t.after(() => modern.client.close());
  assert.equal(modern.client.getServerVersion().name, 'corerp-runtime');
  assert.equal((await modern.client.listTools()).tools.length, 17);
  assert.deepEqual(data(await modern.client.callTool({ name: 'corerp_context', arguments: read })), await call('corerp_context', read));

  let loseReply = true;
  const proxy = createHTTPServer(async (request, response) => {
    try {
      const parts = []; for await (const part of request) parts.push(part);
      const upstream = await fetch(origin + request.url, { method: request.method, headers: { Authorization: request.headers.authorization, 'Content-Type': 'application/json' }, body: request.method === 'POST' ? Buffer.concat(parts) : undefined });
      const body = await upstream.text();
      if (request.url === '/api/v1/rp/turns/run' && loseReply) { loseReply = false; assert.equal(upstream.status, 200); response.destroy(); return; }
      response.writeHead(upstream.status, { 'Content-Type': 'application/json' }); response.end(body);
    } catch { response.destroy(); }
  });
  proxy.listen(0, '127.0.0.1'); await once(proxy, 'listening');
  t.after(() => { proxy.closeAllConnections(); return new Promise(resolve => proxy.close(resolve)); });
  const lossy = await connect(`http://127.0.0.1:${proxy.address().port}`, token); t.after(() => lossy.client.close());
  view = await call('corerp_observe', read);
  const lostSpeech = { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), text: 'MCP 响应丢失后也不能重复这一句话。' };
  const lost = await lossy.client.callTool({ name: 'corerp_dialogue', arguments: lostSpeech });
  assert.equal(lost.isError, true); assert.match(JSON.stringify(lost), /TRANSPORT_UNCERTAIN/u);
  assert.equal(data(await lossy.client.callTool({ name: 'corerp_request_retire', arguments: { ...read, operation: 'dialogue', idempotency_key: lostSpeech.idempotency_key } })).status, 'completed');
  const recovered = data(await lossy.client.callTool({ name: 'corerp_dialogue', arguments: lostSpeech })); assert.equal(recovered.replayed, true);
  assert.equal(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id='entity_m2_rp_lin';"]).toString().trim(), '4');
  assert.equal(connected.stderr().includes(token), false); assert.equal(foreign.stderr().includes(creatorToken), false);
  t.diagnostic(`real MCP legacy+2026-07-28 stdio/runtime PASS fixture ${temp}; MCP-process restart, move/wait mixed interactions and lost accepted reply recovered, exactly four distinct player speeches`);
});
