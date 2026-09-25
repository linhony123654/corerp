import assert from 'node:assert/strict';
import test from 'node:test';
import { chooseLiveSpeech, liveProviderConfiguration, ownSceneForModel } from './live-provider.mjs';

test('live resident provider requires explicit project configuration and forwards only own public scene', async () => {
  assert.throws(() => liveProviderConfiguration({}), /CORERP_LIVE_RUN/u);
  assert.throws(() => liveProviderConfiguration({ CORERP_LIVE_RUN: '1', CORERP_LIVE_ENDPOINT: 'http://example.com/chat/completions', CORERP_LIVE_MODEL_A: 'a', CORERP_LIVE_MODEL_B: 'b' }), /HTTPS/u);
  assert.throws(() => liveProviderConfiguration({ CORERP_LIVE_RUN: '1', CORERP_LIVE_ENDPOINT: 'https://example.com/chat/completions', CORERP_LIVE_MODEL_A: 'a', CORERP_LIVE_MODEL_B: 'b' }), /CORERP_LIVE_API_KEY/u);
  const config = liveProviderConfiguration({ CORERP_LIVE_RUN: '1', CORERP_LIVE_ENDPOINT: 'https://example.com/chat/completions', CORERP_LIVE_MODEL_A: 'a', CORERP_LIVE_MODEL_B: 'b', CORERP_LIVE_API_KEY: 'test-project-key' });
  assert.equal(config.responseFormat, 'json_schema');
  assert.equal(config.maxCompletionTokens, 300);
  assert.throws(() => liveProviderConfiguration({ CORERP_LIVE_RUN: '1', CORERP_LIVE_ENDPOINT: config.endpoint, CORERP_LIVE_MODEL_A: 'a', CORERP_LIVE_MODEL_B: 'b', CORERP_LIVE_API_KEY: 'test-project-key', CORERP_LIVE_MAX_COMPLETION_TOKENS: '9000' }), /max completion tokens/u);
  const jsonMode = liveProviderConfiguration({ CORERP_LIVE_RUN: '1', CORERP_LIVE_ENDPOINT: config.endpoint, CORERP_LIVE_MODEL_A: 'a', CORERP_LIVE_MODEL_B: 'b', CORERP_LIVE_API_KEY: 'test-project-key', CORERP_LIVE_RESPONSE_FORMAT: 'json_object', CORERP_LIVE_MAX_COMPLETION_TOKENS: '1024' });
  const scene = ownSceneForModel({
    controlled_entity: { entity_id: 'entity_private_a', display_name: '阿澜' }, world_time: '2026-09-25T08:00:00Z',
    place_name: '街角咖啡馆', present_entities: [{ entity_id: 'entity_private_b', display_name: '一位路人 event_secret_123' }],
    session_id: 'session_secret_123', private_notes: 'other resident hidden state',
  }, ['上一句 entity_private_a'], ['听见 event_private_b 的秘密']);
  assert.equal(scene.present_people[0], '一位路人 [redacted]');
  assert.deepEqual(scene.recently_heard_speech, ['听见 [redacted] 的秘密']);
  let calls = 0;
  const decision = await chooseLiveSpeech(config, 'a', 'A', scene, async (url, options) => {
    calls++;
    assert.equal(url, 'https://example.com/chat/completions');
    assert.equal(options.headers.Authorization, 'Bearer test-project-key');
    assert.equal(options.redirect, 'manual');
    assert.equal(JSON.parse(options.body).response_format.type, 'json_schema');
    assert.equal(JSON.stringify(JSON.parse(options.body)).includes('entity_private_b'), false);
    assert.equal(JSON.stringify(JSON.parse(options.body)).includes('session_secret_123'), false);
    assert.equal(JSON.stringify(JSON.parse(options.body)).includes('other resident hidden state'), false);
    assert.equal(JSON.stringify(JSON.parse(options.body)).includes('event_private_b'), false);
    return new Response(JSON.stringify({ id: 'chatcmpl-fixture-1', model: 'fixture-a', choices: [{ finish_reason: 'stop', message: { content: JSON.stringify({ speech_act: 'question', text: '今天这里热闹吗？' }) } }] }), { status: 200, headers: { 'x-request-id': 'req-fixture-1' } });
  });
  assert.equal(calls, 1);
  assert.deepEqual(decision, { speech_act: 'question', text: '今天这里热闹吗？', provider_receipt: { response_id: 'chatcmpl-fixture-1', response_model: 'fixture-a', request_id: 'req-fixture-1' } });
  const jsonDecision = await chooseLiveSpeech(jsonMode, 'a', 'A', scene, async (_url, options) => {
    const payload = JSON.parse(options.body);
    assert.deepEqual(payload.response_format, { type: 'json_object' });
    assert.equal(payload.max_completion_tokens, 1024);
    return new Response(JSON.stringify({ choices: [{ finish_reason: 'stop', message: { content: JSON.stringify({ speech_act: 'statement', text: '你好。' }) } }] }), { status: 200 });
  });
  assert.equal(jsonDecision.text, '你好。');
  await assert.rejects(() => chooseLiveSpeech(config, 'a', 'A', scene, async () => new Response(JSON.stringify({ choices: [{ finish_reason: 'length', message: { content: '' } }] }), { status: 200 })), /finish=length, refusal=false, tools=false/u);
  await assert.rejects(() => chooseLiveSpeech(config, 'a', 'B', scene, async () => new Response(JSON.stringify({ choices: [{ finish_reason: 'stop', message: { content: JSON.stringify({ speech_act: 'statement', text: 'event_secret_123' }) } }] }), { status: 200 })), /bounded speech/u);
  let cancelled = false;
  const oversized = new ReadableStream({
    start(controller) { controller.enqueue(new Uint8Array(64 * 1024 + 1)); },
    cancel() { cancelled = true; },
  });
  await assert.rejects(() => chooseLiveSpeech(config, 'a', 'A', scene, async () => new Response(oversized, { status: 200 })), /response envelope invalid/u);
  assert.equal(cancelled, true, 'oversized provider response must be cancelled before full buffering');
});
