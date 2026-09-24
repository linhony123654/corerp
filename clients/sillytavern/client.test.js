import assert from 'node:assert/strict';
import test from 'node:test';
import { CoreRPClient, readRPFrames } from './client.js';

function chunks(text, width = 1) {
  const bytes = new TextEncoder().encode(text);
  return new ReadableStream({ start(controller) {
    for (let i = 0; i < bytes.length; i += width) controller.enqueue(bytes.slice(i, i + width));
    controller.close();
  } });
}

test('SSE Unicode chunk boundaries, CRLF, comments and checkpoint IDs', async () => {
  const source = ': keepalive\r\n\r\nid: first\r\nevent: rp_event\r\ndata: {"text":\r\ndata: "你好"}\r\n\r\nid: second\nevent: rp_checkpoint\ndata: {"more_events":false}\n\n';
  const received = [];
  for await (const frame of readRPFrames(chunks(source))) received.push(frame);
  assert.deepEqual(received, [
    { id: 'first', type: 'rp_event', data: { text: '你好' } },
    { id: 'second', type: 'rp_checkpoint', data: { more_events: false } },
  ]);
});

test('partial or malformed frames never yield a checkpoint', async () => {
  for (const source of [
    'id: incomplete\nevent: rp_event\ndata: {}\n',
    'id: invalid\nevent: rp_event\ndata: not-json\n\n',
    'event: rp_event\ndata: {}\n\n',
    'id: key\nevent: world_event\ndata: {}\n\n',
    'id: bad\0key\nevent: rp_event\ndata: {}\n\n',
    'x'.repeat(1024 * 1024 + 1),
  ]) {
    let yielded = false;
    await assert.rejects(async () => { for await (const frame of readRPFrames(chunks(source, 32768))) { void frame; yielded = true; } });
    assert.equal(yielded, false);
  }
});

test('consumer cancellation releases the subscription', async () => {
  let cancelled = false;
  const stream = new ReadableStream({
    start(controller) { controller.enqueue(new TextEncoder().encode('id: key\nevent: rp_event\ndata: {}\n\n')); },
    cancel() { cancelled = true; },
  });
  for await (const frame of readRPFrames(stream)) { assert.equal(frame.id, 'key'); break; }
  assert.equal(cancelled, true);
  assert.equal(stream.locked, false);
});

test('transport excludes cookies, redirects, URL credentials and implicit retries', async () => {
  for (const origin of ['http://remote.example', 'https://user:pass@example.com', 'https://example.com/api', 'https://example.com?key=secret', 'file:///tmp']) {
    assert.throws(() => new CoreRPClient(origin, 'test-token'));
  }
  let calls = 0;
  const client = new CoreRPClient('http://127.0.0.1:8080', 'test-token', async function (url, options) {
    assert.equal(this, globalThis, 'native browser fetch requires its global receiver');
    calls++;
    assert.equal(url, 'http://127.0.0.1:8080/api/v1/rp/turns/run');
    assert.equal(options.credentials, 'omit');
    assert.equal(options.redirect, 'error');
    assert.equal(options.cache, 'no-store');
    assert.equal(options.headers.get('Authorization'), 'Bearer test-token');
    assert.equal(JSON.parse(options.body).idempotency_key, 'saved-key');
    return new Response('secret remote error should not be surfaced', { status: 409 });
  });
  await assert.rejects(client.call('dialogue', { idempotency_key: 'saved-key' }), error => error.status === 409 && !error.message.includes('secret'));
  assert.equal(calls, 1);
  assert.equal(JSON.stringify(client), '{}');
  await assert.rejects(client.call('toString', {}));
  assert.equal(calls, 1);
});

test('HTTP envelope and event header continuation', async () => {
  const client = new CoreRPClient('https://runtime.example.com', 'test-token', async (url, options) => {
    if (url.includes('/events/stream')) {
      assert.equal(new URL(url).searchParams.get('session_id'), 'session one');
      assert.equal(new URL(url).searchParams.has('cursor'), false);
      assert.equal(options.headers.get('Last-Event-ID'), 'old-checkpoint');
      return new Response(chunks('id: new-checkpoint\nevent: rp_checkpoint\ndata: {}\n\n'), { headers: { 'Content-Type': 'text/event-stream' } });
    }
    return Response.json({ data: { session_id: 'session one' } });
  });
  assert.deepEqual(await client.call('open', {}), { session_id: 'session one' });
  const frames = [];
  for await (const frame of client.events('session one', 'old-checkpoint')) frames.push(frame);
  assert.equal(frames[0].id, 'new-checkpoint');
});
