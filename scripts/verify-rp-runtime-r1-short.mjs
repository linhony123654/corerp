import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { once } from 'node:events';
import { readFile, mkdtemp, writeFile, access } from 'node:fs/promises';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

// A short diagnostic over explicitly authored test data. This is separate
// from the frozen real-world Golden and cannot certify its experience gate.
const root = resolve(import.meta.dirname, '..');
const args = process.argv.slice(2);
assert.equal(args.length, 2, 'usage: node scripts/verify-rp-runtime-r1-short.mjs --bin-dir /path/to/binaries');
assert.equal(args[0], '--bin-dir');
const binaries = resolve(args[1]);
for (const name of ['runtime', 'm1', 'setup', 'admin', 'preflight']) await access(join(binaries, name));
for (const key of ['CORERP_LLM_ENDPOINT', 'CORERP_LLM_API_KEY']) assert.ok(process.env[key], `${key} is required`);
const fixtureBytes = await readFile(join(root, 'docs/rp-runtime-r1/rongqing-minimal-test-fixture-2026-10-01.json'));
const fixture = JSON.parse(fixtureBytes);
assert.equal(fixture.kind, 'corerp.r1-experience-fixture.v1');
assert.equal(fixture.authority, 'test_author_only');
assert.equal(fixture.real_world_canon, 'NOT_APPROVED');
const { spec, inputs } = fixture;
const temp = await mkdtemp(join(tmpdir(), 'corerp-r1-short-'));
const db = join(temp, 'world.db'), world = 'r1-short-test-world', branch = 'br_main';
const canonical = value => Array.isArray(value) ? `[${value.map(canonical).join(',')}]` : value && typeof value === 'object' ? `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonical(value[key])}`).join(',')}}` : JSON.stringify(value);
const digest = value => createHash('sha256').update(canonical(value)).digest('hex');
const worldID = (kind, key) => `studio_${kind}_${digest(['corerp.studio-world.v1', world, kind, key])}`;
const sha = bytes => `sha256:${createHash('sha256').update(bytes).digest('hex')}`;
const cli = (name, ...args) => execFileSync(join(binaries, name), ['-db', db, ...args], { cwd: join(root, 'backend'), stdio: 'pipe' }).toString();
const rows = query => JSON.parse(execFileSync('sqlite3', ['-json', db, query]).toString() || '[]');
const head = () => rows(`SELECT head_sequence FROM branches WHERE instance_id='${world}' AND branch_id='${branch}'`)[0].head_sequence;
const creatorToken = randomBytes(24).toString('hex'), playerToken = randomBytes(24).toString('hex');
const specPath = join(temp, 'test-world.json');
await writeFile(specPath, JSON.stringify(spec), { mode: 0o600 });
const preflight = JSON.parse(execFileSync(join(binaries, 'preflight'), ['-spec', specPath]).toString());
assert.equal(preflight.rp_readiness.status, 'READY');

const packages = JSON.parse(await readFile(join(root, 'docs/rp-runtime-r1/real-rongqing-packages.json')));
const [systemPackage, narrativePackage] = ['system', 'narrative'].map(kind => packages.find(p => p.manifest.kind === kind));
cli('m1', '-action', 'inspect');
const prepared = JSON.parse(cli('setup', '-action', 'rp-travel-prepare'));
cli('admin', '-operator', 'principal_operator', '-instance', 'inst_m2_t09', '-branch', branch,
  '-target', 'principal_creator', '-purpose', 'create_world', '-status', 'active', '-expected-head', String(prepared.event_sequence), '-key', 'short-create-grant');
const server = createServer();
server.listen(0, '127.0.0.1');
await once(server, 'listening');
const port = server.address().port;
server.close(); await once(server, 'close');
const origin = `http://127.0.0.1:${port}`;
const env = { ...process.env,
  CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [creatorToken]: 'principal_creator', [playerToken]: 'principal_m2_rp_player' }),
  CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'chat_completions',
  CORERP_LLM_MODEL: 'step-5-preview', CORERP_LLM_TIMEOUT: '120s', CORERP_LLM_ATTEMPTS: '2',
  CORERP_LLM_REASONING_EFFORT: 'low', CORERP_LLM_DECISION_MAX_TOKENS: '4096', CORERP_LLM_DECISION_FORMAT: 'tool_call',
  CORERP_LLM_DISABLE_THINKING: 'false', CORERP_PROVIDER_ALLOWLIST: '', CORERP_PROVIDER_LOCAL_ALLOWLIST: '',
  CORERP_NARRATIVE_PROVIDER: 'full_prose', CORERP_NARRATIVE_ENDPOINT: process.env.CORERP_LLM_ENDPOINT,
  CORERP_NARRATIVE_MODEL: 'step-5-preview', CORERP_NARRATIVE_API_KEY: process.env.CORERP_LLM_API_KEY,
  CORERP_NARRATIVE_TIMEOUT: '90s', CORERP_NARRATIVE_ATTEMPTS: '2' };
const report = { kind: 'corerp.r1-short-diagnostic.v1', model: 'step-5-preview',
  tuning: { decision_timeout: '120s', decision_max_tokens: 4096, reasoning_effort: 'low', decision_format: 'tool_call', narrative_timeout: '90s' },
  runner_sha256: sha(await readFile(join(root, 'scripts/verify-rp-runtime-r1-short.mjs'))),
  fixture_sha256: sha(fixtureBytes), runtime_sha256: sha(await readFile(join(binaries, 'runtime'))),
  fixture_authority: fixture.authority, real_world_canon: fixture.real_world_canon,
  world, artifact_dir: temp, configuration_readiness: preflight.rp_readiness,
  samples: [], status: 'NOT VERIFIED', human_experience: 'PENDING', limits: fixture.limitations };
const reportPath = join(temp, 'short-samples.json');
const save = () => writeFile(reportPath, JSON.stringify(report, null, 2) + '\n', { mode: 0o600 });
const runtime = spawn(join(binaries, 'runtime'), ['-db', db, '-listen', `127.0.0.1:${port}`], { env, stdio: 'ignore' });
async function post(route, body, token = playerToken, expected = 200) {
  const response = await fetch(`${origin}/api/v1/${route}`, { method: 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body), signal: AbortSignal.timeout(180_000) });
  const envelope = await response.json();
  assert.equal(response.status, expected, `${route}: ${envelope.error?.code || response.status}`);
  return envelope.data;
}
try {
  let ready = false;
  for (let i = 0; i < 150; i++) {
    try { if ((await fetch(`${origin}/readyz`)).ok) { ready = true; break; } } catch { /* startup */ }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  assert.ok(ready, 'isolated runtime did not start');
  const created = await post('studio/worlds/create', { authority_instance_id: 'inst_m2_t09', authority_branch_id: branch,
    instance_id: world, idempotency_key: 'short-world', player_principal_id: 'principal_m2_rp_player',
    system_package: systemPackage, narrative_package: narrativePackage, spec }, creatorToken, 201);
  assert.equal(created.rp_readiness.status, 'READY');
  const session = await post('rp/sessions/open', { instance_id: world, branch_id: branch,
    entity_id: created.entity_id, pov: 'second_person', idempotency_key: 'short-session' });
  for (const [index, text] of inputs.entries()) {
    const view = await post('rp/observe', { session_id: session.session_id });
    const started = Date.now();
    const turn = await post('rp/turns/run', { session_id: session.session_id, text,
      expected_cursor: view.observation_cursor, idempotency_key: `short-speech-${index + 1}` });
    const decisions = rows(`SELECT e.event_sequence,m.display_name AS speaker,d.action,COALESCE(u.speech_text,'') AS text,d.event_id FROM rp_npc_decisions d JOIN materialized_entities m ON m.entity_id=d.npc_entity_id JOIN events e ON e.event_id=d.event_id LEFT JOIN rp_utterances u ON u.event_id=d.event_id WHERE d.parent_turn_id='${turn.player_turn_id}' ORDER BY e.event_sequence`);
    const receipts = rows(`SELECT phase,provider_kind,model_id,result,attempt_count,fallback_kind FROM rp_provider_calls WHERE turn_run_id='${turn.turn_run_id}' ORDER BY rowid`);
    const sample = { index: index + 1, text, duration_seconds: (Date.now() - started) / 1000,
      turn_run_id: turn.turn_run_id, world_head: head(), decisions, receipts, public_narrative: turn.narrative_lines };
    report.samples.push(sample); await save();
    console.log(JSON.stringify(sample));
    assert.equal(turn.status, 'settled');
    assert.equal(decisions.length, 1, 'short diagnostic must have exactly one responder');
    const calls = receipts.filter(call => call.phase === 'decision');
    assert.equal(calls.length, 1);
    assert.ok(calls.every(call => call.provider_kind === 'chat_completions' && call.model_id === 'step-5-preview' && call.result === 'success' && call.attempt_count > 0 && !call.fallback_kind), 'NPC decision did not succeed through the real provider');
    assert.ok(decisions[0].text, 'no public reply to review');
    assert.ok(turn.narrative_lines.join('\n').includes(text), 'accepted player speech missing');
    assert.ok(turn.narrative_lines.join('\n').includes(decisions[0].text), 'accepted NPC speech missing');
    if (index === inputs.length - 1) {
      const beforeRender = head();
      const prose = await post('rp/narrative/render', { session_id: session.session_id, turn_run_id: turn.turn_run_id });
      sample.full_prose = prose.view.lines;
      sample.prose_fallback = prose.view.fallback_reason || '';
      sample.prose_receipts = rows(`SELECT phase,provider_kind,model_id,result,attempt_count,fallback_kind FROM rp_provider_calls WHERE turn_run_id='${turn.turn_run_id}' AND phase='narrative' ORDER BY rowid`);
      await save();
      assert.equal(head(), beforeRender, 'prose changed world state');
      assert.equal(sample.prose_fallback, '', 'prose fell back');
      assert.ok(sample.full_prose.join('\n').includes(decisions[0].text), 'prose rewrote accepted NPC speech');
      assert.ok(sample.full_prose.join('\n').includes(text), 'prose rewrote accepted player speech');
      assert.ok(sample.prose_receipts.some(call => call.provider_kind === 'full_prose' && call.result === 'success' && call.attempt_count > 0 && !call.fallback_kind), 'no real prose receipt');
    }
  }
  const known = await post('rp/observe', { session_id: session.session_id });
  assert.ok(known.present_entities.some(p => p.entity_id === worldID('entity', 'jia_mu') && p.display_name === '贾母'), 'declared acquaintance did not establish public recognition');
  report.status = 'PASS: four real decisions and one real prose render; experience remains unreviewed';
} catch (error) {
  report.status = 'FAIL';
  // Local fixed assertion/route failures only; never record response bodies.
  report.failure = error?.name || 'Error';
  process.exitCode = 1;
} finally {
  await save();
  console.log(JSON.stringify({ artifact: reportPath, status: report.status, samples: report.samples.length }));
  if (runtime.exitCode === null && runtime.signalCode === null) {
    const ended = once(runtime, 'exit'); runtime.kill('SIGTERM'); await ended;
  }
}
