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
  const database = join(temp, 'world.db'); const executable = join(temp, 'server'); const setup = join(temp, 'setup'); const controller = join(temp, 'controller');
  const run = (command, args) => execFileSync(command, args, { cwd: join(root, 'backend'), stdio: 'pipe' });
  run('/usr/local/go/bin/go', ['build', '-o', executable, './cmd/corerp-server']);
  run('/usr/local/go/bin/go', ['build', '-o', setup, './cmd/corerp-m2']);
  run('/usr/local/go/bin/go', ['build', '-o', controller, './cmd/corerp-controller']);
  run(setup, ['-db', database, '-action', 'rp-travel-prepare']);
  run('sqlite3', [database, "INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_mcp_resident_a','service','MCP Resident A','active'),('principal_mcp_resident_b','service','MCP Resident B','active');"]);
  const token = randomBytes(24).toString('hex'), creatorToken = randomBytes(24).toString('hex'), operatorToken = randomBytes(24).toString('hex');
  const residentAToken = randomBytes(24).toString('hex'), residentBToken = randomBytes(24).toString('hex');
  const origin = `http://127.0.0.1:${await freePort()}`;
  const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [token]: 'principal_m2_rp_player', [creatorToken]: 'principal_creator', [operatorToken]: 'principal_operator', [residentAToken]: 'principal_mcp_resident_a', [residentBToken]: 'principal_mcp_resident_b' }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'deterministic', CORERP_NARRATIVE_PROVIDER: 'deterministic' };
  for (const name of ['CORERP_LLM_ENDPOINT', 'CORERP_LLM_MODEL', 'CORERP_LLM_API_KEY', 'CORERP_NARRATIVE_ENDPOINT', 'CORERP_NARRATIVE_MODEL', 'CORERP_NARRATIVE_API_KEY']) env[name] = '';
  const runtime = spawn(executable, ['-db', database, '-listen', new URL(origin).host], { env, stdio: 'ignore' });
  t.after(() => stop(runtime)); await ready(origin);
  let connected = await connect(origin, token); t.after(() => connected.client.close());
  const tools = await connected.client.listTools();
  assert.equal(tools.tools.length, 26);
  assert.ok(tools.tools.some(tool => tool.name === 'corerp_round_speech'));
  assert.ok(tools.tools.some(tool => tool.name === 'corerp_round_move'));
  assert.ok(tools.tools.every(tool => !JSON.stringify(tool.inputSchema).includes('principal_id')));
  const call = async (name, args) => data(await connected.client.callTool({ name, arguments: args }));
  const worlds = await call('corerp_worlds', {});
  assert.equal(worlds.bindings.length, 1);
  const { instance_id, branch_id, entity_id } = worlds.bindings[0];
  const session = await call('corerp_session_open', { instance_id, branch_id, entity_id, pov: 'second_person', idempotency_key: randomUUID() });
  const read = { session_id: session.session_id };
  for (const [name, args] of [
    ['corerp_round_read', { ...read, round_id: 'missing-round' }],
    ['corerp_round_wait', { ...read, round_id: 'missing-round', horizon_world_time: '2026-09-22T03:00:00Z', idempotency_key: randomUUID() }],
    ['corerp_round_advance', { ...read, round_id: 'missing-round', budget: 1 }],
  ]) {
    const missing = await connected.client.callTool({ name, arguments: args });
    assert.equal(missing.isError, true); assert.match(JSON.stringify(missing), /NOT_FOUND/u);
  }
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
  view = await call('corerp_observe', read);
  const mapNote = await call('corerp_map_survey', { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID() });
  assert.equal(mapNote.fact.observer_id, entity_id);
  assert.equal((await call('corerp_map_read', read))[0].place_id, view.place_id);
  const creatorCall = async (route, body) => {
    const response = await fetch(`${origin}/api/v1/rp/${route}`, { method: 'POST', headers: { Authorization: `Bearer ${creatorToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    const envelope = await response.json(); assert.equal(response.status, 200, JSON.stringify(envelope)); return envelope.data;
  };
  const scope = { instance_id, branch_id };
  const segment = await creatorCall('locations/materialize', { binding: { ...scope, expected_head: mapNote.event_sequence, idempotency_key: 'mcp-segment' }, parent_location_id: view.place_id, slot_key: 'mcp-road', candidate: { display_name: 'MCP 路段', generator_version: 'local-v1' } });
  await creatorCall('edges/define', { binding: { ...scope, expected_head: segment.event_sequence, idempotency_key: 'mcp-edge' }, from_place_id: view.place_id, to_place_id: 'place_m2_home_ada', segment_place_id: segment.fact.location_id, duration_minutes: 15 });
  view = await call('corerp_observe', read);
  const journeyRequest = { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), from_place_id: view.place_id, to_place_id: 'place_m2_home_ada' };
  const journey = await call('corerp_journey_start', journeyRequest);
  assert.equal(journey.segment_place_id, segment.fact.location_id);
  assert.equal((await call('corerp_journey_start', journeyRequest)).replayed, true);
  view = await call('corerp_observe', read);
  assert.equal(view.place_id, segment.fact.location_id); assert.equal(view.active_journey.journey_id, journey.journey_id);
  await call('corerp_journey_cancel', { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), journey_id: journey.journey_id });
  view = await call('corerp_observe', read);
  assert.equal(view.place_id, segment.fact.location_id); assert.equal(view.active_journey, undefined);
  const retiredKey = randomUUID();
  assert.equal((await call('corerp_request_retire', { operation: 'open', idempotency_key: retiredKey })).status, 'retired');
  const late = await connected.client.callTool({ name: 'corerp_session_open', arguments: { instance_id, branch_id, entity_id, pov: 'first_person', idempotency_key: retiredKey } }); assert.equal(late.isError, true); assert.match(JSON.stringify(late), /REQUEST_RETIRED/u);
  const foreign = await connect(origin, creatorToken); t.after(() => foreign.client.close());
  const denied = await foreign.client.callTool({ name: 'corerp_context', arguments: read }); assert.equal(denied.isError, true); assert.match(JSON.stringify(denied), /NOT_FOUND/u);

  // SDK defaults to the legacy initialize handshake; also pin a genuinely
  // modern connection so both protocol eras are tested, not merely advertised.
  const modern = await connect(origin, token, true); t.after(() => modern.client.close());
  assert.equal(modern.client.getServerVersion().name, 'corerp-runtime');
  assert.equal((await modern.client.listTools()).tools.length, 26);
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
  const head = () => Number(run('sqlite3', [database, "SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main';"]).toString().trim());
  const localController = (action, request) => JSON.parse(execFileSync(controller, ['-db', database, '-action', action], { cwd: join(root, 'backend'), input: JSON.stringify(request), stdio: 'pipe' }).toString());
  for (const [entity, principal, controllerInstance] of [
    ['entity_m2_agent_ada', 'principal_mcp_resident_a', 'mcp-resident-a'],
    ['entity_m2_agent_bo', 'principal_mcp_resident_b', 'mcp-resident-b'],
  ]) {
    const binding = key => ({ ...scope, principal_id: 'principal_operator', expected_head: head(), idempotency_key: key });
    localController('enroll', { binding: binding(randomUUID()), entity_id: entity, controller_principal_id: principal, controller_instance_id: controllerInstance });
    localController('assign', { binding: binding(randomUUID()), entity_id: entity, expected_generation: 0 });
  }
  let residentA = await connect(origin, residentAToken), residentB = await connect(origin, residentBToken);
  t.after(() => residentA.client.close()); t.after(() => residentB.client.close());
  const callA = async (name, args) => data(await residentA.client.callTool({ name, arguments: args }));
  const callB = async (name, args) => data(await residentB.client.callTool({ name, arguments: args }));
  const bindingA = (await callA('corerp_worlds', {})).bindings;
  const bindingB = (await callB('corerp_worlds', {})).bindings;
  assert.equal(bindingA.length, 1); assert.equal(bindingB.length, 1);
  assert.equal(bindingA[0].entity_id, 'entity_m2_agent_ada'); assert.equal(bindingB[0].entity_id, 'entity_m2_agent_bo');
  const residentBinding = item => ({ instance_id: item.instance_id, branch_id: item.branch_id, entity_id: item.entity_id });
  const sessionA = await callA('corerp_session_open', { ...residentBinding(bindingA[0]), pov: 'second_person', idempotency_key: randomUUID() });
  const sessionB = await callB('corerp_session_open', { ...residentBinding(bindingB[0]), pov: 'second_person', idempotency_key: randomUUID() });
  const ownA = { session_id: sessionA.session_id }, ownB = { session_id: sessionB.session_id };
  const humanView = await call('corerp_observe', read);
  await callA('corerp_observe', ownA); await callB('corerp_observe', ownB);
  const operatorResponse = await fetch(`${origin}/api/v1/rp/rounds/open`, { method: 'POST', headers: { Authorization: `Bearer ${operatorToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ binding: { ...scope, expected_head: head(), idempotency_key: randomUUID() }, human_session_id: read.session_id, external_session_ids: [ownA.session_id, ownB.session_id] }) });
  const operatorEnvelope = await operatorResponse.json(); assert.equal(operatorResponse.status, 200, JSON.stringify(operatorEnvelope));
  const roundID = operatorEnvelope.data.round_id, roundA = { ...ownA, round_id: roundID }, roundB = { ...ownB, round_id: roundID }, roundHuman = { ...read, round_id: roundID };
  const forbiddenOtherSession = await residentB.client.callTool({ name: 'corerp_round_read', arguments: roundA });
  assert.equal(forbiddenOtherSession.isError, true); assert.match(JSON.stringify(forbiddenOtherSession), /NOT_FOUND/u);
  const due = new Date(Date.parse(humanView.world_time) + 10 * 60_000).toISOString().replace('.000Z', 'Z');
  const beforeSharedWaits = Number(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted';"]).toString().trim());
  assert.equal((await callA('corerp_round_wait', { ...roundA, horizon_world_time: due, idempotency_key: randomUUID() })).submitted, 1);
  assert.equal((await callB('corerp_round_wait', { ...roundB, horizon_world_time: due, idempotency_key: randomUUID() })).submitted, 2);
  const withoutHuman = await residentA.client.callTool({ name: 'corerp_round_advance', arguments: { ...roundA, budget: 100 } });
  assert.equal(withoutHuman.isError, true); assert.match(JSON.stringify(withoutHuman), /COMMAND_IN_PROGRESS/u);
  await residentA.client.close();
  assert.equal((await call('corerp_round_wait', { ...roundHuman, horizon_world_time: due, idempotency_key: randomUUID() })).submitted, 3);
  const shared = await callB('corerp_round_advance', { ...roundB, budget: 100 });
  assert.equal(shared.status, 'settled'); assert.equal(shared.current_world_time, due);
  for (const forbidden of ['entity_m2_agent_ada', 'entity_m2_agent_bo', 'principal_mcp_resident_a', 'principal_mcp_resident_b', 'event_rp_wait_', 'wait_event_id', 'advance_target']) assert.equal(JSON.stringify(shared).includes(forbidden), false);
  assert.equal((await callB('corerp_round_read', roundB)).event_sequence, shared.event_sequence);
  assert.equal(Number(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted';"]).toString().trim()), beforeSharedWaits + 1);
  residentA = await connect(origin, residentAToken);
  await callA('corerp_session_resume', ownA);
  assert.equal((await callA('corerp_round_advance', { ...roundA, budget: 150 })).replayed, true);
  assert.equal(Number(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted';"]).toString().trim()), beforeSharedWaits + 1);
  let lastBSpeech;
  for (const [residentCall, own, actor, label] of [
    [callA, ownA, 'entity_m2_agent_ada', 'A'],
    [callB, ownB, 'entity_m2_agent_bo', 'B'],
  ]) {
    for (let decision = 1; decision <= 2; decision++) {
      const observed = await residentCall('corerp_observe', own);
      const request = { ...own, expected_cursor: observed.observation_cursor, idempotency_key: randomUUID(), text: `外部居民${label}第${decision}次自主发言。` };
      const spoken = await residentCall('corerp_dialogue', request);
      assert.equal(spoken.status, 'settled');
      if (label === 'B' && decision === 2) lastBSpeech = { request, eventID: spoken.player_event_id };
    }
    assert.equal(Number(run('sqlite3', [database, `SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id='${actor}';`]).toString().trim()), 2);
  }
  const actionHumanView = await call('corerp_observe', read);
  await callA('corerp_observe', ownA); await callB('corerp_observe', ownB);
  const actionOpenResponse = await fetch(`${origin}/api/v1/rp/rounds/open`, { method: 'POST', headers: { Authorization: `Bearer ${operatorToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ binding: { ...scope, expected_head: head(), idempotency_key: randomUUID() }, human_session_id: read.session_id, external_session_ids: [ownA.session_id, ownB.session_id] }) });
  const actionOpenEnvelope = await actionOpenResponse.json(); assert.equal(actionOpenResponse.status, 200, JSON.stringify(actionOpenEnvelope));
  const actionID = actionOpenEnvelope.data.round_id;
  const actionA = { ...ownA, round_id: actionID }, actionB = { ...ownB, round_id: actionID }, actionHuman = { ...read, round_id: actionID };
  const beforeActions = head();
  assert.equal((await callA('corerp_round_speech', { ...actionA, text: 'A提交私有动作提案。', idempotency_key: randomUUID() })).submitted, 1);
  assert.equal((await callB('corerp_round_speech', { ...actionB, text: 'B提交冲突动作提案。', idempotency_key: randomUUID() })).submitted, 2);
  assert.equal(head(), beforeActions, 'MCP action proposal changed Event head before Human response');
  const actionWithoutHuman = await residentB.client.callTool({ name: 'corerp_round_advance', arguments: { ...actionB, budget: 100 } });
  assert.equal(actionWithoutHuman.isError, true); assert.match(JSON.stringify(actionWithoutHuman), /COMMAND_IN_PROGRESS/u);
  const bypassDialogue = await residentA.client.callTool({ name: 'corerp_dialogue', arguments: { ...ownA, expected_cursor: beforeActions, idempotency_key: randomUUID(), text: '不经共享窗口抢先说话。' } });
  assert.equal(bypassDialogue.isError, true); assert.match(JSON.stringify(bypassDialogue), /COMMAND_IN_PROGRESS/u);
  const actionHorizon = new Date(Date.parse(actionHumanView.world_time) + 10 * 60_000).toISOString().replace('.000Z', 'Z');
  assert.equal((await call('corerp_round_wait', { ...actionHuman, horizon_world_time: actionHorizon, idempotency_key: randomUUID() })).submitted, 3);
  const actionSettled = await callB('corerp_round_advance', { ...actionB, budget: 100 });
  assert.equal(actionSettled.status, 'settled'); assert.equal(actionSettled.event_sequence, beforeActions + 1);
  assert.equal(actionSettled.own_disposition, 'deferred_no_effect');
  assert.equal((await callA('corerp_round_read', actionA)).own_disposition, 'action_accepted');
  assert.equal(head(), beforeActions + 1);
  assert.equal(Number(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted';"]).toString().trim()), beforeSharedWaits + 1);
  for (const forbidden of ['principal_mcp_resident_a', 'principal_mcp_resident_b', 'entity_m2_agent_ada', 'entity_m2_agent_bo', 'event_rp_speech_', 'completion_event_id']) assert.equal(JSON.stringify(actionSettled).includes(forbidden), false);
  // Advance once so Human's second wait is not consumed by the same-time
  // anti-starvation rule, then let a different resident's move compete with speech.
  const openResidentRound = async () => {
    const response = await fetch(`${origin}/api/v1/rp/rounds/open`, { method: 'POST', headers: { Authorization: `Bearer ${operatorToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ binding: { ...scope, expected_head: head(), idempotency_key: randomUUID() }, human_session_id: read.session_id, external_session_ids: [ownA.session_id, ownB.session_id] }) });
    const envelope = await response.json(); assert.equal(response.status, 200, JSON.stringify(envelope)); return envelope.data.round_id;
  };
  await call('corerp_observe', read); await callA('corerp_observe', ownA); await callB('corerp_observe', ownB);
  const betweenID = await openResidentRound();
  const betweenAt = new Date(Date.parse(actionHumanView.world_time) + 10 * 60_000).toISOString().replace('.000Z', 'Z');
  for (const [residentCall, own] of [[call, read], [callA, ownA], [callB, ownB]]) await residentCall('corerp_round_wait', { ...own, round_id: betweenID, horizon_world_time: betweenAt, idempotency_key: randomUUID() });
  assert.equal((await callB('corerp_round_advance', { ...ownB, round_id: betweenID, budget: 100 })).current_world_time, betweenAt);
  await call('corerp_observe', read); await callA('corerp_observe', ownA);
  const moveOriginB = (await callB('corerp_observe', ownB)).place_id;
  const moveID = await openResidentRound(), beforeMove = head();
  const moveB = { ...ownB, round_id: moveID, from_place_id: moveOriginB, to_place_id: 'place_m2_cafe', idempotency_key: randomUUID() };
  assert.equal((await callB('corerp_round_move', moveB)).submitted, 1);
  assert.equal((await callA('corerp_round_speech', { ...ownA, round_id: moveID, text: 'A本轮的冲突发言。', idempotency_key: randomUUID() })).submitted, 2);
  assert.equal(head(), beforeMove, 'MCP move proposal mutated world before Human');
  const missingHumanMove = await residentB.client.callTool({ name: 'corerp_round_advance', arguments: { ...ownB, round_id: moveID, budget: 100 } });
  assert.equal(missingHumanMove.isError, true); assert.match(JSON.stringify(missingHumanMove), /COMMAND_IN_PROGRESS/u);
  const moveHorizon = new Date(Date.parse(betweenAt) + 10 * 60_000).toISOString().replace('.000Z', 'Z');
  await call('corerp_round_wait', { ...read, round_id: moveID, horizon_world_time: moveHorizon, idempotency_key: randomUUID() });
  const moveReceipt = await callB('corerp_round_advance', { ...ownB, round_id: moveID, budget: 100 });
  assert.equal(moveReceipt.own_disposition, 'action_accepted'); assert.equal(moveReceipt.current_world_time, betweenAt);
  assert.equal(moveReceipt.event_sequence, beforeMove + 1); assert.equal((await callB('corerp_round_move', moveB)).replayed, true);
  assert.equal((await callA('corerp_round_read', { ...ownA, round_id: moveID })).own_disposition, 'deferred_no_effect');
  assert.equal((await callB('corerp_observe', ownB)).place_id, 'place_m2_cafe');
  for (const [residentCall, own] of [[callA, ownA], [callB, ownB]]) {
    const observed = await residentCall('corerp_observe', own);
    if (observed.place_id !== 'place_m2_cafe') await residentCall('corerp_command', { operation: 'move', request: { ...own, expected_cursor: observed.observation_cursor, idempotency_key: randomUUID(), from_place_id: observed.place_id, to_place_id: 'place_m2_cafe' } });
  }
  const together = await callB('corerp_observe', ownB);
  assert.equal(together.place_id, 'place_m2_cafe');
  assert.ok(together.present_entities.length > 0, 'external residents did not meet in the same place');
  let cursorB;
  for (let page = 0; page < 20; page++) {
    const history = await callB('corerp_events', { ...ownB, ...(cursorB ? { cursor: cursorB } : {}), limit: 50 });
    cursorB = history.next_cursor;
    if (!history.more_events) break;
    if (page === 19) assert.fail('resident B history did not catch up');
  }
  const audibleText = '同场时B应听见这句话。';
  let observedA = await callA('corerp_observe', ownA);
  assert.equal((await callA('corerp_dialogue', { ...ownA, expected_cursor: observedA.observation_cursor, idempotency_key: randomUUID(), text: audibleText })).status, 'settled');
  const heard = await callB('corerp_events', { ...ownB, cursor: cursorB, limit: 50 });
  assert.ok(heard.events.some(event => event.facts.some(fact => fact.kind === 'speaker_said' && fact.text === audibleText)), 'co-located service resident missed A speech');
  const leaving = await callB('corerp_observe', ownB);
  await callB('corerp_command', { operation: 'move', request: { ...ownB, expected_cursor: leaving.observation_cursor, idempotency_key: randomUUID(), from_place_id: leaving.place_id, to_place_id: 'place_m2_work_bo' } });
  const distantText = '离场后B不应听见这句话。';
  observedA = await callA('corerp_observe', ownA);
  assert.equal((await callA('corerp_dialogue', { ...ownA, expected_cursor: observedA.observation_cursor, idempotency_key: randomUUID(), text: distantText })).status, 'settled');
  const afterLeaving = await callB('corerp_events', { ...ownB, cursor: heard.next_cursor, limit: 50 });
  assert.equal(afterLeaving.events.some(event => event.facts.some(fact => fact.kind === 'speaker_said' && fact.text === distantText)), false, 'distant B heard face-to-face speech');
  for (const entity of ['entity_m2_agent_ada', 'entity_m2_agent_bo']) {
    const released = localController('release', { binding: { ...scope, principal_id: 'principal_operator', expected_head: head(), idempotency_key: randomUUID() }, entity_id: entity, expected_generation: 1 });
    assert.equal(released.fact.generation, 2);
  }
  assert.equal((await callA('corerp_worlds', {})).bindings.length, 0);
  assert.equal((await callB('corerp_worlds', {})).bindings.length, 0);
  const staleB = await residentB.client.callTool({ name: 'corerp_observe', arguments: ownB });
  assert.equal(staleB.isError, true); assert.match(JSON.stringify(staleB), /BRANCH_VERSION_CONFLICT/u);
  const oldSpeech = await callB('corerp_dialogue', lastBSpeech.request);
  assert.equal(oldSpeech.replayed, true); assert.equal(oldSpeech.player_event_id, lastBSpeech.eventID);
  const freshB = await residentB.client.callTool({ name: 'corerp_dialogue', arguments: { ...lastBSpeech.request, idempotency_key: randomUUID() } });
  assert.equal(freshB.isError, true); assert.match(JSON.stringify(freshB), /BRANCH_VERSION_CONFLICT/u);
  const afterReleaseHuman = await call('corerp_observe', read);
  const afterReleaseTarget = new Date(Date.parse(afterReleaseHuman.world_time) + 60_000).toISOString().replace('.000Z', 'Z');
  assert.equal((await call('corerp_wait', { ...read, expected_cursor: afterReleaseHuman.observation_cursor, idempotency_key: randomUUID(), target_world_time: afterReleaseTarget, budget: 100 })).status, 'completed');
  assert.equal((await callB('corerp_round_read', roundB)).current_world_time, due, 'released service observed later clock through settled receipt');
  assert.equal((await callB('corerp_round_advance', { ...roundB, budget: 13 })).replayed, true);
  assert.equal(Number(run('sqlite3', [database, "SELECT COUNT(*) FROM events WHERE event_type='RPExternalControllerReleased';"]).toString().trim()), 2);
  assert.equal(connected.stderr().includes(token), false); assert.equal(foreign.stderr().includes(creatorToken), false);
  assert.equal(residentA.stderr().includes(residentAToken), false); assert.equal(residentB.stderr().includes(residentBToken), false);
  t.diagnostic(`real MCP legacy+2026-07-28 stdio/runtime PASS fixture ${temp}; two service residents/Human settled shared waits plus conflicting shared speech and move windows, each scripted resident spoke twice, physical meeting delivered speech and leaving scene stopped hearing, explicit release fenced old proposals and preserved frozen receipts; live model provider not exercised`);
});
