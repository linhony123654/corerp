// Opt-in F3 acceptance driver. It creates a disposable world, runs two
// distinct external residents through real MCP stdio, and asks an explicitly
// configured chat-completions provider for each resident's own decisions.
// No production DB, existing token, generic API key or preview instance is used.
import { execFileSync, spawn } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { mkdtemp, rm } from 'node:fs/promises';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Client } from '@modelcontextprotocol/client';
import { StdioClientTransport } from '@modelcontextprotocol/client/stdio';
import { chooseLiveSpeech, liveProviderConfiguration, ownSceneForModel } from './live-provider.mjs';

const root = resolve(import.meta.dirname, '../..');
const backend = join(root, 'backend');
const entityA = 'entity_m2_agent_ada', entityB = 'entity_m2_agent_bo';

function childEnvironment(extra = {}) {
  const names = ['PATH', 'HOME', 'TMPDIR', 'XDG_CACHE_HOME', 'GOCACHE', 'GOMODCACHE', 'GOPATH', 'GOPROXY', 'GOSUMDB', 'CGO_ENABLED'];
  const inherited = Object.fromEntries(names.filter(name => process.env[name] !== undefined).map(name => [name, process.env[name]]));
  return { ...inherited, ...extra };
}

function run(command, args, options = {}) {
  return execFileSync(command, args, { cwd: backend, env: childEnvironment(), stdio: 'pipe', ...options }).toString();
}

async function freePort() {
  const server = createServer(); server.listen(0, '127.0.0.1'); await once(server, 'listening');
  const port = server.address().port;
  await new Promise(resolveClose => server.close(resolveClose));
  return port;
}

async function waitReady(origin, child) {
  for (let attempt = 0; attempt < 150; attempt++) {
    if (child.exitCode !== null || child.signalCode !== null) throw new Error('disposable Runtime exited before ready');
    try { if ((await fetch(`${origin}/readyz`, { signal: AbortSignal.timeout(1000) })).ok) return; } catch { /* startup */ }
    await new Promise(resolveDelay => setTimeout(resolveDelay, 100));
  }
  throw new Error('disposable Runtime did not become ready');
}

async function connect(origin, token) {
  const client = new Client({ name: 'corerp-live-resident-validation', version: '1.0.0' });
  const transport = new StdioClientTransport({
    command: process.execPath, args: [join(import.meta.dirname, 'index.js')],
    env: childEnvironment({ CORERP_ORIGIN: origin, CORERP_TOKEN: token }), stderr: 'pipe',
  });
  try { await client.connect(transport); } catch { await transport.close(); throw new Error('MCP resident connection failed'); }
  return client;
}

async function tool(client, name, args) {
  const result = await client.callTool({ name, arguments: args });
  if (result.isError) {
    const code = result.structuredContent?.error?.code ?? 'tool_error';
    throw new Error(`${name} failed: ${code}`);
  }
  const envelope = result.structuredContent ?? JSON.parse(result.content.find(block => block.type === 'text').text);
  return envelope.data;
}

function scalar(database, query) {
  return run('sqlite3', [database, query]).trim();
}

function oneMinuteLater(at) {
  const parsed = Date.parse(at);
  if (!Number.isFinite(parsed)) throw new Error('invalid observed world time');
  return new Date(parsed + 60_000).toISOString().replace('.000Z', 'Z');
}

async function stopOwnedProcess(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return;
  const exited = once(child, 'exit');
  child.kill('SIGTERM');
  await exited;
}

export async function validateLiveResidents(config, { retainEvidence = true } = {}) {
  const temp = await mkdtemp(join(tmpdir(), 'corerp-f3-live-'));
  const database = join(temp, 'world.db');
  const executable = join(temp, 'server'), setup = join(temp, 'setup'), controller = join(temp, 'controller');
  run('/usr/local/go/bin/go', ['build', '-o', executable, './cmd/corerp-server']);
  run('/usr/local/go/bin/go', ['build', '-o', setup, './cmd/corerp-m2']);
  run('/usr/local/go/bin/go', ['build', '-o', controller, './cmd/corerp-controller']);
  run(setup, ['-db', database, '-action', 'rp-travel-prepare']);
  run('sqlite3', [database, "INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_live_resident_a','service','Live Resident A','active'),('principal_live_resident_b','service','Live Resident B','active');"]);

  const tokens = { human: randomBytes(24).toString('hex'), operator: randomBytes(24).toString('hex'), A: randomBytes(24).toString('hex'), B: randomBytes(24).toString('hex') };
  const origin = `http://127.0.0.1:${await freePort()}`;
  const runtime = spawn(executable, ['-db', database, '-listen', new URL(origin).host], {
    env: childEnvironment({
      CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [tokens.human]: 'principal_m2_rp_player', [tokens.operator]: 'principal_operator', [tokens.A]: 'principal_live_resident_a', [tokens.B]: 'principal_live_resident_b' }),
      CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'),
      CORERP_DECISION_PROVIDER: 'deterministic', CORERP_NARRATIVE_PROVIDER: 'deterministic',
    }), stdio: 'ignore',
  });
  const clients = [];
  try {
    await waitReady(origin, runtime);
    const human = await connect(origin, tokens.human); clients.push(human);
    const A = await connect(origin, tokens.A); clients.push(A);
    const B = await connect(origin, tokens.B); clients.push(B);
    const scope = { instance_id: 'inst_m2_t09', branch_id: 'br_main' };
    const head = () => Number(scalar(database, "SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main';"));
    const binding = idempotency_key => ({ ...scope, principal_id: 'principal_operator', expected_head: head(), idempotency_key });
    for (const [entity, principal, instance] of [[entityA, 'principal_live_resident_a', 'live-resident-a'], [entityB, 'principal_live_resident_b', 'live-resident-b']]) {
      run(controller, ['-db', database, '-action', 'enroll'], { input: JSON.stringify({ binding: binding(randomUUID()), entity_id: entity, controller_principal_id: principal, controller_instance_id: instance }) });
      run(controller, ['-db', database, '-action', 'assign'], { input: JSON.stringify({ binding: binding(randomUUID()), entity_id: entity, expected_generation: 0 }) });
    }
    const residents = { A: { client: A, entity: entityA, model: config.modelA, history: [], heardSpeech: [] }, B: { client: B, entity: entityB, model: config.modelB, history: [], heardSpeech: [] } };
    for (const [role, client, expected] of [['human', human], ['A', A, entityA], ['B', B, entityB]]) {
      const worlds = await tool(client, 'corerp_worlds', {});
      if (worlds.bindings.length !== 1 || (expected && worlds.bindings[0].entity_id !== expected)) throw new Error(`${role} did not receive exactly its own control binding`);
      const discovered = worlds.bindings[0];
      const session = await tool(client, 'corerp_session_open', {
        instance_id: discovered.instance_id, branch_id: discovered.branch_id, entity_id: discovered.entity_id,
        pov: 'second_person', idempotency_key: randomUUID(),
      });
      if (role === 'human') residents.human = { client, session: session.session_id };
      else residents[role].session = session.session_id;
    }
    // Scripted *setup* only: all real model decisions occur after the two
    // external residents and Human are co-located in the same sourced place.
    for (const role of ['A', 'B']) {
      const resident = residents[role];
      const observed = await tool(resident.client, 'corerp_observe', { session_id: resident.session });
      if (observed.place_id !== 'place_m2_cafe') {
        if (!observed.reachable_places.some(place => place.place_id === 'place_m2_cafe' && place.can_move_now)) throw new Error(`${role} cannot reach fixture cafe`);
        await tool(resident.client, 'corerp_command', { operation: 'move', request: {
          session_id: resident.session, expected_cursor: observed.observation_cursor, idempotency_key: randomUUID(),
          from_place_id: observed.place_id, to_place_id: 'place_m2_cafe',
        } });
      }
    }
    const together = await tool(B, 'corerp_observe', { session_id: residents.B.session });
    if (together.place_id !== 'place_m2_cafe' || together.present_entities.length < 2) throw new Error('real co-location of Human and two residents not observed');
    const postOperator = async (route, body) => {
      const response = await fetch(`${origin}/api/v1/rp/${route}`, { method: 'POST', headers: { Authorization: `Bearer ${tokens.operator}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      if (response.status !== 200) throw new Error(`local operator ${route} failed: HTTP ${response.status}`);
      const envelope = await response.json();
      return envelope.data;
    };
    const observeAll = async () => {
      const result = {};
      for (const role of ['human', 'A', 'B']) result[role] = await tool(residents[role].client, 'corerp_observe', { session_id: residents[role].session });
      return result;
    };
    const openRound = async () => {
      const round = await postOperator('rounds/open', { binding: binding(randomUUID()), human_session_id: residents.human.session, external_session_ids: [residents.A.session, residents.B.session] });
      return round.round_id;
    };
    const settle = async (actor, round_id) => {
      for (let attempt = 0; attempt < 15; attempt++) {
        const receipt = await tool(actor.client, 'corerp_round_advance', { session_id: actor.session, round_id, budget: 10000 });
        if (receipt.status === 'settled') return receipt;
      }
      throw new Error('shared round did not settle within bounded scheduler retries');
    };
    const advanceWorldTime = async baseline => {
      for (let boundary = 0; boundary < 12; boundary++) {
        const observations = await observeAll(), round_id = await openRound();
        const horizon_world_time = oneMinuteLater(observations.human.world_time);
        for (const role of ['human', 'A', 'B']) await tool(residents[role].client, 'corerp_round_wait', { session_id: residents[role].session, round_id, horizon_world_time, idempotency_key: randomUUID() });
        await settle(residents.human, round_id);
        const now = (await tool(human, 'corerp_observe', { session_id: residents.human.session })).world_time;
        if (Date.parse(now) > Date.parse(baseline)) return;
      }
      throw new Error('scheduler did not leave prior action world-time boundary');
    };
    const readOwnHeardSpeech = async resident => {
      for (let page = 0; page < 20; page++) {
        const history = await tool(resident.client, 'corerp_events', {
          session_id: resident.session, ...(resident.eventCursor ? { cursor: resident.eventCursor } : {}), limit: 50,
        });
        for (const event of history.events) {
          for (const fact of event.facts) {
            if (fact.kind === 'speaker_said' && typeof fact.text === 'string') resident.heardSpeech.push(fact.text);
          }
        }
        resident.heardSpeech = resident.heardSpeech.slice(-3);
        resident.eventCursor = history.next_cursor;
        if (!history.more_events) return resident.heardSpeech;
      }
      throw new Error('own visible event stream exceeded the bounded acceptance window');
    };

    const decisions = [], heardFirstA = { value: false };
    for (let index = 0; index < 4; index++) {
      if (index > 0) await advanceWorldTime(decisions[index - 1].world_time);
      const role = index % 2 === 0 ? 'A' : 'B';
      const resident = residents[role], other = residents[role === 'A' ? 'B' : 'A'];
      const observed = await observeAll();
      const round_id = await openRound();
      const heardSpeech = await readOwnHeardSpeech(resident);
      const scene = ownSceneForModel(observed[role], resident.history, heardSpeech);
      const decision = await chooseLiveSpeech(config, resident.model, role, scene);
      await tool(resident.client, 'corerp_round_speech', {
        session_id: resident.session, round_id, speech_act: decision.speech_act,
        text: decision.text, idempotency_key: randomUUID(),
      });
      const horizon_world_time = oneMinuteLater(observed.human.world_time);
      for (const participant of [other, residents.human]) await tool(participant.client, 'corerp_round_wait', { session_id: participant.session, round_id, horizon_world_time, idempotency_key: randomUUID() });
      const receipt = await settle(resident, round_id);
      if (receipt.own_disposition !== 'action_accepted') throw new Error(`${role} model decision was not accepted`);
      if (!Number.isSafeInteger(receipt.event_sequence) || receipt.event_sequence < 1) throw new Error(`${role} accepted receipt has no source Event sequence`);
      const committed = JSON.parse(scalar(database, `SELECT json_object('actor_id',actor_id,'event_type',event_type,'text',json_extract(payload,'$.text')) FROM events WHERE instance_id='inst_m2_t09' AND branch_id='br_main' AND event_sequence=${receipt.event_sequence};`));
      if (committed.actor_id !== resident.entity || committed.event_type !== 'RPSpeechAccepted' || committed.text !== decision.text) {
        throw new Error(`${role} model decision did not match its authoritative speech Event`);
      }
      resident.history.push(decision.text);
      decisions.push({ role, world_time: observed.human.world_time, event_sequence: receipt.event_sequence, speech_act: decision.speech_act, provider_receipt: decision.provider_receipt });
      if (index === 0) {
        heardFirstA.value = (await readOwnHeardSpeech(residents.B)).includes(decision.text);
        if (!heardFirstA.value) throw new Error('co-located resident B did not hear A model speech');
      }
    }
    for (const [role, entity] of [['A', entityA], ['B', entityB]]) {
      const count = Number(scalar(database, `SELECT COUNT(*) FROM events WHERE instance_id='inst_m2_t09' AND branch_id='br_main' AND event_type='RPSpeechAccepted' AND actor_id='${entity}';`));
      if (count !== 2) throw new Error(`${role} did not commit exactly two model-sourced speech Events`);
    }
    return { status: 'completed', evidence_kind: 'provider_provenance_review_required', model_calls: { A: 2, B: 2 }, accepted_speech_events: { A: 2, B: 2 }, co_located_hearing: heardFirstA.value, decisions, provider_host: config.endpointHost, disposable_world: database };
  } finally {
    for (const client of clients.reverse()) { try { await client.close(); } catch { /* close remaining clients */ } }
    await stopOwnedProcess(runtime);
    if (!retainEvidence) await rm(temp, { recursive: true, force: true });
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const config = liveProviderConfiguration(process.env);
    const result = await validateLiveResidents(config);
    process.stdout.write(`${JSON.stringify(result)}\n`);
  } catch (error) {
    // Provider body, bearer token and resident prompt are never printed.
    process.stderr.write(`F3 live resident validation failed: ${error.message}\n`);
    process.exitCode = 1;
  }
}
