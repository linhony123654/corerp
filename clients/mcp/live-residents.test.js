import assert from 'node:assert/strict';
import test from 'node:test';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { access } from 'node:fs/promises';
import { validateLiveResidents } from './live-residents.mjs';
import { liveProviderConfiguration } from './live-provider.mjs';

test('opt-in live driver wiring reaches two distinct MCP residents without a remote provider', { timeout: 180_000 }, async () => {
  const calls = [];
  const provider = createServer(async (request, response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(chunk);
    const body = JSON.parse(Buffer.concat(chunks).toString());
    const { role, own_scene: scene } = JSON.parse(body.messages[1].content);
    assert.doesNotMatch(JSON.stringify(scene), /\b(?:entity|event|principal|session)_[A-Za-z0-9_-]+\b/u);
    calls.push({ model: body.model, role, scene });
    const text = `${role} 的第 ${calls.filter(call => call.role === role).length} 次独立模型占位决策。`;
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end(JSON.stringify({ choices: [{ finish_reason: 'stop', message: { content: JSON.stringify({ speech_act: role === 'A' ? 'question' : 'statement', text }) } }] }));
  });
  provider.listen(0, '127.0.0.1');
  await once(provider, 'listening');
  try {
    const config = liveProviderConfiguration({ CORERP_LIVE_RUN: '1', CORERP_LIVE_ENDPOINT: `http://127.0.0.1:${provider.address().port}/v1/chat/completions`, CORERP_LIVE_MODEL_A: 'fixture-a', CORERP_LIVE_MODEL_B: 'fixture-b' });
    const result = await validateLiveResidents(config, { retainEvidence: false });
    assert.deepEqual(calls.map(call => call.role), ['A', 'B', 'A', 'B']);
    assert.deepEqual(calls.map(call => call.model), ['fixture-a', 'fixture-b', 'fixture-a', 'fixture-b']);
    assert.ok(calls.every(call => call.scene.place && call.scene.character));
    assert.ok(calls[1].scene.recently_heard_speech.includes('A 的第 1 次独立模型占位决策。'));
    assert.ok(calls[2].scene.recently_heard_speech.includes('B 的第 1 次独立模型占位决策。'));
    assert.equal(result.status, 'completed');
    assert.deepEqual(result.model_calls, { A: 2, B: 2 });
    assert.deepEqual(result.accepted_speech_events, { A: 2, B: 2 });
    assert.equal(result.co_located_hearing, true);
    assert.equal(result.evidence_kind, 'provider_provenance_review_required');
    assert.equal(result.decisions.length, 4);
    assert.ok(result.decisions.every(decision => decision.provider_receipt && 'response_id' in decision.provider_receipt));
    await assert.rejects(access(result.disposable_world), { code: 'ENOENT' });
  } finally {
    provider.closeAllConnections();
    await new Promise(resolveClose => provider.close(resolveClose));
  }
});
