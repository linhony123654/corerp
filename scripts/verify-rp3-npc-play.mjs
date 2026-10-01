import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { once } from 'node:events';
import { openSync } from 'node:fs';
import { access, mkdtemp, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { chromium } from 'playwright';
import { createServer as createViteServer } from 'vite';
import { setPlayInputMode } from './play-ui-helpers.mjs';

// Real-model acceptance uses only this disposable Studio world. No source
// world, production database, or player account is read or contacted.
const root = resolve(import.meta.dirname, '..');
const pilot = process.argv.includes('--pilot');
const one = process.argv.includes('--one');
const prose = process.argv.includes('--prose');
const densities = process.argv.includes('--densities');
const persist = process.argv.includes('--persist');
const product = process.argv.includes('--product');
const useProfile = product || process.argv.includes('--profile');
const rongqing = process.argv.includes('--rongqing');
// Reuse the exact verified release build without changing any regression
// inputs or assertions. This also makes evidence identify the tested binary.
const binaryFlag = process.argv.indexOf('--binary-dir');
assert.ok(binaryFlag < 0 || (process.argv[binaryFlag + 1] && !process.argv[binaryFlag + 1].startsWith('--')), '--binary-dir requires a directory');
const binaryDir = binaryFlag < 0 ? '' : resolve(process.argv[binaryFlag + 1]);
// A turn can contain several sequential, individually bounded model calls.
const modelResponseTimeoutMs = 600_000;
if (densities && !prose) throw new Error('--densities requires --prose');
if (persist && (!prose || !one)) throw new Error('--persist requires --one --prose');
if (product && (!prose || !one || persist)) throw new Error('--product requires --one --prose without --persist');
if (rongqing && (!prose || one || product || persist || densities)) throw new Error('--rongqing requires --prose');
if (!process.env.CORERP_LLM_ENDPOINT?.startsWith('https://') || !process.env.CORERP_LLM_MODEL || !process.env.CORERP_LLM_API_KEY)
  throw new Error('R3 live acceptance requires a dedicated HTTPS model endpoint, model, and key');
const profileTimeout = /^(\d+)(s|m)$/.exec(process.env.CORERP_LLM_TIMEOUT || '60s');
if (useProfile) assert.ok(profileTimeout, 'profile timeout must be an integer in seconds or minutes');
const profileTuning = {
  timeoutSeconds: profileTimeout ? Number(profileTimeout[1]) * (profileTimeout[2] === 'm' ? 60 : 1) : 60,
  reasoningEffort: process.env.CORERP_LLM_REASONING_EFFORT || '',
  disableThinking: process.env.CORERP_LLM_DISABLE_THINKING === 'true',
  decisionMaxTokens: Number(process.env.CORERP_LLM_DECISION_MAX_TOKENS || 0),
  interactionMaxTokens: Number(process.env.CORERP_LLM_INTERACTION_MAX_TOKENS || 0),
  decisionFormat: process.env.CORERP_LLM_DECISION_FORMAT || '',
};
const temp = await mkdtemp(join(tmpdir(), 'corerp-rp3-npc-'));
const db = join(temp, 'world.db');
const world = rongqing ? 'rp9-rongqing-world' : 'rp3-npc-world', branch = 'br_main';
const worldTitle = rongqing ? '荣庆堂' : '街区活动室';
const playerKey = rongqing ? 'baoyu' : 'player';
const runtimeOrigin = 'http://127.0.0.1:4428', playOrigin = 'http://127.0.0.1:4429';
const canonical = value => Array.isArray(value) ? `[${value.map(canonical).join(',')}]` : value && typeof value === 'object' ? `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonical(value[key])}`).join(',')}}` : JSON.stringify(value);
const digest = value => createHash('sha256').update(canonical(value)).digest('hex');
const worldID = (kind, key) => `studio_${kind}_${digest(['corerp.studio-world.v1', world, kind, key])}`;
const run = (command, args, cwd = root) => execFileSync(command, args, { cwd, stdio: 'pipe' }).toString();
const sql = query => run('sqlite3', [db, query]).trim();
const head = () => Number(sql(`SELECT head_sequence FROM branches WHERE instance_id='${world}' AND branch_id='${branch}'`));
function bundle(kind) {
  const content = { version: 'corerp.studio-package.v1', ...(kind === 'system' ? { system_rules: { npc_daily_action_budget: 2 } } : { narrative_style: { version: 'corerp.style.v1', pov: 'second_person', tense: 'present', verbosity: 'terse', dialogue_ratio: 100, description_density: 0, inner_monologue_policy: 'none', full_prose: prose, prose_instructions: '', forbidden_patterns: [], narrative_pack_ref: 'builtin/plain@1' } }) };
  return { manifest: { schema_version: 'm0-draft-2026-09-22', id: `rp3.${kind}`, kind, version: '1.0.0', engine_api: 'm0-draft-2026-09-22', requires: [], optional: [], capabilities: [kind === 'system' ? 'rules.npc.daily_budget' : 'narrative.style'], schema_hash: 'sha256:0b7d979a3384df06726118c293ec3533e501cd0740f0819067e8e6191c2667a0', content_hash: `sha256:${digest(content)}`, content_files: [`${kind}.json`] }, content };
}
const cases = [
  '你今天是不是不太高兴？',
  '我看你刚才停下了手里的事，是在想什么吗？',
  '如果你不想说也没关系。我只是想知道能不能帮忙。',
  '你说的那件事，是今天才发生的吗？',
  '我不确定自己听懂了。你最担心的是什么？',
  '那接下来你打算先做哪一步？',
  '我可以听你说，但不会替你做决定。',
  '如果我刚才理解错了，你直接纠正我。',
  '我们刚才聊到哪里了？',
  '谢谢你告诉我这些。现在你希望我怎么做？',
  '你还记得我们昨天一起去海边吗？',
  '去年你借给我的那本书找到了吗？',
  '你认识刚进门的那位陌生人吗？',
  '你知道我真实姓名吗？',
  '刚才有人说你要离开这里，是真的吗？',
  '你为什么会这样想？',
  '你现在有空聊一会儿吗？',
  '如果你忙，我过一会儿再来。',
  '你平时在这里做什么？',
  '这里还有谁在忙自己的事？',
  '你最熟悉这里的什么？',
  '我刚才说过想帮忙，你还记得吗？',
  '你能把刚才的意思说得更具体一点吗？',
  '你有把握，还是只是猜测？',
  '这件事跟我有直接关系吗？',
  '如果我不接受你的建议，你会生气吗？',
  '你希望我替你保密吗？',
  '我没有答应任何事，我们可以先谈谈。',
  '那本不存在的信是谁写的？',
  '你是不是已经看见我口袋里的钥匙？',
  '回到一开始的问题：你现在心情怎么样？',
  '换个说法，你今天看起来有点心事，愿意聊聊吗？',
];
const rongqingCases = [
  '有人吗？', '你是谁呀？', '想你了。', '今儿这边怎么静悄悄的？',
  '我刚刚是不是打断你们了？', '你手里的活忙完了吗？', '若你不愿说，我就不追问。',
  '我记得你说过今天还有事，是什么事？', '你刚刚为什么躲着我？',
  '我们昨天一起去海边了吗？', '去年我借给你的书还在吗？', '你还记得去年那件事吗？',
  '这话是谁亲口告诉你的？', '我想先听你的想法。', '你不用现在答应我。',
  '你有没有觉得我心事重重？', '如果我说我有点难过，你会怎么想？', '你愿意陪我聊一会儿吗？',
  '你说的话我记下了。', '我可没有答应替谁做事。', '我之前说过会帮忙吗？',
  '宝钗，你现在最担心什么？', '别替我做决定，好吗？', '你有没有看到桌上的杯子？',
  '要是我把杯子推过来，你会接吗？', '我可以坐在这里听你们说话吗？',
  '黛玉，我有点想念从前，可又说不清是哪一天。', '刚才那件事是你亲眼见的，还是听来的？',
  '我还是想问，今天有谁在这里？', '换句话说，你愿意告诉我你是谁吗？',
  '我还记得一开始问过有没有人在，你们当时怎么回答？', '这一次先到这里，我们明天再谈。',
];
const people = rongqing ? [
  { key: 'baoyu', name: '宝玉', place: 'hall', player: true, persona: '住在荣庆堂的年轻人，今天只是来找人说话；没有替任何人作决定。' },
  { key: 'daiyu', name: '黛玉', place: 'hall', player: false, persona: '与宝玉相识，正在核对一张诗笺上自己的字句。言语敏锐，有时带一点讥诮；并不知道宝玉没有说出口的心事，也没有承诺过海边旅行或借书。' },
  { key: 'baochai', name: '宝钗', place: 'hall', player: false, persona: '与宝玉相识，正帮荣庆堂清点茶盏与借用的坐席。说话稳妥，关心实际安排；没有见过任何所谓去年的海边往事。' },
  { key: 'xiren', name: '袭人', place: 'hall', player: false, persona: '与宝玉相识，今天在整理堂里的布帘。更在意人的疲惫与照顾边界；没有替宝玉答应过任何事。' },
] : [
  { key: 'player', name: '来访者', place: 'hall', player: true },
  { key: 'mei', name: '梅', place: 'hall', player: false, persona: '在街区活动室负责登记和分配借用桌椅，今天正在核对借用记录。认识来访者，但不掌握对方未说出的私事。做事仔细，遇到不确定的事会问清来源。' },
  { key: 'qiao', name: '乔', place: 'hall', player: false, persona: '常来活动室修理旧收音机，目前手上的一台还没调好。习惯先找具体故障，不会把旁人的猜测当事实。与来访者尚未正式认识。' },
  { key: 'ning', name: '宁', place: 'hall', player: false, persona: '在活动室练习朗读，准备下周的公开演出，怕忘词但想把练习完成。与来访者尚未正式认识，不知道对方私下经历。' },
];
const creatorToken = randomBytes(24).toString('hex'), playerToken = randomBytes(24).toString('hex');
let runtime, vite, browser;
async function stop(child) {
  if (child && child.exitCode === null && child.signalCode === null) { const ended = once(child, 'exit'); child.kill('SIGTERM'); await ended; }
}
async function ready() {
  for (let i = 0; i < 120; i++) { try { if ((await fetch(`${runtimeOrigin}/readyz`)).ok) return; } catch { /* starting */ } await new Promise(resolve => setTimeout(resolve, 100)); }
  throw new Error('temporary R3 runtime did not start');
}
async function post(route, token, body, status = 200) {
  const response = await fetch(`${runtimeOrigin}/api/v1/${route}`, { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  const envelope = await response.json();
  assert.equal(response.status, status, `CoreRP ${route}: ${envelope.error?.code || response.status}`);
  return envelope.data;
}
try {
  for (const [name, program] of [['runtime', 'corerp-server'], ['m1', 'corerp-m1'], ['setup', 'corerp-m2'], ['admin', 'corerp-admin']]) {
    if (binaryDir) {
      await access(join(binaryDir, name));
      await symlink(join(binaryDir, name), join(temp, name));
    } else run('/usr/local/go/bin/go', ['build', '-buildvcs=false', '-o', join(temp, name), `./cmd/${program}`], join(root, 'backend'));
  }
  run(join(temp, 'm1'), ['-db', db, '-action', 'inspect']);
  const setup = JSON.parse(run(join(temp, 'setup'), ['-db', db, '-action', 'rp-travel-prepare']));
  run(join(temp, 'admin'), ['-db', db, '-operator', 'principal_operator', '-instance', 'inst_m2_t09', '-branch', branch, '-target', 'principal_creator', '-purpose', 'create_world', '-status', 'active', '-expected-head', String(setup.event_sequence), '-key', 'rp3-npc-world-grant']);
  const env = { ...process.env, CORERP_AUTH_TOKENS_JSON: JSON.stringify({ [creatorToken]: 'principal_creator', [playerToken]: 'principal_m2_rp_player' }), CORERP_CURSOR_SECRET: randomBytes(32).toString('hex'), CORERP_DECISION_PROVIDER: 'chat_completions', CORERP_LLM_ENDPOINT: process.env.CORERP_LLM_ENDPOINT, CORERP_LLM_MODEL: process.env.CORERP_LLM_MODEL, CORERP_LLM_API_KEY: process.env.CORERP_LLM_API_KEY, CORERP_LLM_TIMEOUT: process.env.CORERP_LLM_TIMEOUT || '60s', CORERP_LLM_ATTEMPTS: '2', CORERP_PROVIDER_ALLOWLIST: '', CORERP_PROVIDER_LOCAL_ALLOWLIST: '', CORERP_NARRATIVE_PROVIDER: prose ? 'full_prose' : 'deterministic', CORERP_NARRATIVE_ENDPOINT: prose ? process.env.CORERP_LLM_ENDPOINT : '', CORERP_NARRATIVE_MODEL: prose ? process.env.CORERP_LLM_MODEL : '', CORERP_NARRATIVE_API_KEY: prose ? process.env.CORERP_LLM_API_KEY : '', CORERP_NARRATIVE_TIMEOUT: prose ? '90s' : '', CORERP_NARRATIVE_ATTEMPTS: prose ? '2' : '' };
  const runtimeLog = openSync(join(temp, 'runtime-stderr.log'), 'a');
  runtime = spawn(join(temp, 'runtime'), ['-db', db, '-listen', '127.0.0.1:4428'], { env, stdio: ['ignore', 'ignore', runtimeLog] }); await ready();
  const created = await post('studio/worlds/create', creatorToken, { authority_instance_id: 'inst_m2_t09', authority_branch_id: branch, instance_id: world, idempotency_key: 'rp3-world', player_principal_id: 'principal_m2_rp_player', system_package: bundle('system'), narrative_package: bundle('narrative'), spec: { version: 'corerp.studio-world.v1', name: worldTitle, start_world_time: '2026-09-22T00:00:00Z', population: 4, opening_money_minor: 30, opening_stock_minor: 2, places: [{ key: 'hall', name: worldTitle, kind: 'public' }, { key: 'lane', name: rongqing ? '庭院小径' : '街区小巷', kind: 'public' }], links: [{ from: 'hall', to: 'lane', minutes: 5 }], people, acquaintances: rongqing ? [['baoyu', 'daiyu'], ['baoyu', 'baochai'], ['baoyu', 'xiren']] : [['player', 'mei']] } }, 201);
  assert.equal(created.entity_id, worldID('entity', playerKey));
  let cupSetup;
  if (rongqing) {
    const binding = key => ({ instance_id: world, branch_id: branch, expected_head: head(), idempotency_key: key });
    const stock = await post('rp/objects/stock/define', creatorToken, { binding: binding('rp9-cup-stock'), sku_code: 'cup', display_name: '杯子', owner_entity_id: created.entity_id });
    const placeID = worldID('place', 'hall');
    const table = await post('rp/objects/anchors/define', creatorToken, { binding: binding('rp9-cup-table'), place_id: placeID, zone_key: 'main', anchor_code: 'table-side', display_name: '桌边' });
    const near = await post('rp/objects/anchors/define', creatorToken, { binding: binding('rp9-cup-near'), place_id: placeID, zone_key: 'main', anchor_code: 'daiyu-side', display_name: '黛玉近旁', near_entity_id: worldID('entity', 'daiyu') });
    const source = await post('rp/objects/sources/define', creatorToken, { binding: binding('rp9-cup-source'), anchor_id: table.fact.anchor_id, owner_entity_id: created.entity_id, sku_id: stock.fact.sku_id, display_name: '杯子' });
    const setupSession = await post('rp/sessions/open', playerToken, { instance_id: world, branch_id: branch, entity_id: created.entity_id, pov: 'second_person', idempotency_key: 'rp9-cup-stage-session' });
    let setupView = await post('rp/observe', playerToken, { session_id: setupSession.session_id });
    const cup = await post('rp/actions/object', playerToken, { session_id: setupSession.session_id, expected_cursor: setupView.observation_cursor, idempotency_key: 'rp9-cup-stage', action: 'stage', source_id: source.fact.source_id });
    setupView = await post('rp/observe', playerToken, { session_id: setupSession.session_id });
    await post('rp/actions/object', playerToken, { session_id: setupSession.session_id, expected_cursor: setupView.observation_cursor, idempotency_key: 'rp9-cup-place', action: 'place', object_id: cup.object_id, anchor_id: table.fact.anchor_id });
    cupSetup = { objectID: cup.object_id, nearAnchorID: near.fact.anchor_id };
  }
  vite = await createViteServer({ root, server: { host: '127.0.0.1', port: 4429, strictPort: true, proxy: { '/api': runtimeOrigin } } }); await vite.listen();
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' });
  if (useProfile) await context.addInitScript(({ endpoint, model, key, tuning, fullProse }) => {
    localStorage.setItem('corerp.api_profiles.v1', JSON.stringify([
      { id: 'r8-live', name: 'R8 selected model', protocol: 'chat_completions', endpoint, model, apiKey: key, ...tuning, fullProse, temperature: 1.8, maxTokens: 9999 },
      { id: 'r8-unsupported', name: 'R8 Responses legacy', protocol: 'responses', endpoint: endpoint.replace('/chat/completions', '/responses'), model, apiKey: '', timeoutSeconds: 60 },
    ]));
    localStorage.setItem('corerp.active_api_profile_id.v1', 'r8-live');
  }, { endpoint: process.env.CORERP_LLM_ENDPOINT, model: process.env.CORERP_LLM_MODEL, key: process.env.CORERP_LLM_API_KEY, tuning: profileTuning, fullProse: prose });
  await context.route('**/*', route => new URL(route.request().url()).origin === playOrigin ? route.continue() : route.abort());
  const page = await context.newPage(), errors = []; page.on('pageerror', error => errors.push(error.message));
  const playRequests = [];
  if (useProfile) page.on('request', request => {
    if (request.url().endsWith('/rp/turns/run') || request.url().endsWith('/rp/narrative/stream') || request.url().endsWith('/rp/interactions/run')) {
      const data = request.postDataJSON();
      playRequests.push({ path: new URL(request.url()).pathname, model: data.model && { endpoint: data.model.endpoint, model: data.model.model, full_prose: !!data.model.full_prose, timeout_seconds: data.model.timeout_seconds, reasoning_effort: data.model.reasoning_effort, disable_thinking: data.model.disable_thinking, decision_max_tokens: data.model.decision_max_tokens, interaction_max_tokens: data.model.interaction_max_tokens, decision_format: data.model.decision_format, has_key: !!data.model.api_key, has_temperature: 'temperature' in data.model, has_max_tokens: 'max_tokens' in data.model }, interaction_mode: data.interaction_mode });
    }
  });
  page.setDefaultTimeout(240_000);
  await page.goto(playOrigin);
  await page.getByLabel('玩家访问凭证').fill(playerToken);
  await page.getByRole('button', { name: '进入世界' }).click();
  await page.getByRole('button', { name: rongqing ? /宝玉/ : /来访者/ }).click();
  await page.getByRole('heading', { name: worldTitle }).waitFor();
  if (product) {
    await page.getByRole('button', { name: `模型：${process.env.CORERP_LLM_MODEL}` }).click();
    assert.equal(await page.getByRole('radio', { name: /R8 Responses legacy/ }).getAttribute('aria-disabled'), 'true', 'unsupported protocol was selectable');
    await page.getByRole('button', { name: '编辑该配置' }).first().click();
    await page.getByRole('button', { name: /高级参数/ }).click();
    assert.equal(await page.getByLabel(/采样温度|最大 Tokens/).count(), 0, 'unsupported sampling controls remain visible');
    await page.getByLabel('超时时间 (秒)').waitFor();
    await page.getByRole('button', { name: '关闭详情' }).click();
  }
  await setPlayInputMode(page, 'speech');
  const samples = [];
  const turnTakingDiagnostics = [];
  let r9Checks;
  let playSessionID = '';
  let persisted;
  for (const [index, speech] of (rongqing ? (pilot ? rongqingCases.slice(0, 3) : rongqingCases) : one ? cases.slice(0, 1) : pilot ? cases.slice(0, 3) : cases).entries()) {
    const pending = page.waitForResponse(response => response.url().endsWith('/rp/turns/run'), { timeout: modelResponseTimeoutMs });
    await page.getByLabel('你想说的话').fill(speech);
    await page.getByRole('button', { name: '说出' }).click();
    const response = await pending, envelope = await response.json();
    assert.equal(response.status(), 200, `turn ${index + 1}: ${envelope.error?.code || response.status()}`);
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending, null, { timeout: modelResponseTimeoutMs });
    const turn = envelope.data;
    assert.equal(turn.status, 'settled', `turn ${index + 1} not settled`);
    assert.match(turn.turn_run_id, /^rpturn_/, `turn ${index + 1} lacks durable turn run ID`);
    const runSessionID = sql(`SELECT session_id FROM rp_turn_runs WHERE turn_run_id='${turn.turn_run_id}'`);
    assert.ok(runSessionID, `turn ${index + 1} has no Play session`);
    if (playSessionID) assert.equal(runSessionID, playSessionID, 'Play switched sessions mid-conversation');
    playSessionID = runSessionID;
    const replies = JSON.parse(sql(`SELECT json_group_array(json_object('speaker',e.display_name,'action',d.action,'text',COALESCE(u.speech_text,''))) FROM rp_npc_decisions d JOIN materialized_entities e ON e.entity_id=d.npc_entity_id LEFT JOIN rp_utterances u ON u.event_id=d.event_id WHERE d.parent_turn_id='${turn.player_turn_id}'`));
    assert.equal(replies.length, 3, `turn ${index + 1} missing committed NPC decisions`);
    const openingVocative = /^([^，,：:]{1,12})[，,：:]/u.exec(speech)?.[1];
    const addressee = rongqing && !pilot ? people.find(person => !person.player && person.name === openingVocative)?.name : undefined;
    if (addressee) {
      // Turn-taking is an experience signal, not a world-integrity rule:
      // someone may reasonably interject or decline to answer. Preserve the
      // exact observed exchange for human review instead of treating a
      // semantic judgment as an authoritative hard gate.
      turnTakingDiagnostics.push({ turn: index + 1, addressee,
        addressedReplied: replies.some(reply => reply.speaker === addressee && reply.text),
        otherSpeakers: replies.filter(reply => reply.speaker !== addressee && reply.text).map(reply => ({ speaker: reply.speaker, text: reply.text })) });
    }
    if (index === 0) {
      const spoken = replies.filter(reply => reply.text);
      assert.ok(spoken.length >= (rongqing ? 1 : 3), 'three-NPC same-question sample lacks actual spoken replies');
      assert.equal(new Set(spoken.map(reply => reply.text)).size, spoken.length, 'same-question replies were mechanically identical');
    }
    const calls = turn.provider_calls?.filter(call => call.phase === 'decision') ?? [];
    assert.equal(calls.length, 3, `turn ${index + 1} missing NPC decision receipts`);
    assert.ok(calls.every(call => call.provider_kind === 'chat_completions' && call.result === 'success' && call.attempt_count > 0), `turn ${index + 1} used fallback: ${JSON.stringify(calls)}`);
    const playNarrativeReceipt = JSON.parse(sql(`SELECT json_object('provider',provider_kind,'result',result,'attempts',attempt_count,'fallback',fallback_kind,'source',render_source) FROM rp_provider_calls WHERE turn_run_id='${turn.turn_run_id}' AND phase='narrative' ORDER BY rowid DESC LIMIT 1`));
    if (rongqing) assert.ok(playNarrativeReceipt.provider === 'full_prose' && playNarrativeReceipt.result === 'success' && playNarrativeReceipt.attempts > 0 && !playNarrativeReceipt.fallback, `turn ${index + 1} Play prose fallback: ${JSON.stringify(playNarrativeReceipt)}`);
    let narrative;
    let styleVariants;
    if (prose && (!rongqing || index === 0 || index === rongqingCases.length - 1)) {
      const rendered = await post('rp/narrative/render', playerToken, { session_id: playSessionID, turn_run_id: turn.turn_run_id });
      narrative = rendered.view;
      if (persist && index === 0) {
        assert.match(narrative.render_id, /^rpr_/, 'completed prose has no durable render ID');
        const firstID = narrative.render_id, beforeVariants = head();
        let observation = await post('rp/observe', playerToken, { session_id: playSessionID });
        let shown = observation.recent_turns.find(item => item.turn_run_id === turn.turn_run_id);
        assert.equal(shown?.render_id, firstID, 'generate → refresh selected another version');
        assert.deepEqual(shown.narrative_lines, narrative.lines, 'generate → refresh changed prose');
        const regenerated = await post('rp/narrative/render', playerToken, { session_id: playSessionID, turn_run_id: turn.turn_run_id });
        assert.match(regenerated.view.render_id, /^rpr_/, 'regeneration has no durable render ID');
        assert.notEqual(regenerated.view.render_id, firstID, 'regeneration reused render version');
        const selected = await post('rp/narrative/select', playerToken, { session_id: playSessionID, turn_run_id: turn.turn_run_id, render_id: firstID });
        assert.deepEqual(selected.lines, narrative.lines, 'select did not recover first render');
        observation = await post('rp/observe', playerToken, { session_id: playSessionID });
        shown = observation.recent_turns.find(item => item.turn_run_id === turn.turn_run_id);
        assert.equal(shown?.render_id, firstID, 'regenerate → select → refresh lost selection');
        assert.deepEqual(shown.narrative_lines, narrative.lines, 'selected prose changed before restart');
        assert.equal(head(), beforeVariants, 'render/selection changed world head');
        persisted = { turnID: turn.turn_run_id, firstID, firstLines: narrative.lines, secondID: regenerated.view.render_id, baseline: turn.narrative_lines };
      }
      const proof = JSON.parse(sql(`SELECT json_object('provider',provider_kind,'result',result,'attempts',attempt_count,'fallback',fallback_kind) FROM rp_provider_calls WHERE turn_run_id='${turn.turn_run_id}' AND phase='narrative' ORDER BY rowid DESC LIMIT 1`));
      assert.ok(proof.provider === 'full_prose' && proof.result === 'success' && proof.attempts > 0 && !proof.fallback && !narrative.fallback_reason, `turn ${index + 1} prose fallback: ${JSON.stringify(proof)}`);
      if (densities && index === 0) {
        const beforeVariants = head();
        styleVariants = {};
        for (const [density, verbosity, descriptionDensity, dialogueRatio] of [
          ['concise', 'terse', 0, 100], ['standard', 'normal', 30, 80], ['long', 'detailed', 70, 50],
        ]) {
          const variant = await post('rp/narrative/render', playerToken, { session_id: playSessionID, turn_run_id: turn.turn_run_id, style_override: { narrative_density: density, verbosity, description_density: descriptionDensity, dialogue_ratio: dialogueRatio } });
          const variantProof = JSON.parse(sql(`SELECT json_object('provider',provider_kind,'result',result,'attempts',attempt_count,'fallback',fallback_kind) FROM rp_provider_calls WHERE turn_run_id='${turn.turn_run_id}' AND phase='narrative' ORDER BY rowid DESC LIMIT 1`));
          assert.ok(variantProof.provider === 'full_prose' && variantProof.result === 'success' && variantProof.attempts > 0 && !variantProof.fallback && !variant.view.fallback_reason, `${density} prose fallback: ${JSON.stringify(variantProof)}`);
          for (const reply of replies) if (reply.text) assert.ok(variant.view.lines.join('\n').includes(reply.text), `${density} dropped accepted NPC speech`);
          assert.ok(variant.view.lines.join('\n').includes(speech), `${density} dropped accepted player speech`);
          styleVariants[density] = variant.view.lines;
        }
        assert.equal(head(), beforeVariants, 'render variants changed authoritative world head');
      }
    }
    // Preserve every actual selected Play display for human review, without
    // requesting an extra model render for intermediate Rongqing turns.
    if (prose && !narrative) {
      const display = (await post('rp/observe', playerToken, { session_id: playSessionID })).recent_turns.find(item => item.turn_run_id === turn.turn_run_id);
      assert.ok(display?.narrative_lines?.length, `turn ${index + 1} has no reviewable selected display`);
      narrative = { lines: display.narrative_lines, render_id: display.render_id };
    }
    samples.push({ index: index + 1, speech, replies, narrative: narrative?.lines, styleVariants, calls: calls.map(({ result, attempt_count }) => ({ result, attempt_count })), playNarrativeReceipt });
    await writeFile(join(temp, 'npc-samples.json'), JSON.stringify({ world, model: process.env.CORERP_LLM_MODEL, viaProfile: useProfile, tuning: profileTuning, samples }, null, 2), { mode: 0o600 });
    console.log(JSON.stringify({ turn: index + 1, replies: replies.length, decisions: calls.length, artifacts: temp }));
  }
  if (useProfile) {
    assert.ok(playRequests.some(item => item.path.endsWith('/rp/turns/run')), 'profile pilot captured no turn request');
    for (const request of playRequests) {
      assert.equal(request.model?.model, process.env.CORERP_LLM_MODEL, 'profile model missing from request');
      assert.equal(request.model.timeout_seconds, profileTuning.timeoutSeconds, 'profile timeout missing');
      assert.equal(request.model.reasoning_effort || '', profileTuning.reasoningEffort, 'profile reasoning effort missing');
      assert.equal(!!request.model.disable_thinking, profileTuning.disableThinking, 'profile thinking option missing');
      assert.equal(request.model.decision_max_tokens || 0, profileTuning.decisionMaxTokens, 'profile decision budget missing');
      assert.equal(request.model.interaction_max_tokens || 0, profileTuning.interactionMaxTokens, 'profile interpretation budget missing');
      assert.equal(request.model.decision_format || '', profileTuning.decisionFormat, 'profile decision format missing');
    }
  }
  if (product) {
    const turnRequest = playRequests.find(item => item.path.endsWith('/rp/turns/run'));
    const proseRequest = playRequests.find(item => item.path.endsWith('/rp/narrative/stream'));
    for (const request of [turnRequest, proseRequest]) {
      assert.equal(request?.model?.model, process.env.CORERP_LLM_MODEL, 'Play model selection did not reach request');
      assert.equal(request.model.endpoint, process.env.CORERP_LLM_ENDPOINT, 'Play endpoint was rewritten unexpectedly');
      assert.equal(request.model.has_key, true, 'Play model override lost credential');
      assert.equal(request.model.has_temperature || request.model.has_max_tokens, false, 'legacy UI-only sampling fields reached request');
    }
    assert.equal(proseRequest.model.full_prose, true, 'Play full-prose toggle did not reach stream');
    await setPlayInputMode(page, 'AUTO');
    const interactionResponse = page.waitForResponse(response => response.url().endsWith('/rp/interactions/run'), { timeout: modelResponseTimeoutMs });
    await page.getByLabel('你想说或做的事').fill('继续剧情');
    await page.getByRole('button', { name: '说出' }).click();
    assert.equal((await interactionResponse).status(), 200, 'AUTO mode did not reach interpretation API');
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending, null, { timeout: modelResponseTimeoutMs });
    const interactionRequest = playRequests.find(item => item.path.endsWith('/rp/interactions/run'));
    assert.equal(interactionRequest?.interaction_mode, 'AUTO', 'Play mode choice did not reach request');
    assert.equal(interactionRequest?.model?.model, process.env.CORERP_LLM_MODEL, 'interaction model override differs from selected profile');
  }
  if (rongqing && !pilot) {
    assert.equal(samples.length, 32, 'R9 requires 30+ continuous Play speech turns');
    const spokenBy = new Set(samples.flatMap(sample => sample.replies.filter(reply => reply.text).map(reply => reply.speaker)));
    assert.deepEqual([...spokenBy].sort(), ['宝钗', '袭人', '黛玉'].sort(), 'R9 did not capture accepted speech from all three NPCs');
    const lastTurnID = sql(`SELECT turn_run_id FROM rp_turn_runs WHERE session_id='${playSessionID}' AND status='settled' ORDER BY rowid DESC LIMIT 1`);
    const beforeRegen = await post('rp/observe', playerToken, { session_id: playSessionID });
    const previous = beforeRegen.recent_turns.find(item => item.turn_run_id === lastTurnID);
    assert.ok(previous?.narrative_lines?.length > 0 && !previous.render_id, 'Play did not save its first live prose as official narrative');
    const beforeRegenHead = head();
    assert.equal(previous.can_regenerate, true, 'last settled player turn cannot be regenerated');
    const latestTurn = page.locator(`[id="turn-${lastTurnID}"]`);
    await latestTurn.locator('.narrative-tools summary').click();
    const regenerateFallbacks = [];
    let regenerated = false;
    for (let attempt = 0; attempt < 3; attempt++) {
      const previousCallID = sql(`SELECT call_id FROM rp_provider_calls WHERE turn_run_id='${lastTurnID}' AND phase='narrative' AND provider_kind='full_prose' ORDER BY rowid DESC LIMIT 1`);
      const streamed = page.waitForResponse(response => response.url().endsWith('/rp/narrative/stream'), { timeout: modelResponseTimeoutMs });
      await latestTurn.getByRole('button', { name: '按当前设置重新生成' }).click();
      const regeneratedResponse = await streamed;
      assert.equal(regeneratedResponse.status(), 200, 'Play regenerate request failed');
      let receipt;
      for (let poll = 0; poll < 300; poll++) {
        const raw = sql(`SELECT json_object('call_id',call_id,'result',result,'fallback',fallback_kind) FROM rp_provider_calls WHERE turn_run_id='${lastTurnID}' AND phase='narrative' AND provider_kind='full_prose' ORDER BY rowid DESC LIMIT 1`);
        receipt = raw ? JSON.parse(raw) : null;
        if (receipt.call_id !== previousCallID && receipt.result !== 'pending') break;
        await new Promise(resolve => setTimeout(resolve, 100));
      }
      assert.ok(receipt?.call_id !== previousCallID && receipt.result !== 'pending', 'Play regeneration left no completed provider receipt');
      if (receipt.result === 'success' && !receipt.fallback) {
        regenerated = true;
        break;
      }
      assert.match(receipt.fallback, /^prose_(?:unavailable|timeout)$/, 'Play regeneration hit a fact or validation fallback');
      regenerateFallbacks.push(receipt.fallback);
      const unchanged = await post('rp/observe', playerToken, { session_id: playSessionID });
      assert.equal(unchanged.recent_turns.find(item => item.turn_run_id === lastTurnID)?.render_id || '', '', 'failed regeneration replaced the selected prose');
      assert.equal(head(), beforeRegenHead, 'failed regeneration changed world head');
    }
    assert.ok(regenerated, `Play regeneration remained unavailable after bounded retries: ${regenerateFallbacks.join(', ')}`);
    await page.waitForFunction(id => document.getElementById(`turn-${id}`)?.querySelector('.narrative-tools')?.textContent?.includes('展示已更新'), lastTurnID, { timeout: 30000 });
    let selected = await post('rp/observe', playerToken, { session_id: playSessionID });
    const regeneratedID = selected.recent_turns.find(item => item.turn_run_id === lastTurnID)?.render_id;
    assert.match(regeneratedID, /^rpr_/, 'Play regenerate did not select a saved render ID');
    assert.notEqual(regeneratedID, previous.render_id, 'Play regenerate reused the old render');
    assert.equal(head(), beforeRegenHead, 'Play regeneration changed world head');

    await setPlayInputMode(page, 'AUTO');
    const submitInteraction = async phrase => {
      const pending = page.waitForResponse(response => response.url().endsWith('/rp/interactions/run'), { timeout: modelResponseTimeoutMs });
      await page.getByLabel('你想说或做的事').fill(phrase);
      await page.getByRole('button', { name: '说出' }).click();
      const response = await pending, envelope = await response.json();
      assert.equal(response.status(), 200, `R9 interaction ${phrase}: ${envelope.error?.code || response.status()}`);
      await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending, null, { timeout: modelResponseTimeoutMs });
      return envelope.data;
    };
    const mixed = await submitInteraction('我把桌上的杯子推到黛玉近旁，然后说「喝一点吧。」');
    assert.equal(mixed.status, 'settled');
    assert.equal(mixed.plan_kind, 'MIXED');
    assert.deepEqual(mixed.outcomes.map(item => item.kind), ['object', 'speech']);
    assert.equal(sql(`SELECT anchor_id FROM rp_objects WHERE object_id='${cupSetup.objectID}'`), cupSetup.nearAnchorID, 'mixed action did not move real cup');
    assert.equal(sql(`SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id='${created.entity_id}' AND speech_text='喝一点吧。'`), '1', 'mixed accepted speech missing or duplicated');
    const continued = await submitInteraction('继续剧情');
    assert.equal(continued.plan_kind, 'CONTINUE', 'meta continue became IC speech');
    assert.equal(sql(`SELECT COUNT(*) FROM rp_utterances WHERE speech_text='继续剧情'`), '0', 'meta prompt leaked into accepted dialogue');

    await stop(runtime);
    runtime = spawn(join(temp, 'runtime'), ['-db', db, '-listen', '127.0.0.1:4428'], { env: { ...env, CORERP_LLM_API_KEY: 'invalid-r9-provider-key', CORERP_NARRATIVE_API_KEY: 'invalid-r9-provider-key', CORERP_LLM_ATTEMPTS: '1', CORERP_NARRATIVE_ATTEMPTS: '1' }, stdio: 'ignore' });
    await ready();
    await page.reload(); await page.getByLabel('玩家访问凭证').fill(playerToken);
    await page.getByRole('button', { name: '继续这段生活' }).click();
    await page.getByRole('heading', { name: worldTitle }).waitFor();
    if (useProfile) {
      // A browser override otherwise keeps the valid key and bypasses the
      // intentionally broken runtime default. Save the fixture failure key
      // through the real settings flow so the active in-memory profile agrees.
      await page.getByRole('button', { name: `模型：${process.env.CORERP_LLM_MODEL}` }).click();
      await page.getByRole('button', { name: '编辑该配置' }).first().click();
      await page.getByLabel('API 密钥 (API Key)').fill('invalid-r9-provider-key');
      await page.getByRole('button', { name: '保存并设为当前生效' }).click();
      await page.getByRole('button', { name: '关闭详情' }).click();
    }
    await setPlayInputMode(page, 'speech');
    const failing = page.waitForResponse(response => response.url().endsWith('/rp/turns/run'), { timeout: modelResponseTimeoutMs });
    await page.getByLabel('你想说的话').fill('你们现在还能听见我吗？');
    await page.getByRole('button', { name: '说出' }).click();
    const failedResponse = await failing, failedEnvelope = await failedResponse.json();
    assert.equal(failedResponse.status(), 200, 'provider failure also rolled back accepted player speech');
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending, null, { timeout: modelResponseTimeoutMs });
    const failedTurn = failedEnvelope.data;
    const failures = failedTurn.provider_calls?.filter(call => call.phase === 'decision') || [];
    // CONTINUE may legitimately move an NPC away. Failure coverage follows
    // the immutable hearing snapshot of this speech, not the opening cast.
    const heardActors = JSON.parse(sql(`SELECT json_extract(e.payload,'$.listener_ids') FROM rp_turn_runs r JOIN events e ON e.event_id=r.player_event_id WHERE r.turn_run_id='${failedTurn.turn_run_id}'`));
    assert.ok(heardActors.length > 0, 'failure check has no heard actors');
    const receiptActors = JSON.parse(sql(`SELECT json_group_array(npc_entity_id) FROM rp_provider_calls WHERE turn_run_id='${failedTurn.turn_run_id}' AND phase='decision'`));
    assert.deepEqual(receiptActors.sort(), [...heardActors].sort(), 'provider failure receipts differ from frozen heard actor set');
    assert.equal(failures.length, heardActors.length, 'public failure receipt count differs from frozen heard actor set');
    assert.ok(failures.every(call => call.result !== 'success' && call.fallback_kind === 'silence'), 'provider failure was hidden as ordinary NPC choice');
    assert.equal(sql(`SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id='${created.entity_id}' AND speech_text='你们现在还能听见我吗？'`), '1', 'failed provider duplicated or lost player speech');
    await page.locator('.turn.narrator').last().getByText('本轮人物模型调用失败或超时', { exact: false }).waitFor();
    await stop(runtime); runtime = spawn(join(temp, 'runtime'), ['-db', db, '-listen', '127.0.0.1:4428'], { env, stdio: 'ignore' }); await ready();
    selected = await post('rp/observe', playerToken, { session_id: playSessionID });
    assert.equal(selected.recent_turns.find(item => item.turn_run_id === lastTurnID)?.render_id, regeneratedID, 'provider failure/restart erased earlier selected prose');
    r9Checks = { continuousPlayerSpeechTurns: 32, distinctNPCs: 3, turnTakingDiagnostics, regenerateRenderID: regeneratedID, regenerateFallbacks, mixedCupEvent: true, metaContinueNoSpeech: true, providerFailureReceipts: failures.length, failureTurnID: failedTurn.turn_run_id };
    await writeFile(join(temp, 'r9-checks.json'), JSON.stringify(r9Checks, null, 2), { mode: 0o600 });
  }
  const eventCount = sql(`SELECT COUNT(*) FROM events WHERE instance_id='${world}' AND branch_id='${branch}'`);
  const utteranceCount = Number(sql(`SELECT COUNT(*) FROM rp_utterances WHERE session_id='${playSessionID}'`));
  assert.ok(utteranceCount >= samples.length, 'Play session lost accepted utterances');
  // Play has a fixed viewport and no document flow height.
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  await page.screenshot({ path: join(temp, 'npc-play-mobile.png'), animations: 'disabled' });
  await writeFile(join(temp, 'npc-samples.json'), JSON.stringify({ world, model: process.env.CORERP_LLM_MODEL, samples }, null, 2), { mode: 0o600 });
  await stop(runtime); runtime = spawn(join(temp, 'runtime'), ['-db', db, '-listen', '127.0.0.1:4428'], { env, stdio: 'ignore' }); await ready();
  await page.reload(); await page.getByLabel('玩家访问凭证').fill(playerToken);
  await page.getByRole('button', { name: '继续这段生活' }).click();
  await page.getByRole('heading', { name: worldTitle }).waitFor();
  assert.equal(Number(sql(`SELECT COUNT(*) FROM rp_utterances WHERE session_id='${playSessionID}' AND speaker_entity_id='${created.entity_id}' AND speech_text='${samples[0].speech}'`)), 1, 'restart lost the first accepted player speech');
  assert.ok((await page.locator('.reading').innerText()).includes(samples.at(-1).speech), 'restart did not show the latest accepted player speech');
  if (persisted) {
    let observation = await post('rp/observe', playerToken, { session_id: playSessionID });
    let shown = observation.recent_turns.find(item => item.turn_run_id === persisted.turnID);
    assert.equal(shown?.render_id, persisted.firstID, 'restart lost selected render ID');
    assert.deepEqual(shown.narrative_lines, persisted.firstLines, 'restart lost selected prose');
    assert.ok((await page.locator('.turn.narrator .prose').first().innerText()).includes(persisted.firstLines[0]), 'browser reload did not show selected prose');
    const baseline = await post('rp/narrative/select', playerToken, { session_id: playSessionID, turn_run_id: persisted.turnID, render_id: '' });
    assert.deepEqual(baseline.lines, persisted.baseline, 'baseline restore differed from canonical turn');
    await page.reload(); await page.getByLabel('玩家访问凭证').fill(playerToken);
    await page.getByRole('button', { name: '继续这段生活' }).click();
    await page.getByRole('heading', { name: worldTitle }).waitFor();
    observation = await post('rp/observe', playerToken, { session_id: playSessionID });
    shown = observation.recent_turns.find(item => item.turn_run_id === persisted.turnID);
    assert.ok(!shown?.render_id && JSON.stringify(shown.narrative_lines) === JSON.stringify(persisted.baseline), 'restored canonical display was not durable');
    assert.equal(head(), Number(sql(`SELECT settled_sequence FROM rp_turn_runs WHERE turn_run_id='${persisted.turnID}'`)), 'display selection moved world head');
  }
  assert.equal(sql(`SELECT COUNT(*) FROM events WHERE instance_id='${world}' AND branch_id='${branch}'`), eventCount, 'restart changed world');
  assert.equal(Number(sql(`SELECT COUNT(*) FROM rp_utterances WHERE session_id='${playSessionID}'`)), utteranceCount, 'restart changed accepted speech');
  assert.deepEqual(errors, [], 'browser page errors');
  console.log(JSON.stringify({ rp3NpcPlay: product ? 'PRODUCT_LIVE' : pilot || one ? 'PILOT_LIVE' : 'FULL_LIVE_SAMPLE_CAPTURED', turns: samples.length, model: process.env.CORERP_LLM_MODEL, artifacts: temp }));
} finally {
  await browser?.close(); await vite?.close(); await stop(runtime);
}
