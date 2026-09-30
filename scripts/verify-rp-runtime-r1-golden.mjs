import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { once } from 'node:events';
import { access, readFile, mkdtemp, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

// Run against an isolated world recreated from the frozen, actual preview
// Studio declaration. This never opens the preview database or real accounts.
const root = resolve(import.meta.dirname, '..');
const base = join(root, 'docs/rp-runtime-r1');
const frozenSpecPath = join(base, 'real-rongqing-world-spec.json');
const args = process.argv.slice(2);
const specFlag = args.indexOf('--spec-file');
assert.ok(specFlag < 0 || (specFlag + 1 < args.length && !args[specFlag + 1].startsWith('--')),
  '--spec-file requires a path to a creator-authored world declaration');
const customSpecPath = specFlag < 0 ? '' : resolve(args[specFlag + 1]);
const binaryFlag = args.indexOf('--binary-dir');
assert.ok(binaryFlag < 0 || (binaryFlag + 1 < args.length && !args[binaryFlag + 1].startsWith('--')),
  '--binary-dir requires a directory with the frozen runtime and setup tools');
const binaryDir = binaryFlag < 0 ? '' : resolve(args[binaryFlag + 1]);
const allowedArgs = customSpecPath ? ['--spec-file', args[specFlag + 1]] : [];
if (binaryDir) allowedArgs.push('--binary-dir', args[binaryFlag + 1]);
if (args.includes('--after')) allowedArgs.push('--after');
assert.equal(args.length, allowedArgs.length, 'unexpected or duplicate golden replay argument');
assert.ok(!customSpecPath || !args.includes('--after'),
  'creator-authored spec is supplementary validation, not the frozen after replay');
assert.ok(!binaryDir || (!customSpecPath && !args.includes('--after')),
  'frozen old binaries may only run the frozen before replay');
const specPath = customSpecPath || frozenSpecPath;
const specBytes = await readFile(specPath);
const spec = JSON.parse(specBytes);
if (customSpecPath) {
  const worldShape = ({ acquaintances, relationships, people, ...world }) => ({
    ...world, people: people.map(({ persona, public_presentation, routine, ...person }) => person),
  });
  const frozenSpec = JSON.parse(await readFile(frozenSpecPath, 'utf8'));
  assert.deepEqual(worldShape(spec), worldShape(frozenSpec),
    'creator-authored overlay may only add persona, public_presentation, routine, acquaintances and relationships');
}
const packages = JSON.parse(await readFile(join(base, 'real-rongqing-packages.json'), 'utf8'));
const [systemPackage, narrativePackage] = ['system', 'narrative'].map(kind => packages.find(p => p.manifest.kind === kind));
assert.ok(systemPackage && narrativePackage, 'frozen actual-world packages are incomplete');
for (const key of ['CORERP_LLM_ENDPOINT', 'CORERP_LLM_MODEL', 'CORERP_LLM_API_KEY'])
  assert.ok(process.env[key], `${key} is required for comparable live-model capture`);
assert.match(process.env.CORERP_LLM_MODEL, /^(?:gemini-3\.8-flash(?:-high)?|step-5-preview)$/,
  'golden replay requires the recorded Gemini 3.8 Flash or Step 5 Preview model');

const label = customSpecPath ? 'authored' : args.includes('--after') ? 'after' : 'before';
const temp = await mkdtemp(join(tmpdir(), `corerp-rp-runtime-r1-${label}-`));
const db = join(temp, 'world.db');
const world = 'rp-runtime-r1-golden-world', branch = 'br_main';
const creatorToken = randomBytes(24).toString('hex'), playerToken = randomBytes(24).toString('hex');
const canonical = value => Array.isArray(value) ? `[${value.map(canonical).join(',')}]` : value && typeof value === 'object' ? `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonical(value[key])}`).join(',')}}` : JSON.stringify(value);
const digest = value => createHash('sha256').update(canonical(value)).digest('hex');
const worldID = (kind, key) => `studio_${kind}_${digest(['corerp.studio-world.v1', world, kind, key])}`;
const run = (command, args, cwd = root) => execFileSync(command, args, { cwd, stdio: 'pipe' }).toString();
const sql = query => run('sqlite3', ['-json', db, query]).trim();
const rows = query => JSON.parse(sql(query) || '[]');
const head = () => rows(`SELECT head_sequence FROM branches WHERE instance_id='${world}' AND branch_id='${branch}'`)[0].head_sequence;
const sha = input => `sha256:${createHash('sha256').update(input).digest('hex')}`;

async function availablePort() {
  const server = createServer();
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const port = server.address().port;
  server.close();
  await once(server, 'close');
  return port;
}

const port = await availablePort(), origin = `http://127.0.0.1:${port}`;
async function ready() {
  for (let i = 0; i < 150; i++) {
    try { if ((await fetch(`${origin}/readyz`)).ok) return; } catch { /* startup */ }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error('isolated golden runtime did not start');
}
async function post(route, token, body, expected = 200) {
  const response = await fetch(`${origin}/api/v1/${route}`, {
    method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body), signal: AbortSignal.timeout(300_000),
  });
  const envelope = await response.json();
  assert.equal(response.status, expected, `${route}: ${envelope.error?.code || response.status}`);
  return envelope.data;
}
function path(from, to) {
  const queue = [[from]], seen = new Set([from]);
  while (queue.length) {
    const route = queue.shift(), last = route.at(-1);
    if (last === to) return route;
    for (const edge of spec.links) {
      const next = edge.from === last ? edge.to : edge.to === last ? edge.from : '';
      if (next && !seen.has(next)) { seen.add(next); queue.push([...route, next]); }
    }
  }
  throw new Error(`no frozen world path ${from} -> ${to}`);
}

let runtime;
try {
  const binaries = binaryDir || temp;
  for (const [name, program] of [['runtime', 'corerp-server'], ['m1', 'corerp-m1'], ['setup', 'corerp-m2'], ['admin', 'corerp-admin']]) {
    if (binaryDir) await access(join(binaries, name));
    else run('/usr/local/go/bin/go', ['build', '-buildvcs=false', '-o', join(binaries, name), `./cmd/${program}`], join(root, 'backend'));
  }
  run(join(binaries, 'm1'), ['-db', db, '-action', 'inspect']);
  const prepared = JSON.parse(run(join(binaries, 'setup'), ['-db', db, '-action', 'rp-travel-prepare']));
  run(join(binaries, 'admin'), ['-db', db, '-operator', 'principal_operator', '-instance', 'inst_m2_t09', '-branch', branch,
    '-target', 'principal_creator', '-purpose', 'create_world', '-status', 'active', '-expected-head', String(prepared.event_sequence), '-key', 'r1-golden-create-grant']);
  const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [creatorToken]: 'principal_creator', [playerToken]: 'principal_m2_rp_player' }),
    CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'chat_completions',
    CORERP_NARRATIVE_PROVIDER: 'full_prose', CORERP_NARRATIVE_ENDPOINT: process.env.CORERP_LLM_ENDPOINT,
    CORERP_NARRATIVE_MODEL: process.env.CORERP_LLM_MODEL, CORERP_NARRATIVE_API_KEY: process.env.CORERP_LLM_API_KEY,
    CORERP_PROVIDER_ALLOWLIST: '', CORERP_PROVIDER_LOCAL_ALLOWLIST: '',
    CORERP_LLM_TIMEOUT: process.env.CORERP_LLM_TIMEOUT || '60s', CORERP_LLM_ATTEMPTS: '2', CORERP_NARRATIVE_TIMEOUT: '90s', CORERP_NARRATIVE_ATTEMPTS: '2' };
  runtime = spawn(join(binaries, 'runtime'), ['-db', db, '-listen', `127.0.0.1:${port}`], { env, stdio: 'ignore' });
  await ready();
  const created = await post('studio/worlds/create', creatorToken, { authority_instance_id: 'inst_m2_t09', authority_branch_id: branch,
    instance_id: world, idempotency_key: 'r1-golden-world', player_principal_id: 'principal_m2_rp_player',
    system_package: systemPackage, narrative_package: narrativePackage, spec }, 201);
  assert.equal(created.entity_id, worldID('entity', 'player'));
  const session = await post('rp/sessions/open', playerToken, { instance_id: world, branch_id: branch,
    entity_id: created.entity_id, pov: 'second_person', idempotency_key: 'r1-golden-play' });
  let place = spec.people.find(p => p.player).place;
  const samples = [];
  let serial = 0;
  async function travel(to) {
    for (const next of path(place, to).slice(1)) {
      const view = await post('rp/observe', playerToken, { session_id: session.session_id });
      await post('rp/actions/move', playerToken, { session_id: session.session_id,
        from_place_id: worldID('place', place), to_place_id: worldID('place', next),
        expected_cursor: view.observation_cursor, idempotency_key: `r1-move-${++serial}` });
      place = next;
    }
  }
  async function speech(scene, text) {
    const view = await post('rp/observe', playerToken, { session_id: session.session_id });
    const turn = await post('rp/turns/run', playerToken, { session_id: session.session_id, text,
      expected_cursor: view.observation_cursor, idempotency_key: `r1-speech-${++serial}` });
    assert.equal(turn.status, 'settled', `${scene}: unsettled turn`);
    const decisions = rows(`SELECT e.event_sequence,m.display_name AS speaker,d.action,COALESCE(u.speech_text,'') AS text,d.event_id FROM rp_npc_decisions d JOIN materialized_entities m ON m.entity_id=d.npc_entity_id JOIN events e ON e.event_id=d.event_id LEFT JOIN rp_utterances u ON u.event_id=d.event_id WHERE d.parent_turn_id='${turn.player_turn_id}' ORDER BY e.event_sequence`);
    const receipts = rows(`SELECT phase,provider_kind,model_id,result,attempt_count,fallback_kind FROM rp_provider_calls WHERE turn_run_id='${turn.turn_run_id}' ORDER BY rowid`);
    const sample = { scene, place, text, turn_run_id: turn.turn_run_id, world_head: head(), decisions, receipts,
      original_narrative: turn.narrative_lines };
    samples.push(sample);
    console.log(JSON.stringify({ scene, text, decisions: decisions.map(d => ({ speaker: d.speaker, action: d.action, text: d.text })), receipts: receipts.map(r => ({ phase: r.phase, result: r.result, model: r.model_id })) }));
    return sample;
  }
  await speech('G6', '你们好呀');
  await speech('G6', '你们叫什么名字呀');
  await travel('fengjieyuan');
  await speech('G7', '来看看凤姐呗');
  await speech('G7', '那就谢谢凤姐了');
  await speech('G7', '你刚才已经把茶递给我了吗？');
  await speech('G4', '我这几日总惦记着姐姐，前儿你说的那桩事，可有下文了？');
  await speech('G5', '凤姐姐天天都好忙');
  await speech('G5', '那凤姐你先处理');
  await travel('rongqingtang');
  await speech('G2', '有谁呀');
  await speech('G2', '你是谁');
  await speech('G2', '有人吗');
  await speech('G2', '你是谁呀');
  await speech('G3', '想你了');
  await speech('G1', '有人吗');
  const g1 = await speech('G1', '找老祖宗呀');
  const prose = await post('rp/narrative/render', playerToken, { session_id: session.session_id, turn_run_id: g1.turn_run_id });
  samples.push({ scene: 'G8', source_turn: g1.turn_run_id, narrative: prose.view.lines, fallback_reason: prose.view.fallback_reason || '' });
  const artifact = { label, model: process.env.CORERP_LLM_MODEL,
    model_config: { decision_timeout: env.CORERP_LLM_TIMEOUT, decision_max_tokens: env.CORERP_LLM_DECISION_MAX_TOKENS || '1024',
      reasoning_effort: env.CORERP_LLM_REASONING_EFFORT || '', decision_attempts: env.CORERP_LLM_ATTEMPTS,
      narrative_timeout: env.CORERP_NARRATIVE_TIMEOUT, narrative_attempts: env.CORERP_NARRATIVE_ATTEMPTS },
    runtime_sha256: sha(await readFile(join(binaries, 'runtime'))), world_spec_sha256: sha(specBytes),
    world_packages_sha256: sha(await readFile(join(base, 'real-rongqing-packages.json'))), world, world_head: head(), samples };
  await writeFile(join(temp, 'golden-samples.json'), JSON.stringify(artifact, null, 2), { mode: 0o600 });
  console.log(JSON.stringify({ golden: label, artifact: join(temp, 'golden-samples.json'), scenes: samples.length, world_head: head() }));
} finally {
  if (runtime && runtime.exitCode === null && runtime.signalCode === null) {
    const ended = once(runtime, 'exit'); runtime.kill('SIGTERM'); await ended;
  }
}
