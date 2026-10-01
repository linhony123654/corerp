import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { once } from 'node:events';
import { readFile, mkdtemp, writeFile, access } from 'node:fs/promises';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

// Paired product capture on an explicitly authored READY world. Neither the
// old frozen Golden nor the preview DB is opened or overwritten.
const root = resolve(import.meta.dirname, '..');
const args = process.argv.slice(2);
assert.ok([4, 7].includes(args.length), 'usage: --bin-dir /path --label before|after [--scenes P2,P3,P6 --skip-render]');
assert.equal(args[0], '--bin-dir'); assert.equal(args[2], '--label');
const binaries = resolve(args[1]), label = args[3];
assert.ok(['before', 'after'].includes(label));
const sliceIDs = args.length === 7 ? args[5].split(',') : null;
if (sliceIDs) {
  assert.equal(args[4], '--scenes'); assert.equal(args[6], '--skip-render');
  assert.ok(sliceIDs.length > 0 && new Set(sliceIDs).size === sliceIDs.length);
}
for (const name of ['runtime', 'm1', 'setup', 'admin', 'preflight']) await access(join(binaries, name));
for (const key of ['CORERP_LLM_ENDPOINT', 'CORERP_LLM_API_KEY']) assert.ok(process.env[key], `${key} is required`);
const fixturePath = join(root, 'docs/rp-runtime-r1/product-golden-scenes-2026-10-01.json');
const fixtureBytes = await readFile(fixturePath), fixture = JSON.parse(fixtureBytes);
const scenes = sliceIDs ? fixture.scenes.filter(scene => sliceIDs.includes(scene.id)) : fixture.scenes;
if (sliceIDs) assert.equal(scenes.length, sliceIDs.length, 'unknown frozen scene ID');
const specBytes = await readFile(join(root, fixture.spec_file)), spec = JSON.parse(specBytes);
const sha = value => createHash('sha256').update(value).digest('hex');
assert.equal(sha(specBytes), fixture.spec_sha256, 'authored canon changed after the comparison was frozen');
const packagesBytes = await readFile(join(root, fixture.packages_file));
const packages = JSON.parse(packagesBytes);
const [systemPackage, narrativePackage] = ['system', 'narrative'].map(kind => packages.find(p => p.manifest.kind === kind));
assert.ok(systemPackage && narrativePackage);
const temp = await mkdtemp(join(tmpdir(), `corerp-product-golden-${label}-`));
const db = join(temp, 'world.db'), world = 'r1-product-golden-world', branch = 'br_main';
const canonical = value => Array.isArray(value) ? `[${value.map(canonical).join(',')}]` : value && typeof value === 'object' ? `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonical(value[key])}`).join(',')}}` : JSON.stringify(value);
const worldID = (kind, key) => `studio_${kind}_${sha(canonical(['corerp.studio-world.v1', world, kind, key]))}`;
const cli = (name, ...argv) => execFileSync(join(binaries, name), ['-db', db, ...argv], { cwd: join(root, 'backend'), stdio: 'pipe' }).toString();
const rows = query => JSON.parse(execFileSync('sqlite3', ['-json', db, query]).toString() || '[]');
const counts = () => ({head: rows(`SELECT head_sequence FROM branches WHERE instance_id='${world}' AND branch_id='${branch}'`)[0].head_sequence, events: rows(`SELECT COUNT(*) AS n FROM events WHERE instance_id='${world}'`)[0].n, utterances: rows('SELECT COUNT(*) AS n FROM rp_utterances')[0].n, decisions: rows('SELECT COUNT(*) AS n FROM rp_npc_decisions')[0].n});
const preflight = JSON.parse(execFileSync(join(binaries, 'preflight'), ['-spec', join(root, fixture.spec_file)]).toString());
assert.equal(preflight.rp_readiness.status, 'READY');
const creatorToken = randomBytes(24).toString('hex'), playerToken = randomBytes(24).toString('hex');
cli('m1', '-action', 'inspect');
const prepared = JSON.parse(cli('setup', '-action', 'rp-travel-prepare'));
cli('admin', '-operator', 'principal_operator', '-instance', 'inst_m2_t09', '-branch', branch, '-target', 'principal_creator', '-purpose', 'create_world', '-status', 'active', '-expected-head', String(prepared.event_sequence), '-key', 'product-create-grant');
const listener = createServer(); listener.listen(0, '127.0.0.1'); await once(listener, 'listening');
const port = listener.address().port; listener.close(); await once(listener, 'close');
const origin = `http://127.0.0.1:${port}`;
const config = fixture.model_config;
const env = {...process.env,
  CORERP_AUTH_TOKENS_JSON: JSON.stringify({[creatorToken]: 'principal_creator', [playerToken]: 'principal_m2_rp_player'}),
  CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'chat_completions',
  CORERP_LLM_MODEL: fixture.model, CORERP_LLM_TIMEOUT: config.decision_timeout,
  CORERP_LLM_ATTEMPTS: String(config.decision_attempts), CORERP_LLM_DECISION_MAX_TOKENS: String(config.decision_max_tokens),
  CORERP_LLM_DECISION_FORMAT: config.decision_format, CORERP_LLM_REASONING_EFFORT: config.reasoning_effort,
  CORERP_LLM_DISABLE_THINKING: String(config.disable_thinking),
  CORERP_NARRATIVE_PROVIDER: 'full_prose', CORERP_NARRATIVE_ENDPOINT: process.env.CORERP_LLM_ENDPOINT,
  CORERP_NARRATIVE_MODEL: fixture.model, CORERP_NARRATIVE_API_KEY: process.env.CORERP_LLM_API_KEY,
  CORERP_NARRATIVE_TIMEOUT: config.narrative_timeout, CORERP_NARRATIVE_ATTEMPTS: String(config.narrative_attempts),
  CORERP_NARRATIVE_REASONING_EFFORT: config.reasoning_effort, CORERP_NARRATIVE_DISABLE_THINKING: String(config.disable_thinking),
  CORERP_PROVIDER_ALLOWLIST: '', CORERP_PROVIDER_LOCAL_ALLOWLIST: '', CORERP_DEBUG_LLM: ''};
const report = {kind: 'corerp.product-golden-capture.v1', label, model: fixture.model, model_config: config,
  runner_sha256: sha(await readFile(import.meta.filename)), fixture_sha256: sha(fixtureBytes),
  spec_sha256: sha(specBytes), packages_sha256: sha(packagesBytes), runtime_sha256: sha(await readFile(join(binaries, 'runtime'))),
  world, artifact_dir: temp, readiness: preflight.rp_readiness, samples: [], restarts: [], renders: [],
  status: 'RUNNING', human_experience: 'PENDING', limits: fixture.limits};
if (sliceIDs) Object.assign(report, {scope: 'targeted accuracy slice; not the full Golden or original32', selected_scene_ids: scenes.map(scene => scene.id), optional_real_render: 'SKIPPED: focus on NPC decisions'});
const reportPath = join(temp, 'product-golden.json');
const save = () => writeFile(reportPath, JSON.stringify(report, null, 2) + '\n', {mode: 0o600});
let runtime, session, serial = 0, place = spec.people.find(p => p.player).place;
async function start() {
  runtime = spawn(join(binaries, 'runtime'), ['-db', db, '-listen', `127.0.0.1:${port}`], {env, stdio: 'ignore'});
  for (let i = 0; i < 150; i++) {
    try { if ((await fetch(`${origin}/readyz`)).ok) return; } catch { /* startup */ }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error('isolated runtime did not start');
}
async function stop() {
  if (runtime && runtime.exitCode === null && runtime.signalCode === null) {
    const ended = once(runtime, 'exit'); runtime.kill('SIGTERM'); await ended;
  }
}
async function post(route, body, token = playerToken, expected = 200) {
  const response = await fetch(`${origin}/api/v1/${route}`, {method: 'POST', headers: {Authorization: `Bearer ${token}`, 'Content-Type': 'application/json'}, body: JSON.stringify(body), signal: AbortSignal.timeout(600_000)});
  const envelope = await response.json();
  assert.equal(response.status, expected, `${route}: ${envelope.error?.code || response.status}`);
  return envelope.data;
}
function path(from, to) {
  const queue = [[from]], seen = new Set([from]);
  while (queue.length) {
    const route = queue.shift(), last = route.at(-1); if (last === to) return route;
    for (const edge of spec.links) {
      const next = edge.from === last ? edge.to : edge.to === last ? edge.from : '';
      if (next && !seen.has(next)) {seen.add(next); queue.push([...route, next]);}
    }
  }
  throw new Error('authored destination is unreachable');
}
async function travel(to) {
  for (const next of path(place, to).slice(1)) {
    const observation = await post('rp/observe', {session_id: session.session_id});
    await post('rp/actions/move', {session_id: session.session_id, from_place_id: worldID('place', place), to_place_id: worldID('place', next), expected_cursor: observation.observation_cursor, idempotency_key: `product-move-${++serial}`});
    place = next;
  }
}
async function displayed(turnID) {
  const observation = await post('rp/observe', {session_id: session.session_id});
  const turns = observation.recent_turns || [];
  const turn = turns.find(t => t.turn_run_id === turnID);
  assert.ok(turn, 'settled turn is missing from history');
  return turn;
}
try {
  await start();
  const created = await post('studio/worlds/create', {authority_instance_id: 'inst_m2_t09', authority_branch_id: branch, instance_id: world, idempotency_key: 'product-world', player_principal_id: 'principal_m2_rp_player', system_package: systemPackage, narrative_package: narrativePackage, spec}, creatorToken, 201);
  assert.equal(created.rp_readiness.status, 'READY');
  session = await post('rp/sessions/open', {instance_id: world, branch_id: branch, entity_id: created.entity_id, pov: 'second_person', idempotency_key: 'product-session'});
  for (const scene of scenes) {
    await travel(scene.place);
    for (const text of scene.inputs) {
      const observation = await post('rp/observe', {session_id: session.session_id}), started = Date.now();
      const turn = await post('rp/turns/run', {session_id: session.session_id, text, expected_cursor: observation.observation_cursor, idempotency_key: `product-turn-${++serial}`});
      const decisions = rows(`SELECT e.event_sequence,m.display_name AS speaker,d.action,COALESCE(u.speech_text,'') AS text,d.event_id FROM rp_npc_decisions d JOIN materialized_entities m ON m.entity_id=d.npc_entity_id JOIN events e ON e.event_id=d.event_id LEFT JOIN rp_utterances u ON u.event_id=d.event_id WHERE d.parent_turn_id='${turn.player_turn_id}' ORDER BY e.event_sequence`);
      const receipts = rows(`SELECT phase,provider_kind,model_id,result,attempt_count,fallback_kind FROM rp_provider_calls WHERE turn_run_id='${turn.turn_run_id}' ORDER BY rowid`);
      const sample = {index: report.samples.length + 1, scene: scene.id, text, duration_seconds: (Date.now() - started) / 1000, turn_run_id: turn.turn_run_id, world: counts(), decisions, receipts, narrative: turn.narrative_lines, composition_version: turn.composition_version || '', fact_groups: turn.fact_groups || [], warnings: turn.warnings || []};
      report.samples.push(sample); await save();
      console.log(JSON.stringify({index: sample.index, scene: scene.id, seconds: sample.duration_seconds, decisions: decisions.map(({speaker, action, text}) => ({speaker, action, text})), warnings: sample.warnings}));
      assert.equal(turn.status, 'settled');
      const decisionCalls = receipts.filter(r => r.phase === 'decision');
      assert.ok(decisionCalls.length > 0 && decisionCalls.every(r => r.provider_kind === 'chat_completions' && r.model_id === fixture.model && r.result === 'success' && r.attempt_count > 0 && !r.fallback_kind), 'turn has a missing/failed real decision receipt');
      const shown = await displayed(turn.turn_run_id);
      assert.deepEqual(shown.narrative_lines, turn.narrative_lines, 'history changed the settled public output');
    }
    if (scene.id === 'P7' || sliceIDs && scene === scenes.at(-1)) {
      const last = report.samples.at(-1), before = counts(), calls = rows('SELECT COUNT(*) AS n FROM rp_provider_calls')[0].n;
      await stop(); await start();
      const shown = await displayed(last.turn_run_id);
      assert.deepEqual(shown.narrative_lines, last.narrative);
      assert.deepEqual(counts(), before, 'restart/history changed committed effects');
      assert.equal(rows('SELECT COUNT(*) AS n FROM rp_provider_calls')[0].n, calls, 'history reconnect made a new model call');
      report.restarts.push({after_scene: scene.id, turn_run_id: last.turn_run_id, status: 'PASS', world: before}); await save();
    }
  }
  if (!sliceIDs) {
  const last = report.samples.at(-1), before = counts();
  const render = await post('rp/narrative/render', {session_id: session.session_id, turn_run_id: last.turn_run_id, style_override: {narrative_density: 'standard', verbosity: 'normal', full_prose: true}});
  const proseReceipts = rows(`SELECT phase,provider_kind,model_id,result,attempt_count,fallback_kind FROM rp_provider_calls WHERE turn_run_id='${last.turn_run_id}' AND phase='narrative' ORDER BY rowid`);
  report.renders.push({source_turn: last.turn_run_id, narrative: render.view.lines, event_ids: render.view.event_ids, fact_groups: render.view.fact_groups || [], composition_version: render.view.composition_version || '', fallback: render.view.fallback_reason || '', receipts: proseReceipts}); await save();
  assert.deepEqual(counts(), before, 'presentation render mutated the committed world');
  assert.ok(!render.view.fallback_reason && proseReceipts.some(r => r.provider_kind === 'full_prose' && r.result === 'success' && r.attempt_count > 0 && !r.fallback_kind), 'optional real composition render failed');
  }
  report.status = 'PASS: real authored product capture and persistence checks; human experience pending';
} catch (error) {
  report.status = 'FAIL'; report.failure_kind = error?.name || 'Error';
  report.failure_message = error?.code || 'capture assertion or runtime request failed'; process.exitCode = 1;
} finally {
  await stop(); await save();
  console.log(JSON.stringify({artifact: reportPath, status: report.status, samples: report.samples.length}));
}
