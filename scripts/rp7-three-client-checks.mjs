import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { randomUUID } from 'node:crypto';
import { resolve, join } from 'node:path';
import { createServer } from 'vite';
import { setPlayInputMode } from './play-ui-helpers.mjs';

const requireMCP = createRequire(new URL('../clients/mcp/package.json', import.meta.url));
const { Client } = requireMCP('@modelcontextprotocol/client');
const { StdioClientTransport } = requireMCP('@modelcontextprotocol/client/stdio');
const root = resolve(import.meta.dirname, '..');

// Actual Play Vue page + installed SillyTavern + MCP wire subprocess. API reads
// below inspect the same real Runtime; none replace one of these three clients.
export async function checkRP7ThreeClients({ page: tavern, browserContext, token, cardAvatar, restartRuntime, authoritySnapshot, identitySnapshot, temp }) {
  const vite = await createServer({ root, server: { host: '127.0.0.1', port: 4199, strictPort: true, proxy: { '/api': 'http://127.0.0.1:4198' } }, logLevel: 'error' });
  await vite.listen();
  let play, mcp;
  const errors = [];
  const identities = identitySnapshot();
  async function connectMCP() {
    const client = new Client({ name: 'corerp-three-client-fixture', version: '1.0.0' });
    const transport = new StdioClientTransport({ command: process.execPath, args: [join(root, 'clients/mcp/index.js')], env: { CORERP_ORIGIN: 'http://127.0.0.1:4198', CORERP_TOKEN: token }, stderr: 'pipe' });
    try { await client.connect(transport); return client; } catch (error) { await transport.close(); throw error; }
  }
  async function tool(name, args) {
    const result = await mcp.callTool({ name, arguments: args });
    assert.notEqual(result.isError, true, JSON.stringify(result));
    return (result.structuredContent ?? JSON.parse(result.content.find(item => item.type === 'text').text)).data;
  }
  const panel = tavern.locator('#corerp-runtime');
  async function playRefresh() {
    const response = play.waitForResponse(r => r.url().endsWith('/rp/observe'));
    await play.getByRole('button', { name: '环顾四周', exact: true }).click();
    const envelope = await (await response).json(); assert.ok(envelope.data);
    await play.getByRole('heading', { name: envelope.data.place_name, exact: true }).waitFor();
    return envelope.data;
  }
  async function tavernRefresh() {
    const response = tavern.waitForResponse(r => r.url().endsWith('/rp/observe'));
    await panel.locator('[data-action=refresh]').click();
    const envelope = await (await response).json(); assert.ok(envelope.data);
    await tavern.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
    return envelope.data;
  }
  try {
    play = await browserContext.newPage(); play.on('pageerror', error => errors.push(error.message));
    await play.goto('http://127.0.0.1:4199');
    await play.getByLabel('玩家访问凭证').fill(token);
    const opened = play.waitForResponse(r => r.url().endsWith('/rp/sessions/open'));
    await play.getByRole('button', { name: '进入世界' }).click();
    assert.equal((await opened).status(), 200, 'actual Play session open rejected');
    await play.getByRole('button', { name: '环顾四周', exact: true }).waitFor();
    const playSession = await play.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).session);
    const tavernSession = await tavern.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.session_id);
    mcp = await connectMCP();
    const bindings = await tool('corerp_worlds', {});
    const binding = bindings.bindings.find(item => item.entity_id === 'entity_m2_rp_lin'); assert.ok(binding);
    const session = await tool('corerp_session_open', { instance_id: binding.instance_id, branch_id: binding.branch_id, entity_id: binding.entity_id, pov: 'second_person', idempotency_key: randomUUID() });
    assert.equal(new Set([playSession, tavernSession, session.session_id]).size, 3, 'must test three real independent sessions');
    assert.equal(identitySnapshot(), identities, 'opening clients materialized extra NPCs');
    await setPlayInputMode(play, 'speech');
    const read = { session_id: session.session_id };
    const words = { play: 'Play 页面发出的同世界联合验收发言。', tavern: '酒馆页面发出的同世界联合验收发言。', mcp: 'MCP 工具发出的同世界联合验收发言。' };

    await playRefresh();
    const playReply = play.waitForResponse(r => r.url().endsWith('/rp/turns/run'));
    await play.getByLabel('你想说的话').fill(words.play);
    await play.getByRole('button', { name: '说出' }).click();
    assert.equal((await playReply).status(), 200);
    await play.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending);
    await tavern.waitForFunction(text => document.querySelector('#corerp-runtime pre').textContent.includes(text), words.play);
    assert.ok((await tool('corerp_observe', read)).recent_turns.some(turn => turn.narrative_lines.some(line => line.includes(words.play))));

    await panel.locator('[name=operation]').selectOption('dialogue');
    await panel.locator('[name=command]').fill(words.tavern);
    await panel.locator('[data-action=submit]').click();
    await tavern.waitForFunction(text => !SillyTavern.getContext().chatMetadata.corerp_runtime.pending && document.querySelector('#corerp-runtime pre').textContent.includes(text), words.tavern);
    await playRefresh(); assert.ok((await play.locator('.reading').innerText()).includes(words.tavern));
    let view = await tool('corerp_observe', read);
    assert.ok(view.recent_turns.some(turn => turn.narrative_lines.some(line => line.includes(words.tavern))));
    const mcpSpeech = { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), text: words.mcp };
    const mcpTurn = await tool('corerp_dialogue', mcpSpeech); assert.equal(mcpTurn.status, 'settled');
    await tavern.waitForFunction(text => document.querySelector('#corerp-runtime pre').textContent.includes(text), words.mcp);
    await playRefresh(); assert.ok((await play.locator('.reading').innerText()).includes(words.mcp));

    // Location/time are shared authority too, not just copied chat text.
    view = await tool('corerp_observe', read);
    const destination = view.reachable_places.find(item => item.can_move_now && item.place_id === 'place_m2_home_ada'); assert.ok(destination);
    await tool('corerp_command', { operation: 'move', request: { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), from_place_id: view.place_id, to_place_id: destination.place_id } });
    view = await tool('corerp_observe', read);
    const target = new Date(Date.parse(view.world_time) + 60_000).toISOString().replace('.000Z', 'Z');
    const waited = await tool('corerp_wait', { ...read, expected_cursor: view.observation_cursor, idempotency_key: randomUUID(), target_world_time: target, budget: 100 }); assert.equal(waited.status, 'completed');

    const history = v => v.recent_turns.map(turn => ({ turn_run_id: turn.turn_run_id, narrative_lines: turn.narrative_lines }));
    async function compareAll() {
      const a = await playRefresh(), b = await tavernRefresh(), c = await tool('corerp_observe', read);
      for (const v of [a, b]) for (const key of ['controlled_entity', 'world_time', 'place_id', 'present_entities', 'observation_cursor']) assert.deepEqual(v[key], c[key], key);
      assert.deepEqual(history(a), history(c)); assert.deepEqual(history(b), history(c));
      for (const text of Object.values(words)) assert.ok(history(c).some(turn => turn.narrative_lines.some(line => line.includes(text))));
      const known = await tool('corerp_context', read);
      const injected = await tavern.evaluate(() => { const value = SillyTavern.getContext().extensionPrompts.corerp_runtime.value; return JSON.parse(value.slice(value.indexOf('{'))); });
      for (const key of ['instance_id', 'branch_id', 'observer_entity_id', 'world_time', 'facts']) assert.deepEqual(injected[key], known[key], key);
      assert.equal(c.place_id, destination.place_id); assert.equal(c.world_time, target);
      assert.equal(identitySnapshot(), identities, 'clients split existing people into copies');
      return c;
    }
    const before = await compareAll();
    await play.screenshot({ path: join(temp, 'three-client-play.png'), fullPage: true });
    const authority = authoritySnapshot();
    await restartRuntime();
    await tavern.waitForFunction(() => document.querySelector('#corerp-runtime [role=status]')?.textContent?.includes('已连接 · 世界见闻已同步'), undefined, { timeout: 35_000 });
    assert.equal(authoritySnapshot(), authority, 'automatic stream reconnect changed shared authority');
    const afterAutomaticReconnect = await compareAll();
    assert.deepEqual(history(afterAutomaticReconnect), history(before));
    await mcp.close(); mcp = undefined;
    await play.reload(); await tavern.reload(); // release streams and page-memory tokens
    await restartRuntime();
    assert.equal(authoritySnapshot(), authority, 'runtime restart changed shared authority');
    await play.getByLabel('玩家访问凭证').waitFor();
    assert.equal(await play.getByLabel('玩家访问凭证').inputValue(), '');
    await play.getByLabel('玩家访问凭证').fill(token);
    await play.getByRole('button', { name: '继续这段生活' }).click();
    await play.getByRole('button', { name: '环顾四周', exact: true }).waitFor();
    assert.equal(await play.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).session), playSession);
    await tavern.waitForSelector('#corerp-runtime', { state: 'attached', timeout: hostUITimeout });
    await tavern.evaluate(async avatar => { const ctx = SillyTavern.getContext(); await ctx.getCharacters(); const id = SillyTavern.getContext().characters.findIndex(c => c.avatar === avatar); await SillyTavern.getContext().selectCharacterById(String(id)); }, cardAvatar);
    assert.equal(await tavern.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.session_id), tavernSession);
    assert.equal(await panel.locator('[name=token]').inputValue(), '');
    if (!await panel.isVisible()) await tavern.locator('#extensions-settings-button').click();
    await panel.locator('[name=token]').fill(token); await panel.locator('[data-action=connect]').click();
    await tavern.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
    mcp = await connectMCP(); await tool('corerp_session_resume', read);
    const replay = await tool('corerp_dialogue', mcpSpeech); assert.equal(replay.replayed, true); assert.equal(replay.player_event_id, mcpTurn.player_event_id);
    const after = await compareAll(); assert.deepEqual(history(after), history(before));
    assert.equal(authoritySnapshot(), authority, 'recovery duplicated shared effects');
    assert.deepEqual(errors, []);
    return { clients: ['Play UI', 'SillyTavern1.19.0 UI', 'MCP stdio'], independentSessions: 3, sameObserver: binding.entity_id, worldTime: after.world_time, place: after.place_id, head: after.observation_cursor, sharedHistory: true, automaticStreamReconnect: true, samePeopleAfterRestart: true, originalMCPRetryNoDuplicate: true };
  } finally {
    await mcp?.close(); await play?.close(); await vite.close();
  }
}
