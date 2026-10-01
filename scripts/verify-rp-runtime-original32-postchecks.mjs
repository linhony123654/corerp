import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { once } from 'node:events';
import { readFile, realpath, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { chromium } from 'playwright';
import { createServer as createViteServer } from 'vite';

// Resume only persistence/UI postchecks. Never repeat the original model run,
// create another failure turn, regenerate prose, or rewrite its FAIL evidence.
const flag = process.argv.indexOf('--artifact');
assert.ok(flag >= 0 && process.argv[flag + 1], 'usage: node scripts/verify-rp-runtime-original32-postchecks.mjs --artifact /tmp/corerp-rp3-npc-...');
const artifact = await realpath(resolve(process.argv[flag + 1]));
assert.match(artifact, /^\/tmp\/corerp-rp3-npc-[^/]+$/, 'only disposable original32 artifacts are permitted');
const root = resolve(import.meta.dirname, '..');
const db = join(artifact, 'world.db');
const binary = await realpath(join(artifact, 'runtime'));
const samplesBytes = await readFile(join(artifact, 'npc-samples.json'));
const captured = JSON.parse(samplesBytes);
assert.equal(captured.world, 'rp9-rongqing-world');
assert.equal(captured.samples.length, 32);
for (const sample of captured.samples) {
  assert.equal(sample.replies.length, 3, `original turn ${sample.index} missing decisions`);
  assert.equal(sample.calls.length, 3);
  assert.ok(sample.calls.every(call => call.result === 'success' && call.attempt_count > 0));
  assert.equal(sample.playNarrativeReceipt.provider, 'full_prose');
  assert.equal(sample.playNarrativeReceipt.result, 'success');
  assert.ok(sample.playNarrativeReceipt.attempts > 0 && !sample.playNarrativeReceipt.fallback);
}
const sql = query => execFileSync('sqlite3', ['-readonly', '-json', db, query], { encoding: 'utf8' }).trim();
const rows = query => JSON.parse(sql(query) || '[]');
const quote = value => `'${value.replaceAll("'", "''")}'`;
const [failure] = rows(`SELECT r.*,u.speech_text,s.principal_id,s.instance_id,s.branch_id,s.controlled_entity_id,e.event_sequence AS speech_sequence,e.payload AS speech_payload FROM rp_turn_runs r JOIN rp_utterances u ON u.turn_id=r.player_turn_id JOIN rp_sessions s ON s.session_id=r.session_id JOIN events e ON e.event_id=r.player_event_id WHERE s.instance_id='rp9-rongqing-world' AND u.speech_text='你们现在还能听见我吗？' ORDER BY r.rowid DESC LIMIT 1`);
assert.ok(failure, 'original failed-provider speech missing');
const heard = JSON.parse(failure.speech_payload).listener_ids;
assert.ok(heard.length > 0);
assert.deepEqual([...heard].sort(), JSON.parse(failure.listener_ids_json).sort());
const receipts = rows(`SELECT npc_entity_id,result,fallback_kind,attempt_count FROM rp_provider_calls WHERE turn_run_id=${quote(failure.turn_run_id)} AND phase='decision'`);
assert.deepEqual(receipts.map(r => r.npc_entity_id).sort(), [...heard].sort());
assert.ok(receipts.every(r => r.result !== 'success' && r.fallback_kind === 'silence' && r.attempt_count > 0));
const silence = rows(`SELECT npc_entity_id,action FROM rp_npc_decisions WHERE parent_turn_id=${quote(failure.player_turn_id)}`);
assert.deepEqual(silence.map(r => r.npc_entity_id).sort(), [...heard].sort());
assert.ok(silence.every(r => r.action === 'silence'));
const heardObservations = rows(`SELECT observer_agent_id FROM observation_records WHERE source_event_id=${quote(failure.player_event_id)} AND json_extract(claim_payload,'$.claim_type')='speaker_said'`);
assert.deepEqual(heardObservations.map(r => r.observer_agent_id).sort(), [...heard].sort());
const [lastOriginalSpeech] = rows(`SELECT e.payload FROM rp_turn_runs r JOIN events e ON e.event_id=r.player_event_id JOIN rp_utterances u ON u.turn_id=r.player_turn_id WHERE r.session_id=${quote(failure.session_id)} AND u.speech_text=${quote(captured.samples.at(-1).speech)}`);
assert.ok(lastOriginalSpeech);
const originalHeard = JSON.parse(lastOriginalSpeech.payload).listener_ids;
assert.equal(originalHeard.length, 3);
assert.ok(heard.every(actor => originalHeard.includes(actor)));
const absent = originalHeard.filter(actor => !heard.includes(actor));
assert.equal(absent.length, 1, 'this artifact must have exactly one departed original listener');
const departures = rows(`SELECT e.event_id,e.event_sequence,e.actor_id,m.display_name AS actor_name,e.payload,src.display_name AS from_place_name,dst.display_name AS to_place_name,p.place_id AS final_place_id,p.last_event_sequence AS position_sequence FROM events e JOIN materialized_entities m ON m.entity_id=e.actor_id JOIN agent_places src ON src.place_id=json_extract(e.payload,'$.from_place_id') JOIN agent_places dst ON dst.place_id=json_extract(e.payload,'$.to_place_id') JOIN agent_positions p ON p.agent_id=e.actor_id WHERE e.instance_id=${quote(failure.instance_id)} AND e.branch_id=${quote(failure.branch_id)} AND e.actor_id=${quote(absent[0])} AND e.event_type='RPNPCMoved' AND json_extract(e.payload,'$.action')='leave' AND e.event_sequence<${failure.speech_sequence} ORDER BY e.event_sequence DESC LIMIT 1`);
assert.equal(departures.length, 1, 'missing exact departed listener movement');
const departed = departures[0], move = JSON.parse(departed.payload);
assert.equal(departed.actor_name, '袭人');
assert.equal(departed.from_place_name, '荣庆堂');
assert.equal(departed.to_place_name, '庭院小径');
assert.equal(move.from_place_id, JSON.parse(failure.speech_payload).place_id);
assert.notEqual(move.to_place_id, move.from_place_id);
assert.equal(departed.final_place_id, move.to_place_id);
assert.equal(departed.position_sequence, departed.event_sequence, 'later position update invalidates departure proof');
assert.ok(departed.position_sequence < failure.speech_sequence);
const [selection] = rows(`SELECT s.turn_run_id,s.render_id,r.lines_json FROM rp_narrative_selections s JOIN rp_narrative_renders r ON r.render_id=s.render_id WHERE s.turn_run_id IN (SELECT turn_run_id FROM rp_turn_runs WHERE session_id=${quote(failure.session_id)}) ORDER BY s.selected_at_utc DESC LIMIT 1`);
assert.ok(selection, 'original previous-turn regeneration selection missing');
const selectedLines = JSON.parse(selection.lines_json);
const counters = () => rows(`SELECT (SELECT COUNT(*) FROM events WHERE instance_id=${quote(failure.instance_id)} AND branch_id=${quote(failure.branch_id)}) AS events,(SELECT COUNT(*) FROM rp_utterances WHERE session_id=${quote(failure.session_id)}) AS utterances,(SELECT COUNT(*) FROM rp_provider_calls WHERE session_id=${quote(failure.session_id)}) AS provider_calls,(SELECT head_sequence FROM branches WHERE instance_id=${quote(failure.instance_id)} AND branch_id=${quote(failure.branch_id)}) AS head`)[0];
const before = counters();
assert.equal(rows(`SELECT COUNT(*) AS n FROM rp_utterances WHERE session_id=${quote(failure.session_id)} AND speaker_entity_id=${quote(failure.controlled_entity_id)} AND speech_text=${quote(failure.speech_text)}`)[0].n, 1);
const token = randomBytes(24).toString('hex');
const cursorSecret = randomBytes(32).toString('hex');
const runtimeOrigin = 'http://127.0.0.1:4438', playOrigin = 'http://127.0.0.1:4439';
// Drop every inherited provider setting, including narrative/interaction keys.
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !/^CORERP_/.test(key)));
Object.assign(env, { CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [token]: failure.principal_id }), CORERP_CURSOR_SECRET: cursorSecret, CORERP_DECISION_PROVIDER: 'deterministic', CORERP_NARRATIVE_PROVIDER: 'deterministic', CORERP_PROVIDER_ALLOWLIST: '', CORERP_PROVIDER_LOCAL_ALLOWLIST: '' });
let runtime, vite, browser;
async function stop() {
  if (runtime && runtime.exitCode === null && runtime.signalCode === null) { const done = once(runtime, 'exit'); runtime.kill('SIGTERM'); await done; }
}
async function start() {
  runtime = spawn(binary, ['-db', db, '-listen', '127.0.0.1:4438'], { env, stdio: ['ignore', 'ignore', 'pipe'] });
  let stderr = ''; runtime.stderr.on('data', data => { stderr += data; });
  for (let i = 0; i < 100; i++) {
    if (runtime.exitCode !== null) throw new Error(`frozen runtime failed: ${stderr}`);
    try { if ((await fetch(`${runtimeOrigin}/readyz`)).ok) return; } catch { /* startup */ }
    await new Promise(done => setTimeout(done, 100));
  }
  throw new Error('frozen runtime did not become ready');
}
async function post(route, body) {
  const response = await fetch(`${runtimeOrigin}/api/v1/${route}`, { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  const envelope = await response.json();
  assert.equal(response.status, 200, `${route}: ${envelope.error?.code || response.status}`);
  return envelope.data;
}
function checkHistory(observation) {
  const previous = observation.recent_turns.find(t => t.turn_run_id === selection.turn_run_id);
  assert.equal(previous?.render_id, selection.render_id, 'restart/history lost selected previous regeneration');
  assert.deepEqual(previous.narrative_lines, selectedLines);
  assert.ok(observation.recent_turns.some(t => t.turn_run_id === failure.turn_run_id), 'history lost failure turn');
}
const receipt = { kind: 'corerp.original32-postchecks.v1', original_status: 'FAIL at fixed-three failure receipt assertion; preserved', status: 'NOT VERIFIED', artifact, runtime_sha256: createHash('sha256').update(await readFile(binary)).digest('hex'), samples_sha256: createHash('sha256').update(samplesBytes).digest('hex'), original_live_turns: 32, original_live_decisions: 96, actual_model_calls_added: 0, failure_turn_id: failure.turn_run_id, frozen_heard_actors: heard, departure_evidence: departures, canonical_restore_branch: { status: 'N/A', reason: 'Original --rongqing --prose rejects --persist; persisted is assigned only inside --persist && index===0, so its canonical restore branch was not entered.' }, selected_previous_turn: selection.turn_run_id, selected_render_id: selection.render_id, before };
try {
  await start();
  checkHistory(await post('rp/observe', { session_id: failure.session_id }));
  // Retry precisely the accepted failure request: idempotence must reuse the
  // failed decision outcomes, even with deterministic defaults after restart.
  const { principal_id, ...request } = JSON.parse(failure.request_json);
  const replay = await post('rp/turns/run', request);
  assert.equal(replay.turn_run_id, failure.turn_run_id);
  const replayFailures = replay.provider_calls.filter(r => r.phase === 'decision');
  assert.equal(replayFailures.length, heard.length);
  assert.ok(replayFailures.every(r => r.result !== 'success' && r.fallback_kind === 'silence'));
  assert.deepEqual(counters(), before, 'idempotent replay changed world/speech/provider calls');
  vite = await createViteServer({ root, server: { host: '127.0.0.1', port: 4439, strictPort: true, proxy: { '/api': runtimeOrigin } } }); await vite.listen();
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' });
  await context.addInitScript(({ session }) => localStorage.setItem('corerp.play.v1', JSON.stringify({ session, openKey: 'original32-postchecks', pending: null })), { session: failure.session_id });
  await context.route('**/*', route => new URL(route.request().url()).origin === playOrigin ? route.continue() : route.abort());
  const page = await context.newPage(), errors = [];
  page.on('pageerror', error => errors.push(error.message));
  page.setDefaultTimeout(30_000);
  async function openPlay() {
    await page.goto(playOrigin);
    await page.getByLabel('玩家访问凭证').fill(token);
    await page.getByRole('button', { name: '继续这段生活' }).click();
    await page.getByRole('heading', { name: '荣庆堂' }).waitFor();
    await page.locator('.turn.narrator').last().getByText('本轮人物模型调用失败或超时', { exact: false }).waitFor();
    assert.ok((await page.locator('.reading').innerText()).includes(failure.speech_text));
    assert.ok((await page.locator('.reading').innerText()).includes(captured.samples.at(-1).speech), 'restart lost final original32 player speech');
    const selected = page.locator(`[id="turn-${selection.turn_run_id}"]`);
    assert.ok((await selected.innerText()).includes(selectedLines[0]), 'Play lost selected previous prose');
  }
  await openPlay();
  await page.screenshot({ path: join(artifact, 'original32-postchecks-mobile.png'), animations: 'disabled' });
  await stop(); await start();
  checkHistory(await post('rp/observe', { session_id: failure.session_id }));
  await openPlay();
  assert.equal(rows(`SELECT COUNT(*) AS n FROM rp_utterances WHERE session_id=${quote(failure.session_id)} AND speaker_entity_id=${quote(failure.controlled_entity_id)} AND speech_text=${quote(captured.samples[0].speech)}`)[0].n, 1, 'restart lost/duplicated first speech');
  assert.deepEqual(counters(), before, 'restart/UI changed events, speech or provider receipts');
  assert.deepEqual(errors, []);
  Object.assign(receipt, { status: 'PASS: resumed deterministic postchecks; original live FAIL retained', failed_speech_once: true, all_heard_failed_silence: true, public_failure_receipts: replayFailures.length, ui_warning: true, selected_regeneration_restart_history: true, final_counts_unchanged: true, after: counters(), browser_errors: errors, checked_at_utc: new Date().toISOString() });
} catch (error) {
  Object.assign(receipt, { status: 'FAIL: resumed postchecks', error: error.message, after: counters() });
  throw error;
} finally {
  await browser?.close(); await vite?.close(); await stop();
  await writeFile(join(artifact, 'original32-postchecks.json'), JSON.stringify(receipt, null, 2), { mode: 0o600 });
}
console.log(JSON.stringify(receipt, null, 2));
