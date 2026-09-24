import assert from 'node:assert/strict';
import test from 'node:test';
import { RuntimeClient } from './runtime.js';

test('fixed origin, credentials and bounded sanitized errors', async () => {
  for (const url of ['http://example.com', 'https://user:password@example.com', 'https://example.com/path', 'https://example.com?token=secret']) assert.throws(() => new RuntimeClient(url, 'test'));
  assert.throws(() => new RuntimeClient('http://localhost:8080', 'bad\ntoken'));
  const token = 'private-token-never-reflect';
  let calls = 0;
  const client = new RuntimeClient('http://127.0.0.1:8080', token, async function(url, options) {
    calls++;
    assert.equal(this, globalThis);
    assert.equal(url, 'http://127.0.0.1:8080/api/v1/rp/observe');
    assert.equal(options.credentials, 'omit'); assert.equal(options.redirect, 'error');
    assert.equal(options.headers.Authorization, `Bearer ${token}`);
    return Response.json({ error: { code: 'PERMISSION_DENIED', message: token } }, { status: 403 });
  });
  const result = await client.call('observe', { session_id: 'own' });
  assert.equal(result.error.code, 'PERMISSION_DENIED');
  assert.equal(JSON.stringify(result).includes(token), false); assert.equal(calls, 1);
  const broken = new RuntimeClient('http://127.0.0.1', token, async () => { throw new Error(token); });
  assert.equal((await broken.call('observe', {})).error.code, 'TRANSPORT_UNCERTAIN');
  const oversized = new RuntimeClient('http://127.0.0.1', token, async () => new Response('x'.repeat(4 * 1024 * 1024 + 1)));
  assert.equal((await oversized.call('observe', {})).error.code, 'TRANSPORT_UNCERTAIN');
});
